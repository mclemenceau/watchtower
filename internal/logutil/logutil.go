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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mclemenceau/watchtower/internal/domain"
	"github.com/mclemenceau/watchtower/internal/ports"
)

// ErrNoLPLog is returned by ResolveLogURL when the Launchpad REST API was
// reachable and returned a valid response for a known build page URL, but
// build_log_url was null. This means Launchpad accepted and ran the build
// but never attached a log — the build was lost or purged before the log
// was uploaded. This is an infrastructure signal, not a product failure:
// there is no build output to analyse.
//
// Callers (AnalyzeLog) must not fall back to the cd-build-log in this case
// because doing so would yield a misleading PRODUCT diagnosis from the
// "Failed to build" line that is actually a symptom of the LP infra problem.
var ErrNoLPLog = errors.New("launchpad build completed with no log available")

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
  "signature": "short-slug-3-to-5-words e.g. apt:missing:libfoo-dev or snap:install:core24",
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
//  4. Fall back to the cd-build-log when Launchpad is unreachable or when
//     no build page URL was found for this arch in the cd-build-log.
//
// Special case: if Launchpad is reachable and returns a valid response for the
// build page but build_log_url is null, ErrNoLPLog is returned. This signals
// that the build ran without producing a log — an infrastructure failure that
// callers must not mask by falling back to the cd-build-log.
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
	if err != nil {
		// Launchpad API unreachable — fall back to cd-build-log.
		src := LogSource{
			URL:         cdLogURL,
			Description: "cd-build-log (Launchpad unavailable)",
		}
		return src, cdContent, nil
	}
	if librarianURL == "" {
		// Launchpad was reachable and returned a valid response, but
		// build_log_url is null. The build ran but produced no log —
		// this is an infrastructure failure. Do NOT fall back to the
		// cd-build-log: its "Failed to build" line would produce a
		// misleading PRODUCT diagnosis.
		return LogSource{}, "", ErrNoLPLog
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

// AnalyzeLog fetches the best available log for the artefact and returns a
// LogAnalysis. It short-circuits the LLM call in two cases:
//
//  1. domain.ExtractFailureSignature recognises a known pattern in the log —
//     the result is built deterministically without an API call.
//  2. ResolveLogURL returns ErrNoLPLog — Launchpad was reachable but the build
//     produced no log. This is an infrastructure failure; a pre-built INFRA
//     analysis is returned and the reclassified kind (BuildFailureKindInfra) is
//     returned as the second value so callers can update the failure record.
//
// The returned BuildFailureKind is non-empty only when the kind differs from
// what ParseBuildStatusFromLog determined at EnrichBuildStatus time (currently
// only the ErrNoLPLog reclassification). Callers should update FailureRecord
// .FailureKind when a non-empty kind is returned.
func AnalyzeLog(
	ctx context.Context,
	art domain.Artefact,
	logFetcher ports.LogFetcher,
	launchpad ports.LaunchpadSource,
	llm ports.LLMClient,
) (domain.LogAnalysis, domain.BuildFailureKind, LogSource, error) {
	src, content, err := ResolveLogURL(ctx, art, logFetcher, launchpad)
	if err != nil {
		if errors.Is(err, ErrNoLPLog) {
			// Launchpad ran the build but produced no log — infra failure.
			// Do not fall back to the cd-build-log; its "Failed to build"
			// line would produce a misleading PRODUCT diagnosis.
			analysis := domain.LogAnalysis{
				Category:   "infra",
				Hypothesis: ErrNoLPLog.Error(),
				Signature:  "lp:missing-build-log",
				NextAction: "Check Launchpad builder health; retry the build",
			}
			return analysis, domain.BuildFailureKindInfra, LogSource{
				Description: "no Launchpad build log available",
			}, nil
		}
		return domain.LogAnalysis{}, "", LogSource{}, err
	}

	truncated := LastNLines(content, 200)

	// Short-circuit: if a known failure pattern is found, build the analysis
	// from the deterministic signature without calling the LLM.
	if sig := domain.ExtractFailureSignature(truncated); sig != "" {
		excerpts := matchingLines(truncated, sig)
		return domain.LogAnalysis{
			Category:    inferCategory(sig),
			Hypothesis:  "Known failure pattern: " + sig,
			Signature:   sig,
			LogExcerpts: excerpts,
			NextAction:  "Search for recent archive changes matching: " + sig,
		}, "", src, nil
	}

	// Novel failure — call the LLM.
	prompt := fmt.Sprintf(
		"Image: %s\n\nBuild log (last 200 lines):\n%s",
		art.Name, truncated,
	)
	raw, err := llm.Complete(ctx, AnalyzeLogSystem, prompt)
	if err != nil {
		return domain.LogAnalysis{}, "", src,
			fmt.Errorf("LLM analysis: %w", err)
	}

	var result domain.LogAnalysis
	if err := json.Unmarshal([]byte(StripCodeFence(raw)), &result); err != nil {
		return domain.LogAnalysis{}, "", src,
			fmt.Errorf("parse LLM response: %w", err)
	}
	return result, "", src, nil
}

// inferCategory returns the LogAnalysis category string that best describes a
// deterministically-matched signature without needing the LLM.
func inferCategory(sig string) string {
	switch {
	case strings.HasPrefix(sig, "apt:") ||
		strings.HasPrefix(sig, "dpkg:") ||
		strings.HasPrefix(sig, "snap:"):
		return "dependency"
	case strings.HasPrefix(sig, "debootstrap:"):
		return "infra"
	default:
		return "unknown"
	}
}

// matchingLines returns up to 3 lines from content that triggered the given
// signature, used as LogExcerpts when short-circuiting the LLM.
func matchingLines(content, sig string) []string {
	// Derive a simple keyword from the signature to search for in the log.
	// e.g. "apt:missing:libfoo-dev" → ["libfoo-dev"], "dpkg:subprocess-error" → ["dpkg"]
	parts := strings.SplitN(sig, ":", 3)
	keywords := make([]string, 0, 2)
	if len(parts) >= 3 && parts[2] != "" {
		keywords = append(keywords, parts[2]) // package/snap name
	}
	if len(parts) >= 1 && parts[0] != "" {
		keywords = append(keywords, parts[0]) // prefix: apt/dpkg/snap/debootstrap
	}

	var out []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		for _, kw := range keywords {
			if strings.Contains(trimmed, kw) {
				out = append(out, trimmed)
				break
			}
		}
		if len(out) >= 3 {
			break
		}
	}
	return out
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
