package repository

import (
	"context"

	"nexus-pro-be/internal/domain"
)

// OrgStore persists tenant organization units.
type OrgStore interface {
	UpsertOrgUnit(context.Context, domain.OrgUnit) error
	GetOrgUnit(ctx context.Context, tenantID, id string) (domain.OrgUnit, bool, error)
	ListOrgUnits(ctx context.Context, tenantID string) ([]domain.OrgUnit, error)
}

// EmployeeStore persists people-domain employee data and import sessions.
type EmployeeStore interface {
	UpsertEmployee(ctx context.Context, employee domain.Employee) error
	GetEmployee(ctx context.Context, tenantID, id string) (domain.Employee, bool, error)
	ListEmployees(ctx context.Context, tenantID string) ([]domain.Employee, error)
	ListEmployeesByQuery(ctx context.Context, tenantID string, query domain.EmployeeQuery) ([]domain.Employee, error)
	ListEmployeePageByQuery(ctx context.Context, tenantID string, query domain.EmployeeQuery) ([]domain.Employee, int, error)
	CountEmployeesByQuery(ctx context.Context, tenantID string, query domain.EmployeeQuery) (int, error)
	NextEmployeeNo(ctx context.Context, tenantID, prefix string) (string, error)
	UpsertEmployeeImportSession(ctx context.Context, session domain.EmployeeImportSession) error
	GetEmployeeImportSession(ctx context.Context, tenantID, id string) (domain.EmployeeImportSession, bool, error)
}
