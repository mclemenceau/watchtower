package mattermost

import (
	"strings"
	"testing"
	"time"

	"github.com/mclemenceau/argus/internal/buildapi"
)

// captureHook records the last message sent via Send.
type captureHook struct {
	last string
	err  error
}

func (c *captureHook) Send(text string) error {
	c.last = text
	return c.err
}

var (
	today     = time.Now().UTC().Format("20060102")
	yesterday = time.Now().UTC().AddDate(0, 0, -1).Format("20060102")
)

var testArtefacts = []buildapi.Artefact{
	// noble: 1 built today, 1 not built (yesterday)
	{ID: 1, Name: "ubuntu-desktop-amd64", OS: "ubuntu", Release: "noble", Version: today},
	{ID: 2, Name: "ubuntu-server-amd64", OS: "ubuntu-server", Release: "noble", Version: yesterday},
	// plucky: 1 built today
	{ID: 3, Name: "ubuntu-desktop-amd64", OS: "ubuntu", Release: "plucky", Version: today},
}

// testArtefactsWithBuilds has artefacts whose Builds field is populated with
// realistic test execution data (no live API calls needed).
var testArtefactsWithBuilds = func() []buildapi.Artefact {
	env := func(name, arch string) buildapi.Environment {
		return buildapi.Environment{Name: name, Architecture: arch}
	}
	return []buildapi.Artefact{
		// 1001 — plucky desktop amd64: Jenkins FAILED (displayable)
		{
			ID: 1001, Name: "plucky-desktop-amd64.iso", OS: "ubuntu", Release: "plucky", Version: today,
			Builds: []buildapi.ArtefactBuild{{
				ID: 2001, Architecture: "amd64",
				TestExecutions: []buildapi.TestExecution{
					{ID: 3001, TestPlan: "Image build", Status: "PASSED", Environment: env("cdimage.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T06:00:00"},
					{ID: 3002, TestPlan: "Jenkins image validation", Status: "FAILED", CILink: "https://platform-qa-jenkins.ps5.ubuntu.com/job/ubuntu-plucky-desktop-amd64-iso-static-validation/1/", Environment: env("platform-qa-jenkins.ps5.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T07:00:00"},
					{ID: 3003, TestPlan: "Manual Testing", Status: "IN_PROGRESS", Environment: env("user manual tests", "amd64"), CreatedAt: "2026-06-26T06:01:00"},
				},
			}},
		},
		// 1002 — plucky desktop arm64: no displayable executions
		{
			ID: 1002, Name: "plucky-desktop-arm64.iso", OS: "ubuntu", Release: "plucky", Version: today,
			Builds: []buildapi.ArtefactBuild{{
				ID: 2002, Architecture: "arm64",
				TestExecutions: []buildapi.TestExecution{
					{ID: 3004, TestPlan: "Image build", Status: "PASSED", Environment: env("cdimage.ubuntu.com", "arm64"), CreatedAt: "2026-06-26T06:00:00"},
					{ID: 3005, TestPlan: "Manual Testing", Status: "IN_PROGRESS", Environment: env("user manual tests", "arm64"), CreatedAt: "2026-06-26T06:01:00"},
				},
			}},
		},
		// 1003 — plucky server amd64: Jenkins PASSED (displayable)
		{
			ID: 1003, Name: "plucky-server-amd64.iso", OS: "ubuntu-server", Release: "plucky", Version: today,
			Builds: []buildapi.ArtefactBuild{{
				ID: 2003, Architecture: "amd64",
				TestExecutions: []buildapi.TestExecution{
					{ID: 3006, TestPlan: "Image build", Status: "PASSED", Environment: env("cdimage.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T06:00:00"},
					{ID: 3007, TestPlan: "Jenkins image validation", Status: "PASSED", CILink: "https://platform-qa-jenkins.ps5.ubuntu.com/job/ubuntu-plucky-server-amd64-iso-static-validation/1/", Environment: env("platform-qa-jenkins.ps5.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T07:00:00"},
					{ID: 3008, TestPlan: "Manual Testing", Status: "IN_PROGRESS", Environment: env("user manual tests", "amd64"), CreatedAt: "2026-06-26T06:01:00"},
				},
			}},
		},
		// 1004 — plucky minimal: no displayable executions
		{
			ID: 1004, Name: "plucky-minimal-amd64.iso", OS: "ubuntu-minimal", Release: "plucky", Version: yesterday,
			Builds: []buildapi.ArtefactBuild{{
				ID: 2004, Architecture: "amd64",
				TestExecutions: []buildapi.TestExecution{
					{ID: 3009, TestPlan: "Image build", Status: "PASSED", Environment: env("cdimage.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T06:00:00"},
					{ID: 3010, TestPlan: "Manual Testing", Status: "IN_PROGRESS", Environment: env("user manual tests", "amd64"), CreatedAt: "2026-06-26T06:01:00"},
				},
			}},
		},
		// 1005 — noble desktop amd64: Jenkins PASSED + Manual Testing PASSED (both displayable)
		{
			ID: 1005, Name: "noble-desktop-amd64.iso", OS: "ubuntu", Release: "noble", Version: yesterday,
			Builds: []buildapi.ArtefactBuild{{
				ID: 2005, Architecture: "amd64",
				TestExecutions: []buildapi.TestExecution{
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
	hook := &captureHook{}
	if err := Dispatch("help", testArtefacts, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"builds status", "builds status <release>", "tests status", "help"} {
		if !strings.Contains(hook.last, want) {
			t.Errorf("help output missing %q, got:\n%s", want, hook.last)
		}
	}
}

func TestDispatchHelpCaseInsensitive(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("HELP", testArtefacts, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "builds status") {
		t.Errorf("help output missing 'builds status' command")
	}
}

// --- builds status (summary) ---

func TestDispatchBuildsStatus(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("builds status", testArtefacts, "", hook, ""); err != nil {
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

func TestDispatchBuildsStatusCaseInsensitive(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("Builds Status", testArtefacts, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "noble") {
		t.Errorf("builds status case-insensitive failed, got:\n%s", hook.last)
	}
}

func TestDispatchBuildsStatusEmptySnapshot(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("builds status", nil, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "No snapshot") {
		t.Errorf("expected 'No snapshot' message, got: %s", hook.last)
	}
}

// --- builds status <release> (detail) ---

func TestDispatchBuildsStatusRelease(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("builds status noble", testArtefacts, "", hook, ""); err != nil {
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
	hook := &captureHook{}
	if err := Dispatch("builds status Noble", testArtefacts, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "ubuntu-desktop-amd64") {
		t.Errorf("builds status Noble should return noble artefacts, got:\n%s", hook.last)
	}
}

func TestDispatchBuildsStatusReleaseUnknown(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("builds status nonexistent", testArtefacts, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "No artefacts found") {
		t.Errorf("expected 'No artefacts found' message, got: %s", hook.last)
	}
}

// --- builds status: log hyperlink for unbuilt artefact ---

func TestDispatchBuildsStatusReleaseLogLink(t *testing.T) {
	imageURL := "https://cdimage.ubuntu.com/ubuntu-server/noble/daily-live/20200101/noble-live-server-amd64.iso"
	logURL := "https://ubuntu-archive-team.ubuntu.com/cd-build-logs/ubuntu-server/noble/daily-live-20200101.log"
	artefacts := []buildapi.Artefact{
		{ID: 1, Name: "ubuntu-server-amd64", OS: "ubuntu-server", Release: "noble", Version: "20200101", ImageURL: imageURL},
		{ID: 2, Name: "ubuntu-desktop-amd64", OS: "ubuntu", Release: "noble", Version: "20200101"},
	}
	hook := &captureHook{}
	if err := Dispatch("builds status noble", artefacts, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantLink := "[not built](" + logURL + ")"
	if !strings.Contains(hook.last, wantLink) {
		t.Errorf("expected Markdown log hyperlink %q in output, got:\n%s", wantLink, hook.last)
	}
	if !strings.Contains(hook.last, "❌ not built") {
		t.Errorf("expected plain '❌ not built' fallback for artefact without imageURL, got:\n%s", hook.last)
	}
	serverRow := "| ubuntu-server-amd64 | ubuntu-server |"
	for _, line := range strings.Split(hook.last, "\n") {
		if strings.Contains(line, serverRow) && strings.Contains(line, "❌ not built") && !strings.Contains(line, "[not built]") {
			t.Errorf("artefact with imageURL should use hyperlink, not plain '❌ not built'; got line:\n%s", line)
		}
	}
}

// --- builds status <release> <product> (product filter) ---

func TestDispatchBuildsStatusReleaseProduct(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("builds status noble ubuntu-server", testArtefacts, "", hook, ""); err != nil {
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
	hook := &captureHook{}
	if err := Dispatch("builds status Noble Ubuntu-Server", testArtefacts, "", hook, ""); err != nil {
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
	hook := &captureHook{}
	if err := Dispatch("builds status noble nonexistent-product", testArtefacts, "", hook, ""); err != nil {
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
	hook := &captureHook{}
	if err := Dispatch("builds", testArtefacts, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "Usage") {
		t.Errorf("expected usage message, got: %s", hook.last)
	}
}

func TestDispatchBuildsUnknownSubcommand(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("builds noble", testArtefacts, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "Usage") {
		t.Errorf("expected usage message for unknown sub-command, got: %s", hook.last)
	}
}

// --- unknown / empty ---

func TestDispatchUnknown(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("banana", testArtefacts, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "didn't understand") {
		t.Errorf("expected 'didn't understand' response, got: %s", hook.last)
	}
	if !strings.Contains(hook.last, "banana") {
		t.Errorf("response should echo the unknown command, got: %s", hook.last)
	}
}

func TestDispatchEmpty(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("   ", testArtefacts, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hook.last != "" {
		t.Errorf("empty message should not produce output, got: %s", hook.last)
	}
}

func TestDispatchBuildsStatusReleaseSortedByProduct(t *testing.T) {
	artefacts := []buildapi.Artefact{
		{ID: 1, Name: "ubuntu-server-amd64", OS: "ubuntu-server", Release: "noble", Version: today},
		{ID: 2, Name: "ubuntu-desktop-amd64", OS: "ubuntu", Release: "noble", Version: today},
	}
	hook := &captureHook{}
	if err := Dispatch("builds status noble", artefacts, "", hook, ""); err != nil {
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

func TestImageAge(t *testing.T) {
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
		got := buildapi.ImageAge(tc.version)
		if tc.wantErr && got != "unknown" {
			t.Errorf("buildapi.ImageAge(%q) = %q, want %q", tc.version, got, "unknown")
		}
		if !tc.wantErr && got == "unknown" {
			t.Errorf("buildapi.ImageAge(%q) returned %q unexpectedly", tc.version, got)
		}
	}
}

// --- progress bar ---

func TestBuildsStatusProgressBar(t *testing.T) {
	artefacts := []buildapi.Artefact{
		{ID: 1, Release: "noble", Version: today},
		{ID: 2, Release: "noble", Version: today},
		{ID: 3, Release: "noble", Version: today},
		{ID: 4, Release: "noble", Version: today},
		{ID: 5, Release: "noble", Version: today},
	}
	hook := &captureHook{}
	if err := Dispatch("builds status", artefacts, "", hook, ""); err != nil {
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
	artefacts := []buildapi.Artefact{
		{ID: 1, Release: "noble", Version: yesterday},
		{ID: 2, Release: "noble", Version: yesterday},
	}
	hook := &captureHook{}
	if err := Dispatch("builds status", artefacts, "", hook, ""); err != nil {
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
	hook := &captureHook{}
	if err := Dispatch("tests status", nil, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "No snapshot") {
		t.Errorf("expected 'No snapshot' message, got: %s", hook.last)
	}
}

func TestDispatchTestsStatus(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("tests status", testArtefactsWithBuilds, "", hook, ""); err != nil {
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
	hook := &captureHook{}
	if err := Dispatch("Tests Status", testArtefactsWithBuilds, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "plucky") {
		t.Errorf("tests status case-insensitive failed, got:\n%s", hook.last)
	}
}

func TestDispatchTestsStatusNoBuildsInSnapshot(t *testing.T) {
	// Artefacts with no Builds field (e.g. snapshot not yet enriched).
	hook := &captureHook{}
	if err := Dispatch("tests status", testArtefacts, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "No test executions found") {
		t.Errorf("expected 'No test executions found' message, got: %s", hook.last)
	}
}

// --- tests status <release> (detail) ---

func TestDispatchTestsStatusRelease(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("tests status plucky", testArtefactsWithBuilds, "", hook, ""); err != nil {
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
	hook := &captureHook{}
	if err := Dispatch("tests status nonexistent", testArtefactsWithBuilds, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "No artefacts found") {
		t.Errorf("expected 'No artefacts found' message, got: %s", hook.last)
	}
}

func TestDispatchTestsStatusReleaseNoTests(t *testing.T) {
	// Release where all artefacts have only Image build + Manual Testing IN_PROGRESS.
	artefacts := []buildapi.Artefact{
		{ID: 1002, Name: "plucky-desktop-arm64.iso", OS: "ubuntu", Release: "plucky", Version: today,
			Builds: testArtefactsWithBuilds[1].Builds},
		{ID: 1004, Name: "plucky-minimal-amd64.iso", OS: "ubuntu-minimal", Release: "plucky", Version: today,
			Builds: testArtefactsWithBuilds[3].Builds},
	}
	hook := &captureHook{}
	if err := Dispatch("tests status plucky", artefacts, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "No test executions found") {
		t.Errorf("expected 'No test executions found' message, got: %s", hook.last)
	}
}

// --- tests status <release> <product> (product filter) ---

func TestDispatchTestsStatusReleaseProduct(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("tests status plucky ubuntu-server", testArtefactsWithBuilds, "", hook, ""); err != nil {
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
	hook := &captureHook{}
	if err := Dispatch("tests status plucky nonexistent-product", testArtefactsWithBuilds, "", hook, ""); err != nil {
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
	hook := &captureHook{}
	if err := Dispatch("tests", testArtefacts, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "Usage") {
		t.Errorf("expected usage message, got: %s", hook.last)
	}
}

func TestDispatchTestsUnknownSubcommand(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("tests noble", testArtefacts, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "Usage") {
		t.Errorf("expected usage message for unknown sub-command, got: %s", hook.last)
	}
}

// --- ci_link hyperlink in tests status detail ---

func TestDispatchTestsStatusReleaseCILink(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("tests status plucky ubuntu", testArtefactsWithBuilds, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The FAILED Jenkins execution for 1001 has a ci_link; status cell must be a hyperlink.
	if !strings.Contains(hook.last, "](https://platform-qa-jenkins") {
		t.Errorf("expected Markdown CI link in FAILED status cell, got:\n%s", hook.last)
	}
}
