package workflow

import (
	"time"

	sdk "go.temporal.io/sdk/workflow"

	"github.com/mclemenceau/watchtower/internal/activities"
	"github.com/mclemenceau/watchtower/internal/domain"
)

// DataRefreshWorkflow fetches a fresh artefact + test-execution snapshot from
// Test Observer and persists it. It never sends any notification — its sole
// purpose is to keep the local snapshot up to date so that bot commands and
// the summary workflow always read recent data.
//
// Intended to run on a frequent cron schedule (e.g. every 30 min via
// REFRESH_CRON_SCHEDULE).
func DataRefreshWorkflow(ctx sdk.Context) error {
	ctx = sdk.WithActivityOptions(ctx, sdk.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
	})

	var act *activities.Activities

	// 1. Fetch fresh artefact list from Test Observer.
	var fresh []domain.Artefact
	if err := sdk.ExecuteActivity(ctx, act.FetchBuildStatus).Get(ctx, &fresh); err != nil {
		return err
	}

	// 2. Enrich each artefact with its test execution data. Non-fatal: fall back
	//    to unenriched artefacts so build status is still available.
	testCtx := sdk.WithActivityOptions(ctx, sdk.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
	})
	var enriched []domain.Artefact
	if err := sdk.ExecuteActivity(testCtx, act.FetchTestExecutions, fresh).Get(testCtx, &enriched); err != nil {
		sdk.GetLogger(ctx).Warn("FetchTestExecutions failed, test data will be stale", "error", err)
		enriched = fresh
	}

	// 3. Persist the enriched snapshot atomically.
	if err := sdk.ExecuteActivity(ctx, act.SaveSnapshot, enriched).Get(ctx, nil); err != nil {
		return err
	}

	return nil
}
