// Package authorizer defines the authorization seam. The local engine
// (internal/authz.LocalEngine) implements it today; an OpenFGA-backed
// implementation can be swapped in via configuration without touching callers.
package authorizer

import (
	"context"
	"errors"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/authz"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/models"
)

// ErrNotImplemented is returned by stub backends.
var ErrNotImplemented = errors.New("authorizer: not implemented")

// Authorizer evaluates authorization decisions.
type Authorizer interface {
	Check(ctx context.Context, req authz.Request) (authz.Decision, error)
	BatchCheck(ctx context.Context, reqs []authz.Request) ([]authz.Decision, error)
	PruneMenus(ctx context.Context, applicationCode string) ([]models.MenuItem, error)
}
