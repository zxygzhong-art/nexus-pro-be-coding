package v1

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// HealthCtrl 定義健康檢查 ctrl 的資料結構。
type HealthCtrl struct {
	readinessChecks map[string]ReadinessCheck
}

// RegisterRoutes 註冊此 controller 的 HTTP 路由。
func (c HealthCtrl) RegisterRoutes(router *gin.Engine) {
	router.GET("/healthz", func(ginCtx *gin.Context) { c.health(ginCtx.Writer, ginCtx.Request) })
	router.GET("/readyz", func(ginCtx *gin.Context) { c.ready(ginCtx.Writer, ginCtx.Request) })
}

// health 處理健康檢查的 HTTP 請求。
func (c HealthCtrl) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// ready 處理就緒檢查的 HTTP 請求。
func (c HealthCtrl) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	status := "ok"
	code := http.StatusOK
	checks := map[string]string{}
	if len(c.readinessChecks) == 0 {
		checks["application"] = "ok"
	}
	for name, check := range c.readinessChecks {
		if err := check(ctx); err != nil {
			status = "degraded"
			code = http.StatusServiceUnavailable
			checks[name] = "error"
			continue
		}
		checks[name] = "ok"
	}
	writeJSON(w, code, map[string]any{
		"status": status,
		"checks": checks,
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}
