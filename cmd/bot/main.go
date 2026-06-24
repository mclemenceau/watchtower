package main

import (
	"context"
	"flag"
	"io"
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
	verbose := flag.Bool("v", false, "enable verbose logging")
	flag.Parse()

	// Silence Go's standard logger and the Temporal SDK logger unless -v is set.
	if !*verbose {
		log.SetOutput(io.Discard)
	}

	cfg, err := config.Load()
	if err != nil {
		// Always print fatal config errors regardless of verbosity.
		os.Stderr.WriteString("config: " + err.Error() + "\n")
		os.Exit(1)
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

	// Connect to Temporal. Pass a no-op logger to suppress SDK output.
	temporalLogger := newTemporalLogger(*verbose)
	c, err := client.Dial(client.Options{
		HostPort: cfg.TemporalHost,
		Logger:   temporalLogger,
	})
	if err != nil {
		os.Stderr.WriteString("bot: dial temporal: " + err.Error() + "\n")
		os.Exit(1)
	}
	defer c.Close()

	act := &activities.Activities{
		Artefacts:      buildapi.NewHTTPClient(cfg.TestObserverURL),
		Snapshot:       snap,
		Hook:           hook,
		DefaultRelease: cfg.DefaultRelease,
	}

	// Register and start the Temporal worker in the background.
	w := worker.New(c, taskQueue, worker.Options{
		WorkerStopTimeout: 0,
	})
	w.RegisterWorkflow(argusworkflow.ChangeWatchWorkflow)
	w.RegisterActivity(act)

	go func() {
		if err := w.Run(worker.InterruptCh()); err != nil {
			log.Printf("temporal worker stopped: %v", err)
		}
	}()

	// Terminate any stale workflows whose types are no longer registered.
	terminateStaleWorkflows(c, "status-table", "query")

	// Start cron workflow (idempotent — Temporal ignores if already running).
	startCronWorkflows(c)

	// If the snapshot is empty (first boot), trigger an immediate fetch so
	// commands work right away without waiting up to 10 min for the cron.
	triggerInitialFetch(c, snap)

	log.Printf("bot started on task queue %q (temporal: %s)", taskQueue, cfg.TemporalHost)

	// Start the Mattermost channel poller in the background (no-op when credentials are absent).
	pollerCtx, cancelPoller := context.WithCancel(context.Background())
	defer cancelPoller()
	go mattermost.RunPoller(pollerCtx, mattermost.PollerConfig{
		ServerURL: cfg.MattermostServerURL,
		Token:     cfg.MattermostToken,
		ChannelID: cfg.MattermostChannelID,
		Interval:  cfg.MattermostPollInterval,
		Keyword:   cfg.WatchtowerKeyword,
	}, snap, cfg.DefaultRelease, hook, nil)

	// Run the interactive REPL — blocks until stdin is closed or Ctrl-D.
	mattermost.RunREPL(context.Background(), os.Stdin, hook, snap, cfg.DefaultRelease, cfg.WatchtowerKeyword)
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

// terminateStaleWorkflows terminates workflows by ID that are no longer
// supported by this version of the bot (e.g. removed workflow types).
// Errors are logged and ignored — the workflow may already be gone.
func terminateStaleWorkflows(c client.Client, ids ...string) {
	for _, id := range ids {
		err := c.TerminateWorkflow(context.Background(), id, "", "workflow type removed in bot redesign")
		if err != nil {
			log.Printf("note: terminate stale workflow %q: %v", id, err)
		} else {
			log.Printf("terminated stale workflow %q", id)
		}
	}
}

// triggerInitialFetch runs one ChangeWatchWorkflow synchronously if the snapshot
// is empty, so commands are usable immediately on first boot.
func triggerInitialFetch(c client.Client, snap *state.Snapshot) {
	artefacts, err := snap.Read()
	if err != nil || len(artefacts) > 0 {
		return // snapshot already populated or unreadable — don't block
	}
	log.Print("snapshot empty — triggering initial fetch (this may take a few seconds)...")
	run, err := c.ExecuteWorkflow(
		context.Background(),
		client.StartWorkflowOptions{
			ID:        "change-watch-init",
			TaskQueue: taskQueue,
		},
		argusworkflow.ChangeWatchWorkflow,
	)
	if err != nil {
		log.Printf("note: initial fetch start: %v", err)
		return
	}
	if err := run.Get(context.Background(), nil); err != nil {
		log.Printf("note: initial fetch: %v", err)
	} else {
		log.Print("initial fetch complete")
	}
}
