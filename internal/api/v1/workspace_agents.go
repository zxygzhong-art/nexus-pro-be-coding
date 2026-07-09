package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// WorkspaceAgentCtrl 定義工作區 Agent 管理 ctrl。
type WorkspaceAgentCtrl struct {
	routes routeBinder
	svc    service.AgentFacade
}

// RegisterRoutes 註冊工作區 Agent 管理路由。
func (c WorkspaceAgentCtrl) RegisterRoutes(router *gin.RouterGroup) {
	workspace := router.Group("/workspace")

	workspace.GET("/agent-models", c.routes.Handle("agent.model", "read", c.listModels))
	workspace.POST("/agent-models", c.routes.Handle("agent.model", "create", c.createModel))
	workspace.PATCH("/agent-models/:id", c.routes.Handle("agent.model", "update", c.updateModel, ResourceID(PathParamID)))
	workspace.DELETE("/agent-models/:id", c.routes.Handle("agent.model", "delete", c.deleteModel, ResourceID(PathParamID)))
	workspace.POST("/agent-models/:id/sync", c.routes.Handle("agent.model", "update", c.syncModel, ResourceID(PathParamID)))
	workspace.POST("/agent-models/:id/test", c.routes.Handle("agent.model", "update", c.testModel, ResourceID(PathParamID)))

	workspace.GET("/agents/templates", c.routes.Handle("agent.definition", "read", c.templates))
	workspace.GET("/agents/tools", c.routes.Handle("agent.definition", "read", c.tools))
	workspace.GET("/agents/audits", c.routes.Handle("agent.definition", "read", c.audits))
	workspace.GET("/agents/bundle", c.routes.Handle("agent.definition", "read", c.exportBundle))
	workspace.POST("/agents/bundle", c.routes.Handle("agent.definition", "create", c.importBundle))
	workspace.GET("/agents", c.routes.Handle("agent.definition", "read", c.listDefinitions))
	workspace.POST("/agents", c.routes.Handle("agent.definition", "create", c.createDefinition))
	workspace.PATCH("/agents/:id", c.routes.Handle("agent.definition", "update", c.updateDefinition, ResourceID(PathParamID)))
	workspace.DELETE("/agents/:id", c.routes.Handle("agent.definition", "delete", c.deleteDefinition, ResourceID(PathParamID)))
	workspace.POST("/agents/:id/trial", c.routes.Handle("agent.definition", "update", c.trial, ResourceID(PathParamID)))
	workspace.POST("/agents/:id/rollback", c.routes.Handle("agent.definition", "update", c.rollback, ResourceID(PathParamID)))
}

func (c WorkspaceAgentCtrl) listModels(w http.ResponseWriter, _ *http.Request, ctx domain.RequestContext) error {
	items, err := c.svc.ListModels(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
	return nil
}

func (c WorkspaceAgentCtrl) createModel(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateAgentModelInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateModel(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

func (c WorkspaceAgentCtrl) updateModel(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateAgentModelInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateModel(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

func (c WorkspaceAgentCtrl) deleteModel(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteModel(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

func (c WorkspaceAgentCtrl) testModel(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.TestModel(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

func (c WorkspaceAgentCtrl) syncModel(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.SyncModel(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

func (c WorkspaceAgentCtrl) listDefinitions(w http.ResponseWriter, _ *http.Request, ctx domain.RequestContext) error {
	items, err := c.svc.ListDefinitions(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
	return nil
}

func (c WorkspaceAgentCtrl) createDefinition(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateAgentDefinitionInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateDefinition(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

func (c WorkspaceAgentCtrl) updateDefinition(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateAgentDefinitionInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateDefinition(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

func (c WorkspaceAgentCtrl) deleteDefinition(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteDefinition(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

func (c WorkspaceAgentCtrl) trial(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.AgentTrialInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.Trial(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

func (c WorkspaceAgentCtrl) rollback(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.RollbackAgentDefinitionInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.RollbackDefinition(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

func (c WorkspaceAgentCtrl) templates(w http.ResponseWriter, _ *http.Request, ctx domain.RequestContext) error {
	items, err := c.svc.Templates(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
	return nil
}

func (c WorkspaceAgentCtrl) tools(w http.ResponseWriter, _ *http.Request, ctx domain.RequestContext) error {
	items, err := c.svc.Tools(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
	return nil
}

func (c WorkspaceAgentCtrl) audits(w http.ResponseWriter, _ *http.Request, ctx domain.RequestContext) error {
	items, err := c.svc.ListAgentAudits(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
	return nil
}

func (c WorkspaceAgentCtrl) exportBundle(w http.ResponseWriter, _ *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.ExportBundle(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

func (c WorkspaceAgentCtrl) importBundle(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.AgentBundle
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.ImportBundle(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}
