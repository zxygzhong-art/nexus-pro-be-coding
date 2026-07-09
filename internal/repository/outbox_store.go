package repository

import (
	"context"

	"nexus-pro-be/internal/domain"
)

// OutboxStore 定義 outbox 儲存層的行為契約。
type OutboxStore interface {
	AppendOutboxEvent(context.Context, domain.OutboxEvent) error
	ListOutboxEvents(ctx context.Context, tenantID string) ([]domain.OutboxEvent, error)
	// ClaimOutboxEvents atomically claims up to limit dispatchable events for processing.
	// maxRetries excludes rows whose retry_count has already reached the limit.
	ClaimOutboxEvents(ctx context.Context, tenantID string, limit, maxRetries int) ([]domain.OutboxEvent, error)
	UpdateOutboxEvent(context.Context, domain.OutboxEvent) error
}
