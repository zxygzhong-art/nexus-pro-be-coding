package memory

import (
	"context"

	"nexus-pro-be/internal/repository"
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
		tenants:                cloneMap(s.tenants, copyTenant),
		accounts:               cloneNestedMap(s.accounts, copyAccount),
		userIdentities:         cloneNestedMap(s.userIdentities, copyUserIdentity),
		userGroups:             cloneNestedMap(s.userGroups, copyUserGroup),
		groupMemberships:       cloneNestedMap(s.groupMemberships, copyGroupMembership),
		permissionSets:         cloneNestedMap(s.permissionSets, copyPermissionSet),
		permissionPackages:     cloneMap(s.permissionPackages, copyPermissionPackage),
		permissionSetTemplates: cloneNestedMap(s.permissionSetTemplates, copyPermissionSetTemplate),
		userGroupTemplates:     cloneNestedMap(s.userGroupTemplates, copyUserGroupTemplate),
		assumableRoleTemplates: cloneNestedMap(s.assumableRoleTemplates, copyAssumableRoleTemplate),
		permissionImports:      cloneNestedMap(s.permissionImports, copyPermissionPackageImport),
		permissionCatalog:      cloneNestedMap(s.permissionCatalog, copyPermissionCatalogItem),
		menuItems:              cloneNestedMap(s.menuItems, copyMenuItem),
		permissionSetItems:     cloneNestedMap(s.permissionSetItems, copyPermissionSetItem),
		assignments:            cloneNestedMap(s.assignments, copyPermissionSetAssignment),
		dataScopes:             cloneNestedMap(s.dataScopes, copyDataScope),
		fieldPolicies:          cloneNestedMap(s.fieldPolicies, copyFieldPolicy),
		assumableRoles:         cloneNestedMap(s.assumableRoles, copyAssumableRole),
		roleSessions:           cloneNestedMap(s.roleSessions, copyAssumableRoleSession),
		orgUnits:               cloneNestedMap(s.orgUnits, copyOrgUnit),
		positions:              cloneNestedMap(s.positions, copyPosition),
		employees:              cloneNestedMap(s.employees, copyEmployee),
		employeeNoSequences:    cloneNestedMap(s.employeeNoSequences, func(v int) int { return v }),
		employeeImports:        cloneNestedMap(s.employeeImports, copyEmployeeImportSession),
		employmentContracts:    cloneNestedMap(s.employmentContracts, copyEmploymentContract),
		attendancePolicies:     cloneMap(s.attendancePolicies, copyAttendancePolicy),
		leaveBalances:          cloneNestedMap(s.leaveBalances, copyLeaveBalance),
		leaveRequests:          cloneNestedMap(s.leaveRequests, copyLeaveRequest),
		attendanceWorksites:    cloneNestedMap(s.attendanceWorksites, copyAttendanceWorksite),
		attendanceShifts:       cloneNestedMap(s.attendanceShifts, copyAttendanceShift),
		attendanceAssignments:  cloneNestedMap(s.attendanceAssignments, copyAttendanceShiftAssignment),
		attendanceClockRecords: cloneNestedMap(s.attendanceClockRecords, copyAttendanceClockRecord),
		attendanceSummaries:    cloneNestedMap(s.attendanceSummaries, copyAttendanceDailySummary),
		attendanceCorrections:  cloneNestedMap(s.attendanceCorrections, copyAttendanceCorrectionRequest),
		overtimeRequests:       cloneNestedMap(s.overtimeRequests, copyOvertimeRequest),
		formTemplates:          cloneNestedMap(s.formTemplates, copyFormTemplate),
		formInstances:          cloneNestedMap(s.formInstances, copyFormInstance),
		workflowRuns:           cloneNestedMap(s.workflowRuns, copyWorkflowRun),
		workflowStageInstances: cloneNestedMap(s.workflowStageInstances, copyWorkflowStageInstance),
		workflowStageAssignees: cloneNestedMap(s.workflowStageAssignees, copyWorkflowStageAssignee),
		workflowActions:        cloneSliceMap(s.workflowActions, copyWorkflowAction),
		platformTaskItems:      cloneNestedMap(s.platformTaskItems, copyPlatformTaskRecordItem),
		platformTaskTodos:      cloneNestedMap(s.platformTaskTodos, copyPlatformTaskTodoRecord),
		agentRuns:              cloneNestedMap(s.agentRuns, copyAgentRun),
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
	s.employeeImports = next.employeeImports
	s.employmentContracts = next.employmentContracts
	s.attendancePolicies = next.attendancePolicies
	s.leaveBalances = next.leaveBalances
	s.leaveRequests = next.leaveRequests
	s.attendanceWorksites = next.attendanceWorksites
	s.attendanceShifts = next.attendanceShifts
	s.attendanceAssignments = next.attendanceAssignments
	s.attendanceClockRecords = next.attendanceClockRecords
	s.attendanceSummaries = next.attendanceSummaries
	s.attendanceCorrections = next.attendanceCorrections
	s.overtimeRequests = next.overtimeRequests
	s.formTemplates = next.formTemplates
	s.formInstances = next.formInstances
	s.workflowRuns = next.workflowRuns
	s.workflowStageInstances = next.workflowStageInstances
	s.workflowStageAssignees = next.workflowStageAssignees
	s.workflowActions = next.workflowActions
	s.platformTaskItems = next.platformTaskItems
	s.platformTaskTodos = next.platformTaskTodos
	s.agentRuns = next.agentRuns
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
func cloneNestedMap[T any](src map[string]map[string]T, clone func(T) T) map[string]map[string]T {
	dst := make(map[string]map[string]T, len(src))
	for tenantID, bucket := range src {
		nextBucket := make(map[string]T, len(bucket))
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
