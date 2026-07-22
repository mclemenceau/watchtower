package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	DefaultRelease     string
	SummaryForReleases []string // ordered list of releases to include in the scheduled summary; empty = all
	SummaryForProducts []string // restrict summaries to these OS/product names; empty = all
	TestObserverURL    string
	TemporalHost       string

	// Mattermost bot credentials (all required for the bot to connect)
	MattermostServerURL string // base URL of Mattermost server, e.g. http://192.168.1.193:8065
	MattermostBotToken  string // bot token from System Console → Integrations → Bot Accounts
	MattermostBotUserID string // user ID of the bot account (used to suppress self-echoes)

	// Bot behaviour
	WatchtowerKeyword        string        // trigger keyword (default @watchtower)
	MattermostReconnectDelay time.Duration // WebSocket reconnect delay (default 5s)

	// LLM-assisted intent resolution (optional — feature is disabled when OpenRouterAPIKey is empty)
	OpenRouterAPIKey string
	LLMModel         string

	// Cron schedules for the two background workflows
	RefreshCronSchedule string // dataset refresh interval (default every 30 min)
	SummaryCronSchedule string // when to post the scheduled build summary (default 07:00/15:00/23:00 UTC)
}

func Load() (*Config, error) {
	// Load .env from the working directory if it exists.
	// Variables already set in the environment take precedence (don't overwrite).
	if err := loadDotEnv(".env"); err != nil {
		return nil, err
	}

	reconnectDelay, err := parseDurationEnv("MATTERMOST_RECONNECT_DELAY", 5*time.Second)
	if err != nil {
		return nil, err
	}
	return &Config{
		DefaultRelease:           os.Getenv("DEFAULT_RELEASE"), // empty = auto-detect from data
		SummaryForReleases:       parseSummaryList(os.Getenv("SUMMARY_FOR_RELEASES")),
		SummaryForProducts:       parseSummaryList(os.Getenv("SUMMARY_FOR_PRODUCTS")),
		TestObserverURL:          envOrDefault("TEST_OBSERVER_URL", "https://tests-api.ubuntu.com"),
		TemporalHost:             envOrDefault("TEMPORAL_HOST", "localhost:7233"),
		MattermostServerURL:      os.Getenv("MATTERMOST_SERVER_URL"),
		MattermostBotToken:       os.Getenv("MATTERMOST_BOT_TOKEN"),
		MattermostBotUserID:      os.Getenv("MATTERMOST_BOT_USER_ID"),
		WatchtowerKeyword:        envOrDefault("WATCHTOWER_KEYWORD", "@watchtower"),
		MattermostReconnectDelay: reconnectDelay,
		OpenRouterAPIKey:         os.Getenv("OPENROUTER_API_KEY"),
		LLMModel:                 envOrDefault("LLM_MODEL", "openai/gpt-4o-mini"),
		RefreshCronSchedule:      envOrDefault("REFRESH_CRON_SCHEDULE", "*/30 * * * *"),
		SummaryCronSchedule:      envOrDefault("SUMMARY_CRON_SCHEDULE", "0 7,15,23 * * *"),
	}, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// parseDurationEnv reads key as a time.Duration string (e.g. "30s", "1m").
// Returns def if the variable is unset or empty, and an error if the value is
// set but cannot be parsed.
func parseDurationEnv(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("config: %s=%q is not a valid duration: %w", key, v, err)
	}
	return d, nil
}

// parseSummaryList splits a comma-separated value into a slice of trimmed,
// non-empty strings. Returns nil (= include all) when val is empty.
func parseSummaryList(val string) []string {
	if val == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(val, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// loadDotEnv reads a .env file and sets any variable that is not already present
// in the environment. Lines beginning with '#' and blank lines are ignored.
// The file is optional — a missing file is silently skipped.
// Format: KEY=VALUE (no export keyword, no quoting required, inline comments not supported).
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // .env is optional
		}
		return fmt.Errorf("config: open %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			return fmt.Errorf("config: %s line %d: expected KEY=VALUE, got %q", path, lineNum, line)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return fmt.Errorf("config: %s line %d: empty key", path, lineNum)
		}
		// Only set if not already in the environment — shell wins.
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("config: setenv %s: %w", key, err)
			}
		}
	}
	return scanner.Err()
}
