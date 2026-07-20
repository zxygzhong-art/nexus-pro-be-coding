package v1

import (
	"context"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"nexus-pro-api/internal/domain"
)

// RateLimiter 定義速率限流器的行為契約。
type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
}

// rateLimit 處理速率限制。
func (a *API) rateLimit(limiter RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		allowed, err := limiter.Allow(c.Request.Context(), c.ClientIP())
		if err != nil {
			if a.rateLimitFailClosed {
				a.logger.Warn("rate limiter unavailable, rejecting request", "client_ip", c.ClientIP(), "error", err)
				c.Abort()
				a.writeError(c.Writer, c.Request, domain.E(503, "rate_limiter_unavailable", "request rate limiter is temporarily unavailable"))
				return
			}
			// 預設 fail open，避免限流後端故障拖垮 API。
			a.logger.Warn("rate limiter unavailable, allowing request", "client_ip", c.ClientIP(), "error", err)
			c.Next()
			return
		}
		if !allowed {
			c.Abort()
			a.writeError(c.Writer, c.Request, domain.TooManyRequests("request rate limit exceeded"))
			return
		}
		c.Next()
	}
}

const (
	localRateLimiterMaxEntries = 10000
	localRateLimiterIdleTTL    = 3 * time.Minute
)

// LocalRateLimiter 定義本機速率限流器的資料結構。
type LocalRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*localRateLimiterEntry
	rps      rate.Limit
	burst    int
	now      func() time.Time
}

type localRateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewLocalRateLimiter 建立本機速率限流器。
func NewLocalRateLimiter(rps, burst int) *LocalRateLimiter {
	if rps < 1 {
		rps = 1
	}
	if burst < 1 {
		burst = rps
	}
	return &LocalRateLimiter{
		limiters: map[string]*localRateLimiterEntry{},
		rps:      rate.Limit(rps),
		burst:    burst,
		now:      time.Now,
	}
}

// Allow 判斷是否允許目前流程。
func (l *LocalRateLimiter) Allow(_ context.Context, key string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	entry, ok := l.limiters[key]
	if !ok {
		if len(l.limiters) >= localRateLimiterMaxEntries {
			l.pruneLocked(now)
		}
		if len(l.limiters) >= localRateLimiterMaxEntries {
			return false, nil
		}
		entry = &localRateLimiterEntry{limiter: rate.NewLimiter(l.rps, l.burst)}
		l.limiters[key] = entry
	}
	entry.lastSeen = now
	return entry.limiter.AllowN(now, 1), nil
}

// pruneLocked 處理 prune locked。
func (l *LocalRateLimiter) pruneLocked(now time.Time) {
	for key, entry := range l.limiters {
		if now.Sub(entry.lastSeen) > localRateLimiterIdleTTL {
			delete(l.limiters, key)
		}
	}
}
