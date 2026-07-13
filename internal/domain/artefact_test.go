package domain

import (
	"strings"
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

// --- LogURLFromImageURLForDate ---

func TestLogURLFromImageURLForDate_SubstitutesDate(t *testing.T) {
	// An image URL with an old date; the returned log URL must use the supplied date.
	imageURL := "https://cdimage.ubuntu.com/ubuntu-server/stonking/daily-live/20260415/stonking-live-server-amd64.iso"
	want := "https://ubuntu-archive-team.ubuntu.com/cd-build-logs/ubuntu-server/stonking/daily-live-20261231.log"
	if got := LogURLFromImageURLForDate(imageURL, "20261231"); got != want {
		t.Errorf("LogURLFromImageURLForDate(%q, %q)\n got  %q\n want %q", imageURL, "20261231", got, want)
	}
}

func TestLogURLFromImageURLForDate_Empty(t *testing.T) {
	if got := LogURLFromImageURLForDate("", "20261231"); got != "" {
		t.Errorf("LogURLFromImageURLForDate(%q, %q) = %q, want empty string", "", "20261231", got)
	}
}

func TestLogURLFromImageURLForDate_WrongHost(t *testing.T) {
	imageURL := "https://example.com/ubuntu-server/stonking/daily-live/20260415/stonking-live-server-amd64.iso"
	if got := LogURLFromImageURLForDate(imageURL, "20261231"); got != "" {
		t.Errorf("LogURLFromImageURLForDate with wrong host should return %q, got %q", "", got)
	}
}

// --- LogCell ---

func TestLogCell_WithURL(t *testing.T) {
	// LogCell uses today's date regardless of the date embedded in the image URL.
	imageURL := "https://cdimage.ubuntu.com/ubuntu-server/stonking/daily-live/20260415/stonking-live-server-amd64.iso"
	today := time.Now().UTC().Format("20060102")
	wantLogURL := "https://ubuntu-archive-team.ubuntu.com/cd-build-logs/ubuntu-server/stonking/daily-live-" + today + ".log"
	want := "[🔗](" + wantLogURL + ")"
	if got := LogCell(imageURL); got != want {
		t.Errorf("LogCell(%q)\n got  %q\n want %q", imageURL, got, want)
	}
}

func TestLogCell_UsesTodayNotEmbeddedDate(t *testing.T) {
	// Explicitly verify that the log URL contains today's date, not the date in the image URL.
	imageURL := "https://cdimage.ubuntu.com/ubuntu-server/stonking/daily-live/19990101/stonking-live-server-amd64.iso"
	today := time.Now().UTC().Format("20060102")
	got := LogCell(imageURL)
	if got == "❌" {
		t.Fatalf("LogCell(%q) = ❌, want a valid link", imageURL)
	}
	if !strings.Contains(got, today) {
		t.Errorf("LogCell(%q)\n got  %q\n expected URL to contain today's date %q, not the embedded date 19990101", imageURL, got, today)
	}
	if strings.Contains(got, "19990101") {
		t.Errorf("LogCell(%q)\n got  %q\n URL must not contain the old embedded date 19990101", imageURL, got)
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

// --- IsDisplayable ---

func TestIsDisplayable_ImageBuild(t *testing.T) {
	te := TestExecution{TestPlan: "Image build", Status: "PASSED"}
	if IsDisplayable(te) {
		t.Error("Image build should not be displayable")
	}
}

func TestIsDisplayable_ManualTestingInProgress(t *testing.T) {
	te := TestExecution{TestPlan: "Manual Testing", Status: "IN_PROGRESS"}
	if IsDisplayable(te) {
		t.Error("Manual Testing IN_PROGRESS should not be displayable")
	}
}

func TestIsDisplayable_ManualTestingPassed(t *testing.T) {
	te := TestExecution{TestPlan: "Manual Testing", Status: "PASSED"}
	if !IsDisplayable(te) {
		t.Error("Manual Testing PASSED should be displayable")
	}
}

func TestIsDisplayable_JenkinsValidation(t *testing.T) {
	te := TestExecution{TestPlan: "Jenkins image validation", Status: "FAILED"}
	if !IsDisplayable(te) {
		t.Error("Jenkins image validation should be displayable")
	}
}

// --- ExecStatusEmoji ---

func TestExecStatusEmoji(t *testing.T) {
	cases := []struct {
		status string
		want   string
	}{
		{"PASSED", "✅"},
		{"FAILED", "❌"},
		{"IN_PROGRESS", "🔄"},
		{"NOT_STARTED", "⏳"},
		{"SOMETHING_ELSE", "⚠️"},
	}
	for _, tc := range cases {
		got := ExecStatusEmoji(tc.status)
		if got != tc.want {
			t.Errorf("ExecStatusEmoji(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

// --- ImageAge ---

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
		got := ImageAge(tc.version)
		if tc.wantErr && got != "unknown" {
			t.Errorf("ImageAge(%q) = %q, want %q", tc.version, got, "unknown")
		}
		if !tc.wantErr && got == "unknown" {
			t.Errorf("ImageAge(%q) returned %q unexpectedly", tc.version, got)
		}
	}
}
