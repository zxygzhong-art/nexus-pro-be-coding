package jobs

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
)

const defaultEHRMSEmployeeSyncInterval = 24 * time.Hour

// EHRMSEmployeeSyncService 定義 eHRMS 員工 sync 服務的行為契約。
type EHRMSEmployeeSyncService interface {
	SyncEHRMSEmployees(domain.RequestContext, domain.EHRMSEmployeeSyncInput) (domain.EHRMSEmployeeSyncResponse, error)
}

// EHRMSEmployeeSyncOptions 定義 eHRMS 員工 sync 選項的資料結構。
type EHRMSEmployeeSyncOptions struct {
	Interval   time.Duration
	Mode       string
	TenantID   string
	AccountID  string
	RunOnStart bool
}

// EHRMSEmployeeSyncScheduler 定義 eHRMS 員工 sync scheduler 的資料結構。
type EHRMSEmployeeSyncScheduler struct {
	service EHRMSEmployeeSyncService
	logger  *slog.Logger
	now     func() time.Time
}

// NewEHRMSEmployeeSyncScheduler 建立 eHRMS 員工 sync scheduler。
func NewEHRMSEmployeeSyncScheduler(service EHRMSEmployeeSyncService, logger *slog.Logger) *EHRMSEmployeeSyncScheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &EHRMSEmployeeSyncScheduler{
		service: service,
		logger:  logger,
		now:     time.Now,
	}
}

// Run 執行背景工作主迴圈。
func (s *EHRMSEmployeeSyncScheduler) Run(ctx context.Context, opts EHRMSEmployeeSyncOptions) {
	opts = normalizeEHRMSEmployeeSyncOptions(opts)
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
func (s *EHRMSEmployeeSyncScheduler) SyncOnce(ctx context.Context, opts EHRMSEmployeeSyncOptions) (domain.EHRMSEmployeeSyncResponse, error) {
	opts = normalizeEHRMSEmployeeSyncOptions(opts)
	if s == nil || s.service == nil {
		return domain.EHRMSEmployeeSyncResponse{}, errors.New("eHRMS employee sync scheduler requires service")
	}
	if opts.TenantID == "" {
		return domain.EHRMSEmployeeSyncResponse{}, errors.New("EHRMS_SYNC_TENANT_ID is required")
	}
	if opts.AccountID == "" {
		return domain.EHRMSEmployeeSyncResponse{}, errors.New("EHRMS_SYNC_ACCOUNT_ID is required")
	}
	requestID := "ehrms-sync-" + s.now().UTC().Format("20060102T150405Z")
	return s.service.SyncEHRMSEmployees(domain.RequestContext{
		Context:   ctx,
		TenantID:  opts.TenantID,
		AccountID: opts.AccountID,
		RequestID: requestID,
		TraceID:   requestID,
	}, domain.EHRMSEmployeeSyncInput{Mode: opts.Mode})
}

// syncAndLog 同步 and log。
func (s *EHRMSEmployeeSyncScheduler) syncAndLog(ctx context.Context, opts EHRMSEmployeeSyncOptions) {
	result, err := s.SyncOnce(ctx, opts)
	if err != nil {
		s.logger.WarnContext(ctx, "eHRMS employee sync failed", "error", err)
		return
	}
	s.logger.InfoContext(ctx, "eHRMS employee sync completed",
		"fetched", result.Fetched,
		"created", result.Created,
		"updated", result.Updated,
		"skipped", result.Skipped,
		"failed", result.Failed,
		"departments_upserted", result.DepartmentsUpserted,
		"positions_upserted", result.PositionsUpserted,
		"mode", result.Mode,
	)
}

// normalizeEHRMSEmployeeSyncOptions 正規化eHRMS 員工 sync 選項。
func normalizeEHRMSEmployeeSyncOptions(opts EHRMSEmployeeSyncOptions) EHRMSEmployeeSyncOptions {
	if opts.Interval <= 0 {
		opts.Interval = defaultEHRMSEmployeeSyncInterval
	}
	opts.Mode = strings.ToLower(strings.TrimSpace(opts.Mode))
	if opts.Mode == "" {
		opts.Mode = "upsert"
	}
	opts.TenantID = strings.TrimSpace(opts.TenantID)
	opts.AccountID = strings.TrimSpace(opts.AccountID)
	return opts
}
