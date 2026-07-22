package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/service"
	agentservice "nexus-pro-api/internal/service/agent"
)

const pathParamAgentFileID = "file_id"

// AgentCtrl 定義 agent ctrl 的資料結構。
type AgentCtrl struct {
	routes routeBinder
	svc    service.AgentFacade
}

// RegisterRoutes 註冊此 controller 的 HTTP 路由。
func (c AgentCtrl) RegisterRoutes(router *gin.RouterGroup) {
	agents := router.Group("/agents")
	agents.POST("/chat", c.routes.Handle("agent.run", "create", c.chatAgent))
	agents.POST("/confirmations/:id/execute", c.routes.Handle("agent.run", "create", c.executeAgentConfirmation, PathParam(PathParamID)))
	agents.GET("/sessions", c.routes.Handle("agent.run", "read", c.listAgentSessions))
	agents.POST("/sessions", c.routes.Handle("agent.run", "create", c.createAgentSession))
	agents.POST("/sessions/:id/clear-context", c.routes.Handle("agent.run", "create", c.clearAgentSessionContext, ResourceID(PathParamID)))
	agents.DELETE("/sessions/:id", c.routes.Handle("agent.run", "delete", c.deleteAgentSession, ResourceID(PathParamID)))
	agents.GET("/sessions/:id/messages", c.routes.Handle("agent.run", "read", c.listAgentSessionMessages, ResourceID(PathParamID)))
	agents.POST("/sessions/:id/files", c.routes.Handle("agent.run", "create", c.uploadAgentSessionFile, ResourceID(PathParamID)))
	agents.GET("/sessions/:id/files/:file_id", c.routes.Handle("agent.run", "read", c.downloadAgentSessionFile, ResourceID(PathParamID), PathParam(pathParamAgentFileID)))
	agents.DELETE("/sessions/:id/files/:file_id", c.routes.Handle("agent.run", "create", c.deleteAgentSessionFile, ResourceID(PathParamID), PathParam(pathParamAgentFileID)))
	agents.GET("/memories", c.routes.Handle("agent.run", "read", c.listAgentMemories))
}

// executeAgentConfirmation 執行使用者在 Agent 卡片上明確確認的一次性操作。
func (c AgentCtrl) executeAgentConfirmation(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.ExecuteAgentConfirmationInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	result, err := c.svc.ExecuteConfirmation(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}

// listAgentSessions 處理 agent 會話列表的 HTTP 請求。
func (c AgentCtrl) listAgentSessions(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	values := r.URL.Query()
	pageSize, err := positiveIntQuery(values.Get("page_size"), "page_size", domain.MaxPageSize)
	if err != nil {
		return err
	}
	query := domain.ListAgentSessionsQuery{
		AgentID:  strings.TrimSpace(values.Get("agent_id")),
		Status:   strings.TrimSpace(values.Get("status")),
		Cursor:   strings.TrimSpace(values.Get("cursor")),
		PageSize: pageSize,
	}
	page, err := c.svc.ListSessions(ctx, query)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, page)
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

// clearAgentSessionContext 處理清除 agent 會話上下文的 HTTP 請求。
func (c AgentCtrl) clearAgentSessionContext(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.ClearSessionContext(ctx, r.PathValue(PathParamID))
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
	values := r.URL.Query()
	pageSize, err := positiveIntQuery(values.Get("page_size"), "page_size", domain.MaxPageSize)
	if err != nil {
		return err
	}
	query := domain.ListAgentSessionMessagesQuery{
		Cursor:   strings.TrimSpace(values.Get("cursor")),
		PageSize: pageSize,
	}
	page, err := c.svc.ListSessionMessages(ctx, r.PathValue(PathParamID), query)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, page)
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
	var streamMu sync.Mutex
	wroteHeader := false
	wroteErrorEvent := false
	writeStreamHeader := func() {
		if wroteHeader {
			return
		}
		header := w.Header()
		header.Set("Content-Type", "text/event-stream; charset=utf-8")
		header.Set("Cache-Control", "no-cache, no-transform")
		header.Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		wroteHeader = true
	}
	emit := func(_ context.Context, event domain.AgentChatEvent) error {
		streamMu.Lock()
		defer streamMu.Unlock()
		writeStreamHeader()
		if err := writeSSEEvent(w, event); err != nil {
			return err
		}
		if event.Event == domain.AgentChatEventError {
			wroteErrorEvent = true
		}
		flusher.Flush()
		return nil
	}
	run, err := c.svc.Chat(ctx, input, emit)
	if err != nil {
		streamMu.Lock()
		defer streamMu.Unlock()
		if wroteHeader {
			if !wroteErrorEvent {
				_ = writeSSEEvent(w, agentservice.AgentRuntimeFailureEvent(ctx, run.ID))
			}
			flusher.Flush()
			return nil
		}
		return err
	}
	return nil
}

// uploadAgentSessionFile stages a UTF-8 text attachment in the current context version.
func (c AgentCtrl) uploadAgentSessionFile(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	r.Body = http.MaxBytesReader(w, r.Body, 11<<20)
	if err := r.ParseMultipartForm(11 << 20); err != nil {
		return domain.BadRequest("invalid multipart form: " + err.Error())
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return domain.BadRequest("file is required")
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, (10<<20)+1))
	if err != nil {
		return domain.BadRequest("read conversation file: " + err.Error())
	}
	item, err := c.svc.UploadSessionFile(ctx, r.PathValue(PathParamID), domain.UploadAgentSessionFileInput{
		Filename: header.Filename, ContentType: header.Header.Get("Content-Type"), Content: content,
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// downloadAgentSessionFile proxies authorized object bytes without exposing SFTPGo credentials or keys.
func (c AgentCtrl) downloadAgentSessionFile(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	download, err := c.svc.DownloadSessionFile(ctx, r.PathValue(PathParamID), r.PathValue(pathParamAgentFileID))
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", download.File.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": download.File.OriginalFilename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(download.Content)
	return err
}

// deleteAgentSessionFile deletes only a draft that has not been attached to a message.
func (c AgentCtrl) deleteAgentSessionFile(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteSessionFile(ctx, r.PathValue(PathParamID), r.PathValue(pathParamAgentFileID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
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
