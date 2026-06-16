package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

type IAMCtrl struct {
	routes routeBinder
	svc    service.IAMService
}

func (c IAMCtrl) RegisterRoutes(router *gin.RouterGroup) {
	iam := router.Group("/iam")
	iam.GET("/permissions", c.routes.Handle("iam.permission", "read", c.listPermissions))
	iam.GET("/user-groups", c.routes.Handle("iam.user_group", "read", c.listUserGroups))
	iam.POST("/user-groups", c.routes.Handle("iam.user_group", "create", c.createUserGroup))
	iam.GET("/permission-sets", c.routes.Handle("iam.permission_set", "read", c.listPermissionSets))
	iam.POST("/permission-sets", c.routes.Handle("iam.permission_set", "create", c.createPermissionSet))
	iam.GET("/permission-set-assignments", c.routes.Handle("iam.permission_set_assignment", "read", c.listPermissionSetAssignments))
	iam.POST("/permission-set-assignments", c.routes.Handle("iam.permission_set_assignment", "create", c.createPermissionSetAssignment))
	iam.GET("/data-scopes", c.routes.Handle("iam.data_scope", "read", c.listDataScopes))
	iam.POST("/data-scopes", c.routes.Handle("iam.data_scope", "create", c.createDataScope))
	iam.GET("/field-policies", c.routes.Handle("iam.field_policy", "read", c.listFieldPolicies))
	iam.POST("/field-policies", c.routes.Handle("iam.field_policy", "create", c.createFieldPolicy))
	iam.GET("/assumable-roles", c.routes.Handle("iam.assumable_role", "read", c.listAssumableRoles))
	iam.POST("/assumable-roles", c.routes.Handle("iam.assumable_role", "create", c.createAssumableRole))
	iam.POST("/assumable-roles/:id/assume", c.routes.Handle("iam.assumable_role", "assume", c.assumeRole, ResourceID(PathParamID)))
}

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

func (c IAMCtrl) listPermissionSetAssignments(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListPermissionSetAssignmentPage(ctx, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

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
