package v1

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// API 定義 API 的資料結構。
type API struct {
	logger              *slog.Logger
	identity            service.IdentityFacade
	me                  service.MeFacade
	authz               service.AuthzFacade
	iam                 service.IAMFacade
	hr                  service.HRFacade
	attendance          service.AttendanceFacade
	platform            service.PlatformFacade
	workspace           service.WorkspaceFacade
	workflow            service.WorkflowFacade
	agent               service.AgentFacade
	notification        service.NotificationFacade
	audit               service.AuditFacade
	tokenResolver       TokenResolver
	telemetryService    string
	readinessChecks     map[string]ReadinessCheck
	corsAllowedOrigins  []string
	trustedProxies      []string
	rateLimiter         RateLimiter
	rateLimitFailClosed bool
	disableSwagger      bool
	metrics             *apiMetrics
}

// HandlerFunc 表示 handler func。
type HandlerFunc func(http.ResponseWriter, *http.Request, domain.RequestContext) error

// ReadinessCheck 表示就緒檢查 check。
type ReadinessCheck func(context.Context) error

// Options 定義選項的資料結構。
type Options struct {
	TokenResolver        TokenResolver
	TelemetryServiceName string
	ReadinessChecks      map[string]ReadinessCheck
	// CORSAllowedOrigins 啟用只接受精確來源比對的 CORS middleware。
	// 空值表示不輸出 CORS headers，維持既有行為。
	CORSAllowedOrigins []string
	// TrustedProxies 列出允許信任 forwarding headers 的 proxy CIDR/IP。
	// 空值表示解析 client IP 時不信任任何 proxy。
	TrustedProxies []string
	// RateLimiter 非 nil 時會啟用每個 client IP 的請求限流。
	RateLimiter RateLimiter
	// RateLimitFailClosed 為 true 時，限流後端錯誤會拒絕請求（503），而非放行。
	RateLimitFailClosed bool
	// DisableSwagger 為 true 時不註冊 /swagger 與 /openapi.yaml（production 預設關閉）。
	DisableSwagger bool
}

// New 建立 API v1 的主要物件。
func New(app *service.Service, logger *slog.Logger, options ...Options) *API {
	if logger == nil {
		logger = slog.Default()
	}
	cfg := Options{}
	if len(options) > 0 {
		cfg = options[0]
	}
	if cfg.TokenResolver == nil {
		cfg.TokenResolver = noTokenResolver{}
	}
	api := &API{
		logger:              logger,
		tokenResolver:       cfg.TokenResolver,
		telemetryService:    cfg.TelemetryServiceName,
		readinessChecks:     copyReadinessChecks(cfg.ReadinessChecks),
		corsAllowedOrigins:  cfg.CORSAllowedOrigins,
		trustedProxies:      cfg.TrustedProxies,
		rateLimiter:         cfg.RateLimiter,
		rateLimitFailClosed: cfg.RateLimitFailClosed,
		disableSwagger:      cfg.DisableSwagger,
		metrics:             newAPIMetrics(),
	}
	if app != nil {
		api.identity = app.Identity()
		api.me = app.Me()
		api.authz = app.Authz()
		api.iam = app.IAM()
		api.hr = app.HR()
		api.attendance = app.Attendance()
		api.platform = app.Platform()
		api.workspace = app.Workspace()
		api.workflow = app.Workflow()
		api.agent = app.Agent()
		api.notification = app.Notifications()
		api.audit = app.Audit()
	}
	return api
}

// Routes 處理路由。
func (a *API) Routes() http.Handler {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	// 預設不信任 proxy headers，因此 c.ClientIP() 會回傳 peer address。
	// 部署在 reverse proxy 或 load balancer 後方時，需設定信任的 proxy。
	// TRUSTED_PROXIES 以逗號分隔 CIDR/IP，供 logs 與限流使用正確 client IP。
	// 這樣才能安全地從 X-Forwarded-For 推導 client IP。
	if len(a.trustedProxies) > 0 {
		_ = router.SetTrustedProxies(a.trustedProxies)
	} else {
		_ = router.SetTrustedProxies(nil)
	}
	router.Use(a.recovery(), a.requestLogger(), a.metrics.middleware())
	if len(a.corsAllowedOrigins) > 0 {
		router.Use(corsMiddleware(a.corsAllowedOrigins))
	}
	if a.rateLimiter != nil {
		router.Use(a.rateLimit(a.rateLimiter))
	}
	if a.telemetryService != "" {
		router.Use(otelgin.Middleware(a.telemetryService))
	}

	a.RegisterRoutes(router)

	return router
}

// copyReadinessChecks 複製就緒檢查 checks。
func copyReadinessChecks(src map[string]ReadinessCheck) map[string]ReadinessCheck {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]ReadinessCheck, len(src))
	for name, check := range src {
		if check != nil {
			dst[name] = check
		}
	}
	return dst
}
