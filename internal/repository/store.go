package repository

import "context"

type Store interface {
	TenantStore
	AccountStore
	IAMStore
	OrgStore
	EmployeeStore
	AttendanceStore
	WorkflowStore
	KnowledgeStore
	AgentStore
	AuditStore
	AuthzEventStore
}

type TenantTransactor interface {
	WithTenantTransaction(ctx context.Context, tenantID string, fn func(Store) error) error
}

func WithinTenantTransaction(ctx context.Context, store Store, tenantID string, fn func(Store) error) error {
	if tx, ok := store.(TenantTransactor); ok {
		return tx.WithTenantTransaction(ctx, tenantID, fn)
	}
	return fn(store)
}
