package v1

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/service"
)

// PlatformCtrl 定義平臺 ctrl 的資料結構。
type PlatformCtrl struct {
	routes routeBinder
	svc    service.PlatformFacade
}

// RegisterRoutes 註冊此 controller 的 HTTP 路由。
func (c PlatformCtrl) RegisterRoutes(router *gin.RouterGroup) {
	platform := router.Group("/platform")
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
func (c PlatformCtrl) forms(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	values := r.URL.Query()
	item, err := c.svc.Forms(ctx, domain.PlatformFormsQuery{
		Status:   strings.TrimSpace(values.Get("status")),
		Template: strings.TrimSpace(values.Get("template")),
		Search:   strings.TrimSpace(values.Get("search")),
		Page:     page.Page,
		PageSize: page.PageSize,
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// tasks 處理任務的 HTTP 請求。
func (c PlatformCtrl) tasks(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	query, err := platformTasksQueryFromRequest(r)
	if err != nil {
		return err
	}
	item, err := c.svc.Tasks(ctx, query)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// platformTasksQueryFromRequest 從 query params 解析任務 cursor 分頁與時間窗條件。
func platformTasksQueryFromRequest(r *http.Request) (domain.PlatformTasksQuery, error) {
	values := r.URL.Query()
	pageSize, err := positiveIntQuery(values.Get("page_size"), "page_size", domain.MaxPageSize)
	if err != nil {
		return domain.PlatformTasksQuery{}, err
	}
	from, err := optionalTimeQuery(values.Get("from"), "from", false)
	if err != nil {
		return domain.PlatformTasksQuery{}, err
	}
	to, err := optionalTimeQuery(values.Get("to"), "to", true)
	if err != nil {
		return domain.PlatformTasksQuery{}, err
	}
	return domain.PlatformTasksQuery{
		Cursor:   strings.TrimSpace(values.Get("cursor")),
		PageSize: pageSize,
		From:     from,
		To:       to,
	}, nil
}

// optionalTimeQuery 解析可選的時間查詢參數，支援 RFC3339 與 2006-01-02 日期格式。
// 日期格式的 to 參數會視為當日結束（回傳翌日零時作為排他上界）。
func optionalTimeQuery(raw, name string, endOfDay bool) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.UTC(), nil
	}
	if parsed, err := time.Parse("2006-01-02", raw); err == nil {
		parsed = parsed.UTC()
		if endOfDay {
			parsed = parsed.AddDate(0, 0, 1)
		}
		return parsed, nil
	}
	return time.Time{}, domain.BadRequest(name + " must be RFC3339 or YYYY-MM-DD")
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
