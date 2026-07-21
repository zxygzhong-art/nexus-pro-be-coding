package service

import (
	"nexus-pro-api/internal/utils"
	"strings"
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
	units = inheritClosedOrgUnitState(units)
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
			ID:             utils.NewID("ou"),
			TenantID:       ctx.TenantID,
			Code:           strings.TrimSpace(input.Code),
			Name:           strings.TrimSpace(input.Name),
			ParentID:       strings.TrimSpace(input.ParentID),
			ShowInOrgChart: true,
			CreatedAt:      tx.Now(),
			UpdatedAt:      tx.Now(),
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
			units, err := tx.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
			if err != nil {
				return err
			}
			next.Closed = orgUnitHasClosedAncestor(next, units)
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

// UpdateOrgUnit 更新組織單位的服務流程。
func (c HRService) UpdateOrgUnit(ctx RequestContext, id string, input UpdateOrgUnitInput) (OrgUnit, error) {
	orgUnitID := strings.TrimSpace(id)
	if orgUnitID == "" {
		return OrgUnit{}, BadRequest("org unit id is required")
	}
	if _, _, err := c.Service.requireServiceAuthz(ctx, AppHR, ResourceOrgUnit, ActionUpdate, orgUnitID); err != nil {
		return OrgUnit{}, err
	}
	var unit OrgUnit
	if err := c.withTransaction(ctx, func(tx HRService) error {
		current, ok, err := tx.store.GetOrgUnit(goContext(ctx), ctx.TenantID, orgUnitID)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("org unit", orgUnitID)
		}
		before := current
		next := current
		next.UpdatedAt = tx.Now()
		if input.Code != nil {
			next.Code = strings.TrimSpace(*input.Code)
		}
		if input.Name != nil {
			name := strings.TrimSpace(*input.Name)
			if name == "" {
				return BadRequest("org unit name is required")
			}
			next.Name = name
		}
		parentChanged := false
		if input.ParentID != nil {
			parentID := strings.TrimSpace(*input.ParentID)
			if parentID == next.ID {
				return BadRequest("org unit cannot be its own parent")
			}
			if parentID != next.ParentID {
				parentChanged = true
				next.ParentID = parentID
			}
		}
		if input.Closed != nil {
			next.Closed = *input.Closed
		}
		if input.ShowInOrgChart != nil {
			next.ShowInOrgChart = *input.ShowInOrgChart
		}
		if parentChanged {
			if err := tx.rebuildOrgUnitPaths(ctx, &next); err != nil {
				return err
			}
		}
		units, err := tx.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
		if err != nil {
			return err
		}
		if orgUnitHasClosedAncestor(next, units) {
			if input.Closed != nil && !*input.Closed {
				return BadRequest("org unit cannot be reopened while an ancestor is closed")
			}
			next.Closed = true
		}
		if err := tx.store.UpsertOrgUnit(goContext(ctx), next); err != nil {
			return err
		}
		if input.ShowInOrgChart != nil {
			if err := tx.store.UpdateOrgUnitOrgChartVisibility(
				goContext(ctx), ctx.TenantID, next.ID, next.ShowInOrgChart, next.UpdatedAt,
			); err != nil {
				return err
			}
		}
		if parentChanged {
			if err := tx.updateOrgUnitDescendantPaths(ctx, before, next); err != nil {
				return err
			}
		}
		if next.Closed {
			if err := tx.closeOrgUnitDescendants(ctx, next); err != nil {
				return err
			}
		}
		if err := tx.Service.syncOrgUnitRelationshipTuples(ctx, before, next); err != nil {
			return err
		}
		if err := tx.touchAuthzConfig(ctx, "org.unit.upsert", map[string]any{
			"org_unit_id": next.ID,
			"parent_id":   next.ParentID,
		}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "org.unit.update", "org_unit", next.ID, "medium", map[string]any{
			"before_parent_id":         before.ParentID,
			"after_parent_id":          next.ParentID,
			"before_closed":            before.Closed,
			"after_closed":             next.Closed,
			"before_show_in_org_chart": before.ShowInOrgChart,
			"after_show_in_org_chart":  next.ShowInOrgChart,
		}); err != nil {
			return err
		}
		unit = next
		return nil
	}); err != nil {
		return OrgUnit{}, err
	}
	return unit, nil
}

// inheritClosedOrgUnitState 在讀取投影中保證關閉狀態沿祖先鏈向下繼承。
func inheritClosedOrgUnitState(units []OrgUnit) []OrgUnit {
	byID := make(map[string]OrgUnit, len(units))
	for _, unit := range units {
		byID[unit.ID] = unit
	}
	out := make([]OrgUnit, len(units))
	for index, unit := range units {
		current := unit
		seen := map[string]struct{}{unit.ID: {}}
		for parentID := strings.TrimSpace(unit.ParentID); parentID != ""; {
			if _, exists := seen[parentID]; exists {
				break
			}
			seen[parentID] = struct{}{}
			parent, ok := byID[parentID]
			if !ok {
				break
			}
			if parent.Closed {
				current.Closed = true
				break
			}
			parentID = strings.TrimSpace(parent.ParentID)
		}
		out[index] = current
	}
	return out
}

// orgUnitHasClosedAncestor 判斷目標組織單元的任一祖先是否已關閉。
func orgUnitHasClosedAncestor(unit OrgUnit, units []OrgUnit) bool {
	byID := make(map[string]OrgUnit, len(units))
	for _, candidate := range units {
		byID[candidate.ID] = candidate
	}
	for _, ancestorID := range unit.Path {
		if ancestorID == unit.ID {
			continue
		}
		if ancestor, ok := byID[ancestorID]; ok && ancestor.Closed {
			return true
		}
	}
	return false
}

// closeOrgUnitDescendants 將關閉狀態遞歸寫入目標組織單元的全部後代。
func (c HRService) closeOrgUnitDescendants(ctx RequestContext, parent OrgUnit) error {
	units, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
	if err != nil {
		return err
	}
	for _, unit := range units {
		if unit.ID == parent.ID || unit.Closed || !orgUnitPathHasPrefix(unit.Path, parent.Path) {
			continue
		}
		unit.Closed = true
		unit.UpdatedAt = c.Now()
		if err := c.store.UpsertOrgUnit(goContext(ctx), unit); err != nil {
			return err
		}
	}
	return nil
}

func (c HRService) rebuildOrgUnitPaths(ctx RequestContext, unit *OrgUnit) error {
	if unit.ParentID == "" {
		unit.Path = []string{unit.ID}
		return nil
	}
	parent, ok, err := c.store.GetOrgUnit(goContext(ctx), ctx.TenantID, unit.ParentID)
	if err != nil {
		return err
	}
	if !ok {
		return NotFound("org unit", unit.ParentID)
	}
	for _, pathID := range parent.Path {
		if pathID == unit.ID {
			return BadRequest("org unit parent would create a cycle")
		}
	}
	unit.Path = append(utils.CopyStrings(parent.Path), unit.ID)
	return nil
}

func (c HRService) updateOrgUnitDescendantPaths(ctx RequestContext, before, next OrgUnit) error {
	units, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
	if err != nil {
		return err
	}
	oldPrefix := utils.CopyStrings(before.Path)
	newPrefix := utils.CopyStrings(next.Path)
	for _, unit := range units {
		if unit.ID == next.ID {
			continue
		}
		if !orgUnitPathHasPrefix(unit.Path, oldPrefix) {
			continue
		}
		updated := unit
		updated.Path = append(utils.CopyStrings(newPrefix), unit.Path[len(oldPrefix):]...)
		updated.UpdatedAt = c.Now()
		if err := c.store.UpsertOrgUnit(goContext(ctx), updated); err != nil {
			return err
		}
		if err := c.Service.syncOrgUnitRelationshipTuples(ctx, unit, updated); err != nil {
			return err
		}
	}
	return nil
}

func orgUnitPathHasPrefix(path, prefix []string) bool {
	if len(path) < len(prefix) {
		return false
	}
	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}
	return true
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
	decision.FieldPolicies = employeeExportFieldPolicies(decision.FieldPolicies)
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

// employeeExportFieldPolicies 保留欄位拒絕規則，但讓具備匯出權限的公司信箱維持可用原值。
func employeeExportFieldPolicies(policies map[string]string) map[string]string {
	if strings.TrimSpace(policies["company_email"]) != "mask" {
		return policies
	}
	out := make(map[string]string, len(policies)-1)
	for field, effect := range policies {
		if field != "company_email" {
			out[field] = effect
		}
	}
	return out
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
			return ForbiddenDataScope("employee is outside data scope")
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
			return ForbiddenDataScope("employee is outside data scope")
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
	decision.FieldPolicies = employeeDetailFieldPolicies(decision.FieldPolicies)
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
		return Employee{}, ForbiddenDataScope("employee is outside data scope")
	}
	if err := c.auditSensitiveEmployeeRead(ctx, decision, visible, visible[0].ID); err != nil {
		return Employee{}, err
	}
	return visible[0], nil
}

// employeeDetailFieldPolicies 讓授權後的詳情讀取回傳可用原值，同時保留 hide 與 deny 限制。
func employeeDetailFieldPolicies(policies map[string]string) map[string]string {
	if len(policies) == 0 {
		return policies
	}
	out := make(map[string]string, len(policies))
	for field, effect := range policies {
		if strings.TrimSpace(effect) != "mask" {
			out[field] = effect
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
			return ForbiddenDataScope("employee is outside data scope")
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
