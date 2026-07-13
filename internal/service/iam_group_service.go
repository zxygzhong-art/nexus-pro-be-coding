package service

import (
	"nexus-pro-be/internal/utils"
	"strings"
	"time"
)

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

// DeleteUserGroup 刪除使用者群組；若仍有成員、指派或 trust_policy 引用則拒絕。
func (c IAMService) DeleteUserGroup(ctx RequestContext, id string) (UserGroup, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceUserGroup, ActionDelete, id); err != nil {
		return UserGroup{}, err
	}
	group, ok, err := c.store.GetUserGroup(goContext(ctx), ctx.TenantID, strings.TrimSpace(id))
	if err != nil {
		return UserGroup{}, err
	}
	if !ok {
		return UserGroup{}, NotFound("user group", id)
	}
	if err := c.ensureUserGroupDeletable(ctx, group); err != nil {
		return UserGroup{}, err
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		memberships, err := tx.store.ListGroupMembershipsForGroup(goContext(ctx), ctx.TenantID, group.ID)
		if err != nil {
			return err
		}
		for _, membership := range memberships {
			if _, _, err := tx.store.DeleteGroupMembership(goContext(ctx), ctx.TenantID, membership.UserGroupID, membership.AccountID); err != nil {
				return err
			}
		}
		if err := tx.Service.syncUserGroupRelationshipTuples(ctx, group, UserGroup{ID: group.ID, TenantID: group.TenantID}); err != nil {
			return err
		}
		accounts, err := tx.store.ListAccounts(goContext(ctx), ctx.TenantID)
		if err != nil {
			return err
		}
		for _, account := range accounts {
			if !utils.ContainsString(account.UserGroupIDs, group.ID) {
				continue
			}
			if err := tx.store.RemoveAccountGroup(goContext(ctx), ctx.TenantID, account.ID, group.ID); err != nil {
				return err
			}
		}
		deleted, ok, err := tx.store.DeleteUserGroup(goContext(ctx), ctx.TenantID, group.ID)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("user group", id)
		}
		if err := tx.touchAuthzConfig(ctx, "iam.user_group.delete", map[string]any{"user_group_id": deleted.ID}); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.user_group.delete", "user_group", deleted.ID, "high", map[string]any{
			"name": deleted.Name,
		})
	}); err != nil {
		return UserGroup{}, err
	}
	c.logWarn(ctx, "user group deleted", "user_group_id", group.ID)
	return group, nil
}

// ensureUserGroupDeletable 檢查使用者群組是否仍有成員、指派或 trust_policy 引用。
func (c IAMService) ensureUserGroupDeletable(ctx RequestContext, group UserGroup) error {
	if len(group.MemberAccountIDs) > 0 {
		return Conflict("user group still has members")
	}
	memberships, err := c.store.ListGroupMembershipsForGroup(goContext(ctx), ctx.TenantID, group.ID)
	if err != nil {
		return err
	}
	if len(memberships) > 0 {
		return Conflict("user group still has members")
	}
	now := c.Now()
	assignments, err := c.store.ListPermissionSetAssignments(goContext(ctx), ctx.TenantID)
	if err != nil {
		return err
	}
	for _, assignment := range assignments {
		if assignment.PrincipalType == string(PrincipalTypeUserGroup) && assignment.PrincipalID == group.ID && permissionSetAssignmentActive(assignment, now) {
			return Conflict("user group has active permission set assignments")
		}
	}
	roles, err := c.store.ListAssumableRoles(goContext(ctx), ctx.TenantID)
	if err != nil {
		return err
	}
	for _, role := range roles {
		if assumableRoleTrustPolicyReferencesUserGroup(role, group.ID) {
			return Conflict("user group is referenced by assumable role trust policy")
		}
	}
	return nil
}

// assumableRoleTrustPolicyReferencesUserGroup 判斷 trust_policy 是否引用指定使用者群組。
func assumableRoleTrustPolicyReferencesUserGroup(role AssumableRole, groupID string) bool {
	if len(role.TrustPolicy) == 0 {
		return false
	}
	allowed := stringSet(append(stringSliceFromAny(role.TrustPolicy["user_groups"]), stringSliceFromAny(role.TrustPolicy["user_group_ids"])...))
	_, ok := allowed[groupID]
	return ok
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
	membership := GroupMembership{
		ID:                 utils.NewID("ugm"),
		TenantID:           ctx.TenantID,
		UserGroupID:        group.ID,
		AccountID:          accountID,
		ValidFrom:          now,
		ValidUntil:         validUntil,
		Source:             source,
		ApprovalInstanceID: strings.TrimSpace(input.ApprovalInstanceID),
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
