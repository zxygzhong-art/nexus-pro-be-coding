// Package identity resolves the caller's tenant + account from request inputs.
// The header provider (dev default) trusts X-Tenant-ID / X-Account-ID; the
// Keycloak stub validates a bearer JWT once enabled.
package identity

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned by stub providers.
var ErrNotImplemented = errors.New("identity: not implemented")

// ErrUnauthenticated indicates the request carries no usable identity.
var ErrUnauthenticated = errors.New("identity: unauthenticated")

// Headers carries the request inputs a provider may consult.
type Headers struct {
	TenantID    string // X-Tenant-ID
	AccountID   string // X-Account-ID
	BearerToken string // Authorization: Bearer <token>
}

// Identity is a resolved (but not yet authorized) caller.
type Identity struct {
	TenantID  string
	AccountID string
	Email     string
	Name      string
	Claims    map[string]any
}

// Provider resolves an Identity from request headers/token.
type Provider interface {
	Resolve(ctx context.Context, h Headers) (Identity, error)
}
