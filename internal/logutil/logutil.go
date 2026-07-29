// Package logutil provides shared helpers for build log fetching, two-hop
// Launchpad log resolution, LLM analysis, and related string utilities.
//
// It is intentionally kept free of any application or activity imports so that
// both the application layer (interactive investigate command) and the
// activities layer (background FailureAnalysisWorkflow) can import it without
// creating a circular dependency.
package logutil

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mclemenceau/watchtower/internal/domain"
	"github.com/mclemenceau/watchtower/internal/ports"
)

// AnalyzeLogSystem is the system prompt used for all LLM log analysis calls,
// both interactive (investigate command) and background (FailureAnalysisWorkflow).
// Keeping it in one place ensures consistent behaviour and makes prompt
// iteration easy.
const AnalyzeLogSystem = `You are a build failure analyst for Ubuntu image builds.
Given a build log, identify the root cause of the failure.
Respond with valid JSON only — no markdown, no extra text:
{
  "category": "infra|code|dependency|flaky|unknown",
  "hypothesis": "one-sentence root cause",
  "log_excerpts": ["most relevant line 1", "most relevant line 2"],
  "next_action": "recommended next step for the engineer"
}`

// LogSource describes which log was ultimately analysed.
type LogSource struct {
	URL         string
	Description string // shown to the user in investigation reports
}

// ResolveLogURL performs the two-hop resolution from an artefact to the best
// available log URL:
//
//  1. Derive the cd-build-log URL from art.ImageURL for today's date.
//  2. Fetch that log and scan for per-arch Launchpad build page URLs.
//  3. If a Launchpad build page URL is found for the primary arch, call the
//     Launchpad REST API to get the librarian log URL.
//  4. Fall back to the cd-build-log whenever any step fails or returns nothing.
//
// launchpad may be nil — in that case step 3 is skipped and the cd-build-log
// content fetched in step 2 is returned directly.
func ResolveLogURL(
	ctx context.Context,
	art domain.Artefact,
	logFetcher ports.LogFetcher,
	launchpad ports.LaunchpadSource,
) (LogSource, string, error) {
	// Always use today's date for the cd-build-log URL — the date embedded in
	// Artefact.ImageURL is the last successful build, not the current failing one.
	today := time.Now().UTC().Format("20060102")
	cdLogURL := domain.LogURLFromImageURLForDate(art.ImageURL, today)
	if cdLogURL == "" {
		return LogSource{}, "", fmt.Errorf(
			"no log URL available: image URL is missing or unrecognised",
		)
	}

	// Fetch the cd-build-log (always needed — either as fallback content or to
	// extract per-arch Launchpad build page URLs).
	cdContent, err := logFetcher.Fetch(ctx, cdLogURL)
	if err != nil {
		return LogSource{}, "", fmt.Errorf("fetch cd-build-log: %w", err)
	}

	// If no Launchpad resolver is configured, use the cd-build-log as-is.
	if launchpad == nil {
		src := LogSource{URL: cdLogURL, Description: "cd-build-log"}
		return src, cdContent, nil
	}

	// Determine the primary architecture to investigate.
	arch := domain.PrimaryBuildArch(art.Builds)
	if arch == "" {
		arch = "amd64" // sensible default when no build data in snapshot
	}

	// Parse Launchpad build page URLs out of the cd-build-log.
	lpURLs := domain.ParseLaunchpadBuildURLs(cdContent)
	buildPageURL := MatchLaunchpadURL(lpURLs, arch)
	if buildPageURL == "" {
		// No Launchpad link found for this arch — fall back to cd-build-log.
		src := LogSource{URL: cdLogURL, Description: "cd-build-log"}
		return src, cdContent, nil
	}

	// Resolve the librarian log URL via the Launchpad REST API.
	librarianURL, err := launchpad.FetchBuildLogURL(ctx, buildPageURL)
	if err != nil || librarianURL == "" {
		// Launchpad API unavailable or log not yet posted — fall back.
		src := LogSource{
			URL:         cdLogURL,
			Description: "cd-build-log (Launchpad unavailable)",
		}
		return src, cdContent, nil
	}

	// Fetch the actual Launchpad librarian log (may be gzip-compressed).
	libContent, err := logFetcher.Fetch(ctx, librarianURL)
	if err != nil {
		// Librarian fetch failed — fall back to cd-build-log content.
		src := LogSource{
			URL:         cdLogURL,
			Description: "cd-build-log (librarian fetch failed)",
		}
		return src, cdContent, nil
	}

	src := LogSource{
		URL:         librarianURL,
		Description: fmt.Sprintf("Launchpad librarian (%s)", arch),
	}
	return src, libContent, nil
}

// AnalyzeLog fetches the best available log for the artefact, truncates it,
// calls the LLM, and returns the parsed LogAnalysis plus the human-readable
// source description. It is the single implementation used by both the
// interactive investigate command and the background FailureAnalysisWorkflow.
func AnalyzeLog(
	ctx context.Context,
	art domain.Artefact,
	logFetcher ports.LogFetcher,
	launchpad ports.LaunchpadSource,
	llm ports.LLMClient,
) (domain.LogAnalysis, LogSource, error) {
	src, content, err := ResolveLogURL(ctx, art, logFetcher, launchpad)
	if err != nil {
		return domain.LogAnalysis{}, LogSource{}, err
	}

	truncated := LastNLines(content, 200)
	prompt := fmt.Sprintf(
		"Image: %s\n\nBuild log (last 200 lines):\n%s",
		art.Name, truncated,
	)
	raw, err := llm.Complete(ctx, AnalyzeLogSystem, prompt)
	if err != nil {
		return domain.LogAnalysis{}, src,
			fmt.Errorf("LLM analysis: %w", err)
	}

	var result domain.LogAnalysis
	if err := json.Unmarshal([]byte(StripCodeFence(raw)), &result); err != nil {
		return domain.LogAnalysis{}, src,
			fmt.Errorf("parse LLM response: %w", err)
	}
	return result, src, nil
}

// LastNLines returns the last n lines of text.
func LastNLines(text string, n int) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= n {
		return text
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// MatchLaunchpadURL finds the Launchpad build page URL in lpURLs whose label
// best matches the given arch string. The cd-build-log uses full build labels
// (e.g. "amd64", "desktop-preinstalled-arm64-raspi") not bare arch strings
// (e.g. "arm64+raspi"). Matching normalises "+" to "-" in both the arch and
// label, then checks for exact match followed by substring match.
//
// Returns "" when no matching label is found.
func MatchLaunchpadURL(lpURLs map[string]string, arch string) string {
	normArch := strings.ReplaceAll(arch, "+", "-")
	for label, url := range lpURLs {
		normLabel := strings.ReplaceAll(label, "+", "-")
		if normLabel == normArch {
			return url
		}
		if strings.Contains(normLabel, normArch) {
			return url
		}
	}
	return ""
}

// StripCodeFence removes ```json ... ``` wrappers that some models add.
func StripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if i := strings.Index(s, "\n"); i != -1 {
		s = s[i+1:]
	}
	if i := strings.LastIndex(s, "```"); i != -1 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
