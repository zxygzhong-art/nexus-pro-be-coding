package service

import (
	"strings"
	"time"

	"nexus-pro-be/internal/utils"
)

// ListUserGroups returns groups with membership rows as the authoritative current member source.
func (c IAMService) ListUserGroups(ctx RequestContext) ([]UserGroup, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceUserGroup, ActionRead, ""); err != nil {
		return nil, err
	}
	groups, err := c.store.ListUserGroups(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	at := c.Now()
	for i := range groups {
		groups[i], err = c.userGroupWithActiveMembers(ctx, groups[i], at)
		if err != nil {
			return nil, err
		}
	}
	return groups, nil
}

// ListUserGroupPage 列出使用者羣組分頁的服務流程。
func (c IAMService) ListUserGroupPage(ctx RequestContext, page PageRequest) (PageResponse[UserGroup], error) {
	items, err := c.ListUserGroups(ctx)
	if err != nil {
		return PageResponse[UserGroup]{}, err
	}
	items = utils.SortUserGroups(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

// CreateUserGroup 建立使用者羣組的服務流程。
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
		storedGroup := group
		storedGroup.MemberAccountIDs = nil
		if err := tx.store.UpsertUserGroup(goContext(ctx), storedGroup); err != nil {
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

// UpdateUserGroup 更新使用者羣組基本資訊與權限集合。
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
	group, err = c.userGroupWithActiveMembers(ctx, group, c.Now())
	if err != nil {
		return UserGroup{}, err
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

// DeleteUserGroup 刪除使用者羣組；若仍有成員、指派或 trust_policy 引用則拒絕。
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
	group, err = c.userGroupWithActiveMembers(ctx, group, c.Now())
	if err != nil {
		return UserGroup{}, err
	}
	if err := c.ensureUserGroupDeletable(ctx, group); err != nil {
		return UserGroup{}, err
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		if err := tx.Service.syncUserGroupRelationshipTuples(ctx, group, UserGroup{ID: group.ID, TenantID: group.TenantID}); err != nil {
			return err
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

// ensureUserGroupDeletable 檢查使用者羣組是否仍有成員、指派或 trust_policy 引用。
func (c IAMService) ensureUserGroupDeletable(ctx RequestContext, group UserGroup) error {
	memberships, err := c.store.ListGroupMembershipsForGroup(goContext(ctx), ctx.TenantID, group.ID)
	if err != nil {
		return err
	}
	for _, membership := range memberships {
		if groupMembershipActiveAt(membership, c.Now()) {
			return Conflict("user group still has members")
		}
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

// assumableRoleTrustPolicyReferencesUserGroup 判斷 trust_policy 是否引用指定使用者羣組。
func assumableRoleTrustPolicyReferencesUserGroup(role AssumableRole, groupID string) bool {
	if len(role.TrustPolicy) == 0 {
		return false
	}
	allowed := stringSet(append(stringSliceFromAny(role.TrustPolicy["user_groups"]), stringSliceFromAny(role.TrustPolicy["user_group_ids"])...))
	_, ok := allowed[groupID]
	return ok
}

// ListUserGroupMemberPage 列出使用者羣組成員分頁。
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
	active := make([]GroupMembership, 0, len(items))
	for _, item := range items {
		if groupMembershipActiveAt(item, c.Now()) {
			active = append(active, item)
		}
	}
	return utils.PageResponse(active, page), nil
}

// AddUserGroupMember 新增或更新使用者羣組成員。
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
	group, err = c.userGroupWithActiveMembers(ctx, group, c.Now())
	if err != nil {
		return GroupMembership{}, err
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
	if source := strings.TrimSpace(input.Source); source != "" && source != "manual" {
		return GroupMembership{}, BadRequest("direct group membership changes must use source manual")
	}
	if strings.TrimSpace(input.ApprovalInstanceID) != "" {
		return GroupMembership{}, BadRequest("approval_instance_id must be supplied by a verified workflow integration")
	}
	source := "manual"
	now := c.Now()
	if validUntil != nil && !now.Before(*validUntil) {
		return GroupMembership{}, BadRequest("valid_until must be later than valid_from")
	}
	membership := GroupMembership{
		ID:                 utils.NewID("ugm"),
		TenantID:           ctx.TenantID,
		UserGroupID:        group.ID,
		AccountID:          accountID,
		ValidFrom:          now,
		ValidUntil:         validUntil,
		Source:             source,
		ApprovalInstanceID: "",
		CreatedBy:          ctx.AccountID,
		CreatedAt:          now,
	}
	if current, exists, err := c.store.GetGroupMembership(goContext(ctx), ctx.TenantID, group.ID, accountID); err != nil {
		return GroupMembership{}, err
	} else if exists && groupMembershipActiveAt(current, now) {
		membership.ID = current.ID
		membership.ValidFrom = current.ValidFrom
		membership.CreatedAt = current.CreatedAt
	}
	after := group
	after.MemberAccountIDs = uniqueStrings(append(after.MemberAccountIDs, accountID))
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		if err := tx.store.UpsertGroupMembership(goContext(ctx), membership); err != nil {
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

// RemoveUserGroupMember 移除使用者羣組成員。
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
	group, err = c.userGroupWithActiveMembers(ctx, group, c.Now())
	if err != nil {
		return err
	}
	after := group
	after.MemberAccountIDs = removeString(after.MemberAccountIDs, accountID)
	existing, membershipExists, err := c.store.GetGroupMembership(goContext(ctx), ctx.TenantID, groupID, accountID)
	if err != nil {
		return err
	}
	if !membershipExists || !groupMembershipActiveAt(existing, c.Now()) {
		return NotFound("group membership", accountID)
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		if _, ok, err := tx.store.CloseGroupMembership(goContext(ctx), ctx.TenantID, groupID, accountID, tx.Now()); err != nil {
			return err
		} else if !ok {
			return NotFound("group membership", accountID)
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

// userGroupWithActiveMembers rebuilds the compatibility projection from membership history.
func (c IAMService) userGroupWithActiveMembers(ctx RequestContext, group UserGroup, at time.Time) (UserGroup, error) {
	memberships, err := c.store.ListGroupMembershipsForGroup(goContext(ctx), ctx.TenantID, group.ID)
	if err != nil {
		return UserGroup{}, err
	}
	members := make([]string, 0, len(memberships))
	for _, membership := range memberships {
		if groupMembershipActiveAt(membership, at) {
			members = append(members, membership.AccountID)
		}
	}
	group.MemberAccountIDs = uniqueStrings(members)
	return group, nil
}

// groupMembershipAuditDetails 建立羣組成員審計 details。
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
