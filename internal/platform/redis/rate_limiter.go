package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const rateLimiterKeyPrefix = "ratelimit"

// FixedWindowRateLimiter 定義 fixed window 速率限流器的資料結構。
type FixedWindowRateLimiter struct {
	client *goredis.Client
	limit  int64
	window time.Duration
	now    func() time.Time
}

// NewFixedWindowRateLimiter 建立 fixed window 速率限流器。
func NewFixedWindowRateLimiter(client *goredis.Client, rps, burst int) *FixedWindowRateLimiter {
	if rps < 1 {
		rps = 1
	}
	if burst < rps {
		burst = rps
	}
	windowSeconds := (burst + rps - 1) / rps
	return &FixedWindowRateLimiter{
		client: client,
		limit:  int64(rps * windowSeconds),
		window: time.Duration(windowSeconds) * time.Second,
		now:    time.Now,
	}
}

// Allow 判斷是否允許目前流程。
func (l *FixedWindowRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	windowStart := l.now().UTC().Truncate(l.window).Unix()
	redisKey := fmt.Sprintf("%s:%s:%d", rateLimiterKeyPrefix, key, windowStart)
	pipe := l.client.TxPipeline()
	count := pipe.Incr(ctx, redisKey)
	// 保留一個額外視窗，避免慢讀取者看到尚未過期的 key。
	pipe.Expire(ctx, redisKey, 2*l.window)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, fmt.Errorf("rate limiter incr: %w", err)
	}
	return count.Val() <= l.limit, nil
}
