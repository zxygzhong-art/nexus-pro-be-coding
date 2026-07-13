package memory

import "nexus-pro-be/internal/domain"

// Domain 說明儲存層資料契約。
type (
	Tenant                          = domain.Tenant
	Account                         = domain.Account
	UserIdentity                    = domain.UserIdentity
	UserGroup                       = domain.UserGroup
	GroupMembership                 = domain.GroupMembership
	PermissionSet                   = domain.PermissionSet
	Permission                      = domain.Permission
	PermissionPackage               = domain.PermissionPackage
	PermissionPackageContent        = domain.PermissionPackageContent
	PermissionSetTemplate           = domain.PermissionSetTemplate
	UserGroupTemplate               = domain.UserGroupTemplate
	AssumableRoleTemplate           = domain.AssumableRoleTemplate
	PermissionPackageImport         = domain.PermissionPackageImport
	PermissionCatalogItem           = domain.PermissionCatalogItem
	MenuItem                        = domain.MenuItem
	PermissionSetItem               = domain.PermissionSetItem
	PermissionSetAssignment         = domain.PermissionSetAssignment
	DataScope                       = domain.DataScope
	FieldPolicy                     = domain.FieldPolicy
	AssumableRole                   = domain.AssumableRole
	AssumableRoleSession            = domain.AssumableRoleSession
	OrgUnit                         = domain.OrgUnit
	Position                        = domain.Position
	Employee                        = domain.Employee
	EmployeeExperience              = domain.EmployeeExperience
	EmployeeQuery                   = domain.EmployeeQuery
	EmployeeImportSession           = domain.EmployeeImportSession
	EmployeeImportRow               = domain.EmployeeImportRow
	EmploymentContract              = domain.EmploymentContract
	AttendancePolicy                = domain.AttendancePolicy
	AttendanceLeaveType             = domain.AttendanceLeaveType
	LeaveBalance                    = domain.LeaveBalance
	LeaveRequest                    = domain.LeaveRequest
	AttendanceWorksite              = domain.AttendanceWorksite
	AttendanceShift                 = domain.AttendanceShift
	AttendanceShiftAssignment       = domain.AttendanceShiftAssignment
	AttendanceClockRecord           = domain.AttendanceClockRecord
	AttendanceDailySummary          = domain.AttendanceDailySummary
	AttendanceCorrectionRequest     = domain.AttendanceCorrectionRequest
	OvertimeRequest                 = domain.OvertimeRequest
	FormTemplate                    = domain.FormTemplate
	FormTemplateVersion             = domain.FormTemplateVersion
	FormInstance                    = domain.FormInstance
	FormInstanceFieldValue          = domain.FormInstanceFieldValue
	PlatformTaskRecordItem          = domain.PlatformTaskRecordItem
	PlatformTaskTodoRecord          = domain.PlatformTaskTodoRecord
	Reference                       = domain.Reference
	AgentRun                        = domain.AgentRun
	AgentModel                      = domain.AgentModel
	AgentDefinition                 = domain.AgentDefinition
	AgentDefinitionVersion          = domain.AgentDefinitionVersion
	AgentAudit                      = domain.AgentAudit
	AgentSession                    = domain.AgentSession
	AgentSessionMessage             = domain.AgentSessionMessage
	AgentMemory                     = domain.AgentMemory
	Notification                    = domain.Notification
	NotificationRecipient           = domain.NotificationRecipient
	NotificationItem                = domain.NotificationItem
	NotificationToneCounts          = domain.NotificationToneCounts
	CheckResult                     = domain.CheckResult
	RowError                        = domain.RowError
	AuditLog                        = domain.AuditLog
	OutboxEvent                     = domain.OutboxEvent
	IdentityProvisioningOutboxEvent = domain.IdentityProvisioningOutboxEvent
	AuthzRelationshipTuple          = domain.AuthzRelationshipTuple
	PageRequest                     = domain.PageRequest
)

// 下列常數定義此模組使用的固定值。
const (
	DefaultPage     = domain.DefaultPage
	DefaultPageSize = domain.DefaultPageSize
	MaxPageSize     = domain.MaxPageSize
)
