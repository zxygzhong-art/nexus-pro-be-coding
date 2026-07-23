package repository

import (
	"context"
	"time"

	"nexus-pro-api/internal/domain"
)

// OrgStore 定義組織儲存層的行為契約。
type OrgStore interface {
	UpsertOrgUnit(context.Context, domain.OrgUnit) error
	UpdateOrgUnitOrgChartVisibility(ctx context.Context, tenantID, id string, showInOrgChart bool, updatedAt time.Time) error
	GetOrgUnit(ctx context.Context, tenantID, id string) (domain.OrgUnit, bool, error)
	ListOrgUnits(ctx context.Context, tenantID string) ([]domain.OrgUnit, error)
}

// PositionStore 定義崗位儲存層的行為契約。
type PositionStore interface {
	UpsertPosition(ctx context.Context, position domain.Position) error
	GetPosition(ctx context.Context, tenantID, id string) (domain.Position, bool, error)
	GetPositionByCode(ctx context.Context, tenantID, code string) (domain.Position, bool, error)
	GetPositionByName(ctx context.Context, tenantID, name string) (domain.Position, bool, error)
	ListPositions(ctx context.Context, tenantID string) ([]domain.Position, error)
}

// EmployeeStore 定義員工儲存層的行為契約。
type EmployeeStore interface {
	UpsertEmployee(ctx context.Context, employee domain.Employee) error
	UpdateEmployeeOrgChartVisibility(ctx context.Context, tenantID, id string, showInOrgChart bool, updatedAt time.Time) error
	GetEmployee(ctx context.Context, tenantID, id string) (domain.Employee, bool, error)
	ListEmployees(ctx context.Context, tenantID string) ([]domain.Employee, error)
	GetEmployeeByEmployeeNo(ctx context.Context, tenantID, employeeNo string) (domain.Employee, bool, error)
	GetEmployeeByCompanyEmail(ctx context.Context, tenantID, companyEmail string) (domain.Employee, bool, error)
	GetEmployeeByPersonalEmail(ctx context.Context, tenantID, personalEmail string) (domain.Employee, bool, error)
	GetEmployeeByAccountID(ctx context.Context, tenantID, accountID string) (domain.Employee, bool, error)
	GetEmployeeByBasicInfoField(ctx context.Context, tenantID, fieldName, fieldValue string) (domain.Employee, bool, error)
	ListEmployeesByQuery(ctx context.Context, tenantID string, query domain.EmployeeQuery) ([]domain.Employee, error)
	ListEmployeePageByQuery(ctx context.Context, tenantID string, query domain.EmployeeQuery) ([]domain.Employee, int, error)
	CountEmployeesByQuery(ctx context.Context, tenantID string, query domain.EmployeeQuery) (int, error)
}
