package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/mclemenceau/watchtower/internal/activities"
	mattermostadapter "github.com/mclemenceau/watchtower/internal/adapters/mattermost"
	"github.com/mclemenceau/watchtower/internal/adapters/openrouter"
	"github.com/mclemenceau/watchtower/internal/adapters/testobserver"
	"github.com/mclemenceau/watchtower/internal/config"
	"github.com/mclemenceau/watchtower/internal/domain"
	"github.com/mclemenceau/watchtower/internal/intent"
	"github.com/mclemenceau/watchtower/internal/ports"
	"github.com/mclemenceau/watchtower/internal/state"
	watchtowerworkflow "github.com/mclemenceau/watchtower/internal/workflow"
)

const taskQueue = "watchtower"

// httpLogFetcher implements ports.LogFetcher using a standard http.Client.
type httpLogFetcher struct {
	client *http.Client
}

func (f *httpLogFetcher) Fetch(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("LogFetcher: new request: %w", err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("LogFetcher: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LogFetcher: unexpected status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("LogFetcher: read: %w", err)
	}
	return string(raw), nil
}

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
		fmt.Fprintf(os.Stderr, "config: %s\n", err.Error())
		os.Exit(1)
	}

	// Select notifier: real Mattermost webhook or stdout simulation.
	var notifier ports.Notifier
	if cfg.MattermostWebhookURL != "" {
		notifier = mattermostadapter.NewHTTPNotifier(cfg.MattermostWebhookURL)
		log.Printf("mattermost webhook: %s", cfg.MattermostWebhookURL)
	} else {
		notifier = &mattermostadapter.StdoutNotifier{}
		log.Print("mattermost webhook: stdout simulation (set MATTERMOST_WEBHOOK_URL for real Mattermost)")
	}

	artefactSrc := testobserver.NewHTTPArtefactSource(cfg.TestObserverURL)
	buildSrc := testobserver.NewHTTPBuildSource(cfg.TestObserverURL)
	snap := state.New("state/snapshot.json")
	logFetcher := &httpLogFetcher{client: &http.Client{Timeout: 30 * time.Second}}

	// Build optional LLM-backed intent resolver (disabled when API key is absent).
	var resolver *intent.Resolver
	var llmClient ports.LLMClient
	if cfg.OpenRouterAPIKey != "" {
		llmClient = openrouter.NewClient(cfg.OpenRouterAPIKey, cfg.LLMModel)
		resolver = intent.New(llmClient)
		log.Printf("intent resolver: enabled (model: %s)", cfg.LLMModel)
	} else {
		log.Print("intent resolver: disabled (set OPENROUTER_API_KEY to enable)")
	}

	// Connect to Temporal. Pass a no-op logger to suppress SDK output.
	temporalLogger := newTemporalLogger(*verbose)
	c, err := client.Dial(client.Options{
		HostPort: cfg.TemporalHost,
		Logger:   temporalLogger,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "bot: dial temporal: %s\n", err.Error())
		os.Exit(1)
	}
	defer c.Close()

	act := &activities.Activities{
		Artefacts:      artefactSrc,
		Tests:          buildSrc,
		Snapshot:       snap,
		Hook:           notifier,
		LogFetcher:     logFetcher,
		DefaultRelease: cfg.DefaultRelease,
		LLM:            llmClient,
	}

	// Register and start the Temporal worker in the background.
	w := worker.New(c, taskQueue, worker.Options{
		WorkerStopTimeout: 0,
	})
	w.RegisterWorkflow(watchtowerworkflow.ChangeWatchWorkflow)
	w.RegisterActivity(act)

	go func() {
		if err := w.Run(worker.InterruptCh()); err != nil {
			log.Printf("temporal worker stopped: %v", err)
		}
	}()

	// Terminate any stale workflows whose types are no longer registered.
	terminateStaleWorkflows(c, "status-table", "query")

	// Start cron workflow (idempotent — Temporal ignores if already running).
	startCronWorkflows(c, cfg.CronSchedule)

	// If the snapshot is empty (first boot), trigger an immediate fetch so
	// commands work right away without waiting up to 10 min for the cron.
	triggerInitialFetch(c, snap)

	log.Printf("bot started on task queue %q (temporal: %s)", taskQueue, cfg.TemporalHost)

	// Start the Mattermost channel poller in the background (no-op when credentials are absent).
	pollerCtx, cancelPoller := context.WithCancel(context.Background())
	defer cancelPoller()
	go mattermostadapter.RunPoller(pollerCtx, mattermostadapter.PollerConfig{
		ServerURL: cfg.MattermostServerURL,
		Token:     cfg.MattermostToken,
		ChannelID: cfg.MattermostChannelID,
		Interval:  cfg.MattermostPollInterval,
		Keyword:   cfg.WatchtowerKeyword,
	}, snap, cfg.DefaultRelease, notifier, nil, resolver)

	// Run the interactive REPL — blocks until stdin is closed or Ctrl-D.
	mattermostadapter.RunREPL(context.Background(), os.Stdin, notifier, snap, cfg.DefaultRelease, cfg.WatchtowerKeyword, resolver)
}

func startCronWorkflows(c client.Client, cronSchedule string) {
	_, err := c.ExecuteWorkflow(
		context.Background(),
		client.StartWorkflowOptions{
			ID:           "change-watch",
			TaskQueue:    taskQueue,
			CronSchedule: cronSchedule,
		},
		watchtowerworkflow.ChangeWatchWorkflow,
	)
	if err != nil {
		log.Printf("note: change-watch cron start: %v", err)
	} else {
		log.Printf("change-watch cron scheduled (%s)", cronSchedule)
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
// is empty or contains no test execution data (e.g. first boot after the tests
// feature was added), so commands are usable immediately on first boot.
func triggerInitialFetch(c client.Client, snap *state.Snapshot) {
	artefacts, err := snap.Read()
	if err != nil {
		return // unreadable — don't block
	}
	// Skip if snapshot is already fully populated (has artefacts with builds).
	if len(artefacts) > 0 && hasTestData(artefacts) {
		return
	}
	if len(artefacts) == 0 {
		log.Print("snapshot empty — triggering initial fetch (this may take a few minutes)...")
	} else {
		log.Print("snapshot has no test data — triggering enrichment fetch (this may take a few minutes)...")
	}
	run, err := c.ExecuteWorkflow(
		context.Background(),
		client.StartWorkflowOptions{
			ID:        "change-watch-init",
			TaskQueue: taskQueue,
		},
		watchtowerworkflow.ChangeWatchWorkflow,
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

// hasTestData returns true if at least one artefact has build/test data cached.
func hasTestData(artefacts []domain.Artefact) bool {
	for _, a := range artefacts {
		if len(a.Builds) > 0 {
			return true
		}
	}
	return false
}
