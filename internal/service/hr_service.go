package service

import (
	"strings"

	"nexus-pro-be/internal/utils"
)

type HRService struct {
	*Service
	store hrStore
}

func (c *Service) HR() HRService {
	return HRService{Service: c, store: c.store}
}

func (c *Service) QueryEmployees(ctx RequestContext, query EmployeeQuery) (PageResponse[Employee], error) {
	return c.HR().QueryEmployees(ctx, query)
}

func (c *Service) ListEmployees(ctx RequestContext) ([]Employee, error) {
	return c.HR().ListEmployees(ctx)
}

func (c *Service) CreateEmployee(ctx RequestContext, input CreateEmployeeInput) (Employee, error) {
	return c.HR().CreateEmployee(ctx, input)
}

func (c *Service) PreviewCreateEmployee(ctx RequestContext, input CreateEmployeeInput) (EmployeePreviewResponse, error) {
	return c.HR().PreviewCreateEmployee(ctx, input)
}

func (c *Service) CreateEmployeeAggregate(ctx RequestContext, input CreateEmployeeInput) (Employee, error) {
	return c.HR().CreateEmployeeAggregate(ctx, input)
}

func (c *Service) GetEmployee(ctx RequestContext, id string) (Employee, error) {
	return c.HR().GetEmployee(ctx, id)
}

func (c *Service) UpdateEmployee(ctx RequestContext, id string, input UpdateEmployeeInput) (Employee, error) {
	return c.HR().UpdateEmployee(ctx, id, input)
}

func (c *Service) PreviewUpdateEmployee(ctx RequestContext, id string, input UpdateEmployeeInput) (EmployeePreviewResponse, error) {
	return c.HR().PreviewUpdateEmployee(ctx, id, input)
}

func (c *Service) UpdateEmployeeAvatar(ctx RequestContext, id string, input EmployeeAvatarInput) (Employee, error) {
	return c.HR().UpdateEmployeeAvatar(ctx, id, input)
}

func (c *Service) DeleteEmployeeAvatar(ctx RequestContext, id string) (Employee, error) {
	return c.HR().DeleteEmployeeAvatar(ctx, id)
}

func (c *Service) EmployeeStats(ctx RequestContext, query EmployeeQuery) (EmployeeStats, error) {
	return c.HR().EmployeeStats(ctx, query)
}

func (c *Service) EmployeeOptions(ctx RequestContext) (EmployeeOptions, error) {
	return c.HR().EmployeeOptions(ctx)
}

func (c *Service) EmployeeImportTemplate(ctx RequestContext, format string) ([]byte, string, string, error) {
	return c.HR().EmployeeImportTemplate(ctx, format)
}

func (c *Service) PreviewEmployeeImport(ctx RequestContext, input EmployeeImportPreviewInput) (EmployeeImportSession, error) {
	return c.HR().PreviewEmployeeImport(ctx, input)
}

func (c *Service) ConfirmEmployeeImport(ctx RequestContext, sessionID string, input EmployeeImportConfirmInput) (EmployeeImportSession, error) {
	return c.HR().ConfirmEmployeeImport(ctx, sessionID, input)
}

func (c *Service) ExportEmployeesCSV(ctx RequestContext, query EmployeeQuery) ([]byte, string, error) {
	return c.HR().ExportEmployeesCSV(ctx, query)
}

func (c *Service) ExportEmployees(ctx RequestContext, queries ...EmployeeQuery) ([]Employee, error) {
	return c.HR().ExportEmployees(ctx, queries...)
}

func (c *Service) BatchDeleteEmployees(ctx RequestContext, input BatchDeleteEmployeesInput) (BatchEmployeeResponse, error) {
	return c.HR().BatchDeleteEmployees(ctx, input)
}

func (c *Service) DeleteEmployee(ctx RequestContext, id string) (Employee, error) {
	return c.HR().DeleteEmployee(ctx, id)
}

func (c *Service) InviteEmployee(ctx RequestContext, id string, input InviteEmployeeInput) (Employee, error) {
	return c.HR().InviteEmployee(ctx, id, input)
}

func (c *Service) TransitionEmployeeStatus(ctx RequestContext, id string, input StatusTransitionInput) (Employee, error) {
	return c.HR().TransitionEmployeeStatus(ctx, id, input)
}

func (c *Service) UpdateEmployeeStatus(ctx RequestContext, id, status string) (Employee, error) {
	return c.HR().UpdateEmployeeStatus(ctx, id, status)
}

func (c *Service) ListOrgUnits(ctx RequestContext) ([]OrgUnit, error) {
	return c.HR().ListOrgUnits(ctx)
}

func (c *Service) ListOrgUnitPage(ctx RequestContext, page PageRequest) (PageResponse[OrgUnit], error) {
	return c.HR().ListOrgUnitPage(ctx, page)
}

func (c *Service) CreateOrgUnit(ctx RequestContext, input CreateOrgUnitInput) (OrgUnit, error) {
	return c.HR().CreateOrgUnit(ctx, input)
}

func (c HRService) ListOrgUnits(ctx RequestContext) ([]OrgUnit, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return nil, err
	}
	return c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
}

func (c HRService) ListOrgUnitPage(ctx RequestContext, page PageRequest) (PageResponse[OrgUnit], error) {
	items, err := c.ListOrgUnits(ctx)
	if err != nil {
		return PageResponse[OrgUnit]{}, err
	}
	items = utils.SortOrgUnits(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

func (c HRService) CreateOrgUnit(ctx RequestContext, input CreateOrgUnitInput) (OrgUnit, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return OrgUnit{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return OrgUnit{}, BadRequest("org unit name is required")
	}
	var unit OrgUnit
	if err := c.withTenantTransaction(ctx, func(tx *Service) error {
		next := OrgUnit{
			ID:        utils.NewID("ou"),
			TenantID:  ctx.TenantID,
			Code:      strings.TrimSpace(input.Code),
			Name:      strings.TrimSpace(input.Name),
			ParentID:  strings.TrimSpace(input.ParentID),
			CreatedAt: tx.Now(),
		}
		if next.ParentID != "" {
			parent, ok, err := tx.store.GetOrgUnit(goContext(ctx), ctx.TenantID, next.ParentID)
			if err != nil {
				return err
			}
			if !ok {
				return NotFound("org unit", next.ParentID)
			}
			next.Path = append(utils.CopyStrings(parent.Path), parent.ID, next.ID)
		} else {
			next.Path = []string{next.ID}
		}
		if err := tx.store.UpsertOrgUnit(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.touchAuthzConfig(ctx, "org.unit.upsert", map[string]any{"org_unit_id": next.ID, "parent_id": next.ParentID}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "org.unit.create", "org_unit", next.ID, "medium", map[string]any{"name": next.Name}); err != nil {
			return err
		}
		unit = next
		return nil
	}); err != nil {
		return OrgUnit{}, err
	}
	c.logInfo(ctx, "org unit created",
		"org_unit_id", unit.ID,
		"parent_id", unit.ParentID,
		"code", unit.Code,
	)
	return unit, nil
}

func (c HRService) ListEmployees(ctx RequestContext) ([]Employee, error) {
	response, err := c.QueryEmployees(ctx, EmployeeQuery{})
	if err != nil {
		return nil, err
	}
	return response.Items, nil
}

func (c HRService) CreateEmployee(ctx RequestContext, input CreateEmployeeInput) (Employee, error) {
	return c.CreateEmployeeAggregate(ctx, input)
}

func (c HRService) ExportEmployees(ctx RequestContext, queries ...EmployeeQuery) ([]Employee, error) {
	query := EmployeeQuery{}
	if len(queries) > 0 {
		query = normalizeEmployeeQuery(queries[0])
	}
	account, decision, _, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, Action: ActionExport, Context: map[string]any{"filters": employeeQueryApprovalFilters(query)}},
		AuditTarget{Resource: string(ResourceEmployeeCollection)},
	)
	if err != nil {
		return nil, err
	}
	if err := c.rejectOversizedEmployeeExport(ctx, query, decision); err != nil {
		return nil, err
	}
	items, err := c.listEmployeesForQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	items, err = c.applyEmployeeDecision(ctx, account, items, decision)
	if err != nil {
		return nil, err
	}
	items = filterEmployeeQuery(items, query)
	sortEmployees(items, query.Sort)
	if len(items) > maxEmployeeExportRows {
		return nil, employeeExportLimitError()
	}
	if err := c.audit(ctx, "hr.employee.export", string(ResourceEmployeeCollection), "", string(SeverityHigh), auditDecisionDetails(ctx, decision, map[string]any{
		"filters":           query,
		"row_count":         len(items),
		"restricted_fields": restrictedEmployeeFieldPolicies(decision.FieldPolicies),
	})); err != nil {
		return nil, err
	}
	c.logInfo(ctx, "employees exported",
		"row_count", len(items),
		"restricted_fields", restrictedEmployeeFieldPolicies(decision.FieldPolicies),
		"scope", decision.EffectiveScope,
	)
	return items, nil
}

func (c HRService) DeleteEmployee(ctx RequestContext, id string) (Employee, error) {
	account, decision, audit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: id, Action: ActionDelete},
		AuditTarget{Resource: string(ResourceEmployee), Target: id},
	)
	if err != nil {
		return Employee{}, err
	}
	var employee Employee
	previousStatus := ""
	accountDisabled := false
	if err := c.withTenantTransaction(ctx, func(tx *Service) error {
		next, ok, err := tx.store.GetEmployee(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("employee", id)
		}
		visible, err := tx.filterEmployeesByDecision(ctx, account, []Employee{next}, decision)
		if err != nil {
			return err
		}
		if len(visible) == 0 {
			return forbiddenDataScope("employee is outside data scope")
		}
		before := next
		previousStatus = employeeStatus(before)
		next.Status = string(EmployeeStatusDeleted)
		next.EmploymentStatus = string(EmployeeStatusDeleted)
		next.UpdatedAt = tx.Now()
		next = tx.appendHistoryForChangedEmployment(before, next, "刪除")
		if err := tx.store.UpsertEmployee(goContext(ctx), next); err != nil {
			return err
		}
		if next.AccountID != "" {
			account, ok, err := tx.store.GetAccount(goContext(ctx), ctx.TenantID, next.AccountID)
			if err != nil {
				return err
			}
			if ok {
				account.Status = string(AccountStatusDisabled)
				if err := tx.store.UpsertAccount(goContext(ctx), account); err != nil {
					return err
				}
				accountDisabled = true
			}
		}
		if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeOffboarded), next.ID, map[string]any{"employee_id": next.ID, "status": string(EmployeeStatusDeleted)}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.delete", string(ResourceEmployee), next.ID, string(SeverityHigh), auditDecisionDetails(ctx, decision, map[string]any{
			"previous_status": employeeStatus(before),
			"status":          string(EmployeeStatusDeleted),
		})); err != nil {
			return err
		}
		if err := audit.CommitWith(ctx, tx); err != nil {
			return err
		}
		employee = next
		return nil
	}); err != nil {
		return Employee{}, err
	}
	c.logWarn(ctx, "employee deleted",
		"employee_id", employee.ID,
		"employee_no", employee.EmployeeNo,
		"previous_status", previousStatus,
		"status", employeeStatus(employee),
		"account_id", employee.AccountID,
		"account_disabled", accountDisabled,
	)
	return employee, nil
}

func (c HRService) UpdateEmployeeStatus(ctx RequestContext, id, status string) (Employee, error) {
	status = normalizeEmployeeStatus(status)
	if status == "" {
		return Employee{}, BadRequest("status is required")
	}
	if status == string(EmployeeStatusResigned) {
		return Employee{}, BadRequest("resigned status requires status-transition")
	}
	if status == string(EmployeeStatusDeleted) {
		return Employee{}, BadRequest("deleted status requires delete")
	}
	if !validEmployeeStatus(status, false) {
		return Employee{}, BadRequest("invalid employee status")
	}
	account, decision, audit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: id, Action: ActionUpdateStatus},
		AuditTarget{Resource: string(ResourceEmployee), Target: id},
	)
	if err != nil {
		return Employee{}, err
	}
	var employee Employee
	previousStatus := ""
	if err := c.withTenantTransaction(ctx, func(tx *Service) error {
		next, ok, err := tx.store.GetEmployee(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("employee", id)
		}
		if err := ensureEmployeeStatusTransition(employeeStatus(next), status); err != nil {
			return err
		}
		visible, err := tx.filterEmployeesByDecision(ctx, account, []Employee{next}, decision)
		if err != nil {
			return err
		}
		if len(visible) == 0 {
			return forbiddenDataScope("employee is outside data scope")
		}
		before := next
		previousStatus = employeeStatus(before)
		next.Status = status
		next.EmploymentStatus = status
		next.UpdatedAt = tx.Now()
		next = tx.appendHistoryForChangedEmployment(before, next, "狀態更新")
		if err := tx.store.UpsertEmployee(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeStatusChanged), next.ID, map[string]any{"employee_id": next.ID, "status": status}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.status_update", string(ResourceEmployee), next.ID, string(SeverityHigh), auditDecisionDetails(ctx, decision, map[string]any{
			"previous_status": employeeStatus(before),
			"status":          status,
		})); err != nil {
			return err
		}
		if err := audit.CommitWith(ctx, tx); err != nil {
			return err
		}
		employee = next
		return nil
	}); err != nil {
		return Employee{}, err
	}
	c.logInfo(ctx, "employee status updated",
		"employee_id", employee.ID,
		"employee_no", employee.EmployeeNo,
		"previous_status", previousStatus,
		"status", employeeStatus(employee),
	)
	return employee, nil
}
