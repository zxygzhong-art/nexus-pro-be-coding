package repository

import (
	"context"

	"nexus-pro-api/internal/domain"
)

// AuditStore 定義稽覈儲存層的行為契約。
type AuditStore interface {
	AppendAuditLog(context.Context, domain.AuditLog) error
	ListAuditLogs(ctx context.Context, tenantID string) ([]domain.AuditLog, error)
	ListAuditLogFacetSources(ctx context.Context, tenantID string) ([]domain.WorkspaceAuditLogFacetSource, error)
	ListAuditLogPage(ctx context.Context, tenantID string, page domain.PageRequest) ([]domain.AuditLog, int, error)
	ListAuditLogPageFiltered(ctx context.Context, tenantID string, query domain.WorkspaceAuditLogQuery, page domain.PageRequest) ([]domain.AuditLog, int, error)
}
