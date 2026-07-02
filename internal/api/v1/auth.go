package v1

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// AuthCtrl wires public authentication endpoints.
type AuthCtrl struct {
	api *API
	svc service.AuthnFacade
}

// RegisterRoutes attaches public OIDC login routes.
func (c AuthCtrl) RegisterRoutes(router *gin.RouterGroup) {
	auth := router.Group("/auth/oidc")
	auth.GET("/:provider/authorize", c.authorizeOIDC)
	auth.GET("/:provider/callback", c.completeOIDCCallback)
}

func (c AuthCtrl) authorizeOIDC(ctx *gin.Context) {
	if c.svc == nil {
		writeJSON(ctx.Writer, http.StatusServiceUnavailable, map[string]any{"error": map[string]any{"code": domain.ErrorCodeInternal, "message": "authentication service is not configured"}})
		return
	}
	provider := strings.TrimSpace(ctx.Param("provider"))
	input := domain.OIDCAuthorizationInput{
		TenantID:  strings.TrimSpace(ctx.Query("tenant_id")),
		ReturnURL: strings.TrimSpace(ctx.Query("return_url")),
	}
	result, err := c.svc.OIDCAuthorizationURL(ctx.Request.Context(), provider, input)
	if err != nil {
		c.api.writeError(ctx.Writer, ctx.Request, err)
		return
	}
	writeJSON(ctx.Writer, http.StatusOK, result)
}

func (c AuthCtrl) completeOIDCCallback(ctx *gin.Context) {
	if c.svc == nil {
		writeJSON(ctx.Writer, http.StatusServiceUnavailable, map[string]any{"error": map[string]any{"code": domain.ErrorCodeInternal, "message": "authentication service is not configured"}})
		return
	}
	provider := strings.TrimSpace(ctx.Param("provider"))
	result, err := c.svc.CompleteOIDCCallback(ctx.Request.Context(), provider, ctx.Query("code"), ctx.Query("state"))
	if err != nil {
		c.api.writeError(ctx.Writer, ctx.Request, err)
		return
	}
	writeJSON(ctx.Writer, http.StatusOK, result)
}
