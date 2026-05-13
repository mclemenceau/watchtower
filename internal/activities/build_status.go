package activities

import (
	"context"
	"fmt"

	"github.com/mclemenceau/argus/internal/buildapi"
	"github.com/mclemenceau/argus/internal/llm"
	"github.com/mclemenceau/argus/internal/state"
)

// Activities holds the dependencies injected at worker startup.
type Activities struct {
	Builds   buildapi.BuildClient
	Snapshot *state.Snapshot
	LLM      llm.LLMClient
	FeedURL  string // base URL of the HTTP server for SSE push
}

func (a *Activities) FetchBuildStatus(ctx context.Context) ([]buildapi.Image, error) {
	images, err := a.Builds.FetchBuilds(ctx)
	if err != nil {
		return nil, fmt.Errorf("FetchBuildStatus: %w", err)
	}
	return images, nil
}

func (a *Activities) LoadSnapshot(_ context.Context) ([]buildapi.Image, error) {
	images, err := a.Snapshot.Read()
	if err != nil {
		return nil, fmt.Errorf("LoadSnapshot: %w", err)
	}
	return images, nil
}

func (a *Activities) SaveSnapshot(_ context.Context, images []buildapi.Image) error {
	if err := a.Snapshot.Write(images); err != nil {
		return fmt.Errorf("SaveSnapshot: %w", err)
	}
	return nil
}
