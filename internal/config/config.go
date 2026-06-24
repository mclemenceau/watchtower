package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	DefaultRelease       string
	TestObserverURL      string
	TemporalHost         string
	MattermostWebhookURL string // empty = stdout simulation

	// Incoming Mattermost polling (all optional — polling is disabled when Token or ChannelID is empty)
	MattermostServerURL    string        // base URL of Mattermost server, e.g. https://chat.example.com
	MattermostToken        string        // personal access token
	MattermostChannelID    string        // channel to poll for incoming commands
	MattermostPollInterval time.Duration // how often to poll (default 15s)
	WatchtowerKeyword      string        // trigger keyword (default @watchtower)

	// TODO: re-add OpenRouterAPIKey + LLMModel when log analysis is implemented
}

func Load() (*Config, error) {
	// Load .env from the working directory if it exists.
	// Variables already set in the environment take precedence (don't overwrite).
	if err := loadDotEnv(".env"); err != nil {
		return nil, err
	}

	pollInterval, err := parseDurationEnv("MATTERMOST_POLL_INTERVAL", 15*time.Second)
	if err != nil {
		return nil, err
	}
	return &Config{
		DefaultRelease:         os.Getenv("DEFAULT_RELEASE"), // empty = auto-detect from data
		TestObserverURL:        envOrDefault("TEST_OBSERVER_URL", "https://tests-api.ubuntu.com"),
		TemporalHost:           envOrDefault("TEMPORAL_HOST", "localhost:7233"),
		MattermostWebhookURL:   os.Getenv("MATTERMOST_WEBHOOK_URL"), // empty = stdout simulation
		MattermostServerURL:    os.Getenv("MATTERMOST_SERVER_URL"),
		MattermostToken:        os.Getenv("MATTERMOST_TOKEN"),
		MattermostChannelID:    os.Getenv("MATTERMOST_CHANNEL_ID"),
		MattermostPollInterval: pollInterval,
		WatchtowerKeyword:      envOrDefault("WATCHTOWER_KEYWORD", "@watchtower"),
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
	defer f.Close()

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
