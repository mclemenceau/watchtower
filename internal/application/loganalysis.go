package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mclemenceau/watchtower/internal/domain"
	"github.com/mclemenceau/watchtower/internal/ports"
)

const investigateSystemPrompt = `You are a build failure analyst for Ubuntu image builds.
Given a build log, identify the root cause of the failure.
Respond with valid JSON only — no markdown, no extra text:
{
  "category": "infra|code|dependency|flaky|unknown",
  "hypothesis": "one-sentence root cause",
  "log_excerpts": ["most relevant line 1", "most relevant line 2"],
  "next_action": "recommended next step for the engineer"
}`

// logSource describes which log was ultimately analysed.
type logSource struct {
	url         string
	description string // shown to the user in the investigation report
}

// resolveLogURL performs the two-hop resolution from an artefact to the best
// available log URL:
//
//  1. Derive the cd-build-log URL from art.ImageURL.
//  2. Fetch that log and scan for per-arch Launchpad build page URLs.
//  3. If a Launchpad build page URL is found for the primary arch, call the
//     Launchpad REST API to get the librarian log URL.
//  4. Fall back to the cd-build-log whenever any step fails or returns nothing.
//
// launchpad may be nil — in that case step 3 is skipped and the cd-build-log
// content fetched in step 2 is returned directly.
func resolveLogURL(
	ctx context.Context,
	art domain.Artefact,
	logFetcher ports.LogFetcher,
	launchpad ports.LaunchpadSource,
) (logSource, string, error) {
	// Always use today's date for the cd-build-log URL — the date embedded in
	// Artefact.ImageURL is the last successful build, not the current failing one.
	today := time.Now().UTC().Format("20060102")
	cdLogURL := domain.LogURLFromImageURLForDate(art.ImageURL, today)
	if cdLogURL == "" {
		return logSource{}, "", fmt.Errorf("no log URL available: image URL is missing or unrecognised")
	}

	// Fetch the cd-build-log (always needed — either as fallback content or to
	// extract per-arch Launchpad build page URLs).
	cdContent, err := logFetcher.Fetch(ctx, cdLogURL)
	if err != nil {
		return logSource{}, "", fmt.Errorf("fetch cd-build-log: %w", err)
	}

	// If no Launchpad resolver is configured, use the cd-build-log as-is.
	if launchpad == nil {
		src := logSource{url: cdLogURL, description: "cd-build-log"}
		return src, cdContent, nil
	}

	// Determine the primary architecture to investigate.
	arch := domain.PrimaryBuildArch(art.Builds)
	if arch == "" {
		arch = "amd64" // sensible default when no build data in snapshot
	}

	// Parse Launchpad build page URLs out of the cd-build-log.
	// The map keys are the full build labels from the log (e.g. "amd64",
	// "desktop-preinstalled-arm64-raspi"), NOT necessarily bare arch strings.
	// Match by looking for a label that contains the arch (normalising "+" to "-").
	lpURLs := domain.ParseLaunchpadBuildURLs(cdContent)
	buildPageURL := matchLaunchpadURL(lpURLs, arch)
	if buildPageURL == "" {
		// No Launchpad link found for this arch — fall back to cd-build-log.
		src := logSource{url: cdLogURL, description: "cd-build-log"}
		return src, cdContent, nil
	}

	// Resolve the librarian log URL via the Launchpad REST API.
	librarianURL, err := launchpad.FetchBuildLogURL(ctx, buildPageURL)
	if err != nil || librarianURL == "" {
		// Launchpad API unavailable or log not yet posted — fall back.
		src := logSource{url: cdLogURL, description: "cd-build-log (Launchpad unavailable)"}
		return src, cdContent, nil
	}

	// Fetch the actual Launchpad librarian log (may be gzip-compressed).
	libContent, err := logFetcher.Fetch(ctx, librarianURL)
	if err != nil {
		// Librarian fetch failed — fall back to cd-build-log content.
		src := logSource{url: cdLogURL, description: "cd-build-log (librarian fetch failed)"}
		return src, cdContent, nil
	}

	src := logSource{
		url:         librarianURL,
		description: fmt.Sprintf("Launchpad librarian (%s)", arch),
	}
	return src, libContent, nil
}

// analyzeLog fetches the best available log for the artefact and runs LLM
// root-cause analysis on it. Returns the analysis and the human-readable
// source description for display.
func analyzeLog(
	ctx context.Context,
	art domain.Artefact,
	logFetcher ports.LogFetcher,
	launchpad ports.LaunchpadSource,
	llm ports.LLMClient,
) (domain.LogAnalysis, string, error) {
	src, content, err := resolveLogURL(ctx, art, logFetcher, launchpad)
	if err != nil {
		return domain.LogAnalysis{}, "", err
	}

	truncated := lastNLines(content, 200)
	prompt := fmt.Sprintf("Image: %s\n\nBuild log (last 200 lines):\n%s", art.Name, truncated)
	raw, err := llm.Complete(ctx, investigateSystemPrompt, prompt)
	if err != nil {
		return domain.LogAnalysis{}, src.description, fmt.Errorf("LLM analysis: %w", err)
	}

	var result domain.LogAnalysis
	if err := json.Unmarshal([]byte(stripAnalysisFence(raw)), &result); err != nil {
		return domain.LogAnalysis{}, src.description, fmt.Errorf("parse LLM response: %w", err)
	}
	return result, src.description, nil
}

// lastNLines returns the last n lines of text.
func lastNLines(text string, n int) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= n {
		return text
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// matchLaunchpadURL finds the Launchpad build page URL in lpURLs whose label
// best matches the given arch string. The cd-build-log uses full build labels
// (e.g. "amd64", "desktop-preinstalled-arm64-raspi") not bare arch strings
// (e.g. "arm64+raspi"). Matching normalises "+" to "-" in both the arch and
// label, then checks for exact match followed by substring match.
//
// Returns "" when no matching label is found.
func matchLaunchpadURL(lpURLs map[string]string, arch string) string {
	// Normalise "+" separators to "-" so "arm64+raspi" → "arm64-raspi".
	normArch := strings.ReplaceAll(arch, "+", "-")

	for label, url := range lpURLs {
		normLabel := strings.ReplaceAll(label, "+", "-")
		// Exact match (covers simple arches like "amd64").
		if normLabel == normArch {
			return url
		}
		// Substring match: label contains the normalised arch
		// (covers "desktop-preinstalled-arm64-raspi" containing "arm64-raspi").
		if strings.Contains(normLabel, normArch) {
			return url
		}
	}
	return ""
}

// stripAnalysisFence removes ```json ... ``` wrappers that some models add.
func stripAnalysisFence(s string) string {
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
