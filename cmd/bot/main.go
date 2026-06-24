package main

import (
	"context"
	"log"
	"os"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/mclemenceau/argus/internal/activities"
	"github.com/mclemenceau/argus/internal/buildapi"
	"github.com/mclemenceau/argus/internal/config"
	"github.com/mclemenceau/argus/internal/mattermost"
	"github.com/mclemenceau/argus/internal/state"
	argusworkflow "github.com/mclemenceau/argus/internal/workflow"
)

const taskQueue = "argus"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Select webhook client: real Mattermost or stdout simulation.
	var hook mattermost.WebhookClient
	if cfg.MattermostWebhookURL != "" {
		hook = mattermost.NewHTTPWebhookClient(cfg.MattermostWebhookURL)
		log.Printf("mattermost webhook: %s", cfg.MattermostWebhookURL)
	} else {
		hook = &mattermost.StdoutWebhookClient{}
		log.Print("mattermost webhook: stdout simulation (set MATTERMOST_WEBHOOK_URL for real Mattermost)")
	}

	snap := state.New("state/snapshot.json")

	// Connect to Temporal.
	c, err := client.Dial(client.Options{
		HostPort: cfg.TemporalHost,
	})
	if err != nil {
		log.Fatalf("bot: dial temporal: %v", err)
	}
	defer c.Close()

	act := &activities.Activities{
		Artefacts:      buildapi.NewHTTPClient(cfg.TestObserverURL),
		Snapshot:       snap,
		Hook:           hook,
		DefaultRelease: cfg.DefaultRelease,
	}

	// Register and start the Temporal worker in the background.
	w := worker.New(c, taskQueue, worker.Options{})
	w.RegisterWorkflow(argusworkflow.ChangeWatchWorkflow)
	w.RegisterActivity(act)

	go func() {
		if err := w.Run(worker.InterruptCh()); err != nil {
			log.Printf("temporal worker stopped: %v", err)
		}
	}()

	// Start cron workflow (idempotent — Temporal ignores if already running).
	startCronWorkflows(c)

	log.Printf("bot started on task queue %q (temporal: %s)", taskQueue, cfg.TemporalHost)

	// Run the interactive REPL — blocks until stdin is closed or Ctrl-D.
	mattermost.RunREPL(context.Background(), os.Stdin, hook, snap, cfg.DefaultRelease)
}

func startCronWorkflows(c client.Client) {
	_, err := c.ExecuteWorkflow(
		context.Background(),
		client.StartWorkflowOptions{
			ID:           "change-watch",
			TaskQueue:    taskQueue,
			CronSchedule: "*/10 * * * *",
		},
		argusworkflow.ChangeWatchWorkflow,
	)
	if err != nil {
		log.Printf("note: change-watch cron start: %v", err)
	} else {
		log.Print("change-watch cron scheduled (every 10 min)")
	}
}
