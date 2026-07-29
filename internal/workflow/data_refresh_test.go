package workflow

import (
	"testing"

	"github.com/mclemenceau/watchtower/internal/domain"
)

// artefact is a helper that builds a minimal domain.Artefact for tests.
func artefact(id int, kind domain.BuildFailureKind) domain.Artefact {
	return domain.Artefact{
		ID:               id,
		BuildLog:         domain.BuildStatusFailed,
		BuildFailureKind: kind,
	}
}

func TestHasNewProductFailures_EmptyReport(t *testing.T) {
	report := domain.ChangeReport{}
	if hasNewProductFailures(report, nil) {
		t.Error("expected false for empty report")
	}
}

func TestHasNewProductFailures_InfraOnly(t *testing.T) {
	art := artefact(1, domain.BuildFailureKindInfra)
	report := domain.ChangeReport{
		NewFailures: []domain.ArtefactDelta{{ArtefactID: 1}},
	}
	if hasNewProductFailures(report, []domain.Artefact{art}) {
		t.Error("expected false for INFRA-only failures")
	}
}

func TestHasNewProductFailures_ProductInNewFailures(t *testing.T) {
	art := artefact(1, domain.BuildFailureKindProduct)
	report := domain.ChangeReport{
		NewFailures: []domain.ArtefactDelta{{ArtefactID: 1}},
	}
	if !hasNewProductFailures(report, []domain.Artefact{art}) {
		t.Error("expected true for PRODUCT failure in NewFailures")
	}
}

func TestHasNewProductFailures_UnknownInNewFailures(t *testing.T) {
	art := artefact(1, domain.BuildFailureKindUnknown)
	report := domain.ChangeReport{
		NewFailures: []domain.ArtefactDelta{{ArtefactID: 1}},
	}
	if !hasNewProductFailures(report, []domain.Artefact{art}) {
		t.Error("expected true for UNKNOWN failure in NewFailures")
	}
}

// TestHasNewProductFailures_FirstBootNewArtefacts is the regression test for
// the first-boot bug: on initial startup all artefacts land in NewArtefacts
// (no prior snapshot to diff against), so NewFailures is empty. Without the
// NewArtefacts check, AnalyseFailures was never triggered on first boot.
func TestHasNewProductFailures_FirstBootNewArtefacts(t *testing.T) {
	infra := artefact(1, domain.BuildFailureKindInfra)
	product := artefact(2, domain.BuildFailureKindProduct)

	// NewFailures is empty — exactly the first-boot scenario.
	report := domain.ChangeReport{
		NewArtefacts: []domain.Artefact{infra, product},
	}

	if !hasNewProductFailures(report, []domain.Artefact{infra, product}) {
		t.Error("expected true: PRODUCT artefact in NewArtefacts should trigger analysis")
	}
}

func TestHasNewProductFailures_FirstBootInfraOnly(t *testing.T) {
	art := artefact(1, domain.BuildFailureKindInfra)
	report := domain.ChangeReport{
		NewArtefacts: []domain.Artefact{art},
	}
	if hasNewProductFailures(report, []domain.Artefact{art}) {
		t.Error("expected false: INFRA-only NewArtefacts should not trigger analysis")
	}
}

func TestHasNewProductFailures_FirstBootBuiltArtefact(t *testing.T) {
	// A PRODUCT kind but BuildLog is BUILT — not a failure, should not trigger.
	art := domain.Artefact{
		ID:               1,
		BuildLog:         domain.BuildStatusBuilt,
		BuildFailureKind: domain.BuildFailureKindProduct,
	}
	report := domain.ChangeReport{
		NewArtefacts: []domain.Artefact{art},
	}
	if hasNewProductFailures(report, []domain.Artefact{art}) {
		t.Error("expected false: BUILT artefact in NewArtefacts should not trigger analysis")
	}
}
