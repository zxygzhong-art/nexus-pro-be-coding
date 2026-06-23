package repository

import "context"

// Store aggregates all persistence contracts required by the service layer.
type Store interface {
	TenantStore
	AccountStore
	IdentityStore
	IAMStore
	OrgStore
	EmployeeStore
	AttendanceStore
	WorkflowStore
	KnowledgeStore
	AgentStore
	AuditStore
	AuthzEventStore
	OutboxStore
}

// TenantTransactor marks stores that can execute tenant-scoped write transactions.
type TenantTransactor interface {
	WithTenantTransaction(ctx context.Context, tenantID string, fn func(Store) error) error
}

// WithinTenantTransaction runs fn in a tenant transaction when the store supports it.
func WithinTenantTransaction(ctx context.Context, store Store, tenantID string, fn func(Store) error) error {
	if tx, ok := store.(TenantTransactor); ok {
		return tx.WithTenantTransaction(ctx, tenantID, fn)
	}
	return fn(store)
}
