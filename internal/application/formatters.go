package application

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mclemenceau/watchtower/internal/domain"
)

// FormatBuildsStatusSummary renders a summary table: one row per release with
// built/total counts and a 10-square progress bar (🟩 per 10% built, 🟥 for the rest).
// Returns a "no snapshot" message when artefacts is empty.
func FormatBuildsStatusSummary(artefacts []domain.Artefact) string {
	if len(artefacts) == 0 {
		return "No snapshot available yet — the first fetch is still in progress."
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
		if domain.IsBuiltToday(art.Version) {
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
	return sb.String()
}

// FormatBuildsStatusRelease renders a detail table for a single release: one row per
// artefact with name, version, age and build status. When product is non-empty only
// artefacts whose OS matches (case-insensitive) are shown.
// Returns an appropriate message when no artefacts match.
func FormatBuildsStatusRelease(artefacts []domain.Artefact, release, product string) string {
	if len(artefacts) == 0 {
		return "No snapshot available yet — the first fetch is still in progress."
	}

	var filtered []domain.Artefact
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
			return fmt.Sprintf("No artefacts found for release **%s** and product **%s**.", release, product)
		}
		return fmt.Sprintf("No artefacts found for release **%s**.", release)
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
	sb.WriteString("| ID | Artefact | Product | Version | Age | Build | Log |\n")
	sb.WriteString("|----|----------|---------|---------|-----|-------|-----|\n")
	for _, art := range filtered {
		fmt.Fprintf(&sb, "| %d | %s | %s | %s | %s | %s | %s |\n",
			art.ID, art.Name, art.OS, art.Version, domain.ImageAge(art.Version), domain.BuildStatus(art.Version), domain.LogCell(art.ImageURL))
	}
	return sb.String()
}

// FormatTestsStatusSummary renders a summary table of test execution status across
// all releases. Only releases that have at least one displayable test execution are
// shown. The progress bar reflects PASSED executions as a fraction of total
// displayable executions (🟩 per 10%, 🟥 for the rest).
func FormatTestsStatusSummary(artefacts []domain.Artefact) string {
	if len(artefacts) == 0 {
		return "No snapshot available yet — the first fetch is still in progress."
	}

	type releaseStat struct {
		passed int
		total  int
	}
	stats := make(map[string]*releaseStat)

	for _, art := range artefacts {
		for _, b := range art.Builds {
			for _, te := range b.TestExecutions {
				if !domain.IsDisplayable(te) {
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
		return "No test executions found in snapshot — test data may still be loading."
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
	return sb.String()
}

// FormatTestsStatusRelease renders a detail table for a single release. Each row
// represents one displayable test execution. Multiple executions of the same test
// plan for the same artefact+build are deduplicated to the latest only (by
// CreatedAt). Artefacts with no displayable executions are omitted. When product
// is non-empty, only artefacts whose OS matches are shown.
func FormatTestsStatusRelease(artefacts []domain.Artefact, release, product string) string {
	if len(artefacts) == 0 {
		return "No snapshot available yet — the first fetch is still in progress."
	}

	// Filter artefacts to the requested release (and optional product).
	var filtered []domain.Artefact
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
			return fmt.Sprintf("No artefacts found for release **%s** and product **%s**.", release, product)
		}
		return fmt.Sprintf("No artefacts found for release **%s**.", release)
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
			latest := make(map[string]domain.TestExecution)
			for _, te := range b.TestExecutions {
				if !domain.IsDisplayable(te) {
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
			return fmt.Sprintf("No test executions found for release **%s** and product **%s**.", release, product)
		}
		return fmt.Sprintf("No test executions found for release **%s**.", release)
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
		statusCell := domain.ExecStatusEmoji(row.status) + " " + row.status
		if row.ciLink != "" {
			statusCell = fmt.Sprintf("%s [%s](%s)", domain.ExecStatusEmoji(row.status), row.status, row.ciLink)
		}
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s |\n",
			row.artefactName, row.product, row.arch, row.testPlan, statusCell)
	}
	return sb.String()
}

// FormatChangeReport renders a Markdown change report message from a ChangeReport.
func FormatChangeReport(r domain.ChangeReport) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "**Change Report** · %s\n\n", time.Now().UTC().Format("2006-01-02 15:04 UTC"))

	if len(r.NewFailures) > 0 {
		sb.WriteString("🔴 **New Failures**\n\n")
		sb.WriteString("| Release | Artefact | Previous Status |\n")
		sb.WriteString("|---------|----------|-----------------|\n")
		for _, f := range r.NewFailures {
			fmt.Fprintf(&sb, "| %s | %s | %s |\n", f.Release, f.Name, f.OldStatus)
		}
		sb.WriteString("\n")
	}

	if len(r.Recoveries) > 0 {
		sb.WriteString("🟢 **Recoveries**\n\n")
		sb.WriteString("| Release | Artefact |\n")
		sb.WriteString("|---------|----------|\n")
		for _, rec := range r.Recoveries {
			fmt.Fprintf(&sb, "| %s | %s |\n", rec.Release, rec.Name)
		}
		sb.WriteString("\n")
	}

	if len(r.OtherChanges) > 0 {
		sb.WriteString("🔵 **Other Changes**\n\n")
		sb.WriteString("| Release | Artefact | Old Status | New Status |\n")
		sb.WriteString("|---------|----------|------------|------------|\n")
		for _, o := range r.OtherChanges {
			fmt.Fprintf(&sb, "| %s | %s | %s | %s |\n", o.Release, o.Name, o.OldStatus, o.NewStatus)
		}
		sb.WriteString("\n")
	}

	if len(r.NewArtefacts) > 0 {
		sb.WriteString("🆕 **New Artefacts**\n\n")
		sb.WriteString("| Release | Product | Artefact | Version | Age | Build | Log |\n")
		sb.WriteString("|---------|---------|----------|---------|-----|-------|-----|\n")
		for _, n := range r.NewArtefacts {
			fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s | %s | %s |\n",
				n.Release, n.OS, n.Name, n.Version, domain.ImageAge(n.Version), domain.BuildStatus(n.Version), domain.LogCell(n.ImageURL))
		}
	}

	return sb.String()
}

// FormatInvestigation renders the LLM log analysis result for a single artefact.
// source is a human-readable description of which log was analysed
// (e.g. "Launchpad librarian (amd64)" or "cd-build-log").
func FormatInvestigation(art domain.Artefact, analysis domain.LogAnalysis, source string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "**Investigation — %s** (ID: %d) · %s\n\n",
		art.Name, art.ID, time.Now().UTC().Format("2006-01-02 15:04 UTC"))
	fmt.Fprintf(&sb, "**Log source:** %s\n", source)
	fmt.Fprintf(&sb, "**Category:** %s\n", analysis.Category)
	fmt.Fprintf(&sb, "**Hypothesis:** %s\n", analysis.Hypothesis)
	if len(analysis.LogExcerpts) > 0 {
		sb.WriteString("\n**Relevant log excerpts:**\n")
		for _, line := range analysis.LogExcerpts {
			fmt.Fprintf(&sb, "- `%s`\n", line)
		}
	}
	fmt.Fprintf(&sb, "\n**Recommended action:** %s\n", analysis.NextAction)
	return sb.String()
}

// HelpText returns the Markdown help message listing all available commands.
func HelpText() string {
	return `**Watchtower — available commands:**

| Command | Description |
|---------|-------------|
| ` + "`builds status`" + `                          | Build summary for all releases with progress bar |
| ` + "`builds status <release>`" + `                | Detailed build status for a specific release (includes artefact IDs) |
| ` + "`builds status <release> <product>`" + `      | Filter detail view to a single product |
| ` + "`tests status`" + `                           | Test summary for all releases with progress bar |
| ` + "`tests status <release>`" + `                 | Detailed test status for a specific release |
| ` + "`tests status <release> <product>`" + `       | Filter test detail view to a single product |
| ` + "`investigate <artefact-id>`" + `              | Fetch build log and run LLM root-cause analysis |
| ` + "`help`" + `                                   | Show this message |

Proactive change reports are posted automatically when build statuses change.`
}
