package mattermost

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mclemenceau/argus/internal/buildapi"
	"github.com/mclemenceau/argus/internal/testapi"
)

// Dispatch routes an incoming message to the appropriate handler and sends the
// reply via hook. artefacts is the current snapshot (may be nil/empty on first boot).
// Each artefact's Builds field is populated by the cron workflow and contains
// cached test execution data used by the `tests` commands.
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
		return handleBuildsStatusRelease(artefacts, parts[2], "", hook)

	case strings.HasPrefix(lower, "builds status ") && len(parts) == 4:
		return handleBuildsStatusRelease(artefacts, parts[2], parts[3], hook)

	case lower == "builds" || (strings.HasPrefix(lower, "builds") && len(parts) == 2):
		return hook.Send("Usage: `builds status` · `builds status <release>` · `builds status <release> <product>`")

	case lower == "tests status":
		return handleTestsStatus(artefacts, hook)

	case strings.HasPrefix(lower, "tests status ") && len(parts) == 3:
		return handleTestsStatusRelease(artefacts, parts[2], "", hook)

	case strings.HasPrefix(lower, "tests status ") && len(parts) == 4:
		return handleTestsStatusRelease(artefacts, parts[2], parts[3], hook)

	case lower == "tests" || (strings.HasPrefix(lower, "tests") && len(parts) == 2):
		return hook.Send("Usage: `tests status` · `tests status <release>` · `tests status <release> <product>`")

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
		if buildapi.IsBuiltToday(art.Version) {
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
// When product is non-empty only artefacts whose OS matches (case-insensitive) are shown.
func handleBuildsStatusRelease(artefacts []buildapi.Artefact, release, product string, hook WebhookClient) error {
	if len(artefacts) == 0 {
		return hook.Send("No snapshot available yet — the first fetch is still in progress.")
	}

	var filtered []buildapi.Artefact
	for _, art := range artefacts {
		if !strings.EqualFold(art.Release, release) {
			continue
		}
		if product != "" && !strings.EqualFold(art.OS, product) {
			continue
		}
		filtered = append(filtered, art)
	}

	if len(filtered) == 0 {
		if product != "" {
			return hook.Send(fmt.Sprintf("No artefacts found for release **%s** and product **%s**.", release, product))
		}
		return hook.Send(fmt.Sprintf("No artefacts found for release **%s**.", release))
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].OS != filtered[j].OS {
			return filtered[i].OS < filtered[j].OS
		}
		return filtered[i].Name < filtered[j].Name
	})

	var sb strings.Builder
	if product != "" {
		fmt.Fprintf(&sb, "**Build Status** · %s · %s · %s\n\n",
			release, product, time.Now().UTC().Format("2006-01-02 15:04 UTC"))
	} else {
		fmt.Fprintf(&sb, "**Build Status** · %s · %s\n\n",
			release, time.Now().UTC().Format("2006-01-02 15:04 UTC"))
	}
	sb.WriteString("| Artefact | Product | Version | Age | Build |\n")
	sb.WriteString("|----------|---------|---------|-----|-------|\n")
	for _, art := range filtered {
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s |\n",
			art.Name, art.OS, art.Version, buildapi.ImageAge(art.Version), buildapi.BuildStatus(art.Version, art.ImageURL))
	}
	return hook.Send(sb.String())
}

// handleTestsStatus renders a summary table of test execution status across all
// releases. Only releases that have at least one displayable test execution are
// shown. The progress bar reflects PASSED executions as a fraction of total
// displayable executions (🟩 per 10%, 🟥 for the rest).
// Test data is read from artefact.Builds, which is populated by the cron workflow.
func handleTestsStatus(artefacts []buildapi.Artefact, hook WebhookClient) error {
	if len(artefacts) == 0 {
		return hook.Send("No snapshot available yet — the first fetch is still in progress.")
	}

	type releaseStat struct {
		passed int
		total  int
	}
	stats := make(map[string]*releaseStat)

	for _, art := range artefacts {
		for _, b := range art.Builds {
			for _, te := range b.TestExecutions {
				if !testapi.IsDisplayable(te) {
					continue
				}
				s, ok := stats[art.Release]
				if !ok {
					s = &releaseStat{}
					stats[art.Release] = s
				}
				s.total++
				if te.Status == "PASSED" {
					s.passed++
				}
			}
		}
	}

	if len(stats) == 0 {
		return hook.Send("No test executions found in snapshot — test data may still be loading.")
	}

	releases := make([]string, 0, len(stats))
	for r := range stats {
		releases = append(releases, r)
	}
	sort.Strings(releases)

	var sb strings.Builder
	fmt.Fprintf(&sb, "**Test Status** · %s\n\n", time.Now().UTC().Format("2006-01-02 15:04 UTC"))
	sb.WriteString("| Release | Passed | Total | Progress |\n")
	sb.WriteString("|---------|--------|-------|----------|\n")
	for _, r := range releases {
		s := stats[r]
		pct := 0
		if s.total > 0 {
			pct = s.passed * 100 / s.total
		}
		green := pct / 10
		red := 10 - green
		bar := strings.Repeat("🟩", green) + strings.Repeat("🟥", red)
		fmt.Fprintf(&sb, "| **%s** | %d | %d | %s |\n", r, s.passed, s.total, bar)
	}
	return hook.Send(sb.String())
}

// handleTestsStatusRelease renders a detail table for a single release.
// Each row represents one displayable test execution. Multiple executions of
// the same test plan for the same artefact+build are deduplicated to the latest
// only (by CreatedAt). Artefacts with no displayable executions are omitted.
// When product is non-empty, only artefacts whose OS matches are shown.
func handleTestsStatusRelease(artefacts []buildapi.Artefact, release, product string, hook WebhookClient) error {
	if len(artefacts) == 0 {
		return hook.Send("No snapshot available yet — the first fetch is still in progress.")
	}

	// Filter artefacts to the requested release (and optional product).
	var filtered []buildapi.Artefact
	for _, art := range artefacts {
		if !strings.EqualFold(art.Release, release) {
			continue
		}
		if product != "" && !strings.EqualFold(art.OS, product) {
			continue
		}
		filtered = append(filtered, art)
	}

	if len(filtered) == 0 {
		if product != "" {
			return hook.Send(fmt.Sprintf("No artefacts found for release **%s** and product **%s**.", release, product))
		}
		return hook.Send(fmt.Sprintf("No artefacts found for release **%s**.", release))
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].OS != filtered[j].OS {
			return filtered[i].OS < filtered[j].OS
		}
		return filtered[i].Name < filtered[j].Name
	})

	// execRow is one displayable row in the output table.
	type execRow struct {
		artefactName string
		product      string
		arch         string
		testPlan     string
		status       string
		ciLink       string
	}

	var rows []execRow
	for _, art := range filtered {
		for _, b := range art.Builds {
			// Deduplicate by test plan, keeping the latest execution per plan.
			latest := make(map[string]buildapi.TestExecution)
			for _, te := range b.TestExecutions {
				if !testapi.IsDisplayable(te) {
					continue
				}
				prev, seen := latest[te.TestPlan]
				if !seen || te.CreatedAt > prev.CreatedAt {
					latest[te.TestPlan] = te
				}
			}
			if len(latest) == 0 {
				continue
			}
			plans := make([]string, 0, len(latest))
			for p := range latest {
				plans = append(plans, p)
			}
			sort.Strings(plans)
			for _, plan := range plans {
				te := latest[plan]
				rows = append(rows, execRow{
					artefactName: art.Name,
					product:      art.OS,
					arch:         b.Architecture,
					testPlan:     te.TestPlan,
					status:       te.Status,
					ciLink:       te.CILink,
				})
			}
		}
	}

	if len(rows) == 0 {
		if product != "" {
			return hook.Send(fmt.Sprintf("No test executions found for release **%s** and product **%s**.", release, product))
		}
		return hook.Send(fmt.Sprintf("No test executions found for release **%s**.", release))
	}

	var sb strings.Builder
	if product != "" {
		fmt.Fprintf(&sb, "**Test Status** · %s · %s · %s\n\n",
			release, product, time.Now().UTC().Format("2006-01-02 15:04 UTC"))
	} else {
		fmt.Fprintf(&sb, "**Test Status** · %s · %s\n\n",
			release, time.Now().UTC().Format("2006-01-02 15:04 UTC"))
	}
	sb.WriteString("| Artefact | Product | Arch | Test Plan | Status |\n")
	sb.WriteString("|----------|---------|------|-----------|--------|\n")
	for _, row := range rows {
		statusCell := testapi.ExecStatusEmoji(row.status) + " " + row.status
		if row.ciLink != "" {
			statusCell = fmt.Sprintf("%s [%s](%s)", testapi.ExecStatusEmoji(row.status), row.status, row.ciLink)
		}
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s |\n",
			row.artefactName, row.product, row.arch, row.testPlan, statusCell)
	}
	return hook.Send(sb.String())
}

func helpText() string {
	return `**ARGUS — available commands:**

| Command | Description |
|---------|-------------|
| ` + "`builds status`" + `                          | Build summary for all releases with progress bar |
| ` + "`builds status <release>`" + `                | Detailed build status for a specific release |
| ` + "`builds status <release> <product>`" + `      | Filter detail view to a single product |
| ` + "`tests status`" + `                           | Test summary for all releases with progress bar |
| ` + "`tests status <release>`" + `                 | Detailed test status for a specific release |
| ` + "`tests status <release> <product>`" + `       | Filter test detail view to a single product |
| ` + "`help`" + `                                   | Show this message |

Proactive change reports are posted automatically when build statuses change.`
}
