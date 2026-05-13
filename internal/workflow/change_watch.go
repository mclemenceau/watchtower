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

	var fresh []buildapi.Image
	if err := sdk.ExecuteActivity(ctx, act.FetchBuildStatus).Get(ctx, &fresh); err != nil {
		return err
	}

	var old []buildapi.Image
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
			"new_images", len(report.NewImages),
		)
		msg := formatChangeReport(report)
		if err := sdk.ExecuteActivity(ctx, act.PushToFeed, msg).Get(ctx, nil); err != nil {
			sdk.GetLogger(ctx).Warn("PushToFeed failed", "error", err)
		}
	}

	return nil
}

func formatChangeReport(r buildapi.ChangeReport) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Change Report (%s) ===\n", time.Now().UTC().Format("15:04 UTC")))
	for _, f := range r.NewFailures {
		sb.WriteString(fmt.Sprintf("🔴 FAILED    %s  (was %s)\n", f.Image, f.OldStatus))
	}
	for _, rec := range r.Recoveries {
		sb.WriteString(fmt.Sprintf("🟢 RECOVERED %s  (now %s)\n", rec.Image, rec.NewStatus))
	}
	for _, o := range r.OtherChanges {
		sb.WriteString(fmt.Sprintf("🔵 CHANGED   %s  (%s → %s)\n", o.Image, o.OldStatus, o.NewStatus))
	}
	for _, n := range r.NewImages {
		sb.WriteString(fmt.Sprintf("🆕 NEW       %s  (%s)\n", n.ID, n.Status))
	}
	return sb.String()
}

func hasChanges(r buildapi.ChangeReport) bool {
	return len(r.NewFailures)+len(r.Recoveries)+len(r.OtherChanges)+len(r.NewImages) > 0
}
