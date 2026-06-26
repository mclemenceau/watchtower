package buildapi

import (
	"fmt"
	"strings"
	"time"
)

const baseLogURL = "https://ubuntu-archive-team.ubuntu.com/cd-build-logs"

// Artefact mirrors the Test Observer API ArtefactResponse for the image family.
// Only fields used by ARGUS are included; extra API fields are silently discarded.
type Artefact struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Version  string `json:"version"` // YYYYMMDD or YYYYMMDD.N (respin); today's date means build succeeded and image is available for testing
	OS       string `json:"os"`
	Release  string `json:"release"`
	Stage    string `json:"stage"`  // pending | current — pipeline release stage, not build state
	Status   string `json:"status"` // APPROVED | MARKED_AS_FAILED | UNDECIDED — test review state, unrelated to build availability
	Archived bool   `json:"archived"`
	ImageURL string `json:"image_url"`
}

type ChangeReport struct {
	NewFailures  []ArtefactDelta `json:"new_failures"`
	Recoveries   []ArtefactDelta `json:"recoveries"`
	OtherChanges []ArtefactDelta `json:"other_changes"`
	NewArtefacts []Artefact      `json:"new_artefacts"`
}

type ArtefactDelta struct {
	Name      string `json:"name"`
	Release   string `json:"release"`
	Version   string `json:"version"`
	OldStatus string `json:"old_status"`
	NewStatus string `json:"new_status"`
}

// IsBuiltToday returns true if the version's base date (YYYYMMDD) matches today in UTC.
func IsBuiltToday(version string) bool {
	base := version
	if i := strings.IndexByte(version, '.'); i != -1 {
		base = version[:i]
	}
	return base == time.Now().UTC().Format("20060102")
}

// BuildStatus returns a display string reflecting whether the image was built today.
// When the image was not built today and imageURL is a recognised cdimage.ubuntu.com
// URL, the "not built" label is wrapped in a Markdown hyperlink to the build log.
func BuildStatus(version, imageURL string) string {
	if IsBuiltToday(version) {
		return "✅ built"
	}
	if logURL := LogURLFromImageURL(imageURL); logURL != "" {
		return fmt.Sprintf("❌ [not built](%s)", logURL)
	}
	return "❌ not built"
}

// LogURLFromImageURL derives the cd-build-log URL from a cdimage.ubuntu.com image URL.
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
	if imageURL == "" {
		return ""
	}
	// Strip scheme ("https://") and split on "/"
	// Expected: ["", "", "cdimage.ubuntu.com", folder, release, log_prefix, date, filename]
	rest := imageURL
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(rest, prefix) {
			rest = rest[len(prefix):]
			break
		}
	}
	parts := strings.SplitN(rest, "/", 8)
	// parts[0]=host, parts[1]=folder, parts[2]=release, parts[3]=log_prefix, parts[4]=date, parts[5]=filename
	if len(parts) < 6 {
		return ""
	}
	host := parts[0]
	if host != "cdimage.ubuntu.com" {
		return ""
	}
	folder := parts[1]
	release := parts[2]
	logPrefix := parts[3]
	date := parts[4]
	// date must be a valid YYYYMMDD (8 digits); ignore respin suffix on the date segment
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
