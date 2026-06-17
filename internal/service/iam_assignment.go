package service

import (
	"strings"

	"nexus-pro-be/internal/utils"
)

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
	items = utils.SortPermissionSetAssignments(items, page.Sort)
	return utils.PageResponse(items, page), nil
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
