package service

import (
	"strings"

	"nexus-pro-be/internal/utils"
)

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
	items = utils.SortPermissionSets(items, page.Sort)
	return utils.PageResponse(items, page), nil
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
		ID:          utils.NewID("ps"),
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
	c.logInfo(ctx, "permission set created",
		"permission_set_id", set.ID,
		"permission_count", len(set.Permissions),
	)
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
	return utils.PageResponse(items, page), nil
}
