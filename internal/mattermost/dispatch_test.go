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

// --- help ---

func TestDispatchHelp(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("help", testArtefacts, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"builds status", "builds status <release>", "help"} {
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
	// Both releases must appear.
	if !strings.Contains(hook.last, "noble") {
		t.Errorf("builds status missing 'noble', got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "plucky") {
		t.Errorf("builds status missing 'plucky', got:\n%s", hook.last)
	}
	// Must contain progress bar squares.
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
	// Must show build status indicators.
	if !strings.Contains(hook.last, "✅") {
		t.Errorf("builds status noble missing ✅, got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "❌") {
		t.Errorf("builds status noble missing ❌, got:\n%s", hook.last)
	}
	// plucky artefact must NOT appear.
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
		// old version + imageURL → should produce a hyperlink
		{ID: 1, Name: "ubuntu-server-amd64", OS: "ubuntu-server", Release: "noble", Version: "20200101", ImageURL: imageURL},
		// old version + no imageURL → plain fallback
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
	// The artefact without an imageURL must still show the plain fallback
	if !strings.Contains(hook.last, "❌ not built") {
		t.Errorf("expected plain '❌ not built' fallback for artefact without imageURL, got:\n%s", hook.last)
	}
	// The server row (with imageURL) must use the hyperlink, not the plain text
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
	// Only the ubuntu-server artefact should appear.
	if !strings.Contains(hook.last, "ubuntu-server-amd64") {
		t.Errorf("expected ubuntu-server-amd64 in output, got:\n%s", hook.last)
	}
	// ubuntu-desktop (OS=ubuntu) must NOT appear.
	if strings.Contains(hook.last, "ubuntu-desktop-amd64") {
		t.Errorf("ubuntu-desktop-amd64 should be filtered out, got:\n%s", hook.last)
	}
	// Header should mention the product.
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
	// Empty message — nothing should be sent.
	if hook.last != "" {
		t.Errorf("empty message should not produce output, got: %s", hook.last)
	}
}

func TestDispatchBuildsStatusReleaseSortedByProduct(t *testing.T) {
	// Artefacts are intentionally ordered with ubuntu-server before ubuntu
	// to verify that the output is sorted by product (OS) regardless.
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
		wantErr bool // "unknown" expected
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
	// 5 artefacts: 5 built today → 100% → 10 green squares, 0 red.
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
	// 2 artefacts: none built today → 0% → 0 green, 10 red.
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
