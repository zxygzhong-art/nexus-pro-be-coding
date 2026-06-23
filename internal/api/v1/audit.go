package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// AuditCtrl wires audit log endpoints to the audit service facade.
type AuditCtrl struct {
	routes routeBinder
	svc    service.AuditFacade
}

// RegisterRoutes attaches audit log routes to the v1 route group.
func (c AuditCtrl) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("/audit-logs", c.routes.Handle("audit.log", "read", c.listAuditLogs))
}

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
