package postgres

import (
	"context"
	"encoding/json"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

// UpsertEHRMSSyncRun 保存同步運行。
func (s *Store) UpsertEHRMSSyncRun(ctx context.Context, v domain.EHRMSSyncRun) error {
	_, err := s.db.Exec(tenantContext(ctx, v.TenantID), `
INSERT INTO ehrms_sync_runs (id, tenant_id, account_id, sync_type, trigger_type, status, current_step, mode, since_date, attempt, max_attempts, retry_of_run_id, request_id, trace_id, error_code, error_message, retryable, next_retry_at, summary, started_at, finished_at, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)
ON CONFLICT (id) DO UPDATE SET status=EXCLUDED.status,current_step=EXCLUDED.current_step,attempt=EXCLUDED.attempt,error_code=EXCLUDED.error_code,error_message=EXCLUDED.error_message,retryable=EXCLUDED.retryable,next_retry_at=EXCLUDED.next_retry_at,summary=EXCLUDED.summary,finished_at=EXCLUDED.finished_at,updated_at=EXCLUDED.updated_at`,
		v.ID, v.TenantID, v.AccountID, v.SyncType, v.TriggerType, v.Status, v.CurrentStep, v.Mode, v.Since, v.Attempt, v.MaxAttempts, v.RetryOfRunID, v.RequestID, v.TraceID, v.ErrorCode, v.ErrorMessage, v.Retryable, v.NextRetryAt, mustJSON(v.Summary), v.StartedAt, v.FinishedAt, v.CreatedAt, v.UpdatedAt)
	return err
}

// UpsertEHRMSSyncRunStep 保存同步步驟。
func (s *Store) UpsertEHRMSSyncRunStep(ctx context.Context, v domain.EHRMSSyncRunStep) error {
	_, err := s.db.Exec(tenantContext(ctx, v.TenantID), `
INSERT INTO ehrms_sync_run_steps (id,tenant_id,run_id,step,sequence,status,attempt,error_code,error_message,summary,started_at,finished_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (id) DO UPDATE SET status=EXCLUDED.status,error_code=EXCLUDED.error_code,error_message=EXCLUDED.error_message,summary=EXCLUDED.summary,finished_at=EXCLUDED.finished_at`,
		v.ID, v.TenantID, v.RunID, v.Step, v.Sequence, v.Status, v.Attempt, v.ErrorCode, v.ErrorMessage, mustJSON(v.Summary), v.StartedAt, v.FinishedAt)
	return err
}

// GetEHRMSSyncRun 取得同步運行。
func (s *Store) GetEHRMSSyncRun(ctx context.Context, tenantID, id string) (domain.EHRMSSyncRun, bool, error) {
	row := s.db.QueryRow(tenantContext(ctx, tenantID), `SELECT id,tenant_id,account_id,sync_type,trigger_type,status,current_step,mode,since_date,attempt,max_attempts,retry_of_run_id,request_id,trace_id,error_code,error_message,retryable,next_retry_at,summary,started_at,finished_at,created_at,updated_at FROM ehrms_sync_runs WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	value, err := scanEHRMSSyncRun(row.Scan)
	if isNotFound(err) {
		return domain.EHRMSSyncRun{}, false, nil
	}
	return value, err == nil, err
}

// ListEHRMSSyncRuns 分頁列出同步運行。
func (s *Store) ListEHRMSSyncRuns(ctx context.Context, tenantID string, page domain.PageRequest) ([]domain.EHRMSSyncRun, int, error) {
	page = utils.NormalizePageRequest(page)
	var total int
	if err := s.db.QueryRow(tenantContext(ctx, tenantID), `SELECT count(*) FROM ehrms_sync_runs WHERE tenant_id=$1`, tenantID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(tenantContext(ctx, tenantID), `SELECT id,tenant_id,account_id,sync_type,trigger_type,status,current_step,mode,since_date,attempt,max_attempts,retry_of_run_id,request_id,trace_id,error_code,error_message,retryable,next_retry_at,summary,started_at,finished_at,created_at,updated_at FROM ehrms_sync_runs WHERE tenant_id=$1 ORDER BY started_at DESC,id DESC LIMIT $2 OFFSET $3`, tenantID, page.PageSize, (page.Page-1)*page.PageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]domain.EHRMSSyncRun, 0)
	for rows.Next() {
		value, err := scanEHRMSSyncRun(rows.Scan)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
	}
	return items, total, rows.Err()
}

// ListEHRMSSyncRunSteps 列出同步步驟。
func (s *Store) ListEHRMSSyncRunSteps(ctx context.Context, tenantID, runID string) ([]domain.EHRMSSyncRunStep, error) {
	rows, err := s.db.Query(tenantContext(ctx, tenantID), `SELECT id,tenant_id,run_id,step,sequence,status,attempt,error_code,error_message,summary,started_at,finished_at FROM ehrms_sync_run_steps WHERE tenant_id=$1 AND run_id=$2 ORDER BY sequence,attempt`, tenantID, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.EHRMSSyncRunStep, 0)
	for rows.Next() {
		var value domain.EHRMSSyncRunStep
		var summary []byte
		if err := rows.Scan(&value.ID, &value.TenantID, &value.RunID, &value.Step, &value.Sequence, &value.Status, &value.Attempt, &value.ErrorCode, &value.ErrorMessage, &summary, &value.StartedAt, &value.FinishedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(summary, &value.Summary)
		items = append(items, value)
	}
	return items, rows.Err()
}

// WithEHRMSSyncLock 使用 session advisory lock 防止同租戶同類型並行同步。
func (s *Store) WithEHRMSSyncLock(ctx context.Context, tenantID, syncType string, fn func() error) (bool, error) {
	if s.pool == nil {
		return true, fn()
	}
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Release()
	key := tenantID + ":" + syncType
	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1, 0))`, key).Scan(&acquired); err != nil || !acquired {
		return acquired, err
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, key)
	}()
	return true, fn()
}

func scanEHRMSSyncRun(scan func(...any) error) (domain.EHRMSSyncRun, error) {
	var value domain.EHRMSSyncRun
	var summary []byte
	err := scan(&value.ID, &value.TenantID, &value.AccountID, &value.SyncType, &value.TriggerType, &value.Status, &value.CurrentStep, &value.Mode, &value.Since, &value.Attempt, &value.MaxAttempts, &value.RetryOfRunID, &value.RequestID, &value.TraceID, &value.ErrorCode, &value.ErrorMessage, &value.Retryable, &value.NextRetryAt, &summary, &value.StartedAt, &value.FinishedAt, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		_ = json.Unmarshal(summary, &value.Summary)
	}
	return value, err
}
