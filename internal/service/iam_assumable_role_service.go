package service

import (
	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
	"strings"
	"time"
)

func (c IAMService) ListAssumableRoles(ctx RequestContext) ([]AssumableRole, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceAssumableRole, ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListAssumableRoles(goContext(ctx), ctx.TenantID)
}

// ListAssumableRolePage 列出 assumable 角色分頁的服務流程。
func (c IAMService) ListAssumableRolePage(ctx RequestContext, page PageRequest) (PageResponse[AssumableRole], error) {
	items, err := c.ListAssumableRoles(ctx)
	if err != nil {
		return PageResponse[AssumableRole]{}, err
	}
	items = utils.SortAssumableRoles(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

// CreateAssumableRole 建立 assumable 角色的服務流程。
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
		if err := tx.Service.syncAssumableRoleRelationshipTuples(ctx, AssumableRole{}, role); err != nil {
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

// UpdateAssumableRole 更新 assumable 角色並同步 trust policy tuples。
func (c IAMService) UpdateAssumableRole(ctx RequestContext, id string, input UpdateAssumableRoleInput) (AssumableRole, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceAssumableRole, ActionUpdate, id); err != nil {
		return AssumableRole{}, err
	}
	role, ok, err := c.store.GetAssumableRole(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return AssumableRole{}, err
	}
	if !ok {
		return AssumableRole{}, NotFound("assumable role", id)
	}
	next := role
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return AssumableRole{}, BadRequest("assumable role name is required")
		}
		next.Name = name
	}
	if input.Description != nil {
		next.Description = strings.TrimSpace(*input.Description)
	}
	if input.PermissionSetIDs != nil {
		for _, permissionSetID := range input.PermissionSetIDs {
			if _, ok, err := c.store.GetPermissionSet(goContext(ctx), ctx.TenantID, permissionSetID); err != nil {
				return AssumableRole{}, err
			} else if !ok {
				return AssumableRole{}, NotFound("permission set", permissionSetID)
			}
		}
		next.PermissionSetIDs = uniqueStrings(input.PermissionSetIDs)
	}
	if input.Trusted != nil {
		next.Trusted = *input.Trusted
	}
	if input.TrustPolicy != nil {
		if len(input.TrustPolicy) == 0 {
			return AssumableRole{}, BadRequest("assumable role trust_policy is required")
		}
		next.TrustPolicy = utils.CopyStringMap(input.TrustPolicy)
	}
	if input.PermissionBoundary != nil {
		if len(input.PermissionBoundary) == 0 {
			return AssumableRole{}, BadRequest("assumable role permission_boundary is required")
		}
		next.PermissionBoundary = utils.CopyStringMap(input.PermissionBoundary)
	}
	if input.SessionDurationSeconds != nil {
		if err := validateAssumableRoleSessionSeconds(*input.SessionDurationSeconds); err != nil {
			return AssumableRole{}, err
		}
		next.SessionDurationSeconds = *input.SessionDurationSeconds
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		if err := tx.store.UpsertAssumableRole(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.Service.syncAssumableRoleRelationshipTuples(ctx, role, next); err != nil {
			return err
		}
		if err := tx.touchAuthzConfig(ctx, "iam.assumable_role.upsert", map[string]any{"assumable_role_id": next.ID}); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.assumable_role.update", "assumable_role", next.ID, "high", map[string]any{"name": next.Name})
	}); err != nil {
		return AssumableRole{}, err
	}
	c.logWarn(ctx, "assumable role updated",
		"assumable_role_id", next.ID,
		"trusted", next.Trusted,
		"permission_set_count", len(next.PermissionSetIDs),
		"session_duration_seconds", next.SessionDurationSeconds,
	)
	return next, nil
}

// DeleteAssumableRole 刪除 assumable 角色；若仍有啟用 session 則拒絕。
func (c IAMService) DeleteAssumableRole(ctx RequestContext, id string) (AssumableRole, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceAssumableRole, ActionDelete, id); err != nil {
		return AssumableRole{}, err
	}
	role, ok, err := c.store.GetAssumableRole(goContext(ctx), ctx.TenantID, strings.TrimSpace(id))
	if err != nil {
		return AssumableRole{}, err
	}
	if !ok {
		return AssumableRole{}, NotFound("assumable role", id)
	}
	sessions, err := c.store.ListActiveAssumableRoleSessionsForRole(goContext(ctx), ctx.TenantID, role.ID)
	if err != nil {
		return AssumableRole{}, err
	}
	if len(sessions) > 0 {
		return AssumableRole{}, Conflict("assumable role has active sessions")
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		assignments, err := tx.store.ListPermissionSetAssignmentsForPrincipal(goContext(ctx), ctx.TenantID, string(PrincipalTypeAssumableRole), role.ID)
		if err != nil {
			return err
		}
		for _, assignment := range assignments {
			if _, ok, err := tx.store.DeletePermissionSetAssignment(goContext(ctx), ctx.TenantID, assignment.ID); err != nil {
				return err
			} else if !ok {
				continue
			}
		}
		if err := tx.Service.syncAssumableRoleRelationshipTuples(ctx, role, AssumableRole{}); err != nil {
			return err
		}
		accounts, err := tx.store.ListAccounts(goContext(ctx), ctx.TenantID)
		if err != nil {
			return err
		}
		for _, account := range accounts {
			if account.ActiveAssumableRoleID != role.ID {
				continue
			}
			account.ActiveAssumableRoleID = ""
			if err := tx.store.UpsertAccount(goContext(ctx), account); err != nil {
				return err
			}
		}
		deleted, ok, err := tx.store.DeleteAssumableRole(goContext(ctx), ctx.TenantID, role.ID)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("assumable role", id)
		}
		if err := tx.touchAuthzConfig(ctx, "iam.assumable_role.delete", map[string]any{"assumable_role_id": deleted.ID}); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.assumable_role.delete", "assumable_role", deleted.ID, "high", map[string]any{
			"name": deleted.Name,
		})
	}); err != nil {
		return AssumableRole{}, err
	}
	c.logWarn(ctx, "assumable role deleted", "assumable_role_id", role.ID)
	return role, nil
}

// AssumeRole 建立 assumed role session角色的服務流程。
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
	trusted, err := c.trustPolicyAllowsAccount(ctx, account, role)
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
	if allowed, checked, err := c.openFGAAssumeRoleAllows(ctx, account, role); checked {
		if err != nil {
			c.logWarn(ctx, "openfga assume role check failed; denying role assumption",
				"assumable_role_id", role.ID,
				"error", err,
			)
			return AssumeRoleResponse{}, Forbidden("openfga role assumption check unavailable")
		} else if !allowed {
			c.logWarn(ctx, "assume role denied by openfga",
				"assumable_role_id", role.ID,
				"relation", openFGARelationCanAssume,
			)
			return AssumeRoleResponse{}, Forbidden("assumable role can_assume relationship denied")
		}
	}

	token, err := utils.NewSecretID("sess")
	if err != nil {
		return AssumeRoleResponse{}, err
	}
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

// validateAssumableRoleSessionSeconds 驗證 assumable 角色 session seconds。
func validateAssumableRoleSessionSeconds(seconds int) error {
	if seconds < 0 {
		return BadRequest("session_duration_seconds must be positive")
	}
	if seconds > maxAssumableRoleSessionSeconds {
		return BadRequest("session_duration_seconds exceeds 12 hour maximum")
	}
	return nil
}

// effectiveAssumableRoleSessionDuration 處理 effective assumable 角色 session duration。
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

// trustPolicyAllowsAccount 只使用目前有效的群組成員關係判斷角色信任。
func (c IAMService) trustPolicyAllowsAccount(ctx RequestContext, account Account, role AssumableRole) (bool, error) {
	if !role.Trusted || len(role.TrustPolicy) == 0 {
		return false, nil
	}
	if valueListContains(role.TrustPolicy["accounts"], account.ID) ||
		valueListContains(role.TrustPolicy["account_ids"], account.ID) {
		return true, nil
	}
	allowedGroups := stringSet(append(stringSliceFromAny(role.TrustPolicy["user_groups"]), stringSliceFromAny(role.TrustPolicy["user_group_ids"])...))
	activeGroups, err := c.activeUserGroupsForAccount(ctx, account)
	if err != nil {
		return false, err
	}
	for _, group := range activeGroups {
		if _, ok := allowedGroups[group.ID]; ok {
			return true, nil
		}
	}
	return false, nil
}

func (c IAMService) openFGAAssumeRoleAllows(ctx RequestContext, account Account, role AssumableRole) (bool, bool, error) {
	if !c.openFGAScopeChecksAvailable() {
		return false, false, nil
	}
	allowed, err := c.relationships.CheckRelationship(goContext(ctx), domain.RelationshipCheck{
		TenantID: ctx.TenantID,
		Subject:  openFGASubjectTypeAccount + ":" + account.ID,
		Relation: openFGARelationCanAssume,
		Object:   openFGATypeAssumableRole + ":" + role.ID,
	})
	return allowed, true, err
}
