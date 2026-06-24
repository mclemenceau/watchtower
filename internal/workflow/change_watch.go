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

	var fresh []buildapi.Artefact
	if err := sdk.ExecuteActivity(ctx, act.FetchBuildStatus).Get(ctx, &fresh); err != nil {
		return err
	}

	var old []buildapi.Artefact
	if err := sdk.ExecuteActivity(ctx, act.LoadSnapshot).Get(ctx, &old); err != nil {
		return err
	}

	report := state.Diff(old, fresh)

	if err := sdk.ExecuteActivity(ctx, act.SaveSnapshot, fresh).Get(ctx, nil); err != nil {
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
	sb.WriteString(fmt.Sprintf("=== Change Report (%s) ===\n", time.Now().UTC().Format("15:04 UTC")))
	for _, f := range r.NewFailures {
		sb.WriteString(fmt.Sprintf("🔴 FAILED     %s  %s  (was %s)\n", f.Release, f.Name, f.OldStatus))
	}
	for _, rec := range r.Recoveries {
		sb.WriteString(fmt.Sprintf("🟢 APPROVED   %s  %s  (was MARKED_AS_FAILED)\n", rec.Release, rec.Name))
	}
	for _, o := range r.OtherChanges {
		sb.WriteString(fmt.Sprintf("🔵 CHANGED    %s  %s  (%s → %s)\n", o.Release, o.Name, o.OldStatus, o.NewStatus))
	}
	for _, n := range r.NewArtefacts {
		sb.WriteString(fmt.Sprintf("🆕 NEW        %s  %s  v%s\n", n.Release, n.Name, n.Version))
	}
	return sb.String()
}

func hasChanges(r buildapi.ChangeReport) bool {
	return len(r.NewFailures)+len(r.Recoveries)+len(r.OtherChanges)+len(r.NewArtefacts) > 0
}
