package identity

import (
	"context"
	"errors"
)

// Chain tries each provider in order, falling through on ErrNotImplemented /
// ErrUnauthenticated. This lets a bearer-token (Keycloak) provider take
// precedence while falling back to the header provider in development.
type Chain struct {
	providers []Provider
}

// NewChain builds a provider chain.
func NewChain(providers ...Provider) *Chain { return &Chain{providers: providers} }

// Resolve returns the first successful identity.
func (c *Chain) Resolve(ctx context.Context, h Headers) (Identity, error) {
	var lastErr error = ErrUnauthenticated
	for _, p := range c.providers {
		id, err := p.Resolve(ctx, h)
		if err == nil {
			return id, nil
		}
		if errors.Is(err, ErrNotImplemented) || errors.Is(err, ErrUnauthenticated) {
			lastErr = err
			continue
		}
		return Identity{}, err
	}
	return Identity{}, lastErr
}
