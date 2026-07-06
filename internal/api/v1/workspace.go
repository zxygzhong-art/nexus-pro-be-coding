package v1

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// WorkspaceCtrl 定義工作區 ctrl 的資料結構。
type WorkspaceCtrl struct {
	routes routeBinder
	svc    service.WorkspaceFacade
}

// RegisterRoutes 註冊此 controller 的 HTTP 路由。
func (c WorkspaceCtrl) RegisterRoutes(router *gin.RouterGroup) {
	workspace := router.Group("/workspace")
	workspace.GET("/overview", c.routes.Handle("hr.employee", "read", c.overview))
	workspace.GET("/organization", c.routes.Handle("hr.employee", "read", c.organization))
	workspace.GET("/turnover", c.routes.Handle("hr.employee", "read", c.turnover))
	workspace.GET("/attendance", c.routes.Handle("attendance.clock", "read", c.attendance))
	workspace.GET("/admins", c.routes.Handle("iam.permission_set_assignment", "read", c.admins))
	workspace.GET("/audit-logs", c.routes.Handle("audit.log", "read", c.auditLogs))
}

// overview 處理總覽的 HTTP 請求。
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

// organization 處理 organization 的 HTTP 請求。
func (c WorkspaceCtrl) organization(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.WorkspaceOrganization(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// turnover 處理人員異動的 HTTP 請求。
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

// attendance 處理考勤的 HTTP 請求。
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

// admins 處理 admins 的 HTTP 請求。
func (c WorkspaceCtrl) admins(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.WorkspaceAdmins(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// auditLogs 處理稽核 logs 的 HTTP 請求。
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

// workspaceIntQuery 處理工作區整數查詢。
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
