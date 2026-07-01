// Package buildapi is a compatibility shim — all types have moved to internal/domain.
// This package re-exports the domain types as aliases so callers can be migrated incrementally.
package buildapi

import (
	"github.com/mclemenceau/watchtower/internal/domain"
)

// Re-export domain types so existing code compiles unchanged.
type Artefact = domain.Artefact
type ArtefactBuild = domain.ArtefactBuild
type TestExecution = domain.TestExecution
type Environment = domain.Environment
type ChangeReport = domain.ChangeReport
type ArtefactDelta = domain.ArtefactDelta

// Re-export pure functions.
var (
	IsBuiltToday       = domain.IsBuiltToday
	BuildStatus        = domain.BuildStatus
	LogCell            = domain.LogCell
	LogURLFromImageURL = domain.LogURLFromImageURL
	ImageAge           = domain.ImageAge
)
