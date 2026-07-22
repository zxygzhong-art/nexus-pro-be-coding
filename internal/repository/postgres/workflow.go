package postgres

import (
	"context"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"nexus-pro-api/internal/domain"
	sqlc "nexus-pro-api/internal/platform/postgres/db"
)

// UpsertWorkflowRun 持久化流程運行實例。
func (s *Store) UpsertWorkflowRun(execCtx context.Context, v domain.WorkflowRun) error {
	// Keep older callers compatible with the delivery columns introduced in
	// migration 000010. Outbox-managed starts always provide both values, while
	// legacy/synchronous callers historically omitted them.
	if v.TemporalStartStatus == "" {
		v.TemporalStartStatus = domain.WorkflowTemporalStartStarted
	}
	if v.TemporalWorkflowID == "" {
		v.TemporalWorkflowID = domain.FormApprovalWorkflowID(v.TenantID, v.FormInstanceID)
	}
	_, err := s.q.UpsertWorkflowRun(tenantContext(execCtx, v.TenantID), sqlc.UpsertWorkflowRunParams{
		ID:                     v.ID,
		TenantID:               v.TenantID,
		FormInstanceID:         v.FormInstanceID,
		TemplateID:             v.TemplateID,
		Version:                int32(v.Version),
		Status:                 v.Status,
		CurrentStageInstanceID: nullableText(v.CurrentStageInstanceID),
		Column8:                []byte(v.StageDefinitionsJSON),
		TemporalStartStatus:    v.TemporalStartStatus,
		TemporalWorkflowID:     v.TemporalWorkflowID,
		TemporalRunID:          v.TemporalRunID,
		TemporalStartEventID:   v.TemporalStartEventID,
		TemporalStartedAt:      nullableTimestamptz(v.TemporalStartedAt),
		CreatedAt:              timestamptz(v.CreatedAt),
		UpdatedAt:              timestamptz(v.UpdatedAt),
	})
	return err
}

// ListPendingWorkflowRuns returns durable starts that are immediately claimable.
func (s *Store) ListPendingWorkflowRuns(execCtx context.Context, tenantID string, staleBefore time.Time, limit int) ([]domain.WorkflowRun, error) {
	if limit <= 0 {
		limit = 100
	}
	items, err := s.q.ListPendingWorkflowRuns(tenantContext(execCtx, tenantID), sqlc.ListPendingWorkflowRunsParams{
		TenantID:    tenantID,
		StaleBefore: timestamptz(staleBefore),
		LimitCount:  int32(limit),
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromWorkflowRun), nil
}

// ClaimWorkflowRunTemporalStart moves a pending or stale claim into the starting state.
// The returned UpdatedAt is the database-normalized fencing token for follow-up writes.
func (s *Store) ClaimWorkflowRunTemporalStart(execCtx context.Context, tenantID, id string, claimedAt, staleBefore time.Time) (domain.WorkflowRun, bool, error) {
	v, err := s.q.ClaimWorkflowRunTemporalStart(tenantContext(execCtx, tenantID), sqlc.ClaimWorkflowRunTemporalStartParams{
		ClaimedAt:   timestamptz(claimedAt),
		TenantID:    tenantID,
		ID:          id,
		StaleBefore: timestamptz(staleBefore),
	})
	if isNotFound(err) {
		return domain.WorkflowRun{}, false, nil
	}
	if err != nil {
		return domain.WorkflowRun{}, false, err
	}
	return fromWorkflowRun(v), true, nil
}

// ReleaseWorkflowRunTemporalStart releases only the worker that owns claimedAt.
func (s *Store) ReleaseWorkflowRunTemporalStart(execCtx context.Context, tenantID, id string, claimedAt, releasedAt time.Time) (bool, error) {
	_, err := s.q.ReleaseWorkflowRunTemporalStart(tenantContext(execCtx, tenantID), sqlc.ReleaseWorkflowRunTemporalStartParams{
		ReleasedAt: timestamptz(releasedAt),
		TenantID:   tenantID,
		ID:         id,
		ClaimedAt:  timestamptz(claimedAt),
	})
	if isNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

// MarkWorkflowRunTemporalStarted records an execution only when the claim token still owns the run.
func (s *Store) MarkWorkflowRunTemporalStarted(execCtx context.Context, tenantID, id string, claimedAt time.Time, execution domain.FormApprovalWorkflowExecution, startedAt time.Time) (domain.WorkflowRun, bool, error) {
	v, err := s.q.MarkWorkflowRunTemporalStarted(tenantContext(execCtx, tenantID), sqlc.MarkWorkflowRunTemporalStartedParams{
		TemporalWorkflowID: execution.WorkflowID,
		TemporalRunID:      execution.RunID,
		StartedAt:          timestamptz(startedAt),
		TenantID:           tenantID,
		ID:                 id,
		ClaimedAt:          timestamptz(claimedAt),
	})
	if isNotFound(err) {
		return domain.WorkflowRun{}, false, nil
	}
	if err != nil {
		return domain.WorkflowRun{}, false, err
	}
	return fromWorkflowRun(v), true, nil
}

// AbandonPendingWorkflowRunTemporalStart abandons a run only before a worker claims it.
func (s *Store) AbandonPendingWorkflowRunTemporalStart(execCtx context.Context, tenantID, id string, abandonedAt time.Time) (domain.WorkflowRun, bool, error) {
	v, err := s.q.AbandonPendingWorkflowRunTemporalStart(tenantContext(execCtx, tenantID), sqlc.AbandonPendingWorkflowRunTemporalStartParams{
		AbandonedAt: timestamptz(abandonedAt),
		TenantID:    tenantID,
		ID:          id,
	})
	if isNotFound(err) {
		return domain.WorkflowRun{}, false, nil
	}
	if err != nil {
		return domain.WorkflowRun{}, false, err
	}
	return fromWorkflowRun(v), true, nil
}

// AbandonClaimedWorkflowRunTemporalStart abandons only the worker that owns claimedAt.
func (s *Store) AbandonClaimedWorkflowRunTemporalStart(execCtx context.Context, tenantID, id string, claimedAt, abandonedAt time.Time) (domain.WorkflowRun, bool, error) {
	v, err := s.q.AbandonClaimedWorkflowRunTemporalStart(tenantContext(execCtx, tenantID), sqlc.AbandonClaimedWorkflowRunTemporalStartParams{
		AbandonedAt: timestamptz(abandonedAt),
		TenantID:    tenantID,
		ID:          id,
		ClaimedAt:   timestamptz(claimedAt),
	})
	if isNotFound(err) {
		return domain.WorkflowRun{}, false, nil
	}
	if err != nil {
		return domain.WorkflowRun{}, false, err
	}
	return fromWorkflowRun(v), true, nil
}

// GetWorkflowRun 取得流程運行實例。
func (s *Store) GetWorkflowRun(execCtx context.Context, tenantID, id string) (domain.WorkflowRun, bool, error) {
	v, err := s.q.GetWorkflowRun(tenantContext(execCtx, tenantID), sqlc.GetWorkflowRunParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.WorkflowRun{}, false, nil
	}
	if err != nil {
		return domain.WorkflowRun{}, false, err
	}
	return fromWorkflowRun(v), true, nil
}

// GetWorkflowRunByFormInstance 取得單據最新流程運行實例。
func (s *Store) GetWorkflowRunByFormInstance(execCtx context.Context, tenantID, formInstanceID string) (domain.WorkflowRun, bool, error) {
	items, err := s.ListWorkflowRunsByFormInstance(execCtx, tenantID, formInstanceID)
	if err != nil {
		return domain.WorkflowRun{}, false, err
	}
	if len(items) == 0 {
		return domain.WorkflowRun{}, false, nil
	}
	return items[len(items)-1], true, nil
}

// ListWorkflowRunsByFormInstance 列出單據的所有流程運行實例。
func (s *Store) ListWorkflowRunsByFormInstance(execCtx context.Context, tenantID, formInstanceID string) ([]domain.WorkflowRun, error) {
	items, err := s.q.ListWorkflowRunsByFormInstance(tenantContext(execCtx, tenantID), sqlc.ListWorkflowRunsByFormInstanceParams{
		TenantID:       tenantID,
		FormInstanceID: formInstanceID,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromWorkflowRun), nil
}

// UpsertWorkflowStageInstance 持久化流程節點實例。
func (s *Store) UpsertWorkflowStageInstance(execCtx context.Context, v domain.WorkflowStageInstance) error {
	_, err := s.q.UpsertWorkflowStageInstance(tenantContext(execCtx, v.TenantID), sqlc.UpsertWorkflowStageInstanceParams{
		ID:          v.ID,
		TenantID:    v.TenantID,
		RunID:       v.RunID,
		StageID:     v.StageID,
		StageType:   v.StageType,
		Label:       v.Label,
		Status:      v.Status,
		Sequence:    int32(v.Sequence),
		Column9:     mustJSON(v.Result),
		StartedAt:   nullableTimestamptz(v.StartedAt),
		CompletedAt: nullableTimestamptz(v.CompletedAt),
	})
	return err
}

// GetWorkflowStageInstance 取得流程節點實例。
func (s *Store) GetWorkflowStageInstance(execCtx context.Context, tenantID, id string) (domain.WorkflowStageInstance, bool, error) {
	v, err := s.q.GetWorkflowStageInstance(tenantContext(execCtx, tenantID), sqlc.GetWorkflowStageInstanceParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.WorkflowStageInstance{}, false, nil
	}
	if err != nil {
		return domain.WorkflowStageInstance{}, false, err
	}
	return fromWorkflowStageInstance(v), true, nil
}

// ListWorkflowStageInstancesByRun 列出流程運行下的節點實例。
func (s *Store) ListWorkflowStageInstancesByRun(execCtx context.Context, tenantID, runID string) ([]domain.WorkflowStageInstance, error) {
	items, err := s.q.ListWorkflowStageInstancesByRun(tenantContext(execCtx, tenantID), sqlc.ListWorkflowStageInstancesByRunParams{
		TenantID: tenantID,
		RunID:    runID,
	})
	if err != nil {
		return nil, err
	}
	out := mapSlice(items, fromWorkflowStageInstance)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	return out, nil
}

// UpsertWorkflowStageAssignee 持久化節點待辦人。
func (s *Store) UpsertWorkflowStageAssignee(execCtx context.Context, v domain.WorkflowStageAssignee) error {
	return s.q.UpsertWorkflowStageAssignee(tenantContext(execCtx, v.TenantID), sqlc.UpsertWorkflowStageAssigneeParams{
		TenantID:        v.TenantID,
		StageInstanceID: v.StageInstanceID,
		AccountID:       v.AccountID,
		Status:          v.Status,
	})
}

// ListWorkflowStageAssignees 列出節點待辦人。
func (s *Store) ListWorkflowStageAssignees(execCtx context.Context, tenantID, stageInstanceID string) ([]domain.WorkflowStageAssignee, error) {
	items, err := s.q.ListWorkflowStageAssignees(tenantContext(execCtx, tenantID), sqlc.ListWorkflowStageAssigneesParams{
		TenantID:        tenantID,
		StageInstanceID: stageInstanceID,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromWorkflowStageAssignee), nil
}

// ListPendingAssigneeStageInstanceIDs 列出帳號待處理節點實例 ID。
func (s *Store) ListPendingAssigneeStageInstanceIDs(execCtx context.Context, tenantID, accountID string) ([]string, error) {
	return s.q.ListPendingAssigneeStageInstanceIDs(tenantContext(execCtx, tenantID), sqlc.ListPendingAssigneeStageInstanceIDsParams{
		TenantID:  tenantID,
		AccountID: accountID,
	})
}

// InsertWorkflowAction 寫入流程審批動作。
func (s *Store) InsertWorkflowAction(execCtx context.Context, v domain.WorkflowAction) error {
	_, err := s.q.InsertWorkflowAction(tenantContext(execCtx, v.TenantID), sqlc.InsertWorkflowActionParams{
		ID:                 v.ID,
		TenantID:           v.TenantID,
		RunID:              v.RunID,
		StageInstanceID:    v.StageInstanceID,
		AccountID:          v.AccountID,
		Action:             v.Action,
		Comment:            v.Comment,
		IdempotencyKey:     v.IdempotencyKey,
		CommandFingerprint: v.CommandFingerprint,
		RequestID:          v.RequestID,
		TraceID:            v.TraceID,
		CreatedAt:          timestamptz(v.CreatedAt),
	})
	return err
}

// GetWorkflowActionByIdempotencyKey loads the durable command receipt for a run.
func (s *Store) GetWorkflowActionByIdempotencyKey(execCtx context.Context, tenantID, runID, idempotencyKey string) (domain.WorkflowAction, bool, error) {
	v, err := s.q.GetWorkflowActionByIdempotencyKey(tenantContext(execCtx, tenantID), sqlc.GetWorkflowActionByIdempotencyKeyParams{
		TenantID:       tenantID,
		RunID:          runID,
		IdempotencyKey: idempotencyKey,
	})
	if isNotFound(err) {
		return domain.WorkflowAction{}, false, nil
	}
	if err != nil {
		return domain.WorkflowAction{}, false, err
	}
	return fromWorkflowAction(v), true, nil
}

// ListWorkflowActionsByRun 列出流程運行下的審批動作。
func (s *Store) ListWorkflowActionsByRun(execCtx context.Context, tenantID, runID string) ([]domain.WorkflowAction, error) {
	items, err := s.q.ListWorkflowActionsByRun(tenantContext(execCtx, tenantID), sqlc.ListWorkflowActionsByRunParams{
		TenantID: tenantID,
		RunID:    runID,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromWorkflowAction), nil
}

func fromWorkflowRun(v sqlc.WorkflowRun) domain.WorkflowRun {
	return domain.WorkflowRun{
		ID:                     v.ID,
		TenantID:               v.TenantID,
		FormInstanceID:         v.FormInstanceID,
		TemplateID:             v.TemplateID,
		Version:                int(v.Version),
		Status:                 v.Status,
		CurrentStageInstanceID: textFrom(v.CurrentStageInstanceID),
		StageDefinitionsJSON:   string(v.StageDefinitionsJson),
		TemporalStartStatus:    v.TemporalStartStatus,
		TemporalWorkflowID:     v.TemporalWorkflowID,
		TemporalRunID:          v.TemporalRunID,
		TemporalStartEventID:   v.TemporalStartEventID,
		TemporalStartedAt:      timeFromPtr(v.TemporalStartedAt),
		CreatedAt:              timeFrom(v.CreatedAt),
		UpdatedAt:              timeFrom(v.UpdatedAt),
	}
}

func fromWorkflowStageInstance(v sqlc.WorkflowStageInstance) domain.WorkflowStageInstance {
	return domain.WorkflowStageInstance{
		ID:          v.ID,
		TenantID:    v.TenantID,
		RunID:       v.RunID,
		StageID:     v.StageID,
		StageType:   v.StageType,
		Label:       v.Label,
		Status:      v.Status,
		Sequence:    int(v.Sequence),
		Result:      jsonMap(v.Result),
		StartedAt:   timeFromPtr(v.StartedAt),
		CompletedAt: timeFromPtr(v.CompletedAt),
	}
}

func fromWorkflowStageAssignee(v sqlc.WorkflowStageAssignee) domain.WorkflowStageAssignee {
	return domain.WorkflowStageAssignee{
		TenantID:        v.TenantID,
		StageInstanceID: v.StageInstanceID,
		AccountID:       v.AccountID,
		Status:          v.Status,
	}
}

func fromWorkflowAction(v sqlc.WorkflowAction) domain.WorkflowAction {
	return domain.WorkflowAction{
		ID:                 v.ID,
		TenantID:           v.TenantID,
		RunID:              v.RunID,
		StageInstanceID:    v.StageInstanceID,
		AccountID:          v.AccountID,
		Action:             v.Action,
		Comment:            v.Comment,
		IdempotencyKey:     v.IdempotencyKey,
		CommandFingerprint: v.CommandFingerprint,
		RequestID:          v.RequestID,
		TraceID:            v.TraceID,
		CreatedAt:          timeFrom(v.CreatedAt),
	}
}

func timeFromPtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}
