package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// AgentCtrl 定義 agent ctrl 的資料結構。
type AgentCtrl struct {
	routes routeBinder
	svc    service.AgentFacade
}

// RegisterRoutes 註冊此 controller 的 HTTP 路由。
func (c AgentCtrl) RegisterRoutes(router *gin.RouterGroup) {
	agents := router.Group("/agents")
	agents.GET("/runs", c.routes.Handle("agent.run", "read", c.listAgentRuns))
	agents.POST("/runs", c.routes.Handle("agent.run", "create", c.createAgentRun))
	agents.POST("/chat", c.routes.Handle("agent.run", "create", c.chatAgent))
	agents.GET("/sessions", c.routes.Handle("agent.run", "read", c.listAgentSessions))
	agents.POST("/sessions", c.routes.Handle("agent.run", "create", c.createAgentSession))
	agents.GET("/sessions/:id", c.routes.Handle("agent.run", "read", c.getAgentSession, ResourceID(PathParamID)))
	agents.PATCH("/sessions/:id", c.routes.Handle("agent.run", "update", c.updateAgentSession, ResourceID(PathParamID)))
	agents.DELETE("/sessions/:id", c.routes.Handle("agent.run", "delete", c.deleteAgentSession, ResourceID(PathParamID)))
	agents.GET("/sessions/:id/messages", c.routes.Handle("agent.run", "read", c.listAgentSessionMessages, ResourceID(PathParamID)))
	agents.GET("/memories", c.routes.Handle("agent.run", "read", c.listAgentMemories))
	agents.POST("/memories", c.routes.Handle("agent.run", "create", c.createAgentMemory))
	agents.PATCH("/memories/:id", c.routes.Handle("agent.run", "update", c.updateAgentMemory, ResourceID(PathParamID)))
	agents.DELETE("/memories/:id", c.routes.Handle("agent.run", "delete", c.deleteAgentMemory, ResourceID(PathParamID)))
}

// listAgentRuns 處理 agent 執行紀錄的 HTTP 請求。
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

// createAgentRun 處理 agent 執行的 HTTP 請求。
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

// listAgentSessions 處理 agent 會話列表的 HTTP 請求。
func (c AgentCtrl) listAgentSessions(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	query := domain.ListAgentSessionsQuery{
		AgentID: strings.TrimSpace(r.URL.Query().Get("agent_id")),
		Status:  strings.TrimSpace(r.URL.Query().Get("status")),
	}
	items, err := c.svc.ListSessions(ctx, query)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// createAgentSession 處理建立 agent 會話的 HTTP 請求。
func (c AgentCtrl) createAgentSession(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateAgentSessionInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateSession(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// getAgentSession 處理取得 agent 會話的 HTTP 請求。
func (c AgentCtrl) getAgentSession(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.GetSession(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// updateAgentSession 處理更新 agent 會話的 HTTP 請求。
func (c AgentCtrl) updateAgentSession(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateAgentSessionInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateSession(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// deleteAgentSession 處理刪除 agent 會話的 HTTP 請求。
func (c AgentCtrl) deleteAgentSession(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteSession(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// listAgentSessionMessages 處理 agent 會話訊息列表的 HTTP 請求。
func (c AgentCtrl) listAgentSessionMessages(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	items, err := c.svc.ListSessionMessages(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// listAgentMemories 處理 agent 記憶列表的 HTTP 請求。
func (c AgentCtrl) listAgentMemories(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	query := domain.ListAgentMemoriesQuery{
		AgentID:   strings.TrimSpace(r.URL.Query().Get("agent_id")),
		SessionID: strings.TrimSpace(r.URL.Query().Get("session_id")),
	}
	items, err := c.svc.ListMemories(ctx, query)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// createAgentMemory 處理建立 agent 記憶的 HTTP 請求。
func (c AgentCtrl) createAgentMemory(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateAgentMemoryInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateMemory(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// updateAgentMemory 處理更新 agent 記憶的 HTTP 請求。
func (c AgentCtrl) updateAgentMemory(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateAgentMemoryInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateMemory(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// deleteAgentMemory 處理刪除 agent 記憶的 HTTP 請求。
func (c AgentCtrl) deleteAgentMemory(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteMemory(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// chatAgent 處理 agent chat SSE 請求。
func (c AgentCtrl) chatAgent(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.AgentChatInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		return domain.E(http.StatusInternalServerError, "internal_error", "streaming is not supported")
	}
	wroteHeader := false
	writeStreamHeader := func() {
		if wroteHeader {
			return
		}
		header := w.Header()
		header.Set("Content-Type", "text/event-stream; charset=utf-8")
		header.Set("Cache-Control", "no-cache")
		header.Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		wroteHeader = true
	}
	emit := func(_ context.Context, event domain.AgentChatEvent) error {
		writeStreamHeader()
		if err := writeSSEEvent(w, event); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	_, err := c.svc.Chat(ctx, input, emit)
	if err != nil {
		if wroteHeader {
			_ = writeSSEEvent(w, domain.AgentChatEvent{Event: domain.AgentChatEventError, Message: err.Error()})
			flusher.Flush()
			return nil
		}
		return err
	}
	return nil
}

func writeSSEEvent(w http.ResponseWriter, event domain.AgentChatEvent) error {
	name := event.Event
	if name == "" {
		name = domain.AgentChatEventMessageDelta
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", name); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", raw); err != nil {
		return err
	}
	return nil
}
