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
	// ClaimOutboxEvents atomically leases up to limit due events. Expired leases are reclaimable.
	ClaimOutboxEvents(ctx context.Context, tenantID string, limit int, claimedAt, leaseUntil time.Time, claimOwner, claimToken string) ([]domain.OutboxEvent, error)
	// FinalizeOutboxEvent updates a claimed row only while its claim token still matches.
	FinalizeOutboxEvent(context.Context, domain.OutboxEvent) (bool, error)
	// RetryOutboxEvent resets a failed, parked, or dead-lettered event for immediate dispatch.
	RetryOutboxEvent(ctx context.Context, tenantID, id string, retriedAt time.Time) (domain.OutboxEvent, bool, error)
	// DeleteSucceededOutboxEventsBefore 刪除指定租戶已成功且建立時間早於 cutoff 的事件,回傳刪除筆數。
	DeleteSucceededOutboxEventsBefore(ctx context.Context, tenantID string, before time.Time) (int64, error)
}
