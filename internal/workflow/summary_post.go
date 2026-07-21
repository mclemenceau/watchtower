package workflow

import (
	"time"

	sdk "go.temporal.io/sdk/workflow"

	"github.com/mclemenceau/watchtower/internal/activities"
)

// SummaryPostWorkflow reads the current snapshot and posts the scheduled build
// summary to the notification channel. It applies the SummaryForReleases and
// SummaryForProducts filters configured on the Activities struct so that only
// the relevant scope is reported.
//
// Intended to run on a less-frequent cron schedule (e.g. 3× per day via
// SUMMARY_CRON_SCHEDULE) to give the team a regular morning/afternoon/evening
// status update without constant noise.
func SummaryPostWorkflow(ctx sdk.Context) error {
	ctx = sdk.WithActivityOptions(ctx, sdk.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
	})

	var act *activities.Activities

	if err := sdk.ExecuteActivity(ctx, act.PostSummary).Get(ctx, nil); err != nil {
		sdk.GetLogger(ctx).Warn("PostSummary failed", "error", err)
	}

	return nil
}
