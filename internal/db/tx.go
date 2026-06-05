package db

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// WithTenant runs fn inside a transaction whose session has app.current_tenant set
// to tenantID via SET LOCAL, so every query is RLS-scoped to that tenant. Using
// SET LOCAL ties the GUC to the transaction lifetime, avoiding leakage across the
// pooled connection.
func WithTenant(ctx context.Context, gdb *gorm.DB, tenantID string, fn func(tx *gorm.DB) error) error {
	if tenantID == "" {
		return fmt.Errorf("tenant id required for tenant-scoped transaction")
	}
	return gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// set_config(name, value, is_local=true) == SET LOCAL, parameterized safely.
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantID).Error; err != nil {
			return fmt.Errorf("set tenant context: %w", err)
		}
		return fn(tx)
	})
}
