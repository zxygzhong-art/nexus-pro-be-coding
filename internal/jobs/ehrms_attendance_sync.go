package jobs

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
)

const defaultEHRMSAttendanceSyncInterval = 24 * time.Hour

// EHRMSAttendanceSyncService 定義 eHRMS 考勤 sync 服務的行為契約。
type EHRMSAttendanceSyncService interface {
	SyncEHRMSAttendance(domain.RequestContext, domain.EHRMSAttendanceSyncInput) (domain.EHRMSAttendanceSyncResponse, error)
}

// EHRMSAttendanceSyncOptions 定義 eHRMS 考勤 sync 選項。
type EHRMSAttendanceSyncOptions struct {
	Interval         time.Duration
	Mode             string
	Since            string
	TenantID         string
	AccountID        string
	DefaultTenantID  string
	DefaultAccountID string
	RunOnStart       bool
}

// EHRMSAttendanceSyncScheduler 定義 eHRMS 考勤 sync scheduler。
type EHRMSAttendanceSyncScheduler struct {
	service EHRMSAttendanceSyncService
	logger  *slog.Logger
	now     func() time.Time
}

// NewEHRMSAttendanceSyncScheduler 建立 eHRMS 考勤 sync scheduler。
func NewEHRMSAttendanceSyncScheduler(service EHRMSAttendanceSyncService, logger *slog.Logger) *EHRMSAttendanceSyncScheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &EHRMSAttendanceSyncScheduler{service: service, logger: logger, now: time.Now}
}

// Run 執行背景工作主迴圈。
func (s *EHRMSAttendanceSyncScheduler) Run(ctx context.Context, opts EHRMSAttendanceSyncOptions) {
	opts = normalizeEHRMSAttendanceSyncOptions(opts)
	if opts.RunOnStart {
		s.syncAndLog(ctx, opts)
	}
	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncAndLog(ctx, opts)
		}
	}
}

// SyncOnce 同步 once。
func (s *EHRMSAttendanceSyncScheduler) SyncOnce(ctx context.Context, opts EHRMSAttendanceSyncOptions) (domain.EHRMSAttendanceSyncResponse, error) {
	opts = normalizeEHRMSAttendanceSyncOptions(opts)
	if s == nil || s.service == nil {
		return domain.EHRMSAttendanceSyncResponse{}, errors.New("eHRMS attendance sync scheduler requires service")
	}
	if opts.TenantID == "" {
		return domain.EHRMSAttendanceSyncResponse{}, errors.New("EHRMS_ATTENDANCE_SYNC_TENANT_ID is required")
	}
	if opts.AccountID == "" {
		return domain.EHRMSAttendanceSyncResponse{}, errors.New("EHRMS_ATTENDANCE_SYNC_ACCOUNT_ID is required")
	}
	requestID := "ehrms-attendance-sync-" + s.now().UTC().Format("20060102T150405Z")
	return s.service.SyncEHRMSAttendance(domain.RequestContext{
		Context:           ctx,
		TenantID:          opts.TenantID,
		AccountID:         opts.AccountID,
		RequestID:         requestID,
		TraceID:           requestID,
		ApprovalConfirmed: true,
	}, domain.EHRMSAttendanceSyncInput{Mode: opts.Mode, Since: opts.Since})
}

func (s *EHRMSAttendanceSyncScheduler) syncAndLog(ctx context.Context, opts EHRMSAttendanceSyncOptions) {
	result, err := s.SyncOnce(ctx, opts)
	if err != nil {
		s.logger.WarnContext(ctx, "eHRMS attendance sync failed", "error", err)
		return
	}
	s.logger.InfoContext(ctx, "eHRMS attendance sync completed",
		"fetched", result.Fetched,
		"created", result.Created,
		"updated", result.Updated,
		"skipped", result.Skipped,
		"failed", result.Failed,
		"mode", result.Mode,
		"since", result.Since,
	)
}

// normalizeEHRMSAttendanceSyncOptions 正規化 eHRMS 考勤 sync 選項。
func normalizeEHRMSAttendanceSyncOptions(opts EHRMSAttendanceSyncOptions) EHRMSAttendanceSyncOptions {
	if opts.Interval <= 0 {
		opts.Interval = defaultEHRMSAttendanceSyncInterval
	}
	opts.Mode = strings.ToLower(strings.TrimSpace(opts.Mode))
	if opts.Mode == "" {
		opts.Mode = "upsert"
	}
	opts.Since = strings.TrimSpace(opts.Since)
	opts.TenantID = strings.TrimSpace(opts.TenantID)
	if opts.TenantID == "" {
		opts.TenantID = strings.TrimSpace(opts.DefaultTenantID)
	}
	opts.AccountID = strings.TrimSpace(opts.AccountID)
	if opts.AccountID == "" {
		opts.AccountID = strings.TrimSpace(opts.DefaultAccountID)
	}
	return opts
}
