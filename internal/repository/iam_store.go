package repository

import (
	"context"
	"time"

	"nexus-pro-be/internal/domain"
)

// IAMStore 定義 IAM 儲存層的行為契約。
type IAMStore interface {
	UpsertUserGroup(context.Context, domain.UserGroup) error
	GetUserGroup(ctx context.Context, tenantID, id string) (domain.UserGroup, bool, error)
	ListUserGroups(ctx context.Context, tenantID string) ([]domain.UserGroup, error)
	DeleteUserGroup(ctx context.Context, tenantID, id string) (domain.UserGroup, bool, error)
	UpsertGroupMembership(context.Context, domain.GroupMembership) error
	DeleteGroupMembership(ctx context.Context, tenantID, userGroupID, accountID string) (domain.GroupMembership, bool, error)
	CloseGroupMembership(ctx context.Context, tenantID, userGroupID, accountID string, validUntil time.Time) (domain.GroupMembership, bool, error)
	GetGroupMembership(ctx context.Context, tenantID, userGroupID, accountID string) (domain.GroupMembership, bool, error)
	ListGroupMembershipsForGroup(ctx context.Context, tenantID, userGroupID string) ([]domain.GroupMembership, error)
	ListActiveGroupMembershipsForAccount(ctx context.Context, tenantID, accountID string, at time.Time) ([]domain.GroupMembership, error)

	UpsertPermissionSet(context.Context, domain.PermissionSet) error
	GetPermissionSet(ctx context.Context, tenantID, id string) (domain.PermissionSet, bool, error)
	ListPermissionSets(ctx context.Context, tenantID string) ([]domain.PermissionSet, error)
	DeletePermissionSet(ctx context.Context, tenantID, id string) (domain.PermissionSet, bool, error)
	ReplacePermissionSetItems(ctx context.Context, tenantID, permissionSetID string, items []domain.PermissionSetItem) error
	ListPermissionSetItemsForSet(ctx context.Context, tenantID, permissionSetID string) ([]domain.PermissionSetItem, error)

	UpsertPermissionCatalogItem(context.Context, domain.PermissionCatalogItem) error
	GetPermissionCatalogItemByKey(ctx context.Context, tenantID, application, resource, action string, permissionType domain.PermissionType) (domain.PermissionCatalogItem, bool, error)
	ListPermissionCatalogItems(ctx context.Context, tenantID string) ([]domain.PermissionCatalogItem, error)
	UpsertMenuItem(context.Context, domain.MenuItem) error
	ListMenuItems(ctx context.Context, tenantID string) ([]domain.MenuItem, error)

	UpsertPermissionPackage(context.Context, domain.PermissionPackage) error
	UpdatePermissionPackageStatus(ctx context.Context, id string, status domain.PermissionPackageStatus, publishedAt *time.Time) (domain.PermissionPackage, bool, error)
	GetPermissionPackage(ctx context.Context, id string) (domain.PermissionPackage, bool, error)
	GetPermissionPackageByApplicationVersion(ctx context.Context, applicationCode, version string) (domain.PermissionPackage, bool, error)
	ListPermissionPackages(context.Context) ([]domain.PermissionPackage, error)
	UpsertPermissionSetTemplate(context.Context, domain.PermissionSetTemplate) error
	ListPermissionSetTemplates(ctx context.Context, packageID string) ([]domain.PermissionSetTemplate, error)
	UpsertUserGroupTemplate(context.Context, domain.UserGroupTemplate) error
	ListUserGroupTemplates(ctx context.Context, packageID string) ([]domain.UserGroupTemplate, error)
	UpsertAssumableRoleTemplate(context.Context, domain.AssumableRoleTemplate) error
	ListAssumableRoleTemplates(ctx context.Context, packageID string) ([]domain.AssumableRoleTemplate, error)
	UpsertPermissionPackageImport(context.Context, domain.PermissionPackageImport) error
	GetPermissionPackageImport(ctx context.Context, tenantID, packageID, version string) (domain.PermissionPackageImport, bool, error)
	ListPermissionPackageImports(ctx context.Context, tenantID string) ([]domain.PermissionPackageImport, error)

	UpsertPermissionSetAssignment(context.Context, domain.PermissionSetAssignment) error
	DeletePermissionSetAssignment(ctx context.Context, tenantID, id string) (domain.PermissionSetAssignment, bool, error)
	ListPermissionSetAssignments(ctx context.Context, tenantID string) ([]domain.PermissionSetAssignment, error)
	ListPermissionSetAssignmentsForPrincipal(ctx context.Context, tenantID, principalType, principalID string) ([]domain.PermissionSetAssignment, error)

	UpsertDataScope(context.Context, domain.DataScope) error
	GetDataScope(ctx context.Context, tenantID, id string) (domain.DataScope, bool, error)
	ListDataScopes(ctx context.Context, tenantID string) ([]domain.DataScope, error)
	UpdateDataScope(context.Context, domain.DataScope) error
	DeleteDataScope(ctx context.Context, tenantID, id string) (domain.DataScope, bool, error)

	UpsertFieldPolicy(context.Context, domain.FieldPolicy) error
	GetFieldPolicy(ctx context.Context, tenantID, id string) (domain.FieldPolicy, bool, error)
	ListFieldPolicies(ctx context.Context, tenantID, applicationCode, resourceType string) ([]domain.FieldPolicy, error)
	DeleteFieldPolicy(ctx context.Context, tenantID, id string) (domain.FieldPolicy, bool, error)

	UpsertAssumableRole(context.Context, domain.AssumableRole) error
	GetAssumableRole(ctx context.Context, tenantID, id string) (domain.AssumableRole, bool, error)
	ListAssumableRoles(ctx context.Context, tenantID string) ([]domain.AssumableRole, error)
	DeleteAssumableRole(ctx context.Context, tenantID, id string) (domain.AssumableRole, bool, error)
	UpsertAssumableRoleSession(context.Context, domain.AssumableRoleSession) error
	GetActiveAssumableRoleSession(ctx context.Context, tenantID, id string) (domain.AssumableRoleSession, bool, error)
	ListActiveAssumableRoleSessionsForRole(ctx context.Context, tenantID, roleID string) ([]domain.AssumableRoleSession, error)
}
