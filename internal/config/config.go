package config

import (
	"os"
)

type Config struct {
	DefaultRelease       string
	TestObserverURL      string
	TemporalHost         string
	MattermostWebhookURL string // empty = stdout simulation
	// TODO: re-add OpenRouterAPIKey + LLMModel when log analysis is implemented
}

func Load() (*Config, error) {
	return &Config{
		DefaultRelease:       os.Getenv("DEFAULT_RELEASE"), // empty = auto-detect from data
		TestObserverURL:      envOrDefault("TEST_OBSERVER_URL", "https://tests-api.ubuntu.com"),
		TemporalHost:         envOrDefault("TEMPORAL_HOST", "localhost:7233"),
		MattermostWebhookURL: os.Getenv("MATTERMOST_WEBHOOK_URL"), // empty = stdout simulation
	}, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
