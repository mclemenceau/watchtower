package application

import (
	"encoding/json"
	"strings"

	"github.com/mclemenceau/watchtower/internal/domain"
)

// contextArtefact is a compact projection of domain.Artefact for LLM context.
// We omit heavy fields (Builds, ImageURL) to keep the token count low.
type contextArtefact struct {
	ID                      int                     `json:"id"`
	Name                    string                  `json:"name"`
	Version                 string                  `json:"version"`
	OS                      string                  `json:"os"`
	Release                 string                  `json:"release"`
	BuildLog                domain.BuildStatusState `json:"build_log,omitempty"`
	BuildFailureKind        domain.BuildFailureKind `json:"build_failure_kind,omitempty"`
	BuildFailureDescription string                  `json:"build_failure_description,omitempty"`
}

// contextFailure is a compact projection of domain.FailureRecord for LLM context.
type contextFailure struct {
	ArtefactID         int                     `json:"artefact_id"`
	ArtefactName       string                  `json:"artefact_name"`
	Release            string                  `json:"release"`
	Product            string                  `json:"product"`
	FirstSeen          string                  `json:"first_seen_version"`
	LastSeen           string                  `json:"last_seen_version"`
	Occurrences        int                     `json:"occurrences"`
	FailureKind        domain.BuildFailureKind `json:"failure_kind,omitempty"`
	FailureDescription string                  `json:"failure_description,omitempty"`
	AnalysisCategory   string                  `json:"analysis_category,omitempty"`
	AnalysisHypothesis string                  `json:"analysis_hypothesis,omitempty"`
	AnalysisNextAction string                  `json:"analysis_next_action,omitempty"`
}

// contextPayload is the top-level structure serialised into contextJSON.
type contextPayload struct {
	Artefacts []contextArtefact `json:"artefacts"`
	Failures  []contextFailure  `json:"failures"`
}

// maxContextArtefacts is the hard cap on artefacts injected into the LLM prompt.
// Keeps token usage bounded even when no filter tokens are found in the message.
const maxContextArtefacts = 50

// semanticFailureWords are message words that indicate the user is asking about
// failures. When present, non-failing artefacts are stripped from the context
// even if a release or product filter matched them.
var semanticFailureWords = []string{
	"fail", "failed", "failing", "failure", "failures",
	"broken", "break", "broke",
	"issue", "issues",
	"problem", "problems",
	"error", "errors",
	"wrong",
	"infra", "infrastructure",
}

// semanticBuildKindMap maps message words to the BuildFailureKind they imply.
// When a key is present in the message, the context is narrowed to artefacts
// with the corresponding failure kind (in addition to any release/product filter).
var semanticBuildKindMap = map[string]domain.BuildFailureKind{
	"infra":          domain.BuildFailureKindInfra,
	"infrastructure": domain.BuildFailureKindInfra,
	"product":        domain.BuildFailureKindProduct,
}

// BuildContext filters the artefact snapshot and failure store to the subset
// most likely relevant to msg, then returns a compact JSON string suitable for
// injection into an LLM prompt.
//
// Filtering strategy:
//  1. Tokenise msg and match against unique release names in the snapshot.
//  2. Match against unique product/OS names in the snapshot.
//  3. Detect semantic failure keywords (fail, broken, issue, infra, …).
//  4. Detect BuildFailureKind keywords (infra → INFRA, product → PRODUCT).
//  5. If release/product matches found: include those artefacts; if failure
//     semantics also detected, narrow further to FAILED-only (and optionally
//     to the matched kind).
//  6. If no release/product match: fall back to all FAILED artefacts (most
//     relevant for open-ended questions), capped at maxContextArtefacts.
//  7. Always include active failures that match the same filters.
//
// Returns "" on marshal error or when both slices are empty.
func BuildContext(msg string, artefacts []domain.Artefact, failures domain.FailureStore) string {
	msgLower := strings.ToLower(msg)

	// Collect unique releases and products present in the snapshot.
	releaseSet := make(map[string]struct{})
	productSet := make(map[string]struct{})
	for _, a := range artefacts {
		if a.Release != "" {
			releaseSet[strings.ToLower(a.Release)] = struct{}{}
		}
		if a.OS != "" {
			productSet[strings.ToLower(a.OS)] = struct{}{}
		}
	}

	// Extract which releases and products the message mentions.
	matchedReleases := extractTokens(msgLower, releaseSet)
	matchedProducts := extractTokens(msgLower, productSet)

	// Detect semantic signals: is the user asking about failures? about a kind?
	failureSemantics := containsAny(msgLower, semanticFailureWords)
	matchedKind := extractBuildKind(msgLower)

	// Filter artefacts using all signals.
	filtered := filterArtefacts(artefacts, matchedReleases, matchedProducts, failureSemantics, matchedKind)

	// Convert to compact projections, capped at maxContextArtefacts.
	ctxArts := make([]contextArtefact, 0, len(filtered))
	for _, a := range filtered {
		if len(ctxArts) >= maxContextArtefacts {
			break
		}
		ctxArts = append(ctxArts, contextArtefact{
			ID:                      a.ID,
			Name:                    a.Name,
			Version:                 a.Version,
			OS:                      a.OS,
			Release:                 a.Release,
			BuildLog:                a.BuildLog,
			BuildFailureKind:        a.BuildFailureKind,
			BuildFailureDescription: a.BuildFailureDescription,
		})
	}

	// Collect active failures matching the same filter.
	activeFailures := failures.ActiveFailures("", "")
	ctxFails := make([]contextFailure, 0, len(activeFailures))
	for _, f := range activeFailures {
		if !failureMatchesFilter(f, matchedReleases, matchedProducts, matchedKind) {
			continue
		}
		cf := contextFailure{
			ArtefactID:         f.ArtefactID,
			ArtefactName:       f.ArtefactName,
			Release:            f.Release,
			Product:            f.Product,
			FirstSeen:          f.FirstSeenVersion,
			LastSeen:           f.LastSeenVersion,
			Occurrences:        f.Occurrences,
			FailureKind:        f.FailureKind,
			FailureDescription: f.FailureDescription,
		}
		if f.Analysis != nil {
			cf.AnalysisCategory = f.Analysis.Category
			cf.AnalysisHypothesis = f.Analysis.Hypothesis
			cf.AnalysisNextAction = f.Analysis.NextAction
		}
		ctxFails = append(ctxFails, cf)
	}

	if len(ctxArts) == 0 && len(ctxFails) == 0 {
		return ""
	}

	payload := contextPayload{
		Artefacts: ctxArts,
		Failures:  ctxFails,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(b)
}

// containsAny returns true if msgLower contains any of the given words as a
// whole word or as a prefix of a word (e.g. "fail" matches "failing").
func containsAny(msgLower string, words []string) bool {
	for _, w := range words {
		if strings.Contains(msgLower, w) {
			return true
		}
	}
	return false
}

// extractBuildKind returns the BuildFailureKind implied by semantic keywords in
// msgLower, or BuildFailureKindNone if no kind-specific keyword is found.
func extractBuildKind(msgLower string) domain.BuildFailureKind {
	for word, kind := range semanticBuildKindMap {
		if strings.Contains(msgLower, word) {
			return kind
		}
	}
	return domain.BuildFailureKindNone
}

// extractTokens returns the subset of keys in tokenSet that are mentioned in the
// lowercased message. A token is considered mentioned when:
//   - the message contains the full token as a substring (e.g. "ubuntu-desktop" in msg), OR
//   - any whitespace-separated word in the message is a substring of the token
//     (e.g. "desktop" in msg matches token "ubuntu-desktop").
//
// tokenSet keys must already be lowercase.
func extractTokens(msgLower string, tokenSet map[string]struct{}) map[string]struct{} {
	// Split message into words for partial matching.
	words := strings.Fields(msgLower)

	matched := make(map[string]struct{})
	for token := range tokenSet {
		// Full token appears in the message.
		if strings.Contains(msgLower, token) {
			matched[token] = struct{}{}
			continue
		}
		// Any message word is a component of the token (e.g. "desktop" → "ubuntu-desktop").
		for _, w := range words {
			if len(w) >= 3 && strings.Contains(token, w) {
				matched[token] = struct{}{}
				break
			}
		}
	}
	return matched
}

// filterArtefacts returns the subset of artefacts matching the given release,
// product, failure-semantics, and kind filters.
//
// When both release and product sets are empty (no tokens found in the message),
// it falls back to returning only FAILED artefacts so the context stays relevant.
//
// When failureSemantics is true and a release/product filter matched, non-FAILED
// artefacts are stripped — the user is clearly asking about problems, not
// about things that are working fine.
//
// When matchedKind is non-empty, only artefacts with that BuildFailureKind are
// included (implicitly requires BuildLog==FAILED since kind is only set on failures).
func filterArtefacts(
	artefacts []domain.Artefact,
	releases, products map[string]struct{},
	failureSemantics bool,
	matchedKind domain.BuildFailureKind,
) []domain.Artefact {
	noFilter := len(releases) == 0 && len(products) == 0

	var out []domain.Artefact
	for _, a := range artefacts {
		if noFilter {
			// Fallback: only include failed artefacts to keep context tight.
			// If a kind was specified, narrow further.
			if a.BuildLog != domain.BuildStatusFailed {
				continue
			}
			if matchedKind != domain.BuildFailureKindNone && a.BuildFailureKind != matchedKind {
				continue
			}
			out = append(out, a)
			continue
		}

		releaseMatch := len(releases) == 0
		if !releaseMatch {
			_, releaseMatch = releases[strings.ToLower(a.Release)]
		}
		productMatch := len(products) == 0
		if !productMatch {
			aOS := strings.ToLower(a.OS)
			for p := range products {
				if strings.Contains(aOS, p) || strings.Contains(p, aOS) {
					productMatch = true
					break
				}
			}
		}
		if !releaseMatch || !productMatch {
			continue
		}

		// Release+product matched. Apply semantic failure filter if active.
		if failureSemantics && a.BuildLog != domain.BuildStatusFailed {
			continue
		}

		// Apply kind filter if a kind keyword was detected.
		if matchedKind != domain.BuildFailureKindNone && a.BuildFailureKind != matchedKind {
			continue
		}

		out = append(out, a)
	}
	return out
}

// failureMatchesFilter reports whether a FailureRecord should be included given
// the matched release, product, and kind sets. When both release and product sets
// are empty, all failures are included. When matchedKind is non-empty, only
// failures with that kind are included.
func failureMatchesFilter(
	f domain.FailureRecord,
	releases, products map[string]struct{},
	matchedKind domain.BuildFailureKind,
) bool {
	if len(releases) == 0 && len(products) == 0 {
		// No release/product filter; still apply kind filter if present.
		if matchedKind != domain.BuildFailureKindNone && f.FailureKind != matchedKind {
			return false
		}
		return true
	}
	releaseMatch := len(releases) == 0
	if !releaseMatch {
		_, releaseMatch = releases[strings.ToLower(f.Release)]
	}
	productMatch := len(products) == 0
	if !productMatch {
		fProd := strings.ToLower(f.Product)
		for p := range products {
			if strings.Contains(fProd, p) || strings.Contains(p, fProd) {
				productMatch = true
				break
			}
		}
	}
	if !releaseMatch || !productMatch {
		return false
	}
	if matchedKind != domain.BuildFailureKindNone && f.FailureKind != matchedKind {
		return false
	}
	return true
}
