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

// EHRMSEmployeeSyncService applies employee master data from the eHRMS upstream.
type EHRMSEmployeeSyncService interface {
	SyncEHRMSEmployees(domain.RequestContext, domain.EHRMSEmployeeSyncInput) (domain.EHRMSEmployeeSyncResponse, error)
}

// EHRMSEmployeeSyncOptions controls the periodic eHRMS employee refresh.
type EHRMSEmployeeSyncOptions struct {
	Interval   time.Duration
	Mode       string
	TenantID   string
	AccountID  string
	RunOnStart bool
}

// EHRMSEmployeeSyncScheduler periodically refreshes employee master data.
type EHRMSEmployeeSyncScheduler struct {
	service EHRMSEmployeeSyncService
	logger  *slog.Logger
	now     func() time.Time
}

// NewEHRMSEmployeeSyncScheduler creates a scheduler for eHRMS employee sync.
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

// Run optionally syncs once on startup and then runs until the context is canceled.
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

// SyncOnce runs one scheduled eHRMS sync with the configured service account.
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
		Context:           ctx,
		TenantID:          opts.TenantID,
		AccountID:         opts.AccountID,
		RequestID:         requestID,
		TraceID:           requestID,
		ApprovalConfirmed: true,
	}, domain.EHRMSEmployeeSyncInput{Mode: opts.Mode})
}

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
		"failed", result.Failed,
		"departments_upserted", result.DepartmentsUpserted,
		"mode", result.Mode,
	)
}

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
