package service

import (
	"strings"
	"time"

	"nexus-pro-be/internal/utils"
)

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
	items = utils.SortAssumableRoles(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

func (c IAMService) CreateAssumableRole(ctx RequestContext, input CreateAssumableRoleInput) (AssumableRole, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
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
	c.logWarn(ctx, "assumable role created",
		"assumable_role_id", role.ID,
		"trusted", role.Trusted,
		"permission_set_count", len(role.PermissionSetIDs),
		"session_duration_seconds", role.SessionDurationSeconds,
	)
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
	if strings.TrimSpace(input.Reason) == "" {
		return AssumeRoleResponse{}, BadRequest("assume role reason is required")
	}
	allowed, err := c.canAssumeRole(ctx, account, role)
	if err != nil {
		return AssumeRoleResponse{}, err
	}
	if !allowed {
		c.logWarn(ctx, "assume role denied by role membership",
			"assumable_role_id", role.ID,
			"reason", "current account cannot assume this role",
		)
		return AssumeRoleResponse{}, Forbidden("current account cannot assume this role")
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
		SessionPolicy:      utils.CopyStringMap(input.SessionPolicy),
		PermissionBoundary: utils.CopyStringMap(role.PermissionBoundary),
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
