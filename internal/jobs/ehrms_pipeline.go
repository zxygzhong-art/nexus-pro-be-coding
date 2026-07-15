package jobs

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository"
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
	locker     repository.EHRMSSyncLocker
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

// WithSyncLocker 啟用跨執行個體的同租戶同步互斥，不保存同步運行資料。
func (s *EHRMSPipelineScheduler) WithSyncLocker(locker repository.EHRMSSyncLocker) *EHRMSPipelineScheduler {
	if s != nil {
		s.locker = locker
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
	var result EHRMSPipelineResult
	execute := func() error {
		var lastErr error
		for attempt := 1; attempt <= opts.RetryAttempts; attempt++ {
			attemptResult, err := s.syncOnceAttempt(ctx, opts)
			result = attemptResult
			lastErr = err
			if err == nil || !pipelineRetryable(err) || attempt == opts.RetryAttempts {
				return lastErr
			}
			delay := ehrmsRetryDelay(opts.RetryBaseDelay, attempt)
			s.logger.WarnContext(ctx, "eHRMS pipeline retry scheduled", "attempt", attempt+1, "delay", delay, "error", err)
			if err := s.sleep(ctx, delay); err != nil {
				return err
			}
		}
		return lastErr
	}
	if s == nil || s.locker == nil {
		return result, execute()
	}
	acquired, err := s.locker.WithEHRMSSyncLock(ctx, strings.TrimSpace(opts.EmployeeTenantID), "ehrms", execute)
	if err != nil {
		return result, err
	}
	if !acquired {
		return result, errors.New("another eHRMS sync is already running")
	}
	return result, nil
}

// syncOnceAttempt 執行一次無重試的 pipeline。
func (s *EHRMSPipelineScheduler) syncOnceAttempt(ctx context.Context, opts EHRMSPipelineOptions) (EHRMSPipelineResult, error) {
	var result EHRMSPipelineResult
	employees, err := s.syncEmployeesOnce(ctx, opts)
	result.Employees = employees
	if err != nil {
		return result, err
	}
	if !opts.AttendanceEnabled {
		return result, nil
	}
	if s.attendance == nil {
		return result, errors.New("eHRMS pipeline scheduler requires attendance service when attendance sync is enabled")
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
		return result, err
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
	return result, nil
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
