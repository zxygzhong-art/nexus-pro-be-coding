package jobs

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
)

const (
	ehrmsDailyCatalogSyncHour = 8
	// ScheduledEHRMSSyncMode is immutable so repeated background runs always update existing rows.
	ScheduledEHRMSSyncMode = "upsert"
)

var ehrmsSyncLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// EHRMSSyncHRService defines the HR operations required by the unified eHRMS sync.
type EHRMSSyncHRService interface {
	SyncEHRMSOrgUnits(domain.RequestContext) (domain.EHRMSOrgUnitSyncResponse, error)
	SyncEHRMSEmployees(domain.RequestContext, domain.EHRMSEmployeeSyncInput) (domain.EHRMSEmployeeSyncResponse, error)
}

// EHRMSSyncAttendanceService defines the attendance operation required by the unified eHRMS sync.
type EHRMSSyncAttendanceService interface {
	SyncEHRMSLeaveTypes(domain.RequestContext) (domain.EHRMSLeaveTypeSyncResponse, error)
	SyncEHRMSAttendance(domain.RequestContext, domain.EHRMSAttendanceSyncInput) (domain.EHRMSAttendanceSyncResponse, error)
}

// EHRMSSyncOptions configures the unified eHRMS sync job.
type EHRMSSyncOptions struct {
	Mode      string
	TenantID  string
	AccountID string
}

// EHRMSSyncResult contains the result of every step in one unified run.
type EHRMSSyncResult struct {
	OrgUnits   domain.EHRMSOrgUnitSyncResponse    `json:"org_units"`
	Employees  domain.EHRMSEmployeeSyncResponse   `json:"employees"`
	LeaveTypes domain.EHRMSLeaveTypeSyncResponse  `json:"leave_types"`
	Attendance domain.EHRMSAttendanceSyncResponse `json:"attendance"`
}

// EHRMSSyncScheduler runs org-unit, employee, and attendance synchronization as one ordered job.
type EHRMSSyncScheduler struct {
	hrService         EHRMSSyncHRService
	attendanceService EHRMSSyncAttendanceService
	logger            *slog.Logger
	now               func() time.Time
}

// NewEHRMSSyncScheduler creates the unified eHRMS scheduler.
func NewEHRMSSyncScheduler(hrService EHRMSSyncHRService, attendanceService EHRMSSyncAttendanceService, logger *slog.Logger) *EHRMSSyncScheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &EHRMSSyncScheduler{
		hrService:         hrService,
		attendanceService: attendanceService,
		logger:            logger,
		now:               time.Now,
	}
}

// Run performs one current-year full sync on startup, refreshes catalogs daily
// at 08:00 UTC+8, and refreshes today's attendance/leave data every 30 minutes.
func (s *EHRMSSyncScheduler) Run(ctx context.Context, opts EHRMSSyncOptions) {
	opts = normalizeEHRMSSyncOptions(opts)
	opts.Mode = ScheduledEHRMSSyncMode
	s.startupSyncAndLog(ctx, opts)
	dailyTimer := time.NewTimer(time.Until(nextEHRMSDailyCatalogSyncTime(s.now())))
	realtimeTimer := time.NewTimer(time.Until(nextEHRMSHalfHourSyncTime(s.now())))
	defer dailyTimer.Stop()
	defer realtimeTimer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-dailyTimer.C:
			s.dailyCatalogSyncAndLog(ctx, opts)
			dailyTimer.Reset(time.Until(nextEHRMSDailyCatalogSyncTime(s.now())))
		case <-realtimeTimer.C:
			s.todaySyncAndLog(ctx, opts)
			realtimeTimer.Reset(time.Until(nextEHRMSHalfHourSyncTime(s.now())))
		}
	}
}

// SyncOnce runs the startup-only current-year full synchronization.
func (s *EHRMSSyncScheduler) SyncOnce(ctx context.Context, opts EHRMSSyncOptions) (EHRMSSyncResult, error) {
	opts, requestContext, syncStartedAt, err := s.prepareSync(ctx, opts, "startup")
	if err != nil {
		return EHRMSSyncResult{}, err
	}
	requestID := requestContext.RequestID
	s.logger.InfoContext(ctx, "eHRMS sync started",
		"request_id", requestID,
		"sync_kind", "startup_full_year",
		"tenant_id", opts.TenantID,
		"mode", opts.Mode,
	)
	result := EHRMSSyncResult{}
	if err := s.syncCatalogStages(ctx, requestContext, opts, &result); err != nil {
		return result, err
	}
	localNow := s.now().In(ehrmsSyncLocation)
	yearStart := time.Date(localNow.Year(), time.January, 1, 0, 0, 0, 0, ehrmsSyncLocation)
	result.Attendance, err = s.syncAttendanceStage(ctx, requestContext, domain.EHRMSAttendanceSyncInput{
		Mode:           opts.Mode,
		Start:          yearStart.Format(time.DateOnly),
		End:            yearStart.AddDate(1, 0, 0).Format(time.DateOnly),
		SkipLeaveTypes: true,
	})
	if err != nil {
		return result, err
	}
	s.logger.InfoContext(ctx, "eHRMS sync stages completed",
		"request_id", requestID,
		"sync_kind", "startup_full_year",
		"elapsed_ms", s.now().Sub(syncStartedAt).Milliseconds(),
	)
	return result, nil
}

// SyncDailyCatalogs refreshes employees, organization/positions, and leave types.
func (s *EHRMSSyncScheduler) SyncDailyCatalogs(ctx context.Context, opts EHRMSSyncOptions) (EHRMSSyncResult, error) {
	opts, requestContext, syncStartedAt, err := s.prepareSync(ctx, opts, "daily-catalogs")
	if err != nil {
		return EHRMSSyncResult{}, err
	}
	result := EHRMSSyncResult{}
	err = s.syncCatalogStages(ctx, requestContext, opts, &result)
	if err != nil {
		return result, err
	}
	s.logger.InfoContext(ctx, "eHRMS daily catalog sync completed",
		"request_id", requestContext.RequestID,
		"elapsed_ms", s.now().Sub(syncStartedAt).Milliseconds(),
	)
	return result, nil
}

// SyncToday refreshes only the current UTC+8 calendar day's attendance and leave facts.
func (s *EHRMSSyncScheduler) SyncToday(ctx context.Context, opts EHRMSSyncOptions) (domain.EHRMSAttendanceSyncResponse, error) {
	opts, requestContext, _, err := s.prepareSync(ctx, opts, "today")
	if err != nil {
		return domain.EHRMSAttendanceSyncResponse{}, err
	}
	localNow := s.now().In(ehrmsSyncLocation)
	start := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, ehrmsSyncLocation)
	return s.syncAttendanceStage(ctx, requestContext, domain.EHRMSAttendanceSyncInput{
		Mode:           opts.Mode,
		Start:          start.Format(time.DateOnly),
		End:            start.AddDate(0, 0, 1).Format(time.DateOnly),
		SkipLeaveTypes: true,
	})
}

func (s *EHRMSSyncScheduler) prepareSync(ctx context.Context, opts EHRMSSyncOptions, kind string) (EHRMSSyncOptions, domain.RequestContext, time.Time, error) {
	opts = normalizeEHRMSSyncOptions(opts)
	if s == nil || s.hrService == nil || s.attendanceService == nil {
		return opts, domain.RequestContext{}, time.Time{}, errors.New("eHRMS sync scheduler requires HR and attendance services")
	}
	if opts.TenantID == "" {
		return opts, domain.RequestContext{}, time.Time{}, errors.New("EHRMS_SYNC_TENANT_ID is required")
	}
	if opts.AccountID == "" {
		return opts, domain.RequestContext{}, time.Time{}, errors.New("EHRMS_SYNC_ACCOUNT_ID is required")
	}
	startedAt := s.now()
	requestID := "ehrms-" + kind + "-" + startedAt.UTC().Format("20060102T150405Z")
	return opts, domain.RequestContext{
		Context:   ctx,
		TenantID:  opts.TenantID,
		AccountID: opts.AccountID,
		RequestID: requestID,
		TraceID:   requestID,
	}, startedAt, nil
}

func (s *EHRMSSyncScheduler) syncCatalogStages(ctx context.Context, requestContext domain.RequestContext, opts EHRMSSyncOptions, result *EHRMSSyncResult) error {
	requestID := requestContext.RequestID
	stageStartedAt := s.now()
	s.logStageStarted(ctx, requestID, "org_units_positions")
	var err error
	result.OrgUnits, err = s.hrService.SyncEHRMSOrgUnits(requestContext)
	if err != nil {
		s.logStageFailed(ctx, requestID, "org_units_positions", stageStartedAt, err)
		return err
	}
	s.logger.InfoContext(ctx, "eHRMS sync stage completed",
		"request_id", requestID,
		"stage", "org_units_positions",
		"elapsed_ms", s.now().Sub(stageStartedAt).Milliseconds(),
		"fetched", result.OrgUnits.Fetched,
		"upserted", result.OrgUnits.Upserted,
	)

	stageStartedAt = s.now()
	s.logStageStarted(ctx, requestID, "employees")
	result.Employees, err = s.hrService.SyncEHRMSEmployees(requestContext, domain.EHRMSEmployeeSyncInput{Mode: opts.Mode})
	if err != nil {
		s.logStageFailed(ctx, requestID, "employees", stageStartedAt, err)
		return err
	}
	s.logger.InfoContext(ctx, "eHRMS sync stage completed",
		"request_id", requestID,
		"stage", "employees",
		"elapsed_ms", s.now().Sub(stageStartedAt).Milliseconds(),
		"fetched", result.Employees.Fetched,
		"created", result.Employees.Created,
		"updated", result.Employees.Updated,
		"skipped", result.Employees.Skipped,
		"failed", result.Employees.Failed,
	)

	stageStartedAt = s.now()
	s.logStageStarted(ctx, requestID, "leave_types")
	result.LeaveTypes, err = s.attendanceService.SyncEHRMSLeaveTypes(requestContext)
	if err != nil {
		s.logStageFailed(ctx, requestID, "leave_types", stageStartedAt, err)
		return err
	}
	s.logger.InfoContext(ctx, "eHRMS sync stage completed",
		"request_id", requestID,
		"stage", "leave_types",
		"elapsed_ms", s.now().Sub(stageStartedAt).Milliseconds(),
		"fetched", result.LeaveTypes.Fetched,
		"upserted", result.LeaveTypes.Upserted,
		"deactivated", result.LeaveTypes.Deactivated,
	)
	return nil
}

func (s *EHRMSSyncScheduler) syncAttendanceStage(ctx context.Context, requestContext domain.RequestContext, input domain.EHRMSAttendanceSyncInput) (domain.EHRMSAttendanceSyncResponse, error) {
	stageStartedAt := s.now()
	s.logStageStarted(ctx, requestContext.RequestID, "attendance_leave")
	result, err := s.attendanceService.SyncEHRMSAttendance(requestContext, input)
	if err != nil {
		s.logStageFailed(ctx, requestContext.RequestID, "attendance_leave", stageStartedAt, err)
		return result, err
	}
	s.logger.InfoContext(ctx, "eHRMS sync stage completed",
		"request_id", requestContext.RequestID,
		"stage", "attendance_leave",
		"elapsed_ms", s.now().Sub(stageStartedAt).Milliseconds(),
		"start", result.Start,
		"end", result.End,
		"fetched", result.Fetched,
		"created", result.Created,
		"updated", result.Updated,
		"skipped", result.Skipped,
		"failed", result.Failed,
		"leave_balances_upserted", result.LeaveBalancesUpserted,
		"leave_balances_failed", result.LeaveBalancesFailed,
		"leave_details_created", result.LeaveDetailsCreated,
		"leave_details_updated", result.LeaveDetailsUpdated,
		"leave_details_failed", result.LeaveDetailsFailed,
	)
	return result, nil
}

func (s *EHRMSSyncScheduler) logStageStarted(ctx context.Context, requestID, stage string) {
	s.logger.InfoContext(ctx, "eHRMS sync stage started",
		"request_id", requestID,
		"stage", stage,
	)
}

func (s *EHRMSSyncScheduler) logStageFailed(ctx context.Context, requestID, stage string, startedAt time.Time, err error) {
	s.logger.WarnContext(ctx, "eHRMS sync stage failed",
		"request_id", requestID,
		"stage", stage,
		"elapsed_ms", s.now().Sub(startedAt).Milliseconds(),
		"error", err,
	)
}

func (s *EHRMSSyncScheduler) startupSyncAndLog(ctx context.Context, opts EHRMSSyncOptions) {
	result, err := s.SyncOnce(ctx, opts)
	if err != nil {
		s.logger.WarnContext(ctx, "eHRMS startup full sync failed", "error", err)
		return
	}
	s.logger.InfoContext(ctx, "eHRMS startup full sync completed",
		"org_units_upserted", result.OrgUnits.Upserted,
		"employees_fetched", result.Employees.Fetched,
		"employees_created", result.Employees.Created,
		"employees_updated", result.Employees.Updated,
		"leave_types_upserted", result.LeaveTypes.Upserted,
		"attendance_fetched", result.Attendance.Fetched,
		"attendance_created", result.Attendance.Created,
		"attendance_updated", result.Attendance.Updated,
		"leave_balances_upserted", result.Attendance.LeaveBalancesUpserted,
		"leave_balances_failed", result.Attendance.LeaveBalancesFailed,
		"leave_details_created", result.Attendance.LeaveDetailsCreated,
		"leave_details_updated", result.Attendance.LeaveDetailsUpdated,
		"leave_details_failed", result.Attendance.LeaveDetailsFailed,
		"mode", result.Employees.Mode,
		"start", result.Attendance.Start,
		"end", result.Attendance.End,
	)
}

func (s *EHRMSSyncScheduler) dailyCatalogSyncAndLog(ctx context.Context, opts EHRMSSyncOptions) {
	result, err := s.SyncDailyCatalogs(ctx, opts)
	if err != nil {
		s.logger.WarnContext(ctx, "eHRMS daily catalog sync failed", "error", err)
		return
	}
	s.logger.InfoContext(ctx, "eHRMS daily catalog sync succeeded",
		"org_units_upserted", result.OrgUnits.Upserted,
		"employees_fetched", result.Employees.Fetched,
		"employees_created", result.Employees.Created,
		"employees_updated", result.Employees.Updated,
		"positions_upserted", result.Employees.PositionsUpserted,
		"leave_types_upserted", result.LeaveTypes.Upserted,
	)
}

func (s *EHRMSSyncScheduler) todaySyncAndLog(ctx context.Context, opts EHRMSSyncOptions) {
	result, err := s.SyncToday(ctx, opts)
	if err != nil {
		s.logger.WarnContext(ctx, "eHRMS 30-minute attendance sync failed", "error", err)
		return
	}
	s.logger.InfoContext(ctx, "eHRMS 30-minute attendance sync succeeded",
		"start", result.Start,
		"end", result.End,
		"attendance_fetched", result.Fetched,
		"attendance_created", result.Created,
		"attendance_updated", result.Updated,
		"leave_balances_upserted", result.LeaveBalancesUpserted,
		"leave_details_created", result.LeaveDetailsCreated,
		"leave_details_updated", result.LeaveDetailsUpdated,
	)
}

func normalizeEHRMSSyncOptions(opts EHRMSSyncOptions) EHRMSSyncOptions {
	opts.Mode = strings.ToLower(strings.TrimSpace(opts.Mode))
	if opts.Mode == "" {
		opts.Mode = ScheduledEHRMSSyncMode
	}
	opts.TenantID = strings.TrimSpace(opts.TenantID)
	opts.AccountID = strings.TrimSpace(opts.AccountID)
	return opts
}

func nextEHRMSDailyCatalogSyncTime(now time.Time) time.Time {
	localNow := now.In(ehrmsSyncLocation)
	candidate := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), ehrmsDailyCatalogSyncHour, 0, 0, 0, ehrmsSyncLocation)
	if candidate.After(localNow) {
		return candidate
	}
	return candidate.AddDate(0, 0, 1)
}

func nextEHRMSHalfHourSyncTime(now time.Time) time.Time {
	localNow := now.In(ehrmsSyncLocation)
	minute := 30
	hour := localNow.Hour()
	if localNow.Minute() >= 30 {
		minute = 0
		hour++
	}
	return time.Date(localNow.Year(), localNow.Month(), localNow.Day(), hour, minute, 0, 0, ehrmsSyncLocation)
}
