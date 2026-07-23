package v1

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/service"
)

// IAMCtrl 定義 IAM ctrl 的資料結構。
type IAMCtrl struct {
	routes routeBinder
	svc    service.IAMFacade
}

// RegisterRoutes 註冊此 controller 的 HTTP 路由。
func (c IAMCtrl) RegisterRoutes(router *gin.RouterGroup) {
	iam := router.Group("/iam")
	iam.GET("/permissions", c.routes.Handle("iam.permission", "read", c.listPermissions))
	iam.GET("/permission-packages", c.routes.Handle("iam.permission_package", "read", c.listPermissionPackages))
	iam.GET("/user-groups", c.routes.Handle("iam.user_group", "read", c.listUserGroups))
	iam.GET("/user-groups/options", c.routes.Handle("iam.user_group", "read", c.listUserGroupOptions))
	iam.POST("/user-groups", c.routes.Handle("iam.user_group", "create", c.createUserGroup))
	iam.PATCH("/user-groups/:id", c.routes.Handle("iam.user_group", "update", c.updateUserGroup, ResourceID(PathParamID)))
	iam.GET("/user-groups/:id/members", c.routes.Handle("iam.user_group", "read", c.listUserGroupMembers, ResourceID(PathParamID)))
	iam.POST("/user-groups/:id/members", c.routes.Handle("iam.user_group", "update", c.addUserGroupMember, ResourceID(PathParamID)))
	iam.DELETE("/user-groups/:id/members/:accountId", c.routes.Handle("iam.user_group", "update", c.removeUserGroupMember, ResourceID(PathParamID), PathParam("accountId")))
	iam.GET("/permission-sets", c.routes.Handle("iam.permission_set", "read", c.listPermissionSets))
	iam.GET("/permission-sets/options", c.routes.Handle("iam.permission_set", "read", c.listPermissionSetOptions))
	iam.POST("/permission-sets", c.routes.Handle("iam.permission_set", "create", c.createPermissionSet))
	iam.PATCH("/permission-sets/:id", c.routes.Handle("iam.permission_set", "update", c.updatePermissionSet, ResourceID(PathParamID)))
	iam.GET("/accounts", c.routes.Handle("iam.account", "read", c.listAccounts))
	iam.GET("/accounts/options", c.routes.Handle("iam.account", "read", c.listAccountOptions))
	iam.GET("/permission-set-assignments", c.routes.Handle("iam.permission_set_assignment", "read", c.listPermissionSetAssignments))
	iam.POST("/permission-set-assignments", c.routes.Handle("iam.permission_set_assignment", "create", c.createPermissionSetAssignment))
	iam.DELETE("/permission-set-assignments/:id", c.routes.Handle("iam.permission_set_assignment", "delete", c.deletePermissionSetAssignment, ResourceID(PathParamID)))
	iam.GET("/data-scopes", c.routes.Handle("iam.data_scope", "read", c.listDataScopes))
	iam.POST("/data-scopes", c.routes.Handle("iam.data_scope", "create", c.createDataScope))
	iam.PATCH("/data-scopes/:id", c.routes.Handle("iam.data_scope", "update", c.updateDataScope, ResourceID(PathParamID)))
	iam.DELETE("/data-scopes/:id", c.routes.Handle("iam.data_scope", "delete", c.deleteDataScope, ResourceID(PathParamID)))
	iam.GET("/field-policies", c.routes.Handle("iam.field_policy", "read", c.listFieldPolicies))
	iam.POST("/field-policies", c.routes.Handle("iam.field_policy", "create", c.createFieldPolicy))
	iam.PATCH("/field-policies/:id", c.routes.Handle("iam.field_policy", "update", c.updateFieldPolicy, ResourceID(PathParamID)))
	iam.DELETE("/field-policies/:id", c.routes.Handle("iam.field_policy", "delete", c.deleteFieldPolicy, ResourceID(PathParamID)))
	iam.GET("/assumable-roles", c.routes.Handle("iam.assumable_role", "read", c.listAssumableRoles))
	iam.GET("/assumable-roles/options", c.routes.Handle("iam.assumable_role", "read", c.listAssumableRoleOptions))
	iam.POST("/assumable-roles", c.routes.Handle("iam.assumable_role", "create", c.createAssumableRole))
	iam.POST("/assumable-roles/:id/assume", c.routes.Handle("iam.assumable_role", "assume", c.assumeRole, ResourceID(PathParamID)))
	// Returning a temporary role is governed by the caller's base me.read access,
	// not by the narrowed assumed-role permissions. The service rechecks ownership.
	iam.DELETE("/assumable-role-sessions/current", c.routes.Handle("platform.me", "read", c.revokeCurrentAssumableRoleSession, CurrentAccessProjection()))
}

// listPermissions 處理權限的 HTTP 請求。
func (c IAMCtrl) listPermissions(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListPermissionPage(ctx, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// listPermissionPackages 處理權限包列表的 HTTP 請求。
func (c IAMCtrl) listPermissionPackages(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListPermissionPackagePage(ctx, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// listUserGroups 處理使用者羣組的 HTTP 請求。
func (c IAMCtrl) listUserGroups(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListUserGroupPage(ctx, r.URL.Query().Get("q"), page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// createUserGroup 處理使用者羣組的 HTTP 請求。
func (c IAMCtrl) createUserGroup(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateUserGroupInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateUserGroup(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// updateUserGroup 處理使用者羣組更新的 HTTP 請求。
func (c IAMCtrl) updateUserGroup(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateUserGroupInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateUserGroup(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// listUserGroupMembers 處理使用者羣組成員列表的 HTTP 請求。
func (c IAMCtrl) listUserGroupMembers(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListUserGroupMemberPage(ctx, r.PathValue(PathParamID), page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// addUserGroupMember 處理新增使用者羣組成員的 HTTP 請求。
func (c IAMCtrl) addUserGroupMember(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.AddUserGroupMemberInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.AddUserGroupMember(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// removeUserGroupMember 處理移除使用者羣組成員的 HTTP 請求。
func (c IAMCtrl) removeUserGroupMember(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	if err := c.svc.RemoveUserGroupMember(ctx, r.PathValue(PathParamID), r.PathValue("accountId")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// listPermissionSets 處理權限集合的 HTTP 請求。
func (c IAMCtrl) listPermissionSets(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListPermissionSetPage(ctx, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// createPermissionSet 處理權限集合的 HTTP 請求。
func (c IAMCtrl) createPermissionSet(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreatePermissionSetInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreatePermissionSet(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// updatePermissionSet 處理權限集合更新的 HTTP 請求。
func (c IAMCtrl) updatePermissionSet(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdatePermissionSetInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdatePermissionSet(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// listAccounts 處理 IAM 帳號目錄的 HTTP 請求。
func (c IAMCtrl) listAccounts(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListIamAccountPage(ctx, r.URL.Query().Get("q"), page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// listPermissionSetAssignments 處理權限集合指派的 HTTP 請求。
func (c IAMCtrl) listPermissionSetAssignments(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	query := domain.PermissionSetAssignmentQuery{
		PrincipalType: strings.TrimSpace(r.URL.Query().Get("principal_type")),
		PrincipalID:   strings.TrimSpace(r.URL.Query().Get("principal_id")),
	}
	items, err := c.svc.ListPermissionSetAssignmentPage(ctx, query, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// createPermissionSetAssignment 處理權限集合指派的 HTTP 請求。
func (c IAMCtrl) createPermissionSetAssignment(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreatePermissionSetAssignmentInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreatePermissionSetAssignment(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// deletePermissionSetAssignment 處理權限集合指派刪除的 HTTP 請求。
func (c IAMCtrl) deletePermissionSetAssignment(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeletePermissionSetAssignment(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// listDataScopes 處理資料範圍的 HTTP 請求。
func (c IAMCtrl) listDataScopes(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListDataScopePage(ctx, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// createDataScope 處理資料範圍的 HTTP 請求。
func (c IAMCtrl) createDataScope(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateDataScopeInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateDataScope(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// updateDataScope 處理資料範圍更新的 HTTP 請求。
func (c IAMCtrl) updateDataScope(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateDataScopeInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateDataScope(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// deleteDataScope 處理資料範圍刪除的 HTTP 請求。
func (c IAMCtrl) deleteDataScope(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteDataScope(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// listFieldPolicies 處理欄位政策的 HTTP 請求。
func (c IAMCtrl) listFieldPolicies(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListFieldPolicyPage(ctx, r.URL.Query().Get("application_code"), r.URL.Query().Get("resource_type"), page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// createFieldPolicy 處理欄位政策的 HTTP 請求。
func (c IAMCtrl) createFieldPolicy(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateFieldPolicyInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateFieldPolicy(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// updateFieldPolicy 處理欄位政策更新的 HTTP 請求。
func (c IAMCtrl) updateFieldPolicy(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateFieldPolicyInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateFieldPolicy(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// deleteFieldPolicy 處理欄位政策刪除的 HTTP 請求。
func (c IAMCtrl) deleteFieldPolicy(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteFieldPolicy(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// listAssumableRoles 處理 assumable 角色的 HTTP 請求。
func (c IAMCtrl) listAssumableRoles(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListAssumableRolePage(ctx, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// createAssumableRole 處理 assumable 角色的 HTTP 請求。
func (c IAMCtrl) createAssumableRole(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateAssumableRoleInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateAssumableRole(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// assumeRole 處理角色的 HTTP 請求。
func (c IAMCtrl) assumeRole(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.AssumeRoleInput
	if _, err := readOptionalJSON(w, r, &input); err != nil {
		return err
	}
	result, err := c.svc.AssumeRole(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, result)
	return nil
}

// revokeCurrentAssumableRoleSession ends only the caller-owned temporary role session.
func (c IAMCtrl) revokeCurrentAssumableRoleSession(w http.ResponseWriter, _ *http.Request, ctx domain.RequestContext) error {
	if err := c.svc.RevokeCurrentAssumableRoleSession(ctx); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// listAccountOptions 處理 IAM 帳號選項的 HTTP 請求。
func (c IAMCtrl) listAccountOptions(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	query, err := optionQueryFromRequest(r)
	if err != nil {
		return err
	}
	options, err := c.svc.ListIamAccountOptions(ctx, query)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, options)
	return nil
}

// listPermissionSetOptions 處理權限集合選項的 HTTP 請求。
func (c IAMCtrl) listPermissionSetOptions(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	query, err := optionQueryFromRequest(r)
	if err != nil {
		return err
	}
	options, err := c.svc.ListPermissionSetOptions(ctx, query)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, options)
	return nil
}

// listUserGroupOptions 處理使用者羣組選項的 HTTP 請求。
func (c IAMCtrl) listUserGroupOptions(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	query, err := optionQueryFromRequest(r)
	if err != nil {
		return err
	}
	options, err := c.svc.ListUserGroupOptions(ctx, query)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, options)
	return nil
}

// listAssumableRoleOptions 處理 assumable 角色選項的 HTTP 請求。
func (c IAMCtrl) listAssumableRoleOptions(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	query, err := optionQueryFromRequest(r)
	if err != nil {
		return err
	}
	options, err := c.svc.ListAssumableRoleOptions(ctx, query)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, options)
	return nil
}

// optionQueryFromRequest 解析輕量選項查詢（q 模糊搜尋 + cursor/page_size 遊標分頁）。
func optionQueryFromRequest(r *http.Request) (domain.OptionQuery, error) {
	values := r.URL.Query()
	pageSize, err := positiveIntQuery(values.Get("page_size"), "page_size", domain.MaxPageSize)
	if err != nil {
		return domain.OptionQuery{}, err
	}
	return domain.OptionQuery{
		Keyword:  strings.TrimSpace(values.Get("q")),
		Cursor:   strings.TrimSpace(values.Get("cursor")),
		PageSize: pageSize,
	}, nil
}
