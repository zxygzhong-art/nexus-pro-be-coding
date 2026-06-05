// Package reqctx stores and retrieves per-request values (principal, tenant-scoped
// DB session, request id) on a context.Context.
package reqctx

import (
	"context"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/ctxkeys"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/principal"
	"gorm.io/gorm"
)

// WithPrincipal returns a context carrying the resolved principal.
func WithPrincipal(ctx context.Context, p principal.Principal) context.Context {
	return context.WithValue(ctx, ctxkeys.Principal, p)
}

// Principal extracts the principal; ok is false when absent.
func Principal(ctx context.Context) (principal.Principal, bool) {
	p, ok := ctx.Value(ctxkeys.Principal).(principal.Principal)
	return p, ok
}

// WithTenantDB returns a context carrying the tenant-scoped GORM session.
func WithTenantDB(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, ctxkeys.TenantDB, tx)
}

// TenantDB extracts the tenant-scoped GORM session; nil when absent.
func TenantDB(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(ctxkeys.TenantDB).(*gorm.DB); ok {
		return tx
	}
	return nil
}

// WithRequestID returns a context carrying the request id.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxkeys.RequestID, id)
}

// RequestID extracts the request id (empty when absent).
func RequestID(ctx context.Context) string {
	if id, ok := ctx.Value(ctxkeys.RequestID).(string); ok {
		return id
	}
	return ""
}
