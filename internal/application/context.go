package application

import (
	"encoding/json"
	"strings"

	"github.com/mclemenceau/watchtower/internal/domain"
)

// contextArtefact is a compact projection of domain.Artefact for LLM context.
// We omit heavy fields (Builds, ImageURL) to keep the token count low.
type contextArtefact struct {
	ID       int                     `json:"id"`
	Name     string                  `json:"name"`
	Version  string                  `json:"version"`
	OS       string                  `json:"os"`
	Release  string                  `json:"release"`
	BuildLog domain.BuildStatusState `json:"build_log,omitempty"`
}

// contextFailure is a compact projection of domain.FailureRecord for LLM context.
type contextFailure struct {
	ArtefactID   int    `json:"artefact_id"`
	ArtefactName string `json:"artefact_name"`
	Release      string `json:"release"`
	Product      string `json:"product"`
	FirstSeen    string `json:"first_seen_version"`
	LastSeen     string `json:"last_seen_version"`
	Occurrences  int    `json:"occurrences"`
}

// contextPayload is the top-level structure serialised into contextJSON.
type contextPayload struct {
	Artefacts []contextArtefact `json:"artefacts"`
	Failures  []contextFailure  `json:"failures"`
}

// maxContextArtefacts is the hard cap on artefacts injected into the LLM prompt.
// Keeps token usage bounded even when no filter tokens are found in the message.
const maxContextArtefacts = 50

// BuildContext filters the artefact snapshot and failure store to the subset
// most likely relevant to msg, then returns a compact JSON string suitable for
// injection into an LLM prompt.
//
// Filtering strategy (keyword extraction):
//  1. Tokenise msg and match against unique release names present in the snapshot
//     (e.g. "noble", "oracular").
//  2. Match against unique product/OS names (e.g. "ubuntu-desktop", "desktop").
//  3. If any matches are found, include only artefacts for those
//     release+product combinations.
//  4. If no matches are found, fall back to all FAILED artefacts (most relevant
//     for any open-ended question) capped at maxContextArtefacts.
//  5. Always include all active failures that match the filtered releases/products.
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

	// Filter artefacts.
	filtered := filterArtefacts(artefacts, matchedReleases, matchedProducts)

	// Convert to compact projections, capped at maxContextArtefacts.
	ctxArts := make([]contextArtefact, 0, len(filtered))
	for _, a := range filtered {
		if len(ctxArts) >= maxContextArtefacts {
			break
		}
		ctxArts = append(ctxArts, contextArtefact{
			ID:       a.ID,
			Name:     a.Name,
			Version:  a.Version,
			OS:       a.OS,
			Release:  a.Release,
			BuildLog: a.BuildLog,
		})
	}

	// Collect active failures matching the same filter.
	activeFailures := failures.ActiveFailures("", "")
	ctxFails := make([]contextFailure, 0, len(activeFailures))
	for _, f := range activeFailures {
		if !failureMatchesFilter(f, matchedReleases, matchedProducts) {
			continue
		}
		ctxFails = append(ctxFails, contextFailure{
			ArtefactID:   f.ArtefactID,
			ArtefactName: f.ArtefactName,
			Release:      f.Release,
			Product:      f.Product,
			FirstSeen:    f.FirstSeenVersion,
			LastSeen:     f.LastSeenVersion,
			Occurrences:  f.Occurrences,
		})
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

// filterArtefacts returns the subset of artefacts matching the given release and
// product filter sets. When both sets are empty (no tokens found in the message),
// it falls back to returning only FAILED artefacts so the context stays relevant.
func filterArtefacts(artefacts []domain.Artefact, releases, products map[string]struct{}) []domain.Artefact {
	noFilter := len(releases) == 0 && len(products) == 0

	var out []domain.Artefact
	for _, a := range artefacts {
		if noFilter {
			// Fallback: only include failed artefacts to keep context tight.
			if a.BuildLog == domain.BuildStatusFailed {
				out = append(out, a)
			}
			continue
		}
		releaseMatch := len(releases) == 0
		if !releaseMatch {
			_, releaseMatch = releases[strings.ToLower(a.Release)]
		}
		productMatch := len(products) == 0
		if !productMatch {
			// Product matching: check both exact OS name and substring
			// (e.g. "desktop" matches "ubuntu-desktop").
			aOS := strings.ToLower(a.OS)
			for p := range products {
				if strings.Contains(aOS, p) || strings.Contains(p, aOS) {
					productMatch = true
					break
				}
			}
		}
		if releaseMatch && productMatch {
			out = append(out, a)
		}
	}
	return out
}

// failureMatchesFilter reports whether a FailureRecord should be included given
// the matched release and product sets. When both sets are empty, all failures
// are included (the caller already handles the fallback for artefacts).
func failureMatchesFilter(f domain.FailureRecord, releases, products map[string]struct{}) bool {
	if len(releases) == 0 && len(products) == 0 {
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
	return releaseMatch && productMatch
}
