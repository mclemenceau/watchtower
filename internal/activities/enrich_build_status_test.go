package activities

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mclemenceau/watchtower/internal/domain"
	"github.com/mclemenceau/watchtower/internal/ports"
)

// stubLogFetcher implements ports.LogFetcher via a URL→content map.
// Any URL not in the map returns domain.ErrLogNotFound.
type stubLogFetcher struct {
	logs map[string]string
}

func (s *stubLogFetcher) Fetch(_ context.Context, url string) (string, error) {
	if content, ok := s.logs[url]; ok {
		return content, nil
	}
	return "", domain.ErrLogNotFound
}

var _ ports.LogFetcher = (*stubLogFetcher)(nil)

// artefactForTest returns a minimal Artefact wired to a cdimage.ubuntu.com URL
// for the given product, release, logPrefix, date, and arch.
func artefactForTest(product, release, logPrefix, date, arch string) domain.Artefact {
	name := release + "-desktop-" + arch + ".iso"
	imageURL := "https://cdimage.ubuntu.com/" + product + "/" + release + "/" + logPrefix + "/" + date + "/" + name
	return domain.Artefact{
		ID:       1,
		Name:     name,
		Version:  date,
		OS:       product,
		Release:  release,
		ImageURL: imageURL,
	}
}

// logURL returns the expected cd-build-log URL for a given product/release/logPrefix/date.
func logURL(product, release, logPrefix, date string) string {
	return "https://ubuntu-archive-team.ubuntu.com/cd-build-logs/" + product + "/" + release + "/" + logPrefix + "-" + date + ".log"
}

// builtLog returns a minimal cd-build-log with a successful "finished at" line
// for the given arch label.
func builtLog(arch string) string {
	return arch + " on Launchpad starting at 2026-07-27 06:00:00\n" +
		arch + " on Launchpad finished at 2026-07-27 07:00:00 (Successfully built)\n"
}

// failedLog returns a minimal cd-build-log with a failed "finished at" line.
func failedLog(arch string) string {
	return arch + " on Launchpad starting at 2026-07-27 06:00:00\n" +
		arch + " on Launchpad finished at 2026-07-27 07:00:00 (Failed to build)\n"
}

// testObserverCrashLog returns a log where LP succeeded but cdimage crashed before
// submitting to Test Observer (the pattern seen for kubuntu/ubuntukylin).
func testObserverCrashLog(arch string) string {
	return arch + " on Launchpad starting at 2026-07-27 06:00:00\n" +
		arch + " on Launchpad finished at 2026-07-27 07:00:00 (Successfully built)\n" +
		"Couldn't submit artifact to Test Observer: Expecting value: line 1 column 1 (char 0)\n" +
		"Traceback (most recent call last):\n" +
		"  File \"build.py\", line 572, in build_image_set_locked\n" +
		"    publisher.publish(date)\n"
}

// todayYesterday returns today and yesterday in YYYYMMDD format (UTC).
func todayYesterday() (string, string) {
	now := time.Now().UTC()
	return now.Format("20060102"), now.AddDate(0, 0, -1).Format("20060102")
}

// TestEnrichBuildStatus_TodayLogBuilt: today's log is available and shows success.
func TestEnrichBuildStatus_TodayLogBuilt(t *testing.T) {
	today, _ := todayYesterday()
	art := artefactForTest("kubuntu", "stonking", "daily-live", "20260725", "amd64")

	fetcher := &stubLogFetcher{logs: map[string]string{
		logURL("kubuntu", "stonking", "daily-live", today): builtLog("kubuntu-amd64"),
	}}
	acts := &Activities{LogFetcher: fetcher}

	result, err := acts.EnrichBuildStatus(context.Background(), []domain.Artefact{art})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].BuildLog != domain.BuildStatusBuilt {
		t.Errorf("BuildLog = %q, want %q", result[0].BuildLog, domain.BuildStatusBuilt)
	}
}

// TestEnrichBuildStatus_TodayLogFailed: today's log shows a product failure.
func TestEnrichBuildStatus_TodayLogFailed(t *testing.T) {
	today, _ := todayYesterday()
	art := artefactForTest("kubuntu", "stonking", "daily-live", "20260725", "amd64")

	fetcher := &stubLogFetcher{logs: map[string]string{
		logURL("kubuntu", "stonking", "daily-live", today): failedLog("kubuntu-amd64"),
	}}
	acts := &Activities{LogFetcher: fetcher}

	result, err := acts.EnrichBuildStatus(context.Background(), []domain.Artefact{art})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].BuildLog != domain.BuildStatusFailed {
		t.Errorf("BuildLog = %q, want %q", result[0].BuildLog, domain.BuildStatusFailed)
	}
}

// TestEnrichBuildStatus_TodayMissingYesterdayBuilt: today's log is 404 but
// yesterday's log shows a successful build. The fallback must surface the
// yesterday result rather than returning NOT_STARTED.
// This is the kubuntu/ubuntukylin pattern: build succeeds on LP but cdimage
// crashes before submitting to Test Observer, then today's build hasn't run yet.
func TestEnrichBuildStatus_TodayMissingYesterdayBuilt(t *testing.T) {
	today, yesterday := todayYesterday()
	art := artefactForTest("kubuntu", "stonking", "daily-live", "20260725", "amd64")

	// today → 404 (not in map); yesterday → success
	fetcher := &stubLogFetcher{logs: map[string]string{
		logURL("kubuntu", "stonking", "daily-live", yesterday): builtLog("kubuntu-amd64"),
	}}
	_ = today // documented: today key absent → ErrLogNotFound
	acts := &Activities{LogFetcher: fetcher}

	result, err := acts.EnrichBuildStatus(context.Background(), []domain.Artefact{art})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].BuildLog != domain.BuildStatusBuilt {
		t.Errorf("BuildLog = %q, want %q (yesterday fallback should surface BUILT)",
			result[0].BuildLog, domain.BuildStatusBuilt)
	}
}

// TestEnrichBuildStatus_TodayMissingYesterdayTestObserverCrash: today's log is
// 404 and yesterday's log shows LP succeeded but cdimage crashed before
// submitting to Test Observer. Must return FAILED/INFRA, not NOT_STARTED.
func TestEnrichBuildStatus_TodayMissingYesterdayTestObserverCrash(t *testing.T) {
	_, yesterday := todayYesterday()
	art := artefactForTest("kubuntu", "stonking", "daily-live", "20260725", "amd64")

	fetcher := &stubLogFetcher{logs: map[string]string{
		logURL("kubuntu", "stonking", "daily-live", yesterday): testObserverCrashLog("kubuntu-amd64"),
	}}
	acts := &Activities{LogFetcher: fetcher}

	result, err := acts.EnrichBuildStatus(context.Background(), []domain.Artefact{art})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].BuildLog != domain.BuildStatusFailed {
		t.Errorf("BuildLog = %q, want %q", result[0].BuildLog, domain.BuildStatusFailed)
	}
	if result[0].BuildFailureKind != domain.BuildFailureKindInfra {
		t.Errorf("BuildFailureKind = %q, want %q", result[0].BuildFailureKind, domain.BuildFailureKindInfra)
	}
	if !strings.Contains(result[0].BuildFailureDescription, "Test Observer") {
		t.Errorf("BuildFailureDescription %q should mention Test Observer", result[0].BuildFailureDescription)
	}
}

// TestEnrichBuildStatus_BothLogsMissing: both today and yesterday return 404.
// Must return NOT_STARTED.
func TestEnrichBuildStatus_BothLogsMissing(t *testing.T) {
	art := artefactForTest("kubuntu", "stonking", "daily-live", "20260725", "amd64")
	fetcher := &stubLogFetcher{logs: map[string]string{}} // all URLs → 404
	acts := &Activities{LogFetcher: fetcher}

	result, err := acts.EnrichBuildStatus(context.Background(), []domain.Artefact{art})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].BuildLog != domain.BuildStatusNotStarted {
		t.Errorf("BuildLog = %q, want %q", result[0].BuildLog, domain.BuildStatusNotStarted)
	}
}

// TestEnrichBuildStatus_VersionIsToday: artefact version is today's date;
// must return BUILT immediately without any log fetch.
func TestEnrichBuildStatus_VersionIsToday(t *testing.T) {
	today, _ := todayYesterday()
	art := artefactForTest("kubuntu", "stonking", "daily-live", today, "amd64")
	// fetcher has no entries — any Fetch call would return 404 and break the test
	fetcher := &stubLogFetcher{logs: map[string]string{}}
	acts := &Activities{LogFetcher: fetcher}

	result, err := acts.EnrichBuildStatus(context.Background(), []domain.Artefact{art})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].BuildLog != domain.BuildStatusBuilt {
		t.Errorf("BuildLog = %q, want %q (today version must be BUILT without log fetch)",
			result[0].BuildLog, domain.BuildStatusBuilt)
	}
}

// TestEnrichBuildStatus_NoLogFetcher: nil LogFetcher leaves BuildLog empty.
func TestEnrichBuildStatus_NoLogFetcher(t *testing.T) {
	art := artefactForTest("kubuntu", "stonking", "daily-live", "20260725", "amd64")
	acts := &Activities{LogFetcher: nil}

	result, err := acts.EnrichBuildStatus(context.Background(), []domain.Artefact{art})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].BuildLog != "" {
		t.Errorf("BuildLog = %q, want empty (no fetcher)", result[0].BuildLog)
	}
}

// TestEnrichBuildStatus_TodayMissingYesterdayFailed: today is 404 and yesterday
// shows a product failure. Fallback must surface FAILED, not NOT_STARTED.
func TestEnrichBuildStatus_TodayMissingYesterdayFailed(t *testing.T) {
	_, yesterday := todayYesterday()
	art := artefactForTest("kubuntu", "stonking", "daily-live", "20260725", "amd64")

	fetcher := &stubLogFetcher{logs: map[string]string{
		logURL("kubuntu", "stonking", "daily-live", yesterday): failedLog("kubuntu-amd64"),
	}}
	acts := &Activities{LogFetcher: fetcher}

	result, err := acts.EnrichBuildStatus(context.Background(), []domain.Artefact{art})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].BuildLog != domain.BuildStatusFailed {
		t.Errorf("BuildLog = %q, want %q", result[0].BuildLog, domain.BuildStatusFailed)
	}
	if result[0].BuildFailureKind != domain.BuildFailureKindProduct {
		t.Errorf("BuildFailureKind = %q, want %q", result[0].BuildFailureKind, domain.BuildFailureKindProduct)
	}
}
