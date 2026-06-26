package workflow

import (
	"fmt"
	"strings"
	"time"

	sdk "go.temporal.io/sdk/workflow"

	"github.com/mclemenceau/argus/internal/activities"
	"github.com/mclemenceau/argus/internal/buildapi"
	"github.com/mclemenceau/argus/internal/state"
)

func ChangeWatchWorkflow(ctx sdk.Context) error {
	ctx = sdk.WithActivityOptions(ctx, sdk.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
	})

	var act *activities.Activities

	// Fetch fresh artefact list from Test Observer.
	var fresh []buildapi.Artefact
	if err := sdk.ExecuteActivity(ctx, act.FetchBuildStatus).Get(ctx, &fresh); err != nil {
		return err
	}

	// Enrich each artefact with its test execution data (one API call per artefact).
	// Use a longer timeout for this activity since it fans out across all artefacts.
	testCtx := sdk.WithActivityOptions(ctx, sdk.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
	})
	var enriched []buildapi.Artefact
	if err := sdk.ExecuteActivity(testCtx, act.FetchTestExecutions, fresh).Get(testCtx, &enriched); err != nil {
		// Non-fatal: fall back to unenriched artefacts so builds still work.
		sdk.GetLogger(ctx).Warn("FetchTestExecutions failed, test data will be stale", "error", err)
		enriched = fresh
	}

	var old []buildapi.Artefact
	if err := sdk.ExecuteActivity(ctx, act.LoadSnapshot).Get(ctx, &old); err != nil {
		return err
	}

	report := state.Diff(old, enriched)

	if err := sdk.ExecuteActivity(ctx, act.SaveSnapshot, enriched).Get(ctx, nil); err != nil {
		return err
	}

	if hasChanges(report) {
		sdk.GetLogger(ctx).Info("changes detected",
			"new_failures", len(report.NewFailures),
			"recoveries", len(report.Recoveries),
			"other_changes", len(report.OtherChanges),
			"new_artefacts", len(report.NewArtefacts),
		)
		msg := formatChangeReport(report)
		if err := sdk.ExecuteActivity(ctx, act.NotifyChannel, msg).Get(ctx, nil); err != nil {
			sdk.GetLogger(ctx).Warn("NotifyChannel failed", "error", err)
		}
	}

	return nil
}

func formatChangeReport(r buildapi.ChangeReport) string {
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
		sb.WriteString("| Release | Product | Artefact | Version | Age | Build |\n")
		sb.WriteString("|---------|---------|----------|---------|-----|-------|\n")
		for _, n := range r.NewArtefacts {
			fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s | %s |\n",
				n.Release, n.OS, n.Name, n.Version, buildapi.ImageAge(n.Version), buildapi.BuildStatus(n.Version, n.ImageURL))
		}
	}

	return sb.String()
}

func hasChanges(r buildapi.ChangeReport) bool {
	return len(r.NewFailures)+len(r.Recoveries)+len(r.OtherChanges)+len(r.NewArtefacts) > 0
}
