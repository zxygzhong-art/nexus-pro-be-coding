package service

import (
	"strings"
	"time"
)

type IAMService struct {
	*Service
}

func (c *Service) IAM() IAMService {
	return IAMService{Service: c}
}

func (c *Service) ListPermissions(ctx RequestContext) ([]Permission, error) {
	return c.IAM().ListPermissions(ctx)
}

func (c *Service) ListPermissionPage(ctx RequestContext, page PageRequest) (PageResponse[Permission], error) {
	return c.IAM().ListPermissionPage(ctx, page)
}

func (c *Service) ListUserGroups(ctx RequestContext) ([]UserGroup, error) {
	return c.IAM().ListUserGroups(ctx)
}

func (c *Service) ListUserGroupPage(ctx RequestContext, page PageRequest) (PageResponse[UserGroup], error) {
	return c.IAM().ListUserGroupPage(ctx, page)
}

func (c *Service) CreateUserGroup(ctx RequestContext, input CreateUserGroupInput) (UserGroup, error) {
	return c.IAM().CreateUserGroup(ctx, input)
}

func (c *Service) ListPermissionSets(ctx RequestContext) ([]PermissionSet, error) {
	return c.IAM().ListPermissionSets(ctx)
}

func (c *Service) ListPermissionSetPage(ctx RequestContext, page PageRequest) (PageResponse[PermissionSet], error) {
	return c.IAM().ListPermissionSetPage(ctx, page)
}

func (c *Service) CreatePermissionSet(ctx RequestContext, input CreatePermissionSetInput) (PermissionSet, error) {
	return c.IAM().CreatePermissionSet(ctx, input)
}

func (c *Service) ListPermissionSetAssignments(ctx RequestContext) ([]PermissionSetAssignment, error) {
	return c.IAM().ListPermissionSetAssignments(ctx)
}

func (c *Service) ListPermissionSetAssignmentPage(ctx RequestContext, page PageRequest) (PageResponse[PermissionSetAssignment], error) {
	return c.IAM().ListPermissionSetAssignmentPage(ctx, page)
}

func (c *Service) CreatePermissionSetAssignment(ctx RequestContext, input CreatePermissionSetAssignmentInput) (PermissionSetAssignment, error) {
	return c.IAM().CreatePermissionSetAssignment(ctx, input)
}

func (c *Service) ListDataScopes(ctx RequestContext) ([]DataScope, error) {
	return c.IAM().ListDataScopes(ctx)
}

func (c *Service) ListDataScopePage(ctx RequestContext, page PageRequest) (PageResponse[DataScope], error) {
	return c.IAM().ListDataScopePage(ctx, page)
}

func (c *Service) CreateDataScope(ctx RequestContext, input CreateDataScopeInput) (DataScope, error) {
	return c.IAM().CreateDataScope(ctx, input)
}

func (c *Service) ListFieldPolicies(ctx RequestContext, applicationCode, resourceType string) ([]FieldPolicy, error) {
	return c.IAM().ListFieldPolicies(ctx, applicationCode, resourceType)
}

func (c *Service) ListFieldPolicyPage(ctx RequestContext, applicationCode, resourceType string, page PageRequest) (PageResponse[FieldPolicy], error) {
	return c.IAM().ListFieldPolicyPage(ctx, applicationCode, resourceType, page)
}

func (c *Service) CreateFieldPolicy(ctx RequestContext, input CreateFieldPolicyInput) (FieldPolicy, error) {
	return c.IAM().CreateFieldPolicy(ctx, input)
}

func (c *Service) ListAssumableRoles(ctx RequestContext) ([]AssumableRole, error) {
	return c.IAM().ListAssumableRoles(ctx)
}

func (c *Service) ListAssumableRolePage(ctx RequestContext, page PageRequest) (PageResponse[AssumableRole], error) {
	return c.IAM().ListAssumableRolePage(ctx, page)
}

func (c *Service) CreateAssumableRole(ctx RequestContext, input CreateAssumableRoleInput) (AssumableRole, error) {
	return c.IAM().CreateAssumableRole(ctx, input)
}

func (c *Service) AssumeRole(ctx RequestContext, roleID string, input AssumeRoleInput) (AssumeRoleResponse, error) {
	return c.IAM().AssumeRole(ctx, roleID, input)
}

func (c IAMService) ListUserGroups(ctx RequestContext) ([]UserGroup, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return nil, err
	}
	return c.store.ListUserGroups(goContext(ctx), ctx.TenantID)
}

func (c IAMService) ListUserGroupPage(ctx RequestContext, page PageRequest) (PageResponse[UserGroup], error) {
	items, err := c.ListUserGroups(ctx)
	if err != nil {
		return PageResponse[UserGroup]{}, err
	}
	items = sortUserGroups(items, page.Sort)
	return pageResponse(items, page), nil
}

func (c IAMService) CreateUserGroup(ctx RequestContext, input CreateUserGroupInput) (UserGroup, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
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
		ID:               newID("ug"),
		TenantID:         ctx.TenantID,
		Name:             strings.TrimSpace(input.Name),
		Description:      strings.TrimSpace(input.Description),
		MemberAccountIDs: uniqueStrings(input.MemberAccountIDs),
		PermissionSetIDs: uniqueStrings(input.PermissionSetIDs),
		CreatedAt:        c.Now(),
	}
	if err := c.withTenantTransaction(ctx, func(tx *Service) error {
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
	return group, nil
}

func (c IAMService) ListPermissionSets(ctx RequestContext) ([]PermissionSet, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return nil, err
	}
	return c.store.ListPermissionSets(goContext(ctx), ctx.TenantID)
}

func (c IAMService) ListPermissionSetPage(ctx RequestContext, page PageRequest) (PageResponse[PermissionSet], error) {
	items, err := c.ListPermissionSets(ctx)
	if err != nil {
		return PageResponse[PermissionSet]{}, err
	}
	items = sortPermissionSets(items, page.Sort)
	return pageResponse(items, page), nil
}

func (c IAMService) CreatePermissionSet(ctx RequestContext, input CreatePermissionSetInput) (PermissionSet, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
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
		ID:          newID("ps"),
		TenantID:    ctx.TenantID,
		Name:        strings.TrimSpace(input.Name),
		Description: strings.TrimSpace(input.Description),
		Permissions: normalizePermissions(input.Permissions),
		CreatedAt:   c.Now(),
	}
	if err := c.withTenantTransaction(ctx, func(tx *Service) error {
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
	return set, nil
}

func (c IAMService) ListPermissions(ctx RequestContext) ([]Permission, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return nil, err
	}
	return defaultPermissions(), nil
}

func (c IAMService) ListPermissionPage(ctx RequestContext, page PageRequest) (PageResponse[Permission], error) {
	items, err := c.ListPermissions(ctx)
	if err != nil {
		return PageResponse[Permission]{}, err
	}
	return pageResponse(items, page), nil
}

func (c IAMService) ListPermissionSetAssignments(ctx RequestContext) ([]PermissionSetAssignment, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return nil, err
	}
	return c.store.ListPermissionSetAssignments(goContext(ctx), ctx.TenantID)
}

func (c IAMService) ListPermissionSetAssignmentPage(ctx RequestContext, page PageRequest) (PageResponse[PermissionSetAssignment], error) {
	items, err := c.ListPermissionSetAssignments(ctx)
	if err != nil {
		return PageResponse[PermissionSetAssignment]{}, err
	}
	items = sortPermissionSetAssignments(items, page.Sort)
	return pageResponse(items, page), nil
}

func (c IAMService) CreatePermissionSetAssignment(ctx RequestContext, input CreatePermissionSetAssignmentInput) (PermissionSetAssignment, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
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
		ID:              newID("psa"),
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
	if err := c.withTenantTransaction(ctx, func(tx *Service) error {
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

func (c IAMService) ListDataScopes(ctx RequestContext) ([]DataScope, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return nil, err
	}
	return c.store.ListDataScopes(goContext(ctx), ctx.TenantID)
}

func (c IAMService) ListDataScopePage(ctx RequestContext, page PageRequest) (PageResponse[DataScope], error) {
	items, err := c.ListDataScopes(ctx)
	if err != nil {
		return PageResponse[DataScope]{}, err
	}
	items = sortDataScopes(items, page.Sort)
	return pageResponse(items, page), nil
}

func (c IAMService) CreateDataScope(ctx RequestContext, input CreateDataScopeInput) (DataScope, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return DataScope{}, err
	}
	if strings.TrimSpace(input.Code) == "" || strings.TrimSpace(input.Name) == "" {
		return DataScope{}, BadRequest("data scope code and name are required")
	}
	scopeType := strings.TrimSpace(input.ScopeType)
	if scopeType == "" {
		scopeType = strings.TrimSpace(input.Code)
	}
	scope := DataScope{
		ID:        newID("ds"),
		TenantID:  ctx.TenantID,
		Code:      strings.TrimSpace(input.Code),
		Name:      strings.TrimSpace(input.Name),
		ScopeType: scopeType,
		Params:    copyStringMap(input.Params),
		CreatedAt: c.Now(),
	}
	if err := c.withTenantTransaction(ctx, func(tx *Service) error {
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

func (c IAMService) ListFieldPolicies(ctx RequestContext, applicationCode, resourceType string) ([]FieldPolicy, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return nil, err
	}
	return c.store.ListFieldPolicies(goContext(ctx), ctx.TenantID, strings.TrimSpace(applicationCode), strings.TrimSpace(resourceType))
}

func (c IAMService) ListFieldPolicyPage(ctx RequestContext, applicationCode, resourceType string, page PageRequest) (PageResponse[FieldPolicy], error) {
	items, err := c.ListFieldPolicies(ctx, applicationCode, resourceType)
	if err != nil {
		return PageResponse[FieldPolicy]{}, err
	}
	items = sortFieldPolicies(items, page.Sort)
	return pageResponse(items, page), nil
}

func (c IAMService) CreateFieldPolicy(ctx RequestContext, input CreateFieldPolicyInput) (FieldPolicy, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
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
		ID:              newID("fp"),
		TenantID:        ctx.TenantID,
		ApplicationCode: strings.TrimSpace(input.ApplicationCode),
		ResourceType:    strings.TrimSpace(input.ResourceType),
		FieldName:       strings.TrimSpace(input.FieldName),
		Effect:          effect,
		MaskStrategy:    strings.TrimSpace(input.MaskStrategy),
		PermissionID:    strings.TrimSpace(input.PermissionID),
		CreatedAt:       c.Now(),
	}
	if err := c.withTenantTransaction(ctx, func(tx *Service) error {
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

func (c IAMService) ListAssumableRoles(ctx RequestContext) ([]AssumableRole, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return nil, err
	}
	return c.store.ListAssumableRoles(goContext(ctx), ctx.TenantID)
}

func (c IAMService) ListAssumableRolePage(ctx RequestContext, page PageRequest) (PageResponse[AssumableRole], error) {
	items, err := c.ListAssumableRoles(ctx)
	if err != nil {
		return PageResponse[AssumableRole]{}, err
	}
	items = sortAssumableRoles(items, page.Sort)
	return pageResponse(items, page), nil
}

func (c IAMService) CreateAssumableRole(ctx RequestContext, input CreateAssumableRoleInput) (AssumableRole, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return AssumableRole{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return AssumableRole{}, BadRequest("assumable role name is required")
	}
	for _, id := range input.PermissionSetIDs {
		if _, ok, err := c.store.GetPermissionSet(goContext(ctx), ctx.TenantID, id); err != nil {
			return AssumableRole{}, err
		} else if !ok {
			return AssumableRole{}, NotFound("permission set", id)
		}
	}
	role := AssumableRole{
		ID:                     newID("ar"),
		TenantID:               ctx.TenantID,
		Name:                   strings.TrimSpace(input.Name),
		Description:            strings.TrimSpace(input.Description),
		PermissionSetIDs:       uniqueStrings(input.PermissionSetIDs),
		Trusted:                input.Trusted,
		TrustPolicy:            copyStringMap(input.TrustPolicy),
		PermissionBoundary:     copyStringMap(input.PermissionBoundary),
		SessionDurationSeconds: input.SessionDurationSeconds,
		CreatedAt:              c.Now(),
	}
	if err := c.withTenantTransaction(ctx, func(tx *Service) error {
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
	return role, nil
}

func (c IAMService) AssumeRole(ctx RequestContext, roleID string, input AssumeRoleInput) (AssumeRoleResponse, error) {
	account, _, err := c.resolveAccount(ctx)
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
	allowed, err := c.canAssumeRole(ctx, account, role)
	if err != nil {
		return AssumeRoleResponse{}, err
	}
	if !allowed {
		return AssumeRoleResponse{}, Forbidden("current account cannot assume this role")
	}

	token := newID("sess")
	duration := time.Duration(role.SessionDurationSeconds) * time.Second
	if duration <= 0 {
		duration = 8 * time.Hour
	}
	if input.DurationMinutes > 0 {
		duration = time.Duration(input.DurationMinutes) * time.Minute
	}
	session := AssumableRoleSession{
		ID:                 token,
		TenantID:           ctx.TenantID,
		AccountID:          account.ID,
		AssumableRoleID:    role.ID,
		SessionPolicy:      copyStringMap(input.SessionPolicy),
		PermissionBoundary: copyStringMap(role.PermissionBoundary),
		ExpiresAt:          c.Now().Add(duration),
		CreatedAt:          c.Now(),
	}
	if err := c.withTenantTransaction(ctx, func(tx *Service) error {
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
