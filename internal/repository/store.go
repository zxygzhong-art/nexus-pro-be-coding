package repository

import (
	"context"
	"time"

	"nexus-pro-be/internal/domain"
)

type TenantStore interface {
	UpsertTenant(context.Context, domain.Tenant) error
	GetTenant(context.Context, string) (domain.Tenant, bool, error)
	ListTenants(context.Context) ([]domain.Tenant, error)
}

type AccountStore interface {
	UpsertAccount(context.Context, domain.Account) error
	GetAccount(ctx context.Context, tenantID, id string) (domain.Account, bool, error)
	ListAccounts(ctx context.Context, tenantID string) ([]domain.Account, error)
	RemoveAccountGroup(ctx context.Context, tenantID, accountID, groupID string) error
	AddAccountGroup(ctx context.Context, tenantID, accountID, groupID string) error
}

type IAMStore interface {
	UpsertUserGroup(context.Context, domain.UserGroup) error
	GetUserGroup(ctx context.Context, tenantID, id string) (domain.UserGroup, bool, error)
	ListUserGroups(ctx context.Context, tenantID string) ([]domain.UserGroup, error)

	UpsertPermissionSet(context.Context, domain.PermissionSet) error
	GetPermissionSet(ctx context.Context, tenantID, id string) (domain.PermissionSet, bool, error)
	ListPermissionSets(ctx context.Context, tenantID string) ([]domain.PermissionSet, error)

	UpsertPermissionSetAssignment(context.Context, domain.PermissionSetAssignment) error
	ListPermissionSetAssignments(ctx context.Context, tenantID string) ([]domain.PermissionSetAssignment, error)
	ListPermissionSetAssignmentsForPrincipal(ctx context.Context, tenantID, principalType, principalID string) ([]domain.PermissionSetAssignment, error)

	UpsertDataScope(context.Context, domain.DataScope) error
	GetDataScope(ctx context.Context, tenantID, id string) (domain.DataScope, bool, error)
	GetDataScopeByCode(ctx context.Context, tenantID, code string) (domain.DataScope, bool, error)
	ListDataScopes(ctx context.Context, tenantID string) ([]domain.DataScope, error)

	UpsertFieldPolicy(context.Context, domain.FieldPolicy) error
	ListFieldPolicies(ctx context.Context, tenantID, applicationCode, resourceType string) ([]domain.FieldPolicy, error)

	UpsertAssumableRole(context.Context, domain.AssumableRole) error
	GetAssumableRole(ctx context.Context, tenantID, id string) (domain.AssumableRole, bool, error)
	ListAssumableRoles(ctx context.Context, tenantID string) ([]domain.AssumableRole, error)
	UpsertAssumableRoleSession(context.Context, domain.AssumableRoleSession) error
	GetActiveAssumableRoleSession(ctx context.Context, tenantID, id string) (domain.AssumableRoleSession, bool, error)
}

type OrgStore interface {
	UpsertOrgUnit(context.Context, domain.OrgUnit) error
	GetOrgUnit(ctx context.Context, tenantID, id string) (domain.OrgUnit, bool, error)
	ListOrgUnits(ctx context.Context, tenantID string) ([]domain.OrgUnit, error)
}

type EmployeeStore interface {
	UpsertEmployee(ctx context.Context, employee domain.Employee) error
	GetEmployee(ctx context.Context, tenantID, id string) (domain.Employee, bool, error)
	ListEmployees(ctx context.Context, tenantID string) ([]domain.Employee, error)
	ListEmployeesByQuery(ctx context.Context, tenantID string, query domain.EmployeeQuery) ([]domain.Employee, error)
	ListEmployeePageByQuery(ctx context.Context, tenantID string, query domain.EmployeeQuery) ([]domain.Employee, int, error)
	CountEmployeesByQuery(ctx context.Context, tenantID string, query domain.EmployeeQuery) (int, error)
	NextEmployeeNo(ctx context.Context, tenantID, prefix string) (string, error)
	UpsertEmployeeImportSession(ctx context.Context, session domain.EmployeeImportSession) error
	GetEmployeeImportSession(ctx context.Context, tenantID, id string) (domain.EmployeeImportSession, bool, error)
}

type AttendanceStore interface {
	UpsertLeaveBalance(context.Context, domain.LeaveBalance) error
	GetLeaveBalance(ctx context.Context, tenantID, id string) (domain.LeaveBalance, bool, error)
	ListLeaveBalances(ctx context.Context, tenantID string) ([]domain.LeaveBalance, error)
	ReserveLeaveBalance(ctx context.Context, tenantID, employeeID, leaveType string, hours float64, updatedAt time.Time) (domain.LeaveBalance, bool, bool, error)

	UpsertLeaveRequest(context.Context, domain.LeaveRequest) error
	GetLeaveRequest(ctx context.Context, tenantID, id string) (domain.LeaveRequest, bool, error)
	ListLeaveRequests(ctx context.Context, tenantID string) ([]domain.LeaveRequest, error)
}

type WorkflowStore interface {
	UpsertFormTemplate(context.Context, domain.FormTemplate) error
	GetFormTemplate(ctx context.Context, tenantID, id string) (domain.FormTemplate, bool, error)
	GetFormTemplateByKey(ctx context.Context, tenantID, key string) (domain.FormTemplate, bool, error)
	ListFormTemplates(ctx context.Context, tenantID string) ([]domain.FormTemplate, error)

	UpsertFormInstance(context.Context, domain.FormInstance) error
	GetFormInstance(ctx context.Context, tenantID, id string) (domain.FormInstance, bool, error)
	ListFormInstances(ctx context.Context, tenantID string) ([]domain.FormInstance, error)
}

type KnowledgeStore interface {
	UpsertKnowledgeArticle(context.Context, domain.KnowledgeArticle) error
	ListKnowledgeArticles(ctx context.Context, tenantID string) ([]domain.KnowledgeArticle, error)
}

type AgentStore interface {
	UpsertAgentRun(context.Context, domain.AgentRun) error
	GetAgentRun(ctx context.Context, tenantID, id string) (domain.AgentRun, bool, error)
	ListAgentRuns(ctx context.Context, tenantID string) ([]domain.AgentRun, error)
	ListAgentRunPage(ctx context.Context, tenantID string, page domain.PageRequest) ([]domain.AgentRun, int, error)
}

type AuditStore interface {
	AppendAuditLog(context.Context, domain.AuditLog) error
	ListAuditLogs(ctx context.Context, tenantID string) ([]domain.AuditLog, error)
	ListAuditLogPage(ctx context.Context, tenantID string, page domain.PageRequest) ([]domain.AuditLog, int, error)
}

type AuthzEventStore interface {
	GetPermissionVersion(ctx context.Context, tenantID string) (int64, error)
	IncrementPermissionVersion(ctx context.Context, tenantID string) (int64, error)
	AppendAuthzOutboxEvent(context.Context, domain.AuthzOutboxEvent) error
	ListAuthzOutboxEvents(ctx context.Context, tenantID string) ([]domain.AuthzOutboxEvent, error)
}

type Store interface {
	TenantStore
	AccountStore
	IAMStore
	OrgStore
	EmployeeStore
	AttendanceStore
	WorkflowStore
	KnowledgeStore
	AgentStore
	AuditStore
	AuthzEventStore
}

type TenantTransactor interface {
	WithTenantTransaction(ctx context.Context, tenantID string, fn func(Store) error) error
}

func WithinTenantTransaction(ctx context.Context, store Store, tenantID string, fn func(Store) error) error {
	if tx, ok := store.(TenantTransactor); ok {
		return tx.WithTenantTransaction(ctx, tenantID, fn)
	}
	return fn(store)
}
