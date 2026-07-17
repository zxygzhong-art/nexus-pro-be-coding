package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"nexus-pro-be/internal/repository"
)

const (
	defaultIdentityProvisioningBatchSize  = 50
	defaultIdentityProvisioningMaxRetries = 5
	defaultIdentityProvisioningInterval   = 30 * time.Second
)

// IdentityProvisioningService 定義身分開通服務的行為契約。
type IdentityProvisioningService interface {
	ProcessIdentityProvisioningOutbox(ctx context.Context, tenantID string, batchSize, maxRetries int) (int, error)
}

// IdentityProvisioningOutboxOptions 定義身分開通 outbox 選項的資料結構。
type IdentityProvisioningOutboxOptions struct {
	BatchSize  int
	MaxRetries int
	Interval   time.Duration
}

// IdentityProvisioningOutboxProcessor 定義身分開通 outbox processor 的資料結構。
type IdentityProvisioningOutboxProcessor struct {
	store   repository.Store
	service IdentityProvisioningService
	logger  *slog.Logger
}

// NewIdentityProvisioningOutboxProcessor 建立身分開通 outbox processor。
func NewIdentityProvisioningOutboxProcessor(store repository.Store, service IdentityProvisioningService, logger *slog.Logger) *IdentityProvisioningOutboxProcessor {
	if logger == nil {
		logger = slog.Default()
	}
	return &IdentityProvisioningOutboxProcessor{
		store:   store,
		service: service,
		logger:  logger,
	}
}

// Run 執行背景工作主迴圈。
func (p *IdentityProvisioningOutboxProcessor) Run(ctx context.Context, opts IdentityProvisioningOutboxOptions) {
	opts = normalizeIdentityProvisioningOptions(opts)
	p.processAllTenantsAndLog(ctx, opts)
	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.processAllTenantsAndLog(ctx, opts)
		}
	}
}

// ProcessAllTenants 處理 all 租戶。
func (p *IdentityProvisioningOutboxProcessor) ProcessAllTenants(ctx context.Context, opts IdentityProvisioningOutboxOptions) (int, error) {
	opts = normalizeIdentityProvisioningOptions(opts)
	if p == nil || p.store == nil {
		return 0, errors.New("identity provisioning processor requires store")
	}
	if p.service == nil {
		return 0, errors.New("identity provisioning processor requires service")
	}
	tenants, err := p.store.ListTenants(ctx)
	if err != nil {
		return 0, err
	}
	processed := 0
	var errs []error
	for _, tenant := range tenants {
		n, err := p.service.ProcessIdentityProvisioningOutbox(ctx, tenant.ID, opts.BatchSize, opts.MaxRetries)
		if err != nil {
			// 單一租戶失敗不阻塞其他租戶,迴圈結束後聚合回傳錯誤。
			p.logger.WarnContext(ctx, "identity provisioning outbox processing failed for tenant", "tenant_id", tenant.ID, "error", err)
			errs = append(errs, fmt.Errorf("tenant %s: %w", tenant.ID, err))
			continue
		}
		processed += n
	}
	return processed, errors.Join(errs...)
}

// processAllTenantsAndLog 處理 all 租戶 and log。
func (p *IdentityProvisioningOutboxProcessor) processAllTenantsAndLog(ctx context.Context, opts IdentityProvisioningOutboxOptions) {
	processed, err := p.ProcessAllTenants(ctx, opts)
	if err != nil {
		p.logger.WarnContext(ctx, "identity provisioning outbox processing failed", "error", err)
	}
	if processed > 0 {
		p.logger.InfoContext(ctx, "identity provisioning outbox processed", "events", processed)
	}
}

// normalizeIdentityProvisioningOptions 正規化身分開通選項。
func normalizeIdentityProvisioningOptions(opts IdentityProvisioningOutboxOptions) IdentityProvisioningOutboxOptions {
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultIdentityProvisioningBatchSize
	}
	if opts.MaxRetries <= 0 {
		opts.MaxRetries = defaultIdentityProvisioningMaxRetries
	}
	if opts.Interval <= 0 {
		opts.Interval = defaultIdentityProvisioningInterval
	}
	return opts
}
