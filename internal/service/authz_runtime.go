package service

import (
	"strings"
	"time"

	authzpkg "nexus-pro-be/internal/domain/authz"
	"nexus-pro-be/internal/utils"
)

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
		allowed, err := c.relationships.CheckRelationship(goContext(ctx), authzpkg.RelationshipCheck{
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
	out := make([]Permission, 0, len(authzpkg.DefaultRoutePolicies))
	for _, policy := range authzpkg.DefaultRoutePolicies {
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
