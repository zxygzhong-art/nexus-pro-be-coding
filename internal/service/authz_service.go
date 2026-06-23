package service

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go.opentelemetry.io/otel/attribute"
	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
	"strings"
	"time"
)

// AuthzService evaluates permissions, scopes, field policies, and approval requirements.
type AuthzService struct {
	*Service
}

// Authz returns the authorization service facade.
func (c *Service) Authz() AuthzService {
	return AuthzService{Service: c}
}

// Check evaluates one authorization request for the current account context.
func (c AuthzService) Check(ctx RequestContext, req CheckRequest) (result CheckResult, err error) {
	ctx, span := startServiceSpan(ctx, "service.authz.check", authzSpanAttributes(req)...)
	defer func() {
		setAuthzSpanResult(span, result)
		finishServiceSpan(span, err)
	}()
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return CheckResult{}, err
	}
	result, err = c.evaluateAuthz(ctx, account, req)
	return result, err
}

// BatchCheck evaluates multiple authorization requests in order.
func (c AuthzService) BatchCheck(ctx RequestContext, req BatchCheckRequest) (result BatchCheckResult, err error) {
	ctx, span := startServiceSpan(ctx, "service.authz.batch_check")
	defer func() {
		span.SetAttributes(attribute.Int("authz.batch_size", len(req.Checks)))
		finishServiceSpan(span, err)
	}()
	results := make([]CheckResult, 0, len(req.Checks))
	for _, item := range req.Checks {
		itemResult, err := c.Check(ctx, item)
		if err != nil {
			return BatchCheckResult{}, err
		}
		results = append(results, itemResult)
	}
	return BatchCheckResult{Results: results}, nil
}

// ValidateApprovalInstance verifies approval evidence for a high-risk request.
func (c AuthzService) ValidateApprovalInstance(ctx RequestContext, req CheckRequest) error {
	return c.Service.ValidateApprovalInstance(ctx, req)
}

type authzGrant struct {
	Permission      Permission
	PermissionSetID string
	Source          string
	Effect          string
	DataScope       *DataScope
}

func (c *Service) evaluateAuthz(ctx RequestContext, account Account, req CheckRequest) (CheckResult, error) {
	req = normalizeCheckRequest(req)
	version, err := c.store.GetPermissionVersion(goContext(ctx), ctx.TenantID)
	if err != nil {
		return CheckResult{}, err
	}
	snapshotKey := c.authzSnapshotKey(ctx, account, req, version)
	useSnapshot := c.shouldUseAuthzSnapshot(ctx)
	if useSnapshot {
		if cached, ok := c.getAuthzSnapshot(goContext(ctx), snapshotKey); ok {
			return cached, nil
		}
	}
	grants, setIDs, assumedRole, boundary, err := c.collectAuthzGrants(ctx, account)
	if err != nil {
		return CheckResult{}, err
	}

	cacheResult := func(result CheckResult) CheckResult {
		if useSnapshot {
			c.setAuthzSnapshot(goContext(ctx), snapshotKey, result)
		}
		return result
	}

	matched := make([]string, 0)
	matchedBy := make([]string, 0)
	deniedBy := make([]string, 0)
	relationshipDeniedBy := make([]string, 0)
	var chosenScope Scope
	var chosenConditions map[string]any
	requiresApproval, riskLevel, approvalType, approvalReason := approvalPolicyForRoute(req)
	permissionKey := permissionKey(req.ApplicationCode, req.ResourceType, req.Action)

	for _, grant := range grants {
		if !permissionMatches(grant.Permission, req, account) {
			continue
		}
		source := grant.Source
		if source == "" {
			source = grant.PermissionSetID
		}
		if permissionEffect(grant) == "deny" {
			deniedBy = append(deniedBy, source)
			continue
		}
		if policyDenies(boundary, permissionKey) {
			deniedBy = append(deniedBy, "permission_boundary")
			continue
		}
		if !policyAllows(boundary, permissionKey) {
			deniedBy = append(deniedBy, "permission_boundary")
			continue
		}
		if relation := relationshipConstraint(grant.Permission); relation != "" {
			allowed, label, err := c.relationshipAllows(ctx, account, req, relation)
			if err != nil {
				return CheckResult{}, err
			}
			if !allowed {
				relationshipDeniedBy = append(relationshipDeniedBy, label)
				continue
			}
			matchedBy = append(matchedBy, label)
		}
		matched = append(matched, permissionLabel(grant.Permission))
		matchedBy = append(matchedBy, source)
		if isHighRiskPermission(grant.Permission) {
			requiresApproval = true
			riskLevel = maxRiskLevel(riskLevel, grant.Permission.RiskLevel)
			if approvalType == "" {
				approvalType = approvalTypeForRisk(grant.Permission.RiskLevel)
			}
			if approvalReason == "" {
				approvalReason = "permission_risk"
			}
		}
		scope, conditions, err := c.conditionsForGrant(ctx, account, grant, req)
		if err != nil {
			return CheckResult{}, err
		}
		chosenScope, chosenConditions = chooseScope(chosenScope, chosenConditions, scope, conditions)
	}
	if c.relationships != nil && len(matched) == 0 && req.ResourceID != "" {
		object := relationshipObject(req)
		allowed, err := c.relationships.CheckRelationship(goContext(ctx), domain.RelationshipCheck{
			TenantID: ctx.TenantID,
			Subject:  "account:" + account.ID,
			Relation: string(req.Action),
			Object:   object,
		})
		if err != nil {
			return CheckResult{}, err
		}
		if allowed {
			matched = append(matched, "openfga:"+object+"#"+string(req.Action))
			matchedBy = append(matchedBy, "openfga")
			chosenScope, chosenConditions = chooseScope(chosenScope, chosenConditions, ScopeObject, map[string]any{
				"tenant_id": ctx.TenantID,
				"object":    object,
				"relation":  req.Action,
			})
		}
	}

	fieldPolicies, err := c.fieldPolicyDecision(ctx, req.ApplicationCode, req.ResourceType)
	if err != nil {
		return CheckResult{}, err
	}
	result := CheckResult{
		Allowed:            len(matched) > 0 && len(deniedBy) == 0,
		MatchedBy:          uniqueStrings(matchedBy),
		MatchedPermissions: uniqueStrings(matched),
		PermissionSetIDs:   uniqueStrings(setIDs),
		Scope:              chosenScope,
		EffectiveScope:     chosenScope,
		Conditions:         chosenConditions,
		FieldPolicies:      fieldPolicies,
		PermissionBoundary: boundary,
		RequiresApproval:   requiresApproval && len(matched) > 0,
		RiskLevel:          riskLevel,
		ApprovalType:       approvalType,
		ApprovalReason:     approvalReason,
		Resource:           req.Resource,
		ApplicationCode:    req.ApplicationCode,
		ResourceType:       req.ResourceType,
		ResourceID:         req.ResourceID,
		Action:             req.Action,
		Target:             req.Target,
	}
	if assumedRole != nil {
		result.AssumedRole = assumedRole
	}
	if len(deniedBy) > 0 {
		result.Allowed = false
		result.Reason = "explicit deny"
		result.MissingPermissions = []string{permissionKey}
		return cacheResult(result), nil
	}
	if len(matched) == 0 {
		if len(relationshipDeniedBy) > 0 {
			result.Reason = "relationship denied"
			result.MissingPermissions = []string{permissionKey}
			result.MatchedBy = uniqueStrings(relationshipDeniedBy)
			return cacheResult(result), nil
		}
		result.Reason = "missing permission"
		result.MissingPermissions = []string{permissionKey}
		return cacheResult(result), nil
	}
	result.Reason = "matched permission"
	return cacheResult(result), nil
}

func (c *Service) collectAuthzGrants(ctx RequestContext, account Account) ([]authzGrant, []string, *AssumedRoleDecision, map[string]any, error) {
	grants := make([]authzGrant, 0)
	setIDs := make([]string, 0)

	addSet := func(setID, source, effect string, scope *DataScope) error {
		set, ok, err := c.store.GetPermissionSet(goContext(ctx), ctx.TenantID, setID)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		setIDs = append(setIDs, set.ID)
		for _, perm := range set.Permissions {
			perm = normalizePermission(perm)
			grants = append(grants, authzGrant{
				Permission:      perm,
				PermissionSetID: set.ID,
				Source:          source,
				Effect:          utils.FirstNonEmpty(effect, "allow"),
				DataScope:       scope,
			})
		}
		return nil
	}
	addAssignments := func(principalType, principalID, sourcePrefix string) error {
		assignments, err := c.store.ListPermissionSetAssignmentsForPrincipal(goContext(ctx), ctx.TenantID, principalType, principalID)
		if err != nil {
			return err
		}
		for _, assignment := range assignments {
			var scope *DataScope
			if assignment.DataScopeID != "" {
				v, ok, err := c.store.GetDataScope(goContext(ctx), ctx.TenantID, assignment.DataScopeID)
				if err != nil {
					return err
				}
				if !ok {
					return NotFound("data scope", assignment.DataScopeID)
				}
				scope = &v
			}
			if err := addSet(assignment.PermissionSetID, sourcePrefix+":"+principalID+":"+assignment.PermissionSetID, assignment.Effect, scope); err != nil {
				return err
			}
		}
		return nil
	}

	for _, id := range account.DirectPermissionSetIDs {
		if err := addSet(id, "direct:"+id, "allow", nil); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	if err := addAssignments("account", account.ID, "account"); err != nil {
		return nil, nil, nil, nil, err
	}

	for _, groupID := range account.UserGroupIDs {
		group, ok, err := c.store.GetUserGroup(goContext(ctx), ctx.TenantID, groupID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if !ok {
			continue
		}
		for _, id := range group.PermissionSetIDs {
			if err := addSet(id, "group:"+group.ID+":"+id, "allow", nil); err != nil {
				return nil, nil, nil, nil, err
			}
		}
		if err := addAssignments("user_group", group.ID, "group"); err != nil {
			return nil, nil, nil, nil, err
		}
	}

	role, session, err := c.activeAssumableRole(ctx, account)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var assumed *AssumedRoleDecision
	boundary := map[string]any(nil)
	if role != nil {
		assumed = &AssumedRoleDecision{RoleID: role.ID, Name: role.Name}
		boundary = utils.CopyStringMap(role.PermissionBoundary)
		for _, id := range role.PermissionSetIDs {
			if err := addSet(id, "assumable_role:"+role.ID+":"+id, "allow", nil); err != nil {
				return nil, nil, nil, nil, err
			}
		}
		if err := addAssignments("assumable_role", role.ID, "assumable_role"); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	if session != nil {
		assumed.SessionID = session.ID
		if len(session.PermissionBoundary) > 0 {
			boundary = mergePolicy(boundary, session.PermissionBoundary)
		}
		if len(session.SessionPolicy) > 0 {
			boundary = mergePolicy(boundary, session.SessionPolicy)
		}
	}

	return grants, uniqueStrings(setIDs), assumed, boundary, nil
}

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

func (c *Service) fieldPolicyDecision(ctx RequestContext, applicationCode ApplicationCode, resourceType ResourceType) (map[string]string, error) {
	policies, err := c.store.ListFieldPolicies(goContext(ctx), ctx.TenantID, string(applicationCode), string(resourceType))
	if err != nil {
		return nil, err
	}
	if len(policies) == 0 {
		return nil, nil
	}
	out := map[string]string{}
	for _, policy := range policies {
		out[policy.FieldName] = policy.Effect
	}
	return out, nil
}

func (c *Service) auditAuthzDecision(ctx RequestContext, action, resource, target string, decision CheckResult) error {
	return c.audit(ctx, action, resource, target, "high", auditDecisionDetails(ctx, decision, nil))
}

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
	return c.store.AppendAuthzOutboxEvent(goContext(ctx), AuthzOutboxEvent{
		ID:         utils.NewID("outbox"),
		TenantID:   ctx.TenantID,
		EventType:  eventType,
		Payload:    payload,
		Status:     "pending",
		RetryCount: 0,
		CreatedAt:  c.Now(),
	})
}

func normalizeCheckRequest(req CheckRequest) CheckRequest {
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

func routeResourceName(applicationCode ApplicationCode, resourceType ResourceType) string {
	if applicationCode == "" || applicationCode == AppPlatform {
		return string(resourceType)
	}
	return string(applicationCode) + "." + string(resourceType)
}

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

func approvalPolicyForRoute(req CheckRequest) (bool, string, string, string) {
	reqResource := strings.TrimSpace(req.Resource)
	for _, policy := range domain.DefaultRoutePolicies {
		if strings.EqualFold(policy.Action, string(req.Action)) && routePolicyMatchesRequest(req, policy, reqResource) {
			risk := string(policy.RiskLevel)
			if policy.RiskLevel == domain.RiskHigh || policy.RiskLevel == domain.RiskCritical {
				return true, risk, approvalTypeForRisk(risk), "route_policy"
			}
			return false, risk, "", ""
		}
	}
	return false, string(domain.RiskNormal), "", ""
}

func routePolicyMatchesRequest(req CheckRequest, policy domain.RoutePolicy, reqResource string) bool {
	if policy.ApplicationCode == string(req.ApplicationCode) && policy.ResourceType == string(req.ResourceType) {
		return true
	}
	if reqResource == "" {
		return false
	}
	return strings.EqualFold(reqResource, legacyRouteResourceName(policy.ApplicationCode, policy.ResourceType))
}

func legacyRouteResourceName(applicationCode, resourceType string) string {
	if applicationCode == string(AppAudit) && resourceType == "audit_log" {
		return "audit.log"
	}
	return routeResourceName(ApplicationCode(applicationCode), ResourceType(resourceType))
}

func approvalTypeForRisk(risk string) string {
	switch risk {
	case string(domain.RiskCritical):
		return "approval"
	case string(domain.RiskHigh):
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
		return string(domain.RiskNormal)
	}
	return a
}

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

func (c HRService) filterEmployeesByDecision(ctx RequestContext, account Account, items []Employee, decision CheckResult) ([]Employee, error) {
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

// AuthzSnapshotCache stores reusable authorization decisions between permission-version changes.
type AuthzSnapshotCache interface {
	GetAuthzSnapshot(ctx context.Context, key string) (CheckResult, bool, error)
	SetAuthzSnapshot(ctx context.Context, key string, result CheckResult, ttl time.Duration) error
	InvalidateTenant(ctx context.Context, tenantID string) error
}

func (c *Service) authzSnapshotKey(ctx RequestContext, account Account, req CheckRequest, version int64) string {
	payload, _ := json.Marshal(map[string]any{
		"tenant_id":                 ctx.TenantID,
		"account_id":                account.ID,
		"assumed_role_session_id":   ctx.AssumedRoleSessionID,
		"permission_version":        version,
		"application_code":          req.ApplicationCode,
		"resource_type":             req.ResourceType,
		"resource_id":               req.ResourceID,
		"resource":                  req.Resource,
		"action":                    req.Action,
		"target":                    req.Target,
		"target_employee_id":        req.TargetEmployeeID,
		"context":                   req.Context,
		"approval_confirmation_set": ctx.ApprovalConfirmed,
	})
	sum := sha1.Sum(payload)
	return fmt.Sprintf("authz:snapshot:%s:%s", ctx.TenantID, hex.EncodeToString(sum[:]))
}

func (c *Service) shouldUseAuthzSnapshot(ctx RequestContext) bool {
	return ctx.AssumedRoleSessionID == ""
}

func (c *Service) getAuthzSnapshot(ctx context.Context, key string) (CheckResult, bool) {
	if c.authzSnapshot == nil {
		return CheckResult{}, false
	}
	result, ok, err := c.authzSnapshot.GetAuthzSnapshot(ctx, key)
	if err != nil || !ok {
		return CheckResult{}, false
	}
	return result, true
}

func (c *Service) setAuthzSnapshot(ctx context.Context, key string, result CheckResult) {
	if c.authzSnapshot == nil {
		return
	}
	_ = c.authzSnapshot.SetAuthzSnapshot(ctx, key, result, 5*time.Minute)
}

func (c *Service) invalidateAuthzSnapshots(ctx context.Context, tenantID string) {
	if c.authzSnapshot == nil {
		return
	}
	_ = c.authzSnapshot.InvalidateTenant(ctx, tenantID)
}

func (c *Service) requireServiceAuthz(ctx RequestContext, app ApplicationCode, resource ResourceType, action Action, resourceID string) (Account, CheckResult, error) {
	account, decision, _, err := c.Authorize(ctx, CheckRequest{
		ApplicationCode: app,
		ResourceType:    resource,
		ResourceID:      resourceID,
		Action:          action,
	}, AuditTarget{})
	return account, decision, err
}

func (c IAMService) requireIAMAuthz(ctx RequestContext, resource ResourceType, action Action, resourceID string) (Account, CheckResult, error) {
	return c.Service.requireServiceAuthz(ctx, AppIAM, resource, action, resourceID)
}

func (c WorkflowService) requireWorkflowAuthz(ctx RequestContext, resource ResourceType, action Action, resourceID string) (Account, CheckResult, error) {
	return c.Service.requireServiceAuthz(ctx, AppWorkflow, resource, action, resourceID)
}

func (c AgentService) requireAgentAuthz(ctx RequestContext, resource ResourceType, action Action, resourceID string) (Account, CheckResult, error) {
	return c.Service.requireServiceAuthz(ctx, AppAgent, resource, action, resourceID)
}
