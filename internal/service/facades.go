package service

import "context"

// AuthnFacade exposes public authentication entrypoints to the API layer.
type AuthnFacade interface {
	OIDCAuthorizationURL(string, OIDCAuthorizationInput) (OIDCAuthorizationResponse, error)
	CompleteOIDCCallback(context.Context, string, string, string) (AuthLoginResponse, error)
}

// IdentityFacade exposes external-principal to local-account mapping to the API layer.
type IdentityFacade interface {
	ResolveAuthenticatedPrincipal(context.Context, AuthenticatedPrincipal) (IdentityResolution, error)
	ResolveBoundAuthenticatedPrincipal(context.Context, AuthenticatedPrincipal) (IdentityResolution, error)
}

// MeFacade exposes current-user read operations to the API layer.
type MeFacade interface {
	Resolve(RequestContext) (MeResponse, error)
	ListMenus(RequestContext) ([]MenuNode, error)
}

// AuthzFacade exposes authorization checks to API routes and explicit authz endpoints.
type AuthzFacade interface {
	Check(RequestContext, CheckRequest) (CheckResult, error)
	BatchCheck(RequestContext, BatchCheckRequest) (BatchCheckResult, error)
	AuditDecision(RequestContext, CheckRequest, CheckResult) error
	ValidateApprovalInstance(RequestContext, CheckRequest) error
}

// IAMFacade exposes IAM management use cases to the API layer.
type IAMFacade interface {
	ListPermissionPage(RequestContext, PageRequest) (PageResponse[Permission], error)
	ListUserGroupPage(RequestContext, PageRequest) (PageResponse[UserGroup], error)
	CreateUserGroup(RequestContext, CreateUserGroupInput) (UserGroup, error)
	ListPermissionSetPage(RequestContext, PageRequest) (PageResponse[PermissionSet], error)
	CreatePermissionSet(RequestContext, CreatePermissionSetInput) (PermissionSet, error)
	ListPermissionSetAssignmentPage(RequestContext, PageRequest) (PageResponse[PermissionSetAssignment], error)
	CreatePermissionSetAssignment(RequestContext, CreatePermissionSetAssignmentInput) (PermissionSetAssignment, error)
	ListDataScopePage(RequestContext, PageRequest) (PageResponse[DataScope], error)
	CreateDataScope(RequestContext, CreateDataScopeInput) (DataScope, error)
	ListFieldPolicyPage(RequestContext, string, string, PageRequest) (PageResponse[FieldPolicy], error)
	CreateFieldPolicy(RequestContext, CreateFieldPolicyInput) (FieldPolicy, error)
	ListAssumableRolePage(RequestContext, PageRequest) (PageResponse[AssumableRole], error)
	CreateAssumableRole(RequestContext, CreateAssumableRoleInput) (AssumableRole, error)
	AssumeRole(RequestContext, string, AssumeRoleInput) (AssumeRoleResponse, error)
}

// HRFacade exposes people-domain and organization use cases to the API layer.
type HRFacade interface {
	QueryEmployees(RequestContext, EmployeeQuery) (PageResponse[Employee], error)
	CreateEmployee(RequestContext, CreateEmployeeInput) (Employee, error)
	PreviewCreateEmployee(RequestContext, CreateEmployeeInput) (EmployeePreviewResponse, error)
	GetEmployee(RequestContext, string) (Employee, error)
	UpdateEmployee(RequestContext, string, UpdateEmployeeInput) (Employee, error)
	PreviewUpdateEmployee(RequestContext, string, UpdateEmployeeInput) (EmployeePreviewResponse, error)
	UpdateEmployeeAvatar(RequestContext, string, EmployeeAvatarInput) (Employee, error)
	DeleteEmployeeAvatar(RequestContext, string) (Employee, error)
	EmployeeStats(RequestContext, EmployeeQuery) (EmployeeStats, error)
	EmployeeOptions(RequestContext) (EmployeeOptions, error)
	EmployeeImportTemplate(RequestContext, string) ([]byte, string, string, error)
	PreviewEmployeeImport(RequestContext, EmployeeImportPreviewInput) (EmployeeImportSession, error)
	ConfirmEmployeeImport(RequestContext, string, EmployeeImportConfirmInput) (EmployeeImportSession, error)
	ExportEmployeesCSV(RequestContext, EmployeeQuery) ([]byte, string, error)
	ExportEmployees(RequestContext, ...EmployeeQuery) ([]Employee, error)
	BatchDeleteEmployees(RequestContext, BatchDeleteEmployeesInput) (BatchEmployeeResponse, error)
	DeleteEmployee(RequestContext, string) (Employee, error)
	InviteEmployee(RequestContext, string, InviteEmployeeInput) (Employee, error)
	TransitionEmployeeStatus(RequestContext, string, StatusTransitionInput) (Employee, error)
	UpdateEmployeeStatus(RequestContext, string, string) (Employee, error)
	ListOrgUnitPage(RequestContext, PageRequest) (PageResponse[OrgUnit], error)
	CreateOrgUnit(RequestContext, CreateOrgUnitInput) (OrgUnit, error)
}

// AttendanceFacade exposes leave balance and leave request use cases.
type AttendanceFacade interface {
	ListLeaveBalancePage(RequestContext, PageRequest) (PageResponse[LeaveBalance], error)
	ListLeaveRequestPage(RequestContext, PageRequest) (PageResponse[LeaveRequest], error)
	CreateLeaveRequest(RequestContext, CreateLeaveRequestInput) (LeaveRequest, error)
	CurrentAttendancePolicy(RequestContext) (AttendancePolicyResponse, error)
	ListAttendanceWorksitePage(RequestContext, PageRequest) (PageResponse[AttendanceWorksite], error)
	CreateAttendanceWorksite(RequestContext, CreateAttendanceWorksiteInput) (AttendanceWorksite, error)
	UpdateAttendanceWorksite(RequestContext, UpdateAttendanceWorksiteInput) (AttendanceWorksite, error)
	ListAttendanceShiftPage(RequestContext, PageRequest) (PageResponse[AttendanceShift], error)
	CreateAttendanceShift(RequestContext, CreateAttendanceShiftInput) (AttendanceShift, error)
	UpdateAttendanceShift(RequestContext, UpdateAttendanceShiftInput) (AttendanceShift, error)
	ListAttendanceShiftAssignmentPage(RequestContext, PageRequest) (PageResponse[AttendanceShiftAssignment], error)
	CreateAttendanceShiftAssignment(RequestContext, CreateAttendanceShiftAssignmentInput) (AttendanceShiftAssignment, error)
	AttendanceClockStatus(RequestContext) (AttendanceClockStatus, error)
	CreateAttendanceClockRecord(RequestContext, CreateAttendanceClockRecordInput) (AttendanceClockRecord, error)
	ListAttendanceClockRecordPage(RequestContext, AttendanceClockRecordQuery, PageRequest) (PageResponse[AttendanceClockRecord], error)
	CreateAttendanceCorrection(RequestContext, CreateAttendanceCorrectionInput) (AttendanceCorrectionRequest, error)
	ListAttendanceCorrectionPage(RequestContext, AttendanceCorrectionQuery, PageRequest) (PageResponse[AttendanceCorrectionRequest], error)
	ApproveAttendanceCorrection(RequestContext, string, ReviewAttendanceCorrectionInput) (AttendanceCorrectionRequest, error)
	RejectAttendanceCorrection(RequestContext, string, ReviewAttendanceCorrectionInput) (AttendanceCorrectionRequest, error)
}

// WorkspaceFacade exposes workspace dashboard aggregates to the API layer.
type WorkspaceFacade interface {
	WorkspaceOverview(RequestContext, WorkspaceOverviewQuery) (WorkspaceOverviewResponse, error)
	WorkspaceOrganization(RequestContext) (WorkspaceOrganizationResponse, error)
	WorkspaceTurnover(RequestContext, WorkspaceTurnoverQuery) (WorkspaceTurnoverResponse, error)
	WorkspaceAttendance(RequestContext, WorkspaceAttendanceQuery) (WorkspaceAttendanceResponse, error)
	WorkspaceAdmins(RequestContext) (WorkspaceAdminsResponse, error)
	WorkspaceAuditLogs(RequestContext, WorkspaceAuditLogQuery, PageRequest) (PageResponse[WorkspaceAuditLog], error)
}

// WorkflowFacade exposes form template and form instance use cases.
type WorkflowFacade interface {
	ListFormTemplatePage(RequestContext, PageRequest) (PageResponse[FormTemplate], error)
	CreateFormTemplate(RequestContext, CreateFormTemplateInput) (FormTemplate, error)
	SubmitForm(RequestContext, SubmitFormInput) (FormInstance, error)
	ApproveForm(RequestContext, string, ApproveFormInput) (FormInstance, error)
}

// AgentFacade exposes agent run use cases.
type AgentFacade interface {
	ListRunPage(RequestContext, PageRequest) (PageResponse[AgentRun], error)
	CreateRun(RequestContext, CreateAgentRunInput) (AgentRun, error)
}

// AuditFacade exposes audit log queries.
type AuditFacade interface {
	ListLogPage(RequestContext, PageRequest) (PageResponse[AuditLog], error)
}

var (
	_ AuthnFacade      = AuthnService{}
	_ IdentityFacade   = IdentityService{}
	_ MeFacade         = MeService{}
	_ AuthzFacade      = AuthzService{}
	_ IAMFacade        = IAMService{}
	_ HRFacade         = HRService{}
	_ AttendanceFacade = AttendanceService{}
	_ WorkspaceFacade  = WorkspaceService{}
	_ WorkflowFacade   = WorkflowService{}
	_ AgentFacade      = AgentService{}
	_ AuditFacade      = AuditService{}
)
