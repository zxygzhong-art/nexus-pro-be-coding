package service

import (
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

const (
	agentSessionHistoryLimit = 12
	agentMemoryContextLimit  = 8
	agentMemoryListLimit     = 50
	agentContextClearedEvent = "context_cleared"
)

// ListSessions 列出目前帳號的 agent 會話。
func (c AgentService) ListSessions(ctx RequestContext, query domain.ListAgentSessionsQuery) ([]domain.AgentSession, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionRead, "")
	if err != nil {
		return nil, err
	}
	return c.store.ListAgentSessionsByAccount(goContext(ctx), ctx.TenantID, account.ID, strings.TrimSpace(query.Status), strings.TrimSpace(query.AgentID))
}

// CreateSession 建立 agent 會話。
func (c AgentService) CreateSession(ctx RequestContext, input domain.CreateAgentSessionInput) (domain.AgentSession, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionCreate, "")
	if err != nil {
		return domain.AgentSession{}, err
	}
	agentID := strings.TrimSpace(input.AgentID)
	if agentID != "" {
		if _, err := c.publishedAgentDefinition(ctx, agentID); err != nil {
			return domain.AgentSession{}, err
		}
	}
	now := c.Now()
	session := domain.AgentSession{
		ID:        utils.NewID("asess"),
		TenantID:  ctx.TenantID,
		AccountID: account.ID,
		AgentID:   agentID,
		Title:     strings.TrimSpace(input.Title),
		Status:    domain.AgentSessionStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := c.store.UpsertAgentSession(goContext(ctx), session); err != nil {
		return domain.AgentSession{}, err
	}
	return session, nil
}

// GetSession 取得目前帳號的 agent 會話。
func (c AgentService) GetSession(ctx RequestContext, id string) (domain.AgentSession, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionRead, strings.TrimSpace(id))
	if err != nil {
		return domain.AgentSession{}, err
	}
	return c.currentAgentSession(ctx, account.ID, id)
}

// UpdateSession 更新 agent 會話。
func (c AgentService) UpdateSession(ctx RequestContext, id string, input domain.UpdateAgentSessionInput) (domain.AgentSession, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionUpdate, strings.TrimSpace(id))
	if err != nil {
		return domain.AgentSession{}, err
	}
	session, err := c.currentAgentSession(ctx, account.ID, id)
	if err != nil {
		return domain.AgentSession{}, err
	}
	if input.Title != nil {
		session.Title = strings.TrimSpace(*input.Title)
	}
	if input.Status != nil {
		status := domain.AgentSessionStatus(strings.TrimSpace(*input.Status))
		if status != domain.AgentSessionStatusActive && status != domain.AgentSessionStatusArchived {
			return domain.AgentSession{}, BadRequest("status must be active or archived")
		}
		session.Status = status
	}
	session.UpdatedAt = c.Now()
	if err := c.store.UpsertAgentSession(goContext(ctx), session); err != nil {
		return domain.AgentSession{}, err
	}
	return session, nil
}

// ClearSessionContext 清除後續 chat 使用的上下文，但保留可見歷史訊息。
func (c AgentService) ClearSessionContext(ctx RequestContext, id string) (domain.AgentSession, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionCreate, strings.TrimSpace(id))
	if err != nil {
		return domain.AgentSession{}, err
	}
	session, err := c.currentAgentSession(ctx, account.ID, id)
	if err != nil {
		return domain.AgentSession{}, err
	}
	if session.Status != domain.AgentSessionStatusActive {
		return domain.AgentSession{}, BadRequest("agent session is archived")
	}
	if err := c.ensureNoActiveAgentRun(ctx, session.ID); err != nil {
		return domain.AgentSession{}, err
	}
	now := c.Now()
	if err := c.store.InsertAgentSessionMessage(goContext(ctx), domain.AgentSessionMessage{
		ID:        utils.NewID("amsg"),
		TenantID:  ctx.TenantID,
		SessionID: session.ID,
		Role:      domain.AgentMessageRoleSystem,
		Content:   "Context cleared",
		Metadata:  map[string]any{"event": agentContextClearedEvent},
		CreatedAt: now,
	}); err != nil {
		return domain.AgentSession{}, err
	}
	session.LastMessageAt = &now
	session.UpdatedAt = now
	if err := c.store.UpsertAgentSession(goContext(ctx), session); err != nil {
		return domain.AgentSession{}, err
	}
	return session, nil
}

// DeleteSession 刪除 agent 會話。
func (c AgentService) DeleteSession(ctx RequestContext, id string) (domain.AgentSession, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionDelete, strings.TrimSpace(id))
	if err != nil {
		return domain.AgentSession{}, err
	}
	session, err := c.currentAgentSession(ctx, account.ID, id)
	if err != nil {
		return domain.AgentSession{}, err
	}
	deleted, ok, err := c.store.DeleteAgentSession(goContext(ctx), ctx.TenantID, session.ID)
	if err != nil {
		return domain.AgentSession{}, err
	}
	if !ok {
		return domain.AgentSession{}, NotFound("agent session", id)
	}
	return deleted, nil
}

// ListSessionMessages 列出目前帳號會話的訊息。
func (c AgentService) ListSessionMessages(ctx RequestContext, sessionID string) ([]domain.AgentSessionMessage, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionRead, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	if _, err := c.currentAgentSession(ctx, account.ID, sessionID); err != nil {
		return nil, err
	}
	return c.store.ListAgentSessionMessages(goContext(ctx), ctx.TenantID, strings.TrimSpace(sessionID))
}

// ListMemories 列出目前帳號的 agent 記憶。
func (c AgentService) ListMemories(ctx RequestContext, query domain.ListAgentMemoriesQuery) ([]domain.AgentMemory, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionRead, "")
	if err != nil {
		return nil, err
	}
	return c.store.ListAgentMemoriesByAccount(goContext(ctx), ctx.TenantID, account.ID, strings.TrimSpace(query.AgentID), strings.TrimSpace(query.SessionID), agentMemoryListLimit)
}

// CreateMemory 建立人工 agent 記憶。
func (c AgentService) CreateMemory(ctx RequestContext, input domain.CreateAgentMemoryInput) (domain.AgentMemory, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionCreate, "")
	if err != nil {
		return domain.AgentMemory{}, err
	}
	memory, err := c.normalizeAgentMemory(ctx, domain.AgentMemory{
		ID:         utils.NewID("amem"),
		TenantID:   ctx.TenantID,
		AccountID:  account.ID,
		AgentID:    strings.TrimSpace(input.AgentID),
		SessionID:  strings.TrimSpace(input.SessionID),
		Key:        strings.TrimSpace(input.Key),
		Content:    strings.TrimSpace(input.Content),
		Source:     domain.AgentMemorySourceManual,
		Importance: input.Importance,
		CreatedAt:  c.Now(),
		UpdatedAt:  c.Now(),
	})
	if err != nil {
		return domain.AgentMemory{}, err
	}
	if err := c.store.UpsertAgentMemory(goContext(ctx), memory); err != nil {
		return domain.AgentMemory{}, err
	}
	return memory, nil
}

// UpdateMemory 更新 agent 記憶。
func (c AgentService) UpdateMemory(ctx RequestContext, id string, input domain.UpdateAgentMemoryInput) (domain.AgentMemory, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionUpdate, strings.TrimSpace(id))
	if err != nil {
		return domain.AgentMemory{}, err
	}
	memory, err := c.currentAgentMemory(ctx, account.ID, id)
	if err != nil {
		return domain.AgentMemory{}, err
	}
	if input.Key != nil {
		memory.Key = strings.TrimSpace(*input.Key)
	}
	if input.Content != nil {
		memory.Content = strings.TrimSpace(*input.Content)
	}
	if input.Importance != nil {
		memory.Importance = *input.Importance
	}
	if input.ExpiresAt != nil {
		expiresAt, err := parseOptionalAgentMemoryTime(*input.ExpiresAt)
		if err != nil {
			return domain.AgentMemory{}, err
		}
		memory.ExpiresAt = expiresAt
	}
	memory.UpdatedAt = c.Now()
	memory, err = c.normalizeAgentMemory(ctx, memory)
	if err != nil {
		return domain.AgentMemory{}, err
	}
	if err := c.store.UpsertAgentMemory(goContext(ctx), memory); err != nil {
		return domain.AgentMemory{}, err
	}
	return memory, nil
}

// DeleteMemory 刪除 agent 記憶。
func (c AgentService) DeleteMemory(ctx RequestContext, id string) (domain.AgentMemory, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionDelete, strings.TrimSpace(id))
	if err != nil {
		return domain.AgentMemory{}, err
	}
	memory, err := c.currentAgentMemory(ctx, account.ID, id)
	if err != nil {
		return domain.AgentMemory{}, err
	}
	deleted, ok, err := c.store.DeleteAgentMemory(goContext(ctx), ctx.TenantID, memory.ID)
	if err != nil {
		return domain.AgentMemory{}, err
	}
	if !ok {
		return domain.AgentMemory{}, NotFound("agent memory", id)
	}
	return deleted, nil
}

func (c AgentService) currentAgentSession(ctx RequestContext, accountID, id string) (domain.AgentSession, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.AgentSession{}, BadRequest("id is required")
	}
	session, ok, err := c.store.GetAgentSession(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return domain.AgentSession{}, err
	}
	if !ok || session.AccountID != accountID {
		return domain.AgentSession{}, NotFound("agent session", id)
	}
	return session, nil
}

func (c AgentService) currentAgentMemory(ctx RequestContext, accountID, id string) (domain.AgentMemory, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.AgentMemory{}, BadRequest("id is required")
	}
	memory, ok, err := c.store.GetAgentMemory(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return domain.AgentMemory{}, err
	}
	if !ok || memory.AccountID != accountID {
		return domain.AgentMemory{}, NotFound("agent memory", id)
	}
	return memory, nil
}

func (c AgentService) normalizeAgentMemory(ctx RequestContext, memory domain.AgentMemory) (domain.AgentMemory, error) {
	memory.Key = strings.TrimSpace(memory.Key)
	memory.Content = strings.TrimSpace(memory.Content)
	if memory.Content == "" {
		return domain.AgentMemory{}, BadRequest("content is required")
	}
	if memory.Source == "" {
		memory.Source = domain.AgentMemorySourceManual
	}
	if memory.Source != domain.AgentMemorySourceAuto && memory.Source != domain.AgentMemorySourceManual {
		return domain.AgentMemory{}, BadRequest("source must be auto or manual")
	}
	if memory.Importance <= 0 {
		memory.Importance = 1
	}
	if memory.SessionID != "" {
		session, err := c.currentAgentSession(ctx, memory.AccountID, memory.SessionID)
		if err != nil {
			return domain.AgentMemory{}, err
		}
		if memory.AgentID == "" {
			memory.AgentID = session.AgentID
		}
	}
	return memory, nil
}

func parseOptionalAgentMemoryTime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, BadRequest("expires_at must be RFC3339")
	}
	t = t.UTC()
	return &t, nil
}

func agentSessionTitleFromMessage(message string) string {
	runes := []rune(strings.TrimSpace(message))
	if len(runes) > 40 {
		runes = runes[:40]
	}
	return string(runes)
}
