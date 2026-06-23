package v1

import (
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
)

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
		c.Set("tenant_id", ctx.TenantID)
		c.Set("account_id", ctx.AccountID)
		traceID, spanID := traceContextIDs(c.Request)
		if traceID == "" {
			traceID = ctx.RequestID
		}
		c.Set("trace_id", traceID)
		c.Set("span_id", spanID)
		c.Writer.Header().Set("X-Request-ID", ctx.RequestID)
		if err := a.authorize(ctx, c.Request, resource, action, authz); err != nil {
			a.writeError(c.Writer, c.Request, err)
			return
		}
		if err := next(c.Writer, c.Request, ctx); err != nil {
			a.writeError(c.Writer, c.Request, err)
			return
		}
	}
}

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
