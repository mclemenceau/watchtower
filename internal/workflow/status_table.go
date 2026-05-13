package workflow

import (
	"fmt"
	"strings"
	"time"

	sdk "go.temporal.io/sdk/workflow"

	"github.com/mclemenceau/argus/internal/activities"
	"github.com/mclemenceau/argus/internal/buildapi"
)

func StatusTableWorkflow(ctx sdk.Context) error {
	ctx = sdk.WithActivityOptions(ctx, sdk.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
	})

	var act *activities.Activities

	var images []buildapi.Image
	if err := sdk.ExecuteActivity(ctx, act.FetchBuildStatus).Get(ctx, &images); err != nil {
		return err
	}

	table := formatStatusTable(images)

	if err := sdk.ExecuteActivity(ctx, act.PushToFeed, table).Get(ctx, nil); err != nil {
		sdk.GetLogger(ctx).Warn("StatusTableWorkflow: PushToFeed failed", "error", err)
	}
	return nil
}

func formatStatusTable(images []buildapi.Image) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Build Status (%s) ===\n", time.Now().UTC().Format("2006-01-02 15:04 UTC")))
	sb.WriteString(fmt.Sprintf("%-35s %-12s %-8s %-10s\n", "IMAGE", "STATUS", "ARCH", "SERIES"))
	sb.WriteString(strings.Repeat("─", 70) + "\n")
	for _, img := range images {
		sb.WriteString(fmt.Sprintf("%-35s %-12s %-8s %-10s\n",
			img.ID, img.Status, img.Arch, img.Series))
	}
	return sb.String()
}
