package service

import "strings"

// listBusinessEmployees returns the employee population exposed to product
// workflows. Storage-level employee reads remain unfiltered for provisioning,
// identity binding, uniqueness checks, and audit reconstruction.
func (c *Service) listBusinessEmployees(ctx RequestContext) ([]Employee, error) {
	return c.store.ListEmployeesByQuery(goContext(ctx), ctx.TenantID, EmployeeQuery{})
}

// getBusinessEmployee applies the same population rule to single-employee
// workflows so a known super-admin employee ID cannot bypass collection filters.
func (c *Service) getBusinessEmployee(ctx RequestContext, employeeID string) (Employee, bool, error) {
	employeeID = strings.TrimSpace(employeeID)
	if employeeID == "" {
		return Employee{}, false, nil
	}
	stored, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, employeeID)
	if err != nil || !ok {
		return Employee{}, ok, err
	}
	items, err := c.store.ListEmployeesByQuery(goContext(ctx), ctx.TenantID, EmployeeQuery{
		EmploymentStatus: employeeStatus(stored),
		Scope:            EmployeeScopeConstraint{EmployeeIDs: []string{employeeID}},
	})
	if err != nil {
		return Employee{}, false, err
	}
	if len(items) == 0 {
		return Employee{}, false, nil
	}
	return items[0], true, nil
}
