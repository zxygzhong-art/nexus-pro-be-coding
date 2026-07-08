package service

import (
	"nexus-pro-be/internal/utils"
	"sort"
	"strings"
)

func (c IAMService) ListPermissionSetAssignments(ctx RequestContext) ([]PermissionSetAssignment, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourcePermissionAssign, ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListPermissionSetAssignments(goContext(ctx), ctx.TenantID)
}

// ListPermissionSetAssignmentPage 列出權限集合指派分頁的服務流程。
func (c IAMService) ListPermissionSetAssignmentPage(ctx RequestContext, query PermissionSetAssignmentQuery, page PageRequest) (PageResponse[PermissionSetAssignment], error) {
	items, err := c.ListPermissionSetAssignments(ctx)
	if err != nil {
		return PageResponse[PermissionSetAssignment]{}, err
	}
	principalType := strings.TrimSpace(query.PrincipalType)
	principalID := strings.TrimSpace(query.PrincipalID)
	if principalType != "" || principalID != "" {
		filtered := make([]PermissionSetAssignment, 0, len(items))
		for _, item := range items {
			if principalType != "" && item.PrincipalType != principalType {
				continue
			}
			if principalID != "" && item.PrincipalID != principalID {
				continue
			}
			filtered = append(filtered, item)
		}
		items = filtered
	}
	items = utils.SortPermissionSetAssignments(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

// CreatePermissionSetAssignment 建立權限集合指派的服務流程。
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

// DeletePermissionSetAssignment 刪除權限集合指派的服務流程。
func (c IAMService) DeletePermissionSetAssignment(ctx RequestContext, id string) (PermissionSetAssignment, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourcePermissionAssign, ActionDelete, id); err != nil {
		return PermissionSetAssignment{}, err
	}
	assignments, err := c.store.ListPermissionSetAssignments(goContext(ctx), ctx.TenantID)
	if err != nil {
		return PermissionSetAssignment{}, err
	}
	var current PermissionSetAssignment
	found := false
	for _, item := range assignments {
		if item.ID == strings.TrimSpace(id) {
			current = item
			found = true
			break
		}
	}
	if !found {
		return PermissionSetAssignment{}, NotFound("permission set assignment", id)
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		deleted, ok, err := tx.store.DeletePermissionSetAssignment(goContext(ctx), ctx.TenantID, current.ID)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("permission set assignment", id)
		}
		if err := tx.touchAuthzConfig(ctx, "iam.permission_assignment.delete", map[string]any{"assignment_id": deleted.ID}); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.permission_assignment.delete", "permission_set_assignment", deleted.ID, "high", map[string]any{
			"principal_type": deleted.PrincipalType,
			"principal_id":   deleted.PrincipalID,
			"permission_set": deleted.PermissionSetID,
			"effect":         deleted.Effect,
		})
	}); err != nil {
		return PermissionSetAssignment{}, err
	}
	return current, nil
}

// validatePermissionSetAssignmentPrincipal 驗證權限集合指派 principal 的服務流程。
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

// ListDataScopes 列出資料範圍的服務流程。
func (c IAMService) ListDataScopes(ctx RequestContext) ([]DataScope, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceDataScope, ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListDataScopes(goContext(ctx), ctx.TenantID)
}

// ListDataScopePage 列出資料範圍分頁的服務流程。
func (c IAMService) ListDataScopePage(ctx RequestContext, page PageRequest) (PageResponse[DataScope], error) {
	items, err := c.ListDataScopes(ctx)
	if err != nil {
		return PageResponse[DataScope]{}, err
	}
	items = utils.SortDataScopes(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

// CreateDataScope 建立資料範圍的服務流程。
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
	if err := c.ensureDataScopeCodeAvailable(ctx, scope.Code, scope.ID); err != nil {
		return DataScope{}, err
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

// UpdateDataScope 更新資料範圍。
func (c IAMService) UpdateDataScope(ctx RequestContext, id string, input UpdateDataScopeInput) (DataScope, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceDataScope, ActionUpdate, id); err != nil {
		return DataScope{}, err
	}
	current, ok, err := c.store.GetDataScope(goContext(ctx), ctx.TenantID, strings.TrimSpace(id))
	if err != nil {
		return DataScope{}, err
	}
	if !ok {
		return DataScope{}, NotFound("data scope", id)
	}
	next := current
	if input.Code != nil {
		code := strings.TrimSpace(*input.Code)
		if code == "" {
			return DataScope{}, BadRequest("data scope code is required")
		}
		next.Code = code
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return DataScope{}, BadRequest("data scope name is required")
		}
		next.Name = name
	}
	if input.ScopeType != nil {
		scopeType := strings.TrimSpace(*input.ScopeType)
		if scopeType == "" {
			scopeType = next.Code
		}
		if !validDataScopeType(scopeType) {
			return DataScope{}, BadRequest("unsupported data scope type")
		}
		next.ScopeType = scopeType
	}
	if input.Params != nil {
		next.Params = utils.CopyStringMap(input.Params)
	}
	if err := c.ensureDataScopeCodeAvailable(ctx, next.Code, next.ID); err != nil {
		return DataScope{}, err
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		if err := tx.store.UpdateDataScope(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.touchAuthzConfig(ctx, "iam.data_scope.update", map[string]any{"data_scope_id": next.ID}); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.data_scope.update", "data_scope", next.ID, "high", map[string]any{
			"code":       next.Code,
			"scope_type": next.ScopeType,
		})
	}); err != nil {
		return DataScope{}, err
	}
	return next, nil
}

// DeleteDataScope 刪除資料範圍。
func (c IAMService) DeleteDataScope(ctx RequestContext, id string) (DataScope, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceDataScope, ActionDelete, id); err != nil {
		return DataScope{}, err
	}
	current, ok, err := c.store.GetDataScope(goContext(ctx), ctx.TenantID, strings.TrimSpace(id))
	if err != nil {
		return DataScope{}, err
	}
	if !ok {
		return DataScope{}, NotFound("data scope", id)
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		deleted, ok, err := tx.store.DeleteDataScope(goContext(ctx), ctx.TenantID, current.ID)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("data scope", id)
		}
		if err := tx.touchAuthzConfig(ctx, "iam.data_scope.delete", map[string]any{"data_scope_id": deleted.ID}); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.data_scope.delete", "data_scope", deleted.ID, "high", map[string]any{
			"code":       deleted.Code,
			"scope_type": deleted.ScopeType,
		})
	}); err != nil {
		return DataScope{}, err
	}
	return current, nil
}

// ListFieldPolicies 列出欄位政策的服務流程。
func (c IAMService) ListFieldPolicies(ctx RequestContext, applicationCode, resourceType string) ([]FieldPolicy, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceFieldPolicy, ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListFieldPolicies(goContext(ctx), ctx.TenantID, strings.TrimSpace(applicationCode), strings.TrimSpace(resourceType))
}

// ListFieldPolicyPage 列出欄位政策分頁的服務流程。
func (c IAMService) ListFieldPolicyPage(ctx RequestContext, applicationCode, resourceType string, page PageRequest) (PageResponse[FieldPolicy], error) {
	items, err := c.ListFieldPolicies(ctx, applicationCode, resourceType)
	if err != nil {
		return PageResponse[FieldPolicy]{}, err
	}
	items = utils.SortFieldPolicies(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

// CreateFieldPolicy 建立欄位政策的服務流程。
func (c IAMService) CreateFieldPolicy(ctx RequestContext, input CreateFieldPolicyInput) (FieldPolicy, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceFieldPolicy, ActionCreate, ""); err != nil {
		return FieldPolicy{}, err
	}
	effect := strings.TrimSpace(input.Effect)
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
	if err := validateFieldPolicy(policy); err != nil {
		return FieldPolicy{}, err
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		if err := tx.store.UpsertFieldPolicy(goContext(ctx), policy); err != nil {
			return err
		}
		if err := tx.touchAuthzConfig(ctx, "iam.field_policy.upsert", map[string]any{"field_policy_id": policy.ID}); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.field_policy.create", "field_policy", policy.ID, "high", fieldPolicyAuditDetails(policy))
	}); err != nil {
		return FieldPolicy{}, err
	}
	return policy, nil
}

// UpdateFieldPolicy 更新欄位政策。
func (c IAMService) UpdateFieldPolicy(ctx RequestContext, id string, input UpdateFieldPolicyInput) (FieldPolicy, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceFieldPolicy, ActionUpdate, id); err != nil {
		return FieldPolicy{}, err
	}
	current, ok, err := c.store.GetFieldPolicy(goContext(ctx), ctx.TenantID, strings.TrimSpace(id))
	if err != nil {
		return FieldPolicy{}, err
	}
	if !ok {
		return FieldPolicy{}, NotFound("field policy", id)
	}
	next := current
	if input.ApplicationCode != nil {
		next.ApplicationCode = strings.TrimSpace(*input.ApplicationCode)
	}
	if input.ResourceType != nil {
		next.ResourceType = strings.TrimSpace(*input.ResourceType)
	}
	if input.FieldName != nil {
		next.FieldName = strings.TrimSpace(*input.FieldName)
	}
	if input.Effect != nil {
		next.Effect = strings.TrimSpace(*input.Effect)
	}
	if input.MaskStrategy != nil {
		next.MaskStrategy = strings.TrimSpace(*input.MaskStrategy)
	}
	if input.PermissionID != nil {
		next.PermissionID = strings.TrimSpace(*input.PermissionID)
	}
	if err := validateFieldPolicy(next); err != nil {
		return FieldPolicy{}, err
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		if err := tx.store.UpsertFieldPolicy(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.touchAuthzConfig(ctx, "iam.field_policy.update", map[string]any{"field_policy_id": next.ID}); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.field_policy.update", "field_policy", next.ID, "high", fieldPolicyAuditDetails(next))
	}); err != nil {
		return FieldPolicy{}, err
	}
	return next, nil
}

// DeleteFieldPolicy 刪除欄位政策。
func (c IAMService) DeleteFieldPolicy(ctx RequestContext, id string) (FieldPolicy, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceFieldPolicy, ActionDelete, id); err != nil {
		return FieldPolicy{}, err
	}
	current, ok, err := c.store.GetFieldPolicy(goContext(ctx), ctx.TenantID, strings.TrimSpace(id))
	if err != nil {
		return FieldPolicy{}, err
	}
	if !ok {
		return FieldPolicy{}, NotFound("field policy", id)
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		deleted, ok, err := tx.store.DeleteFieldPolicy(goContext(ctx), ctx.TenantID, current.ID)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("field policy", id)
		}
		if err := tx.touchAuthzConfig(ctx, "iam.field_policy.delete", map[string]any{"field_policy_id": deleted.ID}); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.field_policy.delete", "field_policy", deleted.ID, "high", fieldPolicyAuditDetails(deleted))
	}); err != nil {
		return FieldPolicy{}, err
	}
	return current, nil
}

// ensureDataScopeCodeAvailable 確認資料範圍 code 不與其他資料範圍衝突。
func (c IAMService) ensureDataScopeCodeAvailable(ctx RequestContext, code, currentID string) error {
	items, err := c.store.ListDataScopes(goContext(ctx), ctx.TenantID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.ID != currentID && strings.EqualFold(item.Code, code) {
			return Conflict("data scope code already exists")
		}
	}
	return nil
}

// validateFieldPolicy 驗證欄位政策。
func validateFieldPolicy(policy FieldPolicy) error {
	if strings.TrimSpace(policy.ApplicationCode) == "" || strings.TrimSpace(policy.ResourceType) == "" || strings.TrimSpace(policy.FieldName) == "" {
		return BadRequest("application_code, resource_type and field_name are required")
	}
	switch strings.TrimSpace(policy.Effect) {
	case "allow", "deny", "mask", "readonly", "hide":
		return nil
	default:
		return BadRequest("field policy effect must be allow, deny, mask, readonly or hide")
	}
}

// fieldPolicyAuditDetails 建立欄位政策審計 details。
func fieldPolicyAuditDetails(policy FieldPolicy) map[string]any {
	return map[string]any{
		"application_code": policy.ApplicationCode,
		"resource_type":    policy.ResourceType,
		"field_name":       policy.FieldName,
		"effect":           policy.Effect,
	}
}

// ListOutboxEventPage 列出 outbox 事件同步狀態。
func (c IAMService) ListOutboxEventPage(ctx RequestContext, query OutboxEventQuery, page PageRequest) (PageResponse[OutboxEvent], error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceOutboxEvent, ActionRead, ""); err != nil {
		return PageResponse[OutboxEvent]{}, err
	}
	items, err := c.Service.store.ListOutboxEvents(goContext(ctx), ctx.TenantID)
	if err != nil {
		return PageResponse[OutboxEvent]{}, err
	}
	items = filterOutboxEvents(items, query)
	sort.SliceStable(items, func(i, j int) bool {
		switch page.Sort {
		case "created_at_asc":
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		default:
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
	})
	return utils.PageResponse(items, page), nil
}

// RetryOutboxEvent 將失敗 outbox 事件重置為待處理。
func (c IAMService) RetryOutboxEvent(ctx RequestContext, id string) (OutboxEvent, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceOutboxEvent, ActionUpdate, id); err != nil {
		return OutboxEvent{}, err
	}
	event, ok, err := c.outboxEventByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return OutboxEvent{}, err
	}
	if !ok {
		return OutboxEvent{}, NotFound("outbox event", id)
	}
	if event.Status != "failed" {
		return OutboxEvent{}, Conflict("only failed outbox events can be retried")
	}
	next := event
	next.Status = "pending"
	next.RetryCount = 0
	next.LastError = ""
	next.ProcessedAt = nil
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		if err := tx.Service.store.UpdateOutboxEvent(goContext(ctx), next); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.outbox_event.retry", "outbox_event", next.ID, "high", map[string]any{
			"event_type":       event.EventType,
			"previous_status":  event.Status,
			"previous_retries": event.RetryCount,
		})
	}); err != nil {
		return OutboxEvent{}, err
	}
	return next, nil
}

// outboxEventByID 依 ID 取得 outbox 事件。
func (c IAMService) outboxEventByID(ctx RequestContext, id string) (OutboxEvent, bool, error) {
	items, err := c.Service.store.ListOutboxEvents(goContext(ctx), ctx.TenantID)
	if err != nil {
		return OutboxEvent{}, false, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, true, nil
		}
	}
	return OutboxEvent{}, false, nil
}

// filterOutboxEvents 套用 outbox 管理查詢條件。
func filterOutboxEvents(items []OutboxEvent, query OutboxEventQuery) []OutboxEvent {
	status := strings.TrimSpace(query.Status)
	eventType := strings.TrimSpace(query.EventType)
	lastError := strings.TrimSpace(query.LastError)
	out := make([]OutboxEvent, 0, len(items))
	for _, item := range items {
		if status != "" && item.Status != status {
			continue
		}
		if eventType != "" && item.EventType != eventType {
			continue
		}
		if lastError != "" && !strings.Contains(strings.ToLower(item.LastError), strings.ToLower(lastError)) {
			continue
		}
		if query.RetryCount != nil && item.RetryCount != *query.RetryCount {
			continue
		}
		if query.HasError != nil {
			hasError := strings.TrimSpace(item.LastError) != ""
			if hasError != *query.HasError {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

// validDataScopeType 處理有效資料範圍 type。
func validDataScopeType(scopeType string) bool {
	switch Scope(scopeType) {
	case ScopeAll, ScopeTenant, ScopeSelf, ScopeOwn, ScopeObject, ScopeDepartment, ScopeDepartmentSubtree, ScopeDirectReports, ScopeAssignedOrgUnits, ScopeCustomCondition, ScopeSystem:
		return true
	default:
		return false
	}
}

// ListAssumableRoles 列出 assumable 角色的服務流程。
