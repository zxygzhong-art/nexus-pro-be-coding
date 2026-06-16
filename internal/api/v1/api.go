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

type API struct {
	app                *service.Service
	logger             *slog.Logger
	allowDemoContext   bool
	allowHeaderContext bool
	tokenResolver      TokenResolver
	telemetryService   string
	readinessChecks    map[string]ReadinessCheck
}

type HandlerFunc func(http.ResponseWriter, *http.Request, domain.RequestContext) error
type ReadinessCheck func(context.Context) error

type Options struct {
	AllowDemoContext     bool
	AllowHeaderContext   bool
	AllowUnsignedJWT     bool
	TokenResolver        TokenResolver
	TelemetryServiceName string
	ReadinessChecks      map[string]ReadinessCheck
}

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
		if cfg.AllowUnsignedJWT {
			cfg.TokenResolver = unsignedJWTResolver{}
		}
	}
	return &API{
		app:                app,
		logger:             logger,
		allowDemoContext:   cfg.AllowDemoContext,
		allowHeaderContext: cfg.AllowHeaderContext,
		tokenResolver:      cfg.TokenResolver,
		telemetryService:   cfg.TelemetryServiceName,
		readinessChecks:    copyReadinessChecks(cfg.ReadinessChecks),
	}
}

func (a *API) Routes() http.Handler {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	_ = router.SetTrustedProxies(nil)
	router.Use(a.recovery(), a.requestLogger())
	if a.telemetryService != "" {
		router.Use(otelgin.Middleware(a.telemetryService))
	}

	a.RegisterRoutes(router)

	return router
}

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
