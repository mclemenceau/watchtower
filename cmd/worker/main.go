package main

import (
	"context"
	"log"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/mclemenceau/argus/internal/activities"
	"github.com/mclemenceau/argus/internal/buildapi"
	"github.com/mclemenceau/argus/internal/config"
	"github.com/mclemenceau/argus/internal/llm"
	"github.com/mclemenceau/argus/internal/state"
	argusworkflow "github.com/mclemenceau/argus/internal/workflow"
)

const taskQueue = "argus"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	c, err := client.Dial(client.Options{
		HostPort: cfg.TemporalHost,
	})
	if err != nil {
		log.Fatalf("worker: dial temporal: %v", err)
	}
	defer c.Close()

	act := &activities.Activities{
		Builds:   buildapi.NewMockClient(),
		Snapshot: state.New("state/snapshot.json"),
		LLM:      llm.NewOpenRouterClient(cfg.OpenRouterAPIKey),
		FeedURL:  cfg.ServerURL,
	}

	w := worker.New(c, taskQueue, worker.Options{})

	w.RegisterWorkflow(argusworkflow.ChangeWatchWorkflow)
	w.RegisterWorkflow(argusworkflow.StatusTableWorkflow)
	w.RegisterWorkflow(argusworkflow.QueryWorkflow)

	w.RegisterActivity(act)

	startCronWorkflows(c)

	log.Printf("worker started on task queue %q (temporal: %s)", taskQueue, cfg.TemporalHost)
	if err := w.Run(worker.InterruptCh()); err != nil {
		log.Fatalf("worker: %v", err)
	}
}

func startCronWorkflows(c client.Client) {
	type cronSpec struct {
		id       string
		schedule string
		wf       interface{}
		label    string
	}
	crons := []cronSpec{
		{"change-watch", "*/10 * * * *", argusworkflow.ChangeWatchWorkflow, "every 10 min"},
		{"status-table", "0 */6 * * *", argusworkflow.StatusTableWorkflow, "every 6 h"},
	}
	for _, s := range crons {
		_, err := c.ExecuteWorkflow(
			context.Background(),
			client.StartWorkflowOptions{
				ID:           s.id,
				TaskQueue:    taskQueue,
				CronSchedule: s.schedule,
			},
			s.wf,
		)
		if err != nil {
			log.Printf("note: %s cron start: %v", s.id, err)
		} else {
			log.Printf("%s cron scheduled (%s)", s.id, s.label)
		}
	}
}
