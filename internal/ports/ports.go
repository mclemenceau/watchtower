package ports

import (
	"context"

	"github.com/mclemenceau/watchtower/internal/domain"
)

// ArtefactSource fetches Ubuntu image artefacts from a data source.
type ArtefactSource interface {
	FetchArtefacts(ctx context.Context) ([]domain.Artefact, error)
}

// BuildSource fetches test execution data for a given artefact.
type BuildSource interface {
	FetchBuilds(ctx context.Context, artefactID int) ([]domain.ArtefactBuild, error)
}

// Notifier sends messages to a communication channel (e.g. Mattermost, stdout).
type Notifier interface {
	Send(text string) error
}

// SnapshotStore persists and retrieves the artefact snapshot.
type SnapshotStore interface {
	Read() ([]domain.Artefact, error)
	Write(artefacts []domain.Artefact) error
}

// LLMClient calls a large language model for text completion.
type LLMClient interface {
	Complete(ctx context.Context, system, prompt string) (string, error)
}

// LogFetcher retrieves build log content from a URL.
type LogFetcher interface {
	Fetch(ctx context.Context, url string) (string, error)
}

// LaunchpadSource resolves Launchpad livefs build information.
type LaunchpadSource interface {
	// FetchBuildLogURL returns the URL of the build log artifact (build_log_url)
	// for a given Launchpad livefs build page URL
	// (e.g. https://launchpad.net/~ubuntu-cdimage/+livefs/ubuntu/stonking/ubuntu/+build/N).
	// Returns ("", nil) when the build has no log yet (build in progress or log unavailable).
	FetchBuildLogURL(ctx context.Context, buildPageURL string) (string, error)
}
