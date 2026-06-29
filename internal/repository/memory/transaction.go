package memory

import (
	"context"

	"nexus-pro-be/internal/repository"
)

var _ repository.Store = (*Store)(nil)

// WithTenantTransaction applies writes to a cloned store and commits only on success.
func (s *Store) WithTenantTransaction(ctx context.Context, tenantID string, fn func(repository.Store) error) error {
	_ = ctx
	_ = tenantID
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clone-and-swap gives tests transaction semantics without a database.
	tx := s.cloneLocked()
	if err := fn(tx); err != nil {
		return err
	}
	s.replaceLocked(tx)
	return nil
}

func (s *Store) cloneLocked() *Store {
	return &Store{
		tenants:                cloneMap(s.tenants, copyTenant),
		accounts:               cloneNestedMap(s.accounts, copyAccount),
		userIdentities:         cloneNestedMap(s.userIdentities, copyUserIdentity),
		userGroups:             cloneNestedMap(s.userGroups, copyUserGroup),
		permissionSets:         cloneNestedMap(s.permissionSets, copyPermissionSet),
		assignments:            cloneNestedMap(s.assignments, copyPermissionSetAssignment),
		dataScopes:             cloneNestedMap(s.dataScopes, copyDataScope),
		fieldPolicies:          cloneNestedMap(s.fieldPolicies, copyFieldPolicy),
		assumableRoles:         cloneNestedMap(s.assumableRoles, copyAssumableRole),
		roleSessions:           cloneNestedMap(s.roleSessions, copyAssumableRoleSession),
		orgUnits:               cloneNestedMap(s.orgUnits, copyOrgUnit),
		employees:              cloneNestedMap(s.employees, copyEmployee),
		employeeNoSequences:    cloneNestedMap(s.employeeNoSequences, func(v int) int { return v }),
		employeeImports:        cloneNestedMap(s.employeeImports, copyEmployeeImportSession),
		leaveBalances:          cloneNestedMap(s.leaveBalances, copyLeaveBalance),
		leaveRequests:          cloneNestedMap(s.leaveRequests, copyLeaveRequest),
		attendanceWorksites:    cloneNestedMap(s.attendanceWorksites, copyAttendanceWorksite),
		attendanceShifts:       cloneNestedMap(s.attendanceShifts, copyAttendanceShift),
		attendanceAssignments:  cloneNestedMap(s.attendanceAssignments, copyAttendanceShiftAssignment),
		attendanceClockRecords: cloneNestedMap(s.attendanceClockRecords, copyAttendanceClockRecord),
		attendanceCorrections:  cloneNestedMap(s.attendanceCorrections, copyAttendanceCorrectionRequest),
		formTemplates:          cloneNestedMap(s.formTemplates, copyFormTemplate),
		formInstances:          cloneNestedMap(s.formInstances, copyFormInstance),
		knowledgeArticles:      cloneNestedMap(s.knowledgeArticles, copyKnowledgeArticle),
		agentRuns:              cloneNestedMap(s.agentRuns, copyAgentRun),
		auditLogs:              cloneSliceMap(s.auditLogs, copyAuditLog),
		permissionVersions:     cloneMap(s.permissionVersions, func(v int64) int64 { return v }),
		authzOutbox:            cloneSliceMap(s.authzOutbox, copyAuthzOutboxEvent),
		outboxEvents:           cloneSliceMap(s.outboxEvents, copyOutboxEvent),
		relationshipTuples:     cloneNestedMap(s.relationshipTuples, func(v AuthzRelationshipTuple) AuthzRelationshipTuple { return v }),
	}
}

func (s *Store) replaceLocked(next *Store) {
	s.tenants = next.tenants
	s.accounts = next.accounts
	s.userIdentities = next.userIdentities
	s.userGroups = next.userGroups
	s.permissionSets = next.permissionSets
	s.assignments = next.assignments
	s.dataScopes = next.dataScopes
	s.fieldPolicies = next.fieldPolicies
	s.assumableRoles = next.assumableRoles
	s.roleSessions = next.roleSessions
	s.orgUnits = next.orgUnits
	s.employees = next.employees
	s.employeeNoSequences = next.employeeNoSequences
	s.employeeImports = next.employeeImports
	s.leaveBalances = next.leaveBalances
	s.leaveRequests = next.leaveRequests
	s.attendanceWorksites = next.attendanceWorksites
	s.attendanceShifts = next.attendanceShifts
	s.attendanceAssignments = next.attendanceAssignments
	s.attendanceClockRecords = next.attendanceClockRecords
	s.attendanceCorrections = next.attendanceCorrections
	s.formTemplates = next.formTemplates
	s.formInstances = next.formInstances
	s.knowledgeArticles = next.knowledgeArticles
	s.agentRuns = next.agentRuns
	s.auditLogs = next.auditLogs
	s.permissionVersions = next.permissionVersions
	s.authzOutbox = next.authzOutbox
	s.outboxEvents = next.outboxEvents
	s.relationshipTuples = next.relationshipTuples
}

func cloneMap[T any](src map[string]T, clone func(T) T) map[string]T {
	dst := make(map[string]T, len(src))
	for key, value := range src {
		dst[key] = clone(value)
	}
	return dst
}

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
