package service

import (
	"strings"

	"nexus-pro-be/internal/utils"
)

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
	items = utils.SortDataScopes(items, page.Sort)
	return utils.PageResponse(items, page), nil
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
	items = utils.SortFieldPolicies(items, page.Sort)
	return utils.PageResponse(items, page), nil
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

func validDataScopeType(scopeType string) bool {
	switch Scope(scopeType) {
	case ScopeAll, ScopeTenant, ScopeSelf, ScopeOwn, ScopeObject, ScopeDepartment, ScopeDepartmentSubtree, ScopeDirectReports, ScopeAssignedOrgUnits, ScopeCustomCondition, ScopeSystem:
		return true
	default:
		return false
	}
}
