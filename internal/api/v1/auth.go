package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/service"
)

// AuthCtrl 定義 auth ctrl 的資料結構。
type AuthCtrl struct {
	api           *API
	svc           service.IdentityFacade
	tokenResolver TokenResolver
}

// RegisterRoutes 註冊此 controller 的 HTTP 路由。
func (c AuthCtrl) RegisterRoutes(router *gin.RouterGroup) {
	auth := router.Group("/auth")
	auth.POST("/sso/google/verify", c.verifyGoogleSSO)
}

// verifyGoogleSSO 處理 Google SSO token 驗證與本機身分綁定。
func (c AuthCtrl) verifyGoogleSSO(ctx *gin.Context) {
	principal, ok, err := c.tokenResolver.Resolve(ctx.Request)
	if err != nil || !ok {
		c.api.writeError(ctx.Writer, ctx.Request, domain.UnauthorizedReason("google_login_failed", "Google login failed"))
		return
	}
	result, err := c.svc.VerifyGoogleSSOLogin(ctx.Request.Context(), principal)
	if err != nil {
		c.api.writeError(ctx.Writer, ctx.Request, err)
		return
	}
	writeJSON(ctx.Writer, http.StatusOK, result)
}
