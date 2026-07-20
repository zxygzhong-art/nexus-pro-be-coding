package repository

import (
	"context"

	"nexus-pro-api/internal/domain"
)

// TenantStore 定義租戶儲存層的行為契約。
type TenantStore interface {
	UpsertTenant(context.Context, domain.Tenant) error
	GetTenant(context.Context, string) (domain.Tenant, bool, error)
	ListTenants(context.Context) ([]domain.Tenant, error)
}
