package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

// HRService 定義 HR 服務的資料結構。
type HRService struct {
	*Service
	store hrStore
}

// HR 處理 HR 的服務流程。
func (c *Service) HR() HRService {
	return HRService{Service: c, store: c.store}
}

// ListOrgUnits 列出組織單位的服務流程。
func (c HRService) ListOrgUnits(ctx RequestContext) ([]OrgUnit, error) {
	account, decision, err := c.Service.requireServiceAuthz(ctx, AppHR, ResourceOrgUnit, ActionRead, "")
	if err != nil {
		return nil, err
	}
	units, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	return c.filterOrgUnitsByDecision(ctx, account, decision, units)
}

// ListOrgUnitPage 列出組織單位分頁的服務流程。
func (c HRService) ListOrgUnitPage(ctx RequestContext, page PageRequest) (PageResponse[OrgUnit], error) {
	items, err := c.ListOrgUnits(ctx)
	if err != nil {
		return PageResponse[OrgUnit]{}, err
	}
	items = utils.SortOrgUnits(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

// CreateOrgUnit 建立組織單位的服務流程。
func (c HRService) CreateOrgUnit(ctx RequestContext, input CreateOrgUnitInput) (OrgUnit, error) {
	if _, _, err := c.Service.requireServiceAuthz(ctx, AppHR, ResourceOrgUnit, ActionCreate, ""); err != nil {
		return OrgUnit{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return OrgUnit{}, BadRequest("org unit name is required")
	}
	var unit OrgUnit
	if err := c.withTransaction(ctx, func(tx HRService) error {
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
			next.Path = append(utils.CopyStrings(parent.Path), next.ID)
		} else {
			next.Path = []string{next.ID}
		}
		if err := tx.store.UpsertOrgUnit(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.Service.syncOrgUnitRelationshipTuples(ctx, OrgUnit{}, next); err != nil {
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

// ListEmployees 列出員工的服務流程。
func (c HRService) ListEmployees(ctx RequestContext) ([]Employee, error) {
	response, err := c.QueryEmployees(ctx, EmployeeQuery{})
	if err != nil {
		return nil, err
	}
	return response.Items, nil
}

// CreateEmployee 建立員工的服務流程。
func (c HRService) CreateEmployee(ctx RequestContext, input CreateEmployeeInput) (Employee, error) {
	return c.CreateEmployeeAggregate(ctx, input)
}

// ExportEmployees 匯出員工的服務流程。
func (c HRService) ExportEmployees(ctx RequestContext, queries ...EmployeeQuery) ([]Employee, error) {
	query := EmployeeQuery{}
	if len(queries) > 0 {
		query = normalizeEmployeeQuery(queries[0])
	}
	items, _, err := c.exportEmployees(ctx, query)
	return items, err
}

// exportEmployees 匯出員工的服務流程。
func (c HRService) exportEmployees(ctx RequestContext, query EmployeeQuery) ([]Employee, CheckResult, error) {
	account, decision, _, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, Action: ActionExport, Context: map[string]any{"filters": employeeQueryApprovalFilters(query)}},
		AuditTarget{Resource: string(ResourceEmployeeCollection)},
	)
	if err != nil {
		return nil, CheckResult{}, err
	}
	scopedQuery, err := c.employeeQueryWithDecisionScope(ctx, account, query, decision)
	if err != nil {
		return nil, CheckResult{}, err
	}
	if err := c.rejectOversizedEmployeeExport(ctx, scopedQuery); err != nil {
		return nil, CheckResult{}, err
	}
	items, err := c.listEmployeesForQuery(ctx, scopedQuery)
	if err != nil {
		return nil, CheckResult{}, err
	}
	items, err = c.applyEmployeeDecision(ctx, account, items, decision)
	if err != nil {
		return nil, CheckResult{}, err
	}
	sortEmployees(items, query.Sort)
	if len(items) > maxEmployeeExportRows {
		return nil, CheckResult{}, employeeExportLimitError()
	}
	if err := c.auditSensitiveEmployeeRead(ctx, decision, items, ""); err != nil {
		return nil, CheckResult{}, err
	}
	if err := c.audit(ctx, "hr.employee.export", string(ResourceEmployeeCollection), "", string(SeverityHigh), auditDecisionDetails(ctx, decision, map[string]any{
		"filters":           query,
		"row_count":         len(items),
		"restricted_fields": restrictedEmployeeFieldPolicies(decision.FieldPolicies),
	})); err != nil {
		return nil, CheckResult{}, err
	}
	c.logInfo(ctx, "employees exported",
		"row_count", len(items),
		"restricted_fields", restrictedEmployeeFieldPolicies(decision.FieldPolicies),
		"scope", decision.EffectiveScope,
	)
	return items, decision, nil
}

// DeleteEmployee 刪除員工的服務流程。
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
	if err := c.withTransaction(ctx, func(tx HRService) error {
		next, ok, err := tx.store.GetEmployee(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("employee", id)
		}
		if employeeStatus(next) == string(EmployeeStatusDeleted) {
			return Conflict("employee is already deleted")
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
				beforeAccount := account
				account.Status = string(AccountStatusDisabled)
				if err := tx.store.UpsertAccount(goContext(ctx), account); err != nil {
					return err
				}
				if err := tx.Service.syncAccountTenantMembershipTuple(ctx, beforeAccount, account); err != nil {
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
		if err := audit.CommitWith(ctx, tx.Service); err != nil {
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

// UpdateEmployeeStatus 更新員工狀態的服務流程。
func (c HRService) UpdateEmployeeStatus(ctx RequestContext, id, status string) (Employee, error) {
	status = normalizeEmployeeStatus(status)
	if status == "" {
		return Employee{}, BadRequest("status is required")
	}
	if status == string(EmployeeStatusResigned) {
		return Employee{}, BadRequest("resigned status requires status-transition")
	}
	if status == string(EmployeeStatusLeaveSuspended) {
		return Employee{}, BadRequest("leave_suspended status requires status-transition")
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
	if err := c.withTransaction(ctx, func(tx HRService) error {
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
		if err := audit.CommitWith(ctx, tx.Service); err != nil {
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

// QueryEmployees 處理查詢員工的服務流程。
func (c HRService) QueryEmployees(ctx RequestContext, query EmployeeQuery) (PageResponse[Employee], error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return PageResponse[Employee]{}, err
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, Action: ActionRead})
	if err != nil {
		return PageResponse[Employee]{}, err
	}
	query = normalizeEmployeeQuery(query)
	if !decision.Allowed {
		c.logWarn(ctx, "employee query denied",
			"reason", decision.Reason,
			"missing_permissions", decision.MissingPermissions,
		)
		return PageResponse[Employee]{}, forbiddenAuthz(decision)
	}
	authzAudit := AuthzAudit{service: c.Service, target: AuditTarget{Event: "hr.employee.query", Resource: string(ResourceEmployeeCollection)}, decision: decision}
	scopedQuery, err := c.employeeQueryWithDecisionScope(ctx, account, query, decision)
	if err != nil {
		return PageResponse[Employee]{}, err
	}
	if decisionUsesOpenFGAScopeCheck(decision) {
		items, err := c.listEmployeesForQuery(ctx, scopedQuery)
		if err != nil {
			return PageResponse[Employee]{}, err
		}
		items, err = c.applyEmployeeDecision(ctx, account, items, decision)
		if err != nil {
			return PageResponse[Employee]{}, err
		}
		sortEmployees(items, query.Sort)
		page := utils.PageResponse(items, PageRequest{Page: query.Page, PageSize: query.PageSize, Sort: query.Sort})
		if err := c.auditSensitiveEmployeeRead(ctx, decision, page.Items, ""); err != nil {
			return PageResponse[Employee]{}, err
		}
		if err := authzAudit.Commit(ctx); err != nil {
			return PageResponse[Employee]{}, err
		}
		return page, nil
	}
	items, total, err := c.store.ListEmployeePageByQuery(goContext(ctx), ctx.TenantID, scopedQuery)
	if err != nil {
		return PageResponse[Employee]{}, err
	}
	items, err = c.applyEmployeeDecision(ctx, account, items, decision)
	if err != nil {
		return PageResponse[Employee]{}, err
	}
	if err := c.auditSensitiveEmployeeRead(ctx, decision, items, ""); err != nil {
		return PageResponse[Employee]{}, err
	}
	if err := authzAudit.Commit(ctx); err != nil {
		return PageResponse[Employee]{}, err
	}
	return PageResponse[Employee]{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize, Sort: query.Sort}, nil
}

// GetEmployee 取得員工的服務流程。
func (c HRService) GetEmployee(ctx RequestContext, id string) (Employee, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return Employee{}, err
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: id, Action: ActionRead})
	if err != nil {
		return Employee{}, err
	}
	if !decision.Allowed {
		return Employee{}, forbiddenAuthz(decision)
	}
	employee, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return Employee{}, err
	}
	if !ok {
		return Employee{}, NotFound("employee", id)
	}
	visible, err := c.applyEmployeeDecision(ctx, account, []Employee{employee}, decision)
	if err != nil {
		return Employee{}, err
	}
	if len(visible) == 0 {
		return Employee{}, forbiddenDataScope("employee is outside data scope")
	}
	if err := c.auditSensitiveEmployeeRead(ctx, decision, visible, visible[0].ID); err != nil {
		return Employee{}, err
	}
	return visible[0], nil
}

// CreateEmployeeAggregate 建立員工 aggregate 的服務流程。
func (c HRService) CreateEmployeeAggregate(ctx RequestContext, input CreateEmployeeInput) (Employee, error) {
	_, _, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, Action: ActionCreate},
		AuditTarget{Event: "hr.employee.create", Resource: string(ResourceEmployee)},
	)
	if err != nil {
		return Employee{}, err
	}
	var employee Employee
	provisionQueued := false
	if err := c.withTransaction(ctx, func(tx HRService) error {
		next, err := tx.employeeFromCreateInput(ctx, input)
		if err != nil {
			return err
		}
		accountPolicy, accountCreated, err := tx.applyEmployeeCreateAccountPolicy(ctx, &next, input)
		if err != nil {
			return err
		}
		if err := tx.store.UpsertEmployee(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.touchEmployeeAuthzIfNeeded(ctx, Employee{}, next, string(EventEmployeeAuthzSubjectCreate)); err != nil {
			return err
		}
		if err := tx.linkEmployeeAccount(ctx, next); err != nil {
			return err
		}
		if next.AccountID != "" && accountPolicy != string(EmployeeAccountPolicyNone) {
			sendInvite := accountPolicy == string(EmployeeAccountPolicyCreatePendingInvite)
			if err := tx.provisionEmployeeIdentityFromAccountID(ctx, next, next.AccountID, sendInvite); err != nil {
				return err
			}
			provisionQueued = true
		}
		eventPayload := map[string]any{"employee_id": next.ID, "account_policy": accountPolicy}
		if next.AccountID != "" {
			eventPayload["account_id"] = next.AccountID
		}
		if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeCreated), next.ID, eventPayload); err != nil {
			return err
		}
		if accountCreated && accountPolicy == string(EmployeeAccountPolicyCreatePendingInvite) {
			if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeInvited), next.ID, map[string]any{"employee_id": next.ID, "account_id": next.AccountID, "account_policy": accountPolicy}); err != nil {
				return err
			}
		}
		if err := tx.audit(ctx, "hr.employee.create", string(ResourceEmployee), next.ID, string(SeverityMedium), map[string]any{"name": next.Name, "account_id": next.AccountID, "account_policy": accountPolicy}); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		employee = next
		return nil
	}); err != nil {
		return Employee{}, err
	}
	if provisionQueued {
		c.runIdentityProvisioningFastPath(ctx)
	}
	c.logInfo(ctx, "employee created",
		"employee_id", employee.ID,
		"employee_no", employee.EmployeeNo,
		"status", employeeStatus(employee),
		"account_id", employee.AccountID,
	)
	return employee, nil
}

// UpdateEmployee 更新員工的服務流程。
func (c HRService) UpdateEmployee(ctx RequestContext, id string, input UpdateEmployeeInput) (Employee, error) {
	account, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: id, Action: ActionUpdate},
		AuditTarget{Event: "hr.employee.update", Resource: string(ResourceEmployee), Target: id},
	)
	if err != nil {
		return Employee{}, err
	}
	var employee Employee
	previousStatus := ""
	if err := c.withTransaction(ctx, func(tx HRService) error {
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
		if fields := forbiddenEmployeePatchFields(input, decision.FieldPolicies); len(fields) > 0 {
			return domainValidation("employee field policy denied update", fields...)
		}
		before := next
		previousStatus = employeeStatus(before)
		if err := tx.applyEmployeePatch(ctx, &next, input); err != nil {
			return err
		}
		next = tx.appendHistoryForChangedEmployment(before, next, "資料更新")
		if err := tx.store.UpsertEmployee(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.touchEmployeeAuthzIfNeeded(ctx, before, next, string(EventEmployeeAuthzSubjectUpdate)); err != nil {
			return err
		}
		if err := tx.linkEmployeeAccount(ctx, next); err != nil {
			return err
		}
		if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeUpdated), next.ID, map[string]any{"employee_id": next.ID}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.update", string(ResourceEmployee), next.ID, string(SeverityMedium), map[string]any{"changed": true}); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		employee = next
		return nil
	}); err != nil {
		return Employee{}, err
	}
	c.logInfo(ctx, "employee updated",
		"employee_id", employee.ID,
		"employee_no", employee.EmployeeNo,
		"previous_status", previousStatus,
		"status", employeeStatus(employee),
		"account_id", employee.AccountID,
	)
	return employee, nil
}

// EmployeeStats 處理員工 stats 的服務流程。
func (c HRService) EmployeeStats(ctx RequestContext, query EmployeeQuery) (EmployeeStats, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return EmployeeStats{}, err
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, Action: ActionRead})
	if err != nil {
		return EmployeeStats{}, err
	}
	if !decision.Allowed {
		return EmployeeStats{}, forbiddenAuthz(decision)
	}
	query = normalizeEmployeeQuery(EmployeeQuery{
		Keyword:          query.Keyword,
		DepartmentID:     query.DepartmentID,
		EmploymentStatus: query.EmploymentStatus,
		Category:         query.Category,
		Sort:             query.Sort,
	})
	scopedQuery, err := c.employeeQueryWithDecisionScope(ctx, account, query, decision)
	if err != nil {
		return EmployeeStats{}, err
	}
	items, err := c.listEmployeesForQuery(ctx, scopedQuery)
	if err != nil {
		return EmployeeStats{}, err
	}
	items, err = c.applyEmployeeDecision(ctx, account, items, decision)
	if err != nil {
		return EmployeeStats{}, err
	}
	now := c.Now()
	stats := EmployeeStats{Total: len(items)}
	for _, item := range items {
		status := employeeStatus(item)
		switch status {
		case string(EmployeeStatusActive):
			stats.Active++
		case string(EmployeeStatusOnboarding):
			stats.Onboarding++
		case string(EmployeeStatusProbation):
			stats.Probation++
		case string(EmployeeStatusLeaveSuspended):
			stats.LeaveSuspended++
		case string(EmployeeStatusResigned):
			stats.Resigned++
		}
		if item.HireDate != nil && sameMonth(*item.HireDate, now) {
			stats.HiredThisMonth++
		}
		if item.ResignDate != nil && sameMonth(*item.ResignDate, now) {
			stats.LeftThisMonth++
		}
	}
	return stats, nil
}

// EmployeeOptions 處理員工選項的服務流程。
func (c HRService) EmployeeOptions(ctx RequestContext) (EmployeeOptions, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return EmployeeOptions{}, err
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, Action: ActionRead})
	if err != nil {
		return EmployeeOptions{}, err
	}
	if !decision.Allowed {
		return EmployeeOptions{}, forbiddenAuthz(decision)
	}
	query, err := c.employeeQueryWithDecisionScope(ctx, account, EmployeeQuery{}, decision)
	if err != nil {
		return EmployeeOptions{}, err
	}
	employees, err := c.listEmployeesForQuery(ctx, query)
	if err != nil {
		return EmployeeOptions{}, err
	}
	employees, err = c.applyEmployeeDecision(ctx, account, employees, decision)
	if err != nil {
		return EmployeeOptions{}, err
	}
	units, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
	if err != nil {
		return EmployeeOptions{}, err
	}
	departments, err := c.employeeDepartmentOptions(ctx, account, decision, units, employees)
	if err != nil {
		return EmployeeOptions{}, err
	}
	return EmployeeOptions{
		Departments:        departments,
		Positions:          uniqueSorted(employeeStringValues(employees, func(v Employee) string { return v.Position })),
		EmploymentStatuses: EmployeeStatuses(false),
		Categories:         EmployeeCategories(),
		JobGrades:          []string{"P1", "P2", "P3", "M1", "M2", "M3"},
		JobLevels:          []string{"junior", "mid", "senior", "staff"},
	}, nil
}

// employeeDepartmentOptions 處理員工部門選項的服務流程。
func (c HRService) employeeDepartmentOptions(ctx RequestContext, account Account, decision CheckResult, units []OrgUnit, employees []Employee) ([]OrgUnit, error) {
	if decisionUsesOpenFGAScopeCheck(decision) {
		return employeeDepartmentOptionsFromEmployees(units, employees), nil
	}
	switch decision.Scope {
	case "", ScopeAll, ScopeTenant, ScopeSystem:
		return append([]OrgUnit(nil), units...), nil
	case ScopeDepartment, ScopeAssignedOrgUnits:
		return orgUnitOptionsByIDs(units, stringSliceFromAny(decision.Conditions["org_unit_ids"])), nil
	case ScopeDepartmentSubtree:
		orgIDs := stringSliceFromAny(decision.Conditions["org_unit_ids"])
		if len(orgIDs) == 0 && account.EmployeeID != "" {
			employee, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, account.EmployeeID)
			if err != nil {
				return nil, err
			}
			if ok && employee.OrgUnitID != "" {
				orgIDs = []string{employee.OrgUnitID}
			}
		}
		return orgUnitOptionsInSubtree(units, orgIDs), nil
	case ScopeCustomCondition:
		if orgIDs := stringSliceFromAny(decision.Conditions["org_unit_ids"]); len(orgIDs) > 0 {
			return orgUnitOptionsByIDs(units, orgIDs), nil
		}
		return employeeDepartmentOptionsFromEmployees(units, employees), nil
	default:
		return employeeDepartmentOptionsFromEmployees(units, employees), nil
	}
}

// filterOrgUnitsByDecision 處理篩選組織單位 by 決策的服務流程。
func (c HRService) filterOrgUnitsByDecision(ctx RequestContext, account Account, decision CheckResult, units []OrgUnit) ([]OrgUnit, error) {
	if decisionUsesOpenFGAScopeCheck(decision) {
		filtered, err := c.filterOrgUnitsByOpenFGAScope(ctx, account, decision, units)
		if err == nil {
			return filtered, nil
		}
		c.logWarn(ctx, "openfga org unit scope check failed; falling back to SQL-derived scope", "error", err)
	}
	switch decision.Scope {
	case "", ScopeAll, ScopeTenant, ScopeSystem:
		return append([]OrgUnit(nil), units...), nil
	case ScopeDepartment, ScopeAssignedOrgUnits:
		return orgUnitOptionsByIDs(units, stringSliceFromAny(decision.Conditions["org_unit_ids"])), nil
	case ScopeDepartmentSubtree:
		orgIDs := stringSliceFromAny(decision.Conditions["org_unit_ids"])
		if len(orgIDs) == 0 && account.EmployeeID != "" {
			employee, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, account.EmployeeID)
			if err != nil {
				return nil, err
			}
			if ok && employee.OrgUnitID != "" {
				orgIDs = []string{employee.OrgUnitID}
			}
		}
		return orgUnitOptionsInSubtree(units, orgIDs), nil
	case ScopeSelf, ScopeOwn:
		orgIDs, err := c.orgUnitIDsForEmployeeIDs(ctx, []string{account.EmployeeID})
		if err != nil {
			return nil, err
		}
		return orgUnitOptionsByIDs(units, orgIDs), nil
	case ScopeDirectReports:
		orgIDs, err := c.orgUnitIDsForEmployeeIDs(ctx, stringSliceFromAny(decision.Conditions["employee_ids"]))
		if err != nil {
			return nil, err
		}
		return orgUnitOptionsByIDs(units, orgIDs), nil
	case ScopeCustomCondition:
		if orgIDs := stringSliceFromAny(decision.Conditions["org_unit_ids"]); len(orgIDs) > 0 {
			return orgUnitOptionsByIDs(units, orgIDs), nil
		}
		orgIDs, err := c.orgUnitIDsForEmployeeIDs(ctx, stringSliceFromAny(decision.Conditions["employee_ids"]))
		if err != nil {
			return nil, err
		}
		return orgUnitOptionsByIDs(units, orgIDs), nil
	default:
		return []OrgUnit{}, nil
	}
}

// orgUnitIDsForEmployeeIDs 處理組織單位 IDs for 員工 IDs 的服務流程。
func (c HRService) orgUnitIDsForEmployeeIDs(ctx RequestContext, employeeIDs []string) ([]string, error) {
	allowedEmployees := stringSet(employeeIDs)
	if len(allowedEmployees) == 0 {
		return nil, nil
	}
	employees, err := c.store.ListEmployees(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	orgIDs := make([]string, 0, len(allowedEmployees))
	for _, employee := range employees {
		if _, ok := allowedEmployees[employee.ID]; ok && employee.OrgUnitID != "" {
			orgIDs = append(orgIDs, employee.OrgUnitID)
		}
	}
	return uniqueStrings(orgIDs), nil
}

// orgUnitOptionsByIDs 處理組織單位選項 by IDs。
func orgUnitOptionsByIDs(units []OrgUnit, ids []string) []OrgUnit {
	allowed := stringSet(ids)
	if len(allowed) == 0 {
		return []OrgUnit{}
	}
	out := make([]OrgUnit, 0, len(allowed))
	for _, unit := range units {
		if _, ok := allowed[unit.ID]; ok {
			out = append(out, unit)
		}
	}
	return out
}

// orgUnitOptionsInSubtree 處理組織單位選項 in subtree。
func orgUnitOptionsInSubtree(units []OrgUnit, roots []string) []OrgUnit {
	allowed := stringSet(roots)
	if len(allowed) == 0 {
		return []OrgUnit{}
	}
	out := make([]OrgUnit, 0)
	for _, unit := range units {
		if orgUnitInScope(units, unit.ID, allowed) {
			out = append(out, unit)
		}
	}
	return out
}

// employeeDepartmentOptionsFromEmployees 處理員工部門選項 來源 員工。
func employeeDepartmentOptionsFromEmployees(units []OrgUnit, employees []Employee) []OrgUnit {
	visible := map[string]struct{}{}
	for _, employee := range employees {
		if employee.OrgUnitID != "" {
			visible[employee.OrgUnitID] = struct{}{}
		}
	}
	out := make([]OrgUnit, 0, len(visible))
	for _, unit := range units {
		if _, ok := visible[unit.ID]; ok {
			out = append(out, unit)
		}
	}
	return out
}

const (
	defaultEmployeePage     = 1
	defaultEmployeePageSize = 20
	maxEmployeePageSize     = 100
)

// listEmployeesForQuery 列出員工 for 查詢的服務流程。
func (c HRService) listEmployeesForQuery(ctx RequestContext, query EmployeeQuery) ([]Employee, error) {
	return c.store.ListEmployeesByQuery(goContext(ctx), ctx.TenantID, query)
}

// employeeQueryWithDecisionScope 處理員工查詢 with 決策範圍的服務流程。
func (c HRService) employeeQueryWithDecisionScope(ctx RequestContext, account Account, query EmployeeQuery, decision CheckResult) (EmployeeQuery, error) {
	query = normalizeEmployeeQuery(query)
	scope, err := c.employeeScopeConstraint(ctx, account, decision)
	if err != nil {
		return EmployeeQuery{}, err
	}
	query.Scope = scope
	return query, nil
}

// employeeScopeConstraint 處理員工範圍 constraint 的服務流程。
func (c HRService) employeeScopeConstraint(ctx RequestContext, account Account, decision CheckResult) (domain.EmployeeScopeConstraint, error) {
	if decisionUsesOpenFGAScopeCheck(decision) {
		return domain.EmployeeScopeConstraint{}, nil
	}
	switch decision.Scope {
	case "", ScopeAll, ScopeTenant, ScopeSystem:
		return domain.EmployeeScopeConstraint{}, nil
	case ScopeSelf, ScopeOwn:
		ids := stringSliceFromAny(decision.Conditions["employee_ids"])
		if len(ids) == 0 && account.EmployeeID != "" {
			ids = []string{account.EmployeeID}
		}
		return employeeScopeByIDs(ids), nil
	case ScopeDepartment, ScopeAssignedOrgUnits:
		return employeeScopeByOrgUnits(stringSliceFromAny(decision.Conditions["org_unit_ids"])), nil
	case ScopeDepartmentSubtree:
		orgIDs := stringSliceFromAny(decision.Conditions["org_unit_ids"])
		if len(orgIDs) == 0 && account.EmployeeID != "" {
			employee, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, account.EmployeeID)
			if err != nil {
				return domain.EmployeeScopeConstraint{}, err
			}
			if ok && employee.OrgUnitID != "" {
				units, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
				if err != nil {
					return domain.EmployeeScopeConstraint{}, err
				}
				orgIDs = orgUnitIDsInSubtree(units, []string{employee.OrgUnitID})
			}
		}
		return employeeScopeByOrgUnits(orgIDs), nil
	case ScopeDirectReports:
		return employeeScopeByIDs(stringSliceFromAny(decision.Conditions["employee_ids"])), nil
	case ScopeCustomCondition:
		return employeeScopeFromConditions(decision.Conditions), nil
	default:
		return domain.EmployeeScopeConstraint{DenyAll: true}, nil
	}
}

// employeeScopeByIDs 處理員工範圍 by IDs。
func employeeScopeByIDs(ids []string) domain.EmployeeScopeConstraint {
	ids = uniqueStrings(ids)
	if len(ids) == 0 {
		return domain.EmployeeScopeConstraint{DenyAll: true}
	}
	return domain.EmployeeScopeConstraint{EmployeeIDs: ids}
}

// employeeScopeByOrgUnits 處理員工範圍 by 組織單位。
func employeeScopeByOrgUnits(ids []string) domain.EmployeeScopeConstraint {
	ids = uniqueStrings(ids)
	if len(ids) == 0 {
		return domain.EmployeeScopeConstraint{DenyAll: true}
	}
	return domain.EmployeeScopeConstraint{OrgUnitIDs: ids}
}

// employeeScopeFromConditions 處理員工範圍 來源 conditions。
func employeeScopeFromConditions(conditions map[string]any) domain.EmployeeScopeConstraint {
	scope := domain.EmployeeScopeConstraint{
		EmployeeIDs: uniqueStrings(stringSliceFromAny(conditions["employee_ids"])),
		OrgUnitIDs:  uniqueStrings(stringSliceFromAny(conditions["org_unit_ids"])),
		Statuses:    uniqueStrings(stringSliceFromAny(conditions["employee_statuses"])),
	}
	if len(scope.Statuses) == 0 {
		scope.Statuses = uniqueStrings(stringSliceFromAny(conditions["statuses"]))
	}
	if len(scope.EmployeeIDs) == 0 && len(scope.OrgUnitIDs) == 0 && len(scope.Statuses) == 0 {
		scope.DenyAll = true
	}
	return scope
}

// normalizeEmployeeQuery 正規化員工查詢。
func normalizeEmployeeQuery(query EmployeeQuery) EmployeeQuery {
	if query.Page <= 0 {
		query.Page = defaultEmployeePage
	}
	if query.PageSize <= 0 {
		query.PageSize = defaultEmployeePageSize
	}
	if query.PageSize > maxEmployeePageSize {
		query.PageSize = maxEmployeePageSize
	}
	if query.Sort == "" {
		query.Sort = "created_at_asc"
	}
	query.EmploymentStatus = normalizeEmployeeStatus(query.EmploymentStatus)
	query.Category = normalizeEmployeeCategory(query.Category)
	return query
}

// sortEmployees 排序員工。
func sortEmployees(items []Employee, sortKey string) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		switch sortKey {
		case "created_at_desc":
			if a.CreatedAt.Equal(b.CreatedAt) {
				return a.ID > b.ID
			}
			return a.CreatedAt.After(b.CreatedAt)
		case "hire_date_desc":
			return timeValue(a.HireDate).After(timeValue(b.HireDate))
		case "hire_date_asc":
			return timeValue(a.HireDate).Before(timeValue(b.HireDate))
		default:
			if a.CreatedAt.Equal(b.CreatedAt) {
				return a.ID < b.ID
			}
			return a.CreatedAt.Before(b.CreatedAt)
		}
	})
}

// employeeStatus 處理員工狀態。
func employeeStatus(item Employee) string {
	return utils.FirstNonEmpty(item.EmploymentStatus, item.Status)
}

// normalizeEmployeeStatus 正規化員工狀態。
func normalizeEmployeeStatus(value string) string {
	return NormalizeEmployeeStatus(value)
}

// normalizeEmployeeCategory 正規化員工分類。
func normalizeEmployeeCategory(value string) string {
	return NormalizeEmployeeCategory(value)
}

// normalizeEmployeeAccountPolicy 正規化員工帳號政策。
func normalizeEmployeeAccountPolicy(value string) string {
	return NormalizeEmployeeAccountPolicy(value)
}

// validEmployeeStatus 處理有效員工狀態。
func validEmployeeStatus(value string, includeDeleted bool) bool {
	status, ok := ParseEmployeeStatus(value)
	return ok && status.Valid(includeDeleted)
}

// validEmployeeCategory 處理有效員工分類。
func validEmployeeCategory(value string) bool {
	category, ok := ParseEmployeeCategory(value)
	return ok && category.Valid()
}

// validEmployeeAccountPolicy 處理有效員工帳號政策。
func validEmployeeAccountPolicy(value string) bool {
	_, ok := ParseEmployeeAccountPolicy(value)
	return ok
}

// employeeTerminalStatus 處理員工 terminal 狀態。
func employeeTerminalStatus(status string) bool {
	status = normalizeEmployeeStatus(status)
	return status == string(EmployeeStatusResigned) || status == string(EmployeeStatusDeleted)
}

// sameMonth 處理 same 月份。
func sameMonth(t time.Time, ref time.Time) bool {
	t = t.UTC()
	ref = ref.UTC()
	return t.Year() == ref.Year() && t.Month() == ref.Month()
}

// timeValue 處理時間 value。
func timeValue(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// formatDate 處理 format 日期。
func formatDate(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

// uniqueSorted 處理 unique sorted。
func uniqueSorted(values []string) []string {
	return uniqueStrings(values)
}

// employeeStringValues 處理員工字串 values。
func employeeStringValues(items []Employee, fn func(Employee) string) []string {
	out := make([]string, 0)
	for _, item := range items {
		if value := strings.TrimSpace(fn(item)); value != "" {
			out = append(out, value)
		}
	}
	return out
}

const employeeNoPrefix = "IKL"

const (
	employeeValidationFullForm      = "full_form"
	employeeValidationImportMinimal = "import_minimal"
)

type employeeUniqueLookupStore interface {
	GetEmployeeByEmployeeNo(context.Context, string, string) (Employee, bool, error)
	GetEmployeeByCompanyEmail(context.Context, string, string) (Employee, bool, error)
	GetEmployeeByPersonalEmail(context.Context, string, string) (Employee, bool, error)
	GetEmployeeByAccountID(context.Context, string, string) (Employee, bool, error)
	GetEmployeeByBasicInfoField(context.Context, string, string, string) (Employee, bool, error)
}

// employeeFromCreateInput 處理員工 來源 create 輸入的服務流程。
func (c HRService) employeeFromCreateInput(ctx RequestContext, input CreateEmployeeInput, reservedEmployeeNos ...map[string]struct{}) (Employee, error) {
	return c.employeeFromCreateInputWithProfile(ctx, input, employeeValidationFullForm, reservedEmployeeNos...)
}

// employeeFromImportInput 處理員工 來源 import 輸入的服務流程。
func (c HRService) employeeFromImportInput(ctx RequestContext, input CreateEmployeeInput, reservedEmployeeNos ...map[string]struct{}) (Employee, error) {
	return c.employeeFromCreateInputWithProfile(ctx, input, employeeValidationImportMinimal, reservedEmployeeNos...)
}

// employeeFromCreateInputWithProfile 處理員工 來源 create 輸入 with 資料檔的服務流程。
func (c HRService) employeeFromCreateInputWithProfile(ctx RequestContext, input CreateEmployeeInput, profile string, reservedEmployeeNos ...map[string]struct{}) (Employee, error) {
	employee, err := c.employeeCreateCandidate(ctx, input)
	if err != nil {
		return Employee{}, err
	}
	if employee.EmployeeNo == "" {
		employeeNo, err := c.generateEmployeeNo(ctx, reservedEmployeeNos...)
		if err != nil {
			return Employee{}, err
		}
		employee.EmployeeNo = employeeNo
	}
	if err := c.ensureEmployeePosition(ctx, &employee, true); err != nil {
		return Employee{}, err
	}
	if err := c.validateEmployee(ctx, employee, "create", profile); err != nil {
		return Employee{}, err
	}
	if err := reserveEmployeeNo(employee.EmployeeNo, reservedEmployeeNos...); err != nil {
		return Employee{}, err
	}
	if len(employee.InternalExperiences) == 0 {
		employee.InternalExperiences = append(employee.InternalExperiences, c.newEmployeeExperience(employee, "新進"))
	}
	return employee, nil
}

// employeeCreateCandidate 處理員工 create 候選的服務流程。
func (c HRService) employeeCreateCandidate(ctx RequestContext, input CreateEmployeeInput) (Employee, error) {
	now := c.Now()
	hireDate, err := optionalDateTime(input.HireDate)
	if err != nil {
		return Employee{}, BadRequest("hire_date must be RFC3339 or YYYY-MM-DD")
	}
	resignDate, err := optionalDateTime(input.ResignDate)
	if err != nil {
		return Employee{}, BadRequest("resign_date must be RFC3339 or YYYY-MM-DD")
	}
	status := normalizeEmployeeStatus(utils.FirstNonEmpty(input.EmploymentStatus, input.Status, string(EmployeeStatusActive)))
	employee := Employee{
		ID:                    utils.NewID("emp"),
		TenantID:              ctx.TenantID,
		EmployeeNo:            strings.TrimSpace(input.EmployeeNo),
		Name:                  strings.TrimSpace(input.Name),
		CompanyEmail:          strings.TrimSpace(input.CompanyEmail),
		PersonalEmail:         strings.TrimSpace(input.PersonalEmail),
		Phone:                 strings.TrimSpace(input.Phone),
		OrgUnitID:             strings.TrimSpace(input.OrgUnitID),
		AccountID:             strings.TrimSpace(input.AccountID),
		ManagerEmployeeID:     strings.TrimSpace(input.ManagerEmployeeID),
		PositionID:            strings.TrimSpace(input.PositionID),
		Position:              strings.TrimSpace(input.Position),
		Category:              normalizeEmployeeCategory(input.Category),
		Status:                status,
		EmploymentStatus:      status,
		HireDate:              hireDate,
		ResignDate:            resignDate,
		BasicInfo:             utils.CopyStringMap(input.BasicInfo),
		EmploymentInfo:        utils.CopyStringMap(input.EmploymentInfo),
		EducationMilitaryInfo: utils.CopyStringMap(input.EducationMilitaryInfo),
		ContactInfo:           utils.CopyStringMap(input.ContactInfo),
		InsuranceInfo:         utils.CopyStringMap(input.InsuranceInfo),
		InternalExperiences:   utils.CopyEmployeeExperiences(input.InternalExperiences),
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	return c.deriveEmployeeHotFields(employee), nil
}

// applyEmployeePatch 處理 apply 員工 patch 的服務流程。
func (c HRService) applyEmployeePatch(ctx RequestContext, employee *Employee, input UpdateEmployeeInput) error {
	return c.applyEmployeePatchWithPositionCreation(ctx, employee, input, true)
}

// applyEmployeePatchWithPositionCreation 處理 apply 員工 patch 的服務流程。
func (c HRService) applyEmployeePatchWithPositionCreation(ctx RequestContext, employee *Employee, input UpdateEmployeeInput, createMissingPosition bool) error {
	if input.Status != nil || input.EmploymentStatus != nil {
		return domainValidation("employee status must be changed through status-transition", FieldError{Tab: employeeTabEmploymentInfo, Field: "status", Code: "transition_required", Message: "status must be changed through status-transition"})
	}
	if input.EmployeeNo != nil {
		employee.EmployeeNo = strings.TrimSpace(*input.EmployeeNo)
	}
	if input.Name != nil {
		employee.Name = strings.TrimSpace(*input.Name)
	}
	if input.CompanyEmail != nil {
		employee.CompanyEmail = strings.TrimSpace(*input.CompanyEmail)
	}
	if input.PersonalEmail != nil {
		employee.PersonalEmail = strings.TrimSpace(*input.PersonalEmail)
	}
	if input.Phone != nil {
		employee.Phone = strings.TrimSpace(*input.Phone)
	}
	if input.OrgUnitID != nil {
		employee.OrgUnitID = strings.TrimSpace(*input.OrgUnitID)
	}
	if input.AccountID != nil {
		employee.AccountID = strings.TrimSpace(*input.AccountID)
	}
	if input.ManagerEmployeeID != nil {
		employee.ManagerEmployeeID = strings.TrimSpace(*input.ManagerEmployeeID)
	}
	if input.PositionID != nil {
		employee.PositionID = strings.TrimSpace(*input.PositionID)
	}
	if input.Position != nil {
		employee.Position = strings.TrimSpace(*input.Position)
	}
	if input.Category != nil {
		employee.Category = normalizeEmployeeCategory(*input.Category)
	}
	if input.HireDate != nil {
		t, err := optionalDateTime(*input.HireDate)
		if err != nil {
			return BadRequest("hire_date must be RFC3339 or YYYY-MM-DD")
		}
		employee.HireDate = t
	}
	if input.ResignDate != nil {
		t, err := optionalDateTime(*input.ResignDate)
		if err != nil {
			return BadRequest("resign_date must be RFC3339 or YYYY-MM-DD")
		}
		employee.ResignDate = t
	}
	employee.BasicInfo = mergeMap(employee.BasicInfo, input.BasicInfo)
	employee.EmploymentInfo = mergeMap(employee.EmploymentInfo, input.EmploymentInfo)
	if input.Position != nil && input.PositionID == nil {
		employee.PositionID = ""
		if employee.EmploymentInfo == nil {
			employee.EmploymentInfo = map[string]any{}
		}
		delete(employee.EmploymentInfo, "position_id")
		employee.EmploymentInfo["position"] = employee.Position
	}
	employee.EducationMilitaryInfo = mergeMap(employee.EducationMilitaryInfo, input.EducationMilitaryInfo)
	employee.ContactInfo = mergeMap(employee.ContactInfo, input.ContactInfo)
	employee.InsuranceInfo = mergeMap(employee.InsuranceInfo, input.InsuranceInfo)
	if input.InternalExperiences != nil {
		employee.InternalExperiences = utils.CopyEmployeeExperiences(input.InternalExperiences)
	}
	*employee = c.deriveEmployeeHotFields(*employee)
	if err := c.ensureEmployeePosition(ctx, employee, createMissingPosition); err != nil {
		return err
	}
	employee.UpdatedAt = c.Now()
	return c.validateEmployee(ctx, *employee, "update", employeeValidationFullForm)
}

// forbiddenEmployeePatchFields 處理禁止員工 patch 欄位。
func forbiddenEmployeePatchFields(input UpdateEmployeeInput, policies map[string]string) []FieldError {
	if len(policies) == 0 {
		return nil
	}
	fields := make([]FieldError, 0)
	add := func(tab, field string) {
		effect := policies[field]
		if effect != "deny" && effect != "hide" && effect != "readonly" {
			return
		}
		fields = append(fields, FieldError{Tab: tab, Field: field, Code: "field_denied", Message: field + " cannot be updated by current permission policy"})
	}
	for _, field := range employeeScalarPatchPolicyFields {
		if field.present(input) {
			add(field.tab, field.field)
		}
	}
	for _, field := range employeeMapPatchPolicyFields {
		addPatchMapFields(&fields, policies, field.tab, field.values(input))
	}
	return fields
}

// addPatchMapFields 處理 add patch map 欄位。
func addPatchMapFields(fields *[]FieldError, policies map[string]string, tab string, values map[string]any) {
	if len(values) == 0 {
		return
	}
	if effect := policies[tab]; effect == "deny" || effect == "hide" || effect == "readonly" {
		*fields = append(*fields, FieldError{Tab: tab, Field: tab, Code: "field_denied", Message: tab + " cannot be updated by current permission policy"})
	}
	for field := range values {
		effect := policies[field]
		if effect != "deny" && effect != "hide" && effect != "readonly" {
			continue
		}
		*fields = append(*fields, FieldError{Tab: tab, Field: field, Code: "field_denied", Message: field + " cannot be updated by current permission policy"})
	}
}

// deriveEmployeeHotFields 推導員工 hot 欄位的服務流程。
func (c HRService) deriveEmployeeHotFields(employee Employee) Employee {
	employee.CompanyEmail = utils.FirstNonEmpty(employee.CompanyEmail, employeeHotValue(employee, "company_email"))
	employee.PersonalEmail = utils.FirstNonEmpty(employee.PersonalEmail, employeeHotValue(employee, "personal_email"))
	employee.Phone = utils.FirstNonEmpty(employee.Phone, employeeHotValue(employee, "phone"))
	employee.OrgUnitID = utils.FirstNonEmpty(employee.OrgUnitID, employeeHotValue(employee, "org_unit_id"))
	employee.ManagerEmployeeID = utils.FirstNonEmpty(employee.ManagerEmployeeID, employeeHotValue(employee, "manager_employee_id"))
	employee.PositionID = utils.FirstNonEmpty(employee.PositionID, employeeHotValue(employee, "position_id"))
	employee.Position = utils.FirstNonEmpty(employee.Position, employeeHotValue(employee, "position"))
	employee.Category = normalizeEmployeeCategory(utils.FirstNonEmpty(employee.Category, employeeHotValue(employee, "category")))
	employee.Name = utils.FirstNonEmpty(employee.Name, employeeHotValue(employee, "name"), strings.TrimSpace(stringFromMap(employee.BasicInfo, "first_name")+" "+stringFromMap(employee.BasicInfo, "last_name")))
	if employee.EmploymentStatus == "" {
		employee.EmploymentStatus = employee.Status
	}
	if employee.Status == "" {
		employee.Status = employee.EmploymentStatus
	}
	employee.EmploymentStatus = normalizeEmployeeStatus(employee.EmploymentStatus)
	employee.Status = normalizeEmployeeStatus(employee.Status)
	return employee
}

// validateEmployee 驗證員工的服務流程。
func (c HRService) validateEmployee(ctx RequestContext, employee Employee, mode string, profile ...string) error {
	validationProfile := employeeValidationFullForm
	if len(profile) > 0 && strings.TrimSpace(profile[0]) != "" {
		validationProfile = strings.TrimSpace(profile[0])
	}
	fields := make([]FieldError, 0)
	if strings.TrimSpace(employee.Name) == "" {
		fields = append(fields, FieldError{Tab: "basic_info", Field: "name", Code: "required", Message: "name is required"})
	}
	if strings.TrimSpace(employee.CompanyEmail) == "" {
		fields = append(fields, FieldError{Tab: "basic_info", Field: "company_email", Code: "required", Message: "company_email is required"})
	}
	if employee.Category != "" && !validEmployeeCategory(employee.Category) {
		fields = append(fields, FieldError{Tab: "employment_info", Field: "category", Code: "invalid", Message: "category must be one of full_time, part_time, intern, contractor, other"})
	}
	if !validEmployeeStatus(employeeStatus(employee), true) {
		fields = append(fields, FieldError{Tab: "employment_info", Field: "employment_status", Code: "invalid", Message: "employment_status must be one of active, probation, leave_suspended, onboarding, resigned, deleted"})
	}
	if strings.TrimSpace(employee.OrgUnitID) != "" {
		if _, ok, err := c.store.GetOrgUnit(goContext(ctx), ctx.TenantID, employee.OrgUnitID); err != nil {
			return err
		} else if !ok {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "org_unit_id", Code: "not_found", Message: "org unit not found"})
		}
	}
	if strings.TrimSpace(employee.AccountID) != "" {
		account, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, employee.AccountID)
		if err != nil {
			return err
		}
		if !ok {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "account_id", Code: "not_found", Message: "account not found"})
		} else {
			if account.EmployeeID != "" && account.EmployeeID != employee.ID {
				fields = append(fields, FieldError{Tab: "employment_info", Field: "account_id", Code: "unique", Message: "account_id already linked"})
			}
			if account.Status == string(AccountStatusDisabled) && !employeeTerminalStatus(employeeStatus(employee)) {
				fields = append(fields, FieldError{Tab: "employment_info", Field: "account_id", Code: "invalid", Message: "disabled account cannot be linked to a non-terminal employee"})
			}
		}
	}
	if strings.TrimSpace(employee.ManagerEmployeeID) != "" {
		if employee.ManagerEmployeeID == employee.ID {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "manager_employee_id", Code: "invalid", Message: "manager cannot be the employee itself"})
		} else if _, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, employee.ManagerEmployeeID); err != nil {
			return err
		} else if !ok {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "manager_employee_id", Code: "not_found", Message: "manager employee not found"})
		}
	}
	if identityType := stringFromMap(employee.BasicInfo, "nationality_type"); identityType == "foreign" {
		for _, field := range []string{"passport_no", "passport_name", "entry_date", "arc_no", "arc_expiry_date", "tax_id", "work_permit_no", "work_permit_expiry_date", "contract_expiry_date", "broker"} {
			if stringFromMap(employee.BasicInfo, field) == "" {
				fields = append(fields, FieldError{Tab: "basic_info", Field: field, Code: "required", Message: field + " is required for foreign employees"})
			}
		}
	} else if idNo := stringFromMap(employee.BasicInfo, "national_id"); idNo == "" && stringFromMap(employee.BasicInfo, "nationality_type") != "" {
		fields = append(fields, FieldError{Tab: "basic_info", Field: "national_id", Code: "required", Message: "national_id is required for local employees"})
	}
	status := employeeStatus(employee)
	if status == string(EmployeeStatusResigned) {
		if employee.ResignDate == nil && stringFromMap(employee.EmploymentInfo, "resign_date") == "" {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "resign_date", Code: "required", Message: "resign_date is required"})
		}
		if stringFromMap(employee.EmploymentInfo, "resign_reason") == "" && mode == "transition" {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "resign_reason", Code: "required", Message: "resign_reason is required"})
		}
	}
	if validationProfile == employeeValidationFullForm {
		fields = append(fields, fullFormEmployeeFieldErrors(employee)...)
	}
	if len(fields) == 0 {
		if uniqueLookup, ok := c.store.(employeeUniqueLookupStore); ok {
			uniqueFields, err := c.employeeUniqueFieldErrors(ctx, uniqueLookup, employee)
			if err != nil {
				return err
			}
			fields = append(fields, uniqueFields...)
		} else if uniqueFields, err := c.employeeUniqueFieldErrorsFromList(ctx, employee); err != nil {
			return err
		} else {
			fields = append(fields, uniqueFields...)
		}
	}
	if len(fields) > 0 {
		return domainValidation("employee validation failed", fields...)
	}
	return nil
}

// fullFormEmployeeFieldErrors 處理 full 表單員工欄位錯誤。
func fullFormEmployeeFieldErrors(employee Employee) []FieldError {
	fields := make([]FieldError, 0)
	addRequired := func(tab, field, message string) {
		fields = append(fields, FieldError{Tab: tab, Field: field, Code: "required", Message: message})
	}
	if strings.TrimSpace(employee.OrgUnitID) == "" {
		addRequired(employeeTabEmploymentInfo, "org_unit_id", "org_unit_id is required")
	}
	if strings.TrimSpace(employee.Position) == "" {
		addRequired(employeeTabEmploymentInfo, "position", "position is required")
	}
	if strings.TrimSpace(employee.Category) == "" {
		addRequired(employeeTabEmploymentInfo, "category", "category is required")
	}
	if strings.TrimSpace(employeeStatus(employee)) == "" {
		addRequired(employeeTabEmploymentInfo, "employment_status", "employment_status is required")
	}
	if employee.HireDate == nil && stringFromMap(employee.EmploymentInfo, "hire_date") == "" {
		addRequired(employeeTabEmploymentInfo, "hire_date", "hire_date is required")
	}
	if identityType := stringFromMap(employee.BasicInfo, "nationality_type"); identityType == "" {
		addRequired(employeeTabBasicInfo, "nationality_type", "nationality_type is required")
	}
	requireAny(&fields, employeeTabEducationMilitaryInfo, employee.EducationMilitaryInfo, "highest_education", "highest_education is required", "highest_education", "education_level", "degree")
	requireAny(&fields, employeeTabEducationMilitaryInfo, employee.EducationMilitaryInfo, "school", "school is required", "school", "school_name")
	requireAny(&fields, employeeTabContactInfo, employee.ContactInfo, "mobile_phone", "mobile_phone is required", "mobile_phone", "phone")
	requireAny(&fields, employeeTabContactInfo, employee.ContactInfo, "address", "address is required", "address", "communication_address")
	requireAny(&fields, employeeTabContactInfo, employee.ContactInfo, "emergency_contact_relation", "emergency_contact_relation is required", "emergency_contact_relation", "emergency_relation")
	requireAny(&fields, employeeTabContactInfo, employee.ContactInfo, "emergency_contact_name", "emergency_contact_name is required", "emergency_contact_name", "emergency_name")
	requireAny(&fields, employeeTabContactInfo, employee.ContactInfo, "emergency_contact_phone", "emergency_contact_phone is required", "emergency_contact_phone", "emergency_phone")
	requireAny(&fields, employeeTabInsuranceInfo, employee.InsuranceInfo, "labor_insurance_date", "labor_insurance_date is required", "labor_insurance_date")
	requireAny(&fields, employeeTabInsuranceInfo, employee.InsuranceInfo, "labor_insurance_level", "labor_insurance_level is required", "labor_insurance_level")
	requirePositiveNumber(&fields, employee.InsuranceInfo, "labor_insurance_salary", "labor_insurance_salary must be positive")
	requireAny(&fields, employeeTabInsuranceInfo, employee.InsuranceInfo, "health_insurance_date", "health_insurance_date is required", "health_insurance_date")
	requireAny(&fields, employeeTabInsuranceInfo, employee.InsuranceInfo, "health_insurance_level", "health_insurance_level is required", "health_insurance_level")
	requirePositiveNumber(&fields, employee.InsuranceInfo, "health_insurance_amount", "health_insurance_amount must be positive")
	return fields
}

// requireAny 處理 require any。
func requireAny(fields *[]FieldError, tab string, values map[string]any, field, message string, keys ...string) {
	for _, key := range keys {
		if mapAnyString(values, key) != "" {
			return
		}
	}
	*fields = append(*fields, FieldError{Tab: tab, Field: field, Code: "required", Message: message})
}

// requirePositiveNumber 處理 require 正數數字。
func requirePositiveNumber(fields *[]FieldError, values map[string]any, field, message string) {
	raw := mapAnyString(values, field)
	if raw == "" {
		*fields = append(*fields, FieldError{Tab: employeeTabInsuranceInfo, Field: field, Code: "required", Message: message})
		return
	}
	number, err := strconv.ParseFloat(raw, 64)
	if err != nil || number <= 0 {
		*fields = append(*fields, FieldError{Tab: employeeTabInsuranceInfo, Field: field, Code: "invalid", Message: message})
	}
}

// employeeUniqueFieldErrors 處理員工 unique 欄位錯誤的服務流程。
func (c HRService) employeeUniqueFieldErrors(ctx RequestContext, store employeeUniqueLookupStore, employee Employee) ([]FieldError, error) {
	fields := make([]FieldError, 0, 8)
	goCtx := goContext(ctx)
	if employee.EmployeeNo != "" {
		existing, ok, err := store.GetEmployeeByEmployeeNo(goCtx, ctx.TenantID, employee.EmployeeNo)
		if err != nil {
			return nil, err
		}
		if ok && existing.ID != employee.ID {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "employee_no", Code: "unique", Message: "employee_no already exists"})
		}
	}
	if employee.CompanyEmail != "" {
		existing, ok, err := store.GetEmployeeByCompanyEmail(goCtx, ctx.TenantID, employee.CompanyEmail)
		if err != nil {
			return nil, err
		}
		if ok && existing.ID != employee.ID {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "company_email", Code: "unique", Message: "company_email already exists"})
		}
	}
	if employee.PersonalEmail != "" {
		existing, ok, err := store.GetEmployeeByPersonalEmail(goCtx, ctx.TenantID, employee.PersonalEmail)
		if err != nil {
			return nil, err
		}
		if ok && existing.ID != employee.ID {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "personal_email", Code: "unique", Message: "personal_email already exists"})
		}
	}
	if employee.AccountID != "" {
		existing, ok, err := store.GetEmployeeByAccountID(goCtx, ctx.TenantID, employee.AccountID)
		if err != nil {
			return nil, err
		}
		if ok && existing.ID != employee.ID {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "account_id", Code: "unique", Message: "account_id already linked"})
		}
	}
	for _, field := range employeeUniqueBasicInfoFields {
		value := stringFromMap(employee.BasicInfo, field)
		if value == "" {
			continue
		}
		existing, ok, err := store.GetEmployeeByBasicInfoField(goCtx, ctx.TenantID, field, value)
		if err != nil {
			return nil, err
		}
		if ok && existing.ID != employee.ID {
			fields = append(fields, FieldError{Tab: "basic_info", Field: field, Code: "unique", Message: field + " already exists"})
		}
	}
	return fields, nil
}

// employeeUniqueFieldErrorsFromList 處理員工 unique 欄位錯誤 來源 列表的服務流程。
func (c HRService) employeeUniqueFieldErrorsFromList(ctx RequestContext, employee Employee) ([]FieldError, error) {
	existingEmployees, err := c.store.ListEmployees(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	fields := make([]FieldError, 0, 8)
	for _, existing := range existingEmployees {
		if existing.ID == employee.ID {
			continue
		}
		if employee.EmployeeNo != "" && existing.EmployeeNo == employee.EmployeeNo {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "employee_no", Code: "unique", Message: "employee_no already exists"})
		}
		if employee.CompanyEmail != "" && strings.EqualFold(existing.CompanyEmail, employee.CompanyEmail) {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "company_email", Code: "unique", Message: "company_email already exists"})
		}
		if employee.PersonalEmail != "" && strings.EqualFold(existing.PersonalEmail, employee.PersonalEmail) {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "personal_email", Code: "unique", Message: "personal_email already exists"})
		}
		if employee.AccountID != "" && existing.AccountID == employee.AccountID {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "account_id", Code: "unique", Message: "account_id already linked"})
		}
		for _, field := range employeeUniqueBasicInfoFields {
			value := stringFromMap(employee.BasicInfo, field)
			if value == "" {
				continue
			}
			if strings.EqualFold(stringFromMap(existing.BasicInfo, field), value) {
				fields = append(fields, FieldError{Tab: "basic_info", Field: field, Code: "unique", Message: field + " already exists"})
			}
		}
	}
	return fields, nil
}

var employeeUniqueBasicInfoFields = []string{
	"national_id",
	"passport_no",
	"arc_no",
	"tax_id",
	"work_permit_no",
}

// generateEmployeeNo 處理 generate 員工 no 的服務流程。
func (c HRService) generateEmployeeNo(ctx RequestContext, reservedEmployeeNos ...map[string]struct{}) (string, error) {
	for {
		employeeNo, err := c.store.NextEmployeeNo(goContext(ctx), ctx.TenantID, employeeNoPrefix)
		if err != nil {
			return "", err
		}
		if !employeeNoReserved(employeeNo, reservedEmployeeNos...) {
			return employeeNo, nil
		}
	}
}

// employeeNoReserved 處理員工 no reserved。
func employeeNoReserved(employeeNo string, reservedEmployeeNos ...map[string]struct{}) bool {
	employeeNo = strings.TrimSpace(employeeNo)
	if employeeNo == "" {
		return false
	}
	for _, reserved := range reservedEmployeeNos {
		if reserved == nil {
			continue
		}
		if _, ok := reserved[employeeNo]; ok {
			return true
		}
	}
	return false
}

// reserveEmployeeNo 保留員工 no。
func reserveEmployeeNo(employeeNo string, reservedEmployeeNos ...map[string]struct{}) error {
	employeeNo = strings.TrimSpace(employeeNo)
	if employeeNo == "" {
		return nil
	}
	for _, reserved := range reservedEmployeeNos {
		if reserved == nil {
			continue
		}
		if _, ok := reserved[employeeNo]; ok {
			return domainValidation("employee validation failed", FieldError{Tab: "basic_info", Field: "employee_no", Code: "duplicate_in_import", Message: "employee_no is duplicated in import batch"})
		}
		reserved[employeeNo] = struct{}{}
	}
	return nil
}

// stringFromMap 處理字串 來源 map。
func stringFromMap(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	if v, ok := values[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// mapAnyString 映射any 字串。
func mapAnyString(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	switch v := values[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	default:
		return ""
	}
}

// mergeMap 合併 map。
func mergeMap(base map[string]any, patch map[string]any) map[string]any {
	if len(base) == 0 && len(patch) == 0 {
		return nil
	}
	out := utils.CopyStringMap(base)
	if out == nil {
		out = map[string]any{}
	}
	for key, value := range patch {
		out[key] = value
	}
	return out
}

// newEmployeeExperience 建立員工經歷的服務流程。
func (c HRService) newEmployeeExperience(employee Employee, reason string) EmployeeExperience {
	return EmployeeExperience{
		ID:                utils.NewID("ehist"),
		StartDate:         employee.HireDate,
		Reason:            utils.FirstNonEmpty(reason, "資料更新"),
		OrgUnitID:         employee.OrgUnitID,
		ManagerEmployeeID: employee.ManagerEmployeeID,
		Position:          employee.Position,
		Category:          employee.Category,
		Status:            employeeStatus(employee),
		Current:           true,
		CreatedAt:         c.Now(),
	}
}

// appendHistoryForChangedEmployment 附加歷史 for changed 任職的服務流程。
func (c HRService) appendHistoryForChangedEmployment(before, after Employee, reason string) Employee {
	if before.OrgUnitID == after.OrgUnitID && before.ManagerEmployeeID == after.ManagerEmployeeID && before.Position == after.Position && before.Category == after.Category && before.Status == after.Status && before.EmploymentStatus == after.EmploymentStatus {
		return after
	}
	for i := range after.InternalExperiences {
		after.InternalExperiences[i].Current = false
		if after.InternalExperiences[i].EndDate == nil {
			t := c.Now()
			after.InternalExperiences[i].EndDate = &t
		}
	}
	after.InternalExperiences = append(after.InternalExperiences, c.newEmployeeExperience(after, reason))
	return after
}

// touchEmployeeAuthzIfNeeded 處理 touch 員工授權 if needed 的服務流程。
func (c HRService) touchEmployeeAuthzIfNeeded(ctx RequestContext, before, after Employee, eventType string) error {
	if before.OrgUnitID == after.OrgUnitID && before.AccountID == after.AccountID && before.ManagerEmployeeID == after.ManagerEmployeeID {
		return nil
	}
	if err := c.syncEmployeeRelationshipTuples(ctx, before, after); err != nil {
		return err
	}
	return c.touchAuthzConfig(ctx, eventType, map[string]any{
		"employee_id":                after.ID,
		"before_org_unit_id":         before.OrgUnitID,
		"after_org_unit_id":          after.OrgUnitID,
		"before_account_id":          before.AccountID,
		"after_account_id":           after.AccountID,
		"before_manager_employee_id": before.ManagerEmployeeID,
		"after_manager_employee_id":  after.ManagerEmployeeID,
	})
}

// linkEmployeeAccount 處理 link 員工帳號的服務流程。
func (c HRService) linkEmployeeAccount(ctx RequestContext, employee Employee) error {
	if employee.AccountID == "" {
		return nil
	}
	account, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, employee.AccountID)
	if err != nil {
		return err
	}
	if ok {
		before := account
		account.EmployeeID = employee.ID
		account.DisplayName = utils.FirstNonEmpty(account.DisplayName, employee.Name)
		account.Email = utils.FirstNonEmpty(account.Email, employee.CompanyEmail)
		if err := c.store.UpsertAccount(goContext(ctx), account); err != nil {
			return err
		}
		return c.Service.syncAccountTenantMembershipTuple(ctx, before, account)
	}
	return nil
}

// applyEmployeeCreateAccountPolicy 處理 apply 員工 create 帳號政策的服務流程。
func (c HRService) applyEmployeeCreateAccountPolicy(ctx RequestContext, employee *Employee, input CreateEmployeeInput) (string, bool, error) {
	policy, err := employeeCreateAccountPolicy(input)
	if err != nil {
		return "", false, err
	}
	switch policy {
	case string(EmployeeAccountPolicyNone):
		return policy, false, nil
	case string(EmployeeAccountPolicyLinkExisting):
		if strings.TrimSpace(employee.AccountID) == "" {
			return "", false, domainValidation("employee account policy validation failed", FieldError{Tab: employeeTabEmploymentInfo, Field: "account_id", Code: "required", Message: "account_id is required when account_policy is link_existing"})
		}
		return policy, false, c.ensureEmployeeLinkedAccount(ctx, *employee)
	case string(EmployeeAccountPolicyCreatePendingInvite), string(EmployeeAccountPolicyCreateActive):
		if strings.TrimSpace(input.AccountID) != "" {
			return "", false, domainValidation("employee account policy validation failed", FieldError{Tab: employeeTabEmploymentInfo, Field: "account_id", Code: "invalid", Message: "account_id is only allowed when account_policy is link_existing"})
		}
		if employeeTerminalStatus(employeeStatus(*employee)) {
			return "", false, domainValidation("employee account policy validation failed", FieldError{Tab: employeeTabEmploymentInfo, Field: "account_policy", Code: "invalid", Message: "terminal employee cannot create a login account"})
		}
		email := strings.TrimSpace(employee.CompanyEmail)
		if email == "" {
			return "", false, domainValidation("employee account policy validation failed", FieldError{Tab: employeeTabBasicInfo, Field: "company_email", Code: "required", Message: "company_email is required to create an account"})
		}
		if err := c.ensureAccountEmailAvailable(ctx, email); err != nil {
			return "", false, err
		}
		accountStatus := string(AccountStatusPendingInvite)
		if policy == string(EmployeeAccountPolicyCreateActive) {
			accountStatus = string(AccountStatusActive)
		}
		account := Account{
			ID:          utils.NewID("acct"),
			TenantID:    ctx.TenantID,
			DisplayName: employee.Name,
			Email:       email,
			EmployeeID:  employee.ID,
			Status:      accountStatus,
			CreatedAt:   c.Now(),
		}
		if err := c.store.UpsertAccount(goContext(ctx), account); err != nil {
			return "", false, err
		}
		if err := c.Service.syncAccountTenantMembershipTuple(ctx, Account{}, account); err != nil {
			return "", false, err
		}
		employee.AccountID = account.ID
		return policy, true, nil
	default:
		return "", false, BadRequest("invalid account_policy")
	}
}

// employeeCreateAccountPolicy 處理員工 create 帳號政策。
func employeeCreateAccountPolicy(input CreateEmployeeInput) (string, error) {
	rawPolicy := strings.TrimSpace(input.AccountPolicy)
	if rawPolicy == "" {
		if strings.TrimSpace(input.AccountID) != "" {
			return string(EmployeeAccountPolicyLinkExisting), nil
		}
		return string(EmployeeAccountPolicyNone), nil
	}
	policy := normalizeEmployeeAccountPolicy(rawPolicy)
	if !validEmployeeAccountPolicy(policy) {
		return "", BadRequest("invalid account_policy")
	}
	return policy, nil
}

// ensureEmployeeLinkedAccount 確保員工 linked 帳號的服務流程。
func (c HRService) ensureEmployeeLinkedAccount(ctx RequestContext, employee Employee) error {
	account, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, employee.AccountID)
	if err != nil {
		return err
	}
	if !ok {
		return domainValidation("employee account validation failed", FieldError{Tab: employeeTabEmploymentInfo, Field: "account_id", Code: "not_found", Message: "account not found"})
	}
	if account.EmployeeID != "" && account.EmployeeID != employee.ID {
		return domainValidation("employee account validation failed", FieldError{Tab: employeeTabEmploymentInfo, Field: "account_id", Code: "unique", Message: "account_id already linked"})
	}
	if account.Status == string(AccountStatusDisabled) && !employeeTerminalStatus(employeeStatus(employee)) {
		return domainValidation("employee account validation failed", FieldError{Tab: employeeTabEmploymentInfo, Field: "account_id", Code: "invalid", Message: "disabled account cannot be linked to a non-terminal employee"})
	}
	return nil
}

// ensureAccountEmailAvailable 確保帳號 email available 的服務流程。
func (c HRService) ensureAccountEmailAvailable(ctx RequestContext, email string) error {
	return c.ensureAccountEmailAvailableForAccount(ctx, email, "")
}

// ensureAccountEmailAvailableForAccount 確保帳號 email available for 帳號的服務流程。
func (c HRService) ensureAccountEmailAvailableForAccount(ctx RequestContext, email, allowedAccountID string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}
	accounts, err := c.store.ListAccounts(goContext(ctx), ctx.TenantID)
	if err != nil {
		return err
	}
	for _, account := range accounts {
		if strings.EqualFold(strings.TrimSpace(account.Email), email) && account.ID != strings.TrimSpace(allowedAccountID) {
			return domainValidation("employee account policy validation failed", FieldError{Tab: employeeTabBasicInfo, Field: "company_email", Code: "unique", Message: "account email already exists"})
		}
	}
	return nil
}

// appendEmployeeEvent 附加員工事件的服務流程。
func (c HRService) appendEmployeeEvent(ctx RequestContext, eventType, target string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["target"] = target
	aggregateType := string(ResourceEmployee)
	if eventType == string(EventEmployeeImported) {
		aggregateType = string(ResourceEmployeeImport)
	}
	return c.store.AppendOutboxEvent(goContext(ctx), OutboxEvent{
		ID:            utils.NewID("outbox"),
		TenantID:      ctx.TenantID,
		EventType:     eventType,
		AggregateType: aggregateType,
		AggregateID:   target,
		Payload:       payload,
		Status:        "pending",
		RetryCount:    0,
		CreatedAt:     c.Now(),
	})
}

// domainValidation 處理 domain 驗證。
func domainValidation(message string, fields ...FieldError) error {
	return ValidationFailed(message, fields)
}

type employeeFieldSource struct {
	tab string
	key string
}

const (
	employeeTabBasicInfo             = "basic_info"
	employeeTabEmploymentInfo        = "employment_info"
	employeeTabEducationMilitaryInfo = "education_military_info"
	employeeTabContactInfo           = "contact_info"
	employeeTabInsuranceInfo         = "insurance_info"
)

var employeeHotFieldSources = map[string][]employeeFieldSource{
	"company_email": {
		{tab: employeeTabBasicInfo, key: "company_email"},
		{tab: employeeTabBasicInfo, key: "email"},
	},
	"personal_email": {
		{tab: employeeTabBasicInfo, key: "personal_email"},
	},
	"phone": {
		{tab: employeeTabContactInfo, key: "mobile_phone"},
		{tab: employeeTabContactInfo, key: "phone"},
	},
	"org_unit_id": {
		{tab: employeeTabEmploymentInfo, key: "org_unit_id"},
		{tab: employeeTabEmploymentInfo, key: "department_id"},
	},
	"manager_employee_id": {
		{tab: employeeTabEmploymentInfo, key: "manager_employee_id"},
		{tab: employeeTabBasicInfo, key: "manager_employee_id"},
	},
	"position_id": {
		{tab: employeeTabEmploymentInfo, key: "position_id"},
	},
	"position": {
		{tab: employeeTabEmploymentInfo, key: "position"},
		{tab: employeeTabEmploymentInfo, key: "job_title"},
	},
	"category": {
		{tab: employeeTabEmploymentInfo, key: "category"},
	},
	"name": {
		{tab: employeeTabBasicInfo, key: "name"},
	},
}

type employeePatchPolicyField struct {
	tab     string
	field   string
	present func(UpdateEmployeeInput) bool
}

var employeeScalarPatchPolicyFields = []employeePatchPolicyField{
	{tab: employeeTabBasicInfo, field: "employee_no", present: func(input UpdateEmployeeInput) bool { return input.EmployeeNo != nil }},
	{tab: employeeTabBasicInfo, field: "name", present: func(input UpdateEmployeeInput) bool { return input.Name != nil }},
	{tab: employeeTabBasicInfo, field: "company_email", present: func(input UpdateEmployeeInput) bool { return input.CompanyEmail != nil }},
	{tab: employeeTabBasicInfo, field: "personal_email", present: func(input UpdateEmployeeInput) bool { return input.PersonalEmail != nil }},
	{tab: employeeTabContactInfo, field: "phone", present: func(input UpdateEmployeeInput) bool { return input.Phone != nil }},
	{tab: employeeTabEmploymentInfo, field: "org_unit_id", present: func(input UpdateEmployeeInput) bool { return input.OrgUnitID != nil }},
	{tab: employeeTabBasicInfo, field: "account_id", present: func(input UpdateEmployeeInput) bool { return input.AccountID != nil }},
	{tab: employeeTabEmploymentInfo, field: "manager_employee_id", present: func(input UpdateEmployeeInput) bool { return input.ManagerEmployeeID != nil }},
	{tab: employeeTabEmploymentInfo, field: "position_id", present: func(input UpdateEmployeeInput) bool { return input.PositionID != nil }},
	{tab: employeeTabEmploymentInfo, field: "position", present: func(input UpdateEmployeeInput) bool { return input.Position != nil }},
	{tab: employeeTabEmploymentInfo, field: "category", present: func(input UpdateEmployeeInput) bool { return input.Category != nil }},
	{tab: employeeTabEmploymentInfo, field: "status", present: func(input UpdateEmployeeInput) bool { return input.Status != nil }},
	{tab: employeeTabEmploymentInfo, field: "employment_status", present: func(input UpdateEmployeeInput) bool { return input.EmploymentStatus != nil }},
	{tab: employeeTabEmploymentInfo, field: "hire_date", present: func(input UpdateEmployeeInput) bool { return input.HireDate != nil }},
	{tab: employeeTabEmploymentInfo, field: "resign_date", present: func(input UpdateEmployeeInput) bool { return input.ResignDate != nil }},
	{tab: employeeTabEmploymentInfo, field: "internal_experiences", present: func(input UpdateEmployeeInput) bool { return input.InternalExperiences != nil }},
}

type employeePatchMapPolicyField struct {
	tab    string
	values func(UpdateEmployeeInput) map[string]any
}

var employeeMapPatchPolicyFields = []employeePatchMapPolicyField{
	{tab: employeeTabBasicInfo, values: func(input UpdateEmployeeInput) map[string]any { return input.BasicInfo }},
	{tab: employeeTabEmploymentInfo, values: func(input UpdateEmployeeInput) map[string]any { return input.EmploymentInfo }},
	{tab: employeeTabEducationMilitaryInfo, values: func(input UpdateEmployeeInput) map[string]any { return input.EducationMilitaryInfo }},
	{tab: employeeTabContactInfo, values: func(input UpdateEmployeeInput) map[string]any { return input.ContactInfo }},
	{tab: employeeTabInsuranceInfo, values: func(input UpdateEmployeeInput) map[string]any { return input.InsuranceInfo }},
}

type employeeExportColumn struct {
	field  string
	header string
	value  func(Employee) string
}

var employeeExportColumns = []employeeExportColumn{
	{field: "employee_no", header: "員工編號", value: func(employee Employee) string { return employee.EmployeeNo }},
	{field: "name", header: "姓名", value: func(employee Employee) string { return employee.Name }},
	{field: "company_email", header: "Email", value: func(employee Employee) string { return employee.CompanyEmail }},
	{field: "org_unit_id", header: "部門", value: func(employee Employee) string { return employee.OrgUnitID }},
	{field: "position", header: "職位", value: func(employee Employee) string { return employee.Position }},
	{field: "category", header: "類別", value: func(employee Employee) string { return employee.Category }},
	{field: "phone", header: "電話", value: func(employee Employee) string { return employee.Phone }},
	{field: "employment_status", header: "狀態", value: func(employee Employee) string { return employeeStatus(employee) }},
	{field: "hire_date", header: "到職日期", value: func(employee Employee) string { return formatDate(employee.HireDate) }},
	{field: "manager_employee_id", header: "主管員工ID", value: func(employee Employee) string { return employee.ManagerEmployeeID }},
}

// employeeExportColumnsForPolicy 處理員工 export columns for 政策。
func employeeExportColumnsForPolicy(policies map[string]string) []employeeExportColumn {
	out := make([]employeeExportColumn, 0, len(employeeExportColumns))
	for _, column := range employeeExportColumns {
		if employeeFieldPolicyHidden(policies[column.field]) {
			continue
		}
		out = append(out, column)
	}
	return out
}

// employeeFieldPolicyHidden 處理員工欄位政策 hidden。
func employeeFieldPolicyHidden(effect string) bool {
	return effect == "hide" || effect == "deny"
}

const (
	employeeImportColumnEmployeeNo = iota
	employeeImportColumnName
	employeeImportColumnEmail
	employeeImportColumnOrgUnit
	employeeImportColumnPosition
	employeeImportColumnCategory
	employeeImportColumnPhone
	employeeImportColumnStatus
	employeeImportColumnHireDate
	employeeImportColumnManagerEmployeeID
	employeeImportColumnAccountPolicy
)

type employeeImportColumn struct {
	header string
}

var employeeImportColumns = []employeeImportColumn{
	{header: "員工編號"},
	{header: "姓名"},
	{header: "Email"},
	{header: "部門"},
	{header: "職位"},
	{header: "類別"},
	{header: "電話"},
	{header: "狀態"},
	{header: "到職日期"},
	{header: "主管員工ID"},
	{header: "帳號策略"},
}

// employeeImportColumnCount 處理員工 import column count。
func employeeImportColumnCount() int {
	return len(employeeImportColumns)
}

// restrictedEmployeeFieldPolicies 處理 restricted 員工欄位政策。
func restrictedEmployeeFieldPolicies(policies map[string]string) map[string][]string {
	out := map[string][]string{}
	for field, effect := range policies {
		switch effect {
		case "mask", "hide", "deny":
			out[effect] = append(out[effect], field)
		}
	}
	for effect, fields := range out {
		out[effect] = uniqueSorted(fields)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// auditSensitiveEmployeeRead 寫入員工敏感欄位明文讀取稽核。
func (c HRService) auditSensitiveEmployeeRead(ctx RequestContext, decision CheckResult, items []Employee, target string) error {
	fields := visibleSensitiveEmployeeFields(decision, items)
	if len(fields) == 0 {
		return nil
	}
	details := auditDecisionDetails(ctx, decision, map[string]any{
		"fields": fields,
		"count":  len(items),
	})
	resource := string(ResourceEmployeeCollection)
	if strings.TrimSpace(target) != "" {
		resource = string(ResourceEmployee)
		details["resource_id"] = target
	} else {
		ids, truncated := employeeAuditResourceIDs(items, 50)
		details["resource_ids"] = ids
		if truncated {
			details["resource_ids_truncated"] = true
		}
	}
	return c.audit(ctx, "hr.employee.sensitive_field.read", resource, target, string(SeverityHigh), details)
}

// visibleSensitiveEmployeeFields 回傳本次響應中以明文返回的預設敏感欄位。
func visibleSensitiveEmployeeFields(decision CheckResult, items []Employee) []string {
	defaults := defaultFieldPolicies(AppHR, ResourceEmployee)
	fields := make([]string, 0, len(defaults))
	for field, defaultEffect := range defaults {
		if !employeeFieldPolicyRestrictsRead(defaultEffect) {
			continue
		}
		if employeeFieldPolicyRestrictsRead(decision.FieldPolicies[field]) {
			continue
		}
		if employeeItemsHaveFieldValue(items, field) {
			fields = append(fields, field)
		}
	}
	sort.Strings(fields)
	return fields
}

func employeeFieldPolicyRestrictsRead(effect string) bool {
	switch strings.TrimSpace(effect) {
	case "mask", "hide", "deny":
		return true
	default:
		return false
	}
}

func employeeItemsHaveFieldValue(items []Employee, field string) bool {
	for _, item := range items {
		if employeeFieldHasValue(item, field) {
			return true
		}
	}
	return false
}

func employeeFieldHasValue(item Employee, field string) bool {
	switch field {
	case "personal_email":
		return stringHasVisibleValue(item.PersonalEmail)
	case "phone":
		return stringHasVisibleValue(item.Phone)
	case "insurance_info":
		return len(item.InsuranceInfo) > 0
	default:
		return mapFieldHasVisibleValue(item.BasicInfo, field) ||
			mapFieldHasVisibleValue(item.ContactInfo, field) ||
			mapFieldHasVisibleValue(item.InsuranceInfo, field) ||
			mapFieldHasVisibleValue(item.EmploymentInfo, field) ||
			mapFieldHasVisibleValue(item.EducationMilitaryInfo, field)
	}
}

func mapFieldHasVisibleValue(values map[string]any, field string) bool {
	if len(values) == 0 {
		return false
	}
	value, ok := values[field]
	if !ok {
		return false
	}
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return stringHasVisibleValue(v)
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	default:
		return true
	}
}

func stringHasVisibleValue(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value != "****"
}

func employeeAuditResourceIDs(items []Employee, limit int) ([]string, bool) {
	if limit <= 0 {
		limit = 50
	}
	ids := make([]string, 0, minInt(len(items), limit))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		ids = append(ids, item.ID)
		if len(ids) >= limit {
			return ids, len(items) > len(ids)
		}
	}
	return ids, false
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// employeeImportInputFromRecord 處理員工 import 輸入 來源 record。
func employeeImportInputFromRecord(record []string) map[string]string {
	input := make(map[string]string, len(employeeImportColumns))
	for i, column := range employeeImportColumns {
		input[column.header] = record[i]
	}
	return input
}

// employeeCreateInputFromImportRecord 處理員工 create 輸入 來源 import record。
func employeeCreateInputFromImportRecord(record []string) CreateEmployeeInput {
	email := strings.TrimSpace(record[employeeImportColumnEmail])
	name := strings.TrimSpace(record[employeeImportColumnName])
	orgUnitID := strings.TrimSpace(record[employeeImportColumnOrgUnit])
	managerEmployeeID := strings.TrimSpace(record[employeeImportColumnManagerEmployeeID])
	position := strings.TrimSpace(record[employeeImportColumnPosition])
	category := normalizeEmployeeCategory(record[employeeImportColumnCategory])
	phone := strings.TrimSpace(record[employeeImportColumnPhone])
	status := normalizeEmployeeStatus(record[employeeImportColumnStatus])
	accountPolicy := normalizeEmployeeAccountPolicy(record[employeeImportColumnAccountPolicy])
	return CreateEmployeeInput{
		EmployeeNo:        strings.TrimSpace(record[employeeImportColumnEmployeeNo]),
		Name:              name,
		CompanyEmail:      email,
		OrgUnitID:         orgUnitID,
		ManagerEmployeeID: managerEmployeeID,
		Position:          position,
		Category:          category,
		Phone:             phone,
		EmploymentStatus:  status,
		Status:            status,
		AccountPolicy:     accountPolicy,
		HireDate:          normalizeImportDate(record[employeeImportColumnHireDate]),
		BasicInfo:         map[string]any{"company_email": email, "name": name},
		EmploymentInfo: map[string]any{
			"org_unit_id":         orgUnitID,
			"manager_employee_id": managerEmployeeID,
			"position":            position,
			"category":            category,
		},
		ContactInfo: map[string]any{"mobile_phone": phone},
	}
}

// employeeHotValue 處理員工 hot value。
func employeeHotValue(employee Employee, field string) string {
	for _, source := range employeeHotFieldSources[field] {
		value := employeeSourceValue(employee, source)
		if value != "" {
			return value
		}
	}
	return ""
}

// employeeSourceValue 處理員工 source value。
func employeeSourceValue(employee Employee, source employeeFieldSource) string {
	switch source.tab {
	case employeeTabBasicInfo:
		return stringFromMap(employee.BasicInfo, source.key)
	case employeeTabEmploymentInfo:
		return stringFromMap(employee.EmploymentInfo, source.key)
	case employeeTabEducationMilitaryInfo:
		return stringFromMap(employee.EducationMilitaryInfo, source.key)
	case employeeTabContactInfo:
		return stringFromMap(employee.ContactInfo, source.key)
	case employeeTabInsuranceInfo:
		return stringFromMap(employee.InsuranceInfo, source.key)
	default:
		return ""
	}
}

const maxEmployeeAvatarBytes = 2 << 20

// PreviewCreateEmployee 預覽create 員工的服務流程。
func (c HRService) PreviewCreateEmployee(ctx RequestContext, input CreateEmployeeInput) (EmployeePreviewResponse, error) {
	_, _, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, Action: ActionCreate},
		AuditTarget{Event: "hr.employee.preview_create", Resource: string(ResourceEmployee)},
	)
	if err != nil {
		return EmployeePreviewResponse{}, err
	}
	employee, err := c.employeeCreateCandidate(ctx, input)
	if err != nil {
		return EmployeePreviewResponse{}, err
	}
	if err := c.ensureEmployeePosition(ctx, &employee, false); err != nil {
		return EmployeePreviewResponse{}, err
	}
	if len(employee.InternalExperiences) == 0 {
		employee.InternalExperiences = append(employee.InternalExperiences, c.newEmployeeExperience(employee, "新進"))
	}
	resp := employeePreviewResponse(employee, nil)
	if err := c.validateEmployee(ctx, employee, "create", employeeValidationFullForm); err != nil {
		if appErr, ok := domain.AsAppError(err); ok && appErr.Code == "validation_failed" {
			resp.FieldErrors = appErr.FieldErrors
			resp.Valid = false
		} else {
			return EmployeePreviewResponse{}, err
		}
	}
	if err := authzAudit.Commit(ctx); err != nil {
		return EmployeePreviewResponse{}, err
	}
	return resp, nil
}

// PreviewUpdateEmployee 預覽update 員工的服務流程。
func (c HRService) PreviewUpdateEmployee(ctx RequestContext, id string, input UpdateEmployeeInput) (EmployeePreviewResponse, error) {
	account, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: id, Action: ActionUpdate},
		AuditTarget{Event: "hr.employee.preview_update", Resource: string(ResourceEmployee), Target: id},
	)
	if err != nil {
		return EmployeePreviewResponse{}, err
	}
	employee, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return EmployeePreviewResponse{}, err
	}
	if !ok {
		return EmployeePreviewResponse{}, NotFound("employee", id)
	}
	visible, err := c.filterEmployeesByDecision(ctx, account, []Employee{employee}, decision)
	if err != nil {
		return EmployeePreviewResponse{}, err
	}
	if len(visible) == 0 {
		return EmployeePreviewResponse{}, forbiddenDataScope("employee is outside data scope")
	}
	if fields := forbiddenEmployeePatchFields(input, decision.FieldPolicies); len(fields) > 0 {
		return EmployeePreviewResponse{}, domainValidation("employee field policy denied update", fields...)
	}
	before := employee
	err = c.applyEmployeePatchWithPositionCreation(ctx, &employee, input, false)
	resp := employeePreviewResponse(employee, employeeDiff(before, employee))
	if err != nil {
		if appErr, ok := domain.AsAppError(err); ok && appErr.Code == "validation_failed" {
			resp.FieldErrors = appErr.FieldErrors
			resp.Valid = false
		} else {
			return EmployeePreviewResponse{}, err
		}
	}
	if err := authzAudit.Commit(ctx); err != nil {
		return EmployeePreviewResponse{}, err
	}
	return resp, nil
}

// UpdateEmployeeAvatar 更新員工 avatar 的服務流程。
func (c HRService) UpdateEmployeeAvatar(ctx RequestContext, id string, input EmployeeAvatarInput) (Employee, error) {
	contentType, err := validateEmployeeAvatarInput(input)
	if err != nil {
		return Employee{}, err
	}
	input.ContentType = contentType
	account, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: id, Action: ActionUpdate},
		AuditTarget{Event: "hr.employee.avatar.update", Resource: string(ResourceEmployee), Target: id},
	)
	if err != nil {
		return Employee{}, err
	}
	if employeeAvatarDenied(decision.FieldPolicies) {
		return Employee{}, domainValidation("employee field policy denied update", FieldError{Tab: employeeTabBasicInfo, Field: "avatar", Code: "field_denied", Message: "avatar cannot be updated by current permission policy"})
	}
	var employee Employee
	var oldKey string
	var newKey string
	newObjectWritten := false
	if err := c.withTransaction(ctx, func(tx HRService) error {
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
		oldKey = stringFromMap(next.BasicInfo, "avatar_object_key")
		newKey = employeeAvatarObjectKey(ctx.TenantID, next.ID, input.Filename, input.ContentType)
		if err := tx.objectStore.PutObject(goContext(ctx), newKey, input.ContentType, input.Content); err != nil {
			return BadRequest("store avatar: " + err.Error())
		}
		newObjectWritten = true
		next.BasicInfo = mergeMap(next.BasicInfo, map[string]any{
			"avatar": map[string]any{
				"object_key":    newKey,
				"content_type":  input.ContentType,
				"original_name": strings.TrimSpace(input.Filename),
			},
			"avatar_object_key":   newKey,
			"avatar_content_type": input.ContentType,
		})
		next.UpdatedAt = tx.Now()
		if err := tx.store.UpsertEmployee(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.avatar.update", string(ResourceEmployee), next.ID, string(SeverityMedium), auditDecisionDetails(ctx, decision, map[string]any{"object_key": newKey})); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		employee = next
		return nil
	}); err != nil {
		if newObjectWritten && newKey != "" {
			c.deleteObjectIfSupported(ctx, newKey)
		}
		return Employee{}, err
	}
	if oldKey != "" && oldKey != newKey {
		c.deleteObjectIfSupported(ctx, oldKey)
	}
	return employee, nil
}

// DeleteEmployeeAvatar 刪除員工 avatar 的服務流程。
func (c HRService) DeleteEmployeeAvatar(ctx RequestContext, id string) (Employee, error) {
	account, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: id, Action: ActionUpdate},
		AuditTarget{Event: "hr.employee.avatar.delete", Resource: string(ResourceEmployee), Target: id},
	)
	if err != nil {
		return Employee{}, err
	}
	if employeeAvatarDenied(decision.FieldPolicies) {
		return Employee{}, domainValidation("employee field policy denied update", FieldError{Tab: employeeTabBasicInfo, Field: "avatar", Code: "field_denied", Message: "avatar cannot be updated by current permission policy"})
	}
	var employee Employee
	var oldKey string
	if err := c.withTransaction(ctx, func(tx HRService) error {
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
		oldKey = stringFromMap(next.BasicInfo, "avatar_object_key")
		next.BasicInfo = utils.CopyStringMap(next.BasicInfo)
		delete(next.BasicInfo, "avatar")
		delete(next.BasicInfo, "avatar_object_key")
		delete(next.BasicInfo, "avatar_content_type")
		next.UpdatedAt = tx.Now()
		if err := tx.store.UpsertEmployee(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.avatar.delete", string(ResourceEmployee), next.ID, string(SeverityMedium), auditDecisionDetails(ctx, decision, map[string]any{"object_key": oldKey})); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		employee = next
		return nil
	}); err != nil {
		return Employee{}, err
	}
	if oldKey != "" {
		c.deleteObjectIfSupported(ctx, oldKey)
	}
	return employee, nil
}

// EmployeeImportTemplate 處理員工 import 範本的服務流程。
func (c HRService) EmployeeImportTemplate(ctx RequestContext, format string) ([]byte, string, string, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return nil, "", "", err
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, Action: ActionRead})
	if err != nil {
		return nil, "", "", err
	}
	if !decision.Allowed {
		return nil, "", "", forbiddenAuthz(decision)
	}
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "csv":
		raw, err := employeeImportTemplateCSV()
		return raw, "employee-import-template.csv", "text/csv; charset=utf-8", err
	case "xlsx":
		raw, err := employeeImportTemplateXLSX()
		return raw, "employee-import-template.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", err
	default:
		return nil, "", "", BadRequest("format must be csv or xlsx")
	}
}

// employeePreviewResponse 處理員工 preview 回應。
func employeePreviewResponse(employee Employee, diff map[string]any) EmployeePreviewResponse {
	return EmployeePreviewResponse{Employee: employee, Detail: domain.EmployeeDetailFromEmployee(employee), Diff: diff, Valid: true}
}

// employeeQueryApprovalFilters 處理員工查詢核准篩選。
func employeeQueryApprovalFilters(query EmployeeQuery) map[string]any {
	out := map[string]any{}
	if query.Keyword != "" {
		out["keyword"] = query.Keyword
	}
	if query.DepartmentID != "" {
		out["department_id"] = query.DepartmentID
	}
	if query.EmploymentStatus != "" {
		out["employment_status"] = query.EmploymentStatus
	}
	if query.Category != "" {
		out["category"] = query.Category
	}
	return out
}

// employeeDiff 處理員工 diff。
func employeeDiff(before, after Employee) map[string]any {
	diff := map[string]any{}
	add := func(field string, oldValue, newValue any) {
		if !reflect.DeepEqual(oldValue, newValue) {
			diff[field] = map[string]any{"before": oldValue, "after": newValue}
		}
	}
	add("employee_no", before.EmployeeNo, after.EmployeeNo)
	add("name", before.Name, after.Name)
	add("company_email", before.CompanyEmail, after.CompanyEmail)
	add("personal_email", before.PersonalEmail, after.PersonalEmail)
	add("phone", before.Phone, after.Phone)
	add("org_unit_id", before.OrgUnitID, after.OrgUnitID)
	add("account_id", before.AccountID, after.AccountID)
	add("manager_employee_id", before.ManagerEmployeeID, after.ManagerEmployeeID)
	add("position_id", before.PositionID, after.PositionID)
	add("position", before.Position, after.Position)
	add("category", before.Category, after.Category)
	add("status", before.Status, after.Status)
	add("employment_status", before.EmploymentStatus, after.EmploymentStatus)
	add("basic_info", before.BasicInfo, after.BasicInfo)
	add("employment_info", before.EmploymentInfo, after.EmploymentInfo)
	add("education_military_info", before.EducationMilitaryInfo, after.EducationMilitaryInfo)
	add("contact_info", before.ContactInfo, after.ContactInfo)
	add("insurance_info", before.InsuranceInfo, after.InsuranceInfo)
	add("internal_experiences", before.InternalExperiences, after.InternalExperiences)
	if len(diff) == 0 {
		return nil
	}
	return diff
}

// validateEmployeeAvatarInput 驗證員工 avatar 輸入。
func validateEmployeeAvatarInput(input EmployeeAvatarInput) (string, error) {
	if len(input.Content) == 0 {
		return "", BadRequest("avatar file is required")
	}
	if len(input.Content) > maxEmployeeAvatarBytes {
		return "", BadRequest("avatar file exceeds 2MB limit")
	}
	declared := normalizedEmployeeAvatarContentType(input.ContentType)
	detected := detectEmployeeAvatarContentType(input.Content)
	if detected == "" {
		return "", BadRequest("avatar file must be a valid image/png, image/jpeg or image/webp")
	}
	if declared != "" && declared != detected {
		return "", BadRequest("avatar content_type does not match file content")
	}
	return detected, nil
}

// normalizedEmployeeAvatarContentType 處理 normalized 員工 avatar content type。
func normalizedEmployeeAvatarContentType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if idx := strings.Index(value, ";"); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	return value
}

// detectEmployeeAvatarContentType 處理 detect 員工 avatar content type。
func detectEmployeeAvatarContentType(raw []byte) string {
	switch {
	case bytes.HasPrefix(raw, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}):
		return "image/png"
	case len(raw) >= 3 && raw[0] == 0xff && raw[1] == 0xd8 && raw[2] == 0xff:
		return "image/jpeg"
	case len(raw) >= 12 && bytes.Equal(raw[0:4], []byte("RIFF")) && bytes.Equal(raw[8:12], []byte("WEBP")):
		return "image/webp"
	default:
		return ""
	}
}

// employeeAvatarDenied 處理員工 avatar 拒絕。
func employeeAvatarDenied(policies map[string]string) bool {
	for _, field := range []string{"basic_info", "avatar", "avatar_object_key", "avatar_content_type"} {
		switch policies[field] {
		case "deny", "hide", "readonly":
			return true
		}
	}
	return false
}

// employeeAvatarObjectKey 處理員工 avatar 物件 key。
func employeeAvatarObjectKey(tenantID, employeeID, filename, contentType string) string {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(filename)))
	if ext == "" {
		switch strings.ToLower(strings.TrimSpace(contentType)) {
		case "image/png":
			ext = ".png"
		case "image/jpeg":
			ext = ".jpg"
		case "image/webp":
			ext = ".webp"
		}
	}
	return "employee-avatars/" + tenantID + "/" + employeeID + "/" + utils.NewID("avatar") + ext
}

// deleteObjectIfSupported 刪除物件 if supported 的服務流程。
func (c HRService) deleteObjectIfSupported(ctx RequestContext, key string) {
	deleter, ok := c.objectStore.(objectDeleter)
	if !ok {
		return
	}
	_ = deleter.DeleteObject(goContext(ctx), key)
}

// employeeImportTemplateCSV 處理員工 import 範本 CSV。
func employeeImportTemplateCSV() ([]byte, error) {
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(&buf)
	if err := writer.Write(employeeImportTemplateHeaders()); err != nil {
		return nil, err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// employeeImportTemplateHeaders 處理員工 import 範本 headers。
func employeeImportTemplateHeaders() []string {
	headers := make([]string, 0, len(employeeImportColumns))
	for _, column := range employeeImportColumns {
		headers = append(headers, column.header)
	}
	return headers
}

// employeeImportTemplateXLSX 處理員工 import 範本 XLSX。
func employeeImportTemplateXLSX() ([]byte, error) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml":        xlsxContentTypesXML,
		"_rels/.rels":                xlsxRelsXML,
		"xl/workbook.xml":            xlsxWorkbookXML,
		"xl/_rels/workbook.xml.rels": xlsxWorkbookRelsXML,
		"xl/worksheets/sheet1.xml":   employeeImportTemplateSheetXML(),
		"xl/sharedStrings.xml":       employeeImportTemplateSharedStringsXML(),
	}
	for name, body := range files {
		file, err := zipWriter.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := file.Write([]byte(body)); err != nil {
			return nil, err
		}
	}
	if err := zipWriter.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// employeeImportTemplateSheetXML 處理員工 import 範本 sheet xml。
func employeeImportTemplateSheetXML() string {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1">`)
	for i := range employeeImportTemplateHeaders() {
		col := string(rune('A' + i))
		buf.WriteString(`<c r="`)
		buf.WriteString(col)
		buf.WriteString(`1" t="s"><v>`)
		buf.WriteString(strconv.Itoa(i))
		buf.WriteString(`</v></c>`)
	}
	buf.WriteString(`</row></sheetData></worksheet>`)
	return buf.String()
}

// employeeImportTemplateSharedStringsXML 處理員工 import 範本 shared 字串 xml。
func employeeImportTemplateSharedStringsXML() string {
	headers := employeeImportTemplateHeaders()
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="`)
	buf.WriteString(strconv.Itoa(len(headers)))
	buf.WriteString(`" uniqueCount="`)
	buf.WriteString(strconv.Itoa(len(headers)))
	buf.WriteString(`">`)
	for _, header := range headers {
		buf.WriteString(`<si><t>`)
		_ = xml.EscapeText(&buf, []byte(header))
		buf.WriteString(`</t></si>`)
	}
	buf.WriteString(`</sst>`)
	return buf.String()
}

const xlsxContentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
<Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/>
</Types>`

const xlsxRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`

const xlsxWorkbookXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<sheets><sheet name="Employees" sheetId="1" r:id="rId1"/></sheets>
</workbook>`

const xlsxWorkbookRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/>
</Relationships>`

// parseEmployeeImport 解析員工 import。
func parseEmployeeImport(filename string, raw []byte) ([]EmployeeImportRow, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".xlsx":
		return parseEmployeeXLSX(raw)
	case ".csv":
		return parseEmployeeCSV(raw)
	default:
		return nil, fmt.Errorf("employee import supports csv and xlsx files")
	}
}

// importContentType 匯入 content type。
func importContentType(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	default:
		return "text/csv; charset=utf-8"
	}
}

// parseEmployeeCSV 解析員工 CSV。
func parseEmployeeCSV(raw []byte) ([]EmployeeImportRow, error) {
	reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})))
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv: %w", err)
	}
	return employeeRowsFromRecords(records)
}

// parseEmployeeXLSX 解析員工 XLSX。
func parseEmployeeXLSX(raw []byte) ([]EmployeeImportRow, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("parse xlsx: %w", err)
	}
	files := map[string]*zip.File{}
	for _, file := range zr.File {
		files[file.Name] = file
	}
	shared, err := readXLSXSharedStrings(files["xl/sharedStrings.xml"])
	if err != nil {
		return nil, err
	}
	sheet := files["xl/worksheets/sheet1.xml"]
	if sheet == nil {
		return nil, fmt.Errorf("xlsx sheet1.xml not found")
	}
	records, err := readXLSXSheet(sheet, shared)
	if err != nil {
		return nil, err
	}
	return employeeRowsFromRecords(records)
}

// employeeRowsFromRecords 處理員工列 來源 records。
func employeeRowsFromRecords(records [][]string) ([]EmployeeImportRow, error) {
	if len(records) < 2 {
		return nil, fmt.Errorf("import file must include header and at least one data row")
	}
	if err := validateEmployeeImportHeader(records[0]); err != nil {
		return nil, err
	}
	rows := make([]EmployeeImportRow, 0, len(records)-1)
	for i, record := range records[1:] {
		record = padRecord(record, employeeImportColumnCount())
		rows = append(rows, EmployeeImportRow{
			RowNumber: i + 2,
			Input:     employeeImportInputFromRecord(record),
			Employee:  employeeCreateInputFromImportRecord(record),
		})
	}
	return rows, nil
}

// validateEmployeeImportHeader 驗證員工 import header。
func validateEmployeeImportHeader(record []string) error {
	headers := employeeImportTemplateHeaders()
	if employeeImportHeaderMatches(record, headers) {
		return nil
	}
	legacyHeaders := headers[:employeeImportColumnAccountPolicy]
	if employeeImportHeaderMatches(record, legacyHeaders) {
		return nil
	}
	if len(record) < len(headers) {
		return fmt.Errorf("import file header must include %d columns", len(headers))
	}
	for i, want := range headers {
		if got := strings.TrimSpace(record[i]); got != want {
			return fmt.Errorf("import file header column %d must be %q", i+1, want)
		}
	}
	return nil
}

// employeeImportHeaderMatches 處理員工 import header matches。
func employeeImportHeaderMatches(record []string, headers []string) bool {
	if len(record) < len(headers) {
		return false
	}
	for i, want := range headers {
		if strings.TrimSpace(record[i]) != want {
			return false
		}
	}
	return true
}

type xlsxSST struct {
	Items []struct {
		Text string `xml:"t"`
	} `xml:"si"`
}

// readXLSXSharedStrings 讀取 XLSX shared 字串。
func readXLSXSharedStrings(file *zip.File) ([]string, error) {
	if file == nil {
		return nil, nil
	}
	raw, err := readLimitedXLSXFile(file, maxEmployeeImportXLSXEntryBytes)
	if err != nil {
		return nil, err
	}
	var sst xlsxSST
	if err := xml.Unmarshal(raw, &sst); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(sst.Items))
	for _, item := range sst.Items {
		out = append(out, item.Text)
	}
	return out, nil
}

type xlsxWorksheet struct {
	Rows []struct {
		Cells []struct {
			Ref   string `xml:"r,attr"`
			Type  string `xml:"t,attr"`
			Value string `xml:"v"`
		} `xml:"c"`
	} `xml:"sheetData>row"`
}

// readXLSXSheet 讀取 XLSX sheet。
func readXLSXSheet(file *zip.File, shared []string) ([][]string, error) {
	raw, err := readLimitedXLSXFile(file, maxEmployeeImportXLSXEntryBytes)
	if err != nil {
		return nil, err
	}
	var sheet xlsxWorksheet
	if err := xml.Unmarshal(raw, &sheet); err != nil {
		return nil, err
	}
	records := make([][]string, 0, len(sheet.Rows))
	for _, row := range sheet.Rows {
		record := make([]string, employeeImportColumnCount())
		for idx, cell := range row.Cells {
			col := idx
			if cell.Ref != "" {
				col = xlsxColumnIndex(cell.Ref)
			}
			if col < 0 || col >= len(record) {
				continue
			}
			value := cell.Value
			if cell.Type == "s" {
				i, _ := strconv.Atoi(value)
				if i >= 0 && i < len(shared) {
					value = shared[i]
				}
			}
			record[col] = value
		}
		records = append(records, record)
	}
	return records, nil
}

// readLimitedXLSXFile 讀取 limited XLSX 檔案。
func readLimitedXLSXFile(file *zip.File, maxBytes int64) ([]byte, error) {
	if file == nil {
		return nil, nil
	}
	if file.UncompressedSize64 > uint64(maxBytes) {
		return nil, fmt.Errorf("xlsx entry %s exceeds %d bytes", file.Name, maxBytes)
	}
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	limited := &io.LimitedReader{R: rc, N: maxBytes + 1}
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("xlsx entry %s exceeds %d bytes", file.Name, maxBytes)
	}
	return raw, nil
}

// xlsxColumnIndex 處理 XLSX column index。
func xlsxColumnIndex(ref string) int {
	col := 0
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}
		col = col*26 + int(r-'A'+1)
	}
	return col - 1
}

// normalizeImportDate 正規化import 日期。
func normalizeImportDate(value string) string {
	value = strings.TrimSpace(value)
	if strings.Count(value, "/") == 2 {
		parts := strings.Split(value, "/")
		if len(parts[1]) == 1 {
			parts[1] = "0" + parts[1]
		}
		if len(parts[2]) == 1 {
			parts[2] = "0" + parts[2]
		}
		return strings.Join(parts, "-")
	}
	return value
}

// padRecord 處理 pad record。
func padRecord(record []string, size int) []string {
	if len(record) >= size {
		return record
	}
	out := make([]string, size)
	copy(out, record)
	return out
}

const (
	maxEmployeeImportBytes          = 10 << 20
	maxEmployeeImportXLSXEntryBytes = maxEmployeeImportBytes
	maxEmployeeImportRows           = 500
)

const (
	employeeImportModeCreate = "create"
	employeeImportModeUpdate = "update"
	employeeImportModeUpsert = "upsert"
)

// normalizeEmployeeImportMode 正規化員工 import mode。
func normalizeEmployeeImportMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", employeeImportModeCreate:
		return employeeImportModeCreate, nil
	case employeeImportModeUpdate:
		return employeeImportModeUpdate, nil
	case employeeImportModeUpsert:
		return employeeImportModeUpsert, nil
	default:
		return "", BadRequest("employee import mode must be create, update, or upsert")
	}
}

// validateEmployeeImportFailurePolicy 驗證員工 import failure 政策。
func validateEmployeeImportFailurePolicy(policy string) error {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "", "all_or_nothing":
		return nil
	default:
		return BadRequest("employee import failure_policy must be all_or_nothing")
	}
}

// safeImportFilename 處理 safe import filename。
func safeImportFilename(filename string) string {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = "employees.csv"
	}
	filename = filepath.Base(strings.ReplaceAll(filename, "\\", "/"))
	if filename == "." || filename == "/" || filename == "" {
		return "employees.csv"
	}
	return strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', 0:
			return '_'
		default:
			return r
		}
	}, filename)
}

// employeeImportObjectKey 處理員工 import 物件 key。
func employeeImportObjectKey(tenantID, sessionID, filename string) string {
	return "employee-imports/" + tenantID + "/" + sessionID + "/" + utils.NewID("file") + "-" + safeImportFilename(filename)
}

// employeeImportSHA256 處理員工 import sha 256。
func employeeImportSHA256(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// employeeImportAuditDetails 處理員工 import 稽核 details。
func employeeImportAuditDetails(session EmployeeImportSession) map[string]any {
	details := utils.CopyStringMap(session.Summary)
	if details == nil {
		details = map[string]any{}
	}
	details["session_id"] = session.ID
	details["filename"] = session.Filename
	details["object_provider"] = session.ObjectProvider
	details["object_bucket"] = session.ObjectBucket
	details["object_key"] = session.ObjectKey
	details["content_type"] = session.ContentType
	details["size_bytes"] = session.SizeBytes
	details["sha256"] = session.SHA256
	details["created_by_account_id"] = session.CreatedByAccountID
	if session.ConfirmedByAccountID != "" {
		details["confirmed_by_account_id"] = session.ConfirmedByAccountID
	}
	return details
}

// PreviewEmployeeImport 預覽員工 import 的服務流程。
func (c HRService) PreviewEmployeeImport(ctx RequestContext, input EmployeeImportPreviewInput) (EmployeeImportSession, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return EmployeeImportSession{}, err
	}
	req := CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, Action: ActionImport}
	decision, err := c.evaluateAuthz(ctx, account, req)
	if err != nil {
		return EmployeeImportSession{}, err
	}
	if !decision.Allowed {
		c.logWarn(ctx, "employee import preview denied",
			"reason", decision.Reason,
			"missing_permissions", decision.MissingPermissions,
		)
		return EmployeeImportSession{}, forbiddenAuthz(decision)
	}
	if decision.RequiresApproval {
		if err := c.confirmApproval(ctx, req); err != nil {
			_ = c.auditAuthzDecision(ctx, "hr.employee.import.preview", "employee_import_session", "", decision)
			if ctx.ApprovalInstanceID != "" {
				return EmployeeImportSession{}, err
			}
			c.logWarn(ctx, "employee import preview requires approval",
				"risk_level", decision.RiskLevel,
				"approval_type", decision.ApprovalType,
				"approval_reason", decision.ApprovalReason,
			)
			return EmployeeImportSession{}, domain.ForbiddenReason("approval_required", "high-risk action requires approval")
		}
	}
	authzAudit := AuthzAudit{service: c.Service, target: AuditTarget{Event: "hr.employee.import.preview", Resource: string(ResourceEmployeeImport)}, decision: decision}
	filename := safeImportFilename(input.Filename)
	raw := []byte(input.Content)
	if len(raw) > maxEmployeeImportBytes {
		return EmployeeImportSession{}, BadRequest("employee import file exceeds 10MB limit")
	}
	contentType := importContentType(filename)
	sha256Value := employeeImportSHA256(raw)
	rows, err := parseEmployeeImport(filename, raw)
	if err != nil {
		return EmployeeImportSession{}, BadRequest(err.Error())
	}
	if len(rows) > maxEmployeeImportRows {
		return EmployeeImportSession{}, BadRequest(fmt.Sprintf("employee import supports at most %d rows", maxEmployeeImportRows))
	}
	sessionID := utils.NewID("eimp")
	objectKey := employeeImportObjectKey(ctx.TenantID, sessionID, filename)
	if err := c.objectStore.PutObject(goContext(ctx), objectKey, contentType, raw); err != nil {
		return EmployeeImportSession{}, BadRequest("store import file: " + err.Error())
	}
	objectCommitted := false
	defer func() {
		if !objectCommitted {
			c.deleteObjectIfSupported(ctx, objectKey)
		}
	}()
	valid := 0
	rowErrors := make([]RowError, 0)
	batch := newEmployeeImportBatchIndex()
	for i := range rows {
		errors, err := c.validateEmployeeImportRow(ctx, rows[i], batch)
		if err != nil {
			return EmployeeImportSession{}, err
		}
		rows[i].Errors = append(rows[i].Errors, errors...)
		rows[i].Valid = len(rows[i].Errors) == 0
		if rows[i].Valid {
			valid++
		}
		rowErrors = append(rowErrors, rows[i].Errors...)
	}
	session := EmployeeImportSession{
		ID:                 sessionID,
		TenantID:           ctx.TenantID,
		Filename:           filename,
		ObjectProvider:     objectStoreProvider(c.objectStore),
		ObjectBucket:       objectStoreBucket(c.objectStore),
		ObjectKey:          objectKey,
		ContentType:        contentType,
		SizeBytes:          int64(len(raw)),
		SHA256:             sha256Value,
		Status:             "previewed",
		Rows:               rows,
		CreatedByAccountID: ctx.AccountID,
		Summary: map[string]any{
			"total":       len(rows),
			"valid":       valid,
			"invalid":     len(rows) - valid,
			"confirmable": valid,
			"error_count": len(rowErrors),
			"filename":    filename,
			"size_bytes":  len(raw),
			"sha256":      sha256Value,
		},
		CreatedAt: c.Now(),
		ExpiresAt: c.Now().Add(24 * time.Hour),
	}
	if err := c.withTransaction(ctx, func(tx HRService) error {
		if err := tx.store.UpsertEmployeeImportSession(goContext(ctx), session); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.import.preview", string(ResourceEmployeeImport), session.ID, string(SeverityMedium), employeeImportAuditDetails(session)); err != nil {
			return err
		}
		return authzAudit.CommitWith(ctx, tx.Service)
	}); err != nil {
		return EmployeeImportSession{}, err
	}
	objectCommitted = true
	c.logInfo(ctx, "employee import preview created",
		"session_id", session.ID,
		"filename", filename,
		"total_rows", len(rows),
		"valid_rows", valid,
		"invalid_rows", len(rows)-valid,
		"error_count", len(rowErrors),
	)
	return session, nil
}

// ConfirmEmployeeImport 確認員工 import 的服務流程。
func (c HRService) ConfirmEmployeeImport(ctx RequestContext, sessionID string, input EmployeeImportConfirmInput) (EmployeeImportSession, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return EmployeeImportSession{}, err
	}
	req := CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: sessionID, Action: ActionImport}
	decision, err := c.evaluateAuthz(ctx, account, req)
	if err != nil {
		return EmployeeImportSession{}, err
	}
	if !decision.Allowed {
		c.logWarn(ctx, "employee import confirmation denied",
			"session_id", sessionID,
			"reason", decision.Reason,
			"missing_permissions", decision.MissingPermissions,
		)
		return EmployeeImportSession{}, forbiddenAuthz(decision)
	}
	if decision.RequiresApproval {
		if err := c.confirmApproval(ctx, req); err != nil {
			_ = c.auditAuthzDecision(ctx, "hr.employee.import.confirm", "employee_import_session", sessionID, decision)
			if ctx.ApprovalInstanceID != "" {
				return EmployeeImportSession{}, err
			}
			c.logWarn(ctx, "employee import confirmation requires approval",
				"session_id", sessionID,
				"risk_level", decision.RiskLevel,
				"approval_type", decision.ApprovalType,
				"approval_reason", decision.ApprovalReason,
			)
			return EmployeeImportSession{}, domain.ForbiddenReason("approval_required", "high-risk action requires approval")
		}
	}
	authzAudit := AuthzAudit{service: c.Service, target: AuditTarget{Event: "hr.employee.import.confirm", Resource: string(ResourceEmployeeImport), Target: sessionID}, decision: decision}
	mode, err := normalizeEmployeeImportMode(input.Mode)
	if err != nil {
		return EmployeeImportSession{}, err
	}
	if err := validateEmployeeImportFailurePolicy(input.FailurePolicy); err != nil {
		return EmployeeImportSession{}, err
	}
	var session EmployeeImportSession
	confirmedCount := 0
	createdCount := 0
	updatedCount := 0
	provisionQueued := false
	var validationErr error
	if err := c.withTransaction(ctx, func(tx HRService) error {
		next, ok, err := tx.store.GetEmployeeImportSession(goContext(ctx), ctx.TenantID, sessionID)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("employee import session", sessionID)
		}
		if terminalEmployeeImportStatus(next.Status) {
			return Conflict("employee import session already confirmed")
		}
		if tx.Now().After(next.ExpiresAt) {
			return BadRequest("employee import session expired")
		}
		results := make([]BatchEmployeeResult, 0, len(next.Rows))
		rowErrors := make([]RowError, 0)
		type importEmployeeWrite struct {
			row      EmployeeImportRow
			employee Employee
			previous Employee
			update   bool
		}
		employees := make([]importEmployeeWrite, 0, len(next.Rows))
		reservedEmployeeNos := map[string]struct{}{}
		batch := newEmployeeImportBatchIndex()
		for i, row := range next.Rows {
			row.Errors = nil
			employee, previous, update, errors, err := tx.prepareEmployeeImportWrite(ctx, row, batch, mode, reservedEmployeeNos)
			if err != nil {
				return err
			}
			if len(errors) > 0 {
				row.Errors = errors
				row.Valid = false
				rowErrors = append(rowErrors, errors...)
				results = append(results, BatchEmployeeResult{RowNumber: row.RowNumber, Success: false, Code: "import_validation_failed", Message: firstRowErrorMessage(errors)})
				next.Rows[i] = row
				continue
			}
			scopeErrors, err := tx.employeeImportScopeErrors(ctx, account, row.RowNumber, employee, previous, update, decision)
			if err != nil {
				return err
			}
			if len(scopeErrors) > 0 {
				row.Errors = scopeErrors
				row.Valid = false
				rowErrors = append(rowErrors, scopeErrors...)
				results = append(results, BatchEmployeeResult{RowNumber: row.RowNumber, Success: false, Code: "import_validation_failed", Message: firstRowErrorMessage(scopeErrors)})
				next.Rows[i] = row
				continue
			}
			row.Valid = true
			next.Rows[i] = row
			employees = append(employees, importEmployeeWrite{row: row, employee: employee, previous: previous, update: update})
		}
		if len(rowErrors) > 0 {
			tx.logWarn(ctx, "employee import confirmation failed validation",
				"session_id", next.ID,
				"total_rows", len(next.Rows),
				"error_count", len(rowErrors),
			)
			next.Status = "failed_validation"
			next.ConfirmedByAccountID = ctx.AccountID
			if next.Summary == nil {
				next.Summary = map[string]any{}
			}
			next.Summary["total"] = len(next.Rows)
			next.Summary["confirmed"] = 0
			next.Summary["created"] = 0
			next.Summary["updated"] = 0
			next.Summary["failed"] = len(next.Rows)
			next.Summary["results"] = results
			next.Summary["row_errors"] = rowErrors
			next.Summary["error_count"] = len(rowErrors)
			next.Summary["mode"] = mode
			next.Summary["failure_policy"] = "all_or_nothing"
			if err := tx.store.UpsertEmployeeImportSession(goContext(ctx), next); err != nil {
				return err
			}
			if err := tx.audit(ctx, "hr.employee.import.confirm_failed", string(ResourceEmployeeImport), next.ID, string(SeverityHigh), employeeImportAuditDetails(next)); err != nil {
				return err
			}
			if err := authzAudit.CommitWith(ctx, tx.Service); err != nil {
				return err
			}
			session = next
			validationErr = ImportValidationFailed("employee import contains invalid rows", rowErrors)
			return nil
		}
		for _, item := range employees {
			employee := item.employee
			accountPolicy := string(EmployeeAccountPolicyNone)
			if !item.update {
				var err error
				accountPolicy, _, err = tx.applyEmployeeCreateAccountPolicy(ctx, &employee, item.row.Employee)
				if err != nil {
					return err
				}
			}
			if err := tx.store.UpsertEmployee(goContext(ctx), employee); err != nil {
				return err
			}
			previous := item.previous
			if err := tx.touchEmployeeAuthzIfNeeded(ctx, previous, employee, string(EventEmployeeAuthzSubjectImport)); err != nil {
				return err
			}
			if err := tx.linkEmployeeAccount(ctx, employee); err != nil {
				return err
			}
			if employee.AccountID != "" && accountPolicy != string(EmployeeAccountPolicyNone) {
				sendInvite := accountPolicy == string(EmployeeAccountPolicyCreatePendingInvite)
				if err := tx.provisionEmployeeIdentityFromAccountID(ctx, employee, employee.AccountID, sendInvite); err != nil {
					return err
				}
				provisionQueued = true
			}
			eventType := string(EventEmployeeCreated)
			action := "created"
			if item.update {
				eventType = string(EventEmployeeUpdated)
				action = "updated"
				updatedCount++
			} else {
				createdCount++
			}
			if err := tx.appendEmployeeEvent(ctx, eventType, employee.ID, map[string]any{"employee_id": employee.ID, "import_session_id": next.ID, "action": action}); err != nil {
				return err
			}
			results = append(results, BatchEmployeeResult{RowNumber: item.row.RowNumber, EmployeeID: employee.ID, Success: true, Message: action})
		}
		now := tx.Now()
		next.Status = "confirmed"
		next.ConfirmedAt = &now
		if next.Summary == nil {
			next.Summary = map[string]any{}
		}
		next.Summary["total"] = len(next.Rows)
		next.Summary["confirmed"] = len(employees)
		next.Summary["created"] = createdCount
		next.Summary["updated"] = updatedCount
		next.Summary["failed"] = 0
		next.Summary["results"] = results
		next.Summary["row_errors"] = rowErrors
		next.Summary["error_count"] = len(rowErrors)
		next.Summary["mode"] = mode
		next.Summary["failure_policy"] = "all_or_nothing"
		next.ConfirmedByAccountID = ctx.AccountID
		if err := tx.store.UpsertEmployeeImportSession(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeImported), next.ID, map[string]any{"session_id": next.ID, "success": len(employees), "created": createdCount, "updated": updatedCount, "failed": len(next.Rows) - len(employees), "mode": mode}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.import.confirm", string(ResourceEmployeeImport), next.ID, string(SeverityHigh), employeeImportAuditDetails(next)); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		confirmedCount = len(employees)
		session = next
		return nil
	}); err != nil {
		return EmployeeImportSession{}, err
	}
	if validationErr != nil {
		return session, validationErr
	}
	if provisionQueued {
		c.runIdentityProvisioningFastPath(ctx)
	}
	c.logInfo(ctx, "employee import confirmed",
		"session_id", session.ID,
		"total_rows", len(session.Rows),
		"confirmed_count", confirmedCount,
		"failed_count", len(session.Rows)-confirmedCount,
		"mode", mode,
	)
	return session, nil
}

// employeeImportScopeErrors 處理員工 import 範圍錯誤的服務流程。
func (c HRService) employeeImportScopeErrors(ctx RequestContext, account Account, rowNumber int, employee Employee, previous Employee, update bool, decision CheckResult) ([]RowError, error) {
	targets := []Employee{employee}
	if update {
		targets = append(targets, previous)
	}
	visible, err := c.filterEmployeesByDecision(ctx, account, targets, decision)
	if err != nil {
		return nil, err
	}
	if len(visible) == len(targets) {
		return nil, nil
	}
	return []RowError{{
		Row:     rowNumber,
		Field:   "authz_scope",
		Code:    "out_of_scope",
		Message: "employee import row is outside authorized scope",
	}}, nil
}

type employeeImportBatchIndex struct {
	employeeNos   map[string]int
	companyEmails map[string]int
	accountIDs    map[string]int
}

// newEmployeeImportBatchIndex 建立員工 import 批次 index。
func newEmployeeImportBatchIndex() *employeeImportBatchIndex {
	return &employeeImportBatchIndex{
		employeeNos:   map[string]int{},
		companyEmails: map[string]int{},
		accountIDs:    map[string]int{},
	}
}

// terminalEmployeeImportStatus 處理 terminal 員工 import 狀態。
func terminalEmployeeImportStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "confirmed", "partially_confirmed", "failed", "failed_validation":
		return true
	default:
		return false
	}
}

// prepareEmployeeImportWrite 處理 prepare 員工 import write 的服務流程。
func (c HRService) prepareEmployeeImportWrite(ctx RequestContext, row EmployeeImportRow, batch *employeeImportBatchIndex, mode string, reservedEmployeeNos map[string]struct{}) (Employee, Employee, bool, []RowError, error) {
	batchErrors := employeeImportBatchErrors(row, batch)
	if mode == employeeImportModeUpdate {
		return c.prepareEmployeeImportUpdate(ctx, row, batchErrors)
	}
	if mode == employeeImportModeUpsert {
		existing, ok, err := c.employeeImportExistingByEmployeeNo(ctx, row.Employee)
		if err != nil {
			return Employee{}, Employee{}, false, nil, err
		}
		if ok {
			return c.prepareEmployeeImportUpdateWithExisting(ctx, row, existing, batchErrors)
		}
	}
	employee, err := c.employeeFromImportInput(ctx, row.Employee, reservedEmployeeNos)
	if err != nil {
		errors, ok := employeeImportErrorsFromError(row.RowNumber, err)
		if !ok {
			return Employee{}, Employee{}, false, nil, err
		}
		return Employee{}, Employee{}, false, append(errors, batchErrors...), nil
	}
	return employee, Employee{}, false, batchErrors, nil
}

// prepareEmployeeImportUpdate 處理 prepare 員工 import update 的服務流程。
func (c HRService) prepareEmployeeImportUpdate(ctx RequestContext, row EmployeeImportRow, batchErrors []RowError) (Employee, Employee, bool, []RowError, error) {
	existing, ok, err := c.employeeImportExistingByEmployeeNo(ctx, row.Employee)
	if err != nil {
		return Employee{}, Employee{}, false, nil, err
	}
	if strings.TrimSpace(row.Employee.EmployeeNo) == "" {
		errors := []RowError{{Row: row.RowNumber, Field: "employee_no", Code: "required", Message: "employee_no is required for update imports"}}
		return Employee{}, Employee{}, false, append(errors, batchErrors...), nil
	}
	if !ok {
		errors := []RowError{{Row: row.RowNumber, Field: "employee_no", Code: "not_found", Message: "employee_no was not found for update import"}}
		return Employee{}, Employee{}, false, append(errors, batchErrors...), nil
	}
	return c.prepareEmployeeImportUpdateWithExisting(ctx, row, existing, batchErrors)
}

// prepareEmployeeImportUpdateWithExisting 處理 prepare 員工 import update with existing 的服務流程。
func (c HRService) prepareEmployeeImportUpdateWithExisting(ctx RequestContext, row EmployeeImportRow, existing Employee, batchErrors []RowError) (Employee, Employee, bool, []RowError, error) {
	candidate, err := c.employeeCreateCandidate(ctx, row.Employee)
	if err != nil {
		errors, ok := employeeImportErrorsFromError(row.RowNumber, err)
		if !ok {
			return Employee{}, Employee{}, false, nil, err
		}
		return Employee{}, Employee{}, false, append(errors, batchErrors...), nil
	}
	next := employeeImportUpdateEmployee(existing, candidate, row.Employee)
	if err := c.ensureEmployeePosition(ctx, &next, true); err != nil {
		errors, ok := employeeImportErrorsFromError(row.RowNumber, err)
		if !ok {
			return Employee{}, Employee{}, false, nil, err
		}
		return Employee{}, Employee{}, false, append(errors, batchErrors...), nil
	}
	if err := c.validateEmployee(ctx, next, "update", employeeValidationImportMinimal); err != nil {
		errors, ok := employeeImportErrorsFromError(row.RowNumber, err)
		if !ok {
			return Employee{}, Employee{}, false, nil, err
		}
		return Employee{}, Employee{}, false, append(errors, batchErrors...), nil
	}
	return next, existing, true, batchErrors, nil
}

// employeeImportExistingByEmployeeNo 處理員工 import existing by 員工 no 的服務流程。
func (c HRService) employeeImportExistingByEmployeeNo(ctx RequestContext, input CreateEmployeeInput) (Employee, bool, error) {
	employeeNo := strings.TrimSpace(input.EmployeeNo)
	if employeeNo == "" {
		return Employee{}, false, nil
	}
	return c.store.GetEmployeeByEmployeeNo(goContext(ctx), ctx.TenantID, employeeNo)
}

// employeeImportUpdateEmployee 處理員工 import update 員工。
func employeeImportUpdateEmployee(existing Employee, candidate Employee, input CreateEmployeeInput) Employee {
	next := existing
	if strings.TrimSpace(input.EmployeeNo) != "" {
		next.EmployeeNo = candidate.EmployeeNo
	}
	if strings.TrimSpace(input.Name) != "" {
		next.Name = candidate.Name
	}
	if strings.TrimSpace(input.CompanyEmail) != "" {
		next.CompanyEmail = candidate.CompanyEmail
	}
	if strings.TrimSpace(input.Phone) != "" {
		next.Phone = candidate.Phone
	}
	if strings.TrimSpace(input.OrgUnitID) != "" {
		next.OrgUnitID = candidate.OrgUnitID
	}
	if strings.TrimSpace(input.ManagerEmployeeID) != "" {
		next.ManagerEmployeeID = candidate.ManagerEmployeeID
	}
	if strings.TrimSpace(input.PositionID) != "" {
		next.PositionID = candidate.PositionID
	}
	if strings.TrimSpace(input.Position) != "" {
		next.Position = candidate.Position
	}
	if strings.TrimSpace(input.Category) != "" {
		next.Category = candidate.Category
	}
	if strings.TrimSpace(input.Status) != "" || strings.TrimSpace(input.EmploymentStatus) != "" {
		next.Status = candidate.Status
		next.EmploymentStatus = candidate.EmploymentStatus
	}
	if strings.TrimSpace(input.HireDate) != "" {
		next.HireDate = candidate.HireDate
	}
	next.BasicInfo = mergeEmployeeImportMap(next.BasicInfo, candidate.BasicInfo)
	next.EmploymentInfo = mergeEmployeeImportMap(next.EmploymentInfo, candidate.EmploymentInfo)
	next.ContactInfo = mergeEmployeeImportMap(next.ContactInfo, candidate.ContactInfo)
	next.UpdatedAt = candidate.UpdatedAt
	next.Category = normalizeEmployeeCategory(next.Category)
	if next.EmploymentStatus == "" {
		next.EmploymentStatus = next.Status
	}
	if next.Status == "" {
		next.Status = next.EmploymentStatus
	}
	next.EmploymentStatus = normalizeEmployeeStatus(next.EmploymentStatus)
	next.Status = normalizeEmployeeStatus(next.Status)
	return next
}

// mergeEmployeeImportMap 合併員工 import map。
func mergeEmployeeImportMap(existing map[string]any, updates map[string]any) map[string]any {
	out := utils.CopyStringMap(existing)
	if out == nil {
		out = map[string]any{}
	}
	for key, value := range updates {
		if strings.TrimSpace(fmt.Sprint(value)) == "" {
			continue
		}
		out[key] = value
	}
	return out
}

// validateEmployeeImportRow 驗證員工 import 列的服務流程。
func (c HRService) validateEmployeeImportRow(ctx RequestContext, row EmployeeImportRow, batch *employeeImportBatchIndex) ([]RowError, error) {
	employee, err := c.employeeCreateCandidate(ctx, row.Employee)
	if err != nil {
		errors, ok := employeeImportErrorsFromError(row.RowNumber, err)
		if ok {
			return append(errors, employeeImportBatchErrors(row, batch)...), nil
		}
		return nil, err
	}
	err = c.validateEmployee(ctx, employee, "create", employeeValidationImportMinimal)
	errors, ok := employeeImportErrorsFromError(row.RowNumber, err)
	if err != nil && !ok {
		return nil, err
	}
	errors = append(errors, employeeImportAccountPolicyErrors(row)...)
	errors = append(errors, employeeImportBatchErrors(row, batch)...)
	return errors, nil
}

// employeeImportAccountPolicyErrors 處理員工 import 帳號政策錯誤。
func employeeImportAccountPolicyErrors(row EmployeeImportRow) []RowError {
	policy := strings.TrimSpace(row.Employee.AccountPolicy)
	if policy == "" || validEmployeeAccountPolicy(policy) {
		return nil
	}
	return []RowError{{Row: row.RowNumber, Field: "account_policy", Code: "invalid", Message: "account_policy must be one of none, link_existing, create_pending_invite, create_active"}}
}

// employeeImportBatchErrors 處理員工 import 批次錯誤。
func employeeImportBatchErrors(row EmployeeImportRow, batch *employeeImportBatchIndex) []RowError {
	if batch == nil {
		return nil
	}
	errors := make([]RowError, 0, 3)
	if employeeNo := strings.TrimSpace(row.Employee.EmployeeNo); employeeNo != "" {
		if firstRow, ok := batch.employeeNos[employeeNo]; ok {
			errors = append(errors, RowError{Row: row.RowNumber, Field: "employee_no", Code: "duplicate_in_file", Message: fmt.Sprintf("employee_no is duplicated with row %d", firstRow)})
		} else {
			batch.employeeNos[employeeNo] = row.RowNumber
		}
	}
	if email := strings.ToLower(strings.TrimSpace(row.Employee.CompanyEmail)); email != "" {
		if firstRow, ok := batch.companyEmails[email]; ok {
			errors = append(errors, RowError{Row: row.RowNumber, Field: "company_email", Code: "duplicate_in_file", Message: fmt.Sprintf("company_email is duplicated with row %d", firstRow)})
		} else {
			batch.companyEmails[email] = row.RowNumber
		}
	}
	if accountID := strings.TrimSpace(row.Employee.AccountID); accountID != "" {
		if firstRow, ok := batch.accountIDs[accountID]; ok {
			errors = append(errors, RowError{Row: row.RowNumber, Field: "account_id", Code: "duplicate_in_file", Message: fmt.Sprintf("account_id is duplicated with row %d", firstRow)})
		} else {
			batch.accountIDs[accountID] = row.RowNumber
		}
	}
	return errors
}

// employeeImportErrorsFromError 處理員工 import 錯誤 來源 錯誤。
func employeeImportErrorsFromError(row int, err error) ([]RowError, bool) {
	if err == nil {
		return nil, true
	}
	appErr, ok := AsAppError(err)
	if !ok || appErr.Status >= 500 {
		return nil, false
	}
	if len(appErr.RowErrors) > 0 {
		return appErr.RowErrors, true
	}
	if len(appErr.FieldErrors) > 0 {
		out := make([]RowError, 0, len(appErr.FieldErrors))
		for _, field := range appErr.FieldErrors {
			out = append(out, RowError{Row: row, Field: field.Field, Code: field.Code, Message: field.Message})
		}
		return out, true
	}
	return []RowError{{Row: row, Code: appErr.Code, Message: appErr.Message}}, true
}

// firstRowErrorMessage 取得第一個列錯誤 message。
func firstRowErrorMessage(errors []RowError) string {
	if len(errors) == 0 {
		return "employee import row failed"
	}
	return errors[0].Message
}

const maxEmployeeExportRows = 5000

// ExportEmployeesCSV 匯出員工 CSV 的服務流程。
func (c HRService) ExportEmployeesCSV(ctx RequestContext, query EmployeeQuery) ([]byte, string, error) {
	query = normalizeEmployeeQuery(query)
	items, decision, err := c.exportEmployees(ctx, query)
	if err != nil {
		return nil, "", err
	}
	if len(items) > maxEmployeeExportRows {
		return nil, "", employeeExportLimitError()
	}
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&buf)
	columns := employeeExportColumnsForPolicy(decision.FieldPolicies)
	headers := make([]string, 0, len(columns))
	for _, column := range columns {
		headers = append(headers, column.header)
	}
	_ = w.Write(headers)
	for _, item := range items {
		record := make([]string, 0, len(columns))
		for _, column := range columns {
			record = append(record, neutralizeSpreadsheetCell(column.value(item)))
		}
		_ = w.Write(record)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "employees.csv", nil
}

// neutralizeSpreadsheetCell 處理 neutralize spreadsheet 儲存格。
func neutralizeSpreadsheetCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}

// rejectOversizedEmployeeExport 駁回 oversized 員工 export 的服務流程。
func (c HRService) rejectOversizedEmployeeExport(ctx RequestContext, query EmployeeQuery) error {
	total, err := c.store.CountEmployeesByQuery(goContext(ctx), ctx.TenantID, query)
	if err != nil {
		return err
	}
	if total > maxEmployeeExportRows {
		return employeeExportLimitError()
	}
	return nil
}

// employeeExportLimitError 處理員工 export 限制錯誤。
func employeeExportLimitError() error {
	return Conflict(fmt.Sprintf("employee export exceeds synchronous limit of %d rows; use async export job", maxEmployeeExportRows))
}

// BatchDeleteEmployees 處理批次 delete 員工的服務流程。
func (c HRService) BatchDeleteEmployees(ctx RequestContext, input BatchDeleteEmployeesInput) (BatchEmployeeResponse, error) {
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return BatchEmployeeResponse{}, BadRequest("reason is required")
	}
	ids := uniqueStrings(input.EmployeeIDs)
	if len(ids) == 0 {
		return BatchEmployeeResponse{}, BadRequest("employee_ids is required")
	}
	account, collectionDecision, _, err := c.Authorize(ctx,
		CheckRequest{
			ApplicationCode: AppHR,
			ResourceType:    ResourceEmployee,
			Action:          ActionDelete,
			Context: map[string]any{"filters": map[string]any{
				"employee_ids": ids,
			}},
		},
		AuditTarget{Event: "hr.employee.batch_delete", Resource: string(ResourceEmployeeCollection)},
	)
	if err != nil {
		return BatchEmployeeResponse{}, err
	}
	checks := make([]CheckRequest, 0, len(ids))
	for _, id := range ids {
		checks = append(checks, CheckRequest{
			ApplicationCode:  AppHR,
			ResourceType:     ResourceEmployee,
			ResourceID:       id,
			TargetEmployeeID: id,
			Action:           ActionDelete,
		})
	}
	batch, err := c.Authz().BatchCheck(ctx, BatchCheckRequest{Checks: checks})
	if err != nil {
		return BatchEmployeeResponse{}, err
	}
	if len(batch.Results) != len(ids) {
		return BatchEmployeeResponse{}, fmt.Errorf("authz batch-check returned %d results for %d employees", len(batch.Results), len(ids))
	}
	results := make([]BatchEmployeeResult, 0, len(ids))
	for i, id := range ids {
		decision := batch.Results[i]
		if !decision.Allowed {
			results = append(results, BatchEmployeeResult{EmployeeID: id, Success: false, Code: authzReasonCode(decision), Message: decision.Reason})
			continue
		}
		employee, accountDisabled, err := c.deleteEmployeeWithDecision(ctx, account, decision, id)
		if err != nil {
			results = append(results, BatchEmployeeResult{EmployeeID: id, Success: false, Code: errorCode(err), Message: err.Error()})
			continue
		}
		action := "soft_deleted"
		if accountDisabled {
			action = "soft_deleted_account_disabled"
		}
		results = append(results, BatchEmployeeResult{EmployeeID: employee.ID, Success: true, Action: action})
	}
	succeeded := 0
	for _, result := range results {
		if result.Success {
			succeeded++
		}
	}
	auditDetails := auditDecisionDetails(ctx, collectionDecision, map[string]any{
		"reason":                 reason,
		"result":                 batchEmployeeAuditResult(succeeded, len(results)),
		"requested_employee_ids": ids,
		"succeeded_employee_ids": batchEmployeeResultIDs(results, true),
		"failed_employee_ids":    batchEmployeeResultIDs(results, false),
		"row_count":              len(results),
		"results":                results,
	})
	if err := c.audit(ctx, "hr.employee.batch_delete", string(ResourceEmployeeCollection), "", string(SeverityHigh), auditDetails); err != nil {
		return BatchEmployeeResponse{}, err
	}
	c.logWarn(ctx, "employee batch delete completed",
		"requested_count", len(uniqueStrings(input.EmployeeIDs)),
		"succeeded_count", succeeded,
		"failed_count", len(results)-succeeded,
		"reason_present", strings.TrimSpace(input.Reason) != "",
	)
	return BatchEmployeeResponse{Results: results}, nil
}

// batchEmployeeAuditResult 處理批次員工稽核結果。
func batchEmployeeAuditResult(succeeded, total int) string {
	switch {
	case total <= 0 || succeeded == 0:
		return "failed"
	case succeeded < total:
		return "partial_success"
	default:
		return "success"
	}
}

// deleteEmployeeWithDecision 刪除員工 with 決策的服務流程。
func (c HRService) deleteEmployeeWithDecision(ctx RequestContext, account Account, decision CheckResult, id string) (Employee, bool, error) {
	var employee Employee
	accountDisabled := false
	if err := c.withTransaction(ctx, func(tx HRService) error {
		next, ok, err := tx.store.GetEmployee(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("employee", id)
		}
		if employeeStatus(next) == string(EmployeeStatusDeleted) {
			return Conflict("employee is already deleted")
		}
		visible, err := tx.filterEmployeesByDecision(ctx, account, []Employee{next}, decision)
		if err != nil {
			return err
		}
		if len(visible) == 0 {
			return forbiddenDataScope("employee is outside data scope")
		}
		before := next
		next.Status = string(EmployeeStatusDeleted)
		next.EmploymentStatus = string(EmployeeStatusDeleted)
		next.UpdatedAt = tx.Now()
		next = tx.appendHistoryForChangedEmployment(before, next, "刪除")
		if err := tx.store.UpsertEmployee(goContext(ctx), next); err != nil {
			return err
		}
		if next.AccountID != "" {
			linkedAccount, ok, err := tx.store.GetAccount(goContext(ctx), ctx.TenantID, next.AccountID)
			if err != nil {
				return err
			}
			if ok {
				beforeAccount := linkedAccount
				linkedAccount.Status = string(AccountStatusDisabled)
				if err := tx.store.UpsertAccount(goContext(ctx), linkedAccount); err != nil {
					return err
				}
				if err := tx.Service.syncAccountTenantMembershipTuple(ctx, beforeAccount, linkedAccount); err != nil {
					return err
				}
				accountDisabled = true
			}
		}
		if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeOffboarded), next.ID, map[string]any{"employee_id": next.ID, "status": string(EmployeeStatusDeleted)}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.delete", string(ResourceEmployee), next.ID, string(SeverityHigh), auditDecisionDetails(ctx, decision, map[string]any{
			"previous_status":  employeeStatus(before),
			"status":           string(EmployeeStatusDeleted),
			"account_disabled": accountDisabled,
		})); err != nil {
			return err
		}
		employee = next
		return nil
	}); err != nil {
		return Employee{}, false, err
	}
	return employee, accountDisabled, nil
}

// batchEmployeeResultIDs 處理批次員工結果 IDs。
func batchEmployeeResultIDs(results []BatchEmployeeResult, success bool) []string {
	ids := make([]string, 0)
	for _, result := range results {
		if result.Success == success {
			ids = append(ids, result.EmployeeID)
		}
	}
	return ids
}

// InviteEmployee 邀請員工的服務流程。
func (c HRService) InviteEmployee(ctx RequestContext, id string, input InviteEmployeeInput) (Employee, error) {
	account, decision, audit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: id, Action: ActionInvite},
		AuditTarget{Resource: string(ResourceEmployee), Target: id},
	)
	if err != nil {
		return Employee{}, err
	}
	var employee Employee
	inviteAccountID := ""
	if err := c.withTransaction(ctx, func(tx HRService) error {
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
		switch employeeStatus(next) {
		case string(EmployeeStatusDeleted), string(EmployeeStatusResigned):
			return Conflict("deleted or resigned employee cannot be invited")
		}
		before := next
		email := strings.TrimSpace(input.Email)
		if email == "" {
			email = next.CompanyEmail
		}
		if email == "" {
			return BadRequest("invite email is required")
		}
		accountID := next.AccountID
		if accountID == "" {
			accountID = utils.NewID("acct")
		}
		if err := tx.ensureAccountEmailAvailableForAccount(ctx, email, accountID); err != nil {
			return err
		}
		inviteAccount := Account{
			ID:          accountID,
			TenantID:    ctx.TenantID,
			DisplayName: next.Name,
			Email:       email,
			EmployeeID:  next.ID,
			Status:      string(AccountStatusPendingInvite),
			CreatedAt:   tx.Now(),
		}
		beforeAccount := Account{}
		existing, ok, err := tx.store.GetAccount(goContext(ctx), ctx.TenantID, accountID)
		if err != nil {
			return err
		}
		if ok {
			beforeAccount = existing
			inviteAccount = existing
			inviteAccount.Email = email
			inviteAccount.EmployeeID = next.ID
			inviteAccount.Status = string(AccountStatusPendingInvite)
		}
		if err := tx.store.UpsertAccount(goContext(ctx), inviteAccount); err != nil {
			return err
		}
		if err := tx.Service.syncAccountTenantMembershipTuple(ctx, beforeAccount, inviteAccount); err != nil {
			return err
		}
		next.AccountID = inviteAccount.ID
		next.EmploymentStatus = utils.FirstNonEmpty(next.EmploymentStatus, string(EmployeeStatusOnboarding))
		next.Status = utils.FirstNonEmpty(next.Status, next.EmploymentStatus)
		next.UpdatedAt = tx.Now()
		if err := tx.store.UpsertEmployee(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.touchEmployeeAuthzIfNeeded(ctx, before, next, string(EventEmployeeAuthzSubjectInvite)); err != nil {
			return err
		}
		if err := tx.provisionEmployeeAccountIdentity(ctx, next, inviteAccount, true); err != nil {
			return err
		}
		if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeInvited), next.ID, map[string]any{"employee_id": next.ID, "account_id": inviteAccount.ID}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.invite", string(ResourceEmployee), next.ID, string(SeverityHigh), auditDecisionDetails(ctx, decision, map[string]any{"email": email, "account_id": inviteAccount.ID})); err != nil {
			return err
		}
		if err := audit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		inviteAccountID = inviteAccount.ID
		employee = next
		return nil
	}); err != nil {
		return Employee{}, err
	}
	c.runIdentityProvisioningFastPath(ctx)
	c.logInfo(ctx, "employee invitation created",
		"employee_id", employee.ID,
		"employee_no", employee.EmployeeNo,
		"account_id", inviteAccountID,
		"status", employeeStatus(employee),
	)
	return employee, nil
}

// TransitionEmployeeStatus 轉換員工狀態的服務流程。
func (c HRService) TransitionEmployeeStatus(ctx RequestContext, id string, input StatusTransitionInput) (Employee, error) {
	status := normalizeEmployeeStatus(input.Status)
	if status == "" {
		return Employee{}, BadRequest("status is required")
	}
	if status == string(EmployeeStatusDeleted) {
		return Employee{}, BadRequest("deleted status requires delete")
	}
	if !validEmployeeStatus(status, false) {
		return Employee{}, BadRequest("invalid employee status")
	}
	account, decision, audit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: id, Action: ActionStatusTransition},
		AuditTarget{Resource: string(ResourceEmployee), Target: id},
	)
	if err != nil {
		return Employee{}, err
	}
	var employee Employee
	previousStatus := ""
	transitionType := ""
	if err := c.withTransaction(ctx, func(tx HRService) error {
		next, ok, err := tx.store.GetEmployee(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("employee", id)
		}
		currentStatus := employeeStatus(next)
		previousStatus = currentStatus
		reinstating := isEmployeeReinstatement(currentStatus, status)
		if err := ensureEmployeeStatusTransitionWithOptions(currentStatus, status, true); err != nil {
			return err
		}
		visible, err := tx.filterEmployeesByDecision(ctx, account, []Employee{next}, decision)
		if err != nil {
			return err
		}
		if len(visible) == 0 {
			return forbiddenDataScope("employee is outside data scope")
		}
		var transitionStart *time.Time
		switch status {
		case string(EmployeeStatusLeaveSuspended):
			fields := make([]FieldError, 0, 3)
			if strings.TrimSpace(input.Reason) == "" {
				fields = append(fields, FieldError{Tab: "employment_info", Field: "reason", Code: "required", Message: "reason is required"})
			}
			if strings.TrimSpace(input.StartDate) == "" {
				fields = append(fields, FieldError{Tab: "employment_info", Field: "start_date", Code: "required", Message: "start_date is required"})
			}
			if strings.TrimSpace(input.EndDate) == "" {
				fields = append(fields, FieldError{Tab: "employment_info", Field: "end_date", Code: "required", Message: "end_date is required"})
			}
			if len(fields) > 0 {
				return domainValidation("leave suspension requires reason, start_date and end_date", fields...)
			}
			start, err := utils.ParseDate(input.StartDate)
			if err != nil {
				return BadRequest("start_date must be RFC3339 or YYYY-MM-DD")
			}
			end, err := utils.ParseDate(input.EndDate)
			if err != nil {
				return BadRequest("end_date must be RFC3339 or YYYY-MM-DD")
			}
			if end.Before(start) {
				return domainValidation("leave suspension date range is invalid", FieldError{Tab: "employment_info", Field: "end_date", Code: "invalid", Message: "end_date must be on or after start_date"})
			}
			transitionStart = &start
		case string(EmployeeStatusResigned):
			if strings.TrimSpace(input.EndDate) == "" || strings.TrimSpace(input.Reason) == "" {
				return domainValidation("resignation requires end_date and reason", FieldError{Tab: "employment_info", Field: "end_date", Code: "required", Message: "end_date is required"}, FieldError{Tab: "employment_info", Field: "reason", Code: "required", Message: "reason is required"})
			}
			resignDate, err := utils.ParseDate(input.EndDate)
			if err != nil {
				return BadRequest("end_date must be RFC3339 or YYYY-MM-DD")
			}
			if next.HireDate != nil && resignDate.Before(*next.HireDate) {
				return domainValidation("resignation date range is invalid", FieldError{Tab: "employment_info", Field: "end_date", Code: "invalid", Message: "end_date must be on or after hire_date"})
			}
			next.ResignDate = &resignDate
			if next.AccountID != "" {
				linkedAccount, ok, err := tx.store.GetAccount(goContext(ctx), ctx.TenantID, next.AccountID)
				if err != nil {
					return err
				}
				if ok {
					beforeAccount := linkedAccount
					linkedAccount.Status = string(AccountStatusDisabled)
					if err := tx.store.UpsertAccount(goContext(ctx), linkedAccount); err != nil {
						return err
					}
					if err := tx.Service.syncAccountTenantMembershipTuple(ctx, beforeAccount, linkedAccount); err != nil {
						return err
					}
				}
			}
		}
		if reinstating {
			if strings.TrimSpace(input.StartDate) == "" || strings.TrimSpace(input.Reason) == "" {
				return domainValidation("reinstatement requires start_date and reason", FieldError{Tab: "employment_info", Field: "start_date", Code: "required", Message: "start_date is required"}, FieldError{Tab: "employment_info", Field: "reason", Code: "required", Message: "reason is required"})
			}
			start, err := utils.ParseDate(input.StartDate)
			if err != nil {
				return BadRequest("start_date must be RFC3339 or YYYY-MM-DD")
			}
			if next.ResignDate != nil && start.Before(*next.ResignDate) {
				return domainValidation("reinstatement date range is invalid", FieldError{Tab: "employment_info", Field: "start_date", Code: "invalid", Message: "start_date must be on or after resign_date"})
			}
			transitionStart = &start
			next.ResignDate = nil
			if next.AccountID != "" {
				linkedAccount, ok, err := tx.store.GetAccount(goContext(ctx), ctx.TenantID, next.AccountID)
				if err != nil {
					return err
				}
				if ok {
					beforeAccount := linkedAccount
					linkedAccount.Status = string(AccountStatusActive)
					if err := tx.store.UpsertAccount(goContext(ctx), linkedAccount); err != nil {
						return err
					}
					if err := tx.Service.syncAccountTenantMembershipTuple(ctx, beforeAccount, linkedAccount); err != nil {
						return err
					}
				}
			}
		}
		before := next
		next.Status = status
		next.EmploymentStatus = status
		next.UpdatedAt = tx.Now()
		next.EmploymentInfo = mergeMap(next.EmploymentInfo, input.Details)
		if next.EmploymentInfo == nil {
			next.EmploymentInfo = map[string]any{}
		}
		if reinstating {
			delete(next.EmploymentInfo, "resign_date")
			delete(next.EmploymentInfo, "resign_reason")
		}
		next.EmploymentInfo["transition_reason"] = input.Reason
		next.EmploymentInfo["transition_start_date"] = input.StartDate
		next.EmploymentInfo["transition_end_date"] = input.EndDate
		next.EmploymentInfo["transition_type"] = employeeTransitionType(currentStatus, status)
		next = tx.appendHistoryForChangedEmployment(before, next, input.Reason)
		if reinstating && transitionStart != nil && len(next.InternalExperiences) > 0 {
			next.InternalExperiences[len(next.InternalExperiences)-1].StartDate = transitionStart
		}
		if err := tx.store.UpsertEmployee(goContext(ctx), next); err != nil {
			return err
		}
		eventType := string(EventEmployeeStatusChanged)
		if status == string(EmployeeStatusResigned) {
			eventType = string(EventEmployeeOffboarded)
		} else if reinstating {
			eventType = string(EventEmployeeReinstated)
		}
		if err := tx.appendEmployeeEvent(ctx, eventType, next.ID, map[string]any{"employee_id": next.ID, "status": status, "reason": input.Reason}); err != nil {
			return err
		}
		if err := tx.audit(ctx, employeeStatusTransitionAuditAction(currentStatus, status), string(ResourceEmployee), next.ID, string(SeverityHigh), auditDecisionDetails(ctx, decision, map[string]any{
			"previous_status": currentStatus,
			"status":          status,
			"transition_type": employeeTransitionType(currentStatus, status),
			"reason":          input.Reason,
			"start_date":      input.StartDate,
			"end_date":        input.EndDate,
		})); err != nil {
			return err
		}
		if err := audit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		transitionType = employeeTransitionType(currentStatus, status)
		employee = next
		return nil
	}); err != nil {
		return Employee{}, err
	}
	c.logWarn(ctx, "employee status transitioned",
		"employee_id", employee.ID,
		"employee_no", employee.EmployeeNo,
		"previous_status", previousStatus,
		"status", employeeStatus(employee),
		"transition_type", transitionType,
	)
	return employee, nil
}

// errorCode 處理錯誤碼。
func errorCode(err error) string {
	if appErr, ok := AsAppError(err); ok {
		return appErr.Code
	}
	return "error"
}

// ensureEmployeeStatusTransition 確保員工狀態轉換。
func ensureEmployeeStatusTransition(current, next string) error {
	return ensureEmployeeStatusTransitionWithOptions(current, next, false)
}

// ensureEmployeeStatusTransitionWithOptions 確保員工狀態轉換 with 選項。
func ensureEmployeeStatusTransitionWithOptions(current, next string, allowReinstatement bool) error {
	current = normalizeEmployeeStatus(current)
	next = normalizeEmployeeStatus(next)
	switch current {
	case string(EmployeeStatusDeleted):
		if current != "" && current != next {
			return Conflict("terminal employee status cannot be changed")
		}
	case string(EmployeeStatusResigned):
		if current != "" && current != next && !(allowReinstatement && isEmployeeReinstatement(current, next)) {
			return Conflict("terminal employee status cannot be changed")
		}
	}
	return nil
}

// isEmployeeReinstatement 判斷是否為員工 reinstatement。
func isEmployeeReinstatement(current, next string) bool {
	current = normalizeEmployeeStatus(current)
	next = normalizeEmployeeStatus(next)
	if current != string(EmployeeStatusResigned) {
		return false
	}
	switch next {
	case string(EmployeeStatusActive), string(EmployeeStatusProbation), string(EmployeeStatusOnboarding):
		return true
	default:
		return false
	}
}

// employeeTransitionType 處理員工轉換 type。
func employeeTransitionType(current, next string) string {
	current = normalizeEmployeeStatus(current)
	next = normalizeEmployeeStatus(next)
	switch {
	case isEmployeeReinstatement(current, next):
		return "reinstatement"
	case next == string(EmployeeStatusResigned):
		return "resignation"
	case next == string(EmployeeStatusLeaveSuspended):
		return "leave_suspension"
	default:
		return "status_change"
	}
}

// employeeStatusTransitionAuditAction 處理員工狀態轉換稽核 action。
func employeeStatusTransitionAuditAction(current, next string) string {
	switch employeeTransitionType(current, next) {
	case "reinstatement":
		return "hr.employee.reinstate"
	case "resignation":
		return "hr.employee.offboard"
	case "leave_suspension":
		return "hr.employee.leave_suspend"
	default:
		return "hr.employee.status_transition"
	}
}

const (
	employeeOwnerRelation   = "owner"
	employeeManagerRelation = "manager"
)

// syncEmployeeRelationshipTuples 同步員工關係 tuple 的服務流程。
func (c HRService) syncEmployeeRelationshipTuples(ctx RequestContext, before, after Employee) error {
	changes, err := c.employeeRelationshipTupleChanges(ctx, before, after)
	if err != nil {
		return err
	}
	for _, change := range changes {
		if err := c.applyAuthzRelationshipTupleChange(ctx, change); err != nil {
			return err
		}
	}
	return nil
}

// employeeRelationshipTupleChanges 處理員工關係 tuple changes 的服務流程。
func (c HRService) employeeRelationshipTupleChanges(ctx RequestContext, before, after Employee) ([]domain.AuthzRelationshipTupleChange, error) {
	now := c.Now()
	objectType := openFGATypeEmployee
	objectID := strings.TrimSpace(after.ID)
	if objectID == "" {
		objectID = strings.TrimSpace(before.ID)
	}
	changes := make([]domain.AuthzRelationshipTupleChange, 0, 8)
	add := func(operation domain.AuthzRelationshipTupleOperation, tupleObjectType, tupleObjectID, relation, subjectType, subjectID string) {
		if strings.TrimSpace(subjectID) == "" || strings.TrimSpace(tupleObjectID) == "" {
			return
		}
		changes = append(changes, domain.AuthzRelationshipTupleChange{
			Operation: operation,
			Tuple: domain.AuthzRelationshipTuple{
				ID:          utils.NewID("rel"),
				TenantID:    ctx.TenantID,
				ObjectType:  tupleObjectType,
				ObjectID:    tupleObjectID,
				Relation:    relation,
				SubjectType: subjectType,
				SubjectID:   subjectID,
				CreatedAt:   now,
			},
		})
	}

	beforeAccountID := strings.TrimSpace(before.AccountID)
	afterAccountID := strings.TrimSpace(after.AccountID)
	if beforeAccountID != "" && beforeAccountID != afterAccountID {
		add(domain.AuthzRelationshipTupleDelete, objectType, objectID, employeeOwnerRelation, openFGASubjectTypeAccount, beforeAccountID)
	}
	if afterAccountID != "" {
		if _, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, afterAccountID); err != nil {
			return nil, err
		} else if ok {
			add(domain.AuthzRelationshipTupleWrite, objectType, objectID, employeeOwnerRelation, openFGASubjectTypeAccount, afterAccountID)
		}
	}

	beforeOrgUnitID := strings.TrimSpace(before.OrgUnitID)
	afterOrgUnitID := strings.TrimSpace(after.OrgUnitID)
	if beforeOrgUnitID != "" && beforeOrgUnitID != afterOrgUnitID {
		add(domain.AuthzRelationshipTupleDelete, objectType, objectID, openFGARelationEmployeeOrg, openFGASubjectTypeOrgUnit, beforeOrgUnitID)
	}
	if afterOrgUnitID != "" {
		add(domain.AuthzRelationshipTupleWrite, objectType, objectID, openFGARelationEmployeeOrg, openFGASubjectTypeOrgUnit, afterOrgUnitID)
	}
	if beforeOrgUnitID != "" && beforeAccountID != "" && (beforeOrgUnitID != afterOrgUnitID || beforeAccountID != afterAccountID) {
		add(domain.AuthzRelationshipTupleDelete, openFGATypeOrgUnit, beforeOrgUnitID, openFGARelationOrgUnitMember, openFGASubjectTypeAccount, beforeAccountID)
	}
	if afterOrgUnitID != "" && afterAccountID != "" {
		if _, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, afterAccountID); err != nil {
			return nil, err
		} else if ok {
			add(domain.AuthzRelationshipTupleWrite, openFGATypeOrgUnit, afterOrgUnitID, openFGARelationOrgUnitMember, openFGASubjectTypeAccount, afterAccountID)
		}
	}

	beforeManagerAccountID, err := c.employeeAccountID(ctx, before.ManagerEmployeeID)
	if err != nil {
		return nil, err
	}
	afterManagerAccountID, err := c.employeeAccountID(ctx, after.ManagerEmployeeID)
	if err != nil {
		return nil, err
	}
	if beforeManagerAccountID != "" && beforeManagerAccountID != afterManagerAccountID {
		add(domain.AuthzRelationshipTupleDelete, objectType, objectID, employeeManagerRelation, openFGASubjectTypeAccount, beforeManagerAccountID)
	}
	if afterManagerAccountID != "" {
		add(domain.AuthzRelationshipTupleWrite, objectType, objectID, employeeManagerRelation, openFGASubjectTypeAccount, afterManagerAccountID)
	}

	return dedupeRelationshipTupleChanges(changes), nil
}

// employeeAccountID 處理員工帳號 ID 的服務流程。
func (c HRService) employeeAccountID(ctx RequestContext, employeeID string) (string, error) {
	employeeID = strings.TrimSpace(employeeID)
	if employeeID == "" {
		return "", nil
	}
	employee, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, employeeID)
	if err != nil || !ok {
		return "", err
	}
	return strings.TrimSpace(employee.AccountID), nil
}

// applyAuthzRelationshipTupleChange 處理 apply 授權關係 tuple change 的服務流程。
func (c HRService) applyAuthzRelationshipTupleChange(ctx RequestContext, change domain.AuthzRelationshipTupleChange) error {
	return c.Service.applyRelationshipTupleChange(ctx, change)
}

// normalizeAuthzRelationshipTuple 正規化授權關係 tuple。
func normalizeAuthzRelationshipTuple(ctx RequestContext, tuple domain.AuthzRelationshipTuple, now time.Time) domain.AuthzRelationshipTuple {
	tuple.TenantID = utils.FirstNonEmpty(strings.TrimSpace(tuple.TenantID), ctx.TenantID)
	tuple.ObjectType = strings.TrimSpace(tuple.ObjectType)
	tuple.ObjectID = strings.TrimSpace(tuple.ObjectID)
	tuple.Relation = strings.TrimSpace(tuple.Relation)
	tuple.SubjectType = strings.TrimSpace(tuple.SubjectType)
	tuple.SubjectID = strings.TrimSpace(tuple.SubjectID)
	if tuple.ID == "" {
		tuple.ID = utils.NewID("rel")
	}
	if tuple.CreatedAt.IsZero() {
		tuple.CreatedAt = now
	}
	return tuple
}

// relationshipOutboxEventType 處理關係 outbox 事件 type。
func relationshipOutboxEventType(operation domain.AuthzRelationshipTupleOperation) string {
	switch operation {
	case domain.AuthzRelationshipTupleDelete:
		return string(domain.EventOpenFGARelationshipDelete)
	default:
		return string(domain.EventOpenFGARelationshipWrite)
	}
}

// relationshipTuplePayload 處理關係 tuple payload。
func relationshipTuplePayload(operation domain.AuthzRelationshipTupleOperation, tuple domain.AuthzRelationshipTuple) map[string]any {
	return map[string]any{
		"operation":    string(operation),
		"object_type":  tuple.ObjectType,
		"object_id":    tuple.ObjectID,
		"relation":     tuple.Relation,
		"subject_type": tuple.SubjectType,
		"subject_id":   tuple.SubjectID,
	}
}

// dedupeRelationshipTupleChanges 處理 dedupe 關係 tuple changes。
func dedupeRelationshipTupleChanges(changes []domain.AuthzRelationshipTupleChange) []domain.AuthzRelationshipTupleChange {
	out := make([]domain.AuthzRelationshipTupleChange, 0, len(changes))
	seen := map[string]struct{}{}
	for _, change := range changes {
		key := string(change.Operation) + "\x00" + change.Tuple.ObjectType + "\x00" + change.Tuple.ObjectID + "\x00" + change.Tuple.Relation + "\x00" + change.Tuple.SubjectType + "\x00" + change.Tuple.SubjectID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, change)
	}
	return out
}
