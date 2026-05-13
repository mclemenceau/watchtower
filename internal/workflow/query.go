package workflow

import (
	"time"

	sdk "go.temporal.io/sdk/workflow"

	"github.com/mclemenceau/argus/internal/activities"
	"github.com/mclemenceau/argus/internal/buildapi"
)

func QueryWorkflow(ctx sdk.Context, query string) (buildapi.AgentReply, error) {
	ctx = sdk.WithActivityOptions(ctx, sdk.ActivityOptions{
		StartToCloseTimeout: 90 * time.Second,
	})

	var act *activities.Activities

	// 1. Load the snapshot maintained by ChangeWatchWorkflow (fast local read).
	var artefacts []buildapi.Artefact
	if err := sdk.ExecuteActivity(ctx, act.LoadSnapshot).Get(ctx, &artefacts); err != nil {
		return buildapi.AgentReply{}, err
	}

	// First boot: snapshot not written yet — fall back to a live fetch.
	if len(artefacts) == 0 {
		if err := sdk.ExecuteActivity(ctx, act.FetchBuildStatus).Get(ctx, &artefacts); err != nil {
			return buildapi.AgentReply{}, err
		}
	}

	// 2. Single LLM call: full snapshot + query → markdown answer.
	var reply buildapi.AgentReply
	if err := sdk.ExecuteActivity(ctx, act.AnswerQuery, query, artefacts).Get(ctx, &reply); err != nil {
		return buildapi.AgentReply{}, err
	}
	reply.WorkflowID = sdk.GetInfo(ctx).WorkflowExecution.ID
	return reply, nil
}
