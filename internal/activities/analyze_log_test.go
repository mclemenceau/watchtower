package activities

import (
	"context"
	"testing"

	"github.com/mclemenceau/argus/internal/llm"
)

func TestAnalyzeLogParsesJSON(t *testing.T) {
	response := `{"category":"dependency","hypothesis":"apt cannot locate package libfoo-dev","log_excerpts":["E: Unable to locate package libfoo-dev"],"next_action":"Check apt sources.list for the plucky pocket"}`

	act := &Activities{LLM: &llm.MockLLMClient{Response: response}}

	result, err := act.AnalyzeLog(context.Background(), "ubuntu-server-amd64", "E: Unable to locate package libfoo-dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Category != "dependency" {
		t.Errorf("Category: got %q, want %q", result.Category, "dependency")
	}
	if result.Hypothesis == "" {
		t.Error("Hypothesis should not be empty")
	}
	if len(result.LogExcerpts) == 0 {
		t.Error("LogExcerpts should not be empty")
	}
	if result.NextAction == "" {
		t.Error("NextAction should not be empty")
	}
}

func TestAnalyzeLogStripsCodeFence(t *testing.T) {
	response := "```json\n{\"category\":\"infra\",\"hypothesis\":\"runner OOM\",\"log_excerpts\":[\"Killed\"],\"next_action\":\"retry on larger runner\"}\n```"

	act := &Activities{LLM: &llm.MockLLMClient{Response: response}}

	result, err := act.AnalyzeLog(context.Background(), "ubuntu-desktop-amd64", "Killed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Category != "infra" {
		t.Errorf("Category: got %q, want %q", result.Category, "infra")
	}
}

func TestAnalyzeLogInvalidJSON(t *testing.T) {
	act := &Activities{LLM: &llm.MockLLMClient{Response: "not json at all"}}

	_, err := act.AnalyzeLog(context.Background(), "x", "log")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestImageAge(t *testing.T) {
	cases := []struct {
		version string
		wantErr bool // "unknown" returned
	}{
		{"20240101", false},
		{"20240101.1", false},  // respin suffix
		{"20240101.12", false}, // double-digit respin
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
