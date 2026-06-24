package mattermost

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mclemenceau/argus/internal/buildapi"
	"github.com/mclemenceau/argus/internal/state"
)

// Dispatch routes an incoming message to the appropriate handler and sends the
// reply via hook. artefacts is the current snapshot (may be nil/empty on first boot).
func Dispatch(msg string, artefacts []buildapi.Artefact, defaultRelease string, hook WebhookClient) error {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return nil
	}

	lower := strings.ToLower(msg)

	switch {
	case lower == "help":
		return hook.Send(helpText())

	case lower == "status":
		return handleStatus(artefacts, defaultRelease, hook)

	case strings.HasPrefix(lower, "builds"):
		parts := strings.Fields(msg)
		if len(parts) < 2 {
			return hook.Send("Usage: `builds <release>` — e.g. `builds noble`")
		}
		return handleBuilds(artefacts, parts[1], hook)

	case lower == "releases":
		return handleReleases(artefacts, hook)

	default:
		return hook.Send(fmt.Sprintf("I didn't understand `%s`. Type `help` for available commands.", msg))
	}
}

// handleStatus renders the full status table for the current/default release.
func handleStatus(artefacts []buildapi.Artefact, defaultRelease string, hook WebhookClient) error {
	if len(artefacts) == 0 {
		return hook.Send("No snapshot available yet — the first fetch is still in progress.")
	}

	release := defaultRelease
	if release == "" {
		release = state.LatestRelease(artefacts)
	}

	var filtered []buildapi.Artefact
	for _, art := range artefacts {
		if art.Release == release {
			filtered = append(filtered, art)
		}
	}

	if len(filtered) == 0 {
		return hook.Send(fmt.Sprintf("No artefacts found for release **%s**.", release))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "**Build Status — %s** · %s\n\n",
		release, time.Now().UTC().Format("2006-01-02 15:04 UTC"))
	sb.WriteString("| Name | Product | Release | Age | Status |\n")
	sb.WriteString("|------|---------|---------|-----|--------|\n")
	for _, art := range filtered {
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s |\n",
			art.Name, art.OS, art.Release, imageAge(art.Version), statusEmoji(art.Status))
	}
	return hook.Send(sb.String())
}

// handleBuilds lists all builds for a specific release.
func handleBuilds(artefacts []buildapi.Artefact, release string, hook WebhookClient) error {
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
		return hook.Send(fmt.Sprintf("No builds found for release **%s**.", release))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "**Builds for %s** (%d artefacts)\n\n", release, len(filtered))
	sb.WriteString("| Name | Product | Version | Age | Status |\n")
	sb.WriteString("|------|---------|---------|-----|--------|\n")
	for _, art := range filtered {
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s |\n",
			art.Name, art.OS, art.Version, imageAge(art.Version), statusEmoji(art.Status))
	}
	return hook.Send(sb.String())
}

// handleReleases lists all known releases in the current snapshot.
func handleReleases(artefacts []buildapi.Artefact, hook WebhookClient) error {
	if len(artefacts) == 0 {
		return hook.Send("No snapshot available yet — the first fetch is still in progress.")
	}

	counts := make(map[string]int)
	for _, art := range artefacts {
		counts[art.Release]++
	}

	// Sort releases alphabetically for deterministic output.
	releases := make([]string, 0, len(counts))
	for r := range counts {
		releases = append(releases, r)
	}
	sort.Strings(releases)

	var sb strings.Builder
	sb.WriteString("**Known releases:**\n\n")
	for _, r := range releases {
		sb.WriteString(fmt.Sprintf("- **%s** (%d artefacts)\n", r, counts[r]))
	}
	return hook.Send(sb.String())
}

func helpText() string {
	return `**ARGUS — available commands:**

| Command | Description |
|---------|-------------|
| ` + "`status`" + `           | Current build status table for the latest release |
| ` + "`builds <release>`" + ` | All builds for a specific release (e.g. ` + "`builds noble`" + `) |
| ` + "`releases`" + `         | List all known releases |
| ` + "`help`" + `             | Show this message |

Proactive change reports are posted automatically when build statuses change.`
}

func statusEmoji(status string) string {
	switch status {
	case "APPROVED":
		return "✅ approved"
	case "MARKED_AS_FAILED":
		return "❌ failed"
	default:
		return "⏳ pending"
	}
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
