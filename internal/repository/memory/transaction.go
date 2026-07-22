package memory

import (
	"context"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository"
)

var _ repository.Store = (*Store)(nil)

// WithTenantTransaction 從儲存層附加租戶 transaction。
func (s *Store) WithTenantTransaction(ctx context.Context, tenantID string, fn func(repository.Store) error) error {
	_ = ctx
	_ = tenantID
	s.mu.Lock()
	defer s.mu.Unlock()

	// clone-and-swap 讓測試在沒有資料庫時仍具備 transaction 語義。
	tx := s.cloneLocked()
	if err := fn(tx); err != nil {
		return err
	}
	s.replaceLocked(tx)
	return nil
}

// cloneLocked 從儲存層複製 locked。
func (s *Store) cloneLocked() *Store {
	return &Store{
		tenants:                          cloneMap(s.tenants, copyTenant),
		accounts:                         cloneNestedMap(s.accounts, copyAccount),
		userIdentities:                   cloneNestedMap(s.userIdentities, copyUserIdentity),
		userGroups:                       cloneNestedMap(s.userGroups, copyUserGroup),
		groupMemberships:                 cloneNestedMap(s.groupMemberships, copyGroupMembership),
		permissionSets:                   cloneNestedMap(s.permissionSets, copyPermissionSet),
		permissionPackages:               cloneMap(s.permissionPackages, copyPermissionPackage),
		permissionSetTemplates:           cloneNestedMap(s.permissionSetTemplates, copyPermissionSetTemplate),
		userGroupTemplates:               cloneNestedMap(s.userGroupTemplates, copyUserGroupTemplate),
		assumableRoleTemplates:           cloneNestedMap(s.assumableRoleTemplates, copyAssumableRoleTemplate),
		permissionImports:                cloneNestedMap(s.permissionImports, copyPermissionPackageImport),
		permissionCatalog:                cloneNestedMap(s.permissionCatalog, copyPermissionCatalogItem),
		menuItems:                        cloneNestedMap(s.menuItems, copyMenuItem),
		permissionSetItems:               cloneNestedMap(s.permissionSetItems, copyPermissionSetItem),
		assignments:                      cloneNestedMap(s.assignments, copyPermissionSetAssignment),
		dataScopes:                       cloneNestedMap(s.dataScopes, copyDataScope),
		fieldPolicies:                    cloneNestedMap(s.fieldPolicies, copyFieldPolicy),
		assumableRoles:                   cloneNestedMap(s.assumableRoles, copyAssumableRole),
		roleSessions:                     cloneNestedMap(s.roleSessions, copyAssumableRoleSession),
		orgUnits:                         cloneNestedMap(s.orgUnits, copyOrgUnit),
		positions:                        cloneNestedMap(s.positions, copyPosition),
		employees:                        cloneNestedMap(s.employees, copyEmployee),
		employeeNoSequences:              cloneNestedMap(s.employeeNoSequences, func(v int) int { return v }),
		attendancePolicyVersions:         cloneNestedMap(s.attendancePolicyVersions, copyAttendancePolicy),
		leaveTypes:                       cloneNestedMap(s.leaveTypes, func(v domain.LeaveType) domain.LeaveType { return v }),
		leaveTypeExternalRefs:            cloneNestedMap(s.leaveTypeExternalRefs, func(v LeaveTypeExternalRef) LeaveTypeExternalRef { return v }),
		leaveBalances:                    cloneNestedMap(s.leaveBalances, copyLeaveBalance),
		leaveBalanceEntries:              cloneNestedMap(s.leaveBalanceEntries, copyLeaveBalanceEntry),
		leaveRequests:                    cloneNestedMap(s.leaveRequests, copyLeaveRequest),
		leaveRequestAllocations:          cloneNestedMap(s.leaveRequestAllocations, func(v LeaveRequestAllocation) LeaveRequestAllocation { return v }),
		leaveCases:                       cloneNestedMap(s.leaveCases, func(v LeaveCase) LeaveCase { return v }),
		leaveCaseSources:                 cloneNestedMap(s.leaveCaseSources, func(v LeaveCaseSource) LeaveCaseSource { return v }),
		externalLeaveRecords:             cloneNestedMap(s.externalLeaveRecords, copyExternalLeaveRecord),
		attendanceWorksites:              cloneNestedMap(s.attendanceWorksites, copyAttendanceWorksite),
		attendanceClockRecords:           cloneNestedMap(s.attendanceClockRecords, copyAttendanceClockRecord),
		attendanceSummaries:              cloneNestedMap(s.attendanceSummaries, copyAttendanceDailySummary),
		attendanceDayProjections:         cloneNestedMap(s.attendanceDayProjections, copyAttendanceDayProjection),
		attendanceCorrections:            cloneNestedMap(s.attendanceCorrections, copyAttendanceCorrectionRequest),
		overtimeRequests:                 cloneNestedMap(s.overtimeRequests, copyOvertimeRequest),
		formDefinitionDrafts:             cloneNestedMap(s.formDefinitionDrafts, copyFormDefinitionDraft),
		formTemplates:                    cloneNestedMap(s.formTemplates, copyFormTemplate),
		formTemplateVersions:             cloneNestedMap(s.formTemplateVersions, copyFormTemplateVersion),
		formInstances:                    cloneNestedMap(s.formInstances, copyFormInstance),
		formInstanceFieldValues:          cloneNestedMap(s.formInstanceFieldValues, copyFormInstanceFieldValues),
		formInstanceFiles:                cloneNestedMap(s.formInstanceFiles, copyFormInstanceFile),
		workflowRuns:                     cloneNestedMap(s.workflowRuns, copyWorkflowRun),
		workflowStageInstances:           cloneNestedMap(s.workflowStageInstances, copyWorkflowStageInstance),
		workflowStageAssignees:           cloneNestedMap(s.workflowStageAssignees, copyWorkflowStageAssignee),
		workflowActions:                  cloneSliceMap(s.workflowActions, copyWorkflowAction),
		platformTaskItems:                cloneNestedMap(s.platformTaskItems, copyPlatformTaskRecordItem),
		platformTaskTodos:                cloneNestedMap(s.platformTaskTodos, copyPlatformTaskTodoRecord),
		agentRuns:                        cloneNestedMap(s.agentRuns, copyAgentRun),
		agentModels:                      cloneNestedMap(s.agentModels, copyAgentModel),
		agentExternalTools:               cloneNestedMap(s.agentExternalTools, copyAgentExternalTool),
		agentDefinitions:                 cloneNestedMap(s.agentDefinitions, copyAgentDefinition),
		agentDefinitionVersions:          cloneNestedMap(s.agentDefinitionVersions, copyAgentDefinitionVersion),
		agentExecutionSteps:              cloneNestedMap(s.agentExecutionSteps, copyExecutionStep),
		agentRevisionExternalTools:       cloneNestedMap(s.agentRevisionExternalTools, copyAgentRevisionExternalTools),
		agentRevisionMemberExternalTools: cloneNestedMap(s.agentRevisionMemberExternalTools, copyAgentRevisionMemberExternalTools),
		agentConfirmations:               cloneNestedMap(s.agentConfirmations, copyAgentConfirmation),
		knowledgeBases:                   cloneNestedMap(s.knowledgeBases, func(v KnowledgeBase) KnowledgeBase { return v }),
		knowledgeDocuments:               cloneNestedMap(s.knowledgeDocuments, func(v KnowledgeDocument) KnowledgeDocument { return v }),
		knowledgeDocumentChunks:          cloneNestedMap(s.knowledgeDocumentChunks, copyKnowledgeDocumentChunk),
		agentSessions:                    cloneNestedMap(s.agentSessions, copyAgentSession),
		agentSessionMessages:             cloneNestedMap(s.agentSessionMessages, copyAgentSessionMessage),
		agentSessionFiles:                cloneNestedMap(s.agentSessionFiles, copyAgentSessionFile),
		agentFileChunks:                  cloneNestedMap(s.agentFileChunks, func(v []string) []string { return append([]string(nil), v...) }),
		agentMessageAttachments: cloneNestedMap(s.agentMessageAttachments, func(v []domain.AgentMessageAttachment) []domain.AgentMessageAttachment {
			out := append([]domain.AgentMessageAttachment(nil), v...)
			for index := range out {
				out[index].File = copyAgentSessionFile(out[index].File)
			}
			return out
		}),
		agentMemories:          cloneNestedMap(s.agentMemories, copyAgentMemory),
		notifications:          cloneNestedMap(s.notifications, copyNotification),
		notificationRecipients: cloneNestedMap(s.notificationRecipients, copyNotificationRecipient),
		auditLogs:              cloneSliceMap(s.auditLogs, copyAuditLog),
		permissionVersions:     cloneMap(s.permissionVersions, func(v int64) int64 { return v }),
		identityOutbox:         cloneSliceMap(s.identityOutbox, func(v IdentityProvisioningOutboxEvent) IdentityProvisioningOutboxEvent { return v }),
		outboxEvents:           cloneSliceMap(s.outboxEvents, copyOutboxEvent),
		relationshipTuples:     cloneNestedMap(s.relationshipTuples, func(v AuthzRelationshipTuple) AuthzRelationshipTuple { return v }),
	}
}

// replaceLocked 從儲存層處理 replace locked。
func (s *Store) replaceLocked(next *Store) {
	s.tenants = next.tenants
	s.accounts = next.accounts
	s.userIdentities = next.userIdentities
	s.userGroups = next.userGroups
	s.groupMemberships = next.groupMemberships
	s.permissionSets = next.permissionSets
	s.permissionPackages = next.permissionPackages
	s.permissionSetTemplates = next.permissionSetTemplates
	s.userGroupTemplates = next.userGroupTemplates
	s.assumableRoleTemplates = next.assumableRoleTemplates
	s.permissionImports = next.permissionImports
	s.permissionCatalog = next.permissionCatalog
	s.menuItems = next.menuItems
	s.permissionSetItems = next.permissionSetItems
	s.assignments = next.assignments
	s.dataScopes = next.dataScopes
	s.fieldPolicies = next.fieldPolicies
	s.assumableRoles = next.assumableRoles
	s.roleSessions = next.roleSessions
	s.orgUnits = next.orgUnits
	s.positions = next.positions
	s.employees = next.employees
	s.employeeNoSequences = next.employeeNoSequences
	s.attendancePolicyVersions = next.attendancePolicyVersions
	s.leaveTypes = next.leaveTypes
	s.leaveTypeExternalRefs = next.leaveTypeExternalRefs
	s.leaveBalances = next.leaveBalances
	s.leaveBalanceEntries = next.leaveBalanceEntries
	s.leaveRequests = next.leaveRequests
	s.leaveRequestAllocations = next.leaveRequestAllocations
	s.leaveCases = next.leaveCases
	s.leaveCaseSources = next.leaveCaseSources
	s.externalLeaveRecords = next.externalLeaveRecords
	s.attendanceWorksites = next.attendanceWorksites
	s.attendanceClockRecords = next.attendanceClockRecords
	s.attendanceSummaries = next.attendanceSummaries
	s.attendanceDayProjections = next.attendanceDayProjections
	s.attendanceCorrections = next.attendanceCorrections
	s.overtimeRequests = next.overtimeRequests
	s.formDefinitionDrafts = next.formDefinitionDrafts
	s.formTemplates = next.formTemplates
	s.formTemplateVersions = next.formTemplateVersions
	s.formInstances = next.formInstances
	s.formInstanceFieldValues = next.formInstanceFieldValues
	s.formInstanceFiles = next.formInstanceFiles
	s.workflowRuns = next.workflowRuns
	s.workflowStageInstances = next.workflowStageInstances
	s.workflowStageAssignees = next.workflowStageAssignees
	s.workflowActions = next.workflowActions
	s.platformTaskItems = next.platformTaskItems
	s.platformTaskTodos = next.platformTaskTodos
	s.agentRuns = next.agentRuns
	s.agentModels = next.agentModels
	s.agentExternalTools = next.agentExternalTools
	s.agentDefinitions = next.agentDefinitions
	s.agentDefinitionVersions = next.agentDefinitionVersions
	s.agentExecutionSteps = next.agentExecutionSteps
	s.agentRevisionExternalTools = next.agentRevisionExternalTools
	s.agentRevisionMemberExternalTools = next.agentRevisionMemberExternalTools
	s.agentConfirmations = next.agentConfirmations
	s.knowledgeBases = next.knowledgeBases
	s.knowledgeDocuments = next.knowledgeDocuments
	s.knowledgeDocumentChunks = next.knowledgeDocumentChunks
	s.agentSessions = next.agentSessions
	s.agentSessionMessages = next.agentSessionMessages
	s.agentSessionFiles = next.agentSessionFiles
	s.agentFileChunks = next.agentFileChunks
	s.agentMessageAttachments = next.agentMessageAttachments
	s.agentMemories = next.agentMemories
	s.notifications = next.notifications
	s.notificationRecipients = next.notificationRecipients
	s.auditLogs = next.auditLogs
	s.permissionVersions = next.permissionVersions
	s.identityOutbox = next.identityOutbox
	s.outboxEvents = next.outboxEvents
	s.relationshipTuples = next.relationshipTuples
}

// cloneMap 複製 map。
func cloneMap[T any](src map[string]T, clone func(T) T) map[string]T {
	dst := make(map[string]T, len(src))
	for key, value := range src {
		dst[key] = clone(value)
	}
	return dst
}

// cloneNestedMap 複製 nested map。
func cloneNestedMap[K comparable, T any](src map[string]map[K]T, clone func(T) T) map[string]map[K]T {
	dst := make(map[string]map[K]T, len(src))
	for tenantID, bucket := range src {
		nextBucket := make(map[K]T, len(bucket))
		for key, value := range bucket {
			nextBucket[key] = clone(value)
		}
		dst[tenantID] = nextBucket
	}
	return dst
}

// cloneSliceMap 複製 slice map。
func cloneSliceMap[T any](src map[string][]T, clone func(T) T) map[string][]T {
	dst := make(map[string][]T, len(src))
	for tenantID, values := range src {
		nextValues := make([]T, len(values))
		for i, value := range values {
			nextValues[i] = clone(value)
		}
		dst[tenantID] = nextValues
	}
	return dst
}
