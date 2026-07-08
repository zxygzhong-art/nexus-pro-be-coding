package v1

import (
	"net/http"
	"net/url"
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
	workspace.GET("", c.routes.Handle("hr.employee", "read", c.aggregate))
	workspace.GET("/overview", c.routes.Handle("hr.employee", "read", c.overview))
	workspace.GET("/employees", c.routes.Handle("hr.employee", "read", c.employees))
	workspace.GET("/organization", c.routes.Handle("hr.employee", "read", c.organization))
	workspace.PATCH("/organization/employees/:id/manager", c.routes.Handle("hr.employee", "update", c.updateOrganizationManager, PathParam(PathParamID)))
	workspace.GET("/attendance", c.routes.Handle("attendance.clock", "read", c.attendance))
	workspace.GET("/turnover", c.routes.Handle("hr.employee", "read", c.turnover))
	workspace.GET("/forms", c.routes.Handle("workflow.form_template", "read", c.formDesign))
	workspace.POST("/forms", c.routes.Handle("workflow.form_template", "create", c.createFormDesign))
	workspace.PATCH("/forms/:id", c.routes.Handle("workflow.form_template", "update", c.updateFormDesign, PathParam(PathParamID)))
	workspace.DELETE("/forms/:id", c.routes.Handle("workflow.form_template", "delete", c.deleteFormDesign, PathParam(PathParamID)))
	workspace.GET("/audit-logs", c.routes.Handle("audit.log", "read", c.auditLogs))
	workspace.GET("/insights", c.routes.Handle("hr.employee", "read", c.insights))
}

// aggregate 處理工作區 aggregate 的 HTTP 請求。
func (c WorkspaceCtrl) aggregate(w http.ResponseWriter, _ *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.Workspace(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
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

// employees 處理員工的 HTTP 請求。
func (c WorkspaceCtrl) employees(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	values := r.URL.Query()
	item, err := c.svc.WorkspaceEmployees(ctx, domain.PlatformWorkspaceEmployeesQuery{
		DepartmentID:     strings.TrimSpace(values.Get("department_id")),
		Department:       strings.TrimSpace(firstQueryValue(values, "department", "dept")),
		Status:           strings.TrimSpace(values.Get("status")),
		EmploymentStatus: strings.TrimSpace(values.Get("employment_status")),
		Keyword:          strings.TrimSpace(firstQueryValue(values, "keyword", "search")),
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// organization 處理 organization 的 HTTP 請求。
func (c WorkspaceCtrl) organization(w http.ResponseWriter, _ *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.WorkspaceOrganization(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// updateOrganizationManager 處理 organization 主管的 HTTP 請求。
func (c WorkspaceCtrl) updateOrganizationManager(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateWorkspaceOrganizationManagerInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateWorkspaceOrganizationManager(ctx, r.PathValue(PathParamID), input)
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

// formDesign 處理表單 design 的 HTTP 請求。
func (c WorkspaceCtrl) formDesign(w http.ResponseWriter, _ *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.WorkspaceFormDesign(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// createFormDesign 處理建立表單 design 的 HTTP 請求。
func (c WorkspaceCtrl) createFormDesign(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.SaveWorkspaceFormDesignInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateWorkspaceFormDesign(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// updateFormDesign 處理更新表單 design 的 HTTP 請求。
func (c WorkspaceCtrl) updateFormDesign(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateWorkspaceFormDesignInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateWorkspaceFormDesign(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// deleteFormDesign 處理刪除表單 design 的 HTTP 請求。
func (c WorkspaceCtrl) deleteFormDesign(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteWorkspaceFormDesign(ctx, r.PathValue(PathParamID))
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

// insights 處理洞察的 HTTP 請求。
func (c WorkspaceCtrl) insights(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.Insights(ctx, domain.PlatformInsightsQuery{
		Month: strings.TrimSpace(r.URL.Query().Get("month")),
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// firstQueryValue 取得第一個查詢 value。
func firstQueryValue(values url.Values, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(values.Get(name)); value != "" {
			return value
		}
	}
	return ""
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
