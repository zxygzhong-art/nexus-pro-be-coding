package v1

import (
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/service"
)

// WorkspaceCtrl 定義工作區 ctrl 的資料結構。
type WorkspaceCtrl struct {
	routes routeBinder
	svc    service.WorkspaceFacade
}

// RegisterRoutes 註冊此 controller 的 HTTP 路由。
func (c WorkspaceCtrl) RegisterRoutes(router *gin.RouterGroup) {
	workspace := router.Group("/workspace")
	workspace.GET("/overview", c.routes.Handle("hr.employee", "read", c.overview, TenantWideScope()))
	workspace.GET("/org-units-directory", c.routes.Handle("hr.org_unit", "read", c.orgUnitsDirectory, TenantWideScope()))
	workspace.GET("/organization", c.routes.Handle("hr.employee", "read", c.organization, TenantWideScope()))
	workspace.GET("/attendance", c.routes.Handle("attendance.clock", "read", c.attendance, TenantWideScope()))
	workspace.GET("/attendance/abnormals", c.routes.Handle("attendance.clock", "read", c.attendanceAbnormals, TenantWideScope()))
	workspace.GET("/attendance/export", c.routes.Handle("attendance.clock", "export", c.exportAttendanceCSV, TenantWideScope()))
	workspace.GET("/turnover", c.routes.Handle("hr.employee", "read", c.turnover, TenantWideScope()))
	workspace.GET("/turnover/export", c.routes.Handle("hr.employee", "export", c.exportTurnoverCSV, TenantWideScope()))
	workspace.GET("/forms", c.routes.Handle("workflow.form_template", "read", c.formDesign, TenantWideScope()))
	workspace.POST("/forms", c.routes.Handle("workflow.form_template", "create", c.createFormDesign, TenantWideScope()))
	workspace.PATCH("/forms/:id", c.routes.Handle("workflow.form_template", "update", c.updateFormDesign, PathParam(PathParamID), TenantWideScope()))
	workspace.POST("/forms/:id/publish", c.routes.Handle("workflow.form_template", "update", c.publishFormDesign, PathParam(PathParamID), TenantWideScope()))
	workspace.DELETE("/forms/:id", c.routes.Handle("workflow.form_template", "delete", c.deleteFormDesign, PathParam(PathParamID), TenantWideScope()))
	workspace.GET("/audit-logs", c.routes.Handle("audit.log", "read", c.auditLogs, TenantWideScope()))
	workspace.GET("/audit-logs/facets", c.routes.Handle("audit.log", "read", c.auditLogFacets, TenantWideScope()))
	workspace.GET("/insights", c.routes.Handle("hr.employee", "read", c.insights, TenantWideScope()))
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

// orgUnitsDirectory 處理組織單位管理頁的聚合讀取請求。
func (c WorkspaceCtrl) orgUnitsDirectory(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	includeEmployees := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_employees")), "true")
	item, err := c.svc.WorkspaceOrgUnitsDirectory(ctx, includeEmployees)
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

// exportTurnoverCSV 處理人員異動 CSV 匯出。
func (c WorkspaceCtrl) exportTurnoverCSV(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	annualYear := workspaceIntQuery(r, "annual_year")
	if annualYear <= 0 {
		annualYear = workspaceIntQuery(r, "annualYear")
	}
	raw, filename, err := c.svc.ExportWorkspaceTurnoverCSV(ctx, domain.WorkspaceTurnoverQuery{
		Year:       workspaceIntQuery(r, "year"),
		Month:      workspaceIntQuery(r, "month"),
		AnnualYear: annualYear,
	}, strings.TrimSpace(r.URL.Query().Get("kind")))
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	_, _ = w.Write(raw)
	return nil
}

// attendance 處理考勤的 HTTP 請求。
func (c WorkspaceCtrl) attendance(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	values := r.URL.Query()
	projection := strings.TrimSpace(firstQueryValue(values, "projection", "view"))
	item, err := c.svc.WorkspaceAttendance(ctx, domain.WorkspaceAttendanceQuery{
		Year:         workspaceIntQuery(r, "year"),
		Month:        workspaceIntQuery(r, "month"),
		Projection:   projection,
		DepartmentID: strings.TrimSpace(values.Get("department_id")),
		Keyword:      strings.TrimSpace(firstQueryValue(values, "keyword", "search")),
		Page:         page.Page,
		PageSize:     page.PageSize,
		Paginated:    projection != "" || values.Has("page") || values.Has("page_size"),
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// attendanceAbnormals returns anomaly rows within an explicitly bounded
// employee page; page/page_size then paginate those filtered anomaly rows.
func (c WorkspaceCtrl) attendanceAbnormals(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	values := r.URL.Query()
	employeePage, err := positiveIntQuery(values.Get("employee_page"), "employee_page", 0)
	if err != nil {
		return err
	}
	employeePageSize, err := positiveIntQuery(values.Get("employee_page_size"), "employee_page_size", domain.MaxPageSize)
	if err != nil {
		return err
	}
	item, err := c.svc.WorkspaceClockAbnormals(ctx, domain.WorkspaceClockAbnormalQuery{
		Year:             workspaceIntQuery(r, "year"),
		Month:            workspaceIntQuery(r, "month"),
		BaseDepartmentID: strings.TrimSpace(values.Get("base_department_id")),
		DepartmentID:     strings.TrimSpace(values.Get("department_id")),
		Keyword:          strings.TrimSpace(firstQueryValue(values, "keyword", "search")),
		Severity:         strings.TrimSpace(values.Get("severity")),
		Page:             page.Page,
		PageSize:         page.PageSize,
		EmployeePage:     employeePage,
		EmployeePageSize: employeePageSize,
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// exportAttendanceCSV 處理考勤 CSV 匯出。
func (c WorkspaceCtrl) exportAttendanceCSV(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	values := r.URL.Query()
	raw, filename, err := c.svc.ExportWorkspaceAttendanceCSV(ctx, domain.WorkspaceAttendanceQuery{
		Year:         workspaceIntQuery(r, "year"),
		Month:        workspaceIntQuery(r, "month"),
		DepartmentID: strings.TrimSpace(values.Get("department_id")),
		Keyword:      strings.TrimSpace(firstQueryValue(values, "keyword", "search")),
	}, strings.TrimSpace(r.URL.Query().Get("kind")))
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	_, _ = w.Write(raw)
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

// publishFormDesign 將目前表單草稿版本發布為新的線上版本。
func (c WorkspaceCtrl) publishFormDesign(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.PublishWorkspaceFormDesignInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.PublishWorkspaceFormDesign(ctx, r.PathValue(PathParamID), input)
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

// auditLogs 處理稽覈 logs 的 HTTP 請求。
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

// auditLogFacets returns tenant-wide filter options with stable operator identifiers.
func (c WorkspaceCtrl) auditLogFacets(w http.ResponseWriter, _ *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.WorkspaceAuditLogFacets(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
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
