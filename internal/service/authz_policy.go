package service

import (
	"fmt"
	"strings"

	authzpkg "nexus-pro-be/internal/domain/authz"
	"nexus-pro-be/internal/utils"
)

func permissionKey(applicationCode ApplicationCode, resourceType ResourceType, action Action) string {
	return fmt.Sprintf("%s.%s.%s", applicationCode, resourceType, action)
}

func permissionEffect(grant authzGrant) string {
	if strings.EqualFold(grant.Effect, "deny") || strings.EqualFold(grant.Permission.Effect, "deny") {
		return "deny"
	}
	return utils.FirstNonEmpty(grant.Permission.Effect, grant.Effect, "allow")
}

func isHighRiskPermission(perm Permission) bool {
	return perm.RiskLevel == "high" || perm.RiskLevel == "critical"
}

func routeRequiresApproval(req CheckRequest) bool {
	for _, policy := range authzpkg.DefaultRoutePolicies {
		if policy.ApplicationCode == string(req.ApplicationCode) &&
			policy.ResourceType == string(req.ResourceType) &&
			strings.EqualFold(policy.Action, string(req.Action)) {
			return policy.RiskLevel == authzpkg.RiskHigh || policy.RiskLevel == authzpkg.RiskCritical
		}
	}
	return false
}

func approvalPolicyForRoute(req CheckRequest) (bool, string, string, string) {
	for _, policy := range authzpkg.DefaultRoutePolicies {
		if policy.ApplicationCode == string(req.ApplicationCode) &&
			policy.ResourceType == string(req.ResourceType) &&
			strings.EqualFold(policy.Action, string(req.Action)) {
			risk := string(policy.RiskLevel)
			if policy.RiskLevel == authzpkg.RiskHigh || policy.RiskLevel == authzpkg.RiskCritical {
				return true, risk, approvalTypeForRisk(risk), "route_policy"
			}
			return false, risk, "", ""
		}
	}
	return false, string(authzpkg.RiskNormal), "", ""
}

func approvalTypeForRisk(risk string) string {
	switch risk {
	case string(authzpkg.RiskCritical):
		return "approval"
	case string(authzpkg.RiskHigh):
		return "confirmation"
	default:
		return ""
	}
}

func maxRiskLevel(a, b string) string {
	if riskRank(b) > riskRank(a) {
		return b
	}
	if a == "" {
		return string(authzpkg.RiskNormal)
	}
	return a
}

func riskRank(risk string) int {
	switch risk {
	case string(authzpkg.RiskCritical):
		return 3
	case string(authzpkg.RiskHigh):
		return 2
	case string(authzpkg.RiskNormal), "":
		return 1
	default:
		return 1
	}
}

func policyDenies(policy map[string]any, key string) bool {
	return policyListContains(policy, "deny", key)
}

func policyAllows(policy map[string]any, key string) bool {
	if len(policy) == 0 {
		return true
	}
	allows, ok := policy["allow"]
	if !ok {
		return true
	}
	return valueListContains(allows, key)
}

func policyListContains(policy map[string]any, field, key string) bool {
	if len(policy) == 0 {
		return false
	}
	return valueListContains(policy[field], key)
}

func valueListContains(value any, key string) bool {
	switch v := value.(type) {
	case []string:
		for _, item := range v {
			if permissionKeyMatches(key, item) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && permissionKeyMatches(key, s) {
				return true
			}
		}
	case string:
		return permissionKeyMatches(key, v)
	}
	return false
}

func permissionKeyMatches(key, pattern string) bool {
	if pattern == "" {
		return false
	}
	if pattern == "*" || strings.EqualFold(key, pattern) {
		return true
	}
	if strings.HasSuffix(pattern, ".*") {
		return strings.HasPrefix(key, strings.TrimSuffix(pattern, "*"))
	}
	return false
}

func mergePolicy(base, extra map[string]any) map[string]any {
	if len(base) == 0 {
		return utils.CopyStringMap(extra)
	}
	out := utils.CopyStringMap(base)
	for key, value := range extra {
		if existing, ok := out[key]; ok && key == "deny" {
			out[key] = appendPolicyList(existing, value)
			continue
		}
		if existing, ok := out[key]; ok && key == "allow" {
			out[key] = intersectPolicyList(existing, value)
			continue
		}
		out[key] = value
	}
	return out
}

func appendPolicyList(a, b any) []any {
	out := make([]any, 0)
	appendOne := func(v any) {
		switch x := v.(type) {
		case []string:
			for _, item := range x {
				out = append(out, item)
			}
		case []any:
			out = append(out, x...)
		case string:
			out = append(out, x)
		}
	}
	appendOne(a)
	appendOne(b)
	return out
}

func intersectPolicyList(a, b any) []any {
	left := policyStrings(a)
	right := policyStrings(b)
	out := make([]string, 0)
	for _, l := range left {
		for _, r := range right {
			switch {
			case permissionKeyMatches(r, l):
				out = append(out, r)
			case permissionKeyMatches(l, r):
				out = append(out, l)
			case strings.EqualFold(l, r):
				out = append(out, l)
			}
		}
	}
	return anyStrings(uniqueStrings(out))
}

func policyStrings(value any) []string {
	switch v := value.(type) {
	case []string:
		return utils.CopyStrings(v)
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

func anyStrings(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func relationshipConstraint(perm Permission) string {
	if strings.TrimSpace(perm.Relation) != "" {
		return strings.TrimSpace(perm.Relation)
	}
	if strings.HasPrefix(perm.Target, "rebac:") {
		return strings.TrimSpace(strings.TrimPrefix(perm.Target, "rebac:"))
	}
	return ""
}

func (c *Service) relationshipAllows(ctx RequestContext, account Account, req CheckRequest, relation string) (bool, string, error) {
	object := relationshipObject(req)
	label := "openfga:" + object + "#" + relation
	if c.relationships == nil || req.ResourceID == "" || relation == "" {
		return false, label, nil
	}
	allowed, err := c.relationships.CheckRelationship(goContext(ctx), authzpkg.RelationshipCheck{
		TenantID: ctx.TenantID,
		Subject:  "account:" + account.ID,
		Relation: relation,
		Object:   object,
	})
	if err != nil {
		return false, label, err
	}
	return allowed, label, nil
}

func relationshipObject(req CheckRequest) string {
	return routeResourceName(req.ApplicationCode, req.ResourceType) + ":" + req.ResourceID
}

func chooseScope(current Scope, currentConditions map[string]any, candidate Scope, candidateConditions map[string]any) (Scope, map[string]any) {
	if candidate == "" {
		return current, currentConditions
	}
	candidateRank := scopeRank(candidate)
	currentRank := scopeRank(current)
	if current == "" || candidateRank > currentRank {
		return candidate, candidateConditions
	}
	if candidateRank == currentRank {
		return current, mergeScopeConditions(currentConditions, candidateConditions)
	}
	return current, currentConditions
}

func mergeScopeConditions(current, candidate map[string]any) map[string]any {
	out := utils.CopyStringMap(current)
	if out == nil {
		out = map[string]any{}
	}
	for key, value := range candidate {
		switch key {
		case "employee_ids", "org_unit_ids":
			merged := uniqueStrings(append(stringSliceFromAny(out[key]), stringSliceFromAny(value)...))
			if len(merged) > 0 {
				out[key] = merged
			}
		default:
			if _, exists := out[key]; !exists {
				out[key] = value
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

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
			return nil, forbiddenDataScope("system data scope requires an assumed role session")
		}
		return out, nil
	case ScopeSelf, ScopeOwn:
		if _, ok := out["employee_ids"]; !ok {
			if account.EmployeeID == "" {
				return nil, forbiddenDataScope("account is not linked to an employee for own scope")
			}
			out["employee_ids"] = []string{account.EmployeeID}
		}
	case ScopeDepartment:
		if _, ok := out["org_unit_ids"]; !ok && account.EmployeeID == "" {
			return nil, forbiddenDataScope("account is not linked to an employee for department scope")
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
	case ScopeDepartmentSubtree:
		if _, ok := out["org_unit_ids"]; !ok && account.EmployeeID == "" {
			return nil, forbiddenDataScope("account is not linked to an employee for department_subtree scope")
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
	case ScopeDirectReports:
		if _, ok := out["employee_ids"]; !ok && account.EmployeeID != "" {
			employees, err := c.store.ListEmployees(goContext(ctx), ctx.TenantID)
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
			return nil, forbiddenDataScope("assigned_org_units scope requires org_unit_ids")
		}
	case ScopeCustomCondition:
		out["scope"] = ScopeCustomCondition
	default:
		out["scope"] = scope
	}
	return out, nil
}

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

func (c *Service) applyEmployeeDecision(ctx RequestContext, account Account, items []Employee, decision CheckResult) ([]Employee, error) {
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

func (c *Service) filterEmployeesByDecision(ctx RequestContext, account Account, items []Employee, decision CheckResult) ([]Employee, error) {
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

func redactString(value string, hide bool) string {
	if hide {
		return ""
	}
	return "***"
}

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
