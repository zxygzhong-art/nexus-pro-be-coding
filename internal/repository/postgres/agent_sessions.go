package postgres

import (
	"context"

	"nexus-pro-be/internal/domain"
	sqlc "nexus-pro-be/internal/platform/postgres/db"
)

// UpsertAgentSession 從儲存層處理 upsert agent 會話。
func (s *Store) UpsertAgentSession(execCtx context.Context, v domain.AgentSession) error {
	_, err := s.q.UpsertAgentSession(tenantContext(execCtx, v.TenantID), sqlc.UpsertAgentSessionParams{
		ID:             v.ID,
		TenantID:       v.TenantID,
		AccountID:      v.AccountID,
		AgentID:        nullableText(v.AgentID),
		Title:          v.Title,
		Status:         string(v.Status),
		ContextVersion: v.ContextVersion,
		LastMessageAt:  nullableTimestamptz(v.LastMessageAt),
		CreatedAt:      timestamptz(v.CreatedAt),
		UpdatedAt:      timestamptz(v.UpdatedAt),
	})
	return err
}

// GetAgentSession 從儲存層取得 agent 會話。
func (s *Store) GetAgentSession(execCtx context.Context, tenantID, id string) (domain.AgentSession, bool, error) {
	v, err := s.q.GetAgentSession(tenantContext(execCtx, tenantID), sqlc.GetAgentSessionParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AgentSession{}, false, nil
	}
	if err != nil {
		return domain.AgentSession{}, false, err
	}
	return fromAgentSession(v), true, nil
}

// GetAgentSessionForUpdate locks an agent session for a context-version write transaction.
func (s *Store) GetAgentSessionForUpdate(execCtx context.Context, tenantID, id string) (domain.AgentSession, bool, error) {
	v, err := s.q.GetAgentSessionForUpdate(tenantContext(execCtx, tenantID), sqlc.GetAgentSessionForUpdateParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AgentSession{}, false, nil
	}
	if err != nil {
		return domain.AgentSession{}, false, err
	}
	return fromAgentSession(v), true, nil
}

// ListAgentSessionsByAccount 從儲存層列出 account 的 agent 會話。
func (s *Store) ListAgentSessionsByAccount(execCtx context.Context, tenantID, accountID, status, agentID string) ([]domain.AgentSession, error) {
	items, err := s.q.ListAgentSessionsByAccount(tenantContext(execCtx, tenantID), sqlc.ListAgentSessionsByAccountParams{
		TenantID:  tenantID,
		AccountID: accountID,
		Status:    status,
		AgentID:   agentID,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAgentSession), nil
}

// DeleteAgentSession 從儲存層刪除 agent 會話。
func (s *Store) DeleteAgentSession(execCtx context.Context, tenantID, id string) (domain.AgentSession, bool, error) {
	v, err := s.q.DeleteAgentSession(tenantContext(execCtx, tenantID), sqlc.DeleteAgentSessionParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AgentSession{}, false, nil
	}
	if err != nil {
		return domain.AgentSession{}, false, err
	}
	return fromAgentSession(v), true, nil
}

// InsertAgentSessionMessage 從儲存層新增 agent 會話訊息。
func (s *Store) InsertAgentSessionMessage(execCtx context.Context, v domain.AgentSessionMessage) error {
	_, err := s.q.InsertAgentSessionMessage(tenantContext(execCtx, v.TenantID), sqlc.InsertAgentSessionMessageParams{
		ID:             v.ID,
		TenantID:       v.TenantID,
		SessionID:      v.SessionID,
		Role:           string(v.Role),
		Content:        v.Content,
		RunID:          nullableText(v.RunID),
		ContextVersion: v.ContextVersion,
		Metadata:       mustJSON(v.Metadata),
		CreatedAt:      timestamptz(v.CreatedAt),
	})
	return err
}

// ListAgentSessionMessages 從儲存層列出 agent 會話訊息。
func (s *Store) ListAgentSessionMessages(execCtx context.Context, tenantID, sessionID string) ([]domain.AgentSessionMessage, error) {
	items, err := s.q.ListAgentSessionMessages(tenantContext(execCtx, tenantID), sqlc.ListAgentSessionMessagesParams{TenantID: tenantID, SessionID: sessionID})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAgentSessionMessage), nil
}

// ListRecentAgentSessionMessages 從儲存層列出最近 agent 會話訊息。
func (s *Store) ListRecentAgentSessionMessages(execCtx context.Context, tenantID, sessionID string, limit int) ([]domain.AgentSessionMessage, error) {
	if limit <= 0 {
		return []domain.AgentSessionMessage{}, nil
	}
	items, err := s.q.ListRecentAgentSessionMessages(tenantContext(execCtx, tenantID), sqlc.ListRecentAgentSessionMessagesParams{
		TenantID:   tenantID,
		SessionID:  sessionID,
		LimitCount: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAgentSessionMessage), nil
}

// CountActiveAgentRunsBySession 從儲存層統計會話中的未完成 agent run。
func (s *Store) CountActiveAgentRunsBySession(execCtx context.Context, tenantID, sessionID string) (int, error) {
	count, err := s.q.CountActiveAgentRunsBySession(tenantContext(execCtx, tenantID), sqlc.CountActiveAgentRunsBySessionParams{
		TenantID:  tenantID,
		SessionID: sessionID,
	})
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// UpsertAgentMemory 從儲存層處理 upsert agent 記憶。
func (s *Store) UpsertAgentMemory(execCtx context.Context, v domain.AgentMemory) error {
	_, err := s.q.UpsertAgentMemory(tenantContext(execCtx, v.TenantID), sqlc.UpsertAgentMemoryParams{
		ID:         v.ID,
		TenantID:   v.TenantID,
		AccountID:  v.AccountID,
		AgentID:    nullableText(v.AgentID),
		SessionID:  nullableText(v.SessionID),
		Key:        v.Key,
		Content:    v.Content,
		Source:     string(v.Source),
		Importance: int32(v.Importance),
		ExpiresAt:  nullableTimestamptz(v.ExpiresAt),
		CreatedAt:  timestamptz(v.CreatedAt),
		UpdatedAt:  timestamptz(v.UpdatedAt),
	})
	return err
}

// GetAgentMemory 從儲存層取得 agent 記憶。
func (s *Store) GetAgentMemory(execCtx context.Context, tenantID, id string) (domain.AgentMemory, bool, error) {
	v, err := s.q.GetAgentMemory(tenantContext(execCtx, tenantID), sqlc.GetAgentMemoryParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AgentMemory{}, false, nil
	}
	if err != nil {
		return domain.AgentMemory{}, false, err
	}
	return fromAgentMemory(v), true, nil
}

// ListAgentMemoriesByAccount 從儲存層列出 account 的 agent 記憶。
func (s *Store) ListAgentMemoriesByAccount(execCtx context.Context, tenantID, accountID, agentID, sessionID string, limit int) ([]domain.AgentMemory, error) {
	if limit <= 0 {
		return []domain.AgentMemory{}, nil
	}
	items, err := s.q.ListAgentMemoriesByAccount(tenantContext(execCtx, tenantID), sqlc.ListAgentMemoriesByAccountParams{
		TenantID:   tenantID,
		AccountID:  accountID,
		AgentID:    agentID,
		SessionID:  sessionID,
		LimitCount: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAgentMemory), nil
}

// DeleteAgentMemory 從儲存層刪除 agent 記憶。
func (s *Store) DeleteAgentMemory(execCtx context.Context, tenantID, id string) (domain.AgentMemory, bool, error) {
	v, err := s.q.DeleteAgentMemory(tenantContext(execCtx, tenantID), sqlc.DeleteAgentMemoryParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AgentMemory{}, false, nil
	}
	if err != nil {
		return domain.AgentMemory{}, false, err
	}
	return fromAgentMemory(v), true, nil
}

func fromAgentSession(v sqlc.AgentSession) domain.AgentSession {
	return domain.AgentSession{
		ID:             v.ID,
		TenantID:       v.TenantID,
		AccountID:      v.AccountID,
		AgentID:        textFrom(v.AgentID),
		Title:          v.Title,
		Status:         domain.AgentSessionStatus(v.Status),
		ContextVersion: v.ContextVersion,
		LastMessageAt:  timePtrFrom(v.LastMessageAt),
		CreatedAt:      timeFrom(v.CreatedAt),
		UpdatedAt:      timeFrom(v.UpdatedAt),
	}
}

func fromAgentSessionMessage(v sqlc.AgentSessionMessage) domain.AgentSessionMessage {
	return domain.AgentSessionMessage{
		ID:             v.ID,
		TenantID:       v.TenantID,
		SessionID:      v.SessionID,
		Role:           domain.AgentMessageRole(v.Role),
		Content:        v.Content,
		RunID:          textFrom(v.RunID),
		ContextVersion: v.ContextVersion,
		Metadata:       jsonMap(v.Metadata),
		CreatedAt:      timeFrom(v.CreatedAt),
	}
}

func fromAgentMemory(v sqlc.AgentMemory) domain.AgentMemory {
	return domain.AgentMemory{
		ID:         v.ID,
		TenantID:   v.TenantID,
		AccountID:  v.AccountID,
		AgentID:    textFrom(v.AgentID),
		SessionID:  textFrom(v.SessionID),
		Key:        v.Key,
		Content:    v.Content,
		Source:     domain.AgentMemorySource(v.Source),
		Importance: int(v.Importance),
		ExpiresAt:  timePtrFrom(v.ExpiresAt),
		CreatedAt:  timeFrom(v.CreatedAt),
		UpdatedAt:  timeFrom(v.UpdatedAt),
	}
}
