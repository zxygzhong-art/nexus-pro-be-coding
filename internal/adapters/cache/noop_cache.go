package cache

import (
	"context"
	"time"
)

// NoopCache always misses; correctness does not depend on caching.
type NoopCache struct{}

// NewNoopCache builds a no-op cache.
func NewNoopCache() *NoopCache { return &NoopCache{} }

func (*NoopCache) GetSnapshot(context.Context, string) (Snapshot, bool, error) {
	return nil, false, nil
}

func (*NoopCache) SetSnapshot(context.Context, string, Snapshot, time.Duration) error {
	return nil
}

func (*NoopCache) Invalidate(context.Context, string) error { return nil }
