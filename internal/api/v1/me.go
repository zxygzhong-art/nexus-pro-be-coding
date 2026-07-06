package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// MeCtrl 定義 me ctrl 的資料結構。
type MeCtrl struct {
	routes routeBinder
	svc    service.MeFacade
}

// RegisterRoutes 註冊此 controller 的 HTTP 路由。
func (c MeCtrl) RegisterRoutes(router *gin.RouterGroup) {
	me := router.Group("/me")
	me.GET("", c.routes.Handle("me", "read", c.getMe))
	me.GET("/menus", c.routes.Handle("me", "read", c.getMenus))
}

// getMe 處理 me 的 HTTP 請求。
func (c MeCtrl) getMe(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	me, err := c.svc.Resolve(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, me)
	return nil
}

// getMenus 處理 menus 的 HTTP 請求。
func (c MeCtrl) getMenus(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	menus, err := c.svc.ListMenus(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, domain.MenuListResponse{Items: menus, Total: len(menus)})
	return nil
}
