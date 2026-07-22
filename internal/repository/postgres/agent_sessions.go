package postgres

import (
	"context"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"nexus-pro-api/internal/domain"
	sqlc "nexus-pro-api/internal/platform/postgres/db"
)

// UpsertAgentSession 從儲存層處理 upsert agent 會話。
func (s *Store) UpsertAgentSession(execCtx context.Context, v domain.AgentSession) error {
	_, err := s.q.UpsertAgentSession(tenantContext(execCtx, v.TenantID), sqlc.UpsertAgentSessionParams{
		ID:             v.ID,
		TenantID:       v.TenantID,
		AccountID:      v.AccountID,
		AgentID:        v.AgentID,
		SegmentID:      v.SegmentID,
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
	return agentSessionFromFields(v.ID, v.TenantID, v.AccountID, textFrom(v.AgentID), v.SegmentID, v.Title, v.Status, v.ContextVersion, v.LastMessageAt, v.CreatedAt, v.UpdatedAt), true, nil
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
	return agentSessionFromFields(v.ID, v.TenantID, v.AccountID, textFrom(v.AgentID), v.SegmentID, v.Title, v.Status, v.ContextVersion, v.LastMessageAt, v.CreatedAt, v.UpdatedAt), true, nil
}

// ListAgentSessionsByAccount 從儲存層列出 account 的 agent 會話。
func (s *Store) ListAgentSessionsByAccount(execCtx context.Context, tenantID, accountID, status, agentID string, page domain.KeysetPage) ([]domain.AgentSession, error) {
	items, err := s.q.ListAgentSessionsByAccount(tenantContext(execCtx, tenantID), sqlc.ListAgentSessionsByAccountParams{
		TenantID:        tenantID,
		AccountID:       accountID,
		Status:          status,
		AgentID:         agentID,
		HasCursor:       page.HasCursor,
		CursorCreatedAt: timestamptz(page.CursorCreatedAt),
		CursorID:        page.CursorID,
		LimitCount:      keysetLimitCount(page.Limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.AgentSession, 0, len(items))
	for _, item := range items {
		out = append(out, agentSessionFromFields(item.ID, item.TenantID, item.AccountID, textFrom(item.AgentID), item.SegmentID, item.Title, item.Status, item.ContextVersion, item.LastMessageAt, item.CreatedAt, item.UpdatedAt))
	}
	return out, nil
}

// ListAgentUsageByAccount returns a filtered and ordered account usage page.
func (s *Store) ListAgentUsageByAccount(execCtx context.Context, tenantID string, query domain.AgentAccountUsageQuery, page domain.PageRequest) ([]domain.AgentAccountUsage, int, error) {
	ctx := tenantContext(execCtx, tenantID)
	total, err := s.q.CountAgentUsageByAccount(ctx, sqlc.CountAgentUsageByAccountParams{
		TenantID: tenantID, SearchQuery: query.Query, AccountStatus: query.Status,
	})
	if err != nil {
		return nil, 0, err
	}
	items, err := s.q.ListAgentUsageByAccount(ctx, sqlc.ListAgentUsageByAccountParams{
		TenantID: tenantID, SearchQuery: query.Query, AccountStatus: query.Status, SortOrder: page.Sort,
		LimitCount: int32(page.PageSize), OffsetCount: int32((page.Page - 1) * page.PageSize),
	})
	if err != nil {
		return nil, 0, err
	}
	return mapSlice(items, fromAgentAccountUsage), int(total), nil
}

// GetAgentUsageByAccount returns one tenant account's aggregate usage.
func (s *Store) GetAgentUsageByAccount(execCtx context.Context, tenantID, accountID string) (domain.AgentAccountUsage, bool, error) {
	item, err := s.q.GetAgentUsageByAccount(tenantContext(execCtx, tenantID), sqlc.GetAgentUsageByAccountParams{
		TenantID: tenantID, AccountID: accountID,
	})
	if isNotFound(err) {
		return domain.AgentAccountUsage{}, false, nil
	}
	if err != nil {
		return domain.AgentAccountUsage{}, false, err
	}
	return fromAgentAccountUsageFields(item.AccountID, item.DisplayName, item.Email, item.Status, item.SessionCount, item.MessageCount, item.LlmCallCount, item.InputTokens, item.CachedTokens, item.OutputTokens, item.TotalTokens, item.ActualTokens, timePtrFrom(item.LastActiveAt)), true, nil
}

// GetAgentUsageSummary returns tenant-wide totals without loading account rows.
func (s *Store) GetAgentUsageSummary(execCtx context.Context, tenantID string) (domain.AgentUsageSummary, error) {
	item, err := s.q.GetAgentUsageSummary(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return domain.AgentUsageSummary{}, err
	}
	return domain.AgentUsageSummary{
		UserCount: int(item.UserCount), UsersWithUsage: int(item.UsersWithUsage),
		SessionCount: item.SessionCount, MessageCount: item.MessageCount, LLMCallCount: item.LlmCallCount,
		InputTokens: item.InputTokens, CachedTokens: item.CachedTokens, OutputTokens: item.OutputTokens,
		TotalTokens: item.TotalTokens, ActualTokens: item.ActualTokens,
	}, nil
}

// ListAgentUsageBySession returns paginated usage for one account's sessions.
func (s *Store) ListAgentUsageBySession(execCtx context.Context, tenantID, accountID string, page domain.PageRequest) ([]domain.AgentSessionUsage, int, error) {
	ctx := tenantContext(execCtx, tenantID)
	total, err := s.q.CountAgentUsageSessionsByAccount(ctx, sqlc.CountAgentUsageSessionsByAccountParams{
		TenantID:  tenantID,
		AccountID: accountID,
	})
	if err != nil {
		return nil, 0, err
	}
	items, err := s.q.ListAgentUsageBySession(ctx, sqlc.ListAgentUsageBySessionParams{
		TenantID:    tenantID,
		AccountID:   accountID,
		LimitCount:  int32(page.PageSize),
		OffsetCount: int32((page.Page - 1) * page.PageSize),
	})
	if err != nil {
		return nil, 0, err
	}
	return mapSlice(items, fromAgentSessionUsage), int(total), nil
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
	return agentSessionFromFields(v.ID, v.TenantID, v.AccountID, textFrom(v.AgentID), v.SegmentID, v.Title, v.Status, v.ContextVersion, v.LastMessageAt, v.CreatedAt, v.UpdatedAt), true, nil
}

// InsertAgentSessionMessage 從儲存層新增 agent 會話訊息。
func (s *Store) InsertAgentSessionMessage(execCtx context.Context, v domain.AgentSessionMessage) error {
	_, err := s.q.InsertAgentSessionMessage(tenantContext(execCtx, v.TenantID), sqlc.InsertAgentSessionMessageParams{
		ID:        v.ID,
		TenantID:  v.TenantID,
		SessionID: v.SessionID,
		Role:      string(v.Role),
		Content:   v.Content,
		RunID:     v.RunID,
		Metadata:  mustJSON(v.Metadata),
		CreatedAt: timestamptz(v.CreatedAt),
	})
	return err
}

// ListAgentSessionMessages 從儲存層列出 agent 會話訊息。
func (s *Store) ListAgentSessionMessages(execCtx context.Context, tenantID, sessionID string, page domain.KeysetPage) ([]domain.AgentSessionMessage, error) {
	items, err := s.q.ListAgentSessionMessages(tenantContext(execCtx, tenantID), sqlc.ListAgentSessionMessagesParams{
		TenantID:        tenantID,
		SessionID:       sessionID,
		HasCursor:       page.HasCursor,
		CursorCreatedAt: timestamptz(page.CursorCreatedAt),
		CursorID:        page.CursorID,
		LimitCount:      keysetLimitCount(page.Limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.AgentSessionMessage, 0, len(items))
	for _, item := range items {
		out = append(out, agentSessionMessageFromFields(item.ID, item.TenantID, item.SessionID, item.SegmentID, item.SequenceNo, item.Role, item.Content, textFrom(item.RunID), item.ContextVersion, item.Metadata, item.CreatedAt))
	}
	return out, nil
}

// keysetLimitCount 將非正數 limit 視為不分頁（內部全量讀取），轉成 SQL LIMIT 參數。
func keysetLimitCount(limit int) int32 {
	if limit <= 0 {
		return math.MaxInt32
	}
	return int32(limit)
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
	out := make([]domain.AgentSessionMessage, 0, len(items))
	for _, item := range items {
		out = append(out, agentSessionMessageFromFields(item.ID, item.TenantID, item.SessionID, item.SegmentID, item.SequenceNo, item.Role, item.Content, textFrom(item.RunID), item.ContextVersion, item.Metadata, item.CreatedAt))
	}
	return out, nil
}

// FailStaleAgentRunsBySession closes interrupted runs without touching a live run inside its timeout window.
func (s *Store) FailStaleAgentRunsBySession(execCtx context.Context, tenantID, sessionID string, staleBefore, failedAt time.Time, reason string) (int, error) {
	result, err := s.db.Exec(tenantContext(execCtx, tenantID), `
UPDATE executions
SET status = 'failed',
    completed_at = $4,
    error_code = CASE WHEN error_code = '' THEN 'interrupted' ELSE error_code END,
    error_category = CASE WHEN error_category = '' THEN 'runtime' ELSE error_category END,
    safe_error_message = CASE WHEN safe_error_message = '' THEN $5 ELSE safe_error_message END,
    updated_at = $4
WHERE tenant_id = $1
  AND conversation_id = $2
  AND status IN ('queued', 'running')
  AND updated_at < $3`, tenantID, sessionID, staleBefore, failedAt, reason)
	if err != nil {
		return 0, err
	}
	return int(result.RowsAffected()), nil
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
	confidence, err := numericFromFloat64(v.Confidence)
	if err != nil {
		return err
	}
	status := v.Status
	if status == "" {
		status = "active"
	}
	_, err = s.q.UpsertAgentMemory(tenantContext(execCtx, v.TenantID), sqlc.UpsertAgentMemoryParams{
		ID: v.ID, TenantID: v.TenantID, AccountID: v.AccountID,
		AgentID: v.AgentID, SessionID: v.SessionID,
		Key: v.Key, Content: v.Content, Source: string(v.Source),
		SourceMessageID: v.SourceMessageID, Confidence: confidence,
		Importance: int32(v.Importance), Status: status,
		ExpiresAt: nullableTimestamptz(v.ExpiresAt),
		CreatedAt: timestamptz(v.CreatedAt), UpdatedAt: timestamptz(v.UpdatedAt),
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
	return agentMemoryFromFields(v.ID, v.TenantID, v.AccountID, textFrom(v.AgentID), textFrom(v.SessionID), textFrom(v.SegmentID), v.Scope, textFrom(v.SourceMessageID), float64FromNumeric(v.Confidence), v.Status, v.Key, v.Content, v.Source, v.Importance, v.ExpiresAt, v.CreatedAt, v.UpdatedAt), true, nil
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
	out := make([]domain.AgentMemory, 0, len(items))
	for _, item := range items {
		out = append(out, agentMemoryFromFields(item.ID, item.TenantID, item.AccountID, textFrom(item.AgentID), textFrom(item.SessionID), textFrom(item.SegmentID), item.Scope, textFrom(item.SourceMessageID), float64FromNumeric(item.Confidence), item.Status, item.Key, item.Content, item.Source, item.Importance, item.ExpiresAt, item.CreatedAt, item.UpdatedAt))
	}
	return out, nil
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
	return agentMemoryFromFields(v.ID, v.TenantID, v.AccountID, textFrom(v.AgentID), textFrom(v.SessionID), textFrom(v.SegmentID), v.Scope, textFrom(v.SourceMessageID), float64FromNumeric(v.Confidence), v.Status, v.Key, v.Content, v.Source, v.Importance, v.ExpiresAt, v.CreatedAt, v.UpdatedAt), true, nil
}

func agentSessionFromFields(id, tenantID, accountID, agentID, segmentID, title, status string, contextVersion int64, lastMessageAt, createdAt, updatedAt pgtype.Timestamptz) domain.AgentSession {
	return domain.AgentSession{
		ID: id, TenantID: tenantID, AccountID: accountID, AgentID: agentID, SegmentID: segmentID,
		Title: title, Status: domain.AgentSessionStatus(status), ContextVersion: contextVersion,
		LastMessageAt: timePtrFrom(lastMessageAt), CreatedAt: timeFrom(createdAt), UpdatedAt: timeFrom(updatedAt),
	}
}

func agentSessionMessageFromFields(id, tenantID, sessionID, segmentID string, sequenceNo int64, role, content, runID string, contextVersion int64, metadata []byte, createdAt pgtype.Timestamptz) domain.AgentSessionMessage {
	return domain.AgentSessionMessage{
		ID: id, TenantID: tenantID, SessionID: sessionID, SegmentID: segmentID, SequenceNo: sequenceNo,
		Role: domain.AgentMessageRole(role), Content: content, RunID: runID,
		ContextVersion: contextVersion, Metadata: jsonMap(metadata), CreatedAt: timeFrom(createdAt),
	}
}

// fromAgentAccountUsage maps the aggregate SQL row without exposing sqlc types.
func fromAgentAccountUsage(v sqlc.ListAgentUsageByAccountRow) domain.AgentAccountUsage {
	return fromAgentAccountUsageFields(v.AccountID, v.DisplayName, v.Email, v.Status, v.SessionCount, v.MessageCount, v.LlmCallCount, v.InputTokens, v.CachedTokens, v.OutputTokens, v.TotalTokens, v.ActualTokens, timePtrFrom(v.LastActiveAt))
}

// fromAgentAccountUsageFields centralizes mapping shared by list and detail queries.
func fromAgentAccountUsageFields(accountID, displayName, email, status string, sessionCount, messageCount, llmCallCount, inputTokens, cachedTokens, outputTokens, totalTokens, actualTokens int64, lastActiveAt *time.Time) domain.AgentAccountUsage {
	return domain.AgentAccountUsage{
		AccountID: accountID, DisplayName: displayName, Email: email, Status: status,
		SessionCount: sessionCount, MessageCount: messageCount, LLMCallCount: llmCallCount,
		InputTokens: inputTokens, CachedTokens: cachedTokens, OutputTokens: outputTokens,
		TotalTokens: totalTokens, ActualTokens: actualTokens, LastActiveAt: lastActiveAt,
	}
}

// fromAgentSessionUsage maps one session aggregate without leaking sqlc types.
func fromAgentSessionUsage(v sqlc.ListAgentUsageBySessionRow) domain.AgentSessionUsage {
	return domain.AgentSessionUsage{
		SessionID:    v.SessionID,
		AccountID:    v.AccountID,
		Title:        v.Title,
		Status:       domain.AgentSessionStatus(v.Status),
		MessageCount: v.MessageCount,
		LLMCallCount: v.LlmCallCount,
		InputTokens:  v.InputTokens,
		CachedTokens: v.CachedTokens,
		OutputTokens: v.OutputTokens,
		TotalTokens:  v.TotalTokens,
		ActualTokens: v.ActualTokens,
		LastActiveAt: timePtrFrom(v.LastActiveAt),
	}
}

func agentMemoryFromFields(id, tenantID, accountID, agentID, sessionID, segmentID, scope, sourceMessageID string, confidence float64, status, key, content, source string, importance int32, expiresAt, createdAt, updatedAt pgtype.Timestamptz) domain.AgentMemory {
	return domain.AgentMemory{
		ID: id, TenantID: tenantID, AccountID: accountID, AgentID: agentID, SessionID: sessionID,
		SegmentID: segmentID, Scope: scope, Key: key, Content: content,
		Source: domain.AgentMemorySource(source), SourceMessageID: sourceMessageID,
		Confidence: confidence, Status: status, Importance: int(importance),
		ExpiresAt: timePtrFrom(expiresAt), CreatedAt: timeFrom(createdAt), UpdatedAt: timeFrom(updatedAt),
	}
}
