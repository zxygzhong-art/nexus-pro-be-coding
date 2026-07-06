package v1

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// apiMetrics 定義 API 指標的資料結構。
type apiMetrics struct {
	registry        *prometheus.Registry
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
}

// newAPIMetrics 建立 API 指標。
func newAPIMetrics() *apiMetrics {
	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	requestsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests by method, route template, and status code.",
	}, []string{"method", "path", "status"})
	requestDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency by method, route template, and status code.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path", "status"})
	registry.MustRegister(requestsTotal, requestDuration)
	return &apiMetrics{
		registry:        registry,
		requestsTotal:   requestsTotal,
		requestDuration: requestDuration,
	}
}

// middleware 處理 middleware。
func (m *apiMetrics) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		path := c.FullPath()
		if path == "" {
			// 未匹配路由的請求共用同一 label，避免洩漏原始 URL。
			path = "unmatched"
		}
		status := strconv.Itoa(c.Writer.Status())
		m.requestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		m.requestDuration.WithLabelValues(c.Request.Method, path, status).Observe(time.Since(start).Seconds())
	}
}

// handler 處理 handler。
func (m *apiMetrics) handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// MetricsHandler 處理指標 handler。
func (a *API) MetricsHandler() http.Handler {
	return a.metrics.handler()
}
