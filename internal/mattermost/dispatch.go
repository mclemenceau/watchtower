package mattermost

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mclemenceau/argus/internal/buildapi"
)

// Dispatch routes an incoming message to the appropriate handler and sends the
// reply via hook. artefacts is the current snapshot (may be nil/empty on first boot).
//
// keyword is the optional trigger prefix (e.g. "@watchtower"). When set, messages
// that do NOT start with the keyword are silently ignored, and the keyword is
// stripped before routing. Pass an empty string to disable keyword filtering
// (every message is dispatched).
func Dispatch(msg string, artefacts []buildapi.Artefact, defaultRelease string, hook WebhookClient, keyword string) error {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return nil
	}

	// Keyword filtering: if a keyword is configured, only process messages that
	// start with it (case-insensitive), then strip the prefix.
	if kw := strings.ToLower(strings.TrimSpace(keyword)); kw != "" {
		lower := strings.ToLower(msg)
		if !strings.HasPrefix(lower, kw) {
			return nil // not addressed to us
		}
		msg = strings.TrimSpace(msg[len(kw):])
		if msg == "" {
			msg = "help" // bare keyword → show help
		}
	}

	lower := strings.ToLower(msg)
	parts := strings.Fields(msg)

	switch {
	case lower == "help":
		return hook.Send(helpText())

	case lower == "builds status":
		return handleBuildsStatus(artefacts, hook)

	case strings.HasPrefix(lower, "builds status ") && len(parts) == 3:
		return handleBuildsStatusRelease(artefacts, parts[2], hook)

	case lower == "builds" || (strings.HasPrefix(lower, "builds") && len(parts) == 2):
		return hook.Send("Usage: `builds status` · `builds status <release>`")

	default:
		return hook.Send(fmt.Sprintf("I didn't understand `%s`. Type `help` for available commands.", msg))
	}
}

// handleBuildsStatus renders a summary table: one row per release with built/total counts
// and a 10-square progress bar (🟩 per 10% built, 🟥 for the rest).
func handleBuildsStatus(artefacts []buildapi.Artefact, hook WebhookClient) error {
	if len(artefacts) == 0 {
		return hook.Send("No snapshot available yet — the first fetch is still in progress.")
	}

	type releaseStat struct {
		total int
		built int
	}
	stats := make(map[string]*releaseStat)
	for _, art := range artefacts {
		s, ok := stats[art.Release]
		if !ok {
			s = &releaseStat{}
			stats[art.Release] = s
		}
		s.total++
		if isBuiltToday(art.Version) {
			s.built++
		}
	}

	releases := make([]string, 0, len(stats))
	for r := range stats {
		releases = append(releases, r)
	}
	sort.Strings(releases)

	var sb strings.Builder
	fmt.Fprintf(&sb, "**Build Status** · %s\n\n", time.Now().UTC().Format("2006-01-02 15:04 UTC"))
	sb.WriteString("| Release | Built | Total | Progress |\n")
	sb.WriteString("|---------|-------|-------|----------|\n")
	for _, r := range releases {
		s := stats[r]
		pct := 0
		if s.total > 0 {
			pct = s.built * 100 / s.total
		}
		green := pct / 10
		red := 10 - green
		bar := strings.Repeat("🟩", green) + strings.Repeat("🟥", red)
		fmt.Fprintf(&sb, "| **%s** | %d | %d | %s |\n", r, s.built, s.total, bar)
	}
	return hook.Send(sb.String())
}

// handleBuildsStatusRelease renders a detail table for a single release:
// one row per artefact with name, version, age and build status.
func handleBuildsStatusRelease(artefacts []buildapi.Artefact, release string, hook WebhookClient) error {
	if len(artefacts) == 0 {
		return hook.Send("No snapshot available yet — the first fetch is still in progress.")
	}

	var filtered []buildapi.Artefact
	for _, art := range artefacts {
		if strings.EqualFold(art.Release, release) {
			filtered = append(filtered, art)
		}
	}

	if len(filtered) == 0 {
		return hook.Send(fmt.Sprintf("No artefacts found for release **%s**.", release))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "**Build Status — %s** · %s\n\n",
		release, time.Now().UTC().Format("2006-01-02 15:04 UTC"))
	sb.WriteString("| Artefact | Product | Version | Age | Build |\n")
	sb.WriteString("|----------|---------|---------|-----|-------|\n")
	for _, art := range filtered {
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s |\n",
			art.Name, art.OS, art.Version, imageAge(art.Version), buildStatus(art.Version))
	}
	return hook.Send(sb.String())
}

func helpText() string {
	return `**ARGUS — available commands:**

| Command | Description |
|---------|-------------|
| ` + "`builds status`" + `           | Build summary for all releases with progress bar |
| ` + "`builds status <release>`" + ` | Detailed build status for a specific release |
| ` + "`help`" + `                    | Show this message |

Proactive change reports are posted automatically when build statuses change.`
}

// isBuiltToday returns true if the version's base date (YYYYMMDD) matches today in UTC.
func isBuiltToday(version string) bool {
	base := version
	if i := strings.IndexByte(version, '.'); i != -1 {
		base = version[:i]
	}
	return base == time.Now().UTC().Format("20060102")
}

// buildStatus returns a display string reflecting whether the image was built today.
func buildStatus(version string) string {
	if isBuiltToday(version) {
		return "✅ built"
	}
	return "❌ not built"
}

// imageAge returns a human-readable age string for a YYYYMMDD or YYYYMMDD.N version field.
func imageAge(version string) string {
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
