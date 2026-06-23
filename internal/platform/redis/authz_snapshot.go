package redis

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"nexus-pro-be/internal/domain"
)

// AuthzSnapshotStore stores authorization decisions in Redis.
type AuthzSnapshotStore struct {
	client *goredis.Client
}

// NewAuthzSnapshotStore creates a Redis-backed authorization snapshot cache.
func NewAuthzSnapshotStore(client *goredis.Client) *AuthzSnapshotStore {
	return &AuthzSnapshotStore{client: client}
}

// GetAuthzSnapshot returns a cached authorization decision when present.
func (s *AuthzSnapshotStore) GetAuthzSnapshot(ctx context.Context, key string) (domain.CheckResult, bool, error) {
	if s == nil || s.client == nil {
		return domain.CheckResult{}, false, nil
	}
	raw, err := s.client.Get(ctx, key).Bytes()
	if err == goredis.Nil {
		return domain.CheckResult{}, false, nil
	}
	if err != nil {
		return domain.CheckResult{}, false, err
	}
	var result domain.CheckResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return domain.CheckResult{}, false, err
	}
	return result, true, nil
}

// SetAuthzSnapshot caches an authorization decision for the supplied TTL.
func (s *AuthzSnapshotStore) SetAuthzSnapshot(ctx context.Context, key string, result domain.CheckResult, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return nil
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, key, raw, ttl).Err()
}

// InvalidateTenant removes all cached authorization decisions for a tenant.
func (s *AuthzSnapshotStore) InvalidateTenant(ctx context.Context, tenantID string) error {
	if s == nil || s.client == nil || strings.TrimSpace(tenantID) == "" {
		return nil
	}
	pattern := "authz:snapshot:" + tenantID + ":*"
	var cursor uint64
	for {
		keys, next, err := s.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := s.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		if next == 0 {
			return nil
		}
		cursor = next
	}
}
