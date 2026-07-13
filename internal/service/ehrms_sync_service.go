package service

import (
	"errors"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

// EHRMSService 提供同步運行的運維查詢與人工恢復。
type EHRMSService struct{ *Service }

// EHRMS 建立 eHRMS 運維服務。
func (c *Service) EHRMS() EHRMSService { return EHRMSService{Service: c} }

// StartSync 啟動一次可追蹤的手動同步。
func (c EHRMSService) StartSync(ctx RequestContext, input StartEHRMSSyncInput) (EHRMSSyncRunDetail, error) {
	mode, err := normalizeEHRMSSyncMode(input.Mode)
	if err != nil {
		return EHRMSSyncRunDetail{}, err
	}
	since := strings.TrimSpace(input.Since)
	now := c.Now()
	syncType := "employees"
	if input.IncludeAttendance {
		syncType = "pipeline"
	}
	run := domain.EHRMSSyncRun{ID: utils.NewID("esr"), TenantID: ctx.TenantID, AccountID: ctx.AccountID, SyncType: syncType, TriggerType: "manual", Status: domain.EHRMSSyncRunStatusRunning, Mode: mode, Since: since, Attempt: 1, MaxAttempts: 1, RequestID: ctx.RequestID, TraceID: ctx.TraceID, StartedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := c.store.UpsertEHRMSSyncRun(goContext(ctx), run); err != nil {
		return EHRMSSyncRunDetail{}, err
	}
	acquired, lockErr := c.store.WithEHRMSSyncLock(goContext(ctx), ctx.TenantID, "ehrms", func() error { return c.executeRetry(ctx, &run) })
	if lockErr != nil && !acquired {
		finishSyncRun(&run, domain.EHRMSSyncRunStatusFailed, "lock_failed", safeSyncError(lockErr), false, c.Now())
		_ = c.store.UpsertEHRMSSyncRun(goContext(ctx), run)
		return EHRMSSyncRunDetail{}, lockErr
	}
	if !acquired {
		finishSyncRun(&run, domain.EHRMSSyncRunStatusSkipped, "sync_in_progress", "another eHRMS sync is already running", false, c.Now())
		_ = c.store.UpsertEHRMSSyncRun(goContext(ctx), run)
		return EHRMSSyncRunDetail{}, Conflict("another eHRMS sync is already running")
	}
	return c.getSyncRun(ctx, run.ID)
}

// ListSyncRunPage 列出當前租戶的同步運行。
func (c EHRMSService) ListSyncRunPage(ctx RequestContext, page PageRequest) (PageResponse[EHRMSSyncRun], error) {
	if _, _, err := c.requireServiceAuthz(ctx, AppHR, ResourceEmployee, ActionRead, ""); err != nil {
		return PageResponse[EHRMSSyncRun]{}, err
	}
	page = utils.NormalizePageRequest(page)
	items, total, err := c.store.ListEHRMSSyncRuns(goContext(ctx), ctx.TenantID, page)
	if err != nil {
		return PageResponse[EHRMSSyncRun]{}, err
	}
	return PageResponse[EHRMSSyncRun]{Items: items, Total: total, Page: page.Page, PageSize: page.PageSize, Sort: page.Sort}, nil
}

// GetSyncRun 取得同步運行與步驟。
func (c EHRMSService) GetSyncRun(ctx RequestContext, id string) (EHRMSSyncRunDetail, error) {
	if _, _, err := c.requireServiceAuthz(ctx, AppHR, ResourceEmployee, ActionRead, id); err != nil {
		return EHRMSSyncRunDetail{}, err
	}
	return c.getSyncRun(ctx, id)
}

// RetrySyncRun 重新執行失敗或部分成功的同步運行。
func (c EHRMSService) RetrySyncRun(ctx RequestContext, id string, input RetryEHRMSSyncRunInput) (EHRMSSyncRunDetail, error) {
	original, ok, err := c.store.GetEHRMSSyncRun(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return EHRMSSyncRunDetail{}, err
	}
	if !ok {
		return EHRMSSyncRunDetail{}, NotFound("ehrms_sync_run", id)
	}
	if original.Status != domain.EHRMSSyncRunStatusFailed && original.Status != domain.EHRMSSyncRunStatusPartial {
		return EHRMSSyncRunDetail{}, Conflict("only failed or partial eHRMS sync runs can be retried")
	}
	mode := strings.TrimSpace(input.Mode)
	if mode == "" {
		mode = original.Mode
	}
	since := strings.TrimSpace(input.Since)
	if since == "" {
		since = original.Since
	}
	now := c.Now()
	run := domain.EHRMSSyncRun{ID: utils.NewID("esr"), TenantID: ctx.TenantID, AccountID: ctx.AccountID, SyncType: original.SyncType, TriggerType: "retry", Status: domain.EHRMSSyncRunStatusRunning, Mode: mode, Since: since, Attempt: 1, MaxAttempts: 1, RetryOfRunID: original.ID, RequestID: ctx.RequestID, TraceID: ctx.TraceID, StartedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := c.store.UpsertEHRMSSyncRun(goContext(ctx), run); err != nil {
		return EHRMSSyncRunDetail{}, err
	}
	acquired, execErr := c.store.WithEHRMSSyncLock(goContext(ctx), ctx.TenantID, "ehrms", func() error {
		return c.executeRetry(ctx, &run)
	})
	if execErr != nil && !acquired {
		finishSyncRun(&run, domain.EHRMSSyncRunStatusFailed, "lock_failed", safeSyncError(execErr), false, c.Now())
		_ = c.store.UpsertEHRMSSyncRun(goContext(ctx), run)
		return EHRMSSyncRunDetail{}, execErr
	}
	if !acquired {
		finishSyncRun(&run, domain.EHRMSSyncRunStatusSkipped, "sync_in_progress", "another eHRMS sync is already running", false, c.Now())
		_ = c.store.UpsertEHRMSSyncRun(goContext(ctx), run)
		return EHRMSSyncRunDetail{}, Conflict("another eHRMS sync is already running")
	}
	if execErr != nil {
		return c.getSyncRun(ctx, run.ID)
	}
	return c.getSyncRun(ctx, run.ID)
}

func (c EHRMSService) executeRetry(ctx RequestContext, run *domain.EHRMSSyncRun) error {
	var err error
	if run.SyncType == "pipeline" || run.SyncType == "employees" {
		err = c.executeRetryStep(ctx, run, "employees", 1, func() (map[string]any, error) {
			result, stepErr := c.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{Mode: run.Mode})
			return employeeSyncSummary(result), stepErr
		})
	}
	if err == nil && (run.SyncType == "pipeline" || run.SyncType == "attendance") {
		err = c.executeRetryStep(ctx, run, "attendance", 2, func() (map[string]any, error) {
			result, stepErr := c.Attendance().SyncEHRMSAttendance(ctx, domain.EHRMSAttendanceSyncInput{Mode: run.Mode, Since: run.Since})
			return attendanceSyncSummary(result), stepErr
		})
	}
	status := domain.EHRMSSyncRunStatusSucceeded
	if err != nil {
		status = domain.EHRMSSyncRunStatusFailed
	} else if syncSummaryHasFailures(run.Summary) {
		status = domain.EHRMSSyncRunStatusPartial
	}
	finishSyncRun(run, status, syncErrorCode(err), safeSyncError(err), isRetryableSyncError(err), c.Now())
	_ = c.store.UpsertEHRMSSyncRun(goContext(ctx), *run)
	return err
}

func (c EHRMSService) executeRetryStep(ctx RequestContext, run *domain.EHRMSSyncRun, name string, sequence int, fn func() (map[string]any, error)) error {
	started := c.Now()
	run.CurrentStep = name
	run.UpdatedAt = started
	_ = c.store.UpsertEHRMSSyncRun(goContext(ctx), *run)
	step := domain.EHRMSSyncRunStep{ID: utils.NewID("ess"), TenantID: ctx.TenantID, RunID: run.ID, Step: name, Sequence: sequence, Status: domain.EHRMSSyncRunStatusRunning, Attempt: 1, StartedAt: started}
	_ = c.store.UpsertEHRMSSyncRunStep(goContext(ctx), step)
	summary, err := fn()
	finished := c.Now()
	step.Summary, step.FinishedAt = summary, &finished
	step.Status = domain.EHRMSSyncRunStatusSucceeded
	if err != nil {
		step.Status, step.ErrorCode, step.ErrorMessage = domain.EHRMSSyncRunStatusFailed, syncErrorCode(err), safeSyncError(err)
	} else if syncSummaryHasFailures(map[string]any{name: summary}) {
		step.Status = domain.EHRMSSyncRunStatusPartial
	}
	_ = c.store.UpsertEHRMSSyncRunStep(goContext(ctx), step)
	run.Summary = mergeSyncSummary(run.Summary, name, summary)
	return err
}

func (c EHRMSService) getSyncRun(ctx RequestContext, id string) (EHRMSSyncRunDetail, error) {
	run, ok, err := c.store.GetEHRMSSyncRun(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return EHRMSSyncRunDetail{}, err
	}
	if !ok {
		return EHRMSSyncRunDetail{}, NotFound("ehrms_sync_run", id)
	}
	steps, err := c.store.ListEHRMSSyncRunSteps(goContext(ctx), ctx.TenantID, id)
	return EHRMSSyncRunDetail{Run: run, Steps: steps}, err
}

func finishSyncRun(run *domain.EHRMSSyncRun, status, code, message string, retryable bool, now time.Time) {
	run.Status, run.ErrorCode, run.ErrorMessage, run.Retryable = status, code, message, retryable
	run.CurrentStep, run.NextRetryAt, run.FinishedAt, run.UpdatedAt = "", nil, &now, now
}

func employeeSyncSummary(v domain.EHRMSEmployeeSyncResponse) map[string]any {
	return map[string]any{"fetched": v.Fetched, "created": v.Created, "updated": v.Updated, "skipped": v.Skipped, "failed": v.Failed, "departments_upserted": v.DepartmentsUpserted, "positions_upserted": v.PositionsUpserted}
}
func attendanceSyncSummary(v domain.EHRMSAttendanceSyncResponse) map[string]any {
	return map[string]any{"fetched": v.Fetched, "created": v.Created, "updated": v.Updated, "skipped": v.Skipped, "failed": v.Failed, "leave_balances_failed": v.LeaveBalancesFailed, "leave_details_failed": v.LeaveDetailsFailed}
}
func mergeSyncSummary(dst map[string]any, step string, summary map[string]any) map[string]any {
	if dst == nil {
		dst = map[string]any{}
	}
	dst[step] = summary
	return dst
}
func syncSummaryHasFailures(summary map[string]any) bool {
	for _, raw := range summary {
		step, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		for key, value := range step {
			if !strings.HasSuffix(key, "failed") && key != "failed" {
				continue
			}
			if count, ok := value.(int); ok && count > 0 {
				return true
			}
		}
	}
	return false
}
func safeSyncError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}
func syncErrorCode(err error) string {
	if err == nil {
		return ""
	}
	if isRetryableSyncError(err) {
		return "temporary_failure"
	}
	return "sync_failed"
}
func isRetryableSyncError(err error) bool {
	if err == nil {
		return false
	}
	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		return appErr.ReasonCode == "ehrms_temporary_failure"
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "fetch ehrms") || strings.Contains(s, "timeout") || strings.Contains(s, "temporar") || strings.Contains(s, "connection") || strings.Contains(s, "unavailable")
}

func ehrmsFetchError(label string, err error) *domain.AppError {
	appErr := domain.BadRequest("fetch eHRMS " + label + " failed")
	var temporary interface{ Temporary() bool }
	if errors.As(err, &temporary) && temporary.Temporary() {
		appErr.ReasonCode = "ehrms_temporary_failure"
	} else {
		appErr.ReasonCode = "ehrms_permanent_failure"
	}
	return appErr
}
