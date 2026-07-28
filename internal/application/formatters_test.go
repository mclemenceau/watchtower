package application

import (
	"strings"
	"testing"
	"time"

	"github.com/mclemenceau/watchtower/internal/domain"
)

// --- FormatBuildsStatusSummary ---

func TestFormatBuildsStatusSummary_ColumnHeaders(t *testing.T) {
	artefacts := []domain.Artefact{
		{ID: 1, Release: "noble", Version: today},
	}
	out := FormatBuildsStatusSummary(artefacts)
	for _, want := range []string{"Release", "Built", "Total", "Progress"} {
		if !strings.Contains(out, want) {
			t.Errorf("FormatBuildsStatusSummary missing column header %q, got:\n%s", want, out)
		}
	}
}

func TestFormatBuildsStatusSummary_ProgressBar100Pct(t *testing.T) {
	artefacts := []domain.Artefact{
		{ID: 1, Release: "noble", Version: today},
		{ID: 2, Release: "noble", Version: today},
	}
	out := FormatBuildsStatusSummary(artefacts)
	wantBar := strings.Repeat("🟩", 10)
	if !strings.Contains(out, wantBar) {
		t.Errorf("100%% bar should be 10 green squares, got:\n%s", out)
	}
	if strings.Contains(out, "🟥") {
		t.Errorf("100%% bar should have no red squares, got:\n%s", out)
	}
}

func TestFormatBuildsStatusSummary_ProgressBar0Pct(t *testing.T) {
	artefacts := []domain.Artefact{
		{ID: 1, Release: "noble", Version: yesterday},
	}
	out := FormatBuildsStatusSummary(artefacts)
	wantBar := strings.Repeat("🟥", 10)
	if !strings.Contains(out, wantBar) {
		t.Errorf("0%% bar should be 10 red squares, got:\n%s", out)
	}
	if strings.Contains(out, "🟩") {
		t.Errorf("0%% bar should have no green squares, got:\n%s", out)
	}
}

func TestFormatBuildsStatusSummary_EmptySnapshot(t *testing.T) {
	out := FormatBuildsStatusSummary(nil)
	if !strings.Contains(out, "No snapshot") {
		t.Errorf("expected 'No snapshot' message, got: %s", out)
	}
}

func TestFormatBuildsStatusSummary_MultipleReleases(t *testing.T) {
	artefacts := []domain.Artefact{
		{ID: 1, Release: "noble", Version: today},
		{ID: 2, Release: "plucky", Version: yesterday},
	}
	out := FormatBuildsStatusSummary(artefacts)
	if !strings.Contains(out, "noble") {
		t.Errorf("expected 'noble' in summary, got:\n%s", out)
	}
	if !strings.Contains(out, "plucky") {
		t.Errorf("expected 'plucky' in summary, got:\n%s", out)
	}
}

// --- FormatBuildsStatusRelease ---

func TestFormatBuildsStatusRelease_ColumnHeaders(t *testing.T) {
	artefacts := []domain.Artefact{
		{ID: 1, Name: "ubuntu-desktop-amd64", OS: "ubuntu", Release: "noble", Version: today},
	}
	out := FormatBuildsStatusRelease(artefacts, "noble", "")
	for _, want := range []string{"ID", "Artefact", "Product", "Version", "Age", "Build", "Log"} {
		if !strings.Contains(out, want) {
			t.Errorf("FormatBuildsStatusRelease missing column header %q, got:\n%s", want, out)
		}
	}
}

func TestFormatBuildsStatusRelease_ArtefactRow(t *testing.T) {
	artefacts := []domain.Artefact{
		{ID: 1, Name: "ubuntu-desktop-amd64", OS: "ubuntu", Release: "noble", Version: today},
	}
	out := FormatBuildsStatusRelease(artefacts, "noble", "")
	if !strings.Contains(out, "ubuntu-desktop-amd64") {
		t.Errorf("expected artefact name in output, got:\n%s", out)
	}
	if !strings.Contains(out, "✅") {
		t.Errorf("expected ✅ for today's build, got:\n%s", out)
	}
}

func TestFormatBuildsStatusRelease_LogLinkPresent(t *testing.T) {
	imageURL := "https://cdimage.ubuntu.com/ubuntu-server/noble/daily-live/20200101/noble-live-server-amd64.iso"
	// LogCell always uses today's date, not the date embedded in imageURL.
	todayDate := time.Now().UTC().Format("20060102")
	logURL := "https://ubuntu-archive-team.ubuntu.com/cd-build-logs/ubuntu-server/noble/daily-live-" + todayDate + ".log"
	artefacts := []domain.Artefact{
		{ID: 1, Name: "ubuntu-server-amd64", OS: "ubuntu-server", Release: "noble", Version: "20200101", ImageURL: imageURL},
	}
	out := FormatBuildsStatusRelease(artefacts, "noble", "")
	wantLink := "[🔗](" + logURL + ")"
	if !strings.Contains(out, wantLink) {
		t.Errorf("expected Markdown log link %q, got:\n%s", wantLink, out)
	}
}

func TestFormatBuildsStatusRelease_EmptySnapshot(t *testing.T) {
	out := FormatBuildsStatusRelease(nil, "noble", "")
	if !strings.Contains(out, "No snapshot") {
		t.Errorf("expected 'No snapshot' message, got: %s", out)
	}
}

func TestFormatBuildsStatusRelease_UnknownRelease(t *testing.T) {
	artefacts := []domain.Artefact{
		{ID: 1, Name: "x", OS: "ubuntu", Release: "noble", Version: today},
	}
	out := FormatBuildsStatusRelease(artefacts, "nonexistent", "")
	if !strings.Contains(out, "No artefacts found") {
		t.Errorf("expected 'No artefacts found' message, got: %s", out)
	}
}

func TestFormatBuildsStatusRelease_ProductFilter(t *testing.T) {
	artefacts := []domain.Artefact{
		{ID: 1, Name: "ubuntu-desktop-amd64", OS: "ubuntu", Release: "noble", Version: today},
		{ID: 2, Name: "ubuntu-server-amd64", OS: "ubuntu-server", Release: "noble", Version: today},
	}
	out := FormatBuildsStatusRelease(artefacts, "noble", "ubuntu-server")
	if !strings.Contains(out, "ubuntu-server-amd64") {
		t.Errorf("expected ubuntu-server-amd64 in output, got:\n%s", out)
	}
	if strings.Contains(out, "ubuntu-desktop-amd64") {
		t.Errorf("ubuntu-desktop-amd64 should be filtered out, got:\n%s", out)
	}
}

func TestFormatBuildsStatusRelease_FailedWithDescription(t *testing.T) {
	artefacts := []domain.Artefact{
		{
			ID: 1, Name: "stonking-wsl-amd64.wsl", OS: "ubuntu-wsl", Release: "stonking",
			Version:                 "20260720",
			BuildLog:                domain.BuildStatusFailed,
			BuildFailureKind:        domain.BuildFailureKindInfra,
			BuildFailureDescription: "cdimage crashed before submitting builds to Launchpad",
		},
	}
	out := FormatBuildsStatusRelease(artefacts, "stonking", "")
	want := "❌ INFRA: cdimage crashed before submitting builds to Launchpad"
	if !strings.Contains(out, want) {
		t.Errorf("expected %q in output, got:\n%s", want, out)
	}
}

func TestFormatBuildsStatusRelease_FailedKindNoDescription(t *testing.T) {
	// FailureKind set but no description — shows "❌ PRODUCT" without colon suffix.
	artefacts := []domain.Artefact{
		{
			ID: 2, Name: "stonking-preinstalled-server-arm64+raspi.img.xz", OS: "ubuntu-server",
			Release:          "stonking",
			Version:          "20260720",
			BuildLog:         domain.BuildStatusFailed,
			BuildFailureKind: domain.BuildFailureKindProduct,
		},
	}
	out := FormatBuildsStatusRelease(artefacts, "stonking", "")
	if !strings.Contains(out, "❌ PRODUCT") {
		t.Errorf("expected '❌ PRODUCT' in output, got:\n%s", out)
	}
	if strings.Contains(out, "❌ PRODUCT:") {
		t.Errorf("expected no colon after PRODUCT when description is empty, got:\n%s", out)
	}
}

func TestFormatBuildsStatusRelease_FailedNoKind(t *testing.T) {
	// No kind set — shows plain "❌".
	artefacts := []domain.Artefact{
		{
			ID: 3, Name: "stonking-desktop-arm64.iso", OS: "ubuntu",
			Release:  "stonking",
			Version:  "20260720",
			BuildLog: domain.BuildStatusFailed,
		},
	}
	out := FormatBuildsStatusRelease(artefacts, "stonking", "")
	if !strings.Contains(out, "❌") {
		t.Errorf("expected ❌ in output, got:\n%s", out)
	}
	if strings.Contains(out, "INFRA") || strings.Contains(out, "PRODUCT") {
		t.Errorf("expected no kind label when BuildFailureKind is empty, got:\n%s", out)
	}
}

func TestFormatTestsStatusSummary_ColumnHeaders(t *testing.T) {
	env := domain.Environment{Name: "jenkins", Architecture: "amd64"}
	artefacts := []domain.Artefact{
		{
			ID: 1, Release: "noble", Version: today,
			Builds: []domain.ArtefactBuild{{
				ID: 10, Architecture: "amd64",
				TestExecutions: []domain.TestExecution{
					{ID: 100, TestPlan: "Jenkins image validation", Status: "PASSED", Environment: env},
				},
			}},
		},
	}
	out := FormatTestsStatusSummary(artefacts)
	for _, want := range []string{"Release", "Passed", "Total", "Progress"} {
		if !strings.Contains(out, want) {
			t.Errorf("FormatTestsStatusSummary missing column header %q, got:\n%s", want, out)
		}
	}
}

func TestFormatTestsStatusSummary_PassedFraction(t *testing.T) {
	env := domain.Environment{Name: "jenkins", Architecture: "amd64"}
	artefacts := []domain.Artefact{
		{
			ID: 1, Release: "noble", Version: today,
			Builds: []domain.ArtefactBuild{{
				ID: 10, Architecture: "amd64",
				TestExecutions: []domain.TestExecution{
					{ID: 100, TestPlan: "Jenkins image validation", Status: "PASSED", Environment: env},
					{ID: 101, TestPlan: "Manual Testing", Status: "PASSED", Environment: env},
				},
			}},
		},
	}
	out := FormatTestsStatusSummary(artefacts)
	// 2/2 passed → 100% bar
	wantBar := strings.Repeat("🟩", 10)
	if !strings.Contains(out, wantBar) {
		t.Errorf("100%% bar expected, got:\n%s", out)
	}
}

func TestFormatTestsStatusSummary_EmptySnapshot(t *testing.T) {
	out := FormatTestsStatusSummary(nil)
	if !strings.Contains(out, "No snapshot") {
		t.Errorf("expected 'No snapshot' message, got: %s", out)
	}
}

func TestFormatTestsStatusSummary_NoBuildsInSnapshot(t *testing.T) {
	artefacts := []domain.Artefact{
		{ID: 1, Release: "noble", Version: today},
	}
	out := FormatTestsStatusSummary(artefacts)
	if !strings.Contains(out, "No test executions found") {
		t.Errorf("expected 'No test executions found' message, got: %s", out)
	}
}

// --- FormatTestsStatusRelease ---

func TestFormatTestsStatusRelease_ColumnHeaders(t *testing.T) {
	env := domain.Environment{Name: "jenkins", Architecture: "amd64"}
	artefacts := []domain.Artefact{
		{
			ID: 1, Name: "noble-desktop-amd64.iso", OS: "ubuntu", Release: "noble", Version: today,
			Builds: []domain.ArtefactBuild{{
				ID: 10, Architecture: "amd64",
				TestExecutions: []domain.TestExecution{
					{ID: 100, TestPlan: "Jenkins image validation", Status: "PASSED", Environment: env},
				},
			}},
		},
	}
	out := FormatTestsStatusRelease(artefacts, "noble", "")
	for _, want := range []string{"Artefact", "Product", "Arch", "Test Plan", "Status"} {
		if !strings.Contains(out, want) {
			t.Errorf("FormatTestsStatusRelease missing column header %q, got:\n%s", want, out)
		}
	}
}

func TestFormatTestsStatusRelease_ExecRows(t *testing.T) {
	env := domain.Environment{Name: "jenkins", Architecture: "amd64"}
	artefacts := []domain.Artefact{
		{
			ID: 1, Name: "noble-desktop-amd64.iso", OS: "ubuntu", Release: "noble", Version: today,
			Builds: []domain.ArtefactBuild{{
				ID: 10, Architecture: "amd64",
				TestExecutions: []domain.TestExecution{
					{ID: 100, TestPlan: "Jenkins image validation", Status: "PASSED", Environment: env},
					{ID: 101, TestPlan: "Image build", Status: "PASSED", Environment: env}, // filtered
				},
			}},
		},
	}
	out := FormatTestsStatusRelease(artefacts, "noble", "")
	if !strings.Contains(out, "noble-desktop-amd64.iso") {
		t.Errorf("expected artefact name in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Jenkins image validation") {
		t.Errorf("expected test plan in output, got:\n%s", out)
	}
	if !strings.Contains(out, "✅") {
		t.Errorf("expected ✅ for PASSED status, got:\n%s", out)
	}
	// Image build should be filtered
	if strings.Contains(out, "Image build") {
		t.Errorf("Image build should be filtered from output, got:\n%s", out)
	}
}

func TestFormatTestsStatusRelease_EmptySnapshot(t *testing.T) {
	out := FormatTestsStatusRelease(nil, "noble", "")
	if !strings.Contains(out, "No snapshot") {
		t.Errorf("expected 'No snapshot' message, got: %s", out)
	}
}

// --- FormatChangeReport ---

func TestFormatChangeReport_NewFailures(t *testing.T) {
	r := domain.ChangeReport{
		NewFailures: []domain.ArtefactDelta{
			{Name: "ubuntu-desktop-amd64", Release: "noble", OldStatus: "UNDECIDED", NewStatus: "MARKED_AS_FAILED"},
		},
	}
	out := FormatChangeReport(r)
	if !strings.Contains(out, "🔴") {
		t.Errorf("expected 🔴 for new failures, got:\n%s", out)
	}
	if !strings.Contains(out, "New Failures") {
		t.Errorf("expected 'New Failures' section, got:\n%s", out)
	}
	if !strings.Contains(out, "ubuntu-desktop-amd64") {
		t.Errorf("expected artefact name in output, got:\n%s", out)
	}
}

func TestFormatChangeReport_Recoveries(t *testing.T) {
	r := domain.ChangeReport{
		Recoveries: []domain.ArtefactDelta{
			{Name: "ubuntu-server-amd64", Release: "noble"},
		},
	}
	out := FormatChangeReport(r)
	if !strings.Contains(out, "🟢") {
		t.Errorf("expected 🟢 for recoveries, got:\n%s", out)
	}
	if !strings.Contains(out, "Recoveries") {
		t.Errorf("expected 'Recoveries' section, got:\n%s", out)
	}
}

func TestFormatChangeReport_OtherChanges(t *testing.T) {
	r := domain.ChangeReport{
		OtherChanges: []domain.ArtefactDelta{
			{Name: "ubuntu-minimal-amd64", Release: "plucky", OldStatus: "UNDECIDED", NewStatus: "APPROVED"},
		},
	}
	out := FormatChangeReport(r)
	if !strings.Contains(out, "🔵") {
		t.Errorf("expected 🔵 for other changes, got:\n%s", out)
	}
	if !strings.Contains(out, "Other Changes") {
		t.Errorf("expected 'Other Changes' section, got:\n%s", out)
	}
}

func TestFormatChangeReport_NewArtefacts(t *testing.T) {
	r := domain.ChangeReport{
		NewArtefacts: []domain.Artefact{
			{ID: 1, Name: "brand-new-iso", Release: "noble", OS: "ubuntu", Version: time.Now().UTC().Format("20060102")},
		},
	}
	out := FormatChangeReport(r)
	if !strings.Contains(out, "🆕") {
		t.Errorf("expected 🆕 for new artefacts, got:\n%s", out)
	}
	if !strings.Contains(out, "New Artefacts") {
		t.Errorf("expected 'New Artefacts' section, got:\n%s", out)
	}
	if !strings.Contains(out, "brand-new-iso") {
		t.Errorf("expected artefact name, got:\n%s", out)
	}
}

func TestFormatChangeReport_EmptyReport(t *testing.T) {
	r := domain.ChangeReport{}
	out := FormatChangeReport(r)
	// Should still produce header
	if !strings.Contains(out, "Change Report") {
		t.Errorf("expected 'Change Report' header, got:\n%s", out)
	}
	// Should NOT include any section headers
	for _, section := range []string{"New Failures", "Recoveries", "Other Changes", "New Artefacts"} {
		if strings.Contains(out, section) {
			t.Errorf("empty report should not contain section %q, got:\n%s", section, out)
		}
	}
}

// --- FormatInvestigation ---

func TestFormatInvestigation_RequiredFields(t *testing.T) {
	art := domain.Artefact{ID: 42, Name: "noble-desktop-amd64.iso", OS: "ubuntu", Release: "noble", Version: yesterday}
	analysis := domain.LogAnalysis{
		Category:    "dependency",
		Hypothesis:  "apt mirror returned 404 for linux-image package",
		LogExcerpts: []string{"E: Failed to fetch http://example.com 404", "dpkg: error processing package"},
		NextAction:  "Retry the build after mirror sync",
	}
	out := FormatInvestigation(art, analysis, "Launchpad librarian (amd64)")

	for _, want := range []string{
		"noble-desktop-amd64.iso",
		"42",
		"dependency",
		"apt mirror returned 404",
		"E: Failed to fetch http://example.com 404",
		"dpkg: error processing package",
		"Retry the build after mirror sync",
		"Category",
		"Hypothesis",
		"Recommended action",
		"Log source:",
		"Launchpad librarian (amd64)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("FormatInvestigation missing %q, got:\n%s", want, out)
		}
	}
}

func TestFormatInvestigation_NoLogExcerpts(t *testing.T) {
	art := domain.Artefact{ID: 1, Name: "plucky-server-amd64.iso", OS: "ubuntu-server", Release: "plucky", Version: yesterday}
	analysis := domain.LogAnalysis{
		Category:    "unknown",
		Hypothesis:  "Could not determine root cause",
		LogExcerpts: nil,
		NextAction:  "Investigate manually",
	}
	out := FormatInvestigation(art, analysis, "cd-build-log")
	if !strings.Contains(out, "plucky-server-amd64.iso") {
		t.Errorf("expected artefact name, got:\n%s", out)
	}
	if strings.Contains(out, "Relevant log excerpts") {
		t.Errorf("should not include 'Relevant log excerpts' section when empty, got:\n%s", out)
	}
	if !strings.Contains(out, "Investigate manually") {
		t.Errorf("expected next action, got:\n%s", out)
	}
	if !strings.Contains(out, "cd-build-log") {
		t.Errorf("expected source 'cd-build-log', got:\n%s", out)
	}
}

// --- FormatScheduledSummary ---

func TestFormatScheduledSummary_EmptySnapshot(t *testing.T) {
	out := FormatScheduledSummary(nil, nil)
	if !strings.Contains(out, "No snapshot") {
		t.Errorf("expected 'No snapshot' message, got: %s", out)
	}
}

func TestFormatScheduledSummary_AllBuiltToday(t *testing.T) {
	artefacts := []domain.Artefact{
		{ID: 1, Name: "noble-desktop-amd64.iso", OS: "ubuntu", Release: "noble", Version: today},
		{ID: 2, Name: "noble-server-amd64.iso", OS: "ubuntu-server", Release: "noble", Version: today},
	}
	out := FormatScheduledSummary(artefacts, []string{"noble"})
	if !strings.Contains(out, "noble") {
		t.Errorf("expected release name 'noble', got:\n%s", out)
	}
	if !strings.Contains(out, "2/2") {
		t.Errorf("expected 2/2 built, got:\n%s", out)
	}
	// All built → sunny emoji, no Infra/Product sections.
	if !strings.Contains(out, "☀️") {
		t.Errorf("expected sunny emoji for 100%%, got:\n%s", out)
	}
	if strings.Contains(out, "Infra") || strings.Contains(out, "Product") {
		t.Errorf("expected no failure sections when all built, got:\n%s", out)
	}
}

func TestFormatScheduledSummary_InfraFailures(t *testing.T) {
	artefacts := []domain.Artefact{
		{ID: 1, Name: "noble-desktop-amd64.iso", OS: "ubuntu", Release: "noble", Version: today},
		{
			ID: 2, Name: "noble-server-amd64.iso", OS: "ubuntu-server", Release: "noble",
			Version:                 yesterday,
			BuildLog:                domain.BuildStatusFailed,
			BuildFailureKind:        domain.BuildFailureKindInfra,
			BuildFailureDescription: "cdimage crashed mid-run, build was orphaned",
		},
		{
			ID: 3, Name: "noble-server-arm64.iso", OS: "ubuntu-server", Release: "noble",
			Version:                 yesterday,
			BuildLog:                domain.BuildStatusFailed,
			BuildFailureKind:        domain.BuildFailureKindInfra,
			BuildFailureDescription: "cdimage crashed mid-run, build was orphaned",
		},
	}
	out := FormatScheduledSummary(artefacts, []string{"noble"})
	if !strings.Contains(out, "Infra (2):") {
		t.Errorf("expected Infra section header, got:\n%s", out)
	}
	// ubuntu-server has both amd64 and arm64 failing → should appear in Infra line.
	if !strings.Contains(out, "ubuntu-server") {
		t.Errorf("expected ubuntu-server in Infra group, got:\n%s", out)
	}
	// ubuntu was built today → must NOT appear in failures.
	if strings.Contains(out, "Product") {
		t.Errorf("expected no Product section, got:\n%s", out)
	}
}

func TestFormatScheduledSummary_ProductFailures(t *testing.T) {
	artefacts := []domain.Artefact{
		{
			ID: 1, Name: "noble-preinstalled-server-arm64+raspi.img.xz", OS: "ubuntu-server",
			Release:                 "noble",
			Version:                 yesterday,
			BuildLog:                domain.BuildStatusFailed,
			BuildFailureKind:        domain.BuildFailureKindProduct,
			BuildFailureDescription: "livefs build failure requires analysis",
		},
		{
			ID: 2, Name: "noble-wsl-amd64.wsl", OS: "ubuntu-wsl",
			Release:                 "noble",
			Version:                 yesterday,
			BuildLog:                domain.BuildStatusFailed,
			BuildFailureKind:        domain.BuildFailureKindProduct,
			BuildFailureDescription: "livefs build failure requires analysis",
		},
	}
	out := FormatScheduledSummary(artefacts, []string{"noble"})
	if !strings.Contains(out, "Product (2):") {
		t.Errorf("expected Product section header, got:\n%s", out)
	}
	if !strings.Contains(out, "ubuntu-server") {
		t.Errorf("expected ubuntu-server in Product group, got:\n%s", out)
	}
	if strings.Contains(out, "Infra") {
		t.Errorf("expected no Infra section, got:\n%s", out)
	}
}

func TestFormatScheduledSummary_MixedInfraAndProduct(t *testing.T) {
	artefacts := []domain.Artefact{
		{
			ID: 1, Name: "noble-desktop-amd64.iso", OS: "ubuntu", Release: "noble",
			Version:                 yesterday,
			BuildLog:                domain.BuildStatusFailed,
			BuildFailureKind:        domain.BuildFailureKindInfra,
			BuildFailureDescription: "cdimage crashed before submitting builds to Launchpad",
		},
		{
			ID: 2, Name: "noble-wsl-amd64.wsl", OS: "ubuntu-wsl", Release: "noble",
			Version:                 yesterday,
			BuildLog:                domain.BuildStatusFailed,
			BuildFailureKind:        domain.BuildFailureKindProduct,
			BuildFailureDescription: "livefs build failure requires analysis",
		},
	}
	out := FormatScheduledSummary(artefacts, []string{"noble"})
	if !strings.Contains(out, "Infra (1):") {
		t.Errorf("expected Infra section, got:\n%s", out)
	}
	if !strings.Contains(out, "Product (1):") {
		t.Errorf("expected Product section, got:\n%s", out)
	}
	// Infra must appear before Product in the output.
	infraPos := strings.Index(out, "Infra")
	productPos := strings.Index(out, "Product")
	if infraPos > productPos {
		t.Errorf("expected Infra section before Product, got:\n%s", out)
	}
}

func TestFormatScheduledSummary_MultipleDescriptions(t *testing.T) {
	// Two INFRA failures with different descriptions → both collapsed into one Infra line.
	artefacts := []domain.Artefact{
		{
			ID: 1, Name: "noble-desktop-amd64.iso", OS: "ubuntu", Release: "noble",
			Version:                 yesterday,
			BuildLog:                domain.BuildStatusFailed,
			BuildFailureKind:        domain.BuildFailureKindInfra,
			BuildFailureDescription: "cdimage crashed before submitting builds to Launchpad",
		},
		{
			ID: 2, Name: "noble-server-amd64.iso", OS: "ubuntu-server", Release: "noble",
			Version:                 yesterday,
			BuildLog:                domain.BuildStatusFailed,
			BuildFailureKind:        domain.BuildFailureKindInfra,
			BuildFailureDescription: "LP build succeeded but image could not be submitted to Test Observer",
		},
	}
	out := FormatScheduledSummary(artefacts, []string{"noble"})
	if !strings.Contains(out, "Infra (2):") {
		t.Errorf("expected Infra section with count 2, got:\n%s", out)
	}
	// Both products should appear in the single Infra line.
	if !strings.Contains(out, "ubuntu") {
		t.Errorf("expected ubuntu in Infra line, got:\n%s", out)
	}
	if !strings.Contains(out, "ubuntu-server") {
		t.Errorf("expected ubuntu-server in Infra line, got:\n%s", out)
	}
}

func TestFormatScheduledSummary_ProductsWithSameArchCollapsed(t *testing.T) {
	// ubuntu and ubuntu-mate both fail on amd64 with same description → one bullet line.
	artefacts := []domain.Artefact{
		{
			ID: 1, Name: "noble-desktop-amd64.iso", OS: "ubuntu", Release: "noble",
			Version:                 yesterday,
			BuildLog:                domain.BuildStatusFailed,
			BuildFailureKind:        domain.BuildFailureKindInfra,
			BuildFailureDescription: "LP build succeeded but image could not be submitted to Test Observer",
		},
		{
			ID: 2, Name: "noble-desktop-amd64.iso", OS: "ubuntu-mate", Release: "noble",
			Version:                 yesterday,
			BuildLog:                domain.BuildStatusFailed,
			BuildFailureKind:        domain.BuildFailureKindInfra,
			BuildFailureDescription: "LP build succeeded but image could not be submitted to Test Observer",
		},
	}
	out := FormatScheduledSummary(artefacts, []string{"noble"})
	// Both products share amd64 → should be collapsed onto one token "ubuntu, ubuntu-mate (amd64)".
	if !strings.Contains(out, "ubuntu, ubuntu-mate (amd64)") {
		t.Errorf("expected collapsed product token 'ubuntu, ubuntu-mate (amd64)', got:\n%s", out)
	}
}

func TestFormatScheduledSummary_ReleaseOrderRespected(t *testing.T) {
	artefacts := []domain.Artefact{
		{ID: 1, Name: "noble-desktop-amd64.iso", Release: "noble", Version: today},
		{ID: 2, Name: "jammy-desktop-amd64.iso", Release: "jammy", Version: today},
		{ID: 3, Name: "plucky-desktop-amd64.iso", Release: "plucky", Version: today},
	}
	out := FormatScheduledSummary(artefacts, []string{"plucky", "noble"})
	pluckyPos := strings.Index(out, "plucky")
	noblePos := strings.Index(out, "noble")
	if pluckyPos < 0 || noblePos < 0 {
		t.Fatalf("expected both releases in output, got:\n%s", out)
	}
	if pluckyPos > noblePos {
		t.Errorf("plucky should appear before noble (env order), got:\n%s", out)
	}
	if strings.Contains(out, "jammy") {
		t.Errorf("jammy should be absent when not in releases list, got:\n%s", out)
	}
}

func TestFormatScheduledSummary_NilReleasesUsesAll(t *testing.T) {
	artefacts := []domain.Artefact{
		{ID: 1, Name: "noble-desktop-amd64.iso", Release: "noble", Version: today},
		{ID: 2, Name: "jammy-desktop-amd64.iso", Release: "jammy", Version: yesterday},
	}
	out := FormatScheduledSummary(artefacts, nil)
	if !strings.Contains(out, "noble") {
		t.Errorf("expected 'noble' in output when releases=nil, got:\n%s", out)
	}
	if !strings.Contains(out, "jammy") {
		t.Errorf("expected 'jammy' in output when releases=nil, got:\n%s", out)
	}
}

func TestFormatScheduledSummary_UnknownReleaseSkipped(t *testing.T) {
	artefacts := []domain.Artefact{
		{ID: 1, Name: "noble-desktop-amd64.iso", Release: "noble", Version: today},
	}
	out := FormatScheduledSummary(artefacts, []string{"noble", "nonexistent"})
	if strings.Contains(out, "nonexistent") {
		t.Errorf("release not in snapshot should be silently skipped, got:\n%s", out)
	}
	if !strings.Contains(out, "noble") {
		t.Errorf("expected 'noble' in output, got:\n%s", out)
	}
}

func TestFormatScheduledSummary_ZeroPct(t *testing.T) {
	artefacts := []domain.Artefact{
		{
			ID: 1, Name: "noble-desktop-amd64.iso", OS: "ubuntu", Release: "noble",
			Version:                 yesterday,
			BuildLog:                domain.BuildStatusFailed,
			BuildFailureKind:        domain.BuildFailureKindInfra,
			BuildFailureDescription: "cdimage crashed mid-run, build was orphaned",
		},
		{
			ID: 2, Name: "noble-server-amd64.iso", OS: "ubuntu-server", Release: "noble",
			Version:                 yesterday,
			BuildLog:                domain.BuildStatusFailed,
			BuildFailureKind:        domain.BuildFailureKindInfra,
			BuildFailureDescription: "cdimage crashed mid-run, build was orphaned",
		},
	}
	out := FormatScheduledSummary(artefacts, []string{"noble"})
	if !strings.Contains(out, "0/2") {
		t.Errorf("expected 0/2 built, got:\n%s", out)
	}
	// 0% → tornado emoji.
	if !strings.Contains(out, "🌪️") {
		t.Errorf("expected tornado emoji for 0%%, got:\n%s", out)
	}
	if !strings.Contains(out, "Infra (2):") {
		t.Errorf("expected Infra section with count, got:\n%s", out)
	}
}

// --- buildWeatherEmoji ---

func TestBuildWeatherEmoji(t *testing.T) {
	cases := []struct {
		built, total int
		want         string
	}{
		{4, 4, "☀️"}, // 100% → sunny
		{3, 4, "🌤️"}, // 75% → partly cloudy
		{4, 5, "🌤️"}, // 80% → partly cloudy
		{2, 4, "⛅"},  // 50% → cloudy
		{3, 5, "⛅"},  // 60% → cloudy
		{1, 4, "🌧️"}, // 25% → rainy
		{2, 5, "🌧️"}, // 40% → rainy (integer div: 2*100/5=40)
		{1, 5, "⛈️"}, // 20% → stormy
		{0, 4, "🌪️"}, // 0% → tornado
		{0, 0, "🌪️"}, // zero total → tornado
	}
	for _, tc := range cases {
		got := buildWeatherEmoji(tc.built, tc.total)
		if got != tc.want {
			t.Errorf("buildWeatherEmoji(%d, %d) = %q, want %q", tc.built, tc.total, got, tc.want)
		}
	}
}

// --- archFromName ---

func TestArchFromName(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"noble-desktop-amd64.iso", "amd64"},
		{"noble-desktop-arm64.iso", "arm64"},
		{"noble-desktop-riscv64.iso", "riscv64"},
		{"noble-server-ppc64el.iso", "ppc64el"},
		{"noble-server-s390x.iso", "s390x"},
		{"noble-server-armhf.iso", "armhf"},
		{"noble-server-i386.iso", "i386"},
		{"noble-server-unknown-arch.iso", "unknown"},
	}
	for _, tc := range cases {
		got := archFromName(tc.name)
		if got != tc.want {
			t.Errorf("archFromName(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}
