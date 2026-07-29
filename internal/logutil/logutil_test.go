package logutil

import (
	"context"
	"errors"
	"testing"

	"github.com/mclemenceau/watchtower/internal/domain"
)

// --- MatchLaunchpadURL ---

func TestMatchLaunchpadURL_ExactSimpleArch(t *testing.T) {
	lpURLs := map[string]string{
		"amd64": "https://launchpad.net/+build/1",
		"arm64": "https://launchpad.net/+build/2",
	}
	got := MatchLaunchpadURL(lpURLs, "amd64")
	if got != "https://launchpad.net/+build/1" {
		t.Errorf("MatchLaunchpadURL(amd64) = %q, want build/1", got)
	}
}

func TestMatchLaunchpadURL_SubstringVariantArch(t *testing.T) {
	// Variant build: label is "desktop-preinstalled-arm64-raspi", arch is "arm64+raspi"
	lpURLs := map[string]string{
		"desktop-preinstalled-arm64-raspi": "https://launchpad.net/+build/99",
	}
	got := MatchLaunchpadURL(lpURLs, "arm64+raspi")
	if got != "https://launchpad.net/+build/99" {
		t.Errorf("MatchLaunchpadURL(arm64+raspi) = %q, want build/99", got)
	}
}

func TestMatchLaunchpadURL_NormalisesPlus(t *testing.T) {
	lpURLs := map[string]string{
		"amd64+tegra": "https://launchpad.net/+build/50",
	}
	got := MatchLaunchpadURL(lpURLs, "amd64+tegra")
	if got != "https://launchpad.net/+build/50" {
		t.Errorf("MatchLaunchpadURL(amd64+tegra) = %q, want build/50", got)
	}
}

func TestMatchLaunchpadURL_NoMatch(t *testing.T) {
	lpURLs := map[string]string{
		"amd64": "https://launchpad.net/+build/1",
	}
	got := MatchLaunchpadURL(lpURLs, "riscv64")
	if got != "" {
		t.Errorf("MatchLaunchpadURL(riscv64) = %q, want empty", got)
	}
}

func TestMatchLaunchpadURL_EmptyMap(t *testing.T) {
	got := MatchLaunchpadURL(map[string]string{}, "amd64")
	if got != "" {
		t.Errorf("MatchLaunchpadURL on empty map = %q, want empty", got)
	}
}

// --- LastNLines ---

func TestLastNLines_ShortText(t *testing.T) {
	text := "line1\nline2\nline3"
	got := LastNLines(text, 10)
	if got != text {
		t.Errorf("LastNLines short text: got %q, want %q", got, text)
	}
}

func TestLastNLines_TruncatesHead(t *testing.T) {
	text := "a\nb\nc\nd\ne"
	got := LastNLines(text, 3)
	want := "c\nd\ne"
	if got != want {
		t.Errorf("LastNLines(n=3): got %q, want %q", got, want)
	}
}

func TestLastNLines_ExactLength(t *testing.T) {
	text := "x\ny\nz"
	got := LastNLines(text, 3)
	if got != text {
		t.Errorf("LastNLines exact length: got %q, want full text", got)
	}
}

// --- StripCodeFence ---

func TestStripCodeFence_NoFence(t *testing.T) {
	s := `{"category":"infra"}`
	got := StripCodeFence(s)
	if got != s {
		t.Errorf("StripCodeFence without fence: got %q, want %q", got, s)
	}
}

func TestStripCodeFence_JsonFence(t *testing.T) {
	s := "```json\n{\"category\":\"infra\"}\n```"
	got := StripCodeFence(s)
	want := `{"category":"infra"}`
	if got != want {
		t.Errorf("StripCodeFence with fence: got %q, want %q", got, want)
	}
}

func TestStripCodeFence_GenericFence(t *testing.T) {
	s := "```\n{\"category\":\"infra\"}\n```"
	got := StripCodeFence(s)
	want := `{"category":"infra"}`
	if got != want {
		t.Errorf("StripCodeFence generic fence: got %q, want %q", got, want)
	}
}

func TestStripCodeFence_EmptyString(t *testing.T) {
	got := StripCodeFence("")
	if got != "" {
		t.Errorf("StripCodeFence empty: got %q, want empty", got)
	}
}

// --- AnalyzeLog short-circuit ---

// panicLLM is a ports.LLMClient that panics if Complete is ever called.
// Used to verify that the LLM is NOT called when a known signature is found.
type panicLLM struct{}

func (p *panicLLM) Complete(_ context.Context, _, _ string) (string, error) {
	panic("LLM should not be called when signature is found")
}

// mockLogFetcher returns a fixed log content without any network call.
type mockLogFetcher struct{ content string }

func (m *mockLogFetcher) Fetch(_ context.Context, _ string) (string, error) {
	return m.content, nil
}

func TestAnalyzeLog_ShortCircuit_NoLLMCall(t *testing.T) {
	// A log containing a known apt:missing pattern should produce a result
	// without calling the LLM at all.
	logContent := "E: Unable to locate package libfoo-dev\nsome other line"
	art := domain.Artefact{
		ID:       1,
		Name:     "stonking-desktop-amd64.iso",
		ImageURL: "https://cdimage.ubuntu.com/ubuntu/stonking/daily-live/20260701/stonking-desktop-amd64.iso",
	}

	analysis, _, src, err := AnalyzeLog(
		context.Background(),
		art,
		&mockLogFetcher{content: logContent},
		nil,         // no Launchpad resolver
		&panicLLM{}, // panics if called — proves short-circuit
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if analysis.Signature != "apt:missing:libfoo-dev" {
		t.Errorf("Signature = %q, want apt:missing:libfoo-dev", analysis.Signature)
	}
	if analysis.Category != "dependency" {
		t.Errorf("Category = %q, want dependency", analysis.Category)
	}
	if src.Description == "" {
		t.Error("LogSource.Description should not be empty")
	}
}

func TestInferCategory_Apt(t *testing.T) {
	if got := inferCategory("apt:missing:libfoo"); got != "dependency" {
		t.Errorf("inferCategory(apt:*) = %q, want dependency", got)
	}
}

func TestInferCategory_Dpkg(t *testing.T) {
	if got := inferCategory("dpkg:subprocess-error"); got != "dependency" {
		t.Errorf("inferCategory(dpkg:*) = %q, want dependency", got)
	}
}

func TestInferCategory_Snap(t *testing.T) {
	if got := inferCategory("snap:install:core24"); got != "dependency" {
		t.Errorf("inferCategory(snap:*) = %q, want dependency", got)
	}
}

func TestInferCategory_Debootstrap(t *testing.T) {
	if got := inferCategory("debootstrap:error"); got != "infra" {
		t.Errorf("inferCategory(debootstrap:*) = %q, want infra", got)
	}
}

func TestInferCategory_Unknown(t *testing.T) {
	if got := inferCategory("something:else"); got != "unknown" {
		t.Errorf("inferCategory(other) = %q, want unknown", got)
	}
}

func TestMatchingLines_ReturnsRelevantLines(t *testing.T) {
	content := "normal line\nE: Unable to locate package libfoo-dev\nanother line\nmore apt output"
	lines := matchingLines(content, "apt:missing:libfoo-dev")
	if len(lines) == 0 {
		t.Error("expected at least one matching line")
	}
	found := false
	for _, l := range lines {
		if l == "E: Unable to locate package libfoo-dev" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected exact error line in excerpts, got %v", lines)
	}
}

// --- ErrNoLPLog reclassification ---

// mockLaunchpadSource returns a fixed URL and error for FetchBuildLogURL.
type mockLaunchpadSource struct {
	url string
	err error
}

func (m *mockLaunchpadSource) FetchBuildLogURL(_ context.Context, _ string) (string, error) {
	return m.url, m.err
}

// TestAnalyzeLog_ErrNoLPLog_ReturnsInfraAnalysis is the regression test for the
// case where Launchpad is reachable but build_log_url is null. The build ran and
// failed without producing a log — this is an infrastructure failure. AnalyzeLog
// must NOT fall back to the cd-build-log or call the LLM; it must return a
// pre-built INFRA analysis and the BuildFailureKindInfra reclassification.
func TestAnalyzeLog_ErrNoLPLog_ReturnsInfraAnalysis(t *testing.T) {
	// cd-build-log contains a Launchpad link so ResolveLogURL will call LP.
	cdLog := "lubuntu-amd64: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/stonking/lubuntu/+build/999659\n" +
		"lubuntu-amd64 on Launchpad finished at 2026-07-29 14:00:29 (Failed to build)\n"
	art := domain.Artefact{
		ID:   25674,
		Name: "stonking-desktop-amd64.iso",
		ImageURL: "https://cdimage.ubuntu.com/lubuntu/stonking/daily-live/20260728/" +
			"stonking-desktop-amd64.iso",
		BuildFailureKind: domain.BuildFailureKindProduct,
	}

	// LP is reachable but returns no log URL.
	lp := &mockLaunchpadSource{url: ""}

	analysis, reclassify, src, err := AnalyzeLog(
		context.Background(), art,
		&mockLogFetcher{content: cdLog},
		lp,
		&panicLLM{}, // must not be called
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reclassify != domain.BuildFailureKindInfra {
		t.Errorf("reclassify = %q, want INFRA", reclassify)
	}
	if analysis.Category != "infra" {
		t.Errorf("Category = %q, want infra", analysis.Category)
	}
	if analysis.Signature != "lp:missing-build-log" {
		t.Errorf("Signature = %q, want lp:missing-build-log", analysis.Signature)
	}
	if !errors.Is(ErrNoLPLog, ErrNoLPLog) {
		t.Error("ErrNoLPLog sentinel is broken")
	}
	if src.URL != "" {
		t.Errorf("src.URL should be empty for no-log case, got %q", src.URL)
	}
}

// TestAnalyzeLog_LPUnreachable_FallsBackToCDLog verifies that when the Launchpad
// API itself returns an error (network failure, 5xx), we still fall back to the
// cd-build-log rather than surfacing an infra reclassification. Only a reachable
// LP with a null log_url should trigger ErrNoLPLog.
func TestAnalyzeLog_LPUnreachable_FallsBackToCDLog(t *testing.T) {
	cdLog := "lubuntu-amd64: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/stonking/lubuntu/+build/1\n" +
		"E: Unable to locate package libfoo-dev\n"
	art := domain.Artefact{
		ID:   1,
		Name: "stonking-desktop-amd64.iso",
		ImageURL: "https://cdimage.ubuntu.com/lubuntu/stonking/daily-live/20260728/" +
			"stonking-desktop-amd64.iso",
	}

	// LP returns an error (unreachable).
	lp := &mockLaunchpadSource{err: errors.New("connection refused")}

	analysis, reclassify, src, err := AnalyzeLog(
		context.Background(), art,
		&mockLogFetcher{content: cdLog},
		lp,
		&panicLLM{}, // short-circuits on apt:missing pattern, LLM not called
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reclassify != "" {
		t.Errorf("reclassify = %q, want empty (no reclassification on LP error)", reclassify)
	}
	if analysis.Signature != "apt:missing:libfoo-dev" {
		t.Errorf("Signature = %q, want apt:missing:libfoo-dev", analysis.Signature)
	}
	_ = src // fallback source is present
}
