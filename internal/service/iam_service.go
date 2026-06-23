package service

import (
	"nexus-pro-be/internal/utils"
	"strings"
	"time"
)

// IAMService implements user group, permission, data scope, and role workflows.
type IAMService struct {
	*Service
	store iamStore
}

const (
	defaultAssumableRoleSessionSeconds = 8 * 60 * 60
	maxAssumableRoleSessionSeconds     = 12 * 60 * 60
)

// IAM returns the IAM service facade.
func (c *Service) IAM() IAMService {
	return IAMService{Service: c, store: c.store}
}

// ListPermissionSets returns permission sets visible to the current account.
func (c IAMService) ListPermissionSets(ctx RequestContext) ([]PermissionSet, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourcePermissionSet, ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListPermissionSets(goContext(ctx), ctx.TenantID)
}

// ListPermissionSetPage returns paginated permission sets.
func (c IAMService) ListPermissionSetPage(ctx RequestContext, page PageRequest) (PageResponse[PermissionSet], error) {
	items, err := c.ListPermissionSets(ctx)
	if err != nil {
		return PageResponse[PermissionSet]{}, err
	}
	items = utils.SortPermissionSets(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

// CreatePermissionSet creates a permission set and bumps the tenant permission version.
func (c IAMService) CreatePermissionSet(ctx RequestContext, input CreatePermissionSetInput) (PermissionSet, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourcePermissionSet, ActionCreate, ""); err != nil {
		return PermissionSet{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return PermissionSet{}, BadRequest("permission set name is required")
	}
	for _, perm := range input.Permissions {
		perm = normalizePermission(perm)
		if strings.TrimSpace(perm.Resource) == "" || strings.TrimSpace(string(perm.Action)) == "" {
			return PermissionSet{}, BadRequest("permission resource and action are required")
		}
	}
	set := PermissionSet{
		ID:          utils.NewID("ps"),
		TenantID:    ctx.TenantID,
		Name:        strings.TrimSpace(input.Name),
		Description: strings.TrimSpace(input.Description),
		Permissions: normalizePermissions(input.Permissions),
		CreatedAt:   c.Now(),
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		if err := tx.store.UpsertPermissionSet(goContext(ctx), set); err != nil {
			return err
		}
		if err := tx.touchAuthzConfig(ctx, "iam.permission_set.upsert", map[string]any{"permission_set_id": set.ID}); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.permission_set.create", "permission_set", set.ID, "medium", map[string]any{"name": set.Name})
	}); err != nil {
		return PermissionSet{}, err
	}
	c.logInfo(ctx, "permission set created",
		"permission_set_id", set.ID,
		"permission_count", len(set.Permissions),
	)
	return set, nil
}

// ListPermissions returns route-derived permissions available for assignment.
func (c IAMService) ListPermissions(ctx RequestContext) ([]Permission, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceType("permission"), ActionRead, ""); err != nil {
		return nil, err
	}
	return defaultPermissions(), nil
}

// ListPermissionPage returns paginated route-derived permissions.
func (c IAMService) ListPermissionPage(ctx RequestContext, page PageRequest) (PageResponse[Permission], error) {
	items, err := c.ListPermissions(ctx)
	if err != nil {
		return PageResponse[Permission]{}, err
	}
	return utils.PageResponse(items, page), nil
}

// ListUserGroups returns user groups visible to the current account.
func (c IAMService) ListUserGroups(ctx RequestContext) ([]UserGroup, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceUserGroup, ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListUserGroups(goContext(ctx), ctx.TenantID)
}

// ListUserGroupPage returns paginated user groups.
func (c IAMService) ListUserGroupPage(ctx RequestContext, page PageRequest) (PageResponse[UserGroup], error) {
	items, err := c.ListUserGroups(ctx)
	if err != nil {
		return PageResponse[UserGroup]{}, err
	}
	items = utils.SortUserGroups(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

// CreateUserGroup creates a group and updates account memberships.
func (c IAMService) CreateUserGroup(ctx RequestContext, input CreateUserGroupInput) (UserGroup, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceUserGroup, ActionCreate, ""); err != nil {
		return UserGroup{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return UserGroup{}, BadRequest("user group name is required")
	}

	for _, id := range input.PermissionSetIDs {
		if _, ok, err := c.store.GetPermissionSet(goContext(ctx), ctx.TenantID, id); err != nil {
			return UserGroup{}, err
		} else if !ok {
			return UserGroup{}, NotFound("permission set", id)
		}
	}
	for _, id := range input.MemberAccountIDs {
		if _, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, id); err != nil {
			return UserGroup{}, err
		} else if !ok {
			return UserGroup{}, NotFound("account", id)
		}
	}

	group := UserGroup{
		ID:               utils.NewID("ug"),
		TenantID:         ctx.TenantID,
		Name:             strings.TrimSpace(input.Name),
		Description:      strings.TrimSpace(input.Description),
		MemberAccountIDs: uniqueStrings(input.MemberAccountIDs),
		PermissionSetIDs: uniqueStrings(input.PermissionSetIDs),
		CreatedAt:        c.Now(),
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		if err := tx.store.UpsertUserGroup(goContext(ctx), group); err != nil {
			return err
		}
		for _, accountID := range group.MemberAccountIDs {
			if err := tx.store.AddAccountGroup(goContext(ctx), ctx.TenantID, accountID, group.ID); err != nil {
				return err
			}
		}
		if err := tx.touchAuthzConfig(ctx, "iam.user_group.upsert", map[string]any{"user_group_id": group.ID}); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.user_group.create", "user_group", group.ID, "medium", map[string]any{"name": group.Name})
	}); err != nil {
		return UserGroup{}, err
	}
	c.logInfo(ctx, "user group created",
		"user_group_id", group.ID,
		"member_count", len(group.MemberAccountIDs),
		"permission_set_count", len(group.PermissionSetIDs),
	)
	return group, nil
}

// ListPermissionSetAssignments returns permission-set assignments visible to the caller.
func (c IAMService) ListPermissionSetAssignments(ctx RequestContext) ([]PermissionSetAssignment, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourcePermissionAssign, ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListPermissionSetAssignments(goContext(ctx), ctx.TenantID)
}

// ListPermissionSetAssignmentPage returns paginated permission-set assignments.
func (c IAMService) ListPermissionSetAssignmentPage(ctx RequestContext, page PageRequest) (PageResponse[PermissionSetAssignment], error) {
	items, err := c.ListPermissionSetAssignments(ctx)
	if err != nil {
		return PageResponse[PermissionSetAssignment]{}, err
	}
	items = utils.SortPermissionSetAssignments(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

// CreatePermissionSetAssignment grants a permission set to a principal with a data scope.
func (c IAMService) CreatePermissionSetAssignment(ctx RequestContext, input CreatePermissionSetAssignmentInput) (PermissionSetAssignment, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourcePermissionAssign, ActionCreate, ""); err != nil {
		return PermissionSetAssignment{}, err
	}
	principalType := strings.TrimSpace(input.PrincipalType)
	principalID := strings.TrimSpace(input.PrincipalID)
	permissionSetID := strings.TrimSpace(input.PermissionSetID)
	if principalType == "" || principalID == "" || permissionSetID == "" {
		return PermissionSetAssignment{}, BadRequest("principal_type, principal_id and permission_set_id are required")
	}
	if err := c.validatePermissionSetAssignmentPrincipal(ctx, principalType, principalID); err != nil {
		return PermissionSetAssignment{}, err
	}
	if _, ok, err := c.store.GetPermissionSet(goContext(ctx), ctx.TenantID, permissionSetID); err != nil {
		return PermissionSetAssignment{}, err
	} else if !ok {
		return PermissionSetAssignment{}, NotFound("permission set", permissionSetID)
	}
	effect := strings.TrimSpace(input.Effect)
	if effect == "" {
		effect = "allow"
	}
	if effect != "allow" && effect != "deny" {
		return PermissionSetAssignment{}, BadRequest("effect must be allow or deny")
	}
	startsAt, err := optionalDateTime(input.StartsAt)
	if err != nil {
		return PermissionSetAssignment{}, BadRequest("starts_at must be RFC3339 or YYYY-MM-DD")
	}
	expiresAt, err := optionalDateTime(input.ExpiresAt)
	if err != nil {
		return PermissionSetAssignment{}, BadRequest("expires_at must be RFC3339 or YYYY-MM-DD")
	}
	dataScopeID := strings.TrimSpace(input.DataScopeID)
	if dataScopeID != "" {
		if _, ok, err := c.store.GetDataScope(goContext(ctx), ctx.TenantID, dataScopeID); err != nil {
			return PermissionSetAssignment{}, err
		} else if !ok {
			return PermissionSetAssignment{}, NotFound("data scope", dataScopeID)
		}
	}
	assignment := PermissionSetAssignment{
		ID:              utils.NewID("psa"),
		TenantID:        ctx.TenantID,
		PrincipalType:   principalType,
		PrincipalID:     principalID,
		PermissionSetID: permissionSetID,
		Effect:          effect,
		DataScopeID:     dataScopeID,
		ConditionID:     strings.TrimSpace(input.ConditionID),
		StartsAt:        startsAt,
		ExpiresAt:       expiresAt,
		CreatedAt:       c.Now(),
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		if err := tx.store.UpsertPermissionSetAssignment(goContext(ctx), assignment); err != nil {
			return err
		}
		if err := tx.touchAuthzConfig(ctx, "iam.permission_assignment.upsert", map[string]any{"assignment_id": assignment.ID}); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.permission_assignment.create", "permission_set_assignment", assignment.ID, "high", map[string]any{
			"principal_type": assignment.PrincipalType,
			"principal_id":   assignment.PrincipalID,
			"permission_set": assignment.PermissionSetID,
			"effect":         assignment.Effect,
		})
	}); err != nil {
		return PermissionSetAssignment{}, err
	}
	c.logWarn(ctx, "permission set assignment created",
		"assignment_id", assignment.ID,
		"principal_type", assignment.PrincipalType,
		"principal_id", assignment.PrincipalID,
		"permission_set_id", assignment.PermissionSetID,
		"effect", assignment.Effect,
		"data_scope_id", assignment.DataScopeID,
	)
	return assignment, nil
}

func (c IAMService) validatePermissionSetAssignmentPrincipal(ctx RequestContext, principalType, principalID string) error {
	switch PrincipalType(principalType) {
	case PrincipalTypeAccount:
		if _, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, principalID); err != nil {
			return err
		} else if !ok {
			return NotFound("account", principalID)
		}
	case PrincipalTypeUserGroup:
		if _, ok, err := c.store.GetUserGroup(goContext(ctx), ctx.TenantID, principalID); err != nil {
			return err
		} else if !ok {
			return NotFound("user group", principalID)
		}
	case PrincipalTypeAssumableRole:
		if _, ok, err := c.store.GetAssumableRole(goContext(ctx), ctx.TenantID, principalID); err != nil {
			return err
		} else if !ok {
			return NotFound("assumable role", principalID)
		}
	default:
		return BadRequest("principal_type must be account, user_group or assumable_role")
	}
	return nil
}

// ListDataScopes returns data scopes visible to the current account.
func (c IAMService) ListDataScopes(ctx RequestContext) ([]DataScope, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceDataScope, ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListDataScopes(goContext(ctx), ctx.TenantID)
}

// ListDataScopePage returns paginated data scopes.
func (c IAMService) ListDataScopePage(ctx RequestContext, page PageRequest) (PageResponse[DataScope], error) {
	items, err := c.ListDataScopes(ctx)
	if err != nil {
		return PageResponse[DataScope]{}, err
	}
	items = utils.SortDataScopes(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

// CreateDataScope creates a reusable data visibility scope.
func (c IAMService) CreateDataScope(ctx RequestContext, input CreateDataScopeInput) (DataScope, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceDataScope, ActionCreate, ""); err != nil {
		return DataScope{}, err
	}
	if strings.TrimSpace(input.Code) == "" || strings.TrimSpace(input.Name) == "" {
		return DataScope{}, BadRequest("data scope code and name are required")
	}
	scopeType := strings.TrimSpace(input.ScopeType)
	if scopeType == "" {
		scopeType = strings.TrimSpace(input.Code)
	}
	if !validDataScopeType(scopeType) {
		return DataScope{}, BadRequest("unsupported data scope type")
	}
	scope := DataScope{
		ID:        utils.NewID("ds"),
		TenantID:  ctx.TenantID,
		Code:      strings.TrimSpace(input.Code),
		Name:      strings.TrimSpace(input.Name),
		ScopeType: scopeType,
		Params:    utils.CopyStringMap(input.Params),
		CreatedAt: c.Now(),
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		if err := tx.store.UpsertDataScope(goContext(ctx), scope); err != nil {
			return err
		}
		if err := tx.touchAuthzConfig(ctx, "iam.data_scope.upsert", map[string]any{"data_scope_id": scope.ID}); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.data_scope.create", "data_scope", scope.ID, "medium", map[string]any{"code": scope.Code})
	}); err != nil {
		return DataScope{}, err
	}
	return scope, nil
}

// ListFieldPolicies returns field policies filtered by optional application and resource.
func (c IAMService) ListFieldPolicies(ctx RequestContext, applicationCode, resourceType string) ([]FieldPolicy, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceFieldPolicy, ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListFieldPolicies(goContext(ctx), ctx.TenantID, strings.TrimSpace(applicationCode), strings.TrimSpace(resourceType))
}

// ListFieldPolicyPage returns paginated field policies.
func (c IAMService) ListFieldPolicyPage(ctx RequestContext, applicationCode, resourceType string, page PageRequest) (PageResponse[FieldPolicy], error) {
	items, err := c.ListFieldPolicies(ctx, applicationCode, resourceType)
	if err != nil {
		return PageResponse[FieldPolicy]{}, err
	}
	items = utils.SortFieldPolicies(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

// CreateFieldPolicy creates a field-level visibility or masking policy.
func (c IAMService) CreateFieldPolicy(ctx RequestContext, input CreateFieldPolicyInput) (FieldPolicy, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceFieldPolicy, ActionCreate, ""); err != nil {
		return FieldPolicy{}, err
	}
	if strings.TrimSpace(input.ApplicationCode) == "" || strings.TrimSpace(input.ResourceType) == "" || strings.TrimSpace(input.FieldName) == "" {
		return FieldPolicy{}, BadRequest("application_code, resource_type and field_name are required")
	}
	effect := strings.TrimSpace(input.Effect)
	switch effect {
	case "allow", "deny", "mask", "readonly", "hide":
	default:
		return FieldPolicy{}, BadRequest("field policy effect must be allow, deny, mask, readonly or hide")
	}
	policy := FieldPolicy{
		ID:              utils.NewID("fp"),
		TenantID:        ctx.TenantID,
		ApplicationCode: strings.TrimSpace(input.ApplicationCode),
		ResourceType:    strings.TrimSpace(input.ResourceType),
		FieldName:       strings.TrimSpace(input.FieldName),
		Effect:          effect,
		MaskStrategy:    strings.TrimSpace(input.MaskStrategy),
		PermissionID:    strings.TrimSpace(input.PermissionID),
		CreatedAt:       c.Now(),
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		if err := tx.store.UpsertFieldPolicy(goContext(ctx), policy); err != nil {
			return err
		}
		if err := tx.touchAuthzConfig(ctx, "iam.field_policy.upsert", map[string]any{"field_policy_id": policy.ID}); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.field_policy.create", "field_policy", policy.ID, "high", map[string]any{
			"application_code": policy.ApplicationCode,
			"resource_type":    policy.ResourceType,
			"field_name":       policy.FieldName,
			"effect":           policy.Effect,
		})
	}); err != nil {
		return FieldPolicy{}, err
	}
	return policy, nil
}

func validDataScopeType(scopeType string) bool {
	switch Scope(scopeType) {
	case ScopeAll, ScopeTenant, ScopeSelf, ScopeOwn, ScopeObject, ScopeDepartment, ScopeDepartmentSubtree, ScopeDirectReports, ScopeAssignedOrgUnits, ScopeCustomCondition, ScopeSystem:
		return true
	default:
		return false
	}
}

// ListAssumableRoles returns roles the current account can inspect.
func (c IAMService) ListAssumableRoles(ctx RequestContext) ([]AssumableRole, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceAssumableRole, ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListAssumableRoles(goContext(ctx), ctx.TenantID)
}

// ListAssumableRolePage returns paginated assumable roles.
func (c IAMService) ListAssumableRolePage(ctx RequestContext, page PageRequest) (PageResponse[AssumableRole], error) {
	items, err := c.ListAssumableRoles(ctx)
	if err != nil {
		return PageResponse[AssumableRole]{}, err
	}
	items = utils.SortAssumableRoles(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

// CreateAssumableRole creates a temporary role definition.
func (c IAMService) CreateAssumableRole(ctx RequestContext, input CreateAssumableRoleInput) (AssumableRole, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceAssumableRole, ActionCreate, ""); err != nil {
		return AssumableRole{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return AssumableRole{}, BadRequest("assumable role name is required")
	}
	if len(input.TrustPolicy) == 0 {
		return AssumableRole{}, BadRequest("assumable role trust_policy is required")
	}
	if len(input.PermissionBoundary) == 0 {
		return AssumableRole{}, BadRequest("assumable role permission_boundary is required")
	}
	if err := validateAssumableRoleSessionSeconds(input.SessionDurationSeconds); err != nil {
		return AssumableRole{}, err
	}
	for _, id := range input.PermissionSetIDs {
		if _, ok, err := c.store.GetPermissionSet(goContext(ctx), ctx.TenantID, id); err != nil {
			return AssumableRole{}, err
		} else if !ok {
			return AssumableRole{}, NotFound("permission set", id)
		}
	}
	role := AssumableRole{
		ID:                     utils.NewID("ar"),
		TenantID:               ctx.TenantID,
		Name:                   strings.TrimSpace(input.Name),
		Description:            strings.TrimSpace(input.Description),
		PermissionSetIDs:       uniqueStrings(input.PermissionSetIDs),
		Trusted:                input.Trusted,
		TrustPolicy:            utils.CopyStringMap(input.TrustPolicy),
		PermissionBoundary:     utils.CopyStringMap(input.PermissionBoundary),
		SessionDurationSeconds: input.SessionDurationSeconds,
		CreatedAt:              c.Now(),
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		if err := tx.store.UpsertAssumableRole(goContext(ctx), role); err != nil {
			return err
		}
		if err := tx.touchAuthzConfig(ctx, "iam.assumable_role.upsert", map[string]any{"assumable_role_id": role.ID}); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.assumable_role.create", "assumable_role", role.ID, "high", map[string]any{"name": role.Name})
	}); err != nil {
		return AssumableRole{}, err
	}
	c.logWarn(ctx, "assumable role created",
		"assumable_role_id", role.ID,
		"trusted", role.Trusted,
		"permission_set_count", len(role.PermissionSetIDs),
		"session_duration_seconds", role.SessionDurationSeconds,
	)
	return role, nil
}

// AssumeRole creates a bounded assumed-role session for the current account.
func (c IAMService) AssumeRole(ctx RequestContext, roleID string, input AssumeRoleInput) (AssumeRoleResponse, error) {
	account, _, err := c.requireIAMAuthz(ctx, ResourceAssumableRole, ActionAssume, roleID)
	if err != nil {
		return AssumeRoleResponse{}, err
	}
	role, ok, err := c.store.GetAssumableRole(goContext(ctx), ctx.TenantID, roleID)
	if err != nil {
		return AssumeRoleResponse{}, err
	}
	if !ok {
		return AssumeRoleResponse{}, NotFound("assumable role", roleID)
	}
	if strings.TrimSpace(input.Reason) == "" {
		return AssumeRoleResponse{}, BadRequest("assume role reason is required")
	}
	trusted, err := c.trustPolicyAllowsAccount(account, role)
	if err != nil {
		return AssumeRoleResponse{}, err
	}
	if !trusted {
		c.logWarn(ctx, "assume role denied by trust policy",
			"assumable_role_id", role.ID,
			"reason", "trust policy does not allow this account",
		)
		return AssumeRoleResponse{}, Forbidden("assumable role trust policy does not allow this account")
	}

	token := utils.NewID("sess")
	duration, err := effectiveAssumableRoleSessionDuration(role.SessionDurationSeconds, input.DurationMinutes)
	if err != nil {
		return AssumeRoleResponse{}, err
	}
	session := AssumableRoleSession{
		ID:                 token,
		TenantID:           ctx.TenantID,
		AccountID:          account.ID,
		AssumableRoleID:    role.ID,
		SessionPolicy:      utils.CopyStringMap(input.SessionPolicy),
		PermissionBoundary: utils.CopyStringMap(role.PermissionBoundary),
		ExpiresAt:          c.Now().Add(duration),
		CreatedAt:          c.Now(),
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		if err := tx.store.UpsertAssumableRoleSession(goContext(ctx), session); err != nil {
			return err
		}
		if err := tx.touchAuthzConfig(ctx, "iam.assumable_role.assume", map[string]any{
			"session_id":        token,
			"assumable_role_id": role.ID,
			"account_id":        account.ID,
			"expires_at":        session.ExpiresAt.Format(time.RFC3339),
		}); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.assumable_role.assume", "assumable_role", role.ID, "high", map[string]any{
			"session_id": token,
			"reason":     input.Reason,
			"expires_at": session.ExpiresAt.Format(time.RFC3339),
		})
	}); err != nil {
		return AssumeRoleResponse{}, err
	}
	c.logWarn(ctx, "assumable role assumed",
		"session_id", token,
		"assumable_role_id", role.ID,
		"expires_at", session.ExpiresAt.Format(time.RFC3339),
		"duration_seconds", int64(duration.Seconds()),
	)
	return AssumeRoleResponse{
		SessionID:          token,
		SessionToken:       token,
		AssumedRole:        role,
		AccountID:          account.ID,
		TenantID:           account.TenantID,
		PermissionBoundary: role.PermissionBoundary,
		ExpiresAt:          session.ExpiresAt.Format(time.RFC3339),
	}, nil
}

func validateAssumableRoleSessionSeconds(seconds int) error {
	if seconds < 0 {
		return BadRequest("session_duration_seconds must be positive")
	}
	if seconds > maxAssumableRoleSessionSeconds {
		return BadRequest("session_duration_seconds exceeds 12 hour maximum")
	}
	return nil
}

func effectiveAssumableRoleSessionDuration(roleSeconds int, requestedMinutes int) (time.Duration, error) {
	if err := validateAssumableRoleSessionSeconds(roleSeconds); err != nil {
		return 0, err
	}
	if requestedMinutes < 0 {
		return 0, BadRequest("duration_minutes must be positive")
	}
	if roleSeconds == 0 {
		roleSeconds = defaultAssumableRoleSessionSeconds
	}
	if requestedMinutes == 0 {
		return time.Duration(roleSeconds) * time.Second, nil
	}
	if requestedMinutes > roleSeconds/60 {
		return 0, BadRequest("duration_minutes exceeds assumable role session duration")
	}
	return time.Duration(requestedMinutes) * time.Minute, nil
}

func (c IAMService) trustPolicyAllowsAccount(account Account, role AssumableRole) (bool, error) {
	if !role.Trusted || len(role.TrustPolicy) == 0 {
		return false, nil
	}
	if valueListContains(role.TrustPolicy["accounts"], account.ID) ||
		valueListContains(role.TrustPolicy["account_ids"], account.ID) {
		return true, nil
	}
	allowedGroups := stringSet(append(stringSliceFromAny(role.TrustPolicy["user_groups"]), stringSliceFromAny(role.TrustPolicy["user_group_ids"])...))
	for _, groupID := range account.UserGroupIDs {
		if _, ok := allowedGroups[groupID]; ok {
			return true, nil
		}
	}
	return false, nil
}
