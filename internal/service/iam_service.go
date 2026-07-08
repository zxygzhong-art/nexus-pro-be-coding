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
	for _, account := range accounts {
		if normalizedKeyword != "" && !accountMatchesKeyword(account, normalizedKeyword) {
			continue
		}
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

// ListPermissions 列出權限的服務流程。
func (c IAMService) ListPermissions(ctx RequestContext) ([]Permission, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourcePermission, ActionRead, ""); err != nil {
		return nil, err
	}
	items, err := c.store.ListPermissionCatalogItems(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return defaultPermissions(), nil
	}
	out := make([]Permission, 0, len(items))
	for _, item := range items {
		out = append(out, permissionFromCatalogItem(item))
	}
	return out, nil
}

// ListPermissionPage 列出權限分頁的服務流程。
func (c IAMService) ListPermissionPage(ctx RequestContext, page PageRequest) (PageResponse[Permission], error) {
	items, err := c.ListPermissions(ctx)
	if err != nil {
		return PageResponse[Permission]{}, err
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
func (c IAMService) ListUserGroups(ctx RequestContext) ([]UserGroup, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceUserGroup, ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListUserGroups(goContext(ctx), ctx.TenantID)
}

// ListUserGroupPage 列出使用者群組分頁的服務流程。
func (c IAMService) ListUserGroupPage(ctx RequestContext, page PageRequest) (PageResponse[UserGroup], error) {
	items, err := c.ListUserGroups(ctx)
	if err != nil {
		return PageResponse[UserGroup]{}, err
	}
	items = utils.SortUserGroups(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

// CreateUserGroup 建立使用者群組的服務流程。
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
			if err := tx.store.UpsertGroupMembership(goContext(ctx), GroupMembership{
				ID:          utils.NewID("ugm"),
				TenantID:    ctx.TenantID,
				UserGroupID: group.ID,
				AccountID:   accountID,
				ValidFrom:   group.CreatedAt,
				Source:      "manual",
				CreatedBy:   ctx.AccountID,
				CreatedAt:   group.CreatedAt,
			}); err != nil {
				return err
			}
		}
		if err := tx.Service.syncUserGroupRelationshipTuples(ctx, UserGroup{}, group); err != nil {
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

// UpdateUserGroup 更新使用者群組基本資訊與權限集合。
func (c IAMService) UpdateUserGroup(ctx RequestContext, id string, input UpdateUserGroupInput) (UserGroup, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceUserGroup, ActionUpdate, id); err != nil {
		return UserGroup{}, err
	}
	group, ok, err := c.store.GetUserGroup(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return UserGroup{}, err
	}
	if !ok {
		return UserGroup{}, NotFound("user group", id)
	}
	next := group
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return UserGroup{}, BadRequest("user group name is required")
		}
		next.Name = name
	}
	if input.Description != nil {
		next.Description = strings.TrimSpace(*input.Description)
	}
	if input.PermissionSetIDs != nil {
		for _, permissionSetID := range input.PermissionSetIDs {
			if _, ok, err := c.store.GetPermissionSet(goContext(ctx), ctx.TenantID, permissionSetID); err != nil {
				return UserGroup{}, err
			} else if !ok {
				return UserGroup{}, NotFound("permission set", permissionSetID)
			}
		}
		next.PermissionSetIDs = uniqueStrings(input.PermissionSetIDs)
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		if err := tx.store.UpsertUserGroup(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.touchAuthzConfig(ctx, "iam.user_group.upsert", map[string]any{"user_group_id": next.ID}); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.user_group.update", "user_group", next.ID, "high", map[string]any{
			"name":               next.Name,
			"permission_set_ids": next.PermissionSetIDs,
		})
	}); err != nil {
		return UserGroup{}, err
	}
	c.logWarn(ctx, "user group updated",
		"user_group_id", next.ID,
		"permission_set_count", len(next.PermissionSetIDs),
	)
	return next, nil
}

// ListUserGroupMemberPage 列出使用者群組成員分頁。
func (c IAMService) ListUserGroupMemberPage(ctx RequestContext, groupID string, page PageRequest) (PageResponse[GroupMembership], error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceUserGroup, ActionRead, groupID); err != nil {
		return PageResponse[GroupMembership]{}, err
	}
	if _, ok, err := c.store.GetUserGroup(goContext(ctx), ctx.TenantID, groupID); err != nil {
		return PageResponse[GroupMembership]{}, err
	} else if !ok {
		return PageResponse[GroupMembership]{}, NotFound("user group", groupID)
	}
	items, err := c.store.ListGroupMembershipsForGroup(goContext(ctx), ctx.TenantID, groupID)
	if err != nil {
		return PageResponse[GroupMembership]{}, err
	}
	return utils.PageResponse(items, page), nil
}

// AddUserGroupMember 新增或更新使用者群組成員。
func (c IAMService) AddUserGroupMember(ctx RequestContext, groupID string, input AddUserGroupMemberInput) (GroupMembership, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceUserGroup, ActionUpdate, groupID); err != nil {
		return GroupMembership{}, err
	}
	accountID := strings.TrimSpace(input.AccountID)
	if accountID == "" {
		return GroupMembership{}, BadRequest("account_id is required")
	}
	group, ok, err := c.store.GetUserGroup(goContext(ctx), ctx.TenantID, groupID)
	if err != nil {
		return GroupMembership{}, err
	}
	if !ok {
		return GroupMembership{}, NotFound("user group", groupID)
	}
	if _, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, accountID); err != nil {
		return GroupMembership{}, err
	} else if !ok {
		return GroupMembership{}, NotFound("account", accountID)
	}
	validUntil, err := optionalDateTime(input.ValidUntil)
	if err != nil {
		return GroupMembership{}, BadRequest("valid_until must be RFC3339 or YYYY-MM-DD")
	}
	source, err := normalizeGroupMembershipSource(input.Source)
	if err != nil {
		return GroupMembership{}, err
	}
	now := c.Now()
	approvalID := strings.TrimSpace(input.ApprovalInstanceID)
	if approvalID == "" {
		approvalID = ctx.ApprovalInstanceID
	}
	membership := GroupMembership{
		ID:                 utils.NewID("ugm"),
		TenantID:           ctx.TenantID,
		UserGroupID:        group.ID,
		AccountID:          accountID,
		ValidFrom:          now,
		ValidUntil:         validUntil,
		Source:             source,
		ApprovalInstanceID: approvalID,
		CreatedBy:          ctx.AccountID,
		CreatedAt:          now,
	}
	after := group
	after.MemberAccountIDs = uniqueStrings(append(after.MemberAccountIDs, accountID))
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		if err := tx.store.UpsertGroupMembership(goContext(ctx), membership); err != nil {
			return err
		}
		if err := tx.store.UpsertUserGroup(goContext(ctx), after); err != nil {
			return err
		}
		if err := tx.store.AddAccountGroup(goContext(ctx), ctx.TenantID, accountID, group.ID); err != nil {
			return err
		}
		if err := tx.Service.syncUserGroupRelationshipTuples(ctx, group, after); err != nil {
			return err
		}
		if err := tx.touchAuthzConfig(ctx, "iam.user_group.member.add", map[string]any{
			"user_group_id": group.ID,
			"account_id":    accountID,
		}); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.user_group.member.add", "user_group", group.ID, "high", groupMembershipAuditDetails(membership))
	}); err != nil {
		return GroupMembership{}, err
	}
	if saved, ok, err := c.store.GetGroupMembership(goContext(ctx), ctx.TenantID, group.ID, accountID); err != nil {
		return GroupMembership{}, err
	} else if ok {
		membership = saved
	}
	c.logWarn(ctx, "user group member added",
		"user_group_id", group.ID,
		"account_id", accountID,
		"source", source,
	)
	return membership, nil
}

// RemoveUserGroupMember 移除使用者群組成員。
func (c IAMService) RemoveUserGroupMember(ctx RequestContext, groupID, accountID string) error {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceUserGroup, ActionUpdate, groupID); err != nil {
		return err
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return BadRequest("account_id is required")
	}
	group, ok, err := c.store.GetUserGroup(goContext(ctx), ctx.TenantID, groupID)
	if err != nil {
		return err
	}
	if !ok {
		return NotFound("user group", groupID)
	}
	after := group
	after.MemberAccountIDs = removeString(after.MemberAccountIDs, accountID)
	existing, membershipExists, err := c.store.GetGroupMembership(goContext(ctx), ctx.TenantID, groupID, accountID)
	if err != nil {
		return err
	}
	if !membershipExists && !utils.ContainsString(group.MemberAccountIDs, accountID) {
		return NotFound("group membership", accountID)
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		if _, _, err := tx.store.DeleteGroupMembership(goContext(ctx), ctx.TenantID, groupID, accountID); err != nil {
			return err
		}
		if err := tx.store.UpsertUserGroup(goContext(ctx), after); err != nil {
			return err
		}
		if err := tx.store.RemoveAccountGroup(goContext(ctx), ctx.TenantID, accountID, groupID); err != nil {
			return err
		}
		if err := tx.Service.syncUserGroupRelationshipTuples(ctx, group, after); err != nil {
			return err
		}
		if err := tx.touchAuthzConfig(ctx, "iam.user_group.member.remove", map[string]any{
			"user_group_id": groupID,
			"account_id":    accountID,
		}); err != nil {
			return err
		}
		details := groupMembershipAuditDetails(existing)
		details["account_id"] = accountID
		details["user_group_id"] = groupID
		return tx.audit(ctx, "iam.user_group.member.remove", "user_group", groupID, "high", details)
	}); err != nil {
		return err
	}
	c.logWarn(ctx, "user group member removed",
		"user_group_id", groupID,
		"account_id", accountID,
	)
	return nil
}

// normalizeGroupMembershipSource 正規化使用者群組成員來源。
func normalizeGroupMembershipSource(value string) (string, error) {
	source := strings.TrimSpace(value)
	if source == "" {
		source = "manual"
	}
	switch source {
	case "manual", "import", "template", "approval":
		return source, nil
	default:
		return "", BadRequest("source must be manual, import, template or approval")
	}
}

// groupMembershipAuditDetails 建立群組成員審計 details。
func groupMembershipAuditDetails(membership GroupMembership) map[string]any {
	details := map[string]any{
		"user_group_id":        membership.UserGroupID,
		"account_id":           membership.AccountID,
		"valid_until":          "",
		"source":               membership.Source,
		"approval_instance_id": membership.ApprovalInstanceID,
	}
	if membership.ValidUntil != nil {
		details["valid_until"] = membership.ValidUntil.UTC().Format(time.RFC3339)
	}
	return details
}

// removeString 移除指定字串並保持其餘順序。
func removeString(values []string, value string) []string {
	out := make([]string, 0, len(values))
	for _, item := range values {
		if item != value {
			out = append(out, item)
		}
	}
	return out
}

// ListPermissionSetAssignments 列出權限集合指派的服務流程。
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
	if allowed, checked, err := c.openFGAAssumeRoleAllows(ctx, account, role); checked {
		if err != nil {
			c.logWarn(ctx, "openfga assume role check failed; falling back to trust policy",
				"assumable_role_id", role.ID,
				"error", err,
			)
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

// trustPolicyAllowsAccount 處理 trust 政策 allows 帳號的服務流程。
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
