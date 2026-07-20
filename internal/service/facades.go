package service

import (
	"context"

	"nexus-pro-api/internal/domain"
)

// IdentityFacade 定義身分 facade 的行為契約。
type IdentityFacade interface {
	ResolveAuthenticatedPrincipal(context.Context, AuthenticatedPrincipal) (IdentityResolution, error)
	ResolveBoundAuthenticatedPrincipal(context.Context, AuthenticatedPrincipal) (IdentityResolution, error)
	VerifyGoogleSSOLogin(context.Context, AuthenticatedPrincipal) (domain.SSOLoginVerification, error)
}

// MeFacade 定義 me facade 的行為契約。
type MeFacade interface {
	Resolve(RequestContext) (MeResponse, error)
	UpdateProfile(RequestContext, UpdateMeProfileInput) (MeResponse, error)
	UpdatePreferences(RequestContext, UpdateMePreferencesInput) (MeResponse, error)
	ChangePassword(RequestContext, ChangePasswordInput) error
	ListMenus(RequestContext) ([]MenuNode, error)
}

// AuthzFacade 定義授權 facade 的行為契約。
type AuthzFacade interface {
	Check(RequestContext, CheckRequest) (CheckResult, error)
	CheckCurrentAccessProjection(RequestContext, CheckRequest) (CheckResult, error)
	BatchCheck(RequestContext, BatchCheckRequest) (BatchCheckResult, error)
	Explain(RequestContext, CheckRequest) (AuthzExplainResponse, error)
	Simulate(RequestContext, AuthzSimulationRequest) (AuthzSimulationResponse, error)
	AuditDecision(RequestContext, CheckRequest, CheckResult) error
}

// IAMFacade 定義 IAM facade 的行為契約。
type IAMFacade interface {
	ListApplications(RequestContext) ([]IAMApplication, error)
	ListResourceTypes(RequestContext) ([]IAMResourceType, error)
	ListPermissionPage(RequestContext, PageRequest) (PageResponse[PermissionCatalogItem], error)
	ListPermissionPackagePage(RequestContext, PageRequest) (PageResponse[PermissionPackage], error)
	RegisterPermissionPackage(RequestContext, PermissionPackageContent) (PermissionPackage, error)
	PublishPermissionPackage(RequestContext, string) (PermissionPackage, error)
	ImportPermissionPackage(RequestContext, string) (PermissionPackageImportResult, error)
	ListRolePage(RequestContext, PageRequest) (PageResponse[IAMRoleProjection], error)
	ListRoleBindingPage(RequestContext, PageRequest) (PageResponse[IAMRoleBindingProjection], error)
	ListUserGroupPage(RequestContext, PageRequest) (PageResponse[UserGroup], error)
	CreateUserGroup(RequestContext, CreateUserGroupInput) (UserGroup, error)
	UpdateUserGroup(RequestContext, string, UpdateUserGroupInput) (UserGroup, error)
	DeleteUserGroup(RequestContext, string) (UserGroup, error)
	ListUserGroupMemberPage(RequestContext, string, PageRequest) (PageResponse[GroupMembership], error)
	AddUserGroupMember(RequestContext, string, AddUserGroupMemberInput) (GroupMembership, error)
	RemoveUserGroupMember(RequestContext, string, string) error
	ListPermissionSetPage(RequestContext, PageRequest) (PageResponse[PermissionSet], error)
	CreatePermissionSet(RequestContext, CreatePermissionSetInput) (PermissionSet, error)
	UpdatePermissionSet(RequestContext, string, UpdatePermissionSetInput) (PermissionSet, error)
	DeletePermissionSet(RequestContext, string) (PermissionSet, error)
	ListIamAccountPage(RequestContext, string, PageRequest) (PageResponse[IamAccountProjection], error)
	ListPermissionSetAssignmentPage(RequestContext, PermissionSetAssignmentQuery, PageRequest) (PageResponse[PermissionSetAssignment], error)
	CreatePermissionSetAssignment(RequestContext, CreatePermissionSetAssignmentInput) (PermissionSetAssignment, error)
	DeletePermissionSetAssignment(RequestContext, string) (PermissionSetAssignment, error)
	ListDataScopePage(RequestContext, PageRequest) (PageResponse[DataScope], error)
	CreateDataScope(RequestContext, CreateDataScopeInput) (DataScope, error)
	UpdateDataScope(RequestContext, string, UpdateDataScopeInput) (DataScope, error)
	DeleteDataScope(RequestContext, string) (DataScope, error)
	ListFieldPolicyPage(RequestContext, string, string, PageRequest) (PageResponse[FieldPolicy], error)
	CreateFieldPolicy(RequestContext, CreateFieldPolicyInput) (FieldPolicy, error)
	UpdateFieldPolicy(RequestContext, string, UpdateFieldPolicyInput) (FieldPolicy, error)
	DeleteFieldPolicy(RequestContext, string) (FieldPolicy, error)
	ListOutboxEventPage(RequestContext, OutboxEventQuery, PageRequest) (PageResponse[OutboxEvent], error)
	RetryOutboxEvent(RequestContext, string) (OutboxEvent, error)
	ListAssumableRolePage(RequestContext, PageRequest) (PageResponse[AssumableRole], error)
	CreateAssumableRole(RequestContext, CreateAssumableRoleInput) (AssumableRole, error)
	UpdateAssumableRole(RequestContext, string, UpdateAssumableRoleInput) (AssumableRole, error)
	DeleteAssumableRole(RequestContext, string) (AssumableRole, error)
	AssumeRole(RequestContext, string, AssumeRoleInput) (AssumeRoleResponse, error)
	RevokeCurrentAssumableRoleSession(RequestContext) error
}

// HRFacade 定義 HR facade 的行為契約。
type HRFacade interface {
	ListPositionPage(RequestContext, PageRequest) (PageResponse[Position], error)
	CreatePosition(RequestContext, CreatePositionInput) (Position, error)
	GetPosition(RequestContext, string) (Position, error)
	UpdatePosition(RequestContext, string, UpdatePositionInput) (Position, error)
	DeletePosition(RequestContext, string) (Position, error)
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
	SyncEHRMSEmployees(RequestContext, EHRMSEmployeeSyncInput) (EHRMSEmployeeSyncResponse, error)
	SyncEHRMSOrgUnits(RequestContext) (EHRMSOrgUnitSyncResponse, error)
	SyncEHRMSPositions(RequestContext) (EHRMSPositionSyncResponse, error)
	ExportEmployeesCSV(RequestContext, EmployeeQuery) ([]byte, string, error)
	ExportEmployees(RequestContext, ...EmployeeQuery) ([]Employee, error)
	BatchDeleteEmployees(RequestContext, BatchDeleteEmployeesInput) (BatchEmployeeResponse, error)
	DeleteEmployee(RequestContext, string) (Employee, error)
	InviteEmployee(RequestContext, string, InviteEmployeeInput) (Employee, error)
	TransitionEmployeeStatus(RequestContext, string, StatusTransitionInput) (Employee, error)
	UpdateEmployeeStatus(RequestContext, string, string) (Employee, error)
	ListEmploymentContractsByEmployee(RequestContext, string) ([]EmploymentContract, error)
	CreateEmploymentContract(RequestContext, string, CreateEmploymentContractInput) (EmploymentContract, error)
	GetEmploymentContract(RequestContext, string) (EmploymentContract, error)
	UpdateEmploymentContract(RequestContext, string, UpdateEmploymentContractInput) (EmploymentContract, error)
	DeleteEmploymentContract(RequestContext, string) (EmploymentContract, error)
	ListOrgUnitPage(RequestContext, PageRequest) (PageResponse[OrgUnit], error)
	CreateOrgUnit(RequestContext, CreateOrgUnitInput) (OrgUnit, error)
	UpdateOrgUnit(RequestContext, string, UpdateOrgUnitInput) (OrgUnit, error)
}

// AttendanceFacade 定義考勤 facade 的行為契約。
type AttendanceFacade interface {
	ListLeaveBalancePage(RequestContext, PageRequest) (PageResponse[LeaveBalance], error)
	ListLeaveBalancePageByQuery(RequestContext, LeaveBalanceQuery, PageRequest) (PageResponse[LeaveBalance], error)
	ListLeaveRequestPage(RequestContext, PageRequest) (PageResponse[LeaveRequest], error)
	ListLeaveRequestPageByQuery(RequestContext, LeaveRequestQuery, PageRequest) (PageResponse[LeaveRequest], error)
	EvaluateLeaveRequest(RequestContext, EvaluateLeaveRequestInput) (LeaveRequestEvaluation, error)
	CreateLeaveRequest(RequestContext, CreateLeaveRequestInput) (LeaveRequest, error)
	ListOvertimeRequestPage(RequestContext, PageRequest) (PageResponse[OvertimeRequest], error)
	CreateOvertimeRequest(RequestContext, CreateOvertimeRequestInput) (OvertimeRequest, error)
	CurrentAttendancePolicy(RequestContext) (AttendancePolicyResponse, error)
	ValidateAttendancePolicy(RequestContext, UpdateAttendancePolicyInput) (AttendancePolicyValidationResult, error)
	PublishAttendancePolicy(RequestContext, UpdateAttendancePolicyInput) (AttendancePolicyResponse, error)
	UpdateAttendancePolicy(RequestContext, UpdateAttendancePolicyInput) (AttendancePolicyResponse, error)
	GrantLeaveBalances(RequestContext, GrantLeaveBalancesInput) (GrantLeaveBalancesResult, error)
	ListLeaveTypeIntegrations(RequestContext) (LeaveTypeIntegrationResponse, error)
	SaveLeaveTypeExternalMapping(RequestContext, SaveLeaveTypeExternalMappingInput) (LeaveTypeExternalMapping, error)
	ExpireLeaveTypeExternalMapping(RequestContext, string) (LeaveTypeExternalMapping, error)
	ListAttendanceWorksitePage(RequestContext, PageRequest) (PageResponse[AttendanceWorksite], error)
	CreateAttendanceWorksite(RequestContext, CreateAttendanceWorksiteInput) (AttendanceWorksite, error)
	UpdateAttendanceWorksite(RequestContext, UpdateAttendanceWorksiteInput) (AttendanceWorksite, error)
	ListAttendanceShiftPage(RequestContext, PageRequest) (PageResponse[AttendanceShift], error)
	CreateAttendanceShift(RequestContext, CreateAttendanceShiftInput) (AttendanceShift, error)
	UpdateAttendanceShift(RequestContext, UpdateAttendanceShiftInput) (AttendanceShift, error)
	ListAttendanceShiftAssignmentPage(RequestContext, PageRequest) (PageResponse[AttendanceShiftAssignment], error)
	CreateAttendanceShiftAssignment(RequestContext, CreateAttendanceShiftAssignmentInput) (AttendanceShiftAssignment, error)
	AttendanceClockStatus(RequestContext) (AttendanceClockStatus, error)
	AttendanceMonthlySummary(RequestContext, string) (AttendanceMonthlySummary, error)
	CreateAttendanceClockRecord(RequestContext, CreateAttendanceClockRecordInput) (AttendanceClockRecord, error)
	ListAttendanceClockRecordPage(RequestContext, AttendanceClockRecordQuery, PageRequest) (PageResponse[AttendanceClockRecord], error)
	SyncEHRMSAttendance(RequestContext, EHRMSAttendanceSyncInput) (EHRMSAttendanceSyncResponse, error)
	CreateAttendanceCorrection(RequestContext, CreateAttendanceCorrectionInput) (AttendanceCorrectionRequest, error)
	ListAttendanceCorrectionPage(RequestContext, AttendanceCorrectionQuery, PageRequest) (PageResponse[AttendanceCorrectionRequest], error)
	ApproveAttendanceCorrection(RequestContext, string, ReviewAttendanceCorrectionInput) (AttendanceCorrectionRequest, error)
	RejectAttendanceCorrection(RequestContext, string, ReviewAttendanceCorrectionInput) (AttendanceCorrectionRequest, error)
}

// PlatformFacade 定義平臺 facade 的行為契約。
type PlatformFacade interface {
	Home(RequestContext) (PlatformHomeResponse, error)
	ListAssistants(RequestContext, PlatformAssistantsQuery) (PlatformAssistantsResponse, error)
	Forms(RequestContext) (PlatformFormsResponse, error)
	Tasks(RequestContext) (PlatformTasksResponse, error)
	CreateTaskItem(RequestContext, CreatePlatformTaskItemInput) (PlatformTaskItem, error)
	UpdateTaskItem(RequestContext, string, UpdatePlatformTaskItemInput) (PlatformTaskItem, error)
	DeleteTaskItem(RequestContext, string) (PlatformTaskItem, error)
	CreateTaskTodo(RequestContext, CreatePlatformTaskTodoInput) (PlatformTaskTodo, error)
	UpdateTaskTodo(RequestContext, string, UpdatePlatformTaskTodoInput) (PlatformTaskTodo, error)
	DeleteTaskTodo(RequestContext, string) (PlatformTaskTodo, error)
	ConvertTaskTodo(RequestContext, string, ConvertPlatformTaskTodoInput) (PlatformTaskItem, error)
}

// WorkspaceFacade 定義工作區 facade 的行為契約。
type WorkspaceFacade interface {
	Workspace(RequestContext) (PlatformWorkspaceResponse, error)
	WorkspaceOverview(RequestContext, WorkspaceOverviewQuery) (WorkspaceOverviewResponse, error)
	WorkspaceEmployees(RequestContext, PlatformWorkspaceEmployeesQuery) (PlatformWorkspaceEmployeesResponse, error)
	WorkspaceOrganization(RequestContext) (WorkspaceOrganizationResponse, error)
	UpdateWorkspaceOrganizationManager(RequestContext, string, UpdateWorkspaceOrganizationManagerInput) (WorkspaceOrganizationResponse, error)
	UpdateWorkspaceOrganizationVisibility(RequestContext, string, UpdateWorkspaceOrganizationVisibilityInput) (WorkspaceOrganizationResponse, error)
	WorkspaceTurnover(RequestContext, WorkspaceTurnoverQuery) (WorkspaceTurnoverResponse, error)
	ExportWorkspaceTurnoverCSV(RequestContext, WorkspaceTurnoverQuery, string) ([]byte, string, error)
	WorkspaceAttendance(RequestContext, WorkspaceAttendanceQuery) (WorkspaceAttendanceResponse, error)
	ExportWorkspaceAttendanceCSV(RequestContext, WorkspaceAttendanceQuery, string) ([]byte, string, error)
	WorkspaceFormDesign(RequestContext) (PlatformFormDesign, error)
	CreateWorkspaceFormDesign(RequestContext, SaveWorkspaceFormDesignInput) (PlatformFormDesign, error)
	UpdateWorkspaceFormDesign(RequestContext, string, UpdateWorkspaceFormDesignInput) (PlatformFormDesign, error)
	DeleteWorkspaceFormDesign(RequestContext, string) (PlatformFormDesign, error)
	WorkspaceAuditLogs(RequestContext, WorkspaceAuditLogQuery, PageRequest) (PageResponse[WorkspaceAuditLog], error)
	WorkspaceAuditLogFacets(RequestContext) (WorkspaceAuditLogFacets, error)
	Insights(RequestContext, PlatformInsightsQuery) (PlatformInsightsResponse, error)
}

// WorkflowFacade 定義流程 facade 的行為契約。
type WorkflowFacade interface {
	FormDataSources(RequestContext) (FormDataSourceCatalogResponse, error)
	GetRuntimeFormTemplate(RequestContext, string, string) (domain.RuntimeFormTemplate, error)
	FormBuilderCapabilities(RequestContext) (domain.FormBuilderCapabilitiesResponse, error)
	ListFormDefinitionDrafts(RequestContext, string, string) ([]domain.FormDefinitionDraft, error)
	GetFormDefinitionDraft(RequestContext, string) (domain.FormDefinitionDraft, error)
	CreateFormDefinitionDraft(RequestContext, domain.CreateFormDefinitionDraftInput) (domain.FormDefinitionDraft, error)
	UpdateFormDefinitionDraft(RequestContext, string, domain.UpdateFormDefinitionDraftInput) (domain.FormDefinitionDraft, error)
	ValidateFormDefinitionDraft(RequestContext, string) (domain.FormDefinitionPreview, error)
	PreviewFormDefinitionDraft(RequestContext, string) (domain.FormDefinitionPreview, error)
	SimulateFormDefinitionWorkflow(RequestContext, string) (domain.FormWorkflowSimulation, error)
	SubmitFormDefinitionDraftForReview(RequestContext, string, int64) (domain.FormDefinitionDraft, error)
	PublishFormDefinitionDraft(RequestContext, string, int64) (domain.FormDefinitionDraft, error)
	ListFormTemplatePage(RequestContext, PageRequest) (PageResponse[FormTemplate], error)
	CreateFormTemplate(RequestContext, CreateFormTemplateInput) (FormTemplate, error)
	ListFormInstancePage(RequestContext, FormInstanceQuery, PageRequest) (PageResponse[FormInstance], error)
	GetFormInstanceDetail(RequestContext, string) (FormInstanceDetail, error)
	ReviewQueue(RequestContext) (WorkflowReviewQueueResponse, error)
	SaveFormDraft(RequestContext, SaveFormDraftInput) (FormInstance, error)
	UpdateFormDraft(RequestContext, string, UpdateFormDraftInput) (FormInstance, error)
	DeleteFormDraft(RequestContext, string) (FormInstance, error)
	SubmitForm(RequestContext, SubmitFormInput) (FormInstance, error)
	ApproveForm(RequestContext, string, ApproveFormInput) (FormInstance, error)
	RejectForm(RequestContext, string, RejectFormInput) (FormInstance, error)
	ReturnForm(RequestContext, string, ReturnFormInput) (FormInstance, error)
	CancelForm(RequestContext, string, CancelFormInput) (FormInstance, error)
	DuplicateForm(RequestContext, string) (FormInstance, error)
	ExportForm(RequestContext, string) (ExportedFormFile, error)
	BulkReviewForms(RequestContext, BulkReviewFormsInput) (BulkReviewFormsResponse, error)
	GetWorkflowFormState(RequestContext, string) (WorkflowFormStateResponse, error)
}

// AgentFacade 定義 agent facade 的行為契約。
type AgentFacade interface {
	ListRunPage(RequestContext, PageRequest) (PageResponse[AgentRun], error)
	CreateRun(RequestContext, CreateAgentRunInput) (AgentRun, error)
	Chat(RequestContext, domain.AgentChatInput, AgentChatEmitFunc) (AgentRun, error)
	ExecuteConfirmation(RequestContext, string, domain.ExecuteAgentConfirmationInput) (domain.AgentConfirmationExecution, error)
	ListSessions(RequestContext, domain.ListAgentSessionsQuery) ([]domain.AgentSession, error)
	CreateSession(RequestContext, domain.CreateAgentSessionInput) (domain.AgentSession, error)
	GetSession(RequestContext, string) (domain.AgentSession, error)
	UpdateSession(RequestContext, string, domain.UpdateAgentSessionInput) (domain.AgentSession, error)
	ClearSessionContext(RequestContext, string) (domain.AgentSession, error)
	DeleteSession(RequestContext, string) (domain.AgentSession, error)
	ListSessionMessages(RequestContext, string) ([]domain.AgentSessionMessage, error)
	ListAccountUsage(RequestContext, domain.AgentAccountUsageQuery, PageRequest) (domain.AgentUsageResponse, error)
	ListAccountSessionUsage(RequestContext, string, PageRequest) (domain.AgentSessionUsagePage, error)
	UploadSessionFile(RequestContext, string, domain.UploadAgentSessionFileInput) (domain.AgentSessionFile, error)
	ListSessionFiles(RequestContext, string) ([]domain.AgentSessionFile, error)
	DownloadSessionFile(RequestContext, string, string) (domain.AgentSessionFileDownload, error)
	DeleteSessionFile(RequestContext, string, string) (domain.AgentSessionFile, error)
	ListMemories(RequestContext, domain.ListAgentMemoriesQuery) ([]domain.AgentMemory, error)
	CreateMemory(RequestContext, domain.CreateAgentMemoryInput) (domain.AgentMemory, error)
	UpdateMemory(RequestContext, string, domain.UpdateAgentMemoryInput) (domain.AgentMemory, error)
	DeleteMemory(RequestContext, string) (domain.AgentMemory, error)
	ListModels(RequestContext) ([]domain.AgentModel, error)
	GetModel(RequestContext, string) (domain.AgentModel, error)
	CreateModel(RequestContext, domain.CreateAgentModelInput) (domain.AgentModel, error)
	UpdateModel(RequestContext, string, domain.UpdateAgentModelInput) (domain.AgentModel, error)
	DeleteModel(RequestContext, string) (domain.AgentModel, error)
	SyncModel(RequestContext, string) (domain.AgentModel, error)
	TestModel(RequestContext, string) (domain.AgentModel, error)
	ListKnowledgeBases(RequestContext) ([]domain.KnowledgeBase, error)
	GetKnowledgeBase(RequestContext, string) (domain.KnowledgeBase, error)
	CreateKnowledgeBase(RequestContext, domain.CreateKnowledgeBaseInput) (domain.KnowledgeBase, error)
	UpdateKnowledgeBase(RequestContext, string, domain.UpdateKnowledgeBaseInput) (domain.KnowledgeBase, error)
	DeleteKnowledgeBase(RequestContext, string) (domain.KnowledgeBase, error)
	ListKnowledgeDocuments(RequestContext, string) ([]domain.KnowledgeDocument, error)
	CreateKnowledgeDocument(RequestContext, string, domain.CreateKnowledgeDocumentInput) (domain.KnowledgeDocument, error)
	UploadKnowledgeDocument(RequestContext, string, domain.UploadKnowledgeDocumentInput) (domain.KnowledgeDocument, error)
	UpdateKnowledgeDocument(RequestContext, string, string, domain.UpdateKnowledgeDocumentInput) (domain.KnowledgeDocument, error)
	DeleteKnowledgeDocument(RequestContext, string, string) (domain.KnowledgeDocument, error)
	SearchKnowledge(RequestContext, domain.KnowledgeSearchInput) (domain.KnowledgeSearchResult, error)
	ListDefinitions(RequestContext) ([]domain.AgentDefinition, error)
	GetDefinition(RequestContext, string) (domain.AgentDefinition, error)
	CreateDefinition(RequestContext, domain.CreateAgentDefinitionInput) (domain.AgentDefinition, error)
	UpdateDefinition(RequestContext, string, domain.UpdateAgentDefinitionInput) (domain.AgentDefinition, error)
	PublishDefinition(RequestContext, string) (domain.AgentDefinition, error)
	UnpublishDefinition(RequestContext, string) (domain.AgentDefinition, error)
	DeleteDefinition(RequestContext, string) (domain.AgentDefinition, error)
	Trial(RequestContext, string, domain.AgentTrialInput) (domain.AgentTrialResult, error)
	RollbackDefinition(RequestContext, string, domain.RollbackAgentDefinitionInput) (domain.AgentDefinition, error)
	Tools(RequestContext) ([]domain.AgentToolMeta, error)
	ListExternalTools(RequestContext) ([]domain.AgentExternalTool, error)
	CreateExternalTool(RequestContext, domain.CreateAgentExternalToolInput) (domain.AgentExternalTool, error)
	DeleteExternalTool(RequestContext, string) (domain.AgentExternalTool, error)
}

// NotificationFacade 定義系統通知 facade 的行為契約。
type NotificationFacade interface {
	ListNotifications(RequestContext, NotificationListQuery) (NotificationListResponse, error)
	UnreadNotificationCount(RequestContext) (NotificationUnreadCountResponse, error)
	MarkNotificationRead(RequestContext, string) (NotificationReadResponse, error)
	MarkAllNotificationsRead(RequestContext) (NotificationReadAllResponse, error)
}

// AuditFacade 定義稽覈 facade 的行為契約。
type AuditFacade interface {
	ListLogPage(RequestContext, PageRequest) (PageResponse[AuditLog], error)
	RecordSecurityEvent(RequestContext, string, string, string, map[string]any) error
}

var (
	_ IdentityFacade     = IdentityService{}
	_ MeFacade           = MeService{}
	_ AuthzFacade        = AuthzService{}
	_ IAMFacade          = IAMService{}
	_ HRFacade           = HRService{}
	_ AttendanceFacade   = AttendanceService{}
	_ PlatformFacade     = PlatformService{}
	_ WorkspaceFacade    = WorkspaceService{}
	_ WorkflowFacade     = WorkflowService{}
	_ NotificationFacade = NotificationService{}
	_ AuditFacade        = AuditService{}
)
