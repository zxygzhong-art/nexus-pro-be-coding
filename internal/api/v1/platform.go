package v1

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// PlatformCtrl 定義平台 ctrl 的資料結構。
type PlatformCtrl struct {
	routes routeBinder
	svc    service.PlatformFacade
}

// RegisterRoutes 註冊此 controller 的 HTTP 路由。
func (c PlatformCtrl) RegisterRoutes(router *gin.RouterGroup) {
	platform := router.Group("/platform")
	platform.GET("/home", c.routes.Handle("me", "read", c.home))
	platform.GET("/assistants", c.routes.Handle("me", "read", c.assistants))
	platform.GET("/forms", c.routes.Handle("me", "read", c.forms))
	platform.GET("/tasks", c.routes.Handle("me", "read", c.tasks))
	platform.POST("/tasks/items", c.routes.Handle("me", "create", c.createTaskItem))
	platform.PATCH("/tasks/items/:id", c.routes.Handle("me", "update", c.updateTaskItem, ResourceID(PathParamID)))
	platform.DELETE("/tasks/items/:id", c.routes.Handle("me", "delete", c.deleteTaskItem, ResourceID(PathParamID)))
	platform.POST("/tasks/todos", c.routes.Handle("me", "create", c.createTaskTodo))
	platform.PATCH("/tasks/todos/:id", c.routes.Handle("me", "update", c.updateTaskTodo, ResourceID(PathParamID)))
	platform.DELETE("/tasks/todos/:id", c.routes.Handle("me", "delete", c.deleteTaskTodo, ResourceID(PathParamID)))
	platform.POST("/tasks/todos/:id/convert", c.routes.Handle("me", "update", c.convertTaskTodo, ResourceID(PathParamID)))
	platform.GET("/workspace", c.routes.Handle("hr.employee", "read", c.workspace))
	platform.POST("/workspace/admins", c.routes.Handle("iam.permission_set_assignment", "create", c.createWorkspaceAdmin))
	platform.PATCH("/workspace/admins/:id/permissions", c.routes.Handle("iam.permission_set_assignment", "update", c.updateWorkspaceAdminPermissions, PathParam(PathParamID)))
	platform.DELETE("/workspace/admins/:id", c.routes.Handle("iam.permission_set_assignment", "delete", c.deleteWorkspaceAdmin, PathParam(PathParamID)))
	platform.POST("/workspace/forms", c.routes.Handle("workflow.form_template", "create", c.createWorkspaceFormDesign))
	platform.PATCH("/workspace/forms/:id", c.routes.Handle("workflow.form_template", "update", c.updateWorkspaceFormDesign, PathParam(PathParamID)))
	platform.DELETE("/workspace/forms/:id", c.routes.Handle("workflow.form_template", "delete", c.deleteWorkspaceFormDesign, PathParam(PathParamID)))
	platform.GET("/workspace/audit-logs", c.routes.Handle("audit.log", "read", c.workspaceAuditLogs))
	platform.GET("/workspace/overview", c.routes.Handle("hr.employee", "read", c.workspaceOverview))
	platform.GET("/workspace/employees", c.routes.Handle("hr.employee", "read", c.workspaceEmployees))
	platform.GET("/workspace/organization", c.routes.Handle("hr.employee", "read", c.workspaceOrganization))
	platform.PATCH("/workspace/organization/employees/:id/manager", c.routes.Handle("hr.employee", "update", c.updateWorkspaceOrganizationManager, PathParam(PathParamID)))
	platform.GET("/workspace/attendance", c.routes.Handle("attendance.clock", "read", c.workspaceAttendance))
	platform.GET("/workspace/turnover", c.routes.Handle("hr.employee", "read", c.workspaceTurnover))
	platform.GET("/insights", c.routes.Handle("hr.employee", "read", c.insights))
}

// home 處理首頁的 HTTP 請求。
func (c PlatformCtrl) home(w http.ResponseWriter, _ *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.Home(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// assistants 處理助理的 HTTP 請求。
func (c PlatformCtrl) assistants(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	values := r.URL.Query()
	item, err := c.svc.ListAssistants(ctx, domain.PlatformAssistantsQuery{
		Tag:    strings.TrimSpace(values.Get("tag")),
		Search: strings.TrimSpace(values.Get("search")),
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// forms 處理表單的 HTTP 請求。
func (c PlatformCtrl) forms(w http.ResponseWriter, _ *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.Forms(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// tasks 處理任務的 HTTP 請求。
func (c PlatformCtrl) tasks(w http.ResponseWriter, _ *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.Tasks(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// createTaskItem 處理任務項目的 HTTP 請求。
func (c PlatformCtrl) createTaskItem(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreatePlatformTaskItemInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateTaskItem(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// updateTaskItem 處理任務項目的 HTTP 請求。
func (c PlatformCtrl) updateTaskItem(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdatePlatformTaskItemInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateTaskItem(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// deleteTaskItem 處理任務項目的 HTTP 請求。
func (c PlatformCtrl) deleteTaskItem(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteTaskItem(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// createTaskTodo 處理任務待辦的 HTTP 請求。
func (c PlatformCtrl) createTaskTodo(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreatePlatformTaskTodoInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateTaskTodo(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// updateTaskTodo 處理任務待辦的 HTTP 請求。
func (c PlatformCtrl) updateTaskTodo(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdatePlatformTaskTodoInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateTaskTodo(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// deleteTaskTodo 處理任務待辦的 HTTP 請求。
func (c PlatformCtrl) deleteTaskTodo(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteTaskTodo(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// convertTaskTodo 處理 convert 任務待辦的 HTTP 請求。
func (c PlatformCtrl) convertTaskTodo(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.ConvertPlatformTaskTodoInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.ConvertTaskTodo(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// workspace 處理工作區的 HTTP 請求。
func (c PlatformCtrl) workspace(w http.ResponseWriter, _ *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.Workspace(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// createWorkspaceAdmin 處理工作區管理員的 HTTP 請求。
func (c PlatformCtrl) createWorkspaceAdmin(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateWorkspaceAdminInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateWorkspaceAdmin(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// updateWorkspaceAdminPermissions 處理工作區管理員權限的 HTTP 請求。
func (c PlatformCtrl) updateWorkspaceAdminPermissions(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateWorkspaceAdminPermissionsInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateWorkspaceAdminPermissions(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// deleteWorkspaceAdmin 處理工作區管理員的 HTTP 請求。
func (c PlatformCtrl) deleteWorkspaceAdmin(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteWorkspaceAdmin(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// createWorkspaceFormDesign 處理工作區表單 design 的 HTTP 請求。
func (c PlatformCtrl) createWorkspaceFormDesign(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
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

// updateWorkspaceFormDesign 處理工作區表單 design 的 HTTP 請求。
func (c PlatformCtrl) updateWorkspaceFormDesign(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
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

// deleteWorkspaceFormDesign 處理工作區表單 design 的 HTTP 請求。
func (c PlatformCtrl) deleteWorkspaceFormDesign(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteWorkspaceFormDesign(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// workspaceAuditLogs 處理工作區稽核 logs 的 HTTP 請求。
func (c PlatformCtrl) workspaceAuditLogs(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
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

// workspaceOverview 處理工作區總覽的 HTTP 請求。
func (c PlatformCtrl) workspaceOverview(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
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

// workspaceEmployees 處理工作區員工的 HTTP 請求。
func (c PlatformCtrl) workspaceEmployees(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
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

// workspaceOrganization 處理工作區 organization 的 HTTP 請求。
func (c PlatformCtrl) workspaceOrganization(w http.ResponseWriter, _ *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.WorkspaceOrganization(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// updateWorkspaceOrganizationManager 處理工作區 organization 主管的 HTTP 請求。
func (c PlatformCtrl) updateWorkspaceOrganizationManager(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
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

// workspaceAttendance 處理工作區考勤的 HTTP 請求。
func (c PlatformCtrl) workspaceAttendance(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
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

// workspaceTurnover 處理工作區人員異動的 HTTP 請求。
func (c PlatformCtrl) workspaceTurnover(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
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

// firstQueryValue 取得第一個查詢 value。
func firstQueryValue(values url.Values, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(values.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

// insights 處理洞察的 HTTP 請求。
func (c PlatformCtrl) insights(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.Insights(ctx, domain.PlatformInsightsQuery{
		Month: strings.TrimSpace(r.URL.Query().Get("month")),
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}
