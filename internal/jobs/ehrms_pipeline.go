package jobs

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository"
	"nexus-pro-be/internal/utils"
)

// EHRMSPipelineOptions 定義 eHRMS 有序同步 pipeline 選項。
type EHRMSPipelineOptions struct {
	Interval             time.Duration
	RunOnStart           bool
	EmployeeMode         string
	EmployeeTenantID     string
	EmployeeAccountID    string
	AttendanceEnabled    bool
	AttendanceInterval   time.Duration
	AttendanceRunOnStart bool
	AttendanceMode       string
	AttendanceSince      string
	AttendanceTenant     string
	AttendanceAccount    string
	TriggerType          string
	RetryOfRunID         string
	RetryAttempts        int
	RetryBaseDelay       time.Duration
}

// EHRMSPipelineResult 定義一次 pipeline 執行結果。
type EHRMSPipelineResult struct {
	Employees  domain.EHRMSEmployeeSyncResponse
	Attendance domain.EHRMSAttendanceSyncResponse
}

// EHRMSPipelineScheduler 依序執行部門/崗位/員工 → 考勤同步。
type EHRMSPipelineScheduler struct {
	employees  *EHRMSEmployeeSyncScheduler
	attendance *EHRMSAttendanceSyncScheduler
	logger     *slog.Logger
	runStore   repository.EHRMSSyncStore
	sleep      func(context.Context, time.Duration) error
}

// NewEHRMSPipelineScheduler 建立 eHRMS pipeline scheduler。
func NewEHRMSPipelineScheduler(employees EHRMSEmployeeSyncService, attendance EHRMSAttendanceSyncService, logger *slog.Logger) *EHRMSPipelineScheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &EHRMSPipelineScheduler{
		employees:  NewEHRMSEmployeeSyncScheduler(employees, logger),
		attendance: NewEHRMSAttendanceSyncScheduler(attendance, logger),
		logger:     logger,
		sleep:      sleepEHRMSRetry,
	}
}

// WithRunStore 啟用持久化運行記錄、互斥與有限重試。
func (s *EHRMSPipelineScheduler) WithRunStore(store repository.EHRMSSyncStore) *EHRMSPipelineScheduler {
	if s != nil {
		s.runStore = store
	}
	return s
}

// Run 執行背景工作主迴圈。
func (s *EHRMSPipelineScheduler) Run(ctx context.Context, opts EHRMSPipelineOptions) {
	opts = normalizeEHRMSPipelineOptions(opts)
	if opts.AttendanceEnabled && opts.AttendanceRunOnStart {
		s.syncAndLog(ctx, opts)
	} else if opts.RunOnStart {
		s.syncEmployeesAndLog(ctx, opts)
	}
	employeeTicker := time.NewTicker(opts.Interval)
	defer employeeTicker.Stop()
	var attendanceTicker *time.Ticker
	var attendanceC <-chan time.Time
	if opts.AttendanceEnabled {
		attendanceTicker = time.NewTicker(opts.AttendanceInterval)
		attendanceC = attendanceTicker.C
		defer attendanceTicker.Stop()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-employeeTicker.C:
			s.syncEmployeesAndLog(ctx, opts)
		case <-attendanceC:
			s.syncAndLog(ctx, opts)
		}
	}
}

// SyncOnce 依序同步員工（含部門/崗位）再同步考勤，並在前置員工同步失敗時停止。
func (s *EHRMSPipelineScheduler) SyncOnce(ctx context.Context, opts EHRMSPipelineOptions) (EHRMSPipelineResult, error) {
	opts = normalizeEHRMSPipelineOptions(opts)
	if s == nil || s.runStore == nil {
		result, _, err := s.syncOnceAttempt(ctx, opts, nil)
		return result, err
	}
	return s.syncOnceTracked(ctx, opts)
}

// syncOnceAttempt 執行一次無重試的 pipeline 並回報失敗步驟。
func (s *EHRMSPipelineScheduler) syncOnceAttempt(ctx context.Context, opts EHRMSPipelineOptions, stepHook func(string)) (EHRMSPipelineResult, string, error) {
	var result EHRMSPipelineResult
	if stepHook != nil {
		stepHook("employees")
	}
	employees, err := s.syncEmployeesOnce(ctx, opts)
	result.Employees = employees
	if err != nil {
		return result, "employees", err
	}
	if !opts.AttendanceEnabled {
		return result, "", nil
	}
	if s.attendance == nil {
		return result, "attendance", errors.New("eHRMS pipeline scheduler requires attendance service when attendance sync is enabled")
	}
	if stepHook != nil {
		stepHook("attendance")
	}

	s.logger.InfoContext(ctx, "eHRMS pipeline step started", "step", "attendance")
	attendance, err := s.attendance.SyncOnce(ctx, EHRMSAttendanceSyncOptions{
		Mode:             opts.AttendanceMode,
		Since:            opts.AttendanceSince,
		TenantID:         opts.AttendanceTenant,
		AccountID:        opts.AttendanceAccount,
		DefaultTenantID:  opts.EmployeeTenantID,
		DefaultAccountID: opts.EmployeeAccountID,
	})
	result.Attendance = attendance
	if err != nil {
		s.logger.WarnContext(ctx, "eHRMS pipeline step failed", "step", "attendance", "error", err)
		return result, "attendance", err
	}
	s.logger.InfoContext(ctx, "eHRMS pipeline step completed",
		"step", "attendance",
		"fetched", attendance.Fetched,
		"created", attendance.Created,
		"updated", attendance.Updated,
		"skipped", attendance.Skipped,
		"failed", attendance.Failed,
		"mode", attendance.Mode,
		"since", attendance.Since,
	)
	return result, "", nil
}

func (s *EHRMSPipelineScheduler) syncOnceTracked(ctx context.Context, opts EHRMSPipelineOptions) (EHRMSPipelineResult, error) {
	now := time.Now().UTC()
	requestID := "ehrms-pipeline-" + now.Format("20060102T150405.000000000Z")
	run := domain.EHRMSSyncRun{ID: utils.NewID("esr"), TenantID: strings.TrimSpace(opts.EmployeeTenantID), AccountID: strings.TrimSpace(opts.EmployeeAccountID), SyncType: "pipeline", TriggerType: opts.TriggerType, Status: domain.EHRMSSyncRunStatusRunning, Mode: opts.EmployeeMode, Since: opts.AttendanceSince, Attempt: 1, MaxAttempts: opts.RetryAttempts, RetryOfRunID: opts.RetryOfRunID, RequestID: requestID, TraceID: requestID, StartedAt: now, CreatedAt: now, UpdatedAt: now}
	if run.TriggerType == "" {
		run.TriggerType = "scheduled"
	}
	if err := s.runStore.UpsertEHRMSSyncRun(ctx, run); err != nil {
		return EHRMSPipelineResult{}, err
	}
	var result EHRMSPipelineResult
	acquired, execErr := s.runStore.WithEHRMSSyncLock(ctx, run.TenantID, "ehrms", func() error {
		var lastErr error
		for attempt := 1; attempt <= opts.RetryAttempts; attempt++ {
			run.Attempt, run.UpdatedAt, run.NextRetryAt = attempt, time.Now().UTC(), nil
			_ = s.runStore.UpsertEHRMSSyncRun(ctx, run)
			attemptResult, failedStep, err := s.syncOnceAttempt(ctx, opts, func(step string) {
				run.CurrentStep, run.UpdatedAt = step, time.Now().UTC()
				_ = s.runStore.UpsertEHRMSSyncRun(ctx, run)
			})
			result = attemptResult
			s.recordAttemptSteps(ctx, &run, attempt, failedStep, attemptResult, err, opts.AttendanceEnabled)
			lastErr = err
			if err == nil || !pipelineRetryable(err) || attempt == opts.RetryAttempts {
				break
			}
			delay := ehrmsRetryDelay(opts.RetryBaseDelay, attempt)
			next := time.Now().UTC().Add(delay)
			run.NextRetryAt, run.ErrorCode, run.ErrorMessage, run.Retryable = &next, "temporary_failure", safePipelineError(err), true
			_ = s.runStore.UpsertEHRMSSyncRun(ctx, run)
			if err := s.sleep(ctx, delay); err != nil {
				lastErr = err
				break
			}
		}
		return lastErr
	})
	finished := time.Now().UTC()
	if !acquired && execErr != nil {
		run.Status, run.ErrorCode, run.ErrorMessage = domain.EHRMSSyncRunStatusFailed, "lock_failed", safePipelineError(execErr)
	} else if !acquired {
		run.Status, run.ErrorCode, run.ErrorMessage = domain.EHRMSSyncRunStatusSkipped, "sync_in_progress", "another eHRMS sync is already running"
	} else if execErr != nil {
		run.Status, run.ErrorCode, run.ErrorMessage, run.Retryable = domain.EHRMSSyncRunStatusFailed, pipelineErrorCode(execErr), safePipelineError(execErr), pipelineRetryable(execErr)
	} else if pipelinePartial(result) {
		run.Status = domain.EHRMSSyncRunStatusPartial
	} else {
		run.Status = domain.EHRMSSyncRunStatusSucceeded
	}
	run.CurrentStep, run.NextRetryAt, run.FinishedAt, run.UpdatedAt = "", nil, &finished, finished
	run.Summary = pipelineSummary(result)
	if err := s.runStore.UpsertEHRMSSyncRun(ctx, run); err != nil && execErr == nil {
		execErr = err
	}
	if !acquired && execErr == nil {
		execErr = errors.New("another eHRMS sync is already running")
	}
	return result, execErr
}

func (s *EHRMSPipelineScheduler) recordAttemptSteps(ctx context.Context, run *domain.EHRMSSyncRun, attempt int, failedStep string, result EHRMSPipelineResult, runErr error, attendanceEnabled bool) {
	now := time.Now().UTC()
	employeeStatus := domain.EHRMSSyncRunStatusSucceeded
	if failedStep == "employees" {
		employeeStatus = domain.EHRMSSyncRunStatusFailed
	} else if result.Employees.Failed > 0 {
		employeeStatus = domain.EHRMSSyncRunStatusPartial
	}
	employee := domain.EHRMSSyncRunStep{ID: utils.NewID("ess"), TenantID: run.TenantID, RunID: run.ID, Step: "employees", Sequence: 1, Status: employeeStatus, Attempt: attempt, Summary: employeePipelineSummary(result.Employees), StartedAt: now, FinishedAt: &now}
	if failedStep == "employees" {
		employee.ErrorCode, employee.ErrorMessage = pipelineErrorCode(runErr), safePipelineError(runErr)
	}
	_ = s.runStore.UpsertEHRMSSyncRunStep(ctx, employee)
	if failedStep == "employees" || !attendanceEnabled {
		return
	}
	attendanceStatus := domain.EHRMSSyncRunStatusSucceeded
	if failedStep == "attendance" {
		attendanceStatus = domain.EHRMSSyncRunStatusFailed
	} else if attendanceFailedCount(result.Attendance) > 0 {
		attendanceStatus = domain.EHRMSSyncRunStatusPartial
	}
	attendance := domain.EHRMSSyncRunStep{ID: utils.NewID("ess"), TenantID: run.TenantID, RunID: run.ID, Step: "attendance", Sequence: 2, Status: attendanceStatus, Attempt: attempt, Summary: attendancePipelineSummary(result.Attendance), StartedAt: now, FinishedAt: &now}
	if failedStep == "attendance" {
		attendance.ErrorCode, attendance.ErrorMessage = pipelineErrorCode(runErr), safePipelineError(runErr)
	}
	_ = s.runStore.UpsertEHRMSSyncRunStep(ctx, attendance)
}

// syncEmployeesOnce 執行 pipeline 的員工前置步驟並記錄結構化結果。
func (s *EHRMSPipelineScheduler) syncEmployeesOnce(ctx context.Context, opts EHRMSPipelineOptions) (domain.EHRMSEmployeeSyncResponse, error) {
	if s == nil || s.employees == nil {
		return domain.EHRMSEmployeeSyncResponse{}, errors.New("eHRMS pipeline scheduler requires employee service")
	}
	s.logger.InfoContext(ctx, "eHRMS pipeline step started", "step", "employees")
	employees, err := s.employees.SyncOnce(ctx, EHRMSEmployeeSyncOptions{
		Mode:      opts.EmployeeMode,
		TenantID:  opts.EmployeeTenantID,
		AccountID: opts.EmployeeAccountID,
	})
	if err != nil {
		s.logger.WarnContext(ctx, "eHRMS pipeline step failed", "step", "employees", "error", err)
		return employees, err
	}
	s.logger.InfoContext(ctx, "eHRMS pipeline step completed",
		"step", "employees",
		"fetched", employees.Fetched,
		"created", employees.Created,
		"updated", employees.Updated,
		"skipped", employees.Skipped,
		"failed", employees.Failed,
		"departments_upserted", employees.DepartmentsUpserted,
		"positions_upserted", employees.PositionsUpserted,
		"mode", employees.Mode,
	)
	return employees, nil
}

// syncEmployeesAndLog 執行員工獨立週期並記錄最終狀態。
func (s *EHRMSPipelineScheduler) syncEmployeesAndLog(ctx context.Context, opts EHRMSPipelineOptions) {
	result, err := s.syncEmployeesOnce(ctx, opts)
	if err != nil {
		s.logger.WarnContext(ctx, "eHRMS employee pipeline sync failed", "error", err, "employees_fetched", result.Fetched)
		return
	}
	s.logger.InfoContext(ctx, "eHRMS employee pipeline sync completed",
		"employees_fetched", result.Fetched,
		"employees_created", result.Created,
		"employees_updated", result.Updated,
		"employees_skipped", result.Skipped,
		"departments_upserted", result.DepartmentsUpserted,
		"positions_upserted", result.PositionsUpserted,
	)
}

func (s *EHRMSPipelineScheduler) syncAndLog(ctx context.Context, opts EHRMSPipelineOptions) {
	result, err := s.SyncOnce(ctx, opts)
	if err != nil {
		s.logger.WarnContext(ctx, "eHRMS pipeline sync finished with errors",
			"error", err,
			"employees_fetched", result.Employees.Fetched,
			"attendance_fetched", result.Attendance.Fetched,
		)
		return
	}
	s.logger.InfoContext(ctx, "eHRMS pipeline sync completed",
		"employees_fetched", result.Employees.Fetched,
		"employees_created", result.Employees.Created,
		"employees_updated", result.Employees.Updated,
		"departments_upserted", result.Employees.DepartmentsUpserted,
		"positions_upserted", result.Employees.PositionsUpserted,
		"attendance_fetched", result.Attendance.Fetched,
		"attendance_created", result.Attendance.Created,
		"attendance_updated", result.Attendance.Updated,
		"attendance_skipped", result.Attendance.Skipped,
		"attendance_failed", result.Attendance.Failed,
	)
}

// normalizeEHRMSPipelineOptions 為員工與考勤週期補齊各自的安全預設值。
func normalizeEHRMSPipelineOptions(opts EHRMSPipelineOptions) EHRMSPipelineOptions {
	if opts.Interval <= 0 {
		opts.Interval = defaultEHRMSEmployeeSyncInterval
	}
	if opts.AttendanceInterval <= 0 {
		opts.AttendanceInterval = defaultEHRMSAttendanceSyncInterval
	}
	if opts.RetryAttempts <= 0 {
		opts.RetryAttempts = 1
	}
	if opts.RetryBaseDelay <= 0 {
		opts.RetryBaseDelay = time.Minute
	}
	return opts
}

func sleepEHRMSRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func ehrmsRetryDelay(base time.Duration, attempt int) time.Duration {
	multipliers := []time.Duration{1, 5, 15}
	index := attempt - 1
	if index < 0 {
		index = 0
	}
	if index >= len(multipliers) {
		index = len(multipliers) - 1
	}
	return base * multipliers[index]
}

func pipelineRetryable(err error) bool {
	if err == nil {
		return false
	}
	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		return appErr.ReasonCode == "ehrms_temporary_failure"
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "fetch ehrms") || strings.Contains(message, "timeout") || strings.Contains(message, "temporar") || strings.Contains(message, "connection") || strings.Contains(message, "unavailable")
}

func pipelineErrorCode(err error) string {
	if err == nil {
		return ""
	}
	if pipelineRetryable(err) {
		return "temporary_failure"
	}
	return "sync_failed"
}
func safePipelineError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}
func attendanceFailedCount(v domain.EHRMSAttendanceSyncResponse) int {
	return v.Failed + v.LeaveBalancesFailed + v.LeaveDetailsFailed
}
func pipelinePartial(v EHRMSPipelineResult) bool {
	return v.Employees.Failed > 0 || attendanceFailedCount(v.Attendance) > 0
}
func employeePipelineSummary(v domain.EHRMSEmployeeSyncResponse) map[string]any {
	return map[string]any{"fetched": v.Fetched, "created": v.Created, "updated": v.Updated, "skipped": v.Skipped, "failed": v.Failed, "departments_upserted": v.DepartmentsUpserted, "positions_upserted": v.PositionsUpserted}
}
func attendancePipelineSummary(v domain.EHRMSAttendanceSyncResponse) map[string]any {
	return map[string]any{"fetched": v.Fetched, "created": v.Created, "updated": v.Updated, "skipped": v.Skipped, "failed": v.Failed, "leave_balances_failed": v.LeaveBalancesFailed, "leave_details_failed": v.LeaveDetailsFailed}
}
func pipelineSummary(v EHRMSPipelineResult) map[string]any {
	return map[string]any{"employees": employeePipelineSummary(v.Employees), "attendance": attendancePipelineSummary(v.Attendance)}
}
