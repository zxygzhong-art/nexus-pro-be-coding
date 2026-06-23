package repository

import (
	"context"

	"nexus-pro-be/internal/domain"
)

type AuditStore interface {
	AppendAuditLog(context.Context, domain.AuditLog) error
	ListAuditLogs(ctx context.Context, tenantID string) ([]domain.AuditLog, error)
	ListAuditLogPage(ctx context.Context, tenantID string, page domain.PageRequest) ([]domain.AuditLog, int, error)
}
