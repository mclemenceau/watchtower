package workflow

import (
	"time"

	sdk "go.temporal.io/sdk/workflow"

	"github.com/mclemenceau/watchtower/internal/activities"
	"github.com/mclemenceau/watchtower/internal/domain"
	"github.com/mclemenceau/watchtower/internal/state"
)

// DataRefreshWorkflow fetches a fresh artefact + test-execution snapshot from
// Test Observer, persists it, updates the failure store with any new failures
// or recoveries detected in the diff, and posts a compact notification when
// one or more artefacts have a new successful build since the previous snapshot.
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

	// 2.5. Enrich artefacts with build log status from today's cd-build-log.
	//      Non-fatal: if log fetching fails, BuildLog fields remain empty and
	//      formatters fall back to the version-date binary status.
	logCtx := sdk.WithActivityOptions(ctx, sdk.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
	})
	var logEnriched []domain.Artefact
	if err := sdk.ExecuteActivity(logCtx, act.EnrichBuildStatus, fresh).Get(logCtx, &logEnriched); err != nil {
		sdk.GetLogger(ctx).Warn("EnrichBuildStatus failed, build log status will be unavailable", "error", err)
		logEnriched = fresh
	}

	// 3. Enrich each artefact with its test execution data. Non-fatal: fall back
	//    to unenriched artefacts so build status is still available.
	testCtx := sdk.WithActivityOptions(ctx, sdk.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
	})
	var enriched []domain.Artefact
	if err := sdk.ExecuteActivity(testCtx, act.FetchTestExecutions, logEnriched).Get(testCtx, &enriched); err != nil {
		sdk.GetLogger(ctx).Warn("FetchTestExecutions failed, test data will be stale", "error", err)
		enriched = logEnriched
	}

	// 4. Persist the enriched snapshot atomically.
	if err := sdk.ExecuteActivity(ctx, act.SaveSnapshot, enriched).Get(ctx, nil); err != nil {
		return err
	}

	// 5. Diff old vs fresh and update the failure store (upsert new failures,
	//    mark recoveries as resolved). This is deterministic and cheap — no LLM.
	// Also run when there are new artefacts: some may already be MARKED_AS_FAILED
	// (first-boot seeding — Diff has no old status to transition from so they
	// appear as NewArtefacts rather than NewFailures).
	report := state.Diff(old, enriched)
	if len(report.NewFailures) > 0 || len(report.Recoveries) > 0 || len(report.NewArtefacts) > 0 {
		if err := sdk.ExecuteActivity(ctx, act.UpdateFailureRecords, report, enriched).Get(ctx, nil); err != nil {
			// Non-fatal: failure store update failing should not abort the refresh.
			sdk.GetLogger(ctx).Warn("UpdateFailureRecords failed", "error", err)
		}
	}

	// 6. Notify the broadcast channel about any newly completed builds.
	//    Non-fatal: a notification failure should not fail the data refresh.
	if len(report.NewBuilds) > 0 {
		notifyCtx := sdk.WithActivityOptions(ctx, sdk.ActivityOptions{
			StartToCloseTimeout: 30 * time.Second,
		})
		if err := sdk.ExecuteActivity(notifyCtx, act.NotifyNewBuilds, report).Get(notifyCtx, nil); err != nil {
			sdk.GetLogger(ctx).Warn("NotifyNewBuilds failed", "error", err)
		}
	}

	return nil
}
