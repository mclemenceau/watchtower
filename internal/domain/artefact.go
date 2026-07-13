// Package domain contains the core types and pure business logic for Watchtower.
// It has zero dependencies on other internal packages; only stdlib is imported.
package domain

import (
	"fmt"
	"strings"
	"time"
)

const baseLogURL = "https://ubuntu-archive-team.ubuntu.com/cd-build-logs"

// Artefact mirrors the Test Observer API ArtefactResponse for the image family.
// Only fields used by Watchtower are included; extra API fields are silently discarded.
// Builds is populated by the cron workflow and cached in the snapshot; it is
// omitted from JSON when empty so existing snapshot files remain compatible.
type Artefact struct {
	ID       int             `json:"id"`
	Name     string          `json:"name"`
	Version  string          `json:"version"` // YYYYMMDD or YYYYMMDD.N (respin); today's date means build succeeded and image is available for testing
	OS       string          `json:"os"`
	Release  string          `json:"release"`
	Stage    string          `json:"stage"`  // pending | current — pipeline release stage, not build state
	Status   string          `json:"status"` // APPROVED | MARKED_AS_FAILED | UNDECIDED — test review state, unrelated to build availability
	Archived bool            `json:"archived"`
	ImageURL string          `json:"image_url"`
	Builds   []ArtefactBuild `json:"builds,omitempty"` // cached from /v1/artefacts/{id}/builds
}

// ArtefactBuild represents one architecture-specific build of an artefact,
// with its associated test executions.
type ArtefactBuild struct {
	ID             int             `json:"id"`
	Architecture   string          `json:"architecture"`
	Revision       *int            `json:"revision,omitempty"`
	TestExecutions []TestExecution `json:"test_executions"`
}

// TestExecution represents a single test run within an ArtefactBuild.
type TestExecution struct {
	ID          int         `json:"id"`
	CILink      string      `json:"ci_link"`
	Status      string      `json:"status"`
	TestPlan    string      `json:"test_plan"`
	Environment Environment `json:"environment"`
	CreatedAt   string      `json:"created_at"`
}

// Environment describes the machine or runner that executed the tests.
type Environment struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Architecture string `json:"architecture"`
}

// ChangeReport categorises status changes between two snapshots.
type ChangeReport struct {
	NewFailures  []ArtefactDelta `json:"new_failures"`
	Recoveries   []ArtefactDelta `json:"recoveries"`
	OtherChanges []ArtefactDelta `json:"other_changes"`
	NewArtefacts []Artefact      `json:"new_artefacts"`
}

// ArtefactDelta represents one artefact's status transition between snapshots.
type ArtefactDelta struct {
	Name      string `json:"name"`
	Release   string `json:"release"`
	Version   string `json:"version"`
	OldStatus string `json:"old_status"`
	NewStatus string `json:"new_status"`
}

// LogAnalysis is the structured output from an LLM log analysis.
type LogAnalysis struct {
	Category    string   `json:"category"` // infra|code|dependency|flaky|unknown
	Hypothesis  string   `json:"hypothesis"`
	LogExcerpts []string `json:"log_excerpts"`
	NextAction  string   `json:"next_action"`
}

// IsBuiltToday returns true if the version's base date (YYYYMMDD) matches today in UTC.
func IsBuiltToday(version string) bool {
	base := version
	if i := strings.IndexByte(version, '.'); i != -1 {
		base = version[:i]
	}
	return base == time.Now().UTC().Format("20060102")
}

// BuildStatus returns an emoji reflecting whether the image was built today.
// ✅ means built today; ❌ means not built. Log URL is handled separately via LogURLFromImageURL.
func BuildStatus(version string) string {
	if IsBuiltToday(version) {
		return "✅"
	}
	return "❌"
}

// LogCell returns a Markdown 🔗 hyperlink to today's build log when imageURL is a
// recognised cdimage.ubuntu.com URL, or ❌ when no log URL can be derived.
// The URL always uses today's UTC date so that the link reflects the current day's
// build attempt rather than the last known working build.
func LogCell(imageURL string) string {
	today := time.Now().UTC().Format("20060102")
	if logURL := LogURLFromImageURLForDate(imageURL, today); logURL != "" {
		return fmt.Sprintf("[🔗](%s)", logURL)
	}
	return "❌"
}

// LogURLFromImageURL derives the cd-build-log URL from a cdimage.ubuntu.com image URL,
// using the date embedded in that URL.
//
// The image URL is expected to follow the pattern:
//
//	https://cdimage.ubuntu.com/{folder}/{release}/{log_prefix}/{date}/{filename}
//
// The returned log URL follows:
//
//	https://ubuntu-archive-team.ubuntu.com/cd-build-logs/{folder}/{release}/{log_prefix}-{date}.log
//
// Returns "" if imageURL is empty, the host is not cdimage.ubuntu.com, or the path
// does not contain the required number of segments.
func LogURLFromImageURL(imageURL string) string {
	folder, release, logPrefix, ok := parseImageURLParts(imageURL)
	if !ok {
		return ""
	}
	// Extract and validate the date from the URL itself (parts[4]).
	rest := imageURL
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(rest, prefix) {
			rest = rest[len(prefix):]
			break
		}
	}
	parts := strings.SplitN(rest, "/", 8)
	date := parts[4]
	if i := strings.IndexByte(date, '.'); i != -1 {
		date = date[:i]
	}
	if len(date) != 8 {
		return ""
	}
	for _, c := range date {
		if c < '0' || c > '9' {
			return ""
		}
	}
	return fmt.Sprintf("%s/%s/%s/%s-%s.log", baseLogURL, folder, release, logPrefix, date)
}

// LogURLFromImageURLForDate derives the cd-build-log URL from a cdimage.ubuntu.com image
// URL but substitutes the provided date (YYYYMMDD) in place of the date embedded in the
// image URL. This is used to construct a log link for a specific date — typically today —
// regardless of which build date the Test Observer API last reported.
//
// Returns "" if imageURL cannot be parsed (same conditions as LogURLFromImageURL).
func LogURLFromImageURLForDate(imageURL, date string) string {
	folder, release, logPrefix, ok := parseImageURLParts(imageURL)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s/%s-%s.log", baseLogURL, folder, release, logPrefix, date)
}

// parseImageURLParts extracts the (folder, release, logPrefix) structural components
// from a cdimage.ubuntu.com image URL. Returns ok=false if the URL is empty, has the
// wrong host, or has too few path segments.
func parseImageURLParts(imageURL string) (folder, release, logPrefix string, ok bool) {
	if imageURL == "" {
		return "", "", "", false
	}
	rest := imageURL
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(rest, prefix) {
			rest = rest[len(prefix):]
			break
		}
	}
	// Expected: host / folder / release / log_prefix / date / filename
	parts := strings.SplitN(rest, "/", 8)
	if len(parts) < 6 {
		return "", "", "", false
	}
	if parts[0] != "cdimage.ubuntu.com" {
		return "", "", "", false
	}
	return parts[1], parts[2], parts[3], true
}

// ImageAge returns a human-readable age string for a YYYYMMDD or YYYYMMDD.N version field.
func ImageAge(version string) string {
	if i := strings.IndexByte(version, '.'); i != -1 {
		version = version[:i]
	}
	if len(version) != 8 {
		return "unknown"
	}
	t, err := time.Parse("20060102", version)
	if err != nil {
		return "unknown"
	}
	days := int(time.Since(t).Hours() / 24)
	switch {
	case days <= 0:
		return "today"
	case days == 1:
		return "1 day"
	case days < 14:
		return fmt.Sprintf("%d days", days)
	case days < 60:
		weeks := days / 7
		if weeks == 1 {
			return "1 week"
		}
		return fmt.Sprintf("%d weeks", weeks)
	default:
		months := days / 30
		if months == 1 {
			return "1 month"
		}
		return fmt.Sprintf("%d months", months)
	}
}

// IsDisplayable returns true when the execution represents a real test result
// worth surfacing to the user. Filtering rules:
//   - "Image build" is always skipped — it is a build availability notification,
//     not a test result.
//   - "Manual Testing" with status IN_PROGRESS is skipped — it means no tester
//     has submitted results yet (placeholder execution with no test_results).
func IsDisplayable(te TestExecution) bool {
	if te.TestPlan == "Image build" {
		return false
	}
	if te.TestPlan == "Manual Testing" && te.Status == "IN_PROGRESS" {
		return false
	}
	return true
}

// ExecStatusEmoji returns an emoji for a TestExecution status string.
func ExecStatusEmoji(status string) string {
	switch status {
	case "PASSED":
		return "✅"
	case "FAILED":
		return "❌"
	case "IN_PROGRESS":
		return "🔄"
	case "NOT_STARTED":
		return "⏳"
	default:
		return "⚠️"
	}
}

// ParseLaunchpadBuildURLs scans a cd-build-log for lines of the form:
//
//	ubuntu-{label}: https://launchpad.net/~ubuntu-cdimage/+livefs/...
//
// and returns a map of label → Launchpad build page URL. The label is the
// portion of the build name after "ubuntu-" (e.g. "amd64" for standard builds,
// "desktop-preinstalled-arm64-raspi" for variant builds). Callers that need to
// match by architecture should use substring matching against these labels.
// Lines that do not match the pattern are silently ignored.
func ParseLaunchpadBuildURLs(cdBuildLog string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(cdBuildLog, "\n") {
		line = strings.TrimSpace(line)
		// Expected format: "ubuntu-{arch}: https://launchpad.net/..."
		if !strings.HasPrefix(line, "ubuntu-") {
			continue
		}
		colon := strings.Index(line, ": ")
		if colon < 0 {
			continue
		}
		arch := line[len("ubuntu-"):colon]
		url := strings.TrimSpace(line[colon+2:])
		if !strings.HasPrefix(url, "https://launchpad.net/") {
			continue
		}
		if !strings.Contains(url, "/+build/") {
			continue
		}
		result[arch] = url
	}
	return result
}

// PrimaryBuildArch returns the architecture to investigate from a slice of
// ArtefactBuilds. Preference order: amd64 > arm64 > first alphabetically.
// Returns "" when builds is empty.
func PrimaryBuildArch(builds []ArtefactBuild) string {
	if len(builds) == 0 {
		return ""
	}
	for _, pref := range []string{"amd64", "arm64"} {
		for _, b := range builds {
			if b.Architecture == pref {
				return pref
			}
		}
	}
	// Fall back to alphabetically first.
	first := builds[0].Architecture
	for _, b := range builds[1:] {
		if b.Architecture < first {
			first = b.Architecture
		}
	}
	return first
}
