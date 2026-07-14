package v1

import (
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
)

// recovery 處理 recovery。
func (a *API) recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				traceID, spanID := traceContextIDs(c.Request)
				requestID := requestIDFrom(c.Request)
				if traceID == "" {
					traceID = requestID
				}
				a.logger.Error("request panic recovered",
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"trace_id", traceID,
					"span_id", spanID,
					"request_id", requestID,
					"tenant_id", stringValue(c, "tenant_id"),
					"account_id", stringValue(c, "account_id"),
					"panic", recovered,
					"stack", string(debug.Stack()),
				)
				c.Abort()
				if !c.Writer.Written() {
					writeJSON(c.Writer, http.StatusInternalServerError, map[string]any{
						"error": map[string]any{
							"code":     domain.ErrorCodeInternal,
							"message":  "internal server error",
							"trace_id": traceID,
						},
					})
				}
			}
		}()
		c.Next()
	}
}

// ginHandle 處理 gin handle。
func (a *API) ginHandle(resource, action string, next HandlerFunc, authz routeAuthz) gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, param := range authz.pathParams {
			c.Request.SetPathValue(param, c.Param(param))
		}
		ctx, err := a.requestContext(c.Request)
		if err != nil {
			a.writeError(c.Writer, c.Request, err)
			return
		}
		ctx = requestContextWithRoute(ctx, c.Request.Method, c.FullPath(), resource, action)
		if err := a.rejectCrossTenantRequest(ctx, c.Request); err != nil {
			a.writeError(c.Writer, c.Request, err)
			return
		}
		c.Set("tenant_id", ctx.TenantID)
		c.Set("account_id", ctx.AccountID)
		c.Set("trace_id", ctx.TraceID)
		c.Set("span_id", ctx.SpanID)
		c.Writer.Header().Set("X-Request-ID", ctx.RequestID)
		if err := a.authorize(ctx, c.Request, c.FullPath(), resource, action, authz); err != nil {
			a.writeError(c.Writer, c.Request, err)
			return
		}
		if err := next(c.Writer, c.Request, ctx); err != nil {
			a.writeError(c.Writer, c.Request, businessRouteError(resource, err))
			return
		}
	}
}

// businessRouteError assigns module fallbacks only to expected application errors.
func businessRouteError(resource string, err error) error {
	appErr, ok := domain.AsAppError(err)
	if !ok {
		return err
	}
	if !isGenericApplicationError(appErr) {
		return err
	}
	code := routeFallbackErrorCode(resource, appErr.Code)
	if code == 0 {
		return err
	}
	return appErr.WithPublicCode(code)
}

// isGenericApplicationError prevents route fallbacks from overwriting a specific public code.
func isGenericApplicationError(appErr *domain.AppError) bool {
	switch appErr.Code {
	case "bad_request":
		return appErr.PublicCode == domain.ErrorCodeBadRequest
	case "validation_failed":
		return appErr.PublicCode == domain.ErrorCodeValidationFailed
	case "not_found":
		return appErr.PublicCode == domain.ErrorCodeNotFound
	case "conflict":
		return appErr.PublicCode == domain.ErrorCodeConflict
	default:
		return false
	}
}

// routeFallbackErrorCode keeps generic bad-request/not-found/conflict errors inside their owning module range.
func routeFallbackErrorCode(resource, kind string) domain.ErrorCode {
	var badRequest, notFound, conflict domain.ErrorCode
	switch {
	case strings.HasPrefix(resource, "attendance."):
		badRequest, notFound, conflict = domain.ErrorCodeAttendanceBadRequest, domain.ErrorCodeAttendanceNotFound, domain.ErrorCodeAttendanceConflict
	case strings.HasPrefix(resource, "workflow."):
		badRequest, notFound, conflict = domain.ErrorCodeWorkflowBadRequest, domain.ErrorCodeWorkflowNotFound, domain.ErrorCodeWorkflowConflict
	case strings.HasPrefix(resource, "agent."):
		badRequest, notFound, conflict = domain.ErrorCodeAgentBadRequest, domain.ErrorCodeAgentNotFound, domain.ErrorCodeAgentConflict
	default:
		return 0
	}
	switch kind {
	case "bad_request", "validation_failed":
		return badRequest
	case "not_found":
		return notFound
	case "conflict":
		return conflict
	default:
		return 0
	}
}

// requestContextWithRoute 附加路由政策元資料供後續稽核 details 使用。
func requestContextWithRoute(ctx domain.RequestContext, method, path, resource, action string) domain.RequestContext {
	for _, policy := range domain.DefaultRoutePolicies {
		if strings.EqualFold(policy.Method, method) && policy.Path == path {
			ctx.RouteApplicationCode = policy.ApplicationCode
			ctx.RouteResourceType = policy.ResourceType
			ctx.RouteAction = policy.Action
			ctx.RoutePath = policy.Path
			return ctx
		}
	}
	app, resourceType := routeResourceParts(resource)
	ctx.RouteApplicationCode = app
	ctx.RouteResourceType = resourceType
	ctx.RouteAction = strings.TrimSpace(action)
	ctx.RoutePath = strings.TrimSpace(path)
	return ctx
}

// rejectCrossTenantRequest 拒絕顯式目標租戶與 token 租戶不一致的請求並寫安全稽核。
func (a *API) rejectCrossTenantRequest(ctx domain.RequestContext, r *http.Request) error {
	targetTenantID := requestTargetTenantID(r)
	if targetTenantID == "" || targetTenantID == ctx.TenantID {
		return nil
	}
	details := map[string]any{
		"result":           "denied",
		"reason_code":      "cross_tenant_denied",
		"token_tenant_id":  ctx.TenantID,
		"target_tenant_id": targetTenantID,
		"method":           r.Method,
		"path":             r.URL.Path,
	}
	if a.audit != nil {
		_ = a.audit.RecordSecurityEvent(ctx, "security.cross_tenant.denied", "tenant", targetTenantID, details)
	}
	return domain.ForbiddenReason("cross_tenant_denied", "cross-tenant access denied")
}

// requestTargetTenantID 解析相容入口中顯式宣告的目標租戶。
func requestTargetTenantID(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("tenant_id"))
}

// routeResourceParts 拆分 route binder 的 resource 字串。
func routeResourceParts(resource string) (string, string) {
	resource = strings.TrimSpace(resource)
	if resource == "" {
		return "platform", ""
	}
	parts := strings.SplitN(resource, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "platform", resource
}

// requestLogger 處理請求 logger。
func (a *API) requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		requestID := requestIDFrom(c.Request)
		if requestID == "" {
			requestID = newRequestID()
			c.Request.Header.Set("X-Request-ID", requestID)
		}
		c.Writer.Header().Set("X-Request-ID", requestID)
		c.Next()
		traceID, spanID := traceContextIDs(c.Request)
		if traceID == "" {
			traceID = requestID
		}
		a.logger.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"elapsed_ms", time.Since(start).Milliseconds(),
			"trace_id", traceID,
			"span_id", spanID,
			"request_id", requestID,
			"tenant_id", stringValue(c, "tenant_id"),
			"account_id", stringValue(c, "account_id"),
			"client_ip", c.ClientIP(),
		)
	}
}
