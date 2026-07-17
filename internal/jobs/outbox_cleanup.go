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
	defaultOutboxCleanupInterval  = 24 * time.Hour
	defaultOutboxCleanupRetention = 7 * 24 * time.Hour
)

// OutboxCleanupOptions 定義 outbox 清理選項的資料結構。
type OutboxCleanupOptions struct {
	Interval  time.Duration
	Retention time.Duration
}

// OutboxCleaner 定期刪除已成功處理的歷史 outbox 事件,避免資料表只增不刪。
type OutboxCleaner struct {
	store  repository.Store
	logger *slog.Logger
	now    func() time.Time
}

// NewOutboxCleaner 建立 outbox 清理 job。
func NewOutboxCleaner(store repository.Store, logger *slog.Logger) *OutboxCleaner {
	if logger == nil {
		logger = slog.Default()
	}
	return &OutboxCleaner{
		store:  store,
		logger: logger,
		now:    time.Now,
	}
}

// Run 執行背景工作主迴圈。
func (c *OutboxCleaner) Run(ctx context.Context, opts OutboxCleanupOptions) {
	opts = normalizeOutboxCleanupOptions(opts)
	c.cleanupAllTenantsAndLog(ctx, opts)
	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.cleanupAllTenantsAndLog(ctx, opts)
		}
	}
}

// CleanupAllTenants 對所有租戶刪除建立時間早於 retention 截止點的已成功事件。
// 單一租戶失敗只記錄並繼續處理其餘租戶,迴圈結束後聚合回傳錯誤。
func (c *OutboxCleaner) CleanupAllTenants(ctx context.Context, opts OutboxCleanupOptions) (int64, error) {
	opts = normalizeOutboxCleanupOptions(opts)
	if c == nil || c.store == nil {
		return 0, errors.New("outbox cleaner requires store")
	}
	tenants, err := c.store.ListTenants(ctx)
	if err != nil {
		return 0, err
	}
	cutoff := c.now().UTC().Add(-opts.Retention)
	var deleted int64
	var errs []error
	for _, tenant := range tenants {
		n, err := c.store.DeleteSucceededOutboxEventsBefore(ctx, tenant.ID, cutoff)
		if err != nil {
			c.logger.WarnContext(ctx, "outbox cleanup failed for tenant", "tenant_id", tenant.ID, "error", err)
			errs = append(errs, fmt.Errorf("tenant %s: %w", tenant.ID, err))
			continue
		}
		deleted += n
	}
	return deleted, errors.Join(errs...)
}

// cleanupAllTenantsAndLog 處理 all 租戶 and log。
func (c *OutboxCleaner) cleanupAllTenantsAndLog(ctx context.Context, opts OutboxCleanupOptions) {
	deleted, err := c.CleanupAllTenants(ctx, opts)
	if err != nil {
		c.logger.WarnContext(ctx, "outbox cleanup failed", "error", err)
	}
	if deleted > 0 {
		c.logger.InfoContext(ctx, "succeeded outbox events cleaned", "events", deleted)
	}
}

// normalizeOutboxCleanupOptions 正規化 outbox 清理選項。
func normalizeOutboxCleanupOptions(opts OutboxCleanupOptions) OutboxCleanupOptions {
	if opts.Interval <= 0 {
		opts.Interval = defaultOutboxCleanupInterval
	}
	if opts.Retention <= 0 {
		opts.Retention = defaultOutboxCleanupRetention
	}
	return opts
}
