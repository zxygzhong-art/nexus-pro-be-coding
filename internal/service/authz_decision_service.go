package service

import (
	"fmt"
	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
	"strings"
	"time"
)

type authzGrant struct {
	Permission      Permission
	PermissionSetID string
	Source          string
	SourceKind      authzGrantSourceKind
	Effect          string
	DataScope       *DataScope
	ExpiresAt       *time.Time
}

type authzGrantSourceKind string

const (
	authzGrantSourceNormal  authzGrantSourceKind = "normal"
	authzGrantSourceAssumed authzGrantSourceKind = "assumed"
)

type authzGrantCollectionOptions struct {
	addUserGroupIDs        []string
	removeUserGroupIDs     map[string]struct{}
	addPermissionSetIDs    []string
	removePermissionSetIDs map[string]struct{}
	assumeRoleID           string
	permissionSetChanges   map[string]authzPermissionSetMutation
}

type authzPermissionSetMutation struct {
	addPermissions    []Permission
	removePermissions []string
}

type authzScopeCacheEntry struct {
	scope      Scope
	conditions map[string]any
	err        error
}

type authzBoundaryScopeCacheEntry struct {
	authzScopeCacheEntry
	hasScope bool
}

// authzDecisionScopeCache is request-local. Projection reuses it while checking
// each materialized route so employee/org lookups are bounded by grants rather
// than multiplied by the number of routes advertised through /me.
type authzDecisionScopeCache struct {
	grantConditions map[string]authzScopeCacheEntry
	boundary        *authzBoundaryScopeCacheEntry
}

type authzTrace struct {
	evaluatedGrants []domain.AuthzEvaluatedGrant
	denySources     []string
	boundaryEffects []domain.AuthzBoundaryEffect
	scopeDerivation domain.AuthzScopeDerivation
}

// evaluateAuthz 處理 evaluate 授權的服務流程。
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
		if useSnapshot && result.Allowed {
			c.setAuthzSnapshot(goContext(ctx), snapshotKey, result, earliestAuthzGrantExpiry(grants))
		}
		return result
	}

	result, err := c.evaluateAuthzDecision(ctx, account, req, grants, setIDs, assumedRole, boundary, nil)
	if err != nil {
		return CheckResult{}, err
	}
	return cacheResult(result), nil
}

// evaluateCurrentAccessProjection validates an explicitly supplied assumed session,
// then checks the base account's permission to read its own authoritative projection.
// Invalid, expired, revoked, cross-tenant, or cross-account sessions never fall back.
func (c *Service) evaluateCurrentAccessProjection(ctx RequestContext, account Account, req CheckRequest) (CheckResult, error) {
	if strings.TrimSpace(ctx.AssumedRoleSessionID) != "" {
		if _, _, err := c.activeAssumableRole(ctx, account); err != nil {
			return CheckResult{}, err
		}
	}
	baseCtx := ctx
	baseCtx.AssumedRoleID = ""
	baseCtx.AssumedRoleSessionID = ""
	return c.evaluateAuthz(baseCtx, account, req)
}

func (c *Service) evaluateAuthzDecision(ctx RequestContext, account Account, req CheckRequest, grants []authzGrant, setIDs []string, assumedRole *AssumedRoleDecision, boundary map[string]any, trace *authzTrace) (CheckResult, error) {
	return c.evaluateAuthzDecisionWithFieldPolicies(ctx, account, req, grants, setIDs, assumedRole, boundary, trace, true, nil)
}

// evaluateAuthzDecisionWithFieldPolicies lets caller-projection checks reuse the
// authoritative grant/boundary/scope engine without performing an unused field
// policy query for every projected permission.
func (c *Service) evaluateAuthzDecisionWithFieldPolicies(ctx RequestContext, account Account, req CheckRequest, grants []authzGrant, setIDs []string, assumedRole *AssumedRoleDecision, boundary map[string]any, trace *authzTrace, includeFieldPolicies bool, scopeCache *authzDecisionScopeCache) (CheckResult, error) {
	req = normalizeCheckRequest(req)
	matched := make([]string, 0)
	matchedBy := make([]string, 0)
	deniedBy := make([]string, 0)
	relationshipDeniedBy := make([]string, 0)
	var normalScope Scope
	var normalConditions map[string]any
	var normalMatched bool
	var normalUsageScopeInvalid bool
	var assumedScope Scope
	var assumedConditions map[string]any
	var assumedMatched bool
	var assumedUsageScopeInvalid bool
	riskLevel := riskLevelForRoute(req)
	permissionKey := permissionKey(req.ApplicationCode, req.ResourceType, req.Action)

	for grantIndex, grant := range grants {
		if !permissionMatches(grant.Permission, req, account) {
			if trace != nil {
				trace.addGrant(grant, false, "", "")
			}
			continue
		}
		source := grant.Source
		if source == "" {
			source = grant.PermissionSetID
		}
		effect := permissionEffect(grant)
		if effect == "deny" {
			deniedBy = append(deniedBy, source)
			if trace != nil {
				trace.addGrant(grant, true, "explicit_deny", "")
				trace.addDenySource(source)
			}
			continue
		}
		if policyDenies(boundary, permissionKey) {
			deniedBy = append(deniedBy, "permission_boundary")
			if trace != nil {
				trace.addGrant(grant, true, "permission_boundary", "")
				trace.addDenySource("permission_boundary")
				trace.addBoundaryEffect(permissionKey, "deny", true)
			}
			continue
		}
		if !policyAllows(boundary, permissionKey) {
			deniedBy = append(deniedBy, "permission_boundary")
			if trace != nil {
				trace.addGrant(grant, true, "permission_boundary", "")
				trace.addDenySource("permission_boundary")
				trace.addBoundaryEffect(permissionKey, "allow_missing", true)
			}
			continue
		}
		if relation := relationshipConstraint(grant.Permission); relation != "" {
			allowed, label, err := c.relationshipAllows(ctx, account, req, relation)
			if err != nil {
				return CheckResult{}, err
			}
			if !allowed {
				relationshipDeniedBy = append(relationshipDeniedBy, label)
				if trace != nil {
					trace.addGrant(grant, true, "relationship", "")
				}
				continue
			}
			matchedBy = append(matchedBy, label)
		}
		scope, conditions, err := c.conditionsForGrantCached(ctx, account, grant, req, grantIndex, scopeCache)
		if err != nil {
			return CheckResult{}, err
		}
		if req.ApplicationCode == AppAgent && req.ResourceType == ResourceUsage && !isTenantWideUsageScope(scope) {
			switch grant.SourceKind {
			case authzGrantSourceAssumed:
				assumedUsageScopeInvalid = true
			default:
				normalUsageScopeInvalid = true
			}
			if trace != nil {
				trace.addGrant(grant, true, "data_scope", scope)
			}
			continue
		}
		matched = append(matched, permissionLabel(grant.Permission))
		matchedBy = append(matchedBy, source)
		if isHighRiskPermission(grant.Permission) {
			riskLevel = maxRiskLevel(riskLevel, grant.Permission.RiskLevel)
		}
		if trace != nil {
			trace.addGrant(grant, true, "", scope)
		}
		switch grant.SourceKind {
		case authzGrantSourceAssumed:
			assumedMatched = true
			assumedScope, assumedConditions = chooseScope(assumedScope, assumedConditions, scope, conditions)
		default:
			normalMatched = true
			normalScope, normalConditions = chooseScope(normalScope, normalConditions, scope, conditions)
		}
	}
	if c.relationships != nil && len(matched) == 0 && req.ResourceID != "" && supportsImplicitRelationshipFallback(req) {
		object := relationshipObject(req)
		switch {
		case policyDenies(boundary, permissionKey):
			deniedBy = append(deniedBy, "permission_boundary")
			if trace != nil {
				trace.addDenySource("permission_boundary")
				trace.addBoundaryEffect(permissionKey, "deny", true)
			}
		case !policyAllows(boundary, permissionKey):
			deniedBy = append(deniedBy, "permission_boundary")
			if trace != nil {
				trace.addDenySource("permission_boundary")
				trace.addBoundaryEffect(permissionKey, "allow_missing", true)
			}
		default:
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
				normalMatched = true
				normalScope, normalConditions = chooseScope(normalScope, normalConditions, ScopeObject, map[string]any{
					"tenant_id": ctx.TenantID,
					"object":    object,
					"relation":  req.Action,
				})
			}
		}
	}

	chosenScope, chosenConditions := normalScope, normalConditions
	scopeIntersectionEmpty := false
	if assumedRole != nil {
		if !normalMatched || !assumedMatched {
			scopeIntersectionEmpty = true
		} else {
			chosenScope, chosenConditions, scopeIntersectionEmpty = intersectScopes(normalScope, normalConditions, assumedScope, assumedConditions)
		}
	}
	var boundaryScope Scope
	if !scopeIntersectionEmpty {
		boundaryScope, boundaryConditions, hasBoundaryScope, err := c.boundaryScopeDecisionCached(ctx, account, boundary, scopeCache)
		if err != nil {
			return CheckResult{}, err
		}
		if hasBoundaryScope {
			if trace != nil {
				trace.addBoundaryEffect(permissionKey, "scope", true)
			}
			chosenScope, chosenConditions, scopeIntersectionEmpty = intersectScopes(chosenScope, chosenConditions, boundaryScope, boundaryConditions)
		}
	}
	if trace != nil {
		trace.setScopeDerivation(normalScope, assumedScope, boundaryScope, chosenScope, scopeIntersectionEmpty)
	}

	matchedPermissions := uniqueStrings(matched)
	matchedSources := uniqueStrings(matchedBy)
	var fieldPolicies map[string]string
	if includeFieldPolicies {
		resolvedFieldPolicies, err := c.fieldPolicyDecision(ctx, req.ApplicationCode, req.ResourceType, permissionKey, matchedPermissions)
		if err != nil {
			return CheckResult{}, err
		}
		fieldPolicies = resolvedFieldPolicies
	}
	result := CheckResult{
		Allowed:            len(matched) > 0 && len(deniedBy) == 0,
		MatchedBy:          matchedSources,
		MatchedPermissions: matchedPermissions,
		PermissionSetIDs:   uniqueStrings(setIDs),
		Scope:              chosenScope,
		EffectiveScope:     chosenScope,
		Conditions:         chosenConditions,
		FieldPolicies:      fieldPolicies,
		PermissionBoundary: boundary,
		RiskLevel:          riskLevel,
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
		return result, nil
	}
	usageScopeInvalid := !normalMatched && normalUsageScopeInvalid
	if assumedRole != nil {
		usageScopeInvalid = usageScopeInvalid || (!assumedMatched && assumedUsageScopeInvalid)
	}
	if req.ApplicationCode == AppAgent && req.ResourceType == ResourceUsage &&
		len(matched) > 0 && !scopeIntersectionEmpty && !isTenantWideUsageScope(chosenScope) {
		usageScopeInvalid = true
	}
	if usageScopeInvalid {
		result.Allowed = false
		result.Reason = "data scope denied"
		result.MissingPermissions = []string{permissionKey}
		return result, nil
	}
	if len(matched) == 0 {
		if len(relationshipDeniedBy) > 0 {
			result.Reason = "relationship denied"
			result.MissingPermissions = []string{permissionKey}
			result.MatchedBy = uniqueStrings(relationshipDeniedBy)
			return result, nil
		}
		result.Reason = "missing permission"
		result.MissingPermissions = []string{permissionKey}
		return result, nil
	}
	if scopeIntersectionEmpty {
		result.Allowed = false
		result.Reason = "scope intersection empty"
		result.MissingPermissions = []string{permissionKey}
		return result, nil
	}
	result.Reason = "matched permission"
	return result, nil
}

// implicitRelationshipFallbackRelations mirrors the object types and relations declared in ops/openfga/model.json.
var implicitRelationshipFallbackRelations = map[string]map[Action]struct{}{
	"tenant": {
		Action("admin"): {}, Action("member"): {}, Action("security_admin"): {},
	},
	"org_unit": {
		Action("editor"): {}, Action("manager"): {}, Action("member"): {}, Action("member_recursive"): {},
		Action("parent"): {}, Action("tenant"): {}, Action("viewer"): {},
	},
	"user_group": {
		Action("manager"): {}, Action("member"): {},
	},
	"assumable_role": {
		Action("approver"): {}, Action("can_approve"): {}, Action("can_assume"): {}, Action("tenant"): {},
		Action("trusted_group"): {}, Action("trusted_user"): {},
	},
	"agent_tool": {
		Action("can_run"): {}, Action("runner"): {}, Action("tenant"): {},
	},
	"hr.employee": {
		ActionDelete: {}, ActionInvite: {}, Action("manager"): {}, Action("org"): {}, Action("owner"): {},
		ActionRead: {}, ActionStatusTransition: {}, ActionUpdate: {}, ActionUpdateStatus: {},
	},
	"agent.knowledge_article": {
		ActionRead: {}, Action("viewer"): {},
	},
}

// supportsImplicitRelationshipFallback permits only object/relation pairs present in the OpenFGA model.
func supportsImplicitRelationshipFallback(req CheckRequest) bool {
	relations, ok := implicitRelationshipFallbackRelations[routeResourceName(req.ApplicationCode, req.ResourceType)]
	if !ok {
		return false
	}
	_, ok = relations[req.Action]
	return ok
}

func isTenantWideUsageScope(scope Scope) bool {
	return scope == ScopeAll || scope == ScopeTenant || scope == ScopeSystem
}

func (c *Service) conditionsForGrantCached(ctx RequestContext, account Account, grant authzGrant, req CheckRequest, grantIndex int, cache *authzDecisionScopeCache) (Scope, map[string]any, error) {
	if cache == nil {
		return c.conditionsForGrant(ctx, account, grant, req)
	}
	if cache.grantConditions == nil {
		cache.grantConditions = map[string]authzScopeCacheEntry{}
	}
	// An empty permission scope is request-sensitive only for the dedicated
	// agent usage resource, where it deliberately fails closed instead of
	// inheriting the legacy ScopeAll default.
	usageWithoutScope := grant.DataScope == nil && grant.Permission.Scope == "" &&
		req.ApplicationCode == AppAgent && req.ResourceType == ResourceUsage
	key := fmt.Sprintf("%d|%t", grantIndex, usageWithoutScope)
	if cached, ok := cache.grantConditions[key]; ok {
		return cached.scope, utils.CopyStringMap(cached.conditions), cached.err
	}
	scope, conditions, err := c.conditionsForGrant(ctx, account, grant, req)
	cache.grantConditions[key] = authzScopeCacheEntry{
		scope:      scope,
		conditions: utils.CopyStringMap(conditions),
		err:        err,
	}
	return scope, conditions, err
}

func (c *Service) boundaryScopeDecisionCached(ctx RequestContext, account Account, boundary map[string]any, cache *authzDecisionScopeCache) (Scope, map[string]any, bool, error) {
	if cache == nil {
		return c.boundaryScopeDecision(ctx, account, boundary)
	}
	if cache.boundary != nil {
		return cache.boundary.scope, utils.CopyStringMap(cache.boundary.conditions), cache.boundary.hasScope, cache.boundary.err
	}
	scope, conditions, hasScope, err := c.boundaryScopeDecision(ctx, account, boundary)
	cache.boundary = &authzBoundaryScopeCacheEntry{
		authzScopeCacheEntry: authzScopeCacheEntry{
			scope:      scope,
			conditions: utils.CopyStringMap(conditions),
			err:        err,
		},
		hasScope: hasScope,
	}
	return scope, conditions, hasScope, err
}

// collectAuthzGrants 處理 collect 授權 grants 的服務流程。
func (c *Service) collectAuthzGrants(ctx RequestContext, account Account) ([]authzGrant, []string, *AssumedRoleDecision, map[string]any, error) {
	return c.collectAuthzGrantsWithOptions(ctx, account, authzGrantCollectionOptions{})
}

func (c *Service) collectAuthzGrantsWithOptions(ctx RequestContext, account Account, opts authzGrantCollectionOptions) ([]authzGrant, []string, *AssumedRoleDecision, map[string]any, error) {
	grants := make([]authzGrant, 0)
	setIDs := make([]string, 0)

	addSet := func(setID, source, effect string, scope *DataScope, sourceKind authzGrantSourceKind, expiresAt *time.Time) error {
		setID = strings.TrimSpace(setID)
		if setID == "" {
			return nil
		}
		if _, removed := opts.removePermissionSetIDs[setID]; removed {
			return nil
		}
		set, ok, err := c.store.GetPermissionSet(goContext(ctx), ctx.TenantID, setID)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if sourceKind == "" {
			sourceKind = authzGrantSourceNormal
		}
		if mutation, ok := opts.permissionSetChanges[set.ID]; ok {
			set.Permissions = applyPermissionSetMutation(set.Permissions, mutation)
		}
		setIDs = append(setIDs, set.ID)
		for _, perm := range set.Permissions {
			perm = normalizePermission(perm)
			grant := authzGrant{
				Permission:      perm,
				PermissionSetID: set.ID,
				Source:          source,
				SourceKind:      sourceKind,
				Effect:          utils.FirstNonEmpty(effect, "allow"),
				DataScope:       scope,
				ExpiresAt:       expiresAt,
			}
			grants = append(grants, grant)
		}
		return nil
	}
	addAssignments := func(principalType, principalID, sourcePrefix string, sourceKind authzGrantSourceKind, inheritedExpiry *time.Time) error {
		assignments, err := c.store.ListPermissionSetAssignmentsForPrincipal(goContext(ctx), ctx.TenantID, principalType, principalID)
		if err != nil {
			return err
		}
		for _, assignment := range assignments {
			if !permissionSetAssignmentActive(assignment, c.Now()) {
				continue
			}
			if strings.TrimSpace(assignment.ConditionID) != "" && strings.TrimSpace(assignment.Effect) != "deny" {
				continue
			}
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
			if err := addSet(assignment.PermissionSetID, sourcePrefix+":"+principalID+":"+assignment.PermissionSetID, assignment.Effect, scope, sourceKind, earliestExpiry(inheritedExpiry, assignment.ExpiresAt)); err != nil {
				return err
			}
		}
		return nil
	}

	for _, id := range account.DirectPermissionSetIDs {
		if err := addSet(id, "direct:"+id, "allow", nil, authzGrantSourceNormal, nil); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	if err := addAssignments("account", account.ID, "account", authzGrantSourceNormal, nil); err != nil {
		return nil, nil, nil, nil, err
	}
	for _, id := range opts.addPermissionSetIDs {
		if err := addSet(id, "simulation:permission_set:"+strings.TrimSpace(id), "allow", nil, authzGrantSourceNormal, nil); err != nil {
			return nil, nil, nil, nil, err
		}
	}

	groups, groupExpiries, err := c.activeUserGroupsForAccountWithExpiries(ctx, account)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	groupSeen := map[string]struct{}{}
	filteredGroups := make([]UserGroup, 0, len(groups)+len(opts.addUserGroupIDs))
	for _, group := range groups {
		if _, removed := opts.removeUserGroupIDs[group.ID]; removed {
			continue
		}
		if _, ok := groupSeen[group.ID]; ok {
			continue
		}
		groupSeen[group.ID] = struct{}{}
		filteredGroups = append(filteredGroups, group)
	}
	for _, id := range opts.addUserGroupIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, removed := opts.removeUserGroupIDs[id]; removed {
			continue
		}
		if _, ok := groupSeen[id]; ok {
			continue
		}
		group, ok, err := c.store.GetUserGroup(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if !ok {
			return nil, nil, nil, nil, NotFound("user group", id)
		}
		groupSeen[id] = struct{}{}
		filteredGroups = append(filteredGroups, group)
	}
	for _, group := range filteredGroups {
		groupExpiry := groupExpiries[group.ID]
		for _, id := range group.PermissionSetIDs {
			if err := addSet(id, "group:"+group.ID+":"+id, "allow", nil, authzGrantSourceNormal, groupExpiry); err != nil {
				return nil, nil, nil, nil, err
			}
		}
		if err := addAssignments("user_group", group.ID, "group", authzGrantSourceNormal, groupExpiry); err != nil {
			return nil, nil, nil, nil, err
		}
	}

	var role *AssumableRole
	var session *AssumableRoleSession
	if strings.TrimSpace(opts.assumeRoleID) != "" {
		item, ok, err := c.store.GetAssumableRole(goContext(ctx), ctx.TenantID, strings.TrimSpace(opts.assumeRoleID))
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if !ok {
			return nil, nil, nil, nil, NotFound("assumable role", strings.TrimSpace(opts.assumeRoleID))
		}
		role = &item
	} else {
		var err error
		role, session, err = c.activeAssumableRole(ctx, account)
		if err != nil {
			return nil, nil, nil, nil, err
		}
	}
	var assumed *AssumedRoleDecision
	boundary := map[string]any(nil)
	if role != nil {
		assumed = &AssumedRoleDecision{RoleID: role.ID, Name: role.Name}
		boundary = utils.CopyStringMap(role.PermissionBoundary)
		for _, id := range role.PermissionSetIDs {
			if err := addSet(id, "assumable_role:"+role.ID+":"+id, "allow", nil, authzGrantSourceAssumed, nil); err != nil {
				return nil, nil, nil, nil, err
			}
		}
		if err := addAssignments("assumable_role", role.ID, "assumable_role", authzGrantSourceAssumed, nil); err != nil {
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

// earliestExpiry 回傳兩個有效期限中較早的一個，避免繼承授權延長上層期限。
func earliestExpiry(left, right *time.Time) *time.Time {
	if left == nil {
		return right
	}
	if right == nil || left.Before(*right) {
		return left
	}
	return right
}

// earliestAuthzGrantExpiry 取得本次決策所有來源中最早的授權失效時間。
func earliestAuthzGrantExpiry(grants []authzGrant) *time.Time {
	var earliest *time.Time
	for _, grant := range grants {
		earliest = earliestExpiry(earliest, grant.ExpiresAt)
	}
	return earliest
}

func simulationOverridesToGrantOptions(overrides AuthzSimulationOverrides) (authzGrantCollectionOptions, error) {
	opts := authzGrantCollectionOptions{
		addUserGroupIDs:        uniqueStrings(overrides.AddUserGroups),
		removeUserGroupIDs:     stringsToSet(overrides.RemoveUserGroups),
		addPermissionSetIDs:    uniqueStrings(overrides.AddPermissionSets),
		removePermissionSetIDs: stringsToSet(overrides.RemovePermissionSets),
		assumeRoleID:           strings.TrimSpace(overrides.AssumeRoleID),
		permissionSetChanges:   map[string]authzPermissionSetMutation{},
	}
	for _, change := range overrides.PermissionSetChanges {
		setID := strings.TrimSpace(change.PermissionSetID)
		if setID == "" {
			return authzGrantCollectionOptions{}, BadRequest("permission_set_id is required for permission_set_changes")
		}
		mutation := opts.permissionSetChanges[setID]
		for _, label := range change.AddPermissions {
			perm, err := permissionFromSimulationLabel(label)
			if err != nil {
				return authzGrantCollectionOptions{}, err
			}
			mutation.addPermissions = append(mutation.addPermissions, perm)
		}
		mutation.removePermissions = append(mutation.removePermissions, change.RemovePermissions...)
		opts.permissionSetChanges[setID] = mutation
	}
	if len(opts.permissionSetChanges) == 0 {
		opts.permissionSetChanges = nil
	}
	return opts, nil
}

func applyPermissionSetMutation(existing []Permission, mutation authzPermissionSetMutation) []Permission {
	out := make([]Permission, 0, len(existing)+len(mutation.addPermissions))
	for _, perm := range existing {
		if permissionRemovedBySimulation(perm, mutation.removePermissions) {
			continue
		}
		out = append(out, perm)
	}
	for _, perm := range mutation.addPermissions {
		if permissionRemovedBySimulation(perm, mutation.removePermissions) {
			continue
		}
		if permissionSliceContains(out, perm) {
			continue
		}
		out = append(out, perm)
	}
	return out
}

func permissionRemovedBySimulation(perm Permission, removeLabels []string) bool {
	if len(removeLabels) == 0 {
		return false
	}
	label := permissionLabel(perm)
	key := permissionKey(normalizePermission(perm).ApplicationCode, normalizePermission(perm).ResourceType, normalizePermission(perm).Action)
	for _, remove := range removeLabels {
		remove = strings.TrimSpace(remove)
		if remove == "" {
			continue
		}
		if permissionLabelMatches(label, remove) || permissionKeyMatches(key, remove) {
			return true
		}
	}
	return false
}

func permissionSliceContains(items []Permission, want Permission) bool {
	wantLabel := permissionLabel(want)
	for _, item := range items {
		if permissionLabel(item) == wantLabel {
			return true
		}
	}
	return false
}

func permissionFromSimulationLabel(label string) (Permission, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return Permission{}, BadRequest("permission label is required")
	}
	base, scope, _ := strings.Cut(label, "#")
	base, target, _ := strings.Cut(base, ":")
	parts := strings.Split(base, ".")
	if len(parts) < 2 {
		return Permission{}, BadRequest("permission label must use resource.action format")
	}
	action := parts[len(parts)-1]
	resource := strings.Join(parts[:len(parts)-1], ".")
	if strings.TrimSpace(resource) == "" || strings.TrimSpace(action) == "" {
		return Permission{}, BadRequest("permission label must use resource.action format")
	}
	app, resourceType := splitResource(resource)
	return normalizePermission(Permission{
		ApplicationCode: app,
		ResourceType:    resourceType,
		Resource:        resource,
		Action:          Action(action),
		Target:          target,
		Scope:           Scope(scope),
		Effect:          "allow",
	}), nil
}

func authzSimulationDiff(before, after CheckResult) AuthzSimulationDiff {
	return AuthzSimulationDiff{
		AllowedChanged:            before.Allowed != after.Allowed,
		BeforeAllowed:             before.Allowed,
		AfterAllowed:              after.Allowed,
		ScopeChanged:              before.EffectiveScope != after.EffectiveScope,
		BeforeScope:               before.EffectiveScope,
		AfterScope:                after.EffectiveScope,
		AddedMatchedBy:            addedStrings(before.MatchedBy, after.MatchedBy),
		RemovedMatchedBy:          removedStrings(before.MatchedBy, after.MatchedBy),
		AddedMatchedPermissions:   addedStrings(before.MatchedPermissions, after.MatchedPermissions),
		RemovedMatchedPermissions: removedStrings(before.MatchedPermissions, after.MatchedPermissions),
	}
}

func addedStrings(before, after []string) []string {
	seen := stringsToSet(before)
	out := make([]string, 0)
	for _, item := range after {
		if strings.TrimSpace(item) == "" {
			continue
		}
		if _, ok := seen[item]; !ok {
			out = append(out, item)
		}
	}
	return uniqueStrings(out)
}

func removedStrings(before, after []string) []string {
	seen := stringsToSet(after)
	out := make([]string, 0)
	for _, item := range before {
		if strings.TrimSpace(item) == "" {
			continue
		}
		if _, ok := seen[item]; !ok {
			out = append(out, item)
		}
	}
	return uniqueStrings(out)
}

func stringsToSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out[value] = struct{}{}
	}
	return out
}

func (t *authzTrace) addGrant(grant authzGrant, matched bool, excludedBy string, scope Scope) {
	if t == nil {
		return
	}
	source, sourceID := authzTraceGrantSource(grant.Source, grant.PermissionSetID)
	if scope == "" {
		scope = authzGrantScope(grant)
	}
	var excluded *string
	if excludedBy != "" {
		value := excludedBy
		excluded = &value
	}
	t.evaluatedGrants = append(t.evaluatedGrants, domain.AuthzEvaluatedGrant{
		Source:          source,
		SourceID:        sourceID,
		PermissionSetID: grant.PermissionSetID,
		Permission:      permissionLabel(grant.Permission),
		Effect:          permissionEffect(grant),
		Matched:         matched,
		Scope:           scope,
		ExcludedBy:      excluded,
	})
}

func (t *authzTrace) addDenySource(source string) {
	if t == nil {
		return
	}
	t.denySources = append(t.denySources, source)
}

func (t *authzTrace) addBoundaryEffect(permission, effect string, matched bool) {
	if t == nil {
		return
	}
	t.boundaryEffects = append(t.boundaryEffects, domain.AuthzBoundaryEffect{
		Source:     "permission_boundary",
		Permission: permission,
		Effect:     effect,
		Matched:    matched,
		ExcludedBy: "permission_boundary",
	})
}

func (t *authzTrace) setScopeDerivation(normal, assumed, boundary, final Scope, intersectionEmpty bool) {
	if t == nil {
		return
	}
	t.scopeDerivation = domain.AuthzScopeDerivation{
		Normal:            normal,
		Assumed:           assumed,
		Boundary:          boundary,
		Final:             final,
		IntersectionEmpty: intersectionEmpty,
	}
}

func (t *authzTrace) response(decision CheckResult) AuthzExplainResponse {
	if t == nil {
		return AuthzExplainResponse{Decision: decision, EvaluatedGrants: []domain.AuthzEvaluatedGrant{}}
	}
	evaluatedGrants := t.evaluatedGrants
	if evaluatedGrants == nil {
		evaluatedGrants = []domain.AuthzEvaluatedGrant{}
	}
	return AuthzExplainResponse{
		Decision:        decision,
		EvaluatedGrants: evaluatedGrants,
		DenySources:     uniqueStrings(t.denySources),
		BoundaryEffects: uniqueBoundaryEffects(t.boundaryEffects),
		ScopeDerivation: t.scopeDerivation,
	}
}

func uniqueBoundaryEffects(items []domain.AuthzBoundaryEffect) []domain.AuthzBoundaryEffect {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]domain.AuthzBoundaryEffect, 0, len(items))
	for _, item := range items {
		key := item.Source + "|" + item.Permission + "|" + item.Effect + "|" + fmt.Sprint(item.Matched) + "|" + item.ExcludedBy
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func authzGrantScope(grant authzGrant) Scope {
	if grant.DataScope != nil {
		return Scope(grant.DataScope.ScopeType)
	}
	if grant.Permission.Scope != "" {
		return grant.Permission.Scope
	}
	return ScopeAll
}

func authzTraceGrantSource(source, permissionSetID string) (string, string) {
	if source == "" {
		return "permission_set", permissionSetID
	}
	parts := strings.Split(source, ":")
	switch parts[0] {
	case "direct":
		return "direct", valueAt(parts, 1, permissionSetID)
	case "account":
		return "account", valueAt(parts, 1, "")
	case "group":
		return "user_group", valueAt(parts, 1, "")
	case "assumable_role":
		return "assumable_role", valueAt(parts, 1, "")
	case "simulation":
		if len(parts) >= 3 && parts[1] == "permission_set" {
			return "simulation_permission_set", parts[2]
		}
		return "simulation", strings.Join(parts[1:], ":")
	default:
		return parts[0], strings.Join(parts[1:], ":")
	}
}

func valueAt(values []string, index int, fallback string) string {
	if index >= 0 && index < len(values) && values[index] != "" {
		return values[index]
	}
	return fallback
}

// activeAssumableRole 處理啟用中 assumable 角色的服務流程。
