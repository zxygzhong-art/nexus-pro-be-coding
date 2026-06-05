// Package service is the HR Core domain service layer (员工管理).
//
// SCOPE: skeleton only. The permission-foundation milestone does NOT implement HR
// business logic. Per the PRD, this domain will own: employee CRUD (6-tab record),
// batch import (CSV/XLSX ≤500 with preview+validation), CSV export, batch delete,
// the employment state machine (试用/在职/留停/离职/待加入), and 内部经历/异动 history.
// Each operation will run inside the tenant tx, apply the authz decision's data
// scope + field policies, and be audited via the IAM middleware.
package service

import (
	"context"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/models"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/apperror"
)

// EmployeeService will own employee read/write/import/export operations.
type EmployeeService struct{}

// NewEmployeeService builds the service.
func NewEmployeeService() *EmployeeService { return &EmployeeService{} }

const notImpl = "员工管理 business logic not implemented in the foundation milestone"

// List returns employees in the caller's data scope (filter: dept/status/category;
// search: name/email/employee_no/phone). TODO: implement.
func (s *EmployeeService) List(_ context.Context) ([]models.Employee, error) {
	return nil, apperror.NotImplemented(notImpl)
}

// Get returns one employee (field policies applied, e.g. id_number/salary masked).
func (s *EmployeeService) Get(_ context.Context, _ string) (*models.Employee, error) {
	return nil, apperror.NotImplemented(notImpl)
}

// Create adds an employee (6-tab record). TODO.
func (s *EmployeeService) Create(_ context.Context, _ *models.Employee) error {
	return apperror.NotImplemented(notImpl)
}

// Update edits an employee. TODO.
func (s *EmployeeService) Update(_ context.Context, _ string, _ *models.Employee) error {
	return apperror.NotImplemented(notImpl)
}

// Delete removes one or more employees (supports batch). TODO.
func (s *EmployeeService) Delete(_ context.Context, _ []string) error {
	return apperror.NotImplemented(notImpl)
}

// Import ingests a CSV/XLSX batch (≤500) with preview+validation. TODO.
func (s *EmployeeService) Import(_ context.Context) error {
	return apperror.NotImplemented(notImpl)
}

// Export streams the filtered employee set as CSV. TODO.
func (s *EmployeeService) Export(_ context.Context) error {
	return apperror.NotImplemented(notImpl)
}

// Assignments returns an employee's 内部经历/异动 history. TODO.
func (s *EmployeeService) Assignments(_ context.Context, _ string) ([]models.EmployeeAssignment, error) {
	return nil, apperror.NotImplemented(notImpl)
}
