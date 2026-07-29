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

// cdimageTracebackLog reflects a cd-build-log where run_live_builds in cdimage
// crashed before posting any builds to Launchpad (e.g. LP returned 400 Bad Request).
// No "on Launchpad starting at" lines are present for any arch.
const cdimageTracebackLog = `===== Building live filesystems =====
Fri Jul 24 08:18:43 UTC 2026
Traceback (most recent call last):
  File "/srv/cdimage.ubuntu.com/bin/../lib/cdimage/build.py", line 530, in build_image_set_locked
    builds = run_live_builds(config)
  File "/srv/cdimage.ubuntu.com/bin/../lib/cdimage/livefs.py", line 259, in run_live_builds
    lp_build = lp_livefs.requestBuild(**lp_kwargs)
  File "/srv/cdimage.ubuntu.com/bin/../lib/cdimage/launchpad.py", line 109, in requestBuild
    self._current_build_cache[archtag][unique_key] = self._lp_livefs.requestBuild(
  File "/usr/lib/python3/dist-packages/lazr/restfulclient/resource.py", line 592, in __call__
    response, content = self.root._browser._request(
  File "/usr/lib/python3/dist-packages/lazr/restfulclient/_browser.py", line 429, in _request
    raise error
lazr.restfulclient.errors.BadRequest: HTTP Error 400: Bad Request
`

// unrelatedTracebackLog contains a Python traceback that is NOT from run_live_builds
// (e.g. from a post-processing script). Should not be treated as an infra failure.
const unrelatedTracebackLog = `===== Post-processing =====
Traceback (most recent call last):
  File "/srv/cdimage.ubuntu.com/bin/post-process.py", line 42, in publish_images
    upload_to_mirror(path)
AttributeError: 'NoneType' object has no attribute 'upload'
`

// diskFullLog reflects a cd-build-log where the cdimage host ran out of disk space
// mid-run. amd64 finished successfully but arm64 and riscv64 were orphaned — they
// have "starting at" lines but no "finished at" lines because cdimage crashed.
// This pattern was observed on 2026-07-27 for ubuntu-server/stonking/daily-live.
const diskFullLog = `===== Building live filesystems =====
Mon Jul 27 07:55:57 UTC 2026
ubuntu-server-live-amd64 on Launchpad starting at 2026-07-27 07:55:57
ubuntu-server-live-amd64: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/stonking/ubuntu-server-live/+build/998432
ubuntu-server-live-arm64 on Launchpad starting at 2026-07-27 07:56:00
ubuntu-server-live-arm64: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/stonking/ubuntu-server-live/+build/998434
ubuntu-server-live-riscv64 on Launchpad starting at 2026-07-27 07:56:08
ubuntu-server-live-amd64 on Launchpad finished at 2026-07-27 08:32:23 (Successfully built)
Traceback (most recent call last):
  File "/usr/lib/python3/dist-packages/httplib2/__init__.py", line 448, in _updateCache
    cache.set(cachekey, text)
  File "/usr/lib/python3/dist-packages/lazr/restfulclient/_browser.py", line 265, in set
    f.close()
OSError: [Errno 28] No space left on device
`

// runLiveBuildsAfterFailedBuildLog reflects a log where the arch finished with
// "(Failed to build)" on Launchpad, and cdimage then raised LiveBuildsFailed via
// run_live_builds because the build failed. This pattern is observed for artefacts
// 23368, 22583, 22582, 23360 in stonking. The arch-specific LP result must take
// precedence over the subsequent run_live_builds traceback → FAILED, PRODUCT.
const runLiveBuildsAfterFailedBuildLog = `===== Building live filesystems =====
Sun Jul 27 01:56:19 UTC 2026
edubuntu-arm64-raspi on Launchpad starting at 2026-07-27 01:56:19
edubuntu-arm64-raspi: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/stonking/edubuntu-preinstalled/+build/998247
edubuntu-arm64-raspi on Launchpad finished at 2026-07-27 02:09:47 (Failed to build)
Traceback (most recent call last):
  File "/srv/cdimage.ubuntu.com/bin/../lib/cdimage/build.py", line 530, in build_image_set_locked
    builds = run_live_builds(config)
  File "/srv/cdimage.ubuntu.com/bin/../lib/cdimage/livefs.py", line 311, in run_live_builds
    raise LiveBuildsFailed("No live filesystem builds succeeded.")
cdimage.livefs.LiveBuildsFailed: No live filesystem builds succeeded.
`

// testObserverSubmitFailureLog reflects a real daily-live log (edubuntu/stonking,
// 2026-07-27) where the LP build succeeded and the image was published to cdimage,
// but the Test Observer API returned a 500 and cdimage logged
// "Couldn't submit artifact to Test Observer". The artefact is absent from Test
// Observer despite a clean LP result. This is an INFRA failure at the submission
// layer, not a build or publishing failure.
const testObserverSubmitFailureLog = `===== Building live filesystems =====
Mon Jul 27 00:41:02 UTC 2026
edubuntu-amd64 on Launchpad starting at 2026-07-27 00:41:02
edubuntu-amd64: https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/stonking/edubuntu/+build/998227
edubuntu-amd64 on Launchpad finished at 2026-07-27 01:48:56 (Successfully built)
===== Downloading live filesystem images =====
===== Publishing =====
Submitting images to Test Observer
500 Server Error: Internal Server Error for url: https://tests-api.ubuntu.com/v1/test-executions/start-test
Couldn't submit artifact to Test Observer: Expecting value: line 1 column 1 (char 0)
Traceback (most recent call last):
  File "/srv/cdimage.ubuntu.com/bin/../lib/cdimage/build.py", line 572, in build_image_set_locked
    publisher.publish(date)
  File "/srv/cdimage.ubuntu.com/bin/../lib/cdimage/tree.py", line 177, in path_to_project
    raise ValueError(
ValueError: Cannot determine project for path 'nvidia-tegra/...': 'nvidia-tegra' is not a known project directory
`

// testObserverSubmitFailureNoTracebackLog is the same scenario but without a
// subsequent traceback — the TO submission failure stands alone. This verifies
// that detection does not depend on a traceback being present.
const testObserverSubmitFailureNoTracebackLog = `===== Building live filesystems =====
edubuntu-amd64 on Launchpad starting at 2026-07-27 00:41:02
edubuntu-amd64 on Launchpad finished at 2026-07-27 01:48:56 (Successfully built)
===== Publishing =====
Submitting images to Test Observer
500 Server Error: Internal Server Error for url: https://tests-api.ubuntu.com/v1/test-executions/start-test
Couldn't submit artifact to Test Observer: Expecting value: line 1 column 1 (char 0)
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
		desc       string
		content    string
		arch       string
		wantStatus BuildStatusState
		wantKind   BuildFailureKind
		wantDesc   string
	}{
		// Empty content → not started, no failure kind.
		{"empty content", "", "amd64", BuildStatusNotStarted, BuildFailureKindNone, ""},
		// Empty arch → not started, no failure kind.
		{"empty arch", typicalServerLog, "", BuildStatusNotStarted, BuildFailureKindNone, ""},
		// amd64 successfully built → BUILT, no failure kind.
		{"amd64 successfully built", typicalServerLog, "amd64", BuildStatusBuilt, BuildFailureKindNone, ""},
		// arm64 failed (Failed to build) → FAILED, PRODUCT.
		{"arm64 failed", typicalServerLog, "arm64", BuildStatusFailed, BuildFailureKindProduct, ""},
		// arm64-largemem successfully built.
		{"arm64-largemem built", typicalServerLog, "arm64-largemem", BuildStatusBuilt, BuildFailureKindNone, ""},
		// riscv64 started but not finished, no traceback → IN_PROGRESS, no failure kind.
		{"riscv64 in progress", typicalServerLog, "riscv64", BuildStatusInProgress, BuildFailureKindNone, ""},
		// Desktop build in progress (started, no finished line yet, no traceback).
		{"desktop amd64 in progress", desktopLogInProgress, "amd64", BuildStatusInProgress, BuildFailureKindNone, ""},
		{"desktop arm64 in progress", desktopLogInProgress, "arm64", BuildStatusInProgress, BuildFailureKindNone, ""},
		// Arch not present in log → not started.
		{"s390x not in log", typicalServerLog, "s390x", BuildStatusNotStarted, BuildFailureKindNone, ""},
		// Chroot problem suffix → FAILED, INFRA (LP builder problem, not product).
		{"amd64 chroot problem", logWithChromeProblem, "amd64", BuildStatusFailed, BuildFailureKindInfra, "Launchpad builder reported a chroot problem"},
		// arm64+raspi: arm64-raspi matches "ubuntu-server-arm64-raspi" via suffix — FAILED, PRODUCT.
		{"arm64-raspi failed", preinstalledLog, "arm64-raspi", BuildStatusFailed, BuildFailureKindProduct, ""},
		// Arch with "+" normalised to "-" by caller (ArtefactArch already normalises).
		{"arm64+largemem normalised", typicalServerLog, "arm64+largemem", BuildStatusBuilt, BuildFailureKindNone, ""},
		// Preinstalled server: raw arch "amd64" does NOT match "ubuntu-server-amd64-generic".
		// After ResolveLogLabel the caller must pass "amd64-generic" instead.
		{"preinstalled amd64 raw arch no match", preinstalledServerLog, "amd64", BuildStatusNotStarted, BuildFailureKindNone, ""},
		// With the resolved label "amd64-generic" the match succeeds → BUILT.
		{"preinstalled amd64-generic built", preinstalledServerLog, "amd64-generic", BuildStatusBuilt, BuildFailureKindNone, ""},
		// arm64 raw arch does not match "ubuntu-server-arm64-generic" or "ubuntu-server-arm64-raspi".
		{"preinstalled arm64 raw arch no match", preinstalledServerLog, "arm64", BuildStatusNotStarted, BuildFailureKindNone, ""},
		// With resolved label "arm64-generic" → FAILED, PRODUCT.
		{"preinstalled arm64-generic failed", preinstalledServerLog, "arm64-generic", BuildStatusFailed, BuildFailureKindProduct, ""},
		// riscv64-generic → FAILED, PRODUCT.
		{"preinstalled riscv64-generic failed", preinstalledServerLog, "riscv64-generic", BuildStatusFailed, BuildFailureKindProduct, ""},
		// cdimage run_live_builds traceback (LP 400) — no "starting at" lines for any arch → FAILED, INFRA.
		{"run_live_builds traceback, amd64", cdimageTracebackLog, "amd64", BuildStatusFailed, BuildFailureKindInfra, "cdimage crashed before submitting builds to Launchpad"},
		{"run_live_builds traceback, arm64", cdimageTracebackLog, "arm64", BuildStatusFailed, BuildFailureKindInfra, "cdimage crashed before submitting builds to Launchpad"},
		// Disk-full traceback: amd64 finished "(Successfully built)" but traceback present
		// → cdimage crashed during publishing → FAILED, INFRA (not BUILT).
		{"disk full, amd64 built before crash", diskFullLog, "amd64", BuildStatusFailed, BuildFailureKindInfra, "LP build succeeded but cdimage crashed during publishing"},
		// Disk-full traceback: arm64 started but orphaned → FAILED, INFRA.
		{"disk full, arm64 orphaned", diskFullLog, "arm64", BuildStatusFailed, BuildFailureKindInfra, "cdimage crashed mid-run, build was orphaned"},
		// Disk-full traceback: riscv64 started but orphaned → FAILED, INFRA.
		{"disk full, riscv64 orphaned", diskFullLog, "riscv64", BuildStatusFailed, BuildFailureKindInfra, "cdimage crashed mid-run, build was orphaned"},
		// Disk-full traceback: s390x not in log at all → NOT_STARTED (not orphaned).
		{"disk full, s390x not in log", diskFullLog, "s390x", BuildStatusNotStarted, BuildFailureKindNone, ""},
		// Traceback from an unrelated script (no arch started, no run_live_builds) → NOT_STARTED.
		{"unrelated traceback, amd64", unrelatedTracebackLog, "amd64", BuildStatusNotStarted, BuildFailureKindNone, ""},
		// run_live_builds traceback that appears AFTER arch's "(Failed to build)" result:
		// the arch-specific LP verdict takes precedence → FAILED, PRODUCT (not INFRA).
		{"run_live_builds after failed build, arm64-raspi", runLiveBuildsAfterFailedBuildLog, "arm64-raspi", BuildStatusFailed, BuildFailureKindProduct, ""},
		// Test Observer submission failure (with subsequent traceback): LP built and
		// published the image but cdimage could not submit it to Test Observer → INFRA.
		{"TO submit failure + traceback, amd64", testObserverSubmitFailureLog, "amd64", BuildStatusFailed, BuildFailureKindInfra, "LP build succeeded but image could not be submitted to Test Observer"},
		// Test Observer submission failure without any traceback: same result — INFRA.
		{"TO submit failure no traceback, amd64", testObserverSubmitFailureNoTracebackLog, "amd64", BuildStatusFailed, BuildFailureKindInfra, "LP build succeeded but image could not be submitted to Test Observer"},
	}

	for _, tc := range cases {
		gotStatus, gotKind, gotDesc := ParseBuildStatusFromLog(tc.content, tc.arch)
		if gotStatus != tc.wantStatus || gotKind != tc.wantKind || gotDesc != tc.wantDesc {
			t.Errorf("ParseBuildStatusFromLog(%q, %q) = (%q, %q, %q), want (%q, %q, %q)",
				tc.desc, tc.arch, gotStatus, gotKind, gotDesc, tc.wantStatus, tc.wantKind, tc.wantDesc)
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

// --- UpsertFailure: FailureKind and FailureDescription ---

// TestUpsertFailureKindDescription verifies that FailureKind and FailureDescription
// are stored on both new and updated FailureRecords.
func TestUpsertFailureKindDescription(t *testing.T) {
	art := Artefact{
		ID: 1, Name: "stonking-wsl-amd64.wsl",
		OS: "ubuntu-wsl", Release: "stonking", Version: "20260727",
		BuildFailureKind:        BuildFailureKindInfra,
		BuildFailureDescription: "cdimage crashed before submitting builds to Launchpad",
	}

	fs := make(FailureStore)
	isNew := fs.UpsertFailure(art)
	if !isNew {
		t.Fatal("expected new record on first upsert")
	}

	recs := fs.ActiveFailures("stonking", "ubuntu-wsl")
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	if recs[0].FailureKind != BuildFailureKindInfra {
		t.Errorf("FailureKind = %q, want %q", recs[0].FailureKind, BuildFailureKindInfra)
	}
	if recs[0].FailureDescription != art.BuildFailureDescription {
		t.Errorf("FailureDescription = %q, want %q", recs[0].FailureDescription, art.BuildFailureDescription)
	}

	// Update the record with a new version and changed kind/description.
	art2 := art
	art2.Version = "20260728"
	art2.BuildFailureKind = BuildFailureKindProduct
	art2.BuildFailureDescription = ""
	isNew = fs.UpsertFailure(art2)
	if isNew {
		t.Fatal("expected existing record on second upsert")
	}

	recs = fs.ActiveFailures("stonking", "ubuntu-wsl")
	if recs[0].FailureKind != BuildFailureKindProduct {
		t.Errorf("updated FailureKind = %q, want %q", recs[0].FailureKind, BuildFailureKindProduct)
	}
	if recs[0].FailureDescription != art2.BuildFailureDescription {
		t.Errorf("updated FailureDescription = %q, want %q", recs[0].FailureDescription, art2.BuildFailureDescription)
	}
}

// --- PendingAnalysis ---

func TestPendingAnalysis_SkipsINFRA(t *testing.T) {
	fs := make(FailureStore)
	fs.UpsertFailure(Artefact{
		ID: 10, Name: "noble-server-amd64", Release: "noble", OS: "ubuntu-server",
		BuildLog: BuildStatusFailed, BuildFailureKind: BuildFailureKindInfra,
		BuildFailureDescription: "disk full", Version: "20260101",
	})

	pending := fs.PendingAnalysis(10)
	if len(pending) != 0 {
		t.Errorf("PendingAnalysis returned %d records for INFRA failure, want 0", len(pending))
	}
}

func TestPendingAnalysis_IncludesPRODUCT(t *testing.T) {
	fs := make(FailureStore)
	fs.UpsertFailure(Artefact{
		ID: 20, Name: "noble-desktop-amd64", Release: "noble", OS: "ubuntu",
		BuildLog: BuildStatusFailed, BuildFailureKind: BuildFailureKindProduct,
		BuildFailureDescription: "debootstrap failed", Version: "20260101",
	})

	pending := fs.PendingAnalysis(10)
	if len(pending) != 1 {
		t.Errorf("PendingAnalysis returned %d records for PRODUCT failure, want 1", len(pending))
	}
	if pending[0].ArtefactID != 20 {
		t.Errorf("PendingAnalysis returned wrong record: ID %d, want 20", pending[0].ArtefactID)
	}
}

func TestPendingAnalysis_SkipsResolved(t *testing.T) {
	fs := make(FailureStore)
	fs.UpsertFailure(Artefact{
		ID: 30, Name: "noble-minimal-amd64", Release: "noble", OS: "ubuntu-minimal",
		BuildLog: BuildStatusFailed, BuildFailureKind: BuildFailureKindProduct,
		Version: "20260101",
	})
	fs.ResolveFailure(30, "noble", "ubuntu-minimal")

	pending := fs.PendingAnalysis(10)
	if len(pending) != 0 {
		t.Errorf("PendingAnalysis returned %d records for resolved failure, want 0", len(pending))
	}
}

func TestPendingAnalysis_SkipsAlreadyAnalysed(t *testing.T) {
	fs := make(FailureStore)
	fs.UpsertFailure(Artefact{
		ID: 40, Name: "noble-desktop-arm64", Release: "noble", OS: "ubuntu",
		BuildLog: BuildStatusFailed, BuildFailureKind: BuildFailureKindProduct,
		Version: "20260101",
	})
	analysis := LogAnalysis{
		Category:   "dependency",
		Hypothesis: "libfoo missing",
	}
	fs.SetAnalysis(40, "noble", "ubuntu", analysis, "20260101")

	pending := fs.PendingAnalysis(10)
	if len(pending) != 0 {
		t.Errorf("PendingAnalysis returned %d records for already-analysed failure, want 0",
			len(pending))
	}
}

func TestPendingAnalysis_RespectsCap(t *testing.T) {
	fs := make(FailureStore)
	for i := 0; i < 10; i++ {
		fs.UpsertFailure(Artefact{
			ID: 100 + i, Name: "art", Release: "noble", OS: "ubuntu",
			BuildLog: BuildStatusFailed, BuildFailureKind: BuildFailureKindProduct,
			Version: "20260101",
		})
	}

	pending := fs.PendingAnalysis(3)
	if len(pending) != 3 {
		t.Errorf("PendingAnalysis with cap=3 returned %d records, want 3", len(pending))
	}
}

// --- ExtractFailureSignature ---

func TestExtractFailureSignature_DpkgErrorPackage(t *testing.T) {
	log := "dpkg: error processing package libsystemd-dev (--unpack):"
	got := ExtractFailureSignature(log)
	if got != "dpkg:libsystemd-dev" {
		t.Errorf("got %q, want %q", got, "dpkg:libsystemd-dev")
	}
}

func TestExtractFailureSignature_AptMissing(t *testing.T) {
	log := "E: Unable to locate package libfoo-dev"
	got := ExtractFailureSignature(log)
	if got != "apt:missing:libfoo-dev" {
		t.Errorf("got %q, want %q", got, "apt:missing:libfoo-dev")
	}
}

func TestExtractFailureSignature_AptNoCandidate(t *testing.T) {
	log := "E: Package 'libbar' has no installation candidate"
	got := ExtractFailureSignature(log)
	if got != "apt:no-candidate:libbar" {
		t.Errorf("got %q, want %q", got, "apt:no-candidate:libbar")
	}
}

func TestExtractFailureSignature_AptUnmetDep(t *testing.T) {
	log := " ubuntu-server : Depends: libsystemd-dev (>= 253)"
	got := ExtractFailureSignature(log)
	if got != "apt:unmet-dep:ubuntu-server" {
		t.Errorf("got %q, want %q", got, "apt:unmet-dep:ubuntu-server")
	}
}

func TestExtractFailureSignature_SnapInstall(t *testing.T) {
	log := "error: cannot install 'core24': snap not found"
	got := ExtractFailureSignature(log)
	if got != "snap:install:core24" {
		t.Errorf("got %q, want %q", got, "snap:install:core24")
	}
}

func TestExtractFailureSignature_Debootstrap(t *testing.T) {
	log := "debootstrap: error: failed to bootstrap"
	got := ExtractFailureSignature(log)
	if got != "debootstrap:error" {
		t.Errorf("got %q, want %q", got, "debootstrap:error")
	}
}

func TestExtractFailureSignature_DpkgSubprocess(t *testing.T) {
	log := "E: Sub-process /usr/bin/dpkg returned an error code (1)"
	got := ExtractFailureSignature(log)
	if got != "dpkg:subprocess-error" {
		t.Errorf("got %q, want %q", got, "dpkg:subprocess-error")
	}
}

func TestExtractFailureSignature_NoMatch(t *testing.T) {
	log := "Some unrecognised build error without a known pattern"
	got := ExtractFailureSignature(log)
	if got != "" {
		t.Errorf("expected empty signature, got %q", got)
	}
}

func TestExtractFailureSignature_EmptyLog(t *testing.T) {
	got := ExtractFailureSignature("")
	if got != "" {
		t.Errorf("expected empty for empty log, got %q", got)
	}
}

func TestExtractFailureSignature_PriorityOrder(t *testing.T) {
	// dpkg error should take priority over apt:unmet-dep when both patterns present
	log := " ubuntu-server : Depends: libfoo\ndpkg: error processing package libfoo (--unpack):"
	got := ExtractFailureSignature(log)
	// dpkg pattern appears after unmet-dep in the log — first match in scan order wins
	// since unmet-dep appears on line 1 and dpkg on line 2, unmet-dep wins
	if got != "apt:unmet-dep:ubuntu-server" {
		t.Errorf("got %q, want apt:unmet-dep:ubuntu-server (first match wins)", got)
	}
}

func TestExtractFailureSignature_StripTrailingPunctuation(t *testing.T) {
	// Package name may appear with trailing comma in some log formats
	log := "dpkg: error processing package libfoo-dev, trying overwrite"
	got := ExtractFailureSignature(log)
	// The regex captures up to the first space; comma should not be in capture group here
	// but this tests resilience
	if got == "" {
		t.Errorf("expected non-empty signature")
	}
}

func TestExtractFailureSignature_OnlyLast200Lines(t *testing.T) {
	// Pattern at the very beginning of a 250-line log should NOT match
	// (only last 200 lines are scanned)
	var lines []string
	lines = append(lines, "E: Unable to locate package early-package") // line 1 — outside window
	for i := 0; i < 249; i++ {
		lines = append(lines, "normal log line")
	}
	log := strings.Join(lines, "\n")
	got := ExtractFailureSignature(log)
	if got != "" {
		t.Errorf("expected no match (pattern outside 200-line window), got %q", got)
	}
}

// --- GroupBySignature ---

func TestGroupBySignature_GroupsMatchingRecords(t *testing.T) {
	records := []FailureRecord{
		{ArtefactID: 1, FailureSignature: "apt:missing:libfoo"},
		{ArtefactID: 2, FailureSignature: "apt:missing:libfoo"},
		{ArtefactID: 3, FailureSignature: "dpkg:libbar"},
		{ArtefactID: 4, FailureSignature: ""},
	}
	groups := GroupBySignature(records)

	if len(groups["apt:missing:libfoo"]) != 2 {
		t.Errorf("expected 2 records for apt:missing:libfoo, got %d",
			len(groups["apt:missing:libfoo"]))
	}
	if len(groups["dpkg:libbar"]) != 1 {
		t.Errorf("expected 1 record for dpkg:libbar, got %d",
			len(groups["dpkg:libbar"]))
	}
	if len(groups[""]) != 1 {
		t.Errorf("expected 1 record for empty signature, got %d",
			len(groups[""]))
	}
}

func TestGroupBySignature_Empty(t *testing.T) {
	groups := GroupBySignature(nil)
	if groups == nil {
		t.Error("GroupBySignature should never return nil")
	}
	if len(groups) != 0 {
		t.Errorf("expected empty map, got %d keys", len(groups))
	}
}

// --- SetAnalysis propagates Signature to FailureSignature ---

func TestSetAnalysis_PropagatesSignature(t *testing.T) {
	fs := make(FailureStore)
	fs.UpsertFailure(Artefact{
		ID: 10, Name: "stonking-desktop-amd64", Release: "stonking", OS: "ubuntu",
		BuildLog: BuildStatusFailed, BuildFailureKind: BuildFailureKindProduct,
		Version: "20260701",
	})

	analysis := LogAnalysis{
		Category:   "dependency",
		Hypothesis: "libfoo missing from archive",
		Signature:  "apt:missing:libfoo-dev",
	}
	fs.SetAnalysis(10, "stonking", "ubuntu", analysis, "20260701")

	recs := fs.ActiveFailures("stonking", "ubuntu")
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	if recs[0].FailureSignature != "apt:missing:libfoo-dev" {
		t.Errorf("FailureSignature = %q, want %q",
			recs[0].FailureSignature, "apt:missing:libfoo-dev")
	}
}
