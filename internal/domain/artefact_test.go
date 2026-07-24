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

// --- ArtefactArch ---

func TestArtefactArch(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		// Standard ISO builds.
		{"stonking-desktop-amd64.iso", "amd64"},
		{"stonking-live-server-amd64.iso", "amd64"},
		{"stonking-desktop-arm64.iso", "arm64"},
		{"noble-live-server-arm64.iso", "arm64"},
		{"stonking-desktop-riscv64.iso", "riscv64"},
		{"stonking-live-server-s390x.iso", "s390x"},
		{"stonking-live-server-ppc64el.iso", "ppc64el"},
		// Variant arches with "+" — normalised to "-".
		{"stonking-live-server-arm64+largemem.iso", "arm64-largemem"},
		{"stonking-preinstalled-server-arm64+raspi.img.xz", "arm64-raspi"},
		{"noble-preinstalled-server-riscv64+icicle.img.xz", "riscv64-icicle"},
		{"jammy-preinstalled-server-arm64+tegra-jetson.img.xz", "arm64-tegra-jetson"},
		// Other extension formats.
		{"stonking-wsl-amd64.wsl", "amd64"},
		{"stonking-base-arm64.tar.gz", "arm64"},
		{"stonking-mini-iso-amd64.iso", "amd64"},
		// Edge cases.
		{"noarch", ""},
		{"", ""},
	}

	for _, tc := range cases {
		got := ArtefactArch(tc.name)
		if got != tc.want {
			t.Errorf("ArtefactArch(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// --- ParseBuildStatusFromLog ---

// typicalServerLog is a real-format cd-build-log for ubuntu-server stonking
// (based on the actual log from 2026-07-24), with multiple arches showing
// different outcomes.
const typicalServerLog = `===== Building live filesystems =====
Fri Jul 24 08:18:43 UTC 2026
ubuntu-server-live-amd64 on Launchpad starting at 2026-07-24 08:18:43
Couldn't find a 'iso-stonking' target, using the default.
ubuntu-server-live-amd64: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/stonking/ubuntu-server-live/+build/996791
ubuntu-server-live-arm64 on Launchpad starting at 2026-07-24 08:18:46
Couldn't find a 'iso-stonking' target, using the default.
ubuntu-server-live-arm64: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/stonking/ubuntu-server-live/+build/996792
ubuntu-server-live-arm64-largemem on Launchpad starting at 2026-07-24 08:18:47
Couldn't find a 'iso-stonking' target, using the default.
ubuntu-server-live-arm64-largemem: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/stonking/ubuntu-server-live/+build/996793
ubuntu-server-live-riscv64 on Launchpad starting at 2026-07-24 08:18:51
Couldn't find a 'iso-stonking' target, using the default.
ubuntu-server-live-riscv64: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/stonking/ubuntu-server-live/+build/996796
ubuntu-server-live-arm64 on Launchpad finished at 2026-07-24 08:32:59 (Failed to build)
ubuntu-server-live-amd64 on Launchpad finished at 2026-07-24 08:54:04 (Successfully built)
ubuntu-server-live-arm64-largemem on Launchpad finished at 2026-07-24 13:45:09 (Successfully built)
`

// desktopLogInProgress is a log where the build has started but not finished.
const desktopLogInProgress = `===== Building live filesystems =====
ubuntu-amd64 on Launchpad starting at 2026-07-24 04:24:17
ubuntu-amd64: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/stonking/ubuntu/+build/996625
ubuntu-arm64 on Launchpad starting at 2026-07-24 04:24:19
ubuntu-arm64: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/stonking/ubuntu/+build/996626
`

// logWithChromeProblem contains a "finished" line whose suffix is neither "Successfully built"
// nor "Failed to build" — any non-success suffix should be treated as FAILED.
const logWithChromeProblem = `ubuntu-amd64 on Launchpad starting at 2026-07-24 04:24:17
ubuntu-amd64 on Launchpad finished at 2026-07-24 04:41:42 (Chroot problem)
`

// preinstalledLog contains raspi/largemem variant arch labels.
const preinstalledLog = `ubuntu-desktop-preinstalled-arm64-raspi on Launchpad starting at 2026-07-24 04:24:23
ubuntu-desktop-preinstalled-arm64-raspi on Launchpad finished at 2026-07-24 04:34:35 (Failed to build)
`

// preinstalledServerLog reflects the real daily-preinstalled log format where labels
// use "{product}-{arch}-{variant}" rather than bare arch tokens.
// Artefacts like "stonking-preinstalled-server-amd64.img.xz" (arch="amd64") or
// "noble-preinstalled-server-riscv64.img.xz" (arch="riscv64") must be matched
// via ResolveLogLabel → label="amd64-generic" / "riscv64-generic".
const preinstalledServerLog = `===== Building live filesystems =====
ubuntu-server-arm64-raspi on Launchpad starting at 2026-07-24 02:10:01
ubuntu-server-riscv64-generic on Launchpad starting at 2026-07-24 02:10:12
ubuntu-server-amd64-generic on Launchpad starting at 2026-07-24 02:10:15
ubuntu-server-arm64-generic on Launchpad starting at 2026-07-24 02:10:17
ubuntu-server-arm64-raspi on Launchpad finished at 2026-07-24 02:31:54 (Failed to build)
ubuntu-server-arm64-generic on Launchpad finished at 2026-07-24 02:42:19 (Failed to build)
ubuntu-server-amd64-generic on Launchpad finished at 2026-07-24 04:44:35 (Successfully built)
ubuntu-server-riscv64-generic on Launchpad finished at 2026-07-24 08:15:54 (Failed to build)
`

// --- LogPrefixFromImageURL ---

func TestLogPrefixFromImageURL(t *testing.T) {
	cases := []struct {
		imageURL string
		want     string
	}{
		{
			"https://cdimage.ubuntu.com/ubuntu-server/stonking/daily-preinstalled/20260627/stonking-preinstalled-server-arm64.img.xz",
			"daily-preinstalled",
		},
		{
			"https://cdimage.ubuntu.com/ubuntu-server/stonking/daily-live/20260724/stonking-live-server-amd64.iso",
			"daily-live",
		},
		{
			"https://cdimage.ubuntu.com/ubuntu/noble/daily-preinstalled/20260724/noble-preinstalled-desktop-arm64+raspi.img.xz",
			"daily-preinstalled",
		},
		// Invalid / empty inputs
		{"", ""},
		{"https://example.com/ubuntu-server/stonking/daily-live/20260724/file.iso", ""},
	}
	for _, tc := range cases {
		got := LogPrefixFromImageURL(tc.imageURL)
		if got != tc.want {
			t.Errorf("LogPrefixFromImageURL(%q) = %q, want %q", tc.imageURL, got, tc.want)
		}
	}
}

// --- ResolveLogLabel ---

func TestResolveLogLabel(t *testing.T) {
	cases := []struct {
		logPrefix string
		arch      string
		want      string
	}{
		// Preinstalled mappings — arch maps to canonical "{arch}-generic" label.
		{"daily-preinstalled", "amd64", "amd64-generic"},
		{"daily-preinstalled", "arm64", "arm64-generic"},
		{"daily-preinstalled", "riscv64", "riscv64-generic"},
		// daily-live: no mapping — label equals arch unchanged.
		{"daily-live", "amd64", "amd64"},
		{"daily-live", "arm64", "arm64"},
		{"daily-live", "riscv64", "riscv64"},
		// Variant arches (already-qualified): no mapping — label returned as-is.
		{"daily-preinstalled", "arm64-raspi", "arm64-raspi"},
		{"daily-preinstalled", "arm64-largemem", "arm64-largemem"},
		// Unknown log prefix: passthrough.
		{"daily", "amd64", "amd64"},
		// Empty inputs.
		{"", "amd64", "amd64"},
		{"daily-preinstalled", "", ""},
	}
	for _, tc := range cases {
		got := ResolveLogLabel(tc.logPrefix, tc.arch)
		if got != tc.want {
			t.Errorf("ResolveLogLabel(%q, %q) = %q, want %q", tc.logPrefix, tc.arch, got, tc.want)
		}
	}
}

func TestParseBuildStatusFromLog(t *testing.T) {
	cases := []struct {
		desc    string
		content string
		arch    string
		want    BuildStatusState
	}{
		// Empty content → not started.
		{"empty content", "", "amd64", BuildStatusNotStarted},
		// Empty arch → not started.
		{"empty arch", typicalServerLog, "", BuildStatusNotStarted},
		// amd64 successfully built.
		{"amd64 successfully built", typicalServerLog, "amd64", BuildStatusBuilt},
		// arm64 failed.
		{"arm64 failed", typicalServerLog, "arm64", BuildStatusFailed},
		// arm64-largemem successfully built.
		{"arm64-largemem built", typicalServerLog, "arm64-largemem", BuildStatusBuilt},
		// riscv64 started but not finished → in progress.
		{"riscv64 in progress", typicalServerLog, "riscv64", BuildStatusInProgress},
		// Desktop build in progress (started, no finished line yet).
		{"desktop amd64 in progress", desktopLogInProgress, "amd64", BuildStatusInProgress},
		{"desktop arm64 in progress", desktopLogInProgress, "arm64", BuildStatusInProgress},
		// Arch not present in log → not started.
		{"s390x not in log", typicalServerLog, "s390x", BuildStatusNotStarted},
		// Chroot problem suffix → FAILED.
		{"amd64 chroot problem", logWithChromeProblem, "amd64", BuildStatusFailed},
		// arm64+raspi: arm64-raspi matches "ubuntu-server-arm64-raspi" via suffix — FAILED.
		{"arm64-raspi failed", preinstalledLog, "arm64-raspi", BuildStatusFailed},
		// Arch with "+" normalised to "-" by caller (ArtefactArch already normalises).
		{"arm64+largemem normalised", typicalServerLog, "arm64+largemem", BuildStatusBuilt},
		// Preinstalled server: raw arch "amd64" does NOT match "ubuntu-server-amd64-generic".
		// After ResolveLogLabel the caller must pass "amd64-generic" instead.
		{"preinstalled amd64 raw arch no match", preinstalledServerLog, "amd64", BuildStatusNotStarted},
		// With the resolved label "amd64-generic" the match succeeds → BUILT.
		{"preinstalled amd64-generic built", preinstalledServerLog, "amd64-generic", BuildStatusBuilt},
		// arm64 raw arch does not match "ubuntu-server-arm64-generic" or "ubuntu-server-arm64-raspi".
		{"preinstalled arm64 raw arch no match", preinstalledServerLog, "arm64", BuildStatusNotStarted},
		// With resolved label "arm64-generic" → FAILED (arm64-generic finished Failed to build).
		{"preinstalled arm64-generic failed", preinstalledServerLog, "arm64-generic", BuildStatusFailed},
		// riscv64-generic → FAILED.
		{"preinstalled riscv64-generic failed", preinstalledServerLog, "riscv64-generic", BuildStatusFailed},
	}

	for _, tc := range cases {
		got := ParseBuildStatusFromLog(tc.content, tc.arch)
		if got != tc.want {
			t.Errorf("ParseBuildStatusFromLog(%q, %q) = %q, want %q",
				tc.desc, tc.arch, got, tc.want)
		}
	}
}

// --- BuildLogIcon ---

func TestBuildLogIcon(t *testing.T) {
	cases := []struct {
		state BuildStatusState
		want  string
	}{
		{BuildStatusBuilt, "✅"},
		{BuildStatusNotStarted, "⏳"},
		{BuildStatusInProgress, "🔄"},
		{BuildStatusFailed, "❌"},
		{BuildStatusUnknown, "❓"},
		{"", "❓"},
	}
	for _, tc := range cases {
		got := BuildLogIcon(tc.state)
		if got != tc.want {
			t.Errorf("BuildLogIcon(%q) = %q, want %q", tc.state, got, tc.want)
		}
	}
}
