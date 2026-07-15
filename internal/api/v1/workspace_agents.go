package v1

import (
	"net/http"
	"strings"

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
	workspace.GET("/knowledge-bases", c.routes.Handle("agent.knowledge_base", "read", c.listKnowledgeBases))
	workspace.POST("/knowledge-bases", c.routes.Handle("agent.knowledge_base", "create", c.createKnowledgeBase))
	workspace.GET("/knowledge-bases/:id", c.routes.Handle("agent.knowledge_base", "read", c.getKnowledgeBase, ResourceID(PathParamID)))
	workspace.PATCH("/knowledge-bases/:id", c.routes.Handle("agent.knowledge_base", "update", c.updateKnowledgeBase, ResourceID(PathParamID)))
	workspace.DELETE("/knowledge-bases/:id", c.routes.Handle("agent.knowledge_base", "delete", c.deleteKnowledgeBase, ResourceID(PathParamID)))
	workspace.GET("/knowledge-bases/:id/documents", c.routes.Handle("agent.knowledge_base", "read", c.listKnowledgeDocuments, ResourceID(PathParamID)))
	workspace.POST("/knowledge-bases/:id/documents", c.routes.Handle("agent.knowledge_base", "update", c.createKnowledgeDocument, ResourceID(PathParamID)))
	workspace.PATCH("/knowledge-bases/:id/documents/:document_id", c.routes.Handle("agent.knowledge_base", "update", c.updateKnowledgeDocument, ResourceID(PathParamID), PathParam(pathParamDocumentID)))
	workspace.DELETE("/knowledge-bases/:id/documents/:document_id", c.routes.Handle("agent.knowledge_base", "update", c.deleteKnowledgeDocument, ResourceID(PathParamID), PathParam(pathParamDocumentID)))
	workspace.POST("/knowledge-bases/:id/search", c.routes.Handle("agent.knowledge_base", "read", c.searchKnowledgeBase, ResourceID(PathParamID)))

	workspace.GET("/agent-models", c.routes.Handle("agent.model", "read", c.listModels))
	workspace.POST("/agent-models", c.routes.Handle("agent.model", "create", c.createModel))
	workspace.PATCH("/agent-models/:id", c.routes.Handle("agent.model", "update", c.updateModel, ResourceID(PathParamID)))
	workspace.DELETE("/agent-models/:id", c.routes.Handle("agent.model", "delete", c.deleteModel, ResourceID(PathParamID)))
	workspace.POST("/agent-models/:id/sync", c.routes.Handle("agent.model", "update", c.syncModel, ResourceID(PathParamID)))
	workspace.POST("/agent-models/:id/test", c.routes.Handle("agent.model", "update", c.testModel, ResourceID(PathParamID)))

	workspace.GET("/agents/tools", c.routes.Handle("agent.tool", "read", c.tools))
	workspace.GET("/agent-usage", c.routes.Handle("agent.definition", "read", c.listAccountUsage))
	workspace.GET("/agent-usage/:id/sessions", c.routes.Handle("agent.definition", "read", c.listAccountSessionUsage, PathParam(PathParamID)))
	workspace.GET("/agents/external-tools", c.routes.Handle("agent.tool", "read", c.listExternalTools))
	workspace.POST("/agents/external-tools", c.routes.Handle("agent.tool", "create", c.createExternalTool))
	workspace.DELETE("/agents/external-tools/:id", c.routes.Handle("agent.tool", "delete", c.deleteExternalTool, ResourceID(PathParamID)))
	workspace.GET("/agents", c.routes.Handle("agent.definition", "read", c.listDefinitions))
	workspace.POST("/agents", c.routes.Handle("agent.definition", "create", c.createDefinition))
	workspace.PATCH("/agents/:id", c.routes.Handle("agent.definition", "update", c.updateDefinition, ResourceID(PathParamID)))
	workspace.POST("/agents/:id/publish", c.routes.Handle("agent.definition", "update", c.publishDefinition, ResourceID(PathParamID)))
	workspace.POST("/agents/:id/unpublish", c.routes.Handle("agent.definition", "update", c.unpublishDefinition, ResourceID(PathParamID)))
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

func (c WorkspaceAgentCtrl) publishDefinition(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.PublishDefinition(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

func (c WorkspaceAgentCtrl) unpublishDefinition(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.UnpublishDefinition(ctx, r.PathValue(PathParamID))
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

func (c WorkspaceAgentCtrl) tools(w http.ResponseWriter, _ *http.Request, ctx domain.RequestContext) error {
	items, err := c.svc.Tools(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
	return nil
}

// listAccountUsage returns a server-filtered page with tenant-wide totals.
func (c WorkspaceAgentCtrl) listAccountUsage(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	usage, err := c.svc.ListAccountUsage(ctx, domain.AgentAccountUsageQuery{
		Query:  strings.TrimSpace(r.URL.Query().Get("query")),
		Status: strings.TrimSpace(r.URL.Query().Get("status")),
	}, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, usage)
	return nil
}

// listAccountSessionUsage returns paginated session usage for one account.
func (c WorkspaceAgentCtrl) listAccountSessionUsage(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	usage, err := c.svc.ListAccountSessionUsage(ctx, r.PathValue(PathParamID), page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, usage)
	return nil
}

// listExternalTools returns tenant-managed external tool registrations.
func (c WorkspaceAgentCtrl) listExternalTools(w http.ResponseWriter, _ *http.Request, ctx domain.RequestContext) error {
	items, err := c.svc.ListExternalTools(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
	return nil
}

// createExternalTool registers external tool metadata without activating runtime execution.
func (c WorkspaceAgentCtrl) createExternalTool(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateAgentExternalToolInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateExternalTool(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// deleteExternalTool removes one tenant-owned external tool registration.
func (c WorkspaceAgentCtrl) deleteExternalTool(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteExternalTool(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}
