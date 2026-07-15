package service

import (
	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
	"sort"
	"strings"
	"time"
)

// IAMService 定義 IAM 服務的資料結構。
type IAMService struct {
	*Service
	store iamStore
}

const (
	defaultAssumableRoleSessionSeconds = 8 * 60 * 60
	maxAssumableRoleSessionSeconds     = 12 * 60 * 60
)

// IAM 處理 IAM 的服務流程。
func (c *Service) IAM() IAMService {
	return IAMService{Service: c, store: c.store}
}

// ListApplications 列出 IAM application 目錄。
func (c IAMService) ListApplications(ctx RequestContext) ([]IAMApplication, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceApplication, ActionRead, ""); err != nil {
		return nil, err
	}
	out := make([]IAMApplication, 0, len(domain.DefaultApplications))
	for _, app := range domain.DefaultApplications {
		out = append(out, IAMApplication{
			Code:        string(app.Code),
			Name:        app.Name,
			Description: app.Description,
		})
	}
	return out, nil
}

// ListResourceTypes 列出 IAM resource type 目錄。
func (c IAMService) ListResourceTypes(ctx RequestContext) ([]IAMResourceType, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceResourceType, ActionRead, ""); err != nil {
		return nil, err
	}
	grouped := map[string]map[string]struct{}{}
	for _, policy := range domain.DefaultRoutePolicies {
		applicationCode := strings.TrimSpace(policy.ApplicationCode)
		resourceType := strings.TrimSpace(policy.ResourceType)
		action := strings.TrimSpace(policy.Action)
		if applicationCode == "" || resourceType == "" || action == "" {
			continue
		}
		key := applicationCode + "\x00" + resourceType
		if grouped[key] == nil {
			grouped[key] = map[string]struct{}{}
		}
		grouped[key][action] = struct{}{}
	}
	out := make([]IAMResourceType, 0, len(grouped))
	for key, actionSet := range grouped {
		parts := strings.SplitN(key, "\x00", 2)
		actions := make([]string, 0, len(actionSet))
		for action := range actionSet {
			actions = append(actions, action)
		}
		sort.Strings(actions)
		out = append(out, IAMResourceType{ApplicationCode: parts[0], ResourceType: parts[1], Actions: actions})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ApplicationCode == out[j].ApplicationCode {
			return out[i].ResourceType < out[j].ResourceType
		}
		return out[i].ApplicationCode < out[j].ApplicationCode
	})
	return out, nil
}

// ListPermissionSets 列出權限集合的服務流程。
func (c IAMService) ListPermissionSets(ctx RequestContext) ([]PermissionSet, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourcePermissionSet, ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListPermissionSets(goContext(ctx), ctx.TenantID)
}

// ListPermissionSetPage 列出權限集合分頁的服務流程。
func (c IAMService) ListPermissionSetPage(ctx RequestContext, page PageRequest) (PageResponse[PermissionSet], error) {
	items, err := c.ListPermissionSets(ctx)
	if err != nil {
		return PageResponse[PermissionSet]{}, err
	}
	items = utils.SortPermissionSets(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

// CreatePermissionSet 建立權限集合的服務流程。
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
		itemCount, err := tx.upsertPermissionSetWithItems(ctx, set)
		if err != nil {
			return err
		}
		if err := tx.touchAuthzConfig(ctx, "iam.permission_set.upsert", map[string]any{"permission_set_id": set.ID}); err != nil {
			return err
		}
		if itemCount > 0 {
			if err := tx.audit(ctx, "iam.permission_catalog.sync", "permission_catalog", set.ID, "medium", map[string]any{
				"permission_set_id": set.ID,
				"permission_count":  itemCount,
			}); err != nil {
				return err
			}
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

// UpdatePermissionSet 更新權限集合的服務流程。
func (c IAMService) UpdatePermissionSet(ctx RequestContext, id string, input UpdatePermissionSetInput) (PermissionSet, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourcePermissionSet, ActionUpdate, id); err != nil {
		return PermissionSet{}, err
	}
	current, ok, err := c.store.GetPermissionSet(goContext(ctx), ctx.TenantID, strings.TrimSpace(id))
	if err != nil {
		return PermissionSet{}, err
	}
	if !ok {
		return PermissionSet{}, NotFound("permission set", id)
	}
	next := current
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return PermissionSet{}, BadRequest("permission set name is required")
		}
		next.Name = name
	}
	if input.Description != nil {
		next.Description = strings.TrimSpace(*input.Description)
	}
	if input.Permissions != nil {
		for _, perm := range input.Permissions {
			perm = normalizePermission(perm)
			if strings.TrimSpace(perm.Resource) == "" || strings.TrimSpace(string(perm.Action)) == "" {
				return PermissionSet{}, BadRequest("permission resource and action are required")
			}
		}
		next.Permissions = normalizePermissions(input.Permissions)
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		itemCount, err := tx.upsertPermissionSetWithItems(ctx, next)
		if err != nil {
			return err
		}
		if err := tx.touchAuthzConfig(ctx, "iam.permission_set.upsert", map[string]any{"permission_set_id": next.ID}); err != nil {
			return err
		}
		if itemCount > 0 {
			if err := tx.audit(ctx, "iam.permission_catalog.sync", "permission_catalog", next.ID, "medium", map[string]any{
				"permission_set_id": next.ID,
				"permission_count":  itemCount,
			}); err != nil {
				return err
			}
		}
		return tx.audit(ctx, "iam.permission_set.update", "permission_set", next.ID, "high", map[string]any{
			"name":             next.Name,
			"permission_count": len(next.Permissions),
		})
	}); err != nil {
		return PermissionSet{}, err
	}
	return next, nil
}

// DeletePermissionSet 刪除權限集合；若仍被引用或有有效指派則拒絕。
func (c IAMService) DeletePermissionSet(ctx RequestContext, id string) (PermissionSet, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourcePermissionSet, ActionDelete, id); err != nil {
		return PermissionSet{}, err
	}
	current, ok, err := c.store.GetPermissionSet(goContext(ctx), ctx.TenantID, strings.TrimSpace(id))
	if err != nil {
		return PermissionSet{}, err
	}
	if !ok {
		return PermissionSet{}, NotFound("permission set", id)
	}
	if err := c.ensurePermissionSetDeletable(ctx, current.ID); err != nil {
		return PermissionSet{}, err
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		assignments, err := tx.store.ListPermissionSetAssignments(goContext(ctx), ctx.TenantID)
		if err != nil {
			return err
		}
		for _, assignment := range assignments {
			if assignment.PermissionSetID != current.ID {
				continue
			}
			if _, ok, err := tx.store.DeletePermissionSetAssignment(goContext(ctx), ctx.TenantID, assignment.ID); err != nil {
				return err
			} else if !ok {
				continue
			}
		}
		deleted, ok, err := tx.store.DeletePermissionSet(goContext(ctx), ctx.TenantID, current.ID)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("permission set", id)
		}
		if err := tx.touchAuthzConfig(ctx, "iam.permission_set.delete", map[string]any{"permission_set_id": deleted.ID}); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.permission_set.delete", "permission_set", deleted.ID, "high", map[string]any{
			"name": deleted.Name,
		})
	}); err != nil {
		return PermissionSet{}, err
	}
	c.logWarn(ctx, "permission set deleted", "permission_set_id", current.ID)
	return current, nil
}

// ensurePermissionSetDeletable 檢查權限集合是否仍被引用或有有效指派。
func (c IAMService) ensurePermissionSetDeletable(ctx RequestContext, permissionSetID string) error {
	now := c.Now()
	assignments, err := c.store.ListPermissionSetAssignments(goContext(ctx), ctx.TenantID)
	if err != nil {
		return err
	}
	for _, assignment := range assignments {
		if assignment.PermissionSetID == permissionSetID && permissionSetAssignmentActive(assignment, now) {
			return Conflict("permission set has active assignments")
		}
	}
	groups, err := c.store.ListUserGroups(goContext(ctx), ctx.TenantID)
	if err != nil {
		return err
	}
	for _, group := range groups {
		if utils.ContainsString(group.PermissionSetIDs, permissionSetID) {
			return Conflict("permission set is referenced by user groups")
		}
	}
	roles, err := c.store.ListAssumableRoles(goContext(ctx), ctx.TenantID)
	if err != nil {
		return err
	}
	for _, role := range roles {
		if utils.ContainsString(role.PermissionSetIDs, permissionSetID) {
			return Conflict("permission set is referenced by assumable roles")
		}
	}
	accounts, err := c.store.ListAccounts(goContext(ctx), ctx.TenantID)
	if err != nil {
		return err
	}
	for _, account := range accounts {
		if utils.ContainsString(account.DirectPermissionSetIDs, permissionSetID) {
			return Conflict("permission set is referenced by accounts")
		}
	}
	return nil
}

// permissionSetAssignmentActive 判斷指派在指定時間是否有效。
func permissionSetAssignmentActive(assignment PermissionSetAssignment, now time.Time) bool {
	if assignment.StartsAt != nil && assignment.StartsAt.After(now) {
		return false
	}
	if assignment.ExpiresAt != nil && !assignment.ExpiresAt.After(now) {
		return false
	}
	return true
}

// ListIamAccountPage 列出 IAM 帳號分頁。
func (c IAMService) ListIamAccountPage(ctx RequestContext, keyword string, page PageRequest) (PageResponse[IamAccountProjection], error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceIAMAccount, ActionRead, ""); err != nil {
		return PageResponse[IamAccountProjection]{}, err
	}
	accounts, err := c.store.ListAccounts(goContext(ctx), ctx.TenantID)
	if err != nil {
		return PageResponse[IamAccountProjection]{}, err
	}
	normalizedKeyword := strings.ToLower(strings.TrimSpace(keyword))
	items := make([]IamAccountProjection, 0, len(accounts))
	at := c.Now()
	for _, account := range accounts {
		if normalizedKeyword != "" && !accountMatchesKeyword(account, normalizedKeyword) {
			continue
		}
		memberships, err := c.store.ListActiveGroupMembershipsForAccount(goContext(ctx), ctx.TenantID, account.ID, at)
		if err != nil {
			return PageResponse[IamAccountProjection]{}, err
		}
		account.UserGroupIDs = make([]string, 0, len(memberships))
		for _, membership := range memberships {
			account.UserGroupIDs = append(account.UserGroupIDs, membership.UserGroupID)
		}
		account.UserGroupIDs = uniqueStrings(account.UserGroupIDs)
		items = append(items, iamAccountProjectionFromAccount(account))
	}
	items = utils.SortIamAccountProjections(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

func accountMatchesKeyword(account Account, keyword string) bool {
	candidate := strings.ToLower(strings.Join([]string{
		account.ID,
		account.DisplayName,
		account.Email,
		account.EmployeeID,
		account.Status,
	}, " "))
	return strings.Contains(candidate, keyword)
}

func iamAccountProjectionFromAccount(account Account) IamAccountProjection {
	return IamAccountProjection{
		ID:                     account.ID,
		TenantID:               account.TenantID,
		DisplayName:            account.DisplayName,
		Email:                  account.Email,
		EmployeeID:             account.EmployeeID,
		Status:                 account.Status,
		UserGroupIDs:           utils.CopyStrings(account.UserGroupIDs),
		DirectPermissionSetIDs: utils.CopyStrings(account.DirectPermissionSetIDs),
		ActiveAssumableRoleID:  account.ActiveAssumableRoleID,
		CreatedAt:              account.CreatedAt,
	}
}

// ListPermissions 列出權限 catalog 的服務流程。
func (c IAMService) ListPermissions(ctx RequestContext) ([]PermissionCatalogItem, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourcePermission, ActionRead, ""); err != nil {
		return nil, err
	}
	items, err := c.store.ListPermissionCatalogItems(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return defaultPermissionCatalogItems(ctx.TenantID, c.Now()), nil
	}
	return items, nil
}

// ListPermissionPage 列出權限 catalog 分頁的服務流程。
func (c IAMService) ListPermissionPage(ctx RequestContext, page PageRequest) (PageResponse[PermissionCatalogItem], error) {
	items, err := c.ListPermissions(ctx)
	if err != nil {
		return PageResponse[PermissionCatalogItem]{}, err
	}
	return utils.PageResponse(items, page), nil
}

// ListRoles 列出 IAM roles 相容投影的服務流程。
func (c IAMService) ListRoles(ctx RequestContext) ([]IAMRoleProjection, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceAssumableRole, ActionRead, ""); err != nil {
		return nil, err
	}
	roles, err := c.store.ListAssumableRoles(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	out := make([]IAMRoleProjection, 0, len(roles))
	for _, role := range roles {
		projection := IAMRoleProjection{
			ID:                     role.ID,
			TenantID:               role.TenantID,
			Name:                   role.Name,
			Description:            role.Description,
			PermissionSetIDs:       utils.CopyStrings(role.PermissionSetIDs),
			Trusted:                role.Trusted,
			TrustPolicy:            utils.CopyStringMap(role.TrustPolicy),
			PermissionBoundary:     utils.CopyStringMap(role.PermissionBoundary),
			SessionDurationSeconds: role.SessionDurationSeconds,
			CreatedAt:              role.CreatedAt,
		}
		for _, setID := range role.PermissionSetIDs {
			set, ok, err := c.store.GetPermissionSet(goContext(ctx), ctx.TenantID, setID)
			if err != nil {
				return nil, err
			}
			if ok {
				projection.PermissionSets = append(projection.PermissionSets, set)
			}
		}
		out = append(out, projection)
	}
	return out, nil
}

// ListRolePage 列出 IAM roles 相容投影分頁的服務流程。
func (c IAMService) ListRolePage(ctx RequestContext, page PageRequest) (PageResponse[IAMRoleProjection], error) {
	items, err := c.ListRoles(ctx)
	if err != nil {
		return PageResponse[IAMRoleProjection]{}, err
	}
	return utils.PageResponse(items, page), nil
}

// ListRoleBindings 列出 IAM role-bindings 相容投影的服務流程。
func (c IAMService) ListRoleBindings(ctx RequestContext) ([]IAMRoleBindingProjection, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourcePermissionAssign, ActionRead, ""); err != nil {
		return nil, err
	}
	assignments, err := c.store.ListPermissionSetAssignments(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	out := make([]IAMRoleBindingProjection, 0, len(assignments))
	for _, assignment := range assignments {
		projection := IAMRoleBindingProjection{
			ID:              assignment.ID,
			TenantID:        assignment.TenantID,
			PrincipalType:   assignment.PrincipalType,
			PrincipalID:     assignment.PrincipalID,
			PermissionSetID: assignment.PermissionSetID,
			Effect:          assignment.Effect,
			DataScopeID:     assignment.DataScopeID,
			ConditionID:     assignment.ConditionID,
			StartsAt:        assignment.StartsAt,
			ExpiresAt:       assignment.ExpiresAt,
			CreatedAt:       assignment.CreatedAt,
		}
		if set, ok, err := c.store.GetPermissionSet(goContext(ctx), ctx.TenantID, assignment.PermissionSetID); err != nil {
			return nil, err
		} else if ok {
			projection.PermissionSet = &set
		}
		out = append(out, projection)
	}
	return out, nil
}

// ListRoleBindingPage 列出 IAM role-bindings 相容投影分頁的服務流程。
func (c IAMService) ListRoleBindingPage(ctx RequestContext, page PageRequest) (PageResponse[IAMRoleBindingProjection], error) {
	items, err := c.ListRoleBindings(ctx)
	if err != nil {
		return PageResponse[IAMRoleBindingProjection]{}, err
	}
	return utils.PageResponse(items, page), nil
}

// ListUserGroups 列出使用者群組的服務流程。
