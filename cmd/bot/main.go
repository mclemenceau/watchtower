package main

import (
	"compress/gzip"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/mclemenceau/watchtower/internal/activities"
	launchpadadapter "github.com/mclemenceau/watchtower/internal/adapters/launchpad"
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

	body := io.Reader(resp.Body)
	// Decompress gzip responses when Go's transport has not already done so.
	needsGzip := !resp.Uncompressed && (strings.HasSuffix(strings.ToLower(url), ".gz") ||
		resp.Header.Get("Content-Type") == "application/x-gzip" ||
		resp.Header.Get("Content-Type") == "application/gzip")
	if needsGzip {
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return "", fmt.Errorf("LogFetcher: gzip open: %w", err)
		}
		defer gr.Close() //nolint:errcheck
		body = gr
	}

	raw, err := io.ReadAll(body)
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
		fmt.Fprintf(os.Stderr, "config: %s\n", err.Error())
		os.Exit(1)
	}

	// Select notifier for proactive summaries (broadcast to all joined channels).
	// Falls back to stdout when Mattermost credentials are absent.
	var notifier ports.Notifier
	if cfg.MattermostServerURL != "" && cfg.MattermostBotToken != "" && cfg.MattermostBotUserID != "" {
		notifier = mattermostadapter.NewBroadcastNotifier(
			cfg.MattermostServerURL,
			cfg.MattermostBotToken,
			cfg.MattermostBotUserID,
		)
		log.Printf("mattermost bot: broadcast notifier active (%s)", cfg.MattermostServerURL)
	} else {
		notifier = &mattermostadapter.StdoutNotifier{}
		log.Print("mattermost bot: stdout simulation (set MATTERMOST_SERVER_URL, MATTERMOST_BOT_TOKEN, MATTERMOST_BOT_USER_ID for real Mattermost)")
	}

	artefactSrc := testobserver.NewHTTPArtefactSource(cfg.TestObserverURL)
	buildSrc := testobserver.NewHTTPBuildSource(cfg.TestObserverURL)
	snap := state.New("state/snapshot.json")
	failureState := state.NewFailureState("state/failures.json")
	logFetcher := &httpLogFetcher{client: &http.Client{Timeout: 30 * time.Second}}
	launchpadSrc := launchpadadapter.NewHTTPLaunchpadSource()

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

	// Connect to Temporal.
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
		Artefacts:          artefactSrc,
		Tests:              buildSrc,
		Snapshot:           snap,
		Failures:           failureState,
		Hook:               notifier,
		LogFetcher:         logFetcher,
		DefaultRelease:     cfg.DefaultRelease,
		SummaryForReleases: cfg.SummaryForReleases,
		SummaryForProducts: cfg.SummaryForProducts,
		LLM:                llmClient,
		MaxAnalysisPerRun:  cfg.MaxFailuresPerAnalysisRun,
	}

	// Register and start the Temporal worker in the background.
	w := worker.New(c, taskQueue, worker.Options{
		WorkerStopTimeout: 0,
	})
	w.RegisterWorkflow(watchtowerworkflow.DataRefreshWorkflow)
	w.RegisterWorkflow(watchtowerworkflow.SummaryPostWorkflow)
	w.RegisterWorkflow(watchtowerworkflow.FailureAnalysisWorkflow)
	w.RegisterActivity(act)

	go func() {
		if err := w.Run(worker.InterruptCh()); err != nil {
			log.Printf("temporal worker stopped: %v", err)
		}
	}()

	// Terminate any stale workflows whose types are no longer registered.
	terminateStaleWorkflows(c, "status-table", "query", "change-watch", "change-watch-init")

	// Start the cron workflows (idempotent — Temporal ignores if already running).
	startDataRefreshWorkflow(c, cfg.RefreshCronSchedule)
	startSummaryPostWorkflow(c, cfg.SummaryCronSchedule)
	startFailureAnalysisWorkflow(c, cfg.FailureAnalysisCronSchedule)

	// If the snapshot is empty (first boot), trigger an immediate fetch.
	triggerInitialFetch(c, snap)

	log.Printf("bot started on task queue %q (temporal: %s, refresh: %s, summary: %s, analysis: %s)",
		taskQueue, cfg.TemporalHost, cfg.RefreshCronSchedule, cfg.SummaryCronSchedule, cfg.FailureAnalysisCronSchedule)

	// triggerAnalysis starts a one-shot FailureAnalysisWorkflow in the background.
	// release may be empty to analyse all pending failures.
	triggerAnalysis := func(release string) error {
		id := "failure-analysis-ondemand"
		if release != "" {
			id = "failure-analysis-ondemand-" + release
		}
		_, err := c.ExecuteWorkflow(
			context.Background(),
			client.StartWorkflowOptions{ID: id, TaskQueue: taskQueue},
			watchtowerworkflow.FailureAnalysisWorkflow,
		)
		return err
	}

	// Start the Mattermost WebSocket bot in the background.
	botCtx, cancelBot := context.WithCancel(context.Background())
	defer cancelBot()
	go mattermostadapter.RunBot(
		botCtx,
		mattermostadapter.BotConfig{
			ServerURL:      cfg.MattermostServerURL,
			Token:          cfg.MattermostBotToken,
			BotUserID:      cfg.MattermostBotUserID,
			Keyword:        cfg.WatchtowerKeyword,
			ReconnectDelay: cfg.MattermostReconnectDelay,
		},
		snap,
		failureState,
		cfg.DefaultRelease,
		cfg.SummaryForProducts,
		cfg.SummaryForReleases,
		nil, // httpClient — bot.go uses its own default
		resolver,
		logFetcher,
		llmClient,
		launchpadSrc,
		triggerAnalysis,
	)

	// Run the interactive REPL — blocks until stdin is closed or Ctrl-D.
	mattermostadapter.RunREPL(
		context.Background(), os.Stdin, notifier,
		snap, failureState,
		cfg.DefaultRelease, cfg.SummaryForProducts, cfg.SummaryForReleases,
		cfg.WatchtowerKeyword, resolver, logFetcher, llmClient, launchpadSrc,
		triggerAnalysis,
	)
}

func startDataRefreshWorkflow(c client.Client, cronSchedule string) {
	_, err := c.ExecuteWorkflow(
		context.Background(),
		client.StartWorkflowOptions{
			ID:           "data-refresh",
			TaskQueue:    taskQueue,
			CronSchedule: cronSchedule,
		},
		watchtowerworkflow.DataRefreshWorkflow,
	)
	if err != nil {
		log.Printf("note: data-refresh cron start: %v", err)
	} else {
		log.Printf("data-refresh cron scheduled (%s)", cronSchedule)
	}
}

func startSummaryPostWorkflow(c client.Client, cronSchedule string) {
	_, err := c.ExecuteWorkflow(
		context.Background(),
		client.StartWorkflowOptions{
			ID:           "summary-post",
			TaskQueue:    taskQueue,
			CronSchedule: cronSchedule,
		},
		watchtowerworkflow.SummaryPostWorkflow,
	)
	if err != nil {
		log.Printf("note: summary-post cron start: %v", err)
	} else {
		log.Printf("summary-post cron scheduled (%s)", cronSchedule)
	}
}

func startFailureAnalysisWorkflow(c client.Client, cronSchedule string) {
	_, err := c.ExecuteWorkflow(
		context.Background(),
		client.StartWorkflowOptions{
			ID:           "failure-analysis",
			TaskQueue:    taskQueue,
			CronSchedule: cronSchedule,
		},
		watchtowerworkflow.FailureAnalysisWorkflow,
	)
	if err != nil {
		log.Printf("note: failure-analysis cron start: %v", err)
	} else {
		log.Printf("failure-analysis cron scheduled (%s)", cronSchedule)
	}
}

// terminateStaleWorkflows terminates workflows by ID that are no longer
// supported by this version of the bot.
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

// triggerInitialFetch runs one DataRefreshWorkflow synchronously if the snapshot
// is empty or contains no test execution data.
func triggerInitialFetch(c client.Client, snap *state.Snapshot) {
	artefacts, err := snap.Read()
	if err != nil {
		return
	}
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
			ID:        "data-refresh-init",
			TaskQueue: taskQueue,
		},
		watchtowerworkflow.DataRefreshWorkflow,
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
