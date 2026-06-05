// Package handler exposes the HR Core HTTP endpoints (员工管理).
//
// SCOPE: skeleton only. Endpoints are registered and permission-gated to mark the
// business landing spots from the PRD, but return 501 until the HR domain is
// implemented.
package handler

import (
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/hr/service"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/response"
	"github.com/gin-gonic/gin"
)

// Handler bundles the HR domain services.
type Handler struct {
	employees *service.EmployeeService
	orgUnits  *service.OrgUnitService
}

// New builds the HR handler.
func New(employees *service.EmployeeService, orgUnits *service.OrgUnitService) *Handler {
	return &Handler{employees: employees, orgUnits: orgUnits}
}

// ListEmployees GET /v1/hr/employees (hr.employee.read).
func (h *Handler) ListEmployees(c *gin.Context) {
	items, err := h.employees.List(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Items(c, items)
}

// GetEmployee GET /v1/hr/employees/:id (hr.employee.read).
func (h *Handler) GetEmployee(c *gin.Context) {
	emp, err := h.employees.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, emp)
}

// CreateEmployee POST /v1/hr/employees (hr.employee.write).
func (h *Handler) CreateEmployee(c *gin.Context) {
	response.Error(c, h.employees.Create(c.Request.Context(), nil))
}

// UpdateEmployee PUT /v1/hr/employees/:id (hr.employee.write).
func (h *Handler) UpdateEmployee(c *gin.Context) {
	response.Error(c, h.employees.Update(c.Request.Context(), c.Param("id"), nil))
}

// DeleteEmployee DELETE /v1/hr/employees/:id (hr.employee.delete).
func (h *Handler) DeleteEmployee(c *gin.Context) {
	response.Error(c, h.employees.Delete(c.Request.Context(), []string{c.Param("id")}))
}

// ImportEmployees POST /v1/hr/employees/import (hr.employee.import).
func (h *Handler) ImportEmployees(c *gin.Context) {
	response.Error(c, h.employees.Import(c.Request.Context()))
}

// ExportEmployees GET /v1/hr/employees/export (hr.employee.export).
func (h *Handler) ExportEmployees(c *gin.Context) {
	response.Error(c, h.employees.Export(c.Request.Context()))
}

// ListEmployeeAssignments GET /v1/hr/employees/:id/assignments (hr.employee.read).
func (h *Handler) ListEmployeeAssignments(c *gin.Context) {
	items, err := h.employees.Assignments(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Items(c, items)
}

// ListOrgUnits GET /v1/org/units (hr.org_unit.read).
func (h *Handler) ListOrgUnits(c *gin.Context) {
	items, err := h.orgUnits.List(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Items(c, items)
}
