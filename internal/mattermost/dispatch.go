// Package mattermost provides the Mattermost I/O adapters for Watchtower.
// Command routing and formatting have moved to internal/application/.
package mattermost

import (
	"context"

	"github.com/mclemenceau/watchtower/internal/application"
	"github.com/mclemenceau/watchtower/internal/domain"
	"github.com/mclemenceau/watchtower/internal/intent"
)

// Dispatch is a backwards-compatibility wrapper around application.Dispatch.
// New callers should use application.Dispatch directly.
//
// Deprecated: use application.Dispatch.
func Dispatch(ctx context.Context, sessionID, msg string, artefacts []domain.Artefact, defaultRelease string, hook WebhookClient, keyword string, resolver *intent.Resolver) error {
	return application.Dispatch(ctx, sessionID, msg, artefacts, defaultRelease, hook, keyword, resolver)
}
