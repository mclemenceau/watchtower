package logutil

import (
	"testing"
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
