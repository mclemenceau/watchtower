// Package ports_test validates that all adapter types satisfy their port interfaces
// at compile time. These are compile-only assertions with no runtime overhead.
package ports_test

import (
	"github.com/mclemenceau/watchtower/internal/adapters/launchpad"
	mattermostadapter "github.com/mclemenceau/watchtower/internal/adapters/mattermost"
	"github.com/mclemenceau/watchtower/internal/adapters/openrouter"
	"github.com/mclemenceau/watchtower/internal/adapters/testobserver"
	"github.com/mclemenceau/watchtower/internal/ports"
	"github.com/mclemenceau/watchtower/internal/state"
)

// Notifier implementations.
var _ ports.Notifier = (*mattermostadapter.StdoutNotifier)(nil)
var _ ports.Notifier = (*mattermostadapter.ChannelNotifier)(nil)
var _ ports.Notifier = (*mattermostadapter.BroadcastNotifier)(nil)

// LLMClient implementations.
var _ ports.LLMClient = (*openrouter.OpenRouterClient)(nil)
var _ ports.LLMClient = (*openrouter.MockLLMClient)(nil)

// SnapshotStore implementation.
var _ ports.SnapshotStore = (*state.Snapshot)(nil)

// ArtefactSource and BuildSource implementations.
var _ ports.ArtefactSource = (*testobserver.HTTPArtefactSource)(nil)
var _ ports.BuildSource = (*testobserver.HTTPBuildSource)(nil)
var _ ports.ArtefactSource = (*testobserver.MockArtefactSource)(nil)
var _ ports.BuildSource = (*testobserver.MockBuildSource)(nil)

// LaunchpadSource implementations.
var _ ports.LaunchpadSource = (*launchpad.HTTPLaunchpadSource)(nil)
var _ ports.LaunchpadSource = (*launchpad.MockLaunchpadSource)(nil)
