package service

type IAMService struct {
	*Service
	store iamStore
}

func (c *Service) IAM() IAMService {
	return IAMService{Service: c, store: c.store}
}

func (c *Service) ListPermissions(ctx RequestContext) ([]Permission, error) {
	return c.IAM().ListPermissions(ctx)
}

func (c *Service) ListPermissionPage(ctx RequestContext, page PageRequest) (PageResponse[Permission], error) {
	return c.IAM().ListPermissionPage(ctx, page)
}

func (c *Service) ListUserGroups(ctx RequestContext) ([]UserGroup, error) {
	return c.IAM().ListUserGroups(ctx)
}

func (c *Service) ListUserGroupPage(ctx RequestContext, page PageRequest) (PageResponse[UserGroup], error) {
	return c.IAM().ListUserGroupPage(ctx, page)
}

func (c *Service) CreateUserGroup(ctx RequestContext, input CreateUserGroupInput) (UserGroup, error) {
	return c.IAM().CreateUserGroup(ctx, input)
}

func (c *Service) ListPermissionSets(ctx RequestContext) ([]PermissionSet, error) {
	return c.IAM().ListPermissionSets(ctx)
}

func (c *Service) ListPermissionSetPage(ctx RequestContext, page PageRequest) (PageResponse[PermissionSet], error) {
	return c.IAM().ListPermissionSetPage(ctx, page)
}

func (c *Service) CreatePermissionSet(ctx RequestContext, input CreatePermissionSetInput) (PermissionSet, error) {
	return c.IAM().CreatePermissionSet(ctx, input)
}

func (c *Service) ListPermissionSetAssignments(ctx RequestContext) ([]PermissionSetAssignment, error) {
	return c.IAM().ListPermissionSetAssignments(ctx)
}

func (c *Service) ListPermissionSetAssignmentPage(ctx RequestContext, page PageRequest) (PageResponse[PermissionSetAssignment], error) {
	return c.IAM().ListPermissionSetAssignmentPage(ctx, page)
}

func (c *Service) CreatePermissionSetAssignment(ctx RequestContext, input CreatePermissionSetAssignmentInput) (PermissionSetAssignment, error) {
	return c.IAM().CreatePermissionSetAssignment(ctx, input)
}

func (c *Service) ListDataScopes(ctx RequestContext) ([]DataScope, error) {
	return c.IAM().ListDataScopes(ctx)
}

func (c *Service) ListDataScopePage(ctx RequestContext, page PageRequest) (PageResponse[DataScope], error) {
	return c.IAM().ListDataScopePage(ctx, page)
}

func (c *Service) CreateDataScope(ctx RequestContext, input CreateDataScopeInput) (DataScope, error) {
	return c.IAM().CreateDataScope(ctx, input)
}

func (c *Service) ListFieldPolicies(ctx RequestContext, applicationCode, resourceType string) ([]FieldPolicy, error) {
	return c.IAM().ListFieldPolicies(ctx, applicationCode, resourceType)
}

func (c *Service) ListFieldPolicyPage(ctx RequestContext, applicationCode, resourceType string, page PageRequest) (PageResponse[FieldPolicy], error) {
	return c.IAM().ListFieldPolicyPage(ctx, applicationCode, resourceType, page)
}

func (c *Service) CreateFieldPolicy(ctx RequestContext, input CreateFieldPolicyInput) (FieldPolicy, error) {
	return c.IAM().CreateFieldPolicy(ctx, input)
}

func (c *Service) ListAssumableRoles(ctx RequestContext) ([]AssumableRole, error) {
	return c.IAM().ListAssumableRoles(ctx)
}

func (c *Service) ListAssumableRolePage(ctx RequestContext, page PageRequest) (PageResponse[AssumableRole], error) {
	return c.IAM().ListAssumableRolePage(ctx, page)
}

func (c *Service) CreateAssumableRole(ctx RequestContext, input CreateAssumableRoleInput) (AssumableRole, error) {
	return c.IAM().CreateAssumableRole(ctx, input)
}

func (c *Service) AssumeRole(ctx RequestContext, roleID string, input AssumeRoleInput) (AssumeRoleResponse, error) {
	return c.IAM().AssumeRole(ctx, roleID, input)
}
