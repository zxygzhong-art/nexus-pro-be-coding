package repository

import (
	"context"

	"nexus-pro-be/internal/domain"
)

// OutboxStore persists tenant-scoped business events for later delivery.
type OutboxStore interface {
	AppendOutboxEvent(context.Context, domain.OutboxEvent) error
	ListOutboxEvents(ctx context.Context, tenantID string) ([]domain.OutboxEvent, error)
}
