package workflow

import (
	"time"

	sdk "go.temporal.io/sdk/workflow"

	"github.com/mclemenceau/watchtower/internal/activities"
	"github.com/mclemenceau/watchtower/internal/domain"
)

// FailureAnalysisWorkflow reads unresolved FailureRecords that have no LLM
// analysis yet, fetches their build logs, runs LLM root-cause analysis, and
// persists the results back to failures.json.
//
// It processes at most MaxAnalysisPerRun records per execution (token cap,
// default 5). This workflow is intended to run on a slow cron schedule
// (e.g. every 8 hours via FAILURE_ANALYSIS_CRON_SCHEDULE) and can also be
// triggered on-demand by the bot when a user explicitly requests it.
func FailureAnalysisWorkflow(ctx sdk.Context) error {
	ao := sdk.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute, // analysis can be slow if LLM is rate-limited
	}
	ctx = sdk.WithActivityOptions(ctx, ao)

	var act *activities.Activities

	// Load the current artefact snapshot so log URLs can be resolved.
	var artefacts []domain.Artefact
	if err := sdk.ExecuteActivity(ctx, act.LoadSnapshot).Get(ctx, &artefacts); err != nil {
		// Non-fatal: if snapshot is unreadable, AnalyseFailures will find no
		// artefacts by ID and will skip all pending records gracefully.
		sdk.GetLogger(ctx).Warn("FailureAnalysisWorkflow: LoadSnapshot failed", "error", err)
	}

	// Run LLM analysis on pending records (capped by MaxAnalysisPerRun).
	if err := sdk.ExecuteActivity(ctx, act.AnalyseFailures, artefacts).Get(ctx, nil); err != nil {
		return err
	}

	return nil
}
