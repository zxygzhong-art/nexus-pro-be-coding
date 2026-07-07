package repository

import (
	"context"

	"nexus-pro-be/internal/domain"
)

// OutboxStore 定義 outbox 儲存層的行為契約。
type OutboxStore interface {
	AppendOutboxEvent(context.Context, domain.OutboxEvent) error
	ListOutboxEvents(ctx context.Context, tenantID string) ([]domain.OutboxEvent, error)
	UpdateOutboxEvent(context.Context, domain.OutboxEvent) error
}
