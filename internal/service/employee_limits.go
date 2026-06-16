package service

import "context"

const (
	defaultEmployeePage     = 1
	defaultEmployeePageSize = 20
	maxEmployeePageSize     = 100
	maxEmployeeImportBytes  = 10 << 20
	maxEmployeeImportRows   = 500
	maxEmployeeExportRows   = 5000
	employeeNoPrefix        = "IKL"
)

type employeeUniqueLookupStore interface {
	GetEmployeeByEmployeeNo(context.Context, string, string) (Employee, bool, error)
	GetEmployeeByCompanyEmail(context.Context, string, string) (Employee, bool, error)
	GetEmployeeByAccountID(context.Context, string, string) (Employee, bool, error)
}
