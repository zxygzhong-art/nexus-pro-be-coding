package repository

import (
	"context"

	"nexus-pro-be/internal/domain"
)

type TenantStore interface {
	UpsertTenant(context.Context, domain.Tenant) error
	GetTenant(context.Context, string) (domain.Tenant, bool, error)
	ListTenants(context.Context) ([]domain.Tenant, error)
}
