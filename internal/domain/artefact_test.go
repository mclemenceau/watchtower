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

// --- ParseLaunchpadBuildURLs ---

func TestParseLaunchpadBuildURLs_TypicalLog(t *testing.T) {
	log := `===== Building live filesystems =====
ubuntu-amd64 on Launchpad starting at 2026-07-13 04:24:02
ubuntu-amd64: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/stonking/ubuntu/+build/989764
ubuntu-arm64: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/stonking/ubuntu/+build/989765
ubuntu-riscv64: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/stonking/ubuntu/+build/989766
ubuntu-amd64 on Launchpad finished at 2026-07-13 04:41:42 (Chroot problem)
`
	got := ParseLaunchpadBuildURLs(log)
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(got), got)
	}
	if got["amd64"] != "https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/stonking/ubuntu/+build/989764" {
		t.Errorf("amd64 URL mismatch: %q", got["amd64"])
	}
	if got["arm64"] != "https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/stonking/ubuntu/+build/989765" {
		t.Errorf("arm64 URL mismatch: %q", got["arm64"])
	}
	if got["riscv64"] != "https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/stonking/ubuntu/+build/989766" {
		t.Errorf("riscv64 URL mismatch: %q", got["riscv64"])
	}
}

func TestParseLaunchpadBuildURLs_NoLinks(t *testing.T) {
	log := "E: Some error\nFailed to build\n"
	got := ParseLaunchpadBuildURLs(log)
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestParseLaunchpadBuildURLs_Empty(t *testing.T) {
	got := ParseLaunchpadBuildURLs("")
	if len(got) != 0 {
		t.Errorf("expected empty map for empty input, got %v", got)
	}
}

func TestParseLaunchpadBuildURLs_IgnoresNonLaunchpadLines(t *testing.T) {
	log := `ubuntu-amd64: https://example.com/not-launchpad/+build/123
ubuntu-arm64: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/stonking/ubuntu/+build/999
ubuntu-riscv64: https://launchpad.net/~ubuntu-cdimage/no-build-keyword/path
`
	got := ParseLaunchpadBuildURLs(log)
	// Only arm64 should match — amd64 is not launchpad.net, riscv64 has no /+build/
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(got), got)
	}
	if _, ok := got["arm64"]; !ok {
		t.Errorf("expected arm64 in result, got %v", got)
	}
}

func TestParseLaunchpadBuildURLs_VariantBuildLabels(t *testing.T) {
	// Real-world log where the label is a full build name, not a bare arch.
	log := "ubuntu-desktop-preinstalled-arm64-raspi: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/resolute/ubuntu-preinstalled/+build/989089\n"
	got := ParseLaunchpadBuildURLs(log)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(got), got)
	}
	wantKey := "desktop-preinstalled-arm64-raspi"
	if _, ok := got[wantKey]; !ok {
		t.Errorf("expected key %q in result, got keys: %v", wantKey, func() []string {
			keys := make([]string, 0, len(got))
			for k := range got {
				keys = append(keys, k)
			}
			return keys
		}())
	}
}

func TestParseLaunchpadBuildURLs_FlavourPrefix(t *testing.T) {
	// Flavoured builds (edubuntu, xubuntu, kubuntu, etc.) use their own prefix.
	// The leading "<flavour>-" must be stripped so callers match on the arch portion.
	log := `edubuntu-arm64-raspi: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/resolute/edubuntu-preinstalled/+build/989883
xubuntu-amd64: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/resolute/xubuntu/+build/989652
`
	got := ParseLaunchpadBuildURLs(log)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(got), got)
	}
	cases := []struct {
		key  string
		want string
	}{
		{"arm64-raspi", "https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/resolute/edubuntu-preinstalled/+build/989883"},
		{"amd64", "https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/resolute/xubuntu/+build/989652"},
	}
	for _, tc := range cases {
		if got[tc.key] != tc.want {
			t.Errorf("key %q: got %q, want %q", tc.key, got[tc.key], tc.want)
		}
	}
}

// --- PrimaryBuildArch (composite arches) ---

func TestPrimaryBuildArch_CompositeArm64(t *testing.T) {
	// arm64+raspi should be preferred over alphabetical fallback
	builds := []ArtefactBuild{
		{Architecture: "arm64+raspi"},
	}
	if got := PrimaryBuildArch(builds); got != "arm64+raspi" {
		t.Errorf("PrimaryBuildArch = %q, want %q", got, "arm64+raspi")
	}
}

func TestPrimaryBuildArch_PrefersAMD64OverCompositeArm64(t *testing.T) {
	builds := []ArtefactBuild{
		{Architecture: "arm64+raspi"},
		{Architecture: "amd64"},
	}
	if got := PrimaryBuildArch(builds); got != "amd64" {
		t.Errorf("PrimaryBuildArch = %q, want %q", got, "amd64")
	}
}

// --- PrimaryBuildArch ---

func TestPrimaryBuildArch_PrefersAMD64(t *testing.T) {
	builds := []ArtefactBuild{
		{Architecture: "riscv64"},
		{Architecture: "arm64"},
		{Architecture: "amd64"},
	}
	if got := PrimaryBuildArch(builds); got != "amd64" {
		t.Errorf("PrimaryBuildArch = %q, want %q", got, "amd64")
	}
}

func TestPrimaryBuildArch_FallsBackToARM64(t *testing.T) {
	builds := []ArtefactBuild{
		{Architecture: "riscv64"},
		{Architecture: "arm64"},
	}
	if got := PrimaryBuildArch(builds); got != "arm64" {
		t.Errorf("PrimaryBuildArch = %q, want %q", got, "arm64")
	}
}

func TestPrimaryBuildArch_AlphabeticalFallback(t *testing.T) {
	builds := []ArtefactBuild{
		{Architecture: "s390x"},
		{Architecture: "ppc64el"},
		{Architecture: "riscv64"},
	}
	if got := PrimaryBuildArch(builds); got != "ppc64el" {
		t.Errorf("PrimaryBuildArch = %q, want %q (alphabetically first)", got, "ppc64el")
	}
}

func TestPrimaryBuildArch_Empty(t *testing.T) {
	if got := PrimaryBuildArch(nil); got != "" {
		t.Errorf("PrimaryBuildArch(nil) = %q, want %q", got, "")
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
		got := ImageAge(tc.version)
		if tc.wantErr && got != "unknown" {
			t.Errorf("ImageAge(%q) = %q, want %q", tc.version, got, "unknown")
		}
		if !tc.wantErr && got == "unknown" {
			t.Errorf("ImageAge(%q) returned %q unexpectedly", tc.version, got)
		}
	}
}
