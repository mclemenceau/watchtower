package workflow

import (
	"time"

	sdk "go.temporal.io/sdk/workflow"

	"github.com/mclemenceau/watchtower/internal/activities"
	"github.com/mclemenceau/watchtower/internal/domain"
	"github.com/mclemenceau/watchtower/internal/state"
)

// DataRefreshWorkflow fetches a fresh artefact + test-execution snapshot from
// Test Observer, persists it, and updates the failure store with any new
// failures or recoveries detected in the diff.
//
// It never sends any notification — its sole purpose is to keep the local
// snapshot and failure store up to date so that bot commands and the summary
// workflow always read recent data.
//
// Intended to run on a frequent cron schedule (e.g. every 30 min via
// REFRESH_CRON_SCHEDULE).
func DataRefreshWorkflow(ctx sdk.Context) error {
	ctx = sdk.WithActivityOptions(ctx, sdk.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
	})

	var act *activities.Activities

	// 1. Load the previous snapshot for diffing.
	var old []domain.Artefact
	if err := sdk.ExecuteActivity(ctx, act.LoadSnapshot).Get(ctx, &old); err != nil {
		// Non-fatal: first boot has no snapshot; diff will treat everything as new.
		sdk.GetLogger(ctx).Warn("LoadSnapshot failed, diff will be empty", "error", err)
	}

	// 2. Fetch fresh artefact list from Test Observer.
	var fresh []domain.Artefact
	if err := sdk.ExecuteActivity(ctx, act.FetchBuildStatus).Get(ctx, &fresh); err != nil {
		return err
	}

	// 3. Enrich each artefact with its test execution data. Non-fatal: fall back
	//    to unenriched artefacts so build status is still available.
	testCtx := sdk.WithActivityOptions(ctx, sdk.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
	})
	var enriched []domain.Artefact
	if err := sdk.ExecuteActivity(testCtx, act.FetchTestExecutions, fresh).Get(testCtx, &enriched); err != nil {
		sdk.GetLogger(ctx).Warn("FetchTestExecutions failed, test data will be stale", "error", err)
		enriched = fresh
	}

	// 4. Persist the enriched snapshot atomically.
	if err := sdk.ExecuteActivity(ctx, act.SaveSnapshot, enriched).Get(ctx, nil); err != nil {
		return err
	}

	// 5. Diff old vs fresh and update the failure store (upsert new failures,
	//    mark recoveries as resolved). This is deterministic and cheap — no LLM.
	report := state.Diff(old, enriched)
	if len(report.NewFailures) > 0 || len(report.Recoveries) > 0 {
		if err := sdk.ExecuteActivity(ctx, act.UpdateFailureRecords, report, enriched).Get(ctx, nil); err != nil {
			// Non-fatal: failure store update failing should not abort the refresh.
			sdk.GetLogger(ctx).Warn("UpdateFailureRecords failed", "error", err)
		}
	}

	return nil
}
