package service

import (
	"fmt"
	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
	"strings"
)

func (c *Service) boundaryScopeDecision(ctx RequestContext, account Account, boundary map[string]any) (Scope, map[string]any, bool, error) {
	if len(boundary) == 0 {
		return "", nil, false, nil
	}
	scope, params, ok := boundaryScopeSpec(boundary)
	if !ok {
		return "", nil, false, nil
	}
	if scope == "" {
		scope = ScopeCustomCondition
	}
	conditions, err := c.scopeConditions(ctx, account, scope, params)
	if err != nil {
		return "", nil, false, err
	}
	return scope, conditions, true, nil
}

func boundaryScopeSpec(boundary map[string]any) (Scope, map[string]any, bool) {
	params := map[string]any{}
	if conditions, ok := mapFromAny(boundary["conditions"]); ok {
		for key, value := range conditions {
			params[key] = value
		}
	}
	scope := scopeFromAny(boundary["scope"])
	if scope == "" {
		scope = scopeFromAny(boundary["scope_type"])
	}
	if dataScope, ok := mapFromAny(boundary["data_scope"]); ok {
		if nestedScope := scopeFromAny(dataScope["scope"]); nestedScope != "" {
			scope = nestedScope
		}
		if nestedScope := scopeFromAny(dataScope["scope_type"]); nestedScope != "" {
			scope = nestedScope
		}
		if nestedParams, ok := mapFromAny(dataScope["params"]); ok {
			for key, value := range nestedParams {
				params[key] = value
			}
		}
		for key, value := range dataScope {
			if isBoundaryScopeParamKey(key) {
				params[key] = value
			}
		}
	}
	for key, value := range boundary {
		if isBoundaryScopeParamKey(key) {
			params[key] = value
		}
	}
	if scope == "" && len(params) == 0 {
		return "", nil, false
	}
	return scope, params, true
}

func isBoundaryScopeParamKey(key string) bool {
	if isStructuredScopeConditionKey(key) {
		return true
	}
	switch key {
	case "scope_check", "scope_check_scope":
		return true
	default:
		return false
	}
}

func scopeFromAny(value any) Scope {
	switch v := value.(type) {
	case Scope:
		return v
	case string:
		return Scope(strings.TrimSpace(v))
	default:
		return ""
	}
}

func mapFromAny(value any) (map[string]any, bool) {
	switch v := value.(type) {
	case map[string]any:
		return v, true
	case map[string]string:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = item
		}
		return out, true
	default:
		return nil, false
	}
}

// scopeRank 處理範圍 rank。
func scopeRank(scope Scope) int {
	switch scope {
	case ScopeSystem:
		return 120
	case ScopeAll, ScopeTenant:
		return 100
	case ScopeDepartmentSubtree, ScopeAssignedOrgUnits:
		return 80
	case ScopeDepartment:
		return 70
	case ScopeDirectReports:
		return 60
	case ScopeSelf, ScopeOwn:
		return 40
	case ScopeCustomCondition:
		return 30
	default:
		return 20
	}
}

// scopeConditions 處理範圍 conditions 的服務流程。
func (c *Service) scopeConditions(ctx RequestContext, account Account, scope Scope, params map[string]any) (map[string]any, error) {
	out := utils.CopyStringMap(params)
	if out == nil {
		out = map[string]any{}
	}
	out["tenant_id"] = ctx.TenantID
	switch scope {
	case "", ScopeAll, ScopeTenant:
		return out, nil
	case ScopeSystem:
		if ctx.AssumedRoleSessionID == "" {
			return nil, ForbiddenDataScope("system data scope requires an assumed role session")
		}
		return out, nil
	case ScopeSelf, ScopeOwn:
		if _, ok := out["employee_ids"]; !ok {
			if account.EmployeeID == "" {
				return nil, ForbiddenDataScope("account is not linked to an employee for own scope")
			}
			out["employee_ids"] = []string{account.EmployeeID}
		}
	case ScopeDepartment:
		if _, ok := out["org_unit_ids"]; !ok && account.EmployeeID == "" {
			return nil, ForbiddenDataScope("account is not linked to an employee for department scope")
		}
		if _, ok := out["org_unit_ids"]; !ok && account.EmployeeID != "" {
			employee, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, account.EmployeeID)
			if err != nil {
				return nil, err
			}
			if ok && employee.OrgUnitID != "" {
				out["org_unit_ids"] = []string{employee.OrgUnitID}
			}
		}
		if c.openFGAScopeChecksAvailable() {
			markOpenFGAScopeCheck(out, scope)
		}
	case ScopeDepartmentSubtree:
		if _, ok := out["org_unit_ids"]; !ok && account.EmployeeID == "" {
			return nil, ForbiddenDataScope("account is not linked to an employee for department_subtree scope")
		}
		if _, ok := out["org_unit_ids"]; !ok && account.EmployeeID != "" {
			employee, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, account.EmployeeID)
			if err != nil {
				return nil, err
			}
			if ok && employee.OrgUnitID != "" {
				units, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
				if err != nil {
					return nil, err
				}
				out["org_unit_ids"] = orgUnitIDsInSubtree(units, []string{employee.OrgUnitID})
			}
		}
		if c.openFGAScopeChecksAvailable() {
			markOpenFGAScopeCheck(out, scope)
		}
	case ScopeDirectReports:
		if _, ok := out["employee_ids"]; !ok && account.EmployeeID != "" {
			employees, err := c.listBusinessEmployees(ctx)
			if err != nil {
				return nil, err
			}
			ids := make([]string, 0)
			for _, employee := range employees {
				if employee.ManagerEmployeeID == account.EmployeeID {
					ids = append(ids, employee.ID)
				}
			}
			out["employee_ids"] = ids
		}
	case ScopeAssignedOrgUnits:
		if len(stringSliceFromAny(out["org_unit_ids"])) == 0 {
			return nil, ForbiddenDataScope("assigned_org_units scope requires org_unit_ids")
		}
	case ScopeCustomCondition:
		if !hasEffectiveCustomScopeFilter(out) {
			return nil, ForbiddenDataScope("custom_condition has no supported effective filter")
		}
		out["scope"] = ScopeCustomCondition
	default:
		out["scope"] = scope
	}
	return out, nil
}

// hasEffectiveCustomScopeFilter 判斷 custom scope 是否至少包含一個可執行的限制條件。
func hasEffectiveCustomScopeFilter(conditions map[string]any) bool {
	for _, key := range []string{"employee_ids", "org_unit_ids", "employee_statuses", "statuses"} {
		if len(stringSliceFromAny(conditions[key])) > 0 {
			return true
		}
	}
	return false
}

// openFGAScopeChecksAvailable 判斷 FGA 資料範圍 check 是否可用。
func (c *Service) openFGAScopeChecksAvailable() bool {
	return c != nil && c.openFGAScopeChecks && c.relationships != nil
}

// markOpenFGAScopeCheck 標記決策要走 FGA scope check。
func markOpenFGAScopeCheck(out map[string]any, scope Scope) {
	if out == nil {
		return
	}
	out["scope_check"] = "openfga"
	out["scope_check_scope"] = string(scope)
}

// decisionUsesOpenFGAScopeCheck 判斷決策是否使用 FGA scope check。
func decisionUsesOpenFGAScopeCheck(decision CheckResult) bool {
	if decision.Conditions == nil {
		return false
	}
	return strings.TrimSpace(fmt.Sprint(decision.Conditions["scope_check"])) == "openfga"
}

// orgUnitIDsInSubtree 處理組織單位 IDs in subtree。
func orgUnitIDsInSubtree(units []OrgUnit, roots []string) []string {
	allowed := map[string]struct{}{}
	for _, id := range roots {
		if strings.TrimSpace(id) != "" {
			allowed[id] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return nil
	}
	out := make([]string, 0, len(units))
	for _, unit := range units {
		if orgUnitInScope(units, unit.ID, allowed) {
			out = append(out, unit.ID)
		}
	}
	return uniqueStrings(out)
}

// applyEmployeeDecision 處理 apply 員工決策的服務流程。
func (c HRService) applyEmployeeDecision(ctx RequestContext, account Account, items []Employee, decision CheckResult) ([]Employee, error) {
	filtered, err := c.filterEmployeesByDecision(ctx, account, items, decision)
	if err != nil {
		return nil, err
	}
	out := make([]Employee, 0, len(filtered))
	for _, item := range filtered {
		out = append(out, maskEmployee(item, decision.FieldPolicies))
	}
	return out, nil
}

// filterEmployeesByDecision 處理篩選員工 by 決策的服務流程。
func (c HRService) filterEmployeesByDecision(ctx RequestContext, account Account, items []Employee, decision CheckResult) ([]Employee, error) {
	if decisionUsesOpenFGAScopeCheck(decision) {
		filtered, err := c.filterEmployeesByOpenFGAScope(ctx, account, decision, items)
		if err == nil {
			return filtered, nil
		}
		c.logWarn(ctx, "openfga employee scope check failed; falling back to SQL-derived scope", "error", err)
	}
	switch decision.Scope {
	case "", ScopeAll, ScopeTenant, ScopeSystem:
		return items, nil
	case ScopeSelf, ScopeOwn:
		out := make([]Employee, 0, 1)
		for _, item := range items {
			if item.ID == account.EmployeeID {
				out = append(out, item)
			}
		}
		return out, nil
	case ScopeDepartment, ScopeAssignedOrgUnits:
		orgIDs := stringSliceFromAny(decision.Conditions["org_unit_ids"])
		if len(orgIDs) == 0 {
			return []Employee{}, nil
		}
		allowed := map[string]struct{}{}
		for _, id := range orgIDs {
			allowed[id] = struct{}{}
		}
		out := make([]Employee, 0)
		for _, item := range items {
			if _, ok := allowed[item.OrgUnitID]; ok {
				out = append(out, item)
			}
		}
		return out, nil
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
		if len(orgIDs) == 0 {
			return []Employee{}, nil
		}
		allowed := map[string]struct{}{}
		for _, id := range orgIDs {
			allowed[id] = struct{}{}
		}
		units, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
		if err != nil {
			return nil, err
		}
		out := make([]Employee, 0)
		for _, item := range items {
			if orgUnitInScope(units, item.OrgUnitID, allowed) {
				out = append(out, item)
			}
		}
		return out, nil
	case ScopeDirectReports:
		ids := stringSliceFromAny(decision.Conditions["employee_ids"])
		allowed := map[string]struct{}{}
		for _, id := range ids {
			allowed[id] = struct{}{}
		}
		out := make([]Employee, 0)
		for _, item := range items {
			if _, ok := allowed[item.ID]; ok {
				out = append(out, item)
			}
		}
		return out, nil
	case ScopeCustomCondition:
		return filterEmployeesByConditions(items, decision.Conditions), nil
	default:
		return []Employee{}, nil
	}
}

// filterEmployeesByOpenFGAScope 使用 OpenFGA 檢查員工資料範圍。
func (c HRService) filterEmployeesByOpenFGAScope(ctx RequestContext, account Account, decision CheckResult, items []Employee) ([]Employee, error) {
	if c.relationships == nil {
		return nil, fmt.Errorf("openfga scope checker is not configured")
	}
	out := make([]Employee, 0, len(items))
	for _, item := range items {
		allowed, err := c.employeeAllowedByOpenFGAScope(ctx, account, decision.Scope, item)
		if err != nil {
			return nil, err
		}
		if allowed {
			out = append(out, item)
		}
	}
	return out, nil
}

// employeeAllowedByOpenFGAScope 判斷單一員工是否通過 FGA scope check。
func (c HRService) employeeAllowedByOpenFGAScope(ctx RequestContext, account Account, scope Scope, employee Employee) (bool, error) {
	orgUnitID := strings.TrimSpace(employee.OrgUnitID)
	if orgUnitID == "" {
		return false, nil
	}
	subject := "account:" + account.ID
	object := openFGATypeOrgUnit + ":" + orgUnitID
	switch scope {
	case ScopeDepartment:
		return c.anyRelationshipAllows(ctx, subject, object, openFGARelationOrgUnitMember, openFGARelationOrgUnitManager)
	case ScopeDepartmentSubtree:
		return c.anyRelationshipAllows(ctx, subject, object, openFGARelationOrgUnitMemberTree, openFGARelationOrgUnitManager)
	default:
		return false, nil
	}
}

// filterOrgUnitsByOpenFGAScope 使用 OpenFGA 檢查組織單位資料範圍。
func (c HRService) filterOrgUnitsByOpenFGAScope(ctx RequestContext, account Account, decision CheckResult, units []OrgUnit) ([]OrgUnit, error) {
	if c.relationships == nil {
		return nil, fmt.Errorf("openfga scope checker is not configured")
	}
	out := make([]OrgUnit, 0, len(units))
	subject := "account:" + account.ID
	for _, unit := range units {
		object := openFGATypeOrgUnit + ":" + unit.ID
		var allowed bool
		var err error
		switch decision.Scope {
		case ScopeDepartment:
			allowed, err = c.anyRelationshipAllows(ctx, subject, object, openFGARelationOrgUnitMember, openFGARelationOrgUnitManager)
		case ScopeDepartmentSubtree:
			allowed, err = c.anyRelationshipAllows(ctx, subject, object, openFGARelationOrgUnitMemberTree, openFGARelationOrgUnitManager)
		default:
			return []OrgUnit{}, nil
		}
		if err != nil {
			return nil, err
		}
		if allowed {
			out = append(out, unit)
		}
	}
	return out, nil
}

// anyRelationshipAllows 依序檢查多個 relation, 任一允許即通過。
func (c HRService) anyRelationshipAllows(ctx RequestContext, subject, object string, relations ...string) (bool, error) {
	for _, relation := range relations {
		allowed, err := c.relationships.CheckRelationship(goContext(ctx), domain.RelationshipCheck{
			TenantID: ctx.TenantID,
			Subject:  subject,
			Relation: relation,
			Object:   object,
		})
		if err != nil {
			return false, err
		}
		if allowed {
			return true, nil
		}
	}
	return false, nil
}

// filterEmployeesByConditions 處理篩選員工 by conditions。
func filterEmployeesByConditions(items []Employee, conditions map[string]any) []Employee {
	employeeIDs := stringSliceFromAny(conditions["employee_ids"])
	orgUnitIDs := stringSliceFromAny(conditions["org_unit_ids"])
	statuses := stringSliceFromAny(conditions["employee_statuses"])
	if len(statuses) == 0 {
		statuses = stringSliceFromAny(conditions["statuses"])
	}
	employeeAllowed := stringSet(employeeIDs)
	orgAllowed := stringSet(orgUnitIDs)
	statusAllowed := stringSet(statuses)
	if len(employeeAllowed) == 0 && len(orgAllowed) == 0 && len(statusAllowed) == 0 {
		return []Employee{}
	}
	out := make([]Employee, 0, len(items))
	for _, item := range items {
		if len(employeeAllowed) > 0 {
			if _, ok := employeeAllowed[item.ID]; !ok {
				continue
			}
		}
		if len(orgAllowed) > 0 {
			if _, ok := orgAllowed[item.OrgUnitID]; !ok {
				continue
			}
		}
		if len(statusAllowed) > 0 {
			if _, ok := statusAllowed[item.Status]; !ok {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

// stringSet 處理字串集合。
func stringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := map[string]struct{}{}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

// maskEmployee 處理 mask 員工。
func maskEmployee(item Employee, policies map[string]string) Employee {
	for field, effect := range policies {
		if effect != "mask" && effect != "hide" && effect != "deny" {
			continue
		}
		hide := effect == "hide" || effect == "deny"
		switch field {
		case "employee_no":
			item.EmployeeNo = redactString(item.EmployeeNo, hide)
		case "name":
			item.Name = redactString(item.Name, hide)
		case "company_email":
			if hide {
				item.CompanyEmail = ""
			} else {
				item.CompanyEmail = maskEmail(item.CompanyEmail)
			}
		case "personal_email":
			if hide {
				item.PersonalEmail = ""
			} else {
				item.PersonalEmail = maskEmail(item.PersonalEmail)
			}
		case "phone":
			if hide {
				item.Phone = ""
			} else {
				item.Phone = maskValue(item.Phone)
			}
		case "position":
			item.Position = redactString(item.Position, hide)
		case "category":
			item.Category = redactString(item.Category, hide)
		case "status":
			item.Status = redactString(item.Status, hide)
		case "employment_status":
			item.EmploymentStatus = redactString(item.EmploymentStatus, hide)
		case "manager_employee_id":
			item.ManagerEmployeeID = redactString(item.ManagerEmployeeID, hide)
		case "hire_date":
			item.HireDate = nil
		case "resign_date":
			item.ResignDate = nil
		case "org_unit_id":
			item.OrgUnitID = ""
		case "account_id":
			item.AccountID = ""
		case "basic_info":
			item.BasicInfo = nil
		case "contact_info":
			item.ContactInfo = nil
		case "insurance_info":
			item.InsuranceInfo = nil
		default:
			item.BasicInfo = redactMapField(item.BasicInfo, field, hide)
			item.ContactInfo = redactMapField(item.ContactInfo, field, hide)
			item.InsuranceInfo = redactMapField(item.InsuranceInfo, field, hide)
			item.EmploymentInfo = redactMapField(item.EmploymentInfo, field, hide)
			item.EducationMilitaryInfo = redactMapField(item.EducationMilitaryInfo, field, hide)
		}
	}
	return item
}

// redactString 處理 redact 字串。
func redactString(value string, hide bool) string {
	if hide {
		return ""
	}
	return "***"
}

// redactMapField 處理 redact map 欄位。
func redactMapField(values map[string]any, field string, hide bool) map[string]any {
	if len(values) == 0 {
		return values
	}
	if _, ok := values[field]; !ok {
		return values
	}
	out := utils.CopyStringMap(values)
	if hide {
		delete(out, field)
	} else {
		out[field] = "***"
	}
	return out
}

// maskValue 處理 mask value。
func maskValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "***"
	}
	return value[:2] + "***" + value[len(value)-2:]
}

// maskEmail 處理 mask email。
func maskEmail(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parts := strings.SplitN(value, "@", 2)
	if len(parts) != 2 {
		return maskValue(value)
	}
	local := parts[0]
	if len(local) <= 2 {
		local = "***"
	} else {
		local = local[:1] + "***" + local[len(local)-1:]
	}
	return local + "@" + parts[1]
}

// orgUnitInScope 處理組織單位 in 範圍。
func orgUnitInScope(units []OrgUnit, orgUnitID string, allowed map[string]struct{}) bool {
	if _, ok := allowed[orgUnitID]; ok {
		return true
	}
	for _, unit := range units {
		if unit.ID != orgUnitID {
			continue
		}
		for _, parentID := range unit.Path {
			if _, ok := allowed[parentID]; ok {
				return true
			}
		}
	}
	return false
}

// stringSliceFromAny 處理字串 slice 來源 any。
func stringSliceFromAny(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	default:
		return nil
	}
}

// AuthzSnapshotCache 定義授權快照快取的行為契約。
