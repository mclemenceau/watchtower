package application

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mclemenceau/watchtower/internal/domain"
)

// captureNotifier records the last message sent via Send.
type captureNotifier struct {
	last string
	err  error
}

func (c *captureNotifier) Send(text string) error {
	c.last = text
	return c.err
}

var (
	today     = time.Now().UTC().Format("20060102")
	yesterday = time.Now().UTC().AddDate(0, 0, -1).Format("20060102")
)

var testArtefacts = []domain.Artefact{
	// noble: 1 built today, 1 not built (yesterday)
	{ID: 1, Name: "ubuntu-desktop-amd64", OS: "ubuntu", Release: "noble", Version: today},
	{ID: 2, Name: "ubuntu-server-amd64", OS: "ubuntu-server", Release: "noble", Version: yesterday},
	// plucky: 1 built today
	{ID: 3, Name: "ubuntu-desktop-amd64", OS: "ubuntu", Release: "plucky", Version: today},
}

// testArtefactsWithBuilds has artefacts whose Builds field is populated with
// realistic test execution data (no live API calls needed).
var testArtefactsWithBuilds = func() []domain.Artefact {
	env := func(name, arch string) domain.Environment {
		return domain.Environment{Name: name, Architecture: arch}
	}
	return []domain.Artefact{
		// 1001 — plucky desktop amd64: Jenkins FAILED (displayable)
		{
			ID: 1001, Name: "plucky-desktop-amd64.iso", OS: "ubuntu", Release: "plucky", Version: today,
			Builds: []domain.ArtefactBuild{{
				ID: 2001, Architecture: "amd64",
				TestExecutions: []domain.TestExecution{
					{ID: 3001, TestPlan: "Image build", Status: "PASSED", Environment: env("cdimage.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T06:00:00"},
					{ID: 3002, TestPlan: "Jenkins image validation", Status: "FAILED", CILink: "https://platform-qa-jenkins.ps5.ubuntu.com/job/ubuntu-plucky-desktop-amd64-iso-static-validation/1/", Environment: env("platform-qa-jenkins.ps5.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T07:00:00"},
					{ID: 3003, TestPlan: "Manual Testing", Status: "IN_PROGRESS", Environment: env("user manual tests", "amd64"), CreatedAt: "2026-06-26T06:01:00"},
				},
			}},
		},
		// 1002 — plucky desktop arm64: no displayable executions
		{
			ID: 1002, Name: "plucky-desktop-arm64.iso", OS: "ubuntu", Release: "plucky", Version: today,
			Builds: []domain.ArtefactBuild{{
				ID: 2002, Architecture: "arm64",
				TestExecutions: []domain.TestExecution{
					{ID: 3004, TestPlan: "Image build", Status: "PASSED", Environment: env("cdimage.ubuntu.com", "arm64"), CreatedAt: "2026-06-26T06:00:00"},
					{ID: 3005, TestPlan: "Manual Testing", Status: "IN_PROGRESS", Environment: env("user manual tests", "arm64"), CreatedAt: "2026-06-26T06:01:00"},
				},
			}},
		},
		// 1003 — plucky server amd64: Jenkins PASSED (displayable)
		{
			ID: 1003, Name: "plucky-server-amd64.iso", OS: "ubuntu-server", Release: "plucky", Version: today,
			Builds: []domain.ArtefactBuild{{
				ID: 2003, Architecture: "amd64",
				TestExecutions: []domain.TestExecution{
					{ID: 3006, TestPlan: "Image build", Status: "PASSED", Environment: env("cdimage.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T06:00:00"},
					{ID: 3007, TestPlan: "Jenkins image validation", Status: "PASSED", CILink: "https://platform-qa-jenkins.ps5.ubuntu.com/job/ubuntu-plucky-server-amd64-iso-static-validation/1/", Environment: env("platform-qa-jenkins.ps5.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T07:00:00"},
					{ID: 3008, TestPlan: "Manual Testing", Status: "IN_PROGRESS", Environment: env("user manual tests", "amd64"), CreatedAt: "2026-06-26T06:01:00"},
				},
			}},
		},
		// 1004 — plucky minimal: no displayable executions
		{
			ID: 1004, Name: "plucky-minimal-amd64.iso", OS: "ubuntu-minimal", Release: "plucky", Version: yesterday,
			Builds: []domain.ArtefactBuild{{
				ID: 2004, Architecture: "amd64",
				TestExecutions: []domain.TestExecution{
					{ID: 3009, TestPlan: "Image build", Status: "PASSED", Environment: env("cdimage.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T06:00:00"},
					{ID: 3010, TestPlan: "Manual Testing", Status: "IN_PROGRESS", Environment: env("user manual tests", "amd64"), CreatedAt: "2026-06-26T06:01:00"},
				},
			}},
		},
		// 1005 — noble desktop amd64: Jenkins PASSED + Manual Testing PASSED (both displayable)
		{
			ID: 1005, Name: "noble-desktop-amd64.iso", OS: "ubuntu", Release: "noble", Version: yesterday,
			Builds: []domain.ArtefactBuild{{
				ID: 2005, Architecture: "amd64",
				TestExecutions: []domain.TestExecution{
					{ID: 3011, TestPlan: "Image build", Status: "PASSED", Environment: env("cdimage.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T06:00:00"},
					{ID: 3012, TestPlan: "Jenkins image validation", Status: "PASSED", CILink: "https://platform-qa-jenkins.ps5.ubuntu.com/job/ubuntu-noble-desktop-amd64-iso-static-validation/1/", Environment: env("platform-qa-jenkins.ps5.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T07:00:00"},
					{ID: 3013, TestPlan: "Manual Testing", Status: "PASSED", Environment: env("user manual tests", "amd64"), CreatedAt: "2026-06-26T08:00:00"},
				},
			}},
		},
	}
}()

// --- help ---

func TestDispatchHelp(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "help", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"builds status", "builds status <release>", "tests status", "help"} {
		if !strings.Contains(hook.last, want) {
			t.Errorf("help output missing %q, got:\n%s", want, hook.last)
		}
	}
}

func TestDispatchHelpCaseInsensitive(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "HELP", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "builds status") {
		t.Errorf("help output missing 'builds status' command")
	}
}

// --- summary command ---

func TestDispatchSummary(t *testing.T) {
	hook := &captureNotifier{}
	// releasesScope = ["noble"] so only noble appears.
	if err := Dispatch(context.Background(), "test", "summary", testArtefacts, nil, []string{"noble"}, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "Build Summary") {
		t.Errorf("expected 'Build Summary' header, got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "noble") {
		t.Errorf("expected 'noble' in summary output, got:\n%s", hook.last)
	}
	// plucky is in testArtefacts but not in releasesScope
	if strings.Contains(hook.last, "plucky") {
		t.Errorf("'plucky' should not appear when not in releasesScope, got:\n%s", hook.last)
	}
}

func TestDispatchSummaryNilReleases(t *testing.T) {
	hook := &captureNotifier{}
	// nil summaryForReleases → all releases
	if err := Dispatch(context.Background(), "test", "summary", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "noble") {
		t.Errorf("expected 'noble' in all-releases summary, got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "plucky") {
		t.Errorf("expected 'plucky' in all-releases summary, got:\n%s", hook.last)
	}
}

func TestDispatchSummaryEmptySnapshot(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "summary", nil, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "No snapshot") {
		t.Errorf("expected 'No snapshot' message, got:\n%s", hook.last)
	}
}

// --- builds status (summary) ---

func TestDispatchBuildsStatus(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "builds status", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "noble") {
		t.Errorf("builds status missing 'noble', got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "plucky") {
		t.Errorf("builds status missing 'plucky', got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "🟩") {
		t.Errorf("builds status missing green squares, got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "🟥") {
		t.Errorf("builds status missing red squares, got:\n%s", hook.last)
	}
}

func TestDispatchBuildsStatusReleasesScope(t *testing.T) {
	// releasesScope = ["noble"] — builds status must only show noble even though
	// the snapshot contains both noble and plucky.
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "builds status", testArtefacts, nil, []string{"noble"}, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "noble") {
		t.Errorf("builds status with scope missing 'noble', got:\n%s", hook.last)
	}
	if strings.Contains(hook.last, "plucky") {
		t.Errorf("'plucky' should not appear when not in releasesScope, got:\n%s", hook.last)
	}
}

func TestDispatchBuildsStatusCaseInsensitive(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "Builds Status", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "noble") {
		t.Errorf("builds status case-insensitive failed, got:\n%s", hook.last)
	}
}

func TestDispatchBuildsStatusEmptySnapshot(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "builds status", nil, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "No snapshot") {
		t.Errorf("expected 'No snapshot' message, got: %s", hook.last)
	}
}

// --- builds status <release> (detail) ---

func TestDispatchBuildsStatusRelease(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "builds status noble", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "noble") {
		t.Errorf("builds status noble missing 'noble', got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "ubuntu-desktop-amd64") {
		t.Errorf("builds status noble missing artefact name, got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "ubuntu-server-amd64") {
		t.Errorf("builds status noble missing second artefact, got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "✅") {
		t.Errorf("builds status noble missing ✅, got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "❌") {
		t.Errorf("builds status noble missing ❌, got:\n%s", hook.last)
	}
	if strings.Contains(hook.last, "plucky") {
		t.Errorf("builds status noble should not contain plucky artefact")
	}
}

func TestDispatchBuildsStatusReleaseCaseInsensitive(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "builds status Noble", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "ubuntu-desktop-amd64") {
		t.Errorf("builds status Noble should return noble artefacts, got:\n%s", hook.last)
	}
}

func TestDispatchBuildsStatusReleaseUnknown(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "builds status nonexistent", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "No artefacts found") {
		t.Errorf("expected 'No artefacts found' message, got: %s", hook.last)
	}
}

// --- builds status: log hyperlink for unbuilt artefact ---

func TestDispatchBuildsStatusReleaseLogLink(t *testing.T) {
	imageURL := "https://cdimage.ubuntu.com/ubuntu-server/noble/daily-live/20200101/noble-live-server-amd64.iso"
	// LogCell always uses today's date, not the date embedded in imageURL.
	todayDate := time.Now().UTC().Format("20060102")
	logURL := "https://ubuntu-archive-team.ubuntu.com/cd-build-logs/ubuntu-server/noble/daily-live-" + todayDate + ".log"
	artefacts := []domain.Artefact{
		{ID: 1, Name: "ubuntu-server-amd64", OS: "ubuntu-server", Release: "noble", Version: "20200101", ImageURL: imageURL},
		{ID: 2, Name: "ubuntu-desktop-amd64", OS: "ubuntu", Release: "noble", Version: "20200101"},
	}
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "builds status noble", artefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Artefact with imageURL should have a 🔗 log link in the Log column
	wantLink := "[🔗](" + logURL + ")"
	if !strings.Contains(hook.last, wantLink) {
		t.Errorf("expected Markdown log hyperlink %q in output, got:\n%s", wantLink, hook.last)
	}
	// Artefact without imageURL should have ❌ in the Log column
	serverRow := "| ubuntu-server-amd64 | ubuntu-server |"
	desktopRow := "| ubuntu-desktop-amd64 | ubuntu |"
	for _, line := range strings.Split(hook.last, "\n") {
		if strings.Contains(line, serverRow) && !strings.Contains(line, wantLink) {
			t.Errorf("server artefact row should contain log link %q; got line:\n%s", wantLink, line)
		}
		if strings.Contains(line, desktopRow) && !strings.HasSuffix(strings.TrimSpace(line), "| ❌ |") {
			t.Errorf("desktop artefact row (no imageURL) should end with '| ❌ |'; got line:\n%s", line)
		}
	}
}

// --- builds status <release> <product> (product filter) ---

func TestDispatchBuildsStatusReleaseProduct(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "builds status noble ubuntu-server", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "ubuntu-server-amd64") {
		t.Errorf("expected ubuntu-server-amd64 in output, got:\n%s", hook.last)
	}
	if strings.Contains(hook.last, "ubuntu-desktop-amd64") {
		t.Errorf("ubuntu-desktop-amd64 should be filtered out, got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "ubuntu-server") {
		t.Errorf("expected product name in header, got:\n%s", hook.last)
	}
}

func TestDispatchBuildsStatusReleaseProductCaseInsensitive(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "builds status Noble Ubuntu-Server", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "ubuntu-server-amd64") {
		t.Errorf("product filter should be case-insensitive, got:\n%s", hook.last)
	}
	if strings.Contains(hook.last, "ubuntu-desktop-amd64") {
		t.Errorf("ubuntu-desktop-amd64 should be filtered out, got:\n%s", hook.last)
	}
}

func TestDispatchBuildsStatusReleaseProductUnknown(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "builds status noble nonexistent-product", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "No artefacts found") {
		t.Errorf("expected 'No artefacts found' message, got: %s", hook.last)
	}
	if !strings.Contains(hook.last, "nonexistent-product") {
		t.Errorf("error message should mention the product, got: %s", hook.last)
	}
}

// --- builds (no args or unknown sub-command) ---

func TestDispatchBuildsNoArgs(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "builds", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "Usage") {
		t.Errorf("expected usage message, got: %s", hook.last)
	}
}

func TestDispatchBuildsUnknownSubcommand(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "builds noble", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "Usage") {
		t.Errorf("expected usage message for unknown sub-command, got: %s", hook.last)
	}
}

// --- unknown / empty ---

func TestDispatchUnknown(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "banana", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "didn't quite get") {
		t.Errorf("expected 'didn't quite get' response, got: %s", hook.last)
	}
	if !strings.Contains(hook.last, "banana") {
		t.Errorf("response should echo the unknown command, got: %s", hook.last)
	}
}

func TestDispatchEmpty(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "   ", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hook.last != "" {
		t.Errorf("empty message should not produce output, got: %s", hook.last)
	}
}

func TestDispatchBuildsStatusReleaseSortedByProduct(t *testing.T) {
	artefacts := []domain.Artefact{
		{ID: 1, Name: "ubuntu-server-amd64", OS: "ubuntu-server", Release: "noble", Version: today},
		{ID: 2, Name: "ubuntu-desktop-amd64", OS: "ubuntu", Release: "noble", Version: today},
	}
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "builds status noble", artefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ubuntuPos := strings.Index(hook.last, "| ubuntu-desktop-amd64 | ubuntu |")
	ubuntuServerPos := strings.Index(hook.last, "| ubuntu-server-amd64 | ubuntu-server |")
	if ubuntuPos == -1 || ubuntuServerPos == -1 {
		t.Fatalf("expected both artefact rows in output, got:\n%s", hook.last)
	}
	if ubuntuPos > ubuntuServerPos {
		t.Errorf("ubuntu (OS=%q) should appear before ubuntu-server (OS=%q) when sorted by product; got:\n%s",
			"ubuntu", "ubuntu-server", hook.last)
	}
}

func TestImageAgeViaDispatch(t *testing.T) {
	cases := []struct {
		version string
		wantErr bool
	}{
		{"20240101", false},
		{"20240101.1", false},
		{"20240101.12", false},
		{"invalid", true},
		{"", true},
	}
	for _, tc := range cases {
		got := domain.ImageAge(tc.version)
		if tc.wantErr && got != "unknown" {
			t.Errorf("domain.ImageAge(%q) = %q, want %q", tc.version, got, "unknown")
		}
		if !tc.wantErr && got == "unknown" {
			t.Errorf("domain.ImageAge(%q) returned %q unexpectedly", tc.version, got)
		}
	}
}

// --- progress bar ---

func TestBuildsStatusProgressBar(t *testing.T) {
	artefacts := []domain.Artefact{
		{ID: 1, Release: "noble", Version: today},
		{ID: 2, Release: "noble", Version: today},
		{ID: 3, Release: "noble", Version: today},
		{ID: 4, Release: "noble", Version: today},
		{ID: 5, Release: "noble", Version: today},
	}
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "builds status", artefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantBar := strings.Repeat("🟩", 10)
	if !strings.Contains(hook.last, wantBar) {
		t.Errorf("100%% bar should be 10 green squares, got:\n%s", hook.last)
	}
	if strings.Contains(hook.last, "🟥") {
		t.Errorf("100%% bar should have no red squares, got:\n%s", hook.last)
	}
}

func TestBuildsStatusProgressBarZero(t *testing.T) {
	artefacts := []domain.Artefact{
		{ID: 1, Release: "noble", Version: yesterday},
		{ID: 2, Release: "noble", Version: yesterday},
	}
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "builds status", artefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantBar := strings.Repeat("🟥", 10)
	if !strings.Contains(hook.last, wantBar) {
		t.Errorf("0%% bar should be 10 red squares, got:\n%s", hook.last)
	}
	if strings.Contains(hook.last, "🟩") {
		t.Errorf("0%% bar should have no green squares, got:\n%s", hook.last)
	}
}

// --- tests status (summary) ---

func TestDispatchTestsStatusEmptySnapshot(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "tests status", nil, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "No snapshot") {
		t.Errorf("expected 'No snapshot' message, got: %s", hook.last)
	}
}

func TestDispatchTestsStatus(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "tests status", testArtefactsWithBuilds, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "plucky") {
		t.Errorf("tests status missing 'plucky', got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "noble") {
		t.Errorf("tests status missing 'noble', got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "🟩") || !strings.Contains(hook.last, "🟥") {
		t.Errorf("tests status missing progress bar squares, got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "Passed") || !strings.Contains(hook.last, "Total") {
		t.Errorf("tests status missing Passed/Total columns, got:\n%s", hook.last)
	}
}

func TestDispatchTestsStatusCaseInsensitive(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "Tests Status", testArtefactsWithBuilds, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "plucky") {
		t.Errorf("tests status case-insensitive failed, got:\n%s", hook.last)
	}
}

func TestDispatchTestsStatusNoBuildsInSnapshot(t *testing.T) {
	// Artefacts with no Builds field (e.g. snapshot not yet enriched).
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "tests status", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "No test executions found") {
		t.Errorf("expected 'No test executions found' message, got: %s", hook.last)
	}
}

// --- tests status <release> (detail) ---

func TestDispatchTestsStatusRelease(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "tests status plucky", testArtefactsWithBuilds, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "plucky-desktop-amd64.iso") {
		t.Errorf("tests status plucky missing desktop artefact, got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "plucky-server-amd64.iso") {
		t.Errorf("tests status plucky missing server artefact, got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "❌") {
		t.Errorf("tests status plucky missing ❌ for failed Jenkins, got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "✅") {
		t.Errorf("tests status plucky missing ✅ for passed Jenkins, got:\n%s", hook.last)
	}
	// Artefacts with no displayable executions must be omitted.
	if strings.Contains(hook.last, "plucky-desktop-arm64.iso") {
		t.Errorf("plucky-desktop-arm64 has no displayable tests and should be omitted, got:\n%s", hook.last)
	}
	if strings.Contains(hook.last, "plucky-minimal-amd64.iso") {
		t.Errorf("plucky-minimal has no displayable tests and should be omitted, got:\n%s", hook.last)
	}
	if strings.Contains(hook.last, "noble") {
		t.Errorf("tests status plucky should not contain noble artefacts, got:\n%s", hook.last)
	}
}

func TestDispatchTestsStatusReleaseUnknown(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "tests status nonexistent", testArtefactsWithBuilds, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "No artefacts found") {
		t.Errorf("expected 'No artefacts found' message, got: %s", hook.last)
	}
}

func TestDispatchTestsStatusReleaseNoTests(t *testing.T) {
	// Release where all artefacts have only Image build + Manual Testing IN_PROGRESS.
	artefacts := []domain.Artefact{
		{ID: 1002, Name: "plucky-desktop-arm64.iso", OS: "ubuntu", Release: "plucky", Version: today,
			Builds: testArtefactsWithBuilds[1].Builds},
		{ID: 1004, Name: "plucky-minimal-amd64.iso", OS: "ubuntu-minimal", Release: "plucky", Version: today,
			Builds: testArtefactsWithBuilds[3].Builds},
	}
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "tests status plucky", artefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "No test executions found") {
		t.Errorf("expected 'No test executions found' message, got: %s", hook.last)
	}
}

// --- tests status <release> <product> (product filter) ---

func TestDispatchTestsStatusReleaseProduct(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "tests status plucky ubuntu-server", testArtefactsWithBuilds, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "plucky-server-amd64.iso") {
		t.Errorf("expected server artefact in output, got:\n%s", hook.last)
	}
	if strings.Contains(hook.last, "plucky-desktop-amd64.iso") {
		t.Errorf("desktop artefact should be filtered out, got:\n%s", hook.last)
	}
}

func TestDispatchTestsStatusReleaseProductUnknown(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "tests status plucky nonexistent-product", testArtefactsWithBuilds, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "No artefacts found") {
		t.Errorf("expected 'No artefacts found' message, got: %s", hook.last)
	}
	if !strings.Contains(hook.last, "nonexistent-product") {
		t.Errorf("error message should mention the product, got: %s", hook.last)
	}
}

// --- tests (no args or unknown sub-command) ---

func TestDispatchTestsNoArgs(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "tests", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "Usage") {
		t.Errorf("expected usage message, got: %s", hook.last)
	}
}

func TestDispatchTestsUnknownSubcommand(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "tests noble", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "Usage") {
		t.Errorf("expected usage message for unknown sub-command, got: %s", hook.last)
	}
}

// --- ci_link hyperlink in tests status detail ---

func TestDispatchTestsStatusReleaseCILink(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "tests status plucky ubuntu", testArtefactsWithBuilds, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The FAILED Jenkins execution for 1001 has a ci_link; status cell must be a hyperlink.
	if !strings.Contains(hook.last, "](https://platform-qa-jenkins") {
		t.Errorf("expected Markdown CI link in FAILED status cell, got:\n%s", hook.last)
	}
}

// --- keyword filtering (ported from mattermost/poller_test.go) ---

func TestDispatchKeywordRequired(t *testing.T) {
	hook := &captureNotifier{}
	// With keyword set, a bare "help" (no keyword prefix) must be ignored.
	if err := Dispatch(context.Background(), "test", "help", testArtefacts, nil, nil, nil, hook, "@watchtower", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hook.last != "" {
		t.Errorf("message without keyword should be ignored, got: %s", hook.last)
	}
}

func TestDispatchKeywordStripped(t *testing.T) {
	hook := &captureNotifier{}
	// "@watchtower help" must route to the help handler.
	if err := Dispatch(context.Background(), "test", "@watchtower help", testArtefacts, nil, nil, nil, hook, "@watchtower", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "builds status") {
		t.Errorf("keyword-prefixed help should produce help output, got: %s", hook.last)
	}
}

func TestDispatchKeywordCaseInsensitive(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "@Watchtower builds status", testArtefacts, nil, nil, nil, hook, "@watchtower", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "noble") {
		t.Errorf("keyword match should be case-insensitive, got: %s", hook.last)
	}
}

func TestDispatchKeywordBareShowsGreeting(t *testing.T) {
	hook := &captureNotifier{}
	// Just the keyword alone (no command) should show the friendly greeting.
	if err := Dispatch(context.Background(), "test", "@watchtower", testArtefacts, nil, nil, nil, hook, "@watchtower", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "Watchtower") {
		t.Errorf("bare keyword should show greeting, got: %s", hook.last)
	}
	if !strings.Contains(hook.last, "help") {
		t.Errorf("greeting should mention 'help' command, got: %s", hook.last)
	}
}

func TestDispatchNoKeyword(t *testing.T) {
	hook := &captureNotifier{}
	// Empty keyword → every message is routed without filtering.
	if err := Dispatch(context.Background(), "test", "help", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "builds status") {
		t.Errorf("empty keyword: help should still produce output, got: %s", hook.last)
	}
}

func TestDispatchKeywordWithBuildsStatus(t *testing.T) {
	hook := &captureNotifier{}
	artefacts := []domain.Artefact{
		{ID: 1, Release: "noble", Version: time.Now().UTC().Format("20060102")},
	}
	if err := Dispatch(context.Background(), "test", "@watchtower builds status", artefacts, nil, nil, nil, hook, "@watchtower", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "noble") {
		t.Errorf("expected builds status output, got: %s", hook.last)
	}
}

// --- investigate ---

// mockLogFetcher returns a fixed log string for any URL.
type mockLogFetcher struct {
	content string
	err     error
}

func (m *mockLogFetcher) Fetch(_ context.Context, _ string) (string, error) {
	return m.content, m.err
}

// mockLLMClient returns a fixed JSON response for any prompt.
type mockLLMClient struct {
	response string
	err      error
}

func (m *mockLLMClient) Complete(_ context.Context, _, _ string) (string, error) {
	return m.response, m.err
}

// mockLaunchpadSource is a simple test double for ports.LaunchpadSource.
type mockLaunchpadSource struct {
	url string
	err error
}

func (m *mockLaunchpadSource) FetchBuildLogURL(_ context.Context, _ string) (string, error) {
	return m.url, m.err
}

// mockFuncLogFetcher calls a provided function for each Fetch call (useful for
// testing multi-call sequences such as cd-build-log then librarian log).
type mockFuncLogFetcher struct {
	fn func(ctx context.Context, url string) (string, error)
}

func (m *mockFuncLogFetcher) Fetch(ctx context.Context, url string) (string, error) {
	return m.fn(ctx, url)
}

func TestDispatchInvestigateUsage(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "investigate", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "Usage") {
		t.Errorf("expected usage message for bare investigate, got: %s", hook.last)
	}
	if !strings.Contains(hook.last, "artefact-id") {
		t.Errorf("expected 'artefact-id' hint in usage, got: %s", hook.last)
	}
}

func TestDispatchInvestigateNonNumericID(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "investigate notanumber", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "Invalid artefact ID") {
		t.Errorf("expected invalid ID message, got: %s", hook.last)
	}
}

func TestDispatchInvestigateUnknownID(t *testing.T) {
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "investigate 9999", testArtefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "not found") {
		t.Errorf("expected 'not found' message for unknown ID, got: %s", hook.last)
	}
}

func TestDispatchInvestigateNoLLM(t *testing.T) {
	// LLM/logFetcher are nil — should return graceful error message.
	artefacts := []domain.Artefact{
		{ID: 42, Name: "noble-desktop-amd64.iso", OS: "ubuntu", Release: "noble", Version: yesterday,
			ImageURL: "https://cdimage.ubuntu.com/ubuntu/noble/daily-live/20260101/noble-desktop-amd64.iso"},
	}
	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "investigate 42", artefacts, nil, nil, nil, hook, "", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "OPENROUTER_API_KEY") {
		t.Errorf("expected LLM-not-configured message, got: %s", hook.last)
	}
}

func TestDispatchInvestigateNoImageURL(t *testing.T) {
	// Artefact exists but has no ImageURL — cannot derive log URL.
	artefacts := []domain.Artefact{
		{ID: 7, Name: "noble-server-amd64.iso", OS: "ubuntu-server", Release: "noble", Version: yesterday},
	}
	hook := &captureNotifier{}
	lf := &mockLogFetcher{content: "some log"}
	llm := &mockLLMClient{response: `{"category":"unknown","hypothesis":"x","log_excerpts":[],"next_action":"y"}`}
	if err := Dispatch(context.Background(), "test", "investigate 7", artefacts, nil, nil, nil, hook, "", nil, lf, llm, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "No log URL") {
		t.Errorf("expected 'No log URL' message, got: %s", hook.last)
	}
}

func TestDispatchInvestigateSuccess(t *testing.T) {
	artefacts := []domain.Artefact{
		{ID: 42, Name: "noble-desktop-amd64.iso", OS: "ubuntu", Release: "noble", Version: yesterday,
			ImageURL: "https://cdimage.ubuntu.com/ubuntu/noble/daily-live/20260101/noble-desktop-amd64.iso"},
	}
	lf := &mockLogFetcher{content: "line1\nline2\nE: Failed to fetch http://example.com 404\n"}
	llmResp := `{"category":"dependency","hypothesis":"apt mirror returned 404","log_excerpts":["E: Failed to fetch http://example.com 404"],"next_action":"Retry the build"}`
	llm := &mockLLMClient{response: llmResp}

	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "investigate 42", artefacts, nil, nil, nil, hook, "", nil, lf, llm, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// First Send call is the "fetching…" progress message; second is the report.
	// The last message should be the formatted investigation.
	if !strings.Contains(hook.last, "noble-desktop-amd64.iso") {
		t.Errorf("expected artefact name in investigation output, got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "dependency") {
		t.Errorf("expected category 'dependency' in output, got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "apt mirror returned 404") {
		t.Errorf("expected hypothesis in output, got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "Retry the build") {
		t.Errorf("expected next_action in output, got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "Log source:") {
		t.Errorf("expected 'Log source:' in output, got:\n%s", hook.last)
	}
}

func TestDispatchInvestigateWithLaunchpad(t *testing.T) {
	// cd-build-log contains a Launchpad build URL; Launchpad returns a librarian URL;
	// librarian log is fetched and analysed.
	artefacts := []domain.Artefact{
		{ID: 42, Name: "noble-desktop-amd64.iso", OS: "ubuntu", Release: "noble", Version: yesterday,
			ImageURL: "https://cdimage.ubuntu.com/ubuntu/noble/daily-live/20260101/noble-desktop-amd64.iso"},
	}

	cdLog := "ubuntu-amd64: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/noble/ubuntu/+build/12345\n"
	libLog := "detailed build log line 1\ndetailed build log line 2\n"

	// mockLogFetcher returns cdLog for first call, libLog for second.
	callCount := 0
	lf := &mockFuncLogFetcher{fn: func(_ context.Context, url string) (string, error) {
		callCount++
		if callCount == 1 {
			return cdLog, nil
		}
		return libLog, nil
	}}
	lp := &mockLaunchpadSource{url: "https://launchpadlibrarian.net/123/buildlog.txt.gz"}
	llmResp := `{"category":"infra","hypothesis":"chroot problem","log_excerpts":["Error: Instance is not running"],"next_action":"Retry"}`
	llm := &mockLLMClient{response: llmResp}

	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "investigate 42", artefacts, nil, nil, nil, hook, "", nil, lf, llm, lp, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "Launchpad librarian") {
		t.Errorf("expected 'Launchpad librarian' source in output, got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "chroot problem") {
		t.Errorf("expected hypothesis in output, got:\n%s", hook.last)
	}
}

func TestDispatchInvestigateLaunchpadFallback(t *testing.T) {
	// Launchpad API returns no log — should fall back to cd-build-log content.
	artefacts := []domain.Artefact{
		{ID: 42, Name: "noble-desktop-amd64.iso", OS: "ubuntu", Release: "noble", Version: yesterday,
			ImageURL: "https://cdimage.ubuntu.com/ubuntu/noble/daily-live/20260101/noble-desktop-amd64.iso"},
	}
	cdLog := "ubuntu-amd64: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/noble/ubuntu/+build/12345\n"
	lf := &mockLogFetcher{content: cdLog}
	lp := &mockLaunchpadSource{url: ""} // empty = no log yet
	llmResp := `{"category":"unknown","hypothesis":"no detail","log_excerpts":[],"next_action":"check"}`
	llm := &mockLLMClient{response: llmResp}

	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "investigate 42", artefacts, nil, nil, nil, hook, "", nil, lf, llm, lp, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "cd-build-log") {
		t.Errorf("expected fallback to 'cd-build-log' in source, got:\n%s", hook.last)
	}
}

func TestDispatchInvestigateLLMError(t *testing.T) {
	artefacts := []domain.Artefact{
		{ID: 42, Name: "noble-desktop-amd64.iso", OS: "ubuntu", Release: "noble", Version: yesterday,
			ImageURL: "https://cdimage.ubuntu.com/ubuntu/noble/daily-live/20260101/noble-desktop-amd64.iso"},
	}
	lf := &mockLogFetcher{content: "some log content"}
	llm := &mockLLMClient{err: fmt.Errorf("LLM unavailable")}

	hook := &captureNotifier{}
	if err := Dispatch(context.Background(), "test", "investigate 42", artefacts, nil, nil, nil, hook, "", nil, lf, llm, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "Investigation failed") {
		t.Errorf("expected 'Investigation failed' in output, got:\n%s", hook.last)
	}
}

// --- matchLaunchpadURL ---

func TestMatchLaunchpadURL_ExactSimpleArch(t *testing.T) {
	lpURLs := map[string]string{
		"amd64": "https://launchpad.net/+build/1",
		"arm64": "https://launchpad.net/+build/2",
	}
	got := matchLaunchpadURL(lpURLs, "amd64")
	if got != "https://launchpad.net/+build/1" {
		t.Errorf("matchLaunchpadURL(amd64) = %q, want build/1", got)
	}
}

func TestMatchLaunchpadURL_SubstringVariantArch(t *testing.T) {
	// Variant build: label is "desktop-preinstalled-arm64-raspi", arch is "arm64+raspi"
	lpURLs := map[string]string{
		"desktop-preinstalled-arm64-raspi": "https://launchpad.net/+build/99",
	}
	got := matchLaunchpadURL(lpURLs, "arm64+raspi")
	if got != "https://launchpad.net/+build/99" {
		t.Errorf("matchLaunchpadURL(arm64+raspi) = %q, want build/99", got)
	}
}

func TestMatchLaunchpadURL_NormalisesPlus(t *testing.T) {
	lpURLs := map[string]string{
		"amd64+tegra": "https://launchpad.net/+build/50",
	}
	got := matchLaunchpadURL(lpURLs, "amd64+tegra")
	if got != "https://launchpad.net/+build/50" {
		t.Errorf("matchLaunchpadURL(amd64+tegra) = %q, want build/50", got)
	}
}

func TestMatchLaunchpadURL_NoMatch(t *testing.T) {
	lpURLs := map[string]string{
		"amd64": "https://launchpad.net/+build/1",
	}
	got := matchLaunchpadURL(lpURLs, "riscv64")
	if got != "" {
		t.Errorf("matchLaunchpadURL(riscv64) = %q, want empty", got)
	}
}

func TestMatchLaunchpadURL_EmptyMap(t *testing.T) {
	got := matchLaunchpadURL(map[string]string{}, "amd64")
	if got != "" {
		t.Errorf("matchLaunchpadURL on empty map = %q, want empty", got)
	}
}
