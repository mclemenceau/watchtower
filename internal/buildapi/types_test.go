package buildapi

import (
	"testing"
	"time"
)

// --- LogURLFromImageURL ---

func TestLogURLFromImageURL_HappyPath(t *testing.T) {
	// Exact example from cd-build-log-map.json
	imageURL := "https://cdimage.ubuntu.com/ubuntu-server/stonking/daily-live/20260415/stonking-live-server-amd64.iso"
	want := "https://ubuntu-archive-team.ubuntu.com/cd-build-logs/ubuntu-server/stonking/daily-live-20260415.log"
	if got := LogURLFromImageURL(imageURL); got != want {
		t.Errorf("LogURLFromImageURL(%q)\n got  %q\n want %q", imageURL, got, want)
	}
}

func TestLogURLFromImageURL_RespinVersion(t *testing.T) {
	// Date segment may carry a .N respin suffix — strip it
	imageURL := "https://cdimage.ubuntu.com/ubuntu/stonking/daily-live/20260415.2/stonking-desktop-amd64.iso"
	want := "https://ubuntu-archive-team.ubuntu.com/cd-build-logs/ubuntu/stonking/daily-live-20260415.log"
	if got := LogURLFromImageURL(imageURL); got != want {
		t.Errorf("LogURLFromImageURL(%q)\n got  %q\n want %q", imageURL, got, want)
	}
}

func TestLogURLFromImageURL_Empty(t *testing.T) {
	if got := LogURLFromImageURL(""); got != "" {
		t.Errorf("LogURLFromImageURL(%q) = %q, want empty string", "", got)
	}
}

func TestLogURLFromImageURL_WrongHost(t *testing.T) {
	imageURL := "https://example.com/ubuntu-server/stonking/daily-live/20260415/stonking-live-server-amd64.iso"
	if got := LogURLFromImageURL(imageURL); got != "" {
		t.Errorf("LogURLFromImageURL with wrong host should return %q, got %q", "", got)
	}
}

func TestLogURLFromImageURL_TooFewSegments(t *testing.T) {
	imageURL := "https://cdimage.ubuntu.com/ubuntu-server/stonking/daily-live"
	if got := LogURLFromImageURL(imageURL); got != "" {
		t.Errorf("LogURLFromImageURL with too few segments should return %q, got %q", "", got)
	}
}

func TestLogURLFromImageURL_InvalidDate(t *testing.T) {
	// Date segment is not 8 digits
	imageURL := "https://cdimage.ubuntu.com/ubuntu-server/stonking/daily-live/notadate/stonking-live-server-amd64.iso"
	if got := LogURLFromImageURL(imageURL); got != "" {
		t.Errorf("LogURLFromImageURL with invalid date should return %q, got %q", "", got)
	}
}

// --- BuildStatus ---

func TestBuildStatus_BuiltToday(t *testing.T) {
	version := time.Now().UTC().Format("20060102")
	got := BuildStatus(version)
	if got != "✅" {
		t.Errorf("BuildStatus(today) = %q, want %q", got, "✅")
	}
}

func TestBuildStatus_NotBuilt(t *testing.T) {
	got := BuildStatus("20200101")
	if got != "❌" {
		t.Errorf("BuildStatus(old) = %q, want %q", got, "❌")
	}
}

// --- LogCell ---

func TestLogCell_WithURL(t *testing.T) {
	imageURL := "https://cdimage.ubuntu.com/ubuntu-server/stonking/daily-live/20260415/stonking-live-server-amd64.iso"
	logURL := "https://ubuntu-archive-team.ubuntu.com/cd-build-logs/ubuntu-server/stonking/daily-live-20260415.log"
	want := "[🔗](" + logURL + ")"
	if got := LogCell(imageURL); got != want {
		t.Errorf("LogCell(%q)\n got  %q\n want %q", imageURL, got, want)
	}
}

func TestLogCell_NoURL(t *testing.T) {
	if got := LogCell(""); got != "❌" {
		t.Errorf("LogCell(%q) = %q, want %q", "", got, "❌")
	}
}

func TestLogCell_MalformedURL(t *testing.T) {
	if got := LogCell("https://not-cdimage.example.com/bad/path"); got != "❌" {
		t.Errorf("LogCell(malformed) = %q, want %q", got, "❌")
	}
}
