package service

import "nexus-pro-be/internal/domain"

// Domain type aliases expose domain contracts through the service package for legacy callers.
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
	AssumedRoleDecision     = domain.AssumedRoleDecision
	OrgUnit                 = domain.OrgUnit
	Employee                = domain.Employee
	EmployeeExperience      = domain.EmployeeExperience
	EmployeeQuery           = domain.EmployeeQuery
	EmployeeStats           = domain.EmployeeStats
	EmployeeOptions         = domain.EmployeeOptions
	EmployeeImportSession   = domain.EmployeeImportSession
	EmployeeImportRow       = domain.EmployeeImportRow
	BatchEmployeeResponse   = domain.BatchEmployeeResponse
	BatchEmployeeResult     = domain.BatchEmployeeResult
	LeaveBalance            = domain.LeaveBalance
	LeaveRequest            = domain.LeaveRequest
	FormTemplate            = domain.FormTemplate
	FormInstance            = domain.FormInstance
	KnowledgeArticle        = domain.KnowledgeArticle
	Reference               = domain.Reference
	AgentRun                = domain.AgentRun
	AgentRunStatus          = domain.AgentRunStatus
	AuditLog                = domain.AuditLog
	OutboxEvent             = domain.OutboxEvent
	AuthzOutboxEvent        = domain.AuthzOutboxEvent
	PageRequest             = domain.PageRequest
	PageResponse[T any]     = domain.PageResponse[T]
	FieldError              = domain.FieldError
	RowError                = domain.RowError
	MenuNode                = domain.MenuNode
	RequestContext          = domain.RequestContext
	CheckRequest            = domain.CheckRequest
	CheckResult             = domain.CheckResult
	BatchCheckRequest       = domain.BatchCheckRequest
	BatchCheckResult        = domain.BatchCheckResult
	Effect                  = domain.Effect
	Severity                = domain.Severity
	PrincipalType           = domain.PrincipalType
	Scope                   = domain.Scope
	ApplicationCode         = domain.ApplicationCode
	ResourceType            = domain.ResourceType
	Action                  = domain.Action
	FieldPolicyEffect       = domain.FieldPolicyEffect
	AccountStatus           = domain.AccountStatus
	EventType               = domain.EventType
	EmployeeStatus          = domain.EmployeeStatus
	EmployeeCategory        = domain.EmployeeCategory
	EmployeeAccountPolicy   = domain.EmployeeAccountPolicy
	MeResponse              = domain.MeResponse
	MenuListResponse        = domain.MenuListResponse
	AssumeRoleResponse      = domain.AssumeRoleResponse
	AuthzExplainResponse    = domain.AuthzExplainResponse
	AuthzSimulationResponse = domain.AuthzSimulationResponse
)

// Domain input aliases keep service method signatures close to API payload contracts.
type (
	CreateUserGroupInput               = domain.CreateUserGroupInput
	CreatePermissionSetInput           = domain.CreatePermissionSetInput
	CreatePermissionSetAssignmentInput = domain.CreatePermissionSetAssignmentInput
	CreateFieldPolicyInput             = domain.CreateFieldPolicyInput
	CreateDataScopeInput               = domain.CreateDataScopeInput
	CreateAssumableRoleInput           = domain.CreateAssumableRoleInput
	AssumeRoleInput                    = domain.AssumeRoleInput
	CreateOrgUnitInput                 = domain.CreateOrgUnitInput
	CreateEmployeeInput                = domain.CreateEmployeeInput
	UpdateEmployeeInput                = domain.UpdateEmployeeInput
	EmployeePreviewResponse            = domain.EmployeePreviewResponse
	EmployeeAvatarInput                = domain.EmployeeAvatarInput
	EmployeeImportPreviewInput         = domain.EmployeeImportPreviewInput
	EmployeeImportConfirmInput         = domain.EmployeeImportConfirmInput
	BatchDeleteEmployeesInput          = domain.BatchDeleteEmployeesInput
	InviteEmployeeInput                = domain.InviteEmployeeInput
	UpdateEmployeeStatusInput          = domain.UpdateEmployeeStatusInput
	StatusTransitionInput              = domain.StatusTransitionInput
	CreateFormTemplateInput            = domain.CreateFormTemplateInput
	SubmitFormInput                    = domain.SubmitFormInput
	ApproveFormInput                   = domain.ApproveFormInput
	CreateLeaveRequestInput            = domain.CreateLeaveRequestInput
	CreateAgentRunInput                = domain.CreateAgentRunInput
)

// Domain constants expose shared pagination, IAM, HR, workflow, and agent values.
const (
	DefaultPage     = domain.DefaultPage
	DefaultPageSize = domain.DefaultPageSize
	MaxPageSize     = domain.MaxPageSize

	EffectAllow = domain.EffectAllow
	EffectDeny  = domain.EffectDeny

	SeverityMedium = domain.SeverityMedium
	SeverityHigh   = domain.SeverityHigh

	PrincipalTypeAccount       = domain.PrincipalTypeAccount
	PrincipalTypeUserGroup     = domain.PrincipalTypeUserGroup
	PrincipalTypeAssumableRole = domain.PrincipalTypeAssumableRole

	ScopeAll               = domain.ScopeAll
	ScopeSelf              = domain.ScopeSelf
	ScopeOwn               = domain.ScopeOwn
	ScopeTenant            = domain.ScopeTenant
	ScopeObject            = domain.ScopeObject
	ScopeDepartment        = domain.ScopeDepartment
	ScopeDepartmentSubtree = domain.ScopeDepartmentSubtree
	ScopeDirectReports     = domain.ScopeDirectReports
	ScopeAssignedOrgUnits  = domain.ScopeAssignedOrgUnits
	ScopeCustomCondition   = domain.ScopeCustomCondition
	ScopeSystem            = domain.ScopeSystem

	AppPlatform   = domain.AppPlatform
	AppHR         = domain.AppHR
	AppIAM        = domain.AppIAM
	AppAttendance = domain.AppAttendance
	AppAgent      = domain.AppAgent
	AppWorkflow   = domain.AppWorkflow
	AppAudit      = domain.AppAudit

	ActionRead             = domain.ActionRead
	ActionCreate           = domain.ActionCreate
	ActionUpdate           = domain.ActionUpdate
	ActionDelete           = domain.ActionDelete
	ActionExport           = domain.ActionExport
	ActionImport           = domain.ActionImport
	ActionAssume           = domain.ActionAssume
	ActionInvite           = domain.ActionInvite
	ActionSubmit           = domain.ActionSubmit
	ActionApprove          = domain.ActionApprove
	ActionCall             = domain.ActionCall
	ActionUpdateStatus     = domain.ActionUpdateStatus
	ActionStatusTransition = domain.ActionStatusTransition

	ResourceEmployee           = domain.ResourceEmployee
	ResourceEmployeeImport     = domain.ResourceEmployeeImport
	ResourceOrgUnit            = domain.ResourceOrgUnit
	ResourceLeave              = domain.ResourceLeave
	ResourceUserGroup          = domain.ResourceUserGroup
	ResourcePermissionSet      = domain.ResourcePermissionSet
	ResourcePermissionAssign   = domain.ResourcePermissionAssign
	ResourceDataScope          = domain.ResourceDataScope
	ResourceFieldPolicy        = domain.ResourceFieldPolicy
	ResourceAssumableRole      = domain.ResourceAssumableRole
	ResourceTool               = domain.ResourceTool
	ResourceKnowledgeArticle   = domain.ResourceKnowledgeArticle
	ResourceEmployeeCollection = domain.ResourceEmployeeCollection
	ResourceFormInstance       = domain.ResourceFormInstance

	AccountStatusActive        = domain.AccountStatusActive
	AccountStatusDisabled      = domain.AccountStatusDisabled
	AccountStatusPendingInvite = domain.AccountStatusPendingInvite

	EmployeeAccountPolicyNone                = domain.EmployeeAccountPolicyNone
	EmployeeAccountPolicyLinkExisting        = domain.EmployeeAccountPolicyLinkExisting
	EmployeeAccountPolicyCreatePendingInvite = domain.EmployeeAccountPolicyCreatePendingInvite
	EmployeeAccountPolicyCreateActive        = domain.EmployeeAccountPolicyCreateActive

	AgentRunStatusQueued    = domain.AgentRunStatusQueued
	AgentRunStatusRunning   = domain.AgentRunStatusRunning
	AgentRunStatusCompleted = domain.AgentRunStatusCompleted
	AgentRunStatusFailed    = domain.AgentRunStatusFailed

	EventEmployeeCreated            = domain.EventEmployeeCreated
	EventEmployeeUpdated            = domain.EventEmployeeUpdated
	EventEmployeeInvited            = domain.EventEmployeeInvited
	EventEmployeeImported           = domain.EventEmployeeImported
	EventEmployeeOffboarded         = domain.EventEmployeeOffboarded
	EventEmployeeReinstated         = domain.EventEmployeeReinstated
	EventEmployeeStatusChanged      = domain.EventEmployeeStatusChanged
	EventEmployeeAuthzSubjectCreate = domain.EventEmployeeAuthzSubjectCreate
	EventEmployeeAuthzSubjectUpdate = domain.EventEmployeeAuthzSubjectUpdate
	EventEmployeeAuthzSubjectInvite = domain.EventEmployeeAuthzSubjectInvite
	EventEmployeeAuthzSubjectImport = domain.EventEmployeeAuthzSubjectImport

	EmployeeStatusActive         = domain.EmployeeStatusActive
	EmployeeStatusProbation      = domain.EmployeeStatusProbation
	EmployeeStatusLeaveSuspended = domain.EmployeeStatusLeaveSuspended
	EmployeeStatusOnboarding     = domain.EmployeeStatusOnboarding
	EmployeeStatusResigned       = domain.EmployeeStatusResigned
	EmployeeStatusDeleted        = domain.EmployeeStatusDeleted
)

// Domain helper aliases preserve older service-package call sites during refactors.
var (
	NotFound                       = domain.NotFound
	BadRequest                     = domain.BadRequest
	Forbidden                      = domain.Forbidden
	Conflict                       = domain.Conflict
	ValidationFailed               = domain.ValidationFailed
	ImportValidationFailed         = domain.ImportValidationFailed
	AsAppError                     = domain.AsAppError
	NormalizeEmployeeStatus        = domain.NormalizeEmployeeStatus
	ParseEmployeeStatus            = domain.ParseEmployeeStatus
	NormalizeEmployeeCategory      = domain.NormalizeEmployeeCategory
	ParseEmployeeCategory          = domain.ParseEmployeeCategory
	NormalizeEmployeeAccountPolicy = domain.NormalizeEmployeeAccountPolicy
	ParseEmployeeAccountPolicy     = domain.ParseEmployeeAccountPolicy
	EmployeeStatuses               = domain.EmployeeStatuses
	EmployeeCategories             = domain.EmployeeCategories
)
