// Package ctxkeys defines typed context keys shared across the request lifecycle.
package ctxkeys

type key string

const (
	// Principal is the resolved caller (tenant + account) for the request.
	Principal key = "principal"
	// RequestID is the per-request correlation id.
	RequestID key = "request_id"
	// TenantDB is the tenant-scoped *gorm.DB session (with app.current_tenant set).
	TenantDB key = "tenant_db"
)
