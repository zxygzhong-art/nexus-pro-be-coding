package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

type AgentCtrl struct {
	routes routeBinder
	svc    service.AgentFacade
}

func (c AgentCtrl) RegisterRoutes(router *gin.RouterGroup) {
	agents := router.Group("/agents")
	agents.GET("/runs", c.routes.Handle("agent.run", "read", c.listAgentRuns))
	agents.POST("/runs", c.routes.Handle("agent.run", "create", c.createAgentRun))
}

func (c AgentCtrl) listAgentRuns(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListRunPage(ctx, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

func (c AgentCtrl) createAgentRun(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateAgentRunInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateRun(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}
