package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

type MeCtrl struct {
	routes routeBinder
	svc    service.MeFacade
}

func (c MeCtrl) RegisterRoutes(router *gin.RouterGroup) {
	me := router.Group("/me")
	me.GET("", c.routes.Handle("me", "read", c.getMe))
	me.GET("/menus", c.routes.Handle("me", "read", c.getMenus))
}

func (c MeCtrl) getMe(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	me, err := c.svc.Resolve(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, me)
	return nil
}

func (c MeCtrl) getMenus(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	menus, err := c.svc.ListMenus(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, domain.MenuListResponse{Items: menus, Total: len(menus)})
	return nil
}
