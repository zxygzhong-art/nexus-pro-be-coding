package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache stores permission snapshots in Redis. It is wired in when REDIS_URL
// is set; otherwise NoopCache is used. Snapshots are opaque bytes (the engine
// JSON-encodes them), keyed per §15.
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache builds a RedisCache from a redis URL (redis://host:port/db).
func NewRedisCache(url string) (*RedisCache, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	return &RedisCache{client: redis.NewClient(opt)}, nil
}

func (c *RedisCache) GetSnapshot(ctx context.Context, key string) (Snapshot, bool, error) {
	b, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

func (c *RedisCache) SetSnapshot(ctx context.Context, key string, s Snapshot, ttl time.Duration) error {
	return c.client.Set(ctx, key, []byte(s), ttl).Err()
}

func (c *RedisCache) Invalidate(ctx context.Context, prefix string) error {
	// Snapshots expire via TTL and the version-bumped key; explicit scan-based
	// invalidation is intentionally omitted here.
	return nil
}
