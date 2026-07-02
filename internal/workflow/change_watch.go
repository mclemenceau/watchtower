package workflow

import (
	"time"

	sdk "go.temporal.io/sdk/workflow"

	"github.com/mclemenceau/watchtower/internal/activities"
	"github.com/mclemenceau/watchtower/internal/application"
	"github.com/mclemenceau/watchtower/internal/domain"
	"github.com/mclemenceau/watchtower/internal/state"
)

func ChangeWatchWorkflow(ctx sdk.Context) error {
	ctx = sdk.WithActivityOptions(ctx, sdk.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
	})

	var act *activities.Activities

	// Fetch fresh artefact list from Test Observer.
	var fresh []domain.Artefact
	if err := sdk.ExecuteActivity(ctx, act.FetchBuildStatus).Get(ctx, &fresh); err != nil {
		return err
	}

	// Enrich each artefact with its test execution data (one API call per artefact).
	// Use a longer timeout for this activity since it fans out across all artefacts.
	testCtx := sdk.WithActivityOptions(ctx, sdk.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
	})
	var enriched []domain.Artefact
	if err := sdk.ExecuteActivity(testCtx, act.FetchTestExecutions, fresh).Get(testCtx, &enriched); err != nil {
		// Non-fatal: fall back to unenriched artefacts so builds still work.
		sdk.GetLogger(ctx).Warn("FetchTestExecutions failed, test data will be stale", "error", err)
		enriched = fresh
	}

	var old []domain.Artefact
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
		msg := application.FormatChangeReport(report)
		if err := sdk.ExecuteActivity(ctx, act.NotifyChannel, msg).Get(ctx, nil); err != nil {
			sdk.GetLogger(ctx).Warn("NotifyChannel failed", "error", err)
		}
	}

	return nil
}

func hasChanges(r domain.ChangeReport) bool {
	return len(r.NewFailures)+len(r.Recoveries)+len(r.OtherChanges)+len(r.NewArtefacts) > 0
}
