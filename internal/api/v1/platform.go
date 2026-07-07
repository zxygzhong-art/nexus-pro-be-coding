package v1

import (
	"net/http"
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
