package agent

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

const (
	agentSessionHistoryLimit = 12
	agentMemoryContextLimit  = 8
	agentMemoryListLimit     = 50
	agentContextClearedEvent = "context_cleared"
)

// ListSessions 列出目前帳號的 agent 會話。
func (c AgentService) ListSessions(ctx RequestContext, query domain.ListAgentSessionsQuery) (domain.AgentSessionListPage, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionRead, "")
	if err != nil {
		return domain.AgentSessionListPage{}, err
	}
	page, err := normalizeAgentKeysetPage(query.Cursor, query.PageSize)
	if err != nil {
		return domain.AgentSessionListPage{}, err
	}
	fetch := page
	fetch.Limit++
	items, err := c.store.ListAgentSessionsByAccount(goContext(ctx), ctx.TenantID, account.ID, strings.TrimSpace(query.Status), strings.TrimSpace(query.AgentID), fetch)
	if err != nil {
		return domain.AgentSessionListPage{}, err
	}
	nextCursor := ""
	if len(items) > page.Limit {
		items = items[:page.Limit]
		last := items[len(items)-1]
		nextCursor = encodeAgentKeysetCursor(last.CreatedAt, last.ID)
	}
	if items == nil {
		items = []domain.AgentSession{}
	}
	return domain.AgentSessionListPage{Items: items, NextCursor: nextCursor}, nil
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
		ID:             utils.NewID("asess"),
		TenantID:       ctx.TenantID,
		AccountID:      account.ID,
		AgentID:        agentID,
		SegmentID:      utils.NewID("aseg"),
		Title:          strings.TrimSpace(input.Title),
		Status:         domain.AgentSessionStatusActive,
		ContextVersion: 1,
		CreatedAt:      now,
		UpdatedAt:      now,
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
	return c.CurrentAgentSession(ctx, account.ID, id)
}

// UpdateSession 更新 agent 會話。
func (c AgentService) UpdateSession(ctx RequestContext, id string, input domain.UpdateAgentSessionInput) (domain.AgentSession, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionUpdate, strings.TrimSpace(id))
	if err != nil {
		return domain.AgentSession{}, err
	}
	var session domain.AgentSession
	if err := c.withTransaction(ctx, func(tx AgentService) error {
		locked, err := tx.lockCurrentAgentSession(ctx, account.ID, id)
		if err != nil {
			return err
		}
		if input.Title != nil {
			locked.Title = strings.TrimSpace(*input.Title)
		}
		if input.Status != nil {
			status := domain.AgentSessionStatus(strings.TrimSpace(*input.Status))
			if status != domain.AgentSessionStatusActive && status != domain.AgentSessionStatusArchived {
				return BadRequest("status must be active or archived")
			}
			locked.Status = status
		}
		locked.UpdatedAt = c.Now()
		if err := tx.store.UpsertAgentSession(goContext(ctx), locked); err != nil {
			return err
		}
		session = locked
		return nil
	}); err != nil {
		return domain.AgentSession{}, err
	}
	return session, nil
}

// ClearSessionContext advances the visible context partition without deleting audit history.
func (c AgentService) ClearSessionContext(ctx RequestContext, id string) (domain.AgentSession, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionCreate, strings.TrimSpace(id))
	if err != nil {
		return domain.AgentSession{}, err
	}
	var session domain.AgentSession
	if err := c.withTransaction(ctx, func(tx AgentService) error {
		locked, err := tx.lockCurrentAgentSession(ctx, account.ID, id)
		if err != nil {
			return err
		}
		if locked.Status != domain.AgentSessionStatusActive {
			return BadRequest("agent session is archived").WithReasonCode("agent_session_archived")
		}
		if err := tx.ensureNoActiveAgentRun(ctx, locked.ID); err != nil {
			return err
		}
		if locked.ContextVersion <= 0 {
			locked.ContextVersion = 1
		}
		locked.ContextVersion++
		locked.SegmentID = utils.NewID("aseg")
		locked.Title = "新對話"
		locked.LastMessageAt = nil
		locked.UpdatedAt = c.Now()
		if err := tx.store.UpsertAgentSession(goContext(ctx), locked); err != nil {
			return err
		}
		session = locked
		return nil
	}); err != nil {
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
	session, err := c.CurrentAgentSession(ctx, account.ID, id)
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
func (c AgentService) ListSessionMessages(ctx RequestContext, sessionID string, query domain.ListAgentSessionMessagesQuery) (domain.AgentSessionMessageListPage, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionRead, strings.TrimSpace(sessionID))
	if err != nil {
		return domain.AgentSessionMessageListPage{}, err
	}
	session, err := c.CurrentAgentSession(ctx, account.ID, sessionID)
	if err != nil {
		return domain.AgentSessionMessageListPage{}, err
	}
	page, err := normalizeAgentKeysetPage(query.Cursor, query.PageSize)
	if err != nil {
		return domain.AgentSessionMessageListPage{}, err
	}
	fetch := page
	fetch.Limit++
	messages, err := c.store.ListAgentSessionMessages(goContext(ctx), ctx.TenantID, strings.TrimSpace(sessionID), fetch)
	if err != nil {
		return domain.AgentSessionMessageListPage{}, err
	}
	nextCursor := ""
	if len(messages) > page.Limit {
		messages = messages[:page.Limit]
		last := messages[len(messages)-1]
		nextCursor = encodeAgentKeysetCursor(last.CreatedAt, last.ID)
	}
	attachments, err := c.store.ListCurrentAgentMessageAttachments(goContext(ctx), ctx.TenantID, strings.TrimSpace(sessionID))
	if err != nil {
		return domain.AgentSessionMessageListPage{}, err
	}
	byMessage := make(map[string][]domain.AgentSessionFile)
	for _, attachment := range attachments {
		byMessage[attachment.MessageID] = append(byMessage[attachment.MessageID], attachment.File)
	}
	for index := range messages {
		messages[index].Attachments = byMessage[messages[index].ID]
	}
	if !page.HasCursor {
		confirmations, err := c.PendingAgentConfirmationMessages(ctx, account.ID, session)
		if err != nil {
			return domain.AgentSessionMessageListPage{}, err
		}
		messages = append(messages, confirmations...)
	}
	if messages == nil {
		messages = []domain.AgentSessionMessage{}
	}
	return domain.AgentSessionMessageListPage{Items: messages, NextCursor: nextCursor}, nil
}

// ListMemories 列出目前帳號的 agent 記憶。
func (c AgentService) ListMemories(ctx RequestContext, query domain.ListAgentMemoriesQuery) ([]domain.AgentMemory, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionRead, "")
	if err != nil {
		return nil, err
	}
	items, err := c.store.ListAgentMemoriesByAccount(goContext(ctx), ctx.TenantID, account.ID, strings.TrimSpace(query.AgentID), strings.TrimSpace(query.SessionID), agentMemoryListLimit)
	if err != nil {
		return nil, err
	}
	out := make([]domain.AgentMemory, 0, len(items))
	for _, item := range items {
		if item.Key != AgentConfirmationMemoryKey {
			out = append(out, item)
		}
	}
	return out, nil
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

// lockCurrentAgentSession serializes writes that must preserve the current context partition.
func (c AgentService) lockCurrentAgentSession(ctx RequestContext, accountID, id string) (domain.AgentSession, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.AgentSession{}, BadRequest("id is required")
	}
	session, ok, err := c.store.GetAgentSessionForUpdate(goContext(ctx), ctx.TenantID, id)
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
	memory.AgentID = strings.TrimSpace(memory.AgentID)
	memory.SessionID = strings.TrimSpace(memory.SessionID)
	memory.SegmentID = strings.TrimSpace(memory.SegmentID)
	memory.SourceMessageID = strings.TrimSpace(memory.SourceMessageID)
	memory.Key = strings.TrimSpace(memory.Key)
	memory.Content = strings.TrimSpace(memory.Content)
	if memory.Key == "" {
		return domain.AgentMemory{}, BadRequest("key is required")
	}
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
	if memory.Importance > 5 {
		return domain.AgentMemory{}, BadRequest("importance must be between 1 and 5")
	}
	if memory.Confidence == 0 {
		memory.Confidence = 1
	}
	if memory.Confidence < 0 || memory.Confidence > 1 {
		return domain.AgentMemory{}, BadRequest("confidence must be between 0 and 1")
	}
	if memory.Status == "" {
		memory.Status = "active"
	}
	if memory.SessionID != "" {
		session, err := c.CurrentAgentSession(ctx, memory.AccountID, memory.SessionID)
		if err != nil {
			return domain.AgentMemory{}, err
		}
		if memory.AgentID != "" && memory.AgentID != session.AgentID {
			return domain.AgentMemory{}, BadRequest("agent id does not match the session")
		}
		memory.Scope = "conversation"
		memory.AgentID = ""
		memory.SegmentID = session.SegmentID
	} else if memory.AgentID != "" {
		memory.Scope = "agent"
		memory.SegmentID = ""
	} else {
		memory.Scope = "global"
		memory.SegmentID = ""
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

// normalizeAgentKeysetPage 正規化 keyset 分頁參數並解碼 (created_at, id) 遊標。
func normalizeAgentKeysetPage(rawCursor string, pageSize int) (domain.KeysetPage, error) {
	if pageSize <= 0 {
		pageSize = domain.DefaultPageSize
	}
	if pageSize > domain.MaxPageSize {
		pageSize = domain.MaxPageSize
	}
	page := domain.KeysetPage{Limit: pageSize}
	rawCursor = strings.TrimSpace(rawCursor)
	if rawCursor == "" {
		return page, nil
	}
	createdAt, id, err := decodeAgentKeysetCursor(rawCursor)
	if err != nil {
		return domain.KeysetPage{}, BadRequest("cursor is invalid")
	}
	page.HasCursor = true
	page.CursorCreatedAt = createdAt
	page.CursorID = id
	return page, nil
}

// encodeAgentKeysetCursor 將最後一列的穩定排序鍵 (created_at, id) 序列化。
func encodeAgentKeysetCursor(createdAt time.Time, id string) string {
	raw := fmt.Sprintf("%d|%s", createdAt.UTC().UnixNano(), id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeAgentKeysetCursor 解析 encodeAgentKeysetCursor 產生的遊標。
func decodeAgentKeysetCursor(cursor string) (time.Time, string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return time.Time{}, "", fmt.Errorf("invalid cursor")
	}
	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, "", err
	}
	return time.Unix(0, nanos).UTC(), parts[1], nil
}

func agentSessionTitleFromMessage(message string) string {
	runes := []rune(strings.TrimSpace(message))
	if len(runes) > 40 {
		runes = runes[:40]
	}
	return string(runes)
}
