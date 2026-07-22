package repository

import (
	"context"
	"errors"
)

// Store 定義儲存層的行為契約。
type Store interface {
	TenantStore
	AccountStore
	IdentityStore
	IAMStore
	OrgStore
	PositionStore
	EmployeeStore
	AttendanceStore
	WorkflowStore
	TaskStore
	AgentStore
	KnowledgeStore
	NotificationStore
	AuditStore
	AuthzEventStore
	OutboxStore
}

// TenantTransactor 定義租戶 transactor 的行為契約。
type TenantTransactor interface {
	WithTenantTransaction(ctx context.Context, tenantID string, fn func(Store) error) error
}

// WithinTenantTransaction 處理 within 租戶 transaction。
func WithinTenantTransaction(ctx context.Context, store Store, tenantID string, fn func(Store) error) error {
	if tx, ok := store.(TenantTransactor); ok {
		return tx.WithTenantTransaction(ctx, tenantID, fn)
	}
	return errors.New("repository store does not support tenant transactions")
}
