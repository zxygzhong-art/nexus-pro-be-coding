// Package cache abstracts the permission-snapshot cache. NoopCache is the default;
// RedisCache is a scaffolded adapter activated when REDIS_URL is set.
package cache

import (
	"context"
	"time"
)

// Snapshot is an opaque cached permission snapshot (JSON-encoded by the adapter).
type Snapshot []byte

// Cache stores per-principal permission snapshots, keyed per §15
// (iam:snapshot:{tenant}:{account}:{app}:{version}). Invalidation is implicit via
// the tenant permission_version embedded in the key.
type Cache interface {
	GetSnapshot(ctx context.Context, key string) (Snapshot, bool, error)
	SetSnapshot(ctx context.Context, key string, s Snapshot, ttl time.Duration) error
	Invalidate(ctx context.Context, prefix string) error
}
