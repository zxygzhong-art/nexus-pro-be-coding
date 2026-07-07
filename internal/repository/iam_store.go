package repository

import (
	"context"

	"nexus-pro-be/internal/domain"
)

// IAMStore 定義 IAM 儲存層的行為契約。
type IAMStore interface {
	UpsertUserGroup(context.Context, domain.UserGroup) error
	GetUserGroup(ctx context.Context, tenantID, id string) (domain.UserGroup, bool, error)
	ListUserGroups(ctx context.Context, tenantID string) ([]domain.UserGroup, error)

	UpsertPermissionSet(context.Context, domain.PermissionSet) error
	GetPermissionSet(ctx context.Context, tenantID, id string) (domain.PermissionSet, bool, error)
	ListPermissionSets(ctx context.Context, tenantID string) ([]domain.PermissionSet, error)
	ReplacePermissionSetItems(ctx context.Context, tenantID, permissionSetID string, items []domain.PermissionSetItem) error
	ListPermissionSetItemsForSet(ctx context.Context, tenantID, permissionSetID string) ([]domain.PermissionSetItem, error)

	UpsertPermissionCatalogItem(context.Context, domain.PermissionCatalogItem) error
	GetPermissionCatalogItemByKey(ctx context.Context, tenantID, application, resource, action string, permissionType domain.PermissionType) (domain.PermissionCatalogItem, bool, error)
	ListPermissionCatalogItems(ctx context.Context, tenantID string) ([]domain.PermissionCatalogItem, error)
	UpsertMenuItem(context.Context, domain.MenuItem) error
	ListMenuItems(ctx context.Context, tenantID string) ([]domain.MenuItem, error)

	UpsertPermissionSetAssignment(context.Context, domain.PermissionSetAssignment) error
	ListPermissionSetAssignments(ctx context.Context, tenantID string) ([]domain.PermissionSetAssignment, error)
	ListPermissionSetAssignmentsForPrincipal(ctx context.Context, tenantID, principalType, principalID string) ([]domain.PermissionSetAssignment, error)

	UpsertDataScope(context.Context, domain.DataScope) error
	GetDataScope(ctx context.Context, tenantID, id string) (domain.DataScope, bool, error)
	ListDataScopes(ctx context.Context, tenantID string) ([]domain.DataScope, error)

	UpsertFieldPolicy(context.Context, domain.FieldPolicy) error
	ListFieldPolicies(ctx context.Context, tenantID, applicationCode, resourceType string) ([]domain.FieldPolicy, error)

	UpsertAssumableRole(context.Context, domain.AssumableRole) error
	GetAssumableRole(ctx context.Context, tenantID, id string) (domain.AssumableRole, bool, error)
	ListAssumableRoles(ctx context.Context, tenantID string) ([]domain.AssumableRole, error)
	UpsertAssumableRoleSession(context.Context, domain.AssumableRoleSession) error
	GetActiveAssumableRoleSession(ctx context.Context, tenantID, id string) (domain.AssumableRoleSession, bool, error)
}
