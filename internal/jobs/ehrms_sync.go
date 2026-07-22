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
	ehrmsSyncMorningHour = 8
	ehrmsSyncEveningHour = 20
)

var ehrmsSyncLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// EHRMSSyncHRService defines the HR operations required by the unified eHRMS sync.
type EHRMSSyncHRService interface {
	SyncEHRMSOrgUnits(domain.RequestContext) (domain.EHRMSOrgUnitSyncResponse, error)
	SyncEHRMSEmployees(domain.RequestContext, domain.EHRMSEmployeeSyncInput) (domain.EHRMSEmployeeSyncResponse, error)
}

// EHRMSSyncAttendanceService defines the attendance operation required by the unified eHRMS sync.
type EHRMSSyncAttendanceService interface {
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

// Run executes once on startup, then at 08:00 and 20:00 UTC+8 every day.
func (s *EHRMSSyncScheduler) Run(ctx context.Context, opts EHRMSSyncOptions) {
	opts = normalizeEHRMSSyncOptions(opts)
	s.syncAndLog(ctx, opts)
	for {
		nextRun := nextEHRMSSyncTime(s.now())
		timer := time.NewTimer(time.Until(nextRun))
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
			s.syncAndLog(ctx, opts)
		}
	}
}

// SyncOnce runs org units, employees, and attendance sequentially.
func (s *EHRMSSyncScheduler) SyncOnce(ctx context.Context, opts EHRMSSyncOptions) (EHRMSSyncResult, error) {
	opts = normalizeEHRMSSyncOptions(opts)
	if s == nil || s.hrService == nil || s.attendanceService == nil {
		return EHRMSSyncResult{}, errors.New("eHRMS sync scheduler requires HR and attendance services")
	}
	if opts.TenantID == "" {
		return EHRMSSyncResult{}, errors.New("EHRMS_SYNC_TENANT_ID is required")
	}
	if opts.AccountID == "" {
		return EHRMSSyncResult{}, errors.New("EHRMS_SYNC_ACCOUNT_ID is required")
	}
	requestID := "ehrms-sync-" + s.now().UTC().Format("20060102T150405Z")
	requestContext := domain.RequestContext{
		Context:   ctx,
		TenantID:  opts.TenantID,
		AccountID: opts.AccountID,
		RequestID: requestID,
		TraceID:   requestID,
	}
	result := EHRMSSyncResult{}
	var err error
	result.OrgUnits, err = s.hrService.SyncEHRMSOrgUnits(requestContext)
	if err != nil {
		return result, err
	}
	result.Employees, err = s.hrService.SyncEHRMSEmployees(requestContext, domain.EHRMSEmployeeSyncInput{Mode: opts.Mode})
	if err != nil {
		return result, err
	}
	result.Attendance, err = s.attendanceService.SyncEHRMSAttendance(requestContext, domain.EHRMSAttendanceSyncInput{Mode: opts.Mode})
	if err != nil {
		return result, err
	}
	return result, nil
}

func (s *EHRMSSyncScheduler) syncAndLog(ctx context.Context, opts EHRMSSyncOptions) {
	result, err := s.SyncOnce(ctx, opts)
	if err != nil {
		s.logger.WarnContext(ctx, "eHRMS sync failed", "error", err)
		return
	}
	s.logger.InfoContext(ctx, "eHRMS sync completed",
		"org_units_upserted", result.OrgUnits.Upserted,
		"employees_fetched", result.Employees.Fetched,
		"employees_created", result.Employees.Created,
		"employees_updated", result.Employees.Updated,
		"attendance_fetched", result.Attendance.Fetched,
		"attendance_created", result.Attendance.Created,
		"attendance_updated", result.Attendance.Updated,
		"leave_balances_upserted", result.Attendance.LeaveBalancesUpserted,
		"leave_details_created", result.Attendance.LeaveDetailsCreated,
		"leave_details_updated", result.Attendance.LeaveDetailsUpdated,
		"mode", result.Employees.Mode,
		"start", result.Attendance.Start,
	)
}

func normalizeEHRMSSyncOptions(opts EHRMSSyncOptions) EHRMSSyncOptions {
	opts.Mode = strings.ToLower(strings.TrimSpace(opts.Mode))
	if opts.Mode == "" {
		opts.Mode = "upsert"
	}
	opts.TenantID = strings.TrimSpace(opts.TenantID)
	opts.AccountID = strings.TrimSpace(opts.AccountID)
	return opts
}

func nextEHRMSSyncTime(now time.Time) time.Time {
	localNow := now.In(ehrmsSyncLocation)
	for _, hour := range []int{ehrmsSyncMorningHour, ehrmsSyncEveningHour} {
		candidate := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), hour, 0, 0, 0, ehrmsSyncLocation)
		if candidate.After(localNow) {
			return candidate
		}
	}
	return time.Date(localNow.Year(), localNow.Month(), localNow.Day()+1, ehrmsSyncMorningHour, 0, 0, 0, ehrmsSyncLocation)
}
