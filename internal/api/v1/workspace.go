package v1

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// WorkspaceCtrl wires workspace dashboard aggregate endpoints.
type WorkspaceCtrl struct {
	routes routeBinder
	svc    service.WorkspaceFacade
}

// RegisterRoutes attaches workspace aggregate routes to the v1 route group.
func (c WorkspaceCtrl) RegisterRoutes(router *gin.RouterGroup) {
	workspace := router.Group("/workspace")
	workspace.GET("/overview", c.routes.Handle("hr.employee", "read", c.overview))
	workspace.GET("/organization", c.routes.Handle("hr.employee", "read", c.organization))
	workspace.GET("/turnover", c.routes.Handle("hr.employee", "read", c.turnover))
	workspace.GET("/attendance", c.routes.Handle("attendance.clock", "read", c.attendance))
	workspace.GET("/admins", c.routes.Handle("iam.permission_set_assignment", "read", c.admins))
	workspace.GET("/audit-logs", c.routes.Handle("audit.log", "read", c.auditLogs))
}

// overview returns homepage HR and attendance widgets.
func (c WorkspaceCtrl) overview(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.WorkspaceOverview(ctx, domain.WorkspaceOverviewQuery{
		Year:  workspaceIntQuery(r, "year"),
		Month: workspaceIntQuery(r, "month"),
		Date:  strings.TrimSpace(r.URL.Query().Get("date")),
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// organization returns the employee organization tree.
func (c WorkspaceCtrl) organization(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.WorkspaceOrganization(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// turnover returns monthly and annual employment movement analysis.
func (c WorkspaceCtrl) turnover(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	annualYear := workspaceIntQuery(r, "annual_year")
	if annualYear <= 0 {
		annualYear = workspaceIntQuery(r, "annualYear")
	}
	item, err := c.svc.WorkspaceTurnover(ctx, domain.WorkspaceTurnoverQuery{
		Year:       workspaceIntQuery(r, "year"),
		Month:      workspaceIntQuery(r, "month"),
		AnnualYear: annualYear,
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// attendance returns monthly attendance and clock matrices.
func (c WorkspaceCtrl) attendance(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.WorkspaceAttendance(ctx, domain.WorkspaceAttendanceQuery{
		Year:  workspaceIntQuery(r, "year"),
		Month: workspaceIntQuery(r, "month"),
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// admins returns HR workspace administrator settings.
func (c WorkspaceCtrl) admins(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.WorkspaceAdmins(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// auditLogs returns filtered workspace audit-log rows.
func (c WorkspaceCtrl) auditLogs(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	values := r.URL.Query()
	items, err := c.svc.WorkspaceAuditLogs(ctx, domain.WorkspaceAuditLogQuery{
		OperatorID: strings.TrimSpace(values.Get("operator_id")),
		Type:       strings.TrimSpace(values.Get("type")),
		From:       strings.TrimSpace(values.Get("from")),
		To:         strings.TrimSpace(values.Get("to")),
		Keyword:    strings.TrimSpace(values.Get("keyword")),
	}, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// workspaceIntQuery parses an optional integer query parameter.
func workspaceIntQuery(r *http.Request, name string) int {
	value := strings.TrimSpace(r.URL.Query().Get(name))
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}
