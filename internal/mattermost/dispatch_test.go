package mattermost

import (
	"strings"
	"testing"

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

var testArtefacts = []buildapi.Artefact{
	{ID: 1, Name: "ubuntu-desktop-amd64", OS: "ubuntu", Release: "noble", Version: "20260601", Status: "APPROVED"},
	{ID: 2, Name: "ubuntu-server-amd64", OS: "ubuntu-server", Release: "noble", Version: "20260601", Status: "MARKED_AS_FAILED"},
	{ID: 3, Name: "ubuntu-desktop-amd64", OS: "ubuntu", Release: "plucky", Version: "20260610", Status: "UNDECIDED"},
}

func TestDispatchHelp(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("help", testArtefacts, "", hook); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "status") {
		t.Errorf("help output missing 'status' command, got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "builds") {
		t.Errorf("help output missing 'builds' command, got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "releases") {
		t.Errorf("help output missing 'releases' command, got:\n%s", hook.last)
	}
}

func TestDispatchHelpCaseInsensitive(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("HELP", testArtefacts, "", hook); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "status") {
		t.Errorf("help output missing 'status' command")
	}
}

func TestDispatchStatus(t *testing.T) {
	hook := &captureHook{}
	// defaultRelease="" → auto-detects plucky (latest version)
	if err := Dispatch("status", testArtefacts, "", hook); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "plucky") {
		t.Errorf("status output should contain 'plucky', got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "Build Status") {
		t.Errorf("status output missing 'Build Status' header, got:\n%s", hook.last)
	}
}

func TestDispatchStatusPinnedRelease(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("status", testArtefacts, "noble", hook); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "noble") {
		t.Errorf("status should show noble when pinned, got:\n%s", hook.last)
	}
	// Should not show plucky artefacts when pinned to noble.
	if strings.Contains(hook.last, "ubuntu-desktop-amd64") && strings.Contains(hook.last, "plucky") {
		t.Errorf("status should not show plucky artefacts when pinned to noble")
	}
}

func TestDispatchStatusEmptySnapshot(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("status", nil, "", hook); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "No snapshot") {
		t.Errorf("expected 'No snapshot' message, got: %s", hook.last)
	}
}

func TestDispatchBuilds(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("builds noble", testArtefacts, "", hook); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "noble") {
		t.Errorf("builds output missing 'noble', got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "ubuntu-desktop-amd64") {
		t.Errorf("builds output missing artefact name, got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "ubuntu-server-amd64") {
		t.Errorf("builds output missing second artefact, got:\n%s", hook.last)
	}
	// plucky artefact must NOT appear in noble builds
	if strings.Contains(hook.last, "plucky") {
		t.Errorf("builds noble should not contain plucky artefact")
	}
}

func TestDispatchBuildsCaseInsensitive(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("builds Noble", testArtefacts, "", hook); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "ubuntu-desktop-amd64") {
		t.Errorf("builds Noble should return noble artefacts, got:\n%s", hook.last)
	}
}

func TestDispatchBuildsUnknownRelease(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("builds nonexistent", testArtefacts, "", hook); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "No builds found") {
		t.Errorf("expected 'No builds found' message, got: %s", hook.last)
	}
}

func TestDispatchBuildsNoRelease(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("builds", testArtefacts, "", hook); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "Usage") {
		t.Errorf("expected usage message, got: %s", hook.last)
	}
}

func TestDispatchReleases(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("releases", testArtefacts, "", hook); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "noble") {
		t.Errorf("releases output missing 'noble', got:\n%s", hook.last)
	}
	if !strings.Contains(hook.last, "plucky") {
		t.Errorf("releases output missing 'plucky', got:\n%s", hook.last)
	}
}

func TestDispatchUnknown(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("banana", testArtefacts, "", hook); err != nil {
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
	if err := Dispatch("   ", testArtefacts, "", hook); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty message — nothing should be sent.
	if hook.last != "" {
		t.Errorf("empty message should not produce output, got: %s", hook.last)
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
		got := imageAge(tc.version)
		if tc.wantErr && got != "unknown" {
			t.Errorf("imageAge(%q) = %q, want %q", tc.version, got, "unknown")
		}
		if !tc.wantErr && got == "unknown" {
			t.Errorf("imageAge(%q) returned %q unexpectedly", tc.version, got)
		}
	}
}
