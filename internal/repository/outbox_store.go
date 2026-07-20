package repository

import (
	"context"
	"time"

	"nexus-pro-api/internal/domain"
)

// OutboxStore 定義 outbox 儲存層的行為契約。
type OutboxStore interface {
	AppendOutboxEvent(context.Context, domain.OutboxEvent) error
	ListOutboxEvents(ctx context.Context, tenantID string) ([]domain.OutboxEvent, error)
	GetOutboxEventByID(ctx context.Context, tenantID, id string) (domain.OutboxEvent, bool, error)
	// ListOutboxEventPage 回傳符合查詢條件的一頁事件與符合條件的總筆數。
	ListOutboxEventPage(ctx context.Context, tenantID string, query domain.OutboxEventQuery, page domain.PageRequest) ([]domain.OutboxEvent, int, error)
	// ClaimOutboxEvents atomically claims up to limit dispatchable events for processing.
	// maxRetries excludes rows whose retry_count has already reached the limit.
	ClaimOutboxEvents(ctx context.Context, tenantID string, limit, maxRetries int) ([]domain.OutboxEvent, error)
	UpdateOutboxEvent(context.Context, domain.OutboxEvent) error
	// DeleteSucceededOutboxEventsBefore 刪除指定租戶已成功且建立時間早於 cutoff 的事件,回傳刪除筆數。
	DeleteSucceededOutboxEventsBefore(ctx context.Context, tenantID string, before time.Time) (int64, error)
}
