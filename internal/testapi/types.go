// Package testapi is a compatibility shim — helpers have moved to internal/domain.
package testapi

import (
	"github.com/mclemenceau/watchtower/internal/domain"
)

// Re-export pure functions.
var (
	IsDisplayable   = domain.IsDisplayable
	ExecStatusEmoji = domain.ExecStatusEmoji
)
