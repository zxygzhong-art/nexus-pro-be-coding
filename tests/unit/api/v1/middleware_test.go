package v1_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1api "nexus-pro-be/internal/api/v1"
	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

// newMiddlewareTestAPI 驗證 middleware test API。
func newMiddlewareTestAPI(options v1api.Options) http.Handler {
	return v1api.New(service.New(memory.NewStore()), nil, options).Routes()
}

// TestCORSHeadersAbsentWhenNotConfigured 驗證 CORS headers absent when not configured。
func TestCORSHeadersAbsentWhenNotConfigured(t *testing.T) {
	handler := newMiddlewareTestAPI(v1api.Options{})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for healthz, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no CORS headers without configuration, got %q", got)
	}
}

// TestCORSAllowsExactlyMatchedOrigin 驗證 CORS allows exactly matched origin。
func TestCORSAllowsExactlyMatchedOrigin(t *testing.T) {
	handler := newMiddlewareTestAPI(v1api.Options{
		CORSAllowedOrigins: []string{"https://app.example.com", "https://admin.example.com"},
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for healthz, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("expected allowed origin header, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected credentials header, got %q", got)
	}
	if !strings.Contains(rec.Header().Get("Vary"), "Origin") {
		t.Fatalf("expected Vary: Origin, got %q", rec.Header().Get("Vary"))
	}
}

// TestCORSRejectsUnlistedAndPrefixedOrigins 驗證 CORS rejects unlisted and prefixed origins。
func TestCORSRejectsUnlistedAndPrefixedOrigins(t *testing.T) {
	handler := newMiddlewareTestAPI(v1api.Options{
		CORSAllowedOrigins: []string{"https://app.example.com"},
	})

	for _, origin := range []string{"https://evil.example.com", "https://app.example.com.evil.com", "http://app.example.com"} {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("expected origin %q to be rejected, got allow header %q", origin, got)
		}
	}
}

// TestCORSPreflightReturnsNoContent 驗證 CORS preflight returns no content。
func TestCORSPreflightReturnsNoContent(t *testing.T) {
	handler := newMiddlewareTestAPI(v1api.Options{
		CORSAllowedOrigins: []string{"https://app.example.com"},
	})

	req := httptest.NewRequest(http.MethodOptions, "/v1/hr/employees", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for preflight, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("expected allowed origin on preflight, got %q", got)
	}
	if !strings.Contains(rec.Header().Get("Access-Control-Allow-Methods"), http.MethodPost) {
		t.Fatalf("expected allowed methods on preflight, got %q", rec.Header().Get("Access-Control-Allow-Methods"))
	}
	if rec.Header().Get("Access-Control-Allow-Headers") == "" {
		t.Fatal("expected allowed headers on preflight")
	}
}

// TestRateLimitReturns429WithErrorEnvelope 驗證速率限制 returns 429 with 錯誤 envelope。
func TestRateLimitReturns429WithErrorEnvelope(t *testing.T) {
	handler := newMiddlewareTestAPI(v1api.Options{
		RateLimiter: v1api.NewLocalRateLimiter(1, 2),
	})

	var lastCode int
	var lastBody []byte
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.RemoteAddr = "203.0.113.10:4000"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		lastCode = rec.Code
		lastBody = rec.Body.Bytes()
	}

	if lastCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after exceeding burst, got %d: %s", lastCode, lastBody)
	}
	payload := decodeError(t, lastBody)
	if payload.Code != domain.ErrorCodeTooManyRequests {
		t.Fatalf("expected error code %d, got %+v", domain.ErrorCodeTooManyRequests, payload)
	}

	// 不同 client IP 會保有自己的 bucket。
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.RemoteAddr = "203.0.113.11:4000"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected fresh client to pass, got %d", rec.Code)
	}
}

// TestLocalRateLimiterRejectsNewKeysAtCapacity 驗證本機速率限流器 rejects new keys at capacity。
func TestLocalRateLimiterRejectsNewKeysAtCapacity(t *testing.T) {
	limiter := v1api.NewLocalRateLimiter(100000, 100000)
	for i := 0; i < 10000; i++ {
		allowed, err := limiter.Allow(context.Background(), fmt.Sprintf("client-%05d", i))
		if err != nil {
			t.Fatalf("unexpected limiter error: %v", err)
		}
		if !allowed {
			t.Fatalf("expected warmup client %d to pass", i)
		}
	}

	allowed, err := limiter.Allow(context.Background(), "client-over-cap")
	if err != nil {
		t.Fatalf("unexpected limiter error: %v", err)
	}
	if allowed {
		t.Fatal("expected a new client over the in-process capacity to be rejected")
	}
}

type failingRateLimiter struct{}

// Allow 驗證目標路徑。
func (failingRateLimiter) Allow(context.Context, string) (bool, error) {
	return false, errors.New("redis unavailable")
}

// TestRateLimitFailsOpenWhenLimiterErrors 驗證速率限制 fails open when 限流器錯誤。
func TestRateLimitFailsOpenWhenLimiterErrors(t *testing.T) {
	handler := newMiddlewareTestAPI(v1api.Options{RateLimiter: failingRateLimiter{}})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected limiter failure to fail open, got %d", rec.Code)
	}
}

// TestRateLimitFailsClosedWhenConfigured 驗證 RATE_LIMIT_FAIL_CLOSED 時限流器錯誤拒絕請求。
func TestRateLimitFailsClosedWhenConfigured(t *testing.T) {
	handler := newMiddlewareTestAPI(v1api.Options{
		RateLimiter:         failingRateLimiter{},
		RateLimitFailClosed: true,
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected limiter failure to fail closed with 503, got %d", rec.Code)
	}
}

// TestMetricsEndpointExposesRequestMetrics 驗證指標 endpoint exposes 請求指標。
func TestMetricsEndpointExposesRequestMetrics(t *testing.T) {
	api := v1api.New(service.New(memory.NewStore()), nil, v1api.Options{})
	handler := api.Routes()

	warm := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	warmRec := httptest.NewRecorder()
	handler.ServeHTTP(warmRec, warm)
	if warmRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for healthz, got %d", warmRec.Code)
	}

	// scrape endpoint 已從業務 router 移到專用 listener。
	businessReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	businessRec := httptest.NewRecorder()
	handler.ServeHTTP(businessRec, businessReq)
	if businessRec.Code != http.StatusNotFound {
		t.Fatalf("expected business router not to serve /metrics, got %d", businessRec.Code)
	}

	// 獨立 handler 仍會暴露業務 router 收集到的 metrics。
	rec := httptest.NewRecorder()
	api.MetricsHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for metrics, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `http_requests_total{method="GET",path="/healthz",status="200"}`) {
		t.Fatalf("expected request counter with route template labels, got:\n%s", truncateForLog(body))
	}
	if !strings.Contains(body, "http_request_duration_seconds_bucket") {
		t.Fatalf("expected latency histogram, got:\n%s", truncateForLog(body))
	}
}

// truncateForLog 驗證 for log。
func truncateForLog(body string) string {
	if len(body) > 2000 {
		return body[:2000] + "..."
	}
	return body
}
