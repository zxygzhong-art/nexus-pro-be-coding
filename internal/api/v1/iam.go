package v1

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// IAMCtrl 定義 IAM ctrl 的資料結構。
type IAMCtrl struct {
	routes routeBinder
	svc    service.IAMFacade
}

// RegisterRoutes 註冊此 controller 的 HTTP 路由。
func (c IAMCtrl) RegisterRoutes(router *gin.RouterGroup) {
	iam := router.Group("/iam")
	iam.GET("/applications", c.routes.Handle("iam.application", "read", c.listApplications))
	iam.GET("/resource-types", c.routes.Handle("iam.resource_type", "read", c.listResourceTypes))
	iam.GET("/permissions", c.routes.Handle("iam.permission", "read", c.listPermissions))
	iam.GET("/permission-packages", c.routes.Handle("iam.permission_package", "read", c.listPermissionPackages))
	iam.POST("/permission-packages", c.routes.Handle("iam.permission_package", "create", c.registerPermissionPackage))
	iam.POST("/permission-packages/:id/publish", c.routes.Handle("iam.permission_package", "publish", c.publishPermissionPackage, ResourceID(PathParamID)))
	iam.POST("/permission-packages/:id/import", c.routes.Handle("iam.permission_package", "import", c.importPermissionPackage, ResourceID(PathParamID)))
	iam.GET("/roles", c.routes.Handle("iam.assumable_role", "read", c.listRoles))
	iam.GET("/role-bindings", c.routes.Handle("iam.permission_set_assignment", "read", c.listRoleBindings))
	iam.GET("/user-groups", c.routes.Handle("iam.user_group", "read", c.listUserGroups))
	iam.POST("/user-groups", c.routes.Handle("iam.user_group", "create", c.createUserGroup))
	iam.PATCH("/user-groups/:id", c.routes.Handle("iam.user_group", "update", c.updateUserGroup, ResourceID(PathParamID)))
	iam.DELETE("/user-groups/:id", c.routes.Handle("iam.user_group", "delete", c.deleteUserGroup, ResourceID(PathParamID)))
	iam.GET("/user-groups/:id/members", c.routes.Handle("iam.user_group", "read", c.listUserGroupMembers, ResourceID(PathParamID)))
	iam.POST("/user-groups/:id/members", c.routes.Handle("iam.user_group", "update", c.addUserGroupMember, ResourceID(PathParamID)))
	iam.DELETE("/user-groups/:id/members/:accountId", c.routes.Handle("iam.user_group", "update", c.removeUserGroupMember, ResourceID(PathParamID), PathParam("accountId")))
	iam.GET("/permission-sets", c.routes.Handle("iam.permission_set", "read", c.listPermissionSets))
	iam.POST("/permission-sets", c.routes.Handle("iam.permission_set", "create", c.createPermissionSet))
	iam.PATCH("/permission-sets/:id", c.routes.Handle("iam.permission_set", "update", c.updatePermissionSet, ResourceID(PathParamID)))
	iam.DELETE("/permission-sets/:id", c.routes.Handle("iam.permission_set", "delete", c.deletePermissionSet, ResourceID(PathParamID)))
	iam.GET("/accounts", c.routes.Handle("iam.account", "read", c.listAccounts))
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
	iam.GET("/outbox-events", c.routes.Handle("iam.outbox_event", "read", c.listOutboxEvents))
	iam.POST("/outbox-events/:id/retry", c.routes.Handle("iam.outbox_event", "update", c.retryOutboxEvent, ResourceID(PathParamID)))
	iam.GET("/assumable-roles", c.routes.Handle("iam.assumable_role", "read", c.listAssumableRoles))
	iam.POST("/assumable-roles", c.routes.Handle("iam.assumable_role", "create", c.createAssumableRole))
	iam.PATCH("/assumable-roles/:id", c.routes.Handle("iam.assumable_role", "update", c.updateAssumableRole, ResourceID(PathParamID)))
	iam.DELETE("/assumable-roles/:id", c.routes.Handle("iam.assumable_role", "delete", c.deleteAssumableRole, ResourceID(PathParamID)))
	iam.POST("/assumable-roles/:id/assume", c.routes.Handle("iam.assumable_role", "assume", c.assumeRole, ResourceID(PathParamID)))
	// Returning a temporary role is governed by the caller's base me.read access,
	// not by the narrowed assumed-role permissions. The service rechecks ownership.
	iam.DELETE("/assumable-role-sessions/current", c.routes.Handle("platform.me", "read", c.revokeCurrentAssumableRoleSession, CurrentAccessProjection()))
}

// listApplications 處理 applications 目錄的 HTTP 請求。
func (c IAMCtrl) listApplications(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	items, err := c.svc.ListApplications(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// listResourceTypes 處理 resource types 目錄的 HTTP 請求。
func (c IAMCtrl) listResourceTypes(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	items, err := c.svc.ListResourceTypes(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
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

// registerPermissionPackage 處理權限包註冊的 HTTP 請求。
func (c IAMCtrl) registerPermissionPackage(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.PermissionPackageContent
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.RegisterPermissionPackage(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// publishPermissionPackage 處理權限包發布的 HTTP 請求。
func (c IAMCtrl) publishPermissionPackage(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.PublishPermissionPackage(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// importPermissionPackage 處理權限包導入的 HTTP 請求。
func (c IAMCtrl) importPermissionPackage(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.ImportPermissionPackage(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// listRoles 處理 roles 相容投影的 HTTP 請求。
func (c IAMCtrl) listRoles(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListRolePage(ctx, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// listRoleBindings 處理 role-bindings 相容投影的 HTTP 請求。
func (c IAMCtrl) listRoleBindings(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListRoleBindingPage(ctx, page)
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
	items, err := c.svc.ListUserGroupPage(ctx, page)
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

// deleteUserGroup 處理使用者羣組刪除的 HTTP 請求。
func (c IAMCtrl) deleteUserGroup(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteUserGroup(ctx, r.PathValue(PathParamID))
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

// deletePermissionSet 處理權限集合刪除的 HTTP 請求。
func (c IAMCtrl) deletePermissionSet(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeletePermissionSet(ctx, r.PathValue(PathParamID))
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

// listOutboxEvents 處理 outbox 事件列表的 HTTP 請求。
func (c IAMCtrl) listOutboxEvents(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	query, err := outboxEventQueryFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListOutboxEventPage(ctx, query, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// retryOutboxEvent 處理 outbox 事件重試的 HTTP 請求。
func (c IAMCtrl) retryOutboxEvent(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.RetryOutboxEvent(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// outboxEventQueryFromRequest 解析 outbox 事件查詢。
func outboxEventQueryFromRequest(r *http.Request) (domain.OutboxEventQuery, error) {
	query := r.URL.Query()
	out := domain.OutboxEventQuery{
		Status:    strings.TrimSpace(query.Get("status")),
		EventType: strings.TrimSpace(query.Get("event_type")),
		LastError: strings.TrimSpace(query.Get("last_error")),
	}
	if raw := strings.TrimSpace(query.Get("has_error")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return domain.OutboxEventQuery{}, domain.BadRequest("has_error must be a boolean")
		}
		out.HasError = &value
	}
	if raw := strings.TrimSpace(query.Get("retry_count")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return domain.OutboxEventQuery{}, domain.BadRequest("retry_count must be a non-negative integer")
		}
		out.RetryCount = &value
	}
	return out, nil
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

// updateAssumableRole 處理 assumable 角色更新的 HTTP 請求。
func (c IAMCtrl) updateAssumableRole(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateAssumableRoleInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateAssumableRole(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// deleteAssumableRole 處理 assumable 角色刪除的 HTTP 請求。
func (c IAMCtrl) deleteAssumableRole(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteAssumableRole(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
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
