package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/service"
)

// AuditCtrl 定義稽覈 ctrl 的資料結構。
type AuditCtrl struct {
	routes routeBinder
	svc    service.AuditFacade
}

// RegisterRoutes 註冊此 controller 的 HTTP 路由。
func (c AuditCtrl) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("/audit-logs", c.routes.Handle("audit.log", "read", c.listAuditLogs))
}

// listAuditLogs 處理稽覈 logs 的 HTTP 請求。
func (c AuditCtrl) listAuditLogs(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListLogPage(ctx, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}
