package service

import (
	"fmt"
	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
	"reflect"
	"strings"
	"time"
)

func (c *Service) activeAssumableRole(ctx RequestContext, account Account) (*AssumableRole, *AssumableRoleSession, error) {
	if ctx.AssumedRoleSessionID != "" {
		session, ok, err := c.store.GetActiveAssumableRoleSession(goContext(ctx), ctx.TenantID, ctx.AssumedRoleSessionID)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			return nil, nil, NotFound("assumable role session", ctx.AssumedRoleSessionID)
		}
		if session.AccountID != account.ID {
			return nil, nil, Forbidden("assumable role session belongs to another account")
		}
		role, ok, err := c.store.GetAssumableRole(goContext(ctx), ctx.TenantID, session.AssumableRoleID)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			return nil, nil, NotFound("assumable role", session.AssumableRoleID)
		}
		return &role, &session, nil
	}
	return nil, nil, nil
}

// conditionsForGrant 處理 conditions for grant 的服務流程。
func (c *Service) conditionsForGrant(ctx RequestContext, account Account, grant authzGrant, req CheckRequest) (Scope, map[string]any, error) {
	if grant.DataScope != nil {
		conditions, err := c.scopeConditions(ctx, account, Scope(grant.DataScope.ScopeType), grant.DataScope.Params)
		return Scope(grant.DataScope.ScopeType), conditions, err
	}
	scope := grant.Permission.Scope
	if scope == "" {
		scope = ScopeAll
	}
	conditions, err := c.scopeConditions(ctx, account, scope, nil)
	return scope, conditions, err
}

// fieldPolicyDecision 處理欄位政策決策的服務流程。
func (c *Service) fieldPolicyDecision(ctx RequestContext, applicationCode ApplicationCode, resourceType ResourceType, permissionKey string, matchedPermissions []string) (map[string]string, error) {
	policies, err := c.store.ListFieldPolicies(goContext(ctx), ctx.TenantID, string(applicationCode), string(resourceType))
	if err != nil {
		return nil, err
	}
	out := defaultFieldPolicies(applicationCode, resourceType)
	if len(policies) == 0 {
		if len(out) == 0 {
			return nil, nil
		}
		return out, nil
	}
	catalogItems, err := c.store.ListPermissionCatalogItems(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	catalogLabels := make(map[string]string, len(catalogItems))
	for _, item := range catalogItems {
		catalogLabels[item.ID] = strings.TrimSpace(item.Resource) + "." + strings.TrimSpace(item.Action)
	}
	explicitRestrictions := map[string]string{}
	explicitAllows := map[string]struct{}{}
	for _, policy := range policies {
		if !fieldPolicyApplies(policy, permissionKey, matchedPermissions, catalogLabels) {
			continue
		}
		field := strings.TrimSpace(policy.FieldName)
		if field == "" {
			continue
		}
		effect := strings.TrimSpace(policy.Effect)
		if effect == "allow" {
			explicitAllows[field] = struct{}{}
			continue
		}
		if current, ok := explicitRestrictions[field]; !ok || fieldPolicyEffectRank(effect) > fieldPolicyEffectRank(current) {
			explicitRestrictions[field] = effect
		}
	}
	for field, effect := range explicitRestrictions {
		out[field] = effect
	}
	for field := range explicitAllows {
		if _, restricted := explicitRestrictions[field]; !restricted {
			delete(out, field)
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// defaultFieldPolicies 處理預設欄位政策。
func defaultFieldPolicies(applicationCode ApplicationCode, resourceType ResourceType) map[string]string {
	if applicationCode == AppAttendance && resourceType == ResourceAttendanceClock {
		return map[string]string{
			"latitude":        "hide",
			"longitude":       "hide",
			"accuracy_meters": "hide",
			"distance_meters": "hide",
			"device_id":       "hide",
			"device_info":     "hide",
			"location_source": "hide",
		}
	}
	if applicationCode != AppHR || resourceType != ResourceEmployee {
		return map[string]string{}
	}
	return map[string]string{
		"personal_email":          "mask",
		"phone":                   "mask",
		"mobile_phone":            "mask",
		"address":                 "mask",
		"communication_address":   "mask",
		"emergency_contact_name":  "mask",
		"emergency_name":          "mask",
		"emergency_contact_phone": "mask",
		"emergency_phone":         "mask",
		"national_id":             "mask",
		"passport_no":             "mask",
		"arc_no":                  "mask",
		"tax_id":                  "mask",
		"work_permit_no":          "mask",
		"insurance_info":          "mask",
		"labor_insurance_salary":  "mask",
		"health_insurance_amount": "mask",
	}
}

// fieldPolicyApplies 同時接受 catalog ID 與既有 label/pattern，確保 UI 保存的 ID 可在 runtime 命中。
func fieldPolicyApplies(policy FieldPolicy, permissionKey string, matchedPermissions []string, catalogLabels map[string]string) bool {
	policyPermission := strings.TrimSpace(policy.PermissionID)
	if policyPermission == "" {
		return true
	}
	if catalogLabel := strings.TrimSpace(catalogLabels[policyPermission]); catalogLabel != "" {
		policyPermission = catalogLabel
	}
	if permissionLabelMatches(permissionKey, policyPermission) {
		return true
	}
	for _, matched := range matchedPermissions {
		if permissionLabelMatches(matched, policyPermission) {
			return true
		}
	}
	return false
}

// permissionLabelMatches 處理權限 label matches。
func permissionLabelMatches(value, pattern string) bool {
	value = strings.TrimSpace(value)
	pattern = strings.TrimSpace(pattern)
	if value == "" || pattern == "" {
		return false
	}
	if wildcardMatch(value, pattern) {
		return true
	}
	valueBase, _, _ := strings.Cut(value, "#")
	patternBase, _, patternHasScope := strings.Cut(pattern, "#")
	if !patternHasScope && permissionKeyMatches(valueBase, patternBase) {
		return true
	}
	return permissionKeyMatches(value, pattern)
}

// fieldPolicyEffectRank 處理欄位政策 effect rank。
func fieldPolicyEffectRank(effect string) int {
	switch effect {
	case "deny":
		return 5
	case "hide":
		return 4
	case "mask":
		return 3
	case "readonly":
		return 2
	case "allow":
		return 1
	default:
		return 0
	}
}

// auditAuthzDecision 處理稽核授權決策的服務流程。
func (c *Service) auditAuthzDecision(ctx RequestContext, action, resource, target string, decision CheckResult) error {
	return c.audit(ctx, action, resource, target, "high", auditDecisionDetails(ctx, decision, nil))
}

// defaultPermissions 處理預設權限。
func defaultPermissions() []Permission {
	out := make([]Permission, 0, len(domain.DefaultRoutePolicies))
	for _, policy := range domain.DefaultRoutePolicies {
		out = append(out, Permission{
			ApplicationCode: ApplicationCode(policy.ApplicationCode),
			ResourceType:    ResourceType(policy.ResourceType),
			Resource:        routeResourceName(ApplicationCode(policy.ApplicationCode), ResourceType(policy.ResourceType)),
			Action:          Action(policy.Action),
			RiskLevel:       string(policy.RiskLevel),
		})
	}
	return out
}

// touchAuthzConfig 處理 touch 授權組態的服務流程。
func (c *Service) touchAuthzConfig(ctx RequestContext, eventType string, payload map[string]any) error {
	version, err := c.store.IncrementPermissionVersion(goContext(ctx), ctx.TenantID)
	if err != nil {
		return err
	}
	c.invalidateAuthzSnapshots(goContext(ctx), ctx.TenantID)
	if payload == nil {
		payload = map[string]any{}
	}
	payload["permission_version"] = version
	return c.store.AppendOutboxEvent(goContext(ctx), OutboxEvent{
		ID:            utils.NewID("outbox"),
		TenantID:      ctx.TenantID,
		EventType:     eventType,
		AggregateType: domain.OutboxAggregateAuthz,
		Payload:       payload,
		Status:        "pending",
		RetryCount:    0,
		CreatedAt:     c.Now(),
	})
}

// normalizeCheckRequest 正規化check 請求。
func normalizeCheckRequest(req CheckRequest) CheckRequest {
	req.RouteMethod = strings.ToUpper(strings.TrimSpace(req.RouteMethod))
	req.RoutePath = strings.TrimSpace(req.RoutePath)
	if req.ApplicationCode == "" || req.ResourceType == "" {
		app, resourceType := splitResource(req.Resource)
		if req.ApplicationCode == "" {
			req.ApplicationCode = app
		}
		if req.ResourceType == "" {
			req.ResourceType = resourceType
		}
	}
	if req.ApplicationCode == "" {
		req.ApplicationCode = AppPlatform
	}
	if req.ResourceType == "" {
		req.ResourceType = ResourceType(req.Resource)
	}
	if req.Resource == "" {
		req.Resource = routeResourceName(req.ApplicationCode, req.ResourceType)
	}
	if req.Target == "" {
		req.Target = req.ResourceID
	}
	if req.TargetEmployeeID == "" && req.ResourceType == ResourceEmployee {
		req.TargetEmployeeID = req.ResourceID
	}
	return req
}

// normalizePermission 正規化權限。
func normalizePermission(perm Permission) Permission {
	if perm.ApplicationCode == "" || perm.ResourceType == "" {
		app, resourceType := splitResource(perm.Resource)
		if perm.ApplicationCode == "" {
			perm.ApplicationCode = app
		}
		if perm.ResourceType == "" {
			perm.ResourceType = resourceType
		}
	}
	if perm.Resource == "" {
		perm.Resource = routeResourceName(perm.ApplicationCode, perm.ResourceType)
	}
	return perm
}

// normalizePermissions 正規化權限。
func normalizePermissions(perms []Permission) []Permission {
	if len(perms) == 0 {
		return nil
	}
	out := make([]Permission, 0, len(perms))
	for _, perm := range perms {
		out = append(out, normalizePermission(perm))
	}
	return out
}

// splitResource 拆分resource。
func splitResource(resource string) (ApplicationCode, ResourceType) {
	if resource == "" {
		return AppPlatform, ""
	}
	if resource == "*" {
		return "*", "*"
	}
	parts := strings.SplitN(resource, ".", 2)
	if len(parts) == 2 {
		return ApplicationCode(parts[0]), ResourceType(parts[1])
	}
	return AppPlatform, ResourceType(resource)
}

// routeResourceName 處理路由 resource 名稱。
func routeResourceName(applicationCode ApplicationCode, resourceType ResourceType) string {
	if applicationCode == "" || applicationCode == AppPlatform {
		return string(resourceType)
	}
	return string(applicationCode) + "." + string(resourceType)
}

// optionalDateTime 處理可選日期時間。
func optionalDateTime(value string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	t, err := utils.ParseDateTime(value)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// permissionKey 處理權限 key。
func permissionKey(applicationCode ApplicationCode, resourceType ResourceType, action Action) string {
	return fmt.Sprintf("%s.%s.%s", applicationCode, resourceType, action)
}

// permissionEffect 處理權限 effect。
func permissionEffect(grant authzGrant) string {
	if strings.EqualFold(grant.Effect, "deny") || strings.EqualFold(grant.Permission.Effect, "deny") {
		return "deny"
	}
	return utils.FirstNonEmpty(grant.Permission.Effect, grant.Effect, "allow")
}

// isHighRiskPermission 判斷是否為high risk 權限。
func isHighRiskPermission(perm Permission) bool {
	return perm.RiskLevel == "high" || perm.RiskLevel == "critical"
}

// riskLevelForRoute 回傳與目前授權請求匹配的路由風險分級。
func riskLevelForRoute(req CheckRequest) string {
	reqResource := strings.TrimSpace(req.Resource)
	if req.RouteMethod != "" || req.RoutePath != "" {
		for _, policy := range domain.DefaultRoutePolicies {
			if routePolicyMatchesHTTPRoute(req, policy, reqResource) {
				return normalizedRiskLevel(policy.RiskLevel)
			}
		}
		return string(domain.RiskNormal)
	}
	for _, policy := range domain.DefaultRoutePolicies {
		if strings.EqualFold(policy.Action, string(req.Action)) && routePolicyMatchesRequest(req, policy, reqResource) {
			return normalizedRiskLevel(policy.RiskLevel)
		}
	}
	return string(domain.RiskNormal)
}

// routePolicyMatchesHTTPRoute 處理路由政策 matches HTTP 路由。
func routePolicyMatchesHTTPRoute(req CheckRequest, policy domain.RoutePolicy, reqResource string) bool {
	if req.RouteMethod != "" && !strings.EqualFold(policy.Method, req.RouteMethod) {
		return false
	}
	if req.RoutePath != "" && policy.Path != req.RoutePath {
		return false
	}
	return strings.EqualFold(policy.Action, string(req.Action)) && routePolicyMatchesRequest(req, policy, reqResource)
}

// routePolicyMatchesRequest 處理路由政策 matches 請求。
func routePolicyMatchesRequest(req CheckRequest, policy domain.RoutePolicy, reqResource string) bool {
	if policy.ApplicationCode == string(req.ApplicationCode) && policy.ResourceType == string(req.ResourceType) {
		return true
	}
	if reqResource == "" {
		return false
	}
	return strings.EqualFold(reqResource, legacyRouteResourceName(policy.ApplicationCode, policy.ResourceType))
}

// normalizedRiskLevel 保證未標記風險的路由以 normal 分級寫入授權結果。
func normalizedRiskLevel(risk domain.RiskLevel) string {
	if risk == "" {
		return string(domain.RiskNormal)
	}
	return string(risk)
}

// legacyRouteResourceName 處理 legacy 路由 resource 名稱。
func legacyRouteResourceName(applicationCode, resourceType string) string {
	if applicationCode == string(AppAudit) && resourceType == "audit_log" {
		return "audit.log"
	}
	return routeResourceName(ApplicationCode(applicationCode), ResourceType(resourceType))
}

// maxRiskLevel 取得較大值risk level。
func maxRiskLevel(a, b string) string {
	if riskRank(b) > riskRank(a) {
		return b
	}
	if a == "" {
		return string(domain.RiskNormal)
	}
	return a
}

// riskRank 處理 risk rank。
func riskRank(risk string) int {
	switch risk {
	case string(domain.RiskCritical):
		return 3
	case string(domain.RiskHigh):
		return 2
	case string(domain.RiskNormal), "":
		return 1
	default:
		return 1
	}
}

// policyDenies 處理政策 denies。
func policyDenies(policy map[string]any, key string) bool {
	return policyListContains(policy, "deny", key)
}

// policyAllows 處理政策 allows。
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

// policyListContains 處理政策列表 contains。
func policyListContains(policy map[string]any, field, key string) bool {
	if len(policy) == 0 {
		return false
	}
	return valueListContains(policy[field], key)
}

// valueListContains 處理 value 列表 contains。
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

// permissionKeyMatches 處理權限 key matches。
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

// mergePolicy 合併政策。
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

// appendPolicyList 附加政策列表。
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

// intersectPolicyList 處理 intersect 政策列表。
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

// policyStrings 處理政策字串。
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

// anyStrings 處理 any 字串。
func anyStrings(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

// relationshipConstraint 處理關係 constraint。
func relationshipConstraint(perm Permission) string {
	if strings.TrimSpace(perm.Relation) != "" {
		return strings.TrimSpace(perm.Relation)
	}
	if strings.HasPrefix(perm.Target, "rebac:") {
		return strings.TrimSpace(strings.TrimPrefix(perm.Target, "rebac:"))
	}
	return ""
}

// relationshipAllows 處理關係 allows 的服務流程。
func (c *Service) relationshipAllows(ctx RequestContext, account Account, req CheckRequest, relation string) (bool, string, error) {
	object := relationshipObject(req)
	label := "openfga:" + object + "#" + relation
	if c.relationships == nil || req.ResourceID == "" || relation == "" {
		return false, label, nil
	}
	allowed, err := c.relationships.CheckRelationship(goContext(ctx), domain.RelationshipCheck{
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

// relationshipObject 處理關係物件。
func relationshipObject(req CheckRequest) string {
	return routeResourceName(req.ApplicationCode, req.ResourceType) + ":" + req.ResourceID
}

// chooseScope 處理 choose 範圍。
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

// mergeScopeConditions 合併範圍 conditions。
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

// intersectScopes 以 AND 語義收斂兩個資料範圍。
func intersectScopes(leftScope Scope, leftConditions map[string]any, rightScope Scope, rightConditions map[string]any) (Scope, map[string]any, bool) {
	if rightScope == "" {
		return leftScope, utils.CopyStringMap(leftConditions), false
	}
	if leftScope == "" {
		return rightScope, utils.CopyStringMap(rightConditions), false
	}
	outConditions, empty := intersectScopeConditions(leftConditions, rightConditions)
	if empty {
		return narrowerScope(leftScope, rightScope), outConditions, true
	}
	outScope := narrowerScope(leftScope, rightScope)
	if requiresCustomIntersectionScope(leftScope, rightScope, outScope, outConditions) {
		outScope = ScopeCustomCondition
		if outConditions == nil {
			outConditions = map[string]any{}
		}
		outConditions["scope"] = ScopeCustomCondition
		delete(outConditions, "scope_check")
		delete(outConditions, "scope_check_scope")
	}
	return outScope, outConditions, false
}

// narrowerScope 回傳交集後較窄的枚舉範圍。
func narrowerScope(left, right Scope) Scope {
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	leftRank := scopeRank(left)
	rightRank := scopeRank(right)
	if rightRank < leftRank {
		return right
	}
	if leftRank < rightRank {
		return left
	}
	if left == right {
		return left
	}
	if left == ScopeTenant && right == ScopeAll {
		return left
	}
	if right == ScopeTenant && left == ScopeAll {
		return right
	}
	return left
}

// intersectScopeConditions 合併兩組條件；同 key 結構化條件取交集，不同 key 保留為 AND。
func intersectScopeConditions(left, right map[string]any) (map[string]any, bool) {
	out := utils.CopyStringMap(left)
	if out == nil {
		out = map[string]any{}
	}
	for key, value := range right {
		if isScopeMetadataKey(key) {
			if _, exists := out[key]; !exists {
				out[key] = value
			}
			continue
		}
		if isStructuredScopeConditionKey(key) {
			rightValues := stringSliceFromAny(value)
			if len(rightValues) == 0 {
				return out, true
			}
			if existing, exists := out[key]; exists {
				leftValues := stringSliceFromAny(existing)
				if len(leftValues) == 0 {
					return out, true
				}
				intersection := intersectStringSlices(leftValues, rightValues)
				if len(intersection) == 0 {
					out[key] = []string{}
					return out, true
				}
				out[key] = intersection
				continue
			}
			out[key] = uniqueStrings(rightValues)
			continue
		}
		if existing, exists := out[key]; exists {
			if key == "tenant_id" && fmt.Sprint(existing) != fmt.Sprint(value) {
				return out, true
			}
			if !reflect.DeepEqual(existing, value) {
				return out, true
			}
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, false
}

func requiresCustomIntersectionScope(left, right, out Scope, conditions map[string]any) bool {
	if out == ScopeCustomCondition {
		return true
	}
	if left != "" && right != "" && left != right && scopeRank(left) == scopeRank(right) {
		return true
	}
	restrictiveKeys := 0
	for _, key := range []string{"employee_ids", "org_unit_ids", "account_ids", "employee_statuses", "statuses"} {
		if len(stringSliceFromAny(conditions[key])) > 0 {
			restrictiveKeys++
		}
	}
	return restrictiveKeys > 1
}

func isScopeMetadataKey(key string) bool {
	switch key {
	case "scope", "scope_check", "scope_check_scope":
		return true
	default:
		return false
	}
}

func isStructuredScopeConditionKey(key string) bool {
	switch key {
	case "employee_ids", "org_unit_ids", "account_ids", "employee_statuses", "statuses":
		return true
	default:
		return false
	}
}

func intersectStringSlices(left, right []string) []string {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	allowed := map[string]struct{}{}
	for _, value := range left {
		if strings.TrimSpace(value) != "" {
			allowed[value] = struct{}{}
		}
	}
	out := make([]string, 0)
	for _, value := range right {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, ok := allowed[value]; ok {
			out = append(out, value)
		}
	}
	return uniqueStrings(out)
}

// boundaryScopeDecision 解析 permission boundary/session policy 中的資料範圍約束。
