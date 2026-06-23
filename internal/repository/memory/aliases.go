package memory

import "nexus-pro-be/internal/domain"

// Domain aliases keep the in-memory store concise while preserving exact domain types.
type (
	Tenant                  = domain.Tenant
	Account                 = domain.Account
	UserGroup               = domain.UserGroup
	PermissionSet           = domain.PermissionSet
	Permission              = domain.Permission
	PermissionSetAssignment = domain.PermissionSetAssignment
	DataScope               = domain.DataScope
	FieldPolicy             = domain.FieldPolicy
	AssumableRole           = domain.AssumableRole
	AssumableRoleSession    = domain.AssumableRoleSession
	OrgUnit                 = domain.OrgUnit
	Employee                = domain.Employee
	EmployeeExperience      = domain.EmployeeExperience
	EmployeeQuery           = domain.EmployeeQuery
	EmployeeImportSession   = domain.EmployeeImportSession
	EmployeeImportRow       = domain.EmployeeImportRow
	LeaveBalance            = domain.LeaveBalance
	LeaveRequest            = domain.LeaveRequest
	FormTemplate            = domain.FormTemplate
	FormInstance            = domain.FormInstance
	KnowledgeArticle        = domain.KnowledgeArticle
	Reference               = domain.Reference
	AgentRun                = domain.AgentRun
	CheckResult             = domain.CheckResult
	RowError                = domain.RowError
	AuditLog                = domain.AuditLog
	AuthzOutboxEvent        = domain.AuthzOutboxEvent
	AuthzRelationshipTuple  = domain.AuthzRelationshipTuple
	PageRequest             = domain.PageRequest
)

// Pagination aliases mirror the domain defaults used by in-memory list helpers.
const (
	DefaultPage     = domain.DefaultPage
	DefaultPageSize = domain.DefaultPageSize
	MaxPageSize     = domain.MaxPageSize
)
