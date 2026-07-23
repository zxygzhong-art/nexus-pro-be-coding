package memory

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

// Store 定義儲存層的資料結構。
type Store struct {
	mu sync.RWMutex

	tenants map[string]Tenant

	accounts                   map[string]map[string]Account
	userIdentities             map[string]map[string]UserIdentity
	userGroups                 map[string]map[string]UserGroup
	groupMemberships           map[string]map[string]GroupMembership
	permissionSets             map[string]map[string]PermissionSet
	permissionPackages         map[string]PermissionPackage
	permissionSetTemplates     map[string]map[string]PermissionSetTemplate
	userGroupTemplates         map[string]map[string]UserGroupTemplate
	assumableRoleTemplates     map[string]map[string]AssumableRoleTemplate
	permissionImports          map[string]map[string]PermissionPackageImport
	permissionCatalog          map[string]map[string]PermissionCatalogItem
	menuItems                  map[string]map[string]MenuItem
	permissionSetItems         map[string]map[string]PermissionSetItem
	assignments                map[string]map[string]PermissionSetAssignment
	dataScopes                 map[string]map[string]DataScope
	fieldPolicies              map[string]map[string]FieldPolicy
	assumableRoles             map[string]map[string]AssumableRole
	roleSessions               map[string]map[string]AssumableRoleSession
	orgUnits                   map[string]map[string]OrgUnit
	positions                  map[string]map[string]Position
	employees                  map[string]map[string]Employee
	attendancePolicyVersions   map[string]map[int]AttendancePolicy
	leaveTypes                 map[string]map[string]domain.LeaveType
	leaveBalances              map[string]map[string]LeaveBalance
	leaveBalanceEntries        map[string]map[string]LeaveBalanceEntry
	leaveRecords               map[string]map[string]LeaveRecord
	leaveRequests              map[string]map[string]LeaveRequest
	attendanceWorksites        map[string]map[string]AttendanceWorksite
	attendanceClockRecords     map[string]map[string]AttendanceClockRecord
	attendanceSummaries        map[string]map[string]AttendanceDailySummary
	attendanceDayProjections   map[string]map[string]AttendanceDayProjection
	attendanceCorrections      map[string]map[string]AttendanceCorrectionRequest
	overtimeRequests           map[string]map[string]OvertimeRequest
	formDefinitionDrafts       map[string]map[string]domain.FormDefinitionDraft
	formTemplates              map[string]map[string]FormTemplate
	formTemplateVersions       map[string]map[string]FormTemplateVersion
	formInstances              map[string]map[string]FormInstance
	formInstanceFieldValues    map[string]map[string][]FormInstanceFieldValue
	formInstanceFiles          map[string]map[string]domain.FormInstanceFile
	workflowRuns               map[string]map[string]domain.WorkflowRun
	workflowStageInstances     map[string]map[string]domain.WorkflowStageInstance
	workflowStageAssignees     map[string]map[string]domain.WorkflowStageAssignee
	workflowActions            map[string][]domain.WorkflowAction
	platformTaskItems          map[string]map[string]PlatformTaskRecordItem
	platformTaskTodos          map[string]map[string]PlatformTaskTodoRecord
	agentRuns                  map[string]map[string]AgentRun
	agentModels                map[string]map[string]AgentModel
	agentExternalTools         map[string]map[string]AgentExternalTool
	agentDefinitions           map[string]map[string]AgentDefinition
	agentDefinitionVersions    map[string]map[string]AgentDefinitionVersion
	agentExecutionSteps        map[string]map[string]domain.ExecutionStep
	agentRevisionExternalTools map[string]map[string][]domain.AgentRevisionExternalTool
	agentConfirmations         map[string]map[string]domain.AgentConfirmationRecord
	knowledgeBases             map[string]map[string]KnowledgeBase
	knowledgeDocuments         map[string]map[string]KnowledgeDocument
	knowledgeDocumentChunks    map[string]map[string]KnowledgeDocumentChunk
	agentSessions              map[string]map[string]AgentSession
	agentSessionMessages       map[string]map[string]AgentSessionMessage
	agentSessionFiles          map[string]map[string]domain.AgentSessionFile
	agentFileChunks            map[string]map[string][]string
	agentMemories              map[string]map[string]AgentMemory
	notifications              map[string]map[string]Notification
	notificationRecipients     map[string]map[string]NotificationRecipient
	auditLogs                  map[string][]AuditLog
	permissionVersions         map[string]int64
	identityOutbox             map[string][]IdentityProvisioningOutboxEvent
	outboxEvents               map[string][]OutboxEvent
	relationshipTuples         map[string]map[string]AuthzRelationshipTuple
}

// NewStore 建立儲存層。
func NewStore() *Store {
	return &Store{
		tenants:                    map[string]Tenant{},
		accounts:                   map[string]map[string]Account{},
		userIdentities:             map[string]map[string]UserIdentity{},
		userGroups:                 map[string]map[string]UserGroup{},
		groupMemberships:           map[string]map[string]GroupMembership{},
		permissionSets:             map[string]map[string]PermissionSet{},
		permissionPackages:         map[string]PermissionPackage{},
		permissionSetTemplates:     map[string]map[string]PermissionSetTemplate{},
		userGroupTemplates:         map[string]map[string]UserGroupTemplate{},
		assumableRoleTemplates:     map[string]map[string]AssumableRoleTemplate{},
		permissionImports:          map[string]map[string]PermissionPackageImport{},
		permissionCatalog:          map[string]map[string]PermissionCatalogItem{},
		menuItems:                  map[string]map[string]MenuItem{},
		permissionSetItems:         map[string]map[string]PermissionSetItem{},
		assignments:                map[string]map[string]PermissionSetAssignment{},
		dataScopes:                 map[string]map[string]DataScope{},
		fieldPolicies:              map[string]map[string]FieldPolicy{},
		assumableRoles:             map[string]map[string]AssumableRole{},
		roleSessions:               map[string]map[string]AssumableRoleSession{},
		orgUnits:                   map[string]map[string]OrgUnit{},
		positions:                  map[string]map[string]Position{},
		employees:                  map[string]map[string]Employee{},
		attendancePolicyVersions:   map[string]map[int]AttendancePolicy{},
		leaveTypes:                 map[string]map[string]domain.LeaveType{},
		leaveBalances:              map[string]map[string]LeaveBalance{},
		leaveBalanceEntries:        map[string]map[string]LeaveBalanceEntry{},
		leaveRecords:               map[string]map[string]LeaveRecord{},
		leaveRequests:              map[string]map[string]LeaveRequest{},
		attendanceWorksites:        map[string]map[string]AttendanceWorksite{},
		attendanceClockRecords:     map[string]map[string]AttendanceClockRecord{},
		attendanceSummaries:        map[string]map[string]AttendanceDailySummary{},
		attendanceDayProjections:   map[string]map[string]AttendanceDayProjection{},
		attendanceCorrections:      map[string]map[string]AttendanceCorrectionRequest{},
		overtimeRequests:           map[string]map[string]OvertimeRequest{},
		formDefinitionDrafts:       map[string]map[string]domain.FormDefinitionDraft{},
		formTemplates:              map[string]map[string]FormTemplate{},
		formTemplateVersions:       map[string]map[string]FormTemplateVersion{},
		formInstances:              map[string]map[string]FormInstance{},
		formInstanceFieldValues:    map[string]map[string][]FormInstanceFieldValue{},
		formInstanceFiles:          map[string]map[string]domain.FormInstanceFile{},
		workflowRuns:               map[string]map[string]domain.WorkflowRun{},
		workflowStageInstances:     map[string]map[string]domain.WorkflowStageInstance{},
		workflowStageAssignees:     map[string]map[string]domain.WorkflowStageAssignee{},
		workflowActions:            map[string][]domain.WorkflowAction{},
		platformTaskItems:          map[string]map[string]PlatformTaskRecordItem{},
		platformTaskTodos:          map[string]map[string]PlatformTaskTodoRecord{},
		agentRuns:                  map[string]map[string]AgentRun{},
		agentModels:                map[string]map[string]AgentModel{},
		agentExternalTools:         map[string]map[string]AgentExternalTool{},
		agentDefinitions:           map[string]map[string]AgentDefinition{},
		agentDefinitionVersions:    map[string]map[string]AgentDefinitionVersion{},
		agentExecutionSteps:        map[string]map[string]domain.ExecutionStep{},
		agentRevisionExternalTools: map[string]map[string][]domain.AgentRevisionExternalTool{},
		agentConfirmations:         map[string]map[string]domain.AgentConfirmationRecord{},
		knowledgeBases:             map[string]map[string]KnowledgeBase{},
		knowledgeDocuments:         map[string]map[string]KnowledgeDocument{},
		knowledgeDocumentChunks:    map[string]map[string]KnowledgeDocumentChunk{},
		agentSessions:              map[string]map[string]AgentSession{},
		agentSessionMessages:       map[string]map[string]AgentSessionMessage{},
		agentSessionFiles:          map[string]map[string]domain.AgentSessionFile{},
		agentFileChunks:            map[string]map[string][]string{},
		agentMemories:              map[string]map[string]AgentMemory{},
		notifications:              map[string]map[string]Notification{},
		notificationRecipients:     map[string]map[string]NotificationRecipient{},
		auditLogs:                  map[string][]AuditLog{},
		permissionVersions:         map[string]int64{},
		identityOutbox:             map[string][]IdentityProvisioningOutboxEvent{},
		outboxEvents:               map[string][]OutboxEvent{},
		relationshipTuples:         map[string]map[string]AuthzRelationshipTuple{},
	}
}

// UpsertTenant 從儲存層處理 upsert 租戶。
func (s *Store) UpsertTenant(_ context.Context, v Tenant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tenants[v.ID] = copyTenant(v)
	// Memory runtime seeds a convenience catalog for tests; Postgres leave_types sync from EHRMS.
	if s.leaveTypes[v.ID] == nil {
		s.leaveTypes[v.ID] = map[string]domain.LeaveType{}
		for _, item := range domain.DefaultLeaveTypes() {
			item.TenantID = v.ID
			s.leaveTypes[v.ID][item.Code] = item
		}
	}
	return nil
}

// GetTenant 從儲存層取得租戶。
func (s *Store) GetTenant(_ context.Context, id string) (Tenant, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.tenants[id]
	if !ok {
		return Tenant{}, false, nil
	}
	return copyTenant(v), true, nil
}

// ListTenants 從儲存層列出租戶。
func (s *Store) ListTenants(_ context.Context) ([]Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Tenant, 0, len(s.tenants))
	for _, v := range s.tenants {
		out = append(out, copyTenant(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// UpsertAccount 從儲存層處理 upsert 帳號。Version > 0 時執行樂觀鎖檢查。
func (s *Store) UpsertAccount(_ context.Context, v Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v.PreferredLocale = domain.PreferredLocaleWithDefault(v.PreferredLocale)
	existing, ok := getNested(s.accounts, v.TenantID, v.ID)
	if ok {
		if v.Version > 0 && existing.Version != v.Version {
			return domain.Conflict("account was modified concurrently")
		}
		v.Version = existing.Version + 1
	} else {
		v.Version = 1
	}
	putNested(s.accounts, v.TenantID, v.ID, copyAccount(v))
	return nil
}

// UpdateAccountPreferredLocale updates one self-service preference without rewriting unrelated account fields.
func (s *Store) UpdateAccountPreferredLocale(_ context.Context, tenantID, id, preferredLocale string) (Account, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := getNested(s.accounts, tenantID, id)
	if !ok {
		return Account{}, false, nil
	}
	account.PreferredLocale = preferredLocale
	account.Version++
	putNested(s.accounts, tenantID, id, copyAccount(account))
	return copyAccount(account), true, nil
}

// GetAccount 從儲存層取得帳號。
func (s *Store) GetAccount(_ context.Context, tenantID, id string) (Account, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.accounts, tenantID, id)
	if !ok {
		return Account{}, false, nil
	}
	return copyAccount(v), true, nil
}

// ListAccounts 從儲存層列出帳號。
func (s *Store) ListAccounts(_ context.Context, tenantID string) ([]Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.accounts[tenantID], copyAccount)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// UpsertUserIdentity 從儲存層處理 upsert 使用者身分。
func (s *Store) UpsertUserIdentity(_ context.Context, v UserIdentity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.userIdentities, v.TenantID, identityKey(v.Provider, v.Subject), copyUserIdentity(v))
	return nil
}

// GetUserIdentity 從儲存層取得使用者身分。
func (s *Store) GetUserIdentity(_ context.Context, tenantID, provider, subject string) (UserIdentity, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.userIdentities, tenantID, identityKey(provider, subject))
	if !ok {
		return UserIdentity{}, false, nil
	}
	return copyUserIdentity(v), true, nil
}

// ListUserIdentities 從儲存層列出使用者身分。
func (s *Store) ListUserIdentities(_ context.Context, tenantID, accountID string) ([]UserIdentity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.userIdentities[tenantID]
	out := make([]UserIdentity, 0)
	for _, v := range src {
		if v.AccountID == accountID {
			out = append(out, copyUserIdentity(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// UpsertUserGroup 從儲存層處理 upsert 使用者羣組。Version > 0 時執行樂觀鎖檢查。
func (s *Store) UpsertUserGroup(_ context.Context, v UserGroup) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := getNested(s.userGroups, v.TenantID, v.ID)
	if ok {
		if v.Version > 0 && existing.Version != v.Version {
			return domain.Conflict("user group was modified concurrently")
		}
		v.Version = existing.Version + 1
	} else {
		v.Version = 1
	}
	putNested(s.userGroups, v.TenantID, v.ID, copyUserGroup(v))
	return nil
}

// GetUserGroup 從儲存層取得使用者羣組。
func (s *Store) GetUserGroup(_ context.Context, tenantID, id string) (UserGroup, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.userGroups, tenantID, id)
	if !ok {
		return UserGroup{}, false, nil
	}
	return copyUserGroup(v), true, nil
}

// ListUserGroups 從儲存層列出使用者羣組。
func (s *Store) ListUserGroups(_ context.Context, tenantID string) ([]UserGroup, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.userGroups[tenantID], copyUserGroup)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// DeleteUserGroup 從儲存層刪除使用者羣組。
func (s *Store) DeleteUserGroup(_ context.Context, tenantID, id string) (UserGroup, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := getNested(s.userGroups, tenantID, id)
	if !ok {
		return UserGroup{}, false, nil
	}
	delete(s.userGroups[tenantID], id)
	return copyUserGroup(v), true, nil
}

// UpsertGroupMembership 從儲存層處理 upsert 使用者羣組成員關係。
func (s *Store) UpsertGroupMembership(_ context.Context, v GroupMembership) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.ID == "" {
		v.ID = groupMembershipKey(v.UserGroupID, v.AccountID)
	}
	putNested(s.groupMemberships, v.TenantID, v.ID, copyGroupMembership(v))
	s.refreshGroupMembershipProjectionLocked(v.TenantID, v.UserGroupID, v.AccountID, v.ValidFrom)
	return nil
}

// DeleteGroupMembership 從儲存層刪除使用者羣組成員關係。
func (s *Store) DeleteGroupMembership(_ context.Context, tenantID, userGroupID, accountID string) (GroupMembership, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bucket := s.groupMemberships[tenantID]
	if bucket == nil {
		return GroupMembership{}, false, nil
	}
	var v GroupMembership
	found := false
	for key, candidate := range bucket {
		if candidate.UserGroupID == userGroupID && candidate.AccountID == accountID {
			if !found || candidate.ValidFrom.After(v.ValidFrom) {
				v = candidate
				found = true
			}
			delete(bucket, key)
		}
	}
	if !found {
		return GroupMembership{}, false, nil
	}
	s.refreshGroupMembershipProjectionLocked(tenantID, userGroupID, accountID, time.Now())
	return copyGroupMembership(v), true, nil
}

// CloseGroupMembership ends the active membership interval without deleting history.
func (s *Store) CloseGroupMembership(_ context.Context, tenantID, userGroupID, accountID string, validUntil time.Time) (GroupMembership, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var selectedKey string
	var selected GroupMembership
	for key, candidate := range s.groupMemberships[tenantID] {
		if candidate.UserGroupID == userGroupID && candidate.AccountID == accountID && membershipActiveAt(candidate, validUntil) {
			if selectedKey == "" || candidate.ValidFrom.After(selected.ValidFrom) {
				selectedKey, selected = key, candidate
			}
		}
	}
	if selectedKey == "" {
		return GroupMembership{}, false, nil
	}
	until := validUntil
	selected.ValidUntil = &until
	s.groupMemberships[tenantID][selectedKey] = selected
	s.refreshGroupMembershipProjectionLocked(tenantID, userGroupID, accountID, validUntil)
	return copyGroupMembership(selected), true, nil
}

// GetGroupMembership 從儲存層取得使用者羣組成員關係。
func (s *Store) GetGroupMembership(_ context.Context, tenantID, userGroupID, accountID string) (GroupMembership, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var v GroupMembership
	found := false
	for _, candidate := range s.groupMemberships[tenantID] {
		if candidate.UserGroupID == userGroupID && candidate.AccountID == accountID && (!found || candidate.ValidFrom.After(v.ValidFrom)) {
			v, found = candidate, true
		}
	}
	if !found {
		return GroupMembership{}, false, nil
	}
	return copyGroupMembership(v), true, nil
}

// ListGroupMembershipsForGroup 從儲存層列出使用者羣組成員關係。
func (s *Store) ListGroupMembershipsForGroup(_ context.Context, tenantID, userGroupID string) ([]GroupMembership, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]GroupMembership, 0)
	for _, item := range s.groupMemberships[tenantID] {
		if item.UserGroupID == userGroupID {
			out = append(out, copyGroupMembership(item))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// ListActiveGroupMembershipsForAccount 從儲存層列出帳號有效使用者羣組成員關係。
func (s *Store) ListActiveGroupMembershipsForAccount(_ context.Context, tenantID, accountID string, at time.Time) ([]GroupMembership, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]GroupMembership, 0)
	for _, item := range s.groupMemberships[tenantID] {
		if item.AccountID == accountID && membershipActiveAt(item, at) {
			out = append(out, copyGroupMembership(item))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// UpsertPermissionSet 從儲存層處理 upsert 權限集合。
func (s *Store) UpsertPermissionSet(_ context.Context, v PermissionSet) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.permissionSets, v.TenantID, v.ID, copyPermissionSet(v))
	return nil
}

// GetPermissionSet 從儲存層取得權限集合。
func (s *Store) GetPermissionSet(_ context.Context, tenantID, id string) (PermissionSet, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.permissionSets, tenantID, id)
	if !ok {
		return PermissionSet{}, false, nil
	}
	return copyPermissionSet(v), true, nil
}

// ListPermissionSets 從儲存層列出權限集合。
func (s *Store) ListPermissionSets(_ context.Context, tenantID string) ([]PermissionSet, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.permissionSets[tenantID], copyPermissionSet)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// DeletePermissionSet 從儲存層刪除權限集合。
func (s *Store) DeletePermissionSet(_ context.Context, tenantID, id string) (PermissionSet, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := getNested(s.permissionSets, tenantID, id)
	if !ok {
		return PermissionSet{}, false, nil
	}
	if bucket := s.permissionSetItems[tenantID]; bucket != nil {
		for itemID, item := range bucket {
			if item.PermissionSetID == id {
				delete(bucket, itemID)
			}
		}
	}
	delete(s.permissionSets[tenantID], id)
	return copyPermissionSet(v), true, nil
}

// ReplacePermissionSetItems 從儲存層替換權限集合項。
func (s *Store) ReplacePermissionSetItems(_ context.Context, tenantID, permissionSetID string, items []PermissionSetItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	bucket := s.permissionSetItems[tenantID]
	for id, item := range bucket {
		if item.PermissionSetID == permissionSetID {
			delete(bucket, id)
		}
	}
	for _, item := range items {
		putNested(s.permissionSetItems, tenantID, item.ID, copyPermissionSetItem(item))
	}
	return nil
}

// ListPermissionSetItemsForSet 從儲存層列出權限集合項。
func (s *Store) ListPermissionSetItemsForSet(_ context.Context, tenantID, permissionSetID string) ([]PermissionSetItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PermissionSetItem, 0)
	for _, item := range s.permissionSetItems[tenantID] {
		if item.PermissionSetID == permissionSetID {
			out = append(out, copyPermissionSetItem(item))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// UpsertPermissionCatalogItem 從儲存層處理 upsert 權限 catalog 項。
func (s *Store) UpsertPermissionCatalogItem(_ context.Context, v PermissionCatalogItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, existing := range s.permissionCatalog[v.TenantID] {
		if existing.Application == v.Application &&
			existing.Resource == v.Resource &&
			existing.Action == v.Action &&
			existing.PermissionType == v.PermissionType {
			if v.ID == "" {
				v.ID = existing.ID
			}
			v.CreatedAt = existing.CreatedAt
			putNested(s.permissionCatalog, v.TenantID, id, copyPermissionCatalogItem(v))
			return nil
		}
	}
	putNested(s.permissionCatalog, v.TenantID, v.ID, copyPermissionCatalogItem(v))
	return nil
}

// GetPermissionCatalogItemByKey 從儲存層取得權限 catalog 項。
func (s *Store) GetPermissionCatalogItemByKey(_ context.Context, tenantID, application, resource, action string, permissionType domain.PermissionType) (PermissionCatalogItem, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.permissionCatalog[tenantID] {
		if item.Application == application &&
			item.Resource == resource &&
			item.Action == action &&
			item.PermissionType == permissionType {
			return copyPermissionCatalogItem(item), true, nil
		}
	}
	return PermissionCatalogItem{}, false, nil
}

// ListPermissionCatalogItems 從儲存層列出權限 catalog。
func (s *Store) ListPermissionCatalogItems(_ context.Context, tenantID string) ([]PermissionCatalogItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.permissionCatalog[tenantID], copyPermissionCatalogItem)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Application != out[j].Application {
			return out[i].Application < out[j].Application
		}
		if out[i].Resource != out[j].Resource {
			return out[i].Resource < out[j].Resource
		}
		if out[i].Action != out[j].Action {
			return out[i].Action < out[j].Action
		}
		return out[i].PermissionType < out[j].PermissionType
	})
	return out, nil
}

// UpsertMenuItem 從儲存層處理 upsert 選單項。
func (s *Store) UpsertMenuItem(_ context.Context, v MenuItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, existing := range s.menuItems[v.TenantID] {
		if existing.Key == v.Key {
			if v.ID == "" {
				v.ID = existing.ID
			}
			v.CreatedAt = existing.CreatedAt
			putNested(s.menuItems, v.TenantID, id, copyMenuItem(v))
			return nil
		}
	}
	putNested(s.menuItems, v.TenantID, v.ID, copyMenuItem(v))
	return nil
}

// ListMenuItems 從儲存層列出選單項。
func (s *Store) ListMenuItems(_ context.Context, tenantID string) ([]MenuItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.menuItems[tenantID], copyMenuItem)
	sort.Slice(out, func(i, j int) bool {
		if out[i].ParentKey != out[j].ParentKey {
			return out[i].ParentKey < out[j].ParentKey
		}
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Key < out[j].Key
	})
	return out, nil
}

// UpsertPermissionPackage 從儲存層處理 upsert 權限包。
func (s *Store) UpsertPermissionPackage(_ context.Context, v PermissionPackage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.permissionPackages[v.ID] = copyPermissionPackage(v)
	return nil
}

// UpdatePermissionPackageStatus 從儲存層更新權限包狀態。
func (s *Store) UpdatePermissionPackageStatus(_ context.Context, id string, status domain.PermissionPackageStatus, publishedAt *time.Time) (PermissionPackage, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.permissionPackages[id]
	if !ok {
		return PermissionPackage{}, false, nil
	}
	current.Status = status
	if publishedAt != nil {
		t := *publishedAt
		current.PublishedAt = &t
	} else {
		current.PublishedAt = nil
	}
	s.permissionPackages[id] = copyPermissionPackage(current)
	return copyPermissionPackage(current), true, nil
}

// GetPermissionPackage 從儲存層取得權限包。
func (s *Store) GetPermissionPackage(_ context.Context, id string) (PermissionPackage, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.permissionPackages[id]
	if !ok {
		return PermissionPackage{}, false, nil
	}
	return copyPermissionPackage(v), true, nil
}

// GetPermissionPackageByApplicationVersion 從儲存層取得權限包 by application/version。
func (s *Store) GetPermissionPackageByApplicationVersion(_ context.Context, applicationCode, version string) (PermissionPackage, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.permissionPackages {
		if item.ApplicationCode == applicationCode && item.Version == version {
			return copyPermissionPackage(item), true, nil
		}
	}
	return PermissionPackage{}, false, nil
}

// ListPermissionPackages 從儲存層列出權限包。
func (s *Store) ListPermissionPackages(_ context.Context) ([]PermissionPackage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PermissionPackage, 0, len(s.permissionPackages))
	for _, item := range s.permissionPackages {
		out = append(out, copyPermissionPackage(item))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ApplicationCode != out[j].ApplicationCode {
			return out[i].ApplicationCode < out[j].ApplicationCode
		}
		return out[i].Version < out[j].Version
	})
	return out, nil
}

// UpsertPermissionSetTemplate 從儲存層處理 upsert 權限集合模板。
func (s *Store) UpsertPermissionSetTemplate(_ context.Context, v PermissionSetTemplate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.permissionSetTemplates, v.PackageID, v.TemplateKey, copyPermissionSetTemplate(v))
	return nil
}

// ListPermissionSetTemplates 從儲存層列出權限集合模板。
func (s *Store) ListPermissionSetTemplates(_ context.Context, packageID string) ([]PermissionSetTemplate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.permissionSetTemplates[packageID], copyPermissionSetTemplate)
	sort.Slice(out, func(i, j int) bool { return out[i].TemplateKey < out[j].TemplateKey })
	return out, nil
}

// UpsertUserGroupTemplate 從儲存層處理 upsert 使用者羣組模板。
func (s *Store) UpsertUserGroupTemplate(_ context.Context, v UserGroupTemplate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.userGroupTemplates, v.PackageID, v.TemplateKey, copyUserGroupTemplate(v))
	return nil
}

// ListUserGroupTemplates 從儲存層列出使用者羣組模板。
func (s *Store) ListUserGroupTemplates(_ context.Context, packageID string) ([]UserGroupTemplate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.userGroupTemplates[packageID], copyUserGroupTemplate)
	sort.Slice(out, func(i, j int) bool { return out[i].TemplateKey < out[j].TemplateKey })
	return out, nil
}

// UpsertAssumableRoleTemplate 從儲存層處理 upsert 可承擔角色模板。
func (s *Store) UpsertAssumableRoleTemplate(_ context.Context, v AssumableRoleTemplate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.assumableRoleTemplates, v.PackageID, v.TemplateKey, copyAssumableRoleTemplate(v))
	return nil
}

// ListAssumableRoleTemplates 從儲存層列出可承擔角色模板。
func (s *Store) ListAssumableRoleTemplates(_ context.Context, packageID string) ([]AssumableRoleTemplate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.assumableRoleTemplates[packageID], copyAssumableRoleTemplate)
	sort.Slice(out, func(i, j int) bool { return out[i].TemplateKey < out[j].TemplateKey })
	return out, nil
}

// UpsertPermissionPackageImport 從儲存層處理 upsert 權限包導入記錄。
func (s *Store) UpsertPermissionPackageImport(_ context.Context, v PermissionPackageImport) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.permissionImports, v.TenantID, permissionPackageImportKey(v.PackageID, v.Version), copyPermissionPackageImport(v))
	return nil
}

// GetPermissionPackageImport 從儲存層取得權限包導入記錄。
func (s *Store) GetPermissionPackageImport(_ context.Context, tenantID, packageID, version string) (PermissionPackageImport, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.permissionImports, tenantID, permissionPackageImportKey(packageID, version))
	if !ok {
		return PermissionPackageImport{}, false, nil
	}
	return copyPermissionPackageImport(v), true, nil
}

// ListPermissionPackageImports 從儲存層列出租戶權限包導入記錄。
func (s *Store) ListPermissionPackageImports(_ context.Context, tenantID string) ([]PermissionPackageImport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.permissionImports[tenantID], copyPermissionPackageImport)
	sort.Slice(out, func(i, j int) bool { return out[i].ImportedAt.Before(out[j].ImportedAt) })
	return out, nil
}

// permissionPackageImportKey 處理權限包導入 key。
func permissionPackageImportKey(packageID, version string) string {
	return packageID + "\x00" + version
}

// UpsertPermissionSetAssignment 從儲存層處理 upsert 權限集合指派。
func (s *Store) UpsertPermissionSetAssignment(_ context.Context, v PermissionSetAssignment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.assignments, v.TenantID, v.ID, copyPermissionSetAssignment(v))
	return nil
}

// DeletePermissionSetAssignment 從儲存層刪除權限集合指派。
func (s *Store) DeletePermissionSetAssignment(_ context.Context, tenantID, id string) (PermissionSetAssignment, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := getNested(s.assignments, tenantID, id)
	if !ok {
		return PermissionSetAssignment{}, false, nil
	}
	delete(s.assignments[tenantID], id)
	return copyPermissionSetAssignment(v), true, nil
}

// ListPermissionSetAssignments 從儲存層列出權限集合指派。
func (s *Store) ListPermissionSetAssignments(_ context.Context, tenantID string) ([]PermissionSetAssignment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.assignments[tenantID], copyPermissionSetAssignment)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// ListPermissionSetAssignmentsForPrincipal 從儲存層列出權限集合指派 for principal。
func (s *Store) ListPermissionSetAssignmentsForPrincipal(_ context.Context, tenantID, principalType, principalID string) ([]PermissionSetAssignment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := nowUTC()
	out := make([]PermissionSetAssignment, 0)
	for _, item := range s.assignments[tenantID] {
		if item.PrincipalType != principalType || item.PrincipalID != principalID {
			continue
		}
		if item.StartsAt != nil && item.StartsAt.After(now) {
			continue
		}
		if item.ExpiresAt != nil && !item.ExpiresAt.After(now) {
			continue
		}
		out = append(out, copyPermissionSetAssignment(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// UpsertDataScope 從儲存層處理 upsert 資料範圍。
func (s *Store) UpsertDataScope(_ context.Context, v DataScope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.dataScopes, v.TenantID, v.ID, copyDataScope(v))
	return nil
}

// GetDataScope 從儲存層取得資料範圍。
func (s *Store) GetDataScope(_ context.Context, tenantID, id string) (DataScope, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.dataScopes, tenantID, id)
	if !ok {
		return DataScope{}, false, nil
	}
	return copyDataScope(v), true, nil
}

// ListDataScopes 從儲存層列出資料範圍。
func (s *Store) ListDataScopes(_ context.Context, tenantID string) ([]DataScope, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.dataScopes[tenantID], copyDataScope)
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

// UpdateDataScope 從儲存層更新資料範圍。
func (s *Store) UpdateDataScope(_ context.Context, v DataScope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.dataScopes, v.TenantID, v.ID, copyDataScope(v))
	return nil
}

// DeleteDataScope 從儲存層刪除資料範圍。
func (s *Store) DeleteDataScope(_ context.Context, tenantID, id string) (DataScope, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := getNested(s.dataScopes, tenantID, id)
	if !ok {
		return DataScope{}, false, nil
	}
	delete(s.dataScopes[tenantID], id)
	return copyDataScope(v), true, nil
}

// UpsertFieldPolicy 從儲存層處理 upsert 欄位政策。
func (s *Store) UpsertFieldPolicy(_ context.Context, v FieldPolicy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.fieldPolicies, v.TenantID, v.ID, copyFieldPolicy(v))
	return nil
}

// GetFieldPolicy 從儲存層取得欄位政策。
func (s *Store) GetFieldPolicy(_ context.Context, tenantID, id string) (FieldPolicy, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.fieldPolicies, tenantID, id)
	if !ok {
		return FieldPolicy{}, false, nil
	}
	return copyFieldPolicy(v), true, nil
}

// ListFieldPolicies 從儲存層列出欄位政策。
func (s *Store) ListFieldPolicies(_ context.Context, tenantID, applicationCode, resourceType string) ([]FieldPolicy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]FieldPolicy, 0)
	for _, v := range s.fieldPolicies[tenantID] {
		if (applicationCode == "" || v.ApplicationCode == applicationCode) && (resourceType == "" || v.ResourceType == resourceType) {
			out = append(out, copyFieldPolicy(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FieldName < out[j].FieldName })
	return out, nil
}

// DeleteFieldPolicy 從儲存層刪除欄位政策。
func (s *Store) DeleteFieldPolicy(_ context.Context, tenantID, id string) (FieldPolicy, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := getNested(s.fieldPolicies, tenantID, id)
	if !ok {
		return FieldPolicy{}, false, nil
	}
	delete(s.fieldPolicies[tenantID], id)
	return copyFieldPolicy(v), true, nil
}

// UpsertAssumableRole 從儲存層處理 upsert assumable 角色。
func (s *Store) UpsertAssumableRole(_ context.Context, v AssumableRole) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.assumableRoles, v.TenantID, v.ID, copyAssumableRole(v))
	return nil
}

// GetAssumableRole 從儲存層取得 assumable 角色。
func (s *Store) GetAssumableRole(_ context.Context, tenantID, id string) (AssumableRole, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.assumableRoles, tenantID, id)
	if !ok {
		return AssumableRole{}, false, nil
	}
	return copyAssumableRole(v), true, nil
}

// ListAssumableRoles 從儲存層列出 assumable 角色。
func (s *Store) ListAssumableRoles(_ context.Context, tenantID string) ([]AssumableRole, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.assumableRoles[tenantID], copyAssumableRole)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// DeleteAssumableRole 從儲存層刪除 assumable 角色。
func (s *Store) DeleteAssumableRole(_ context.Context, tenantID, id string) (AssumableRole, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := getNested(s.assumableRoles, tenantID, id)
	if !ok {
		return AssumableRole{}, false, nil
	}
	if bucket := s.roleSessions[tenantID]; bucket != nil {
		for sessionID, session := range bucket {
			if session.AssumableRoleID == id {
				delete(bucket, sessionID)
			}
		}
	}
	delete(s.assumableRoles[tenantID], id)
	return copyAssumableRole(v), true, nil
}

// UpsertAssumableRoleSession 從儲存層處理 upsert assumable 角色 session。
func (s *Store) UpsertAssumableRoleSession(_ context.Context, v AssumableRoleSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.roleSessions, v.TenantID, v.ID, copyAssumableRoleSession(v))
	return nil
}

// GetAssumableRoleSession 取得 session 原始狀態，供服務層區分失效原因並執行 ownership 驗證。
func (s *Store) GetAssumableRoleSession(_ context.Context, tenantID, id string) (AssumableRoleSession, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.roleSessions, tenantID, id)
	if !ok {
		return AssumableRoleSession{}, false, nil
	}
	return copyAssumableRoleSession(v), true, nil
}

// GetActiveAssumableRoleSession 從儲存層取得啟用中 assumable 角色 session。
func (s *Store) GetActiveAssumableRoleSession(_ context.Context, tenantID, id string) (AssumableRoleSession, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.roleSessions, tenantID, id)
	if !ok {
		return AssumableRoleSession{}, false, nil
	}
	now := nowUTC()
	if v.RevokedAt != nil || !v.ExpiresAt.After(now) {
		return AssumableRoleSession{}, false, nil
	}
	return copyAssumableRoleSession(v), true, nil
}

// RevokeAssumableRoleSession 僅撤銷同租戶、同帳號且尚未撤銷的 session。
func (s *Store) RevokeAssumableRoleSession(_ context.Context, tenantID, accountID, id string, revokedAt time.Time) (AssumableRoleSession, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := getNested(s.roleSessions, tenantID, id)
	if !ok || v.AccountID != accountID || v.RevokedAt != nil {
		return AssumableRoleSession{}, false, nil
	}
	at := revokedAt.UTC()
	v.RevokedAt = &at
	putNested(s.roleSessions, tenantID, id, copyAssumableRoleSession(v))
	return copyAssumableRoleSession(v), true, nil
}

// ListActiveAssumableRoleSessionsForRole 從儲存層列出角色啟用中 session。
func (s *Store) ListActiveAssumableRoleSessionsForRole(_ context.Context, tenantID, roleID string) ([]AssumableRoleSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := nowUTC()
	out := make([]AssumableRoleSession, 0)
	for _, v := range s.roleSessions[tenantID] {
		if v.AssumableRoleID != roleID {
			continue
		}
		if v.RevokedAt != nil || !v.ExpiresAt.After(now) {
			continue
		}
		out = append(out, copyAssumableRoleSession(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// UpsertOrgUnit 從儲存層處理 upsert 組織單位。
func (s *Store) UpsertOrgUnit(_ context.Context, v OrgUnit) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = v.CreatedAt
	}
	if current, ok := getNested(s.orgUnits, v.TenantID, v.ID); ok {
		v.ShowInOrgChart = current.ShowInOrgChart
	} else {
		v.ShowInOrgChart = true
	}
	putNested(s.orgUnits, v.TenantID, v.ID, copyOrgUnit(v))
	return nil
}

// UpdateOrgUnitOrgChartVisibility 更新組織單位在組織圖預覽中的可見性。
func (s *Store) UpdateOrgUnitOrgChartVisibility(_ context.Context, tenantID, id string, showInOrgChart bool, updatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unit, ok := getNested(s.orgUnits, tenantID, id)
	if !ok {
		return nil
	}
	unit.ShowInOrgChart = showInOrgChart
	unit.UpdatedAt = updatedAt
	putNested(s.orgUnits, tenantID, id, unit)
	return nil
}

// GetOrgUnit 從儲存層取得組織單位。
func (s *Store) GetOrgUnit(_ context.Context, tenantID, id string) (OrgUnit, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.orgUnits, tenantID, id)
	if !ok {
		return OrgUnit{}, false, nil
	}
	return copyOrgUnit(v), true, nil
}

// ListOrgUnits 從儲存層列出組織單位。
func (s *Store) ListOrgUnits(_ context.Context, tenantID string) ([]OrgUnit, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.orgUnits[tenantID], copyOrgUnit)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Closed != out[j].Closed {
			return !out[i].Closed
		}
		if out[i].Code != out[j].Code {
			return out[i].Code < out[j].Code
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

// UpsertPosition 從儲存層處理 upsert 崗位。
func (s *Store) UpsertPosition(_ context.Context, v Position) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.positions, v.TenantID, v.ID, copyPosition(v))
	return nil
}

// GetPosition 從儲存層取得崗位。
func (s *Store) GetPosition(_ context.Context, tenantID, id string) (Position, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.positions, tenantID, id)
	if !ok {
		return Position{}, false, nil
	}
	return copyPosition(v), true, nil
}

// GetPositionByCode 從儲存層取得崗位 by code。
func (s *Store) GetPositionByCode(_ context.Context, tenantID, code string) (Position, bool, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return Position{}, false, nil
	}
	return s.getPositionBy(tenantID, func(v Position) bool {
		return strings.ToLower(strings.TrimSpace(v.Code)) == code
	})
}

// GetPositionByName 從儲存層取得崗位 by name。
func (s *Store) GetPositionByName(_ context.Context, tenantID, name string) (Position, bool, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return Position{}, false, nil
	}
	return s.getPositionBy(tenantID, func(v Position) bool {
		return strings.ToLower(strings.TrimSpace(v.Name)) == name
	})
}

// getPositionBy 從儲存層取得崗位 by。
func (s *Store) getPositionBy(tenantID string, match func(Position) bool) (Position, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.positions[tenantID] {
		if match(v) {
			return copyPosition(v), true, nil
		}
	}
	return Position{}, false, nil
}

// ListPositions 從儲存層列出崗位。
func (s *Store) ListPositions(_ context.Context, tenantID string) ([]Position, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.positions[tenantID], copyPosition)
	sort.Slice(out, func(i, j int) bool {
		leftActive := out[i].Status == string(domain.PositionStatusActive)
		rightActive := out[j].Status == string(domain.PositionStatusActive)
		if leftActive != rightActive {
			return leftActive
		}
		if out[i].Name == out[j].Name {
			return out[i].ID < out[j].ID
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// UpsertEmployee 從儲存層處理 upsert 員工。
func (s *Store) UpsertEmployee(_ context.Context, v Employee) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := getNested(s.employees, v.TenantID, v.ID); ok {
		v.ShowInOrgChart = current.ShowInOrgChart
	} else {
		v.ShowInOrgChart = true
	}
	putNested(s.employees, v.TenantID, v.ID, copyEmployee(v))
	return nil
}

// UpdateEmployeeOrgChartVisibility 更新員工在組織圖預覽中的可見性。
func (s *Store) UpdateEmployeeOrgChartVisibility(_ context.Context, tenantID, id string, showInOrgChart bool, updatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	employee, ok := getNested(s.employees, tenantID, id)
	if !ok {
		return nil
	}
	employee.ShowInOrgChart = showInOrgChart
	employee.UpdatedAt = updatedAt
	putNested(s.employees, tenantID, id, employee)
	return nil
}

// GetEmployee 從儲存層取得員工。
func (s *Store) GetEmployee(_ context.Context, tenantID, id string) (Employee, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.employees, tenantID, id)
	if !ok {
		return Employee{}, false, nil
	}
	return copyEmployee(v), true, nil
}

// GetEmployeeByEmployeeNo 從儲存層取得員工 by 員工 no。
func (s *Store) GetEmployeeByEmployeeNo(_ context.Context, tenantID, employeeNo string) (Employee, bool, error) {
	return s.getEmployeeBy(tenantID, func(v Employee) bool {
		return v.EmployeeNo == strings.TrimSpace(employeeNo)
	})
}

// GetEmployeeByCompanyEmail 從儲存層取得員工 by 公司 email。
func (s *Store) GetEmployeeByCompanyEmail(_ context.Context, tenantID, companyEmail string) (Employee, bool, error) {
	email := strings.ToLower(strings.TrimSpace(companyEmail))
	return s.getEmployeeBy(tenantID, func(v Employee) bool {
		return strings.ToLower(strings.TrimSpace(v.CompanyEmail)) == email
	})
}

// GetEmployeeByPersonalEmail 從儲存層取得員工 by personal email。
func (s *Store) GetEmployeeByPersonalEmail(_ context.Context, tenantID, personalEmail string) (Employee, bool, error) {
	email := strings.ToLower(strings.TrimSpace(personalEmail))
	if email == "" {
		return Employee{}, false, nil
	}
	return s.getEmployeeBy(tenantID, func(v Employee) bool {
		return strings.ToLower(strings.TrimSpace(v.PersonalEmail)) == email
	})
}

// GetEmployeeByAccountID 從儲存層取得員工 by 帳號 ID。
func (s *Store) GetEmployeeByAccountID(_ context.Context, tenantID, accountID string) (Employee, bool, error) {
	accountID = strings.TrimSpace(accountID)
	return s.getEmployeeBy(tenantID, func(v Employee) bool {
		return v.AccountID == accountID
	})
}

// GetEmployeeByBasicInfoField 從儲存層取得員工 by 基本 info 欄位。
func (s *Store) GetEmployeeByBasicInfoField(_ context.Context, tenantID, fieldName, fieldValue string) (Employee, bool, error) {
	fieldName = strings.TrimSpace(fieldName)
	fieldValue = strings.ToLower(strings.TrimSpace(fieldValue))
	if fieldName == "" || fieldValue == "" {
		return Employee{}, false, nil
	}
	return s.getEmployeeBy(tenantID, func(v Employee) bool {
		value, _ := v.BasicInfo[fieldName].(string)
		return strings.ToLower(strings.TrimSpace(value)) == fieldValue
	})
}

// getEmployeeBy 從儲存層取得員工 by。
func (s *Store) getEmployeeBy(tenantID string, match func(Employee) bool) (Employee, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.employees[tenantID] {
		if match(v) {
			return copyEmployee(v), true, nil
		}
	}
	return Employee{}, false, nil
}

// ListEmployees 從儲存層列出員工。
func (s *Store) ListEmployees(_ context.Context, tenantID string) ([]Employee, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.employees[tenantID], copyEmployee)
	sortMemoryEmployees(out, "created_at_asc")
	return out, nil
}

// ListEmployeesByQuery 從儲存層列出員工 by 查詢。
func (s *Store) ListEmployeesByQuery(ctx context.Context, tenantID string, query EmployeeQuery) ([]Employee, error) {
	items, err := s.ListEmployees(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	items = s.filterMemoryEmployeesByQuery(tenantID, items, query)
	sortMemoryEmployees(items, query.Sort)
	return items, nil
}

// ListEmployeePageByQuery 從儲存層列出員工分頁 by 查詢。
func (s *Store) ListEmployeePageByQuery(ctx context.Context, tenantID string, query EmployeeQuery) ([]Employee, int, error) {
	items, err := s.ListEmployeesByQuery(ctx, tenantID, query)
	if err != nil {
		return nil, 0, err
	}
	total := len(items)
	query = normalizeMemoryEmployeeQuery(query)
	return paginateMemory(items, query.Page, query.PageSize), total, nil
}

// CountEmployeesByQuery 從儲存層處理 count 員工 by 查詢。
func (s *Store) CountEmployeesByQuery(ctx context.Context, tenantID string, query EmployeeQuery) (int, error) {
	items, err := s.ListEmployeesByQuery(ctx, tenantID, query)
	if err != nil {
		return 0, err
	}
	return len(items), nil
}

// filterMemoryEmployeesByQuery 從儲存層處理篩選 memory 員工 by 查詢。
func (s *Store) filterMemoryEmployeesByQuery(tenantID string, items []Employee, query EmployeeQuery) []Employee {
	out := make([]Employee, 0, len(items))
	query = normalizeMemoryEmployeeQuery(query)
	if query.Scope.DenyAll {
		return []Employee{}
	}
	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))
	employeeAllowed := memoryStringSet(query.Scope.EmployeeIDs)
	orgAllowed := memoryStringSet(query.Scope.OrgUnitIDs)
	statusAllowed := memoryStringSet(query.Scope.Statuses)
	accounts := map[string]Account{}
	if keyword != "" {
		s.mu.RLock()
		for id, account := range s.accounts[tenantID] {
			accounts[id] = copyAccount(account)
		}
		s.mu.RUnlock()
	}
	for _, item := range items {
		status := utils.FirstNonEmpty(item.EmploymentStatus, item.Status)
		employeeMatch := len(employeeAllowed) > 0
		if employeeMatch {
			_, employeeMatch = employeeAllowed[item.ID]
		}
		orgMatch := len(orgAllowed) > 0
		if orgMatch {
			_, orgMatch = orgAllowed[item.OrgUnitID]
		}
		if query.Scope.MatchAnyEntity {
			if !employeeMatch && !orgMatch {
				continue
			}
		} else {
			if len(employeeAllowed) > 0 && !employeeMatch {
				continue
			}
			if len(orgAllowed) > 0 && !orgMatch {
				continue
			}
		}
		if len(statusAllowed) > 0 {
			if _, ok := statusAllowed[status]; !ok {
				continue
			}
		}
		if query.EmploymentStatus != "deleted" && status == "deleted" {
			continue
		}
		if query.DepartmentID != "" && item.OrgUnitID != query.DepartmentID {
			continue
		}
		if query.EmploymentStatus != "" && status != query.EmploymentStatus {
			continue
		}
		if query.Category != "" && item.Category != query.Category {
			continue
		}
		if !memoryEmployeePresentInRange(item, query.PresentFrom, query.PresentTo) {
			continue
		}
		if keyword != "" {
			account := accounts[item.AccountID]
			haystack := strings.ToLower(strings.Join([]string{
				item.Name,
				item.CompanyEmail,
				item.PersonalEmail,
				item.EmployeeNo,
				item.Phone,
				item.AccountID,
				account.DisplayName,
				account.Email,
				account.EmployeeID,
			}, " "))
			if !strings.Contains(haystack, keyword) {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func memoryEmployeePresentInRange(item Employee, fromRaw, toRaw string) bool {
	if strings.TrimSpace(fromRaw) == "" && strings.TrimSpace(toRaw) == "" {
		return true
	}
	status := strings.ToLower(utils.FirstNonEmpty(item.EmploymentStatus, item.Status))
	if status == string(domain.EmployeeStatusDeleted) || (status == string(domain.EmployeeStatusResigned) && item.ResignDate == nil) {
		return false
	}
	if to, err := time.Parse(time.RFC3339, strings.TrimSpace(toRaw)); err == nil && item.HireDate != nil && !item.HireDate.Before(to) {
		return false
	}
	if from, err := time.Parse(time.RFC3339, strings.TrimSpace(fromRaw)); err == nil && item.ResignDate != nil && item.ResignDate.Before(from) {
		return false
	}
	return true
}

// memoryStringSet 處理 memory 字串集合。
func memoryStringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

// normalizeMemoryEmployeeQuery 正規化memory 員工查詢。
func normalizeMemoryEmployeeQuery(query EmployeeQuery) EmployeeQuery {
	if query.Page <= 0 {
		query.Page = DefaultPage
	}
	if query.PageSize <= 0 {
		query.PageSize = DefaultPageSize
	}
	if query.PageSize > MaxPageSize {
		query.PageSize = MaxPageSize
	}
	if query.Sort == "" {
		query.Sort = "created_at_asc"
	}
	query.EmploymentStatus = normalizeMemoryEmployeeStatus(query.EmploymentStatus)
	query.Category = normalizeMemoryEmployeeCategory(query.Category)
	return query
}

// normalizeMemoryEmployeeStatus 正規化memory 員工狀態。
func normalizeMemoryEmployeeStatus(value string) string {
	switch strings.TrimSpace(value) {
	case "在職", "active":
		return "active"
	case "試用中", "probation":
		return "probation"
	case "留停", "留職停薪", "on-leave", "leave_suspended":
		return "leave_suspended"
	case "待加入", "pending", "onboarding":
		return "onboarding"
	case "離職", "resigned":
		return "resigned"
	case "已停用", "deleted":
		return "deleted"
	default:
		return strings.TrimSpace(value)
	}
}

// normalizeMemoryEmployeeCategory 正規化memory 員工分類。
func normalizeMemoryEmployeeCategory(value string) string {
	switch strings.TrimSpace(value) {
	case "全職", "正職", "full-time", "full_time":
		return "full_time"
	case "兼職", "part-time", "part_time":
		return "part_time"
	case "實習", "intern":
		return "intern"
	case "約聘", "contract", "contractor":
		return "contractor"
	case "其他", "other":
		return "other"
	default:
		return strings.TrimSpace(value)
	}
}

// sortMemoryEmployees 排序memory 員工。
func sortMemoryEmployees(items []Employee, sortKey string) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if sortKey == "attendance_asc" {
			left := strings.ToLower(utils.FirstNonEmpty(a.EmployeeNo, a.ID))
			right := strings.ToLower(utils.FirstNonEmpty(b.EmployeeNo, b.ID))
			if left != right {
				return left < right
			}
			if !a.CreatedAt.Equal(b.CreatedAt) {
				return a.CreatedAt.Before(b.CreatedAt)
			}
			return a.ID < b.ID
		}
		leftRank := domain.EmployeeStatusSortRank(utils.FirstNonEmpty(a.EmploymentStatus, a.Status))
		rightRank := domain.EmployeeStatusSortRank(utils.FirstNonEmpty(b.EmploymentStatus, b.Status))
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		switch sortKey {
		case "created_at_desc":
			if a.CreatedAt.Equal(b.CreatedAt) {
				return a.ID > b.ID
			}
			return a.CreatedAt.After(b.CreatedAt)
		case "hire_date_desc":
			return memoryTimeValue(a.HireDate).After(memoryTimeValue(b.HireDate))
		case "hire_date_asc":
			return memoryTimeValue(a.HireDate).Before(memoryTimeValue(b.HireDate))
		default:
			if a.CreatedAt.Equal(b.CreatedAt) {
				return a.ID < b.ID
			}
			return a.CreatedAt.Before(b.CreatedAt)
		}
	})
}

// paginateMemory 處理 paginate memory。
func paginateMemory[T any](items []T, page, pageSize int) []T {
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []T{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	out := make([]T, end-start)
	copy(out, items[start:end])
	return out
}

// memoryLeaveRequestMatches 處理 memory 請假請求 matches。
func memoryLeaveRequestMatches(item LeaveRequest, query domain.LeaveRequestQuery) bool {
	if len(query.EmployeeIDs) > 0 {
		allowed := map[string]struct{}{}
		for _, id := range query.EmployeeIDs {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				allowed[trimmed] = struct{}{}
			}
		}
		if len(allowed) > 0 {
			if _, ok := allowed[item.EmployeeID]; !ok {
				return false
			}
		}
	}
	if status := strings.TrimSpace(query.Status); status != "" && !strings.EqualFold(item.Status, status) {
		return false
	}
	if from, ok := memoryDateOnly(query.FromDate); ok && item.EndAt.Before(from) {
		return false
	}
	if to, ok := memoryDateOnly(query.ToDate); ok && !item.StartAt.Before(to.AddDate(0, 0, 1)) {
		return false
	}
	return true
}

// memoryFormInstanceMatches 處理 memory 表單實例 matches。
func memoryFormInstanceMatches(item FormInstance, templateKey, templateName, applicantName string, query domain.FormInstanceQuery) bool {
	if status := strings.TrimSpace(query.Status); status != "" && item.Status != status {
		return false
	}
	if templateID := strings.TrimSpace(query.TemplateID); templateID != "" && item.TemplateID != templateID {
		return false
	}
	if key := strings.TrimSpace(query.TemplateKey); key != "" && templateKey != key {
		return false
	}
	if accountID := strings.TrimSpace(query.ApplicantAccountID); accountID != "" && item.ApplicantAccountID != accountID {
		return false
	}
	if search := strings.ToLower(strings.TrimSpace(query.Search)); search != "" {
		matched := false
		for _, field := range []string{templateKey, templateName, applicantName} {
			if strings.Contains(strings.ToLower(field), search) {
				matched = true
				break
			}
		}
		if !matched {
			for _, value := range item.Payload {
				if strings.Contains(strings.ToLower(fmt.Sprintf("%v", value)), search) {
					matched = true
					break
				}
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// memoryDateOnly 處理 memory 日期 only。
func memoryDateOnly(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

// memoryTimeValue 處理 memory 時間 value。
func memoryTimeValue(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// InsertAttendancePolicyVersion appends one immutable attendance policy version.
func (s *Store) InsertAttendancePolicyVersion(_ context.Context, v AttendancePolicy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.Version <= 0 {
		v.Version = 1
	}
	if s.attendancePolicyVersions[v.TenantID] == nil {
		s.attendancePolicyVersions[v.TenantID] = map[int]AttendancePolicy{}
	}
	if _, exists := s.attendancePolicyVersions[v.TenantID][v.Version]; exists {
		return domain.Conflict("attendance policy was modified concurrently")
	}
	s.attendancePolicyVersions[v.TenantID][v.Version] = copyAttendancePolicy(v)
	return nil
}

// GetAttendancePolicy 從儲存層取得考勤政策。
func (s *Store) GetAttendancePolicy(_ context.Context, tenantID string) (AttendancePolicy, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	versions := s.attendancePolicyVersions[tenantID]
	if len(versions) == 0 {
		return AttendancePolicy{}, false, nil
	}
	latestVersion := 0
	for version := range versions {
		if version > latestVersion {
			latestVersion = version
		}
	}
	return copyAttendancePolicy(versions[latestVersion]), true, nil
}

// GetAttendancePolicyAsOf resolves the immutable policy version effective at
// the supplied instant instead of silently applying today's rules to history.
func (s *Store) GetAttendancePolicyAsOf(_ context.Context, tenantID string, asOf time.Time) (AttendancePolicy, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var selected AttendancePolicy
	found := false
	for _, candidate := range s.attendancePolicyVersions[tenantID] {
		if candidate.EffectiveFrom != nil && candidate.EffectiveFrom.After(asOf) {
			continue
		}
		if !found || attendancePolicyComesAfter(candidate, selected) {
			selected, found = candidate, true
		}
	}
	return copyAttendancePolicy(selected), found, nil
}

func attendancePolicyComesAfter(left, right AttendancePolicy) bool {
	if left.EffectiveFrom == nil {
		return right.EffectiveFrom == nil && left.Version > right.Version
	}
	if right.EffectiveFrom == nil || !left.EffectiveFrom.Equal(*right.EffectiveFrom) {
		return right.EffectiveFrom == nil || left.EffectiveFrom.After(*right.EffectiveFrom)
	}
	return left.Version > right.Version
}

// ListLeaveTypes returns tenant-owned leave_types rows.
func (s *Store) ListLeaveTypes(_ context.Context, tenantID string) ([]domain.LeaveType, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]domain.LeaveType, 0, len(s.leaveTypes[tenantID]))
	for _, item := range s.leaveTypes[tenantID] {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].DisplayOrder != items[j].DisplayOrder {
			return items[i].DisplayOrder < items[j].DisplayOrder
		}
		return items[i].Code < items[j].Code
	})
	return items, nil
}

// UpsertLeaveType writes one leave catalog row from EHRMS sync (present rows are active).
func (s *Store) UpsertLeaveType(_ context.Context, v domain.LeaveType) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v.Code = strings.ToLower(strings.TrimSpace(v.Code))
	if strings.TrimSpace(v.ID) == "" {
		v.ID = domain.StableLeaveTypeID(v.Code)
	} else {
		v.ID = strings.ToLower(strings.TrimSpace(v.ID))
	}
	v.ParentID = strings.ToLower(strings.TrimSpace(v.ParentID))
	v.ParentCode = strings.ToLower(strings.TrimSpace(v.ParentCode))
	switch strings.ToLower(strings.TrimSpace(v.Kind)) {
	case "category":
		v.Kind = "category"
		v.ParentID = ""
		v.ParentCode = ""
	case "special_group":
		v.Kind = "special_group"
		v.ParentID = ""
		v.ParentCode = ""
	default:
		v.Kind = "item"
		if _, ok := s.leaveTypes[v.TenantID][v.ParentID]; v.ParentID != "" && !ok {
			v.ParentID = ""
			v.ParentCode = ""
		}
	}
	v.Enabled = true
	if s.leaveTypes[v.TenantID] == nil {
		s.leaveTypes[v.TenantID] = map[string]domain.LeaveType{}
	}
	s.leaveTypes[v.TenantID][v.ID] = v
	return nil
}

// UpsertLeaveTypeEnabled updates leave_types.status for one exact catalog ID.
func (s *Store) UpsertLeaveTypeEnabled(_ context.Context, tenantID, id string, enabled bool, _ string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.ToLower(strings.TrimSpace(id))
	item, ok := s.leaveTypes[tenantID][id]
	if !ok {
		return domain.NotFound("leave type", id)
	}
	item.Enabled = enabled
	s.leaveTypes[tenantID][id] = item
	return nil
}

// DeactivateMissingLeaveTypes marks leave type nodes not in activeIDs as inactive.
func (s *Store) DeactivateMissingLeaveTypes(_ context.Context, tenantID string, activeIDs []string, _ time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keep := map[string]struct{}{}
	for _, id := range activeIDs {
		id = strings.ToLower(strings.TrimSpace(id))
		if id != "" {
			keep[id] = struct{}{}
		}
	}
	var deactivated int64
	for id, item := range s.leaveTypes[tenantID] {
		if _, ok := keep[id]; ok {
			continue
		}
		if !item.Enabled {
			continue
		}
		item.Enabled = false
		s.leaveTypes[tenantID][id] = item
		deactivated++
	}
	return deactivated, nil
}

// UpsertLeaveBalance 從儲存層處理 upsert 請假 balance。
func (s *Store) UpsertLeaveBalance(_ context.Context, v LeaveBalance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v.LeaveType = strings.ToLower(strings.TrimSpace(v.LeaveType))
	v.LeaveTypeID = strings.TrimSpace(v.LeaveTypeID)
	if strings.TrimSpace(v.LeaveTypeID) == "" {
		v.LeaveTypeID = domain.StableLeaveTypeID(v.LeaveType)
	}
	if strings.TrimSpace(v.Source) == "" {
		v.Source = "nexus"
	}
	if v.EntitlementYear == 0 {
		v.EntitlementYear = v.UpdatedAt.Year()
	}
	for id, existing := range s.leaveBalances[v.TenantID] {
		if existing.EmployeeID == v.EmployeeID && existing.LeaveTypeID == v.LeaveTypeID &&
			existing.EntitlementYear == v.EntitlementYear && id != v.ID {
			v.ID = existing.ID
			break
		}
	}
	putNested(s.leaveBalances, v.TenantID, v.ID, copyLeaveBalance(v))
	return nil
}

func (s *Store) EnsureLocalLeaveBalanceAnchor(_ context.Context, v LeaveBalance) (LeaveBalance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v.LeaveType = strings.ToLower(strings.TrimSpace(v.LeaveType))
	v.LeaveTypeID = strings.TrimSpace(v.LeaveTypeID)
	if v.LeaveTypeID == "" {
		v.LeaveTypeID = domain.StableLeaveTypeID(v.LeaveType)
	}
	if v.EntitlementYear == 0 {
		v.EntitlementYear = v.UpdatedAt.Year()
	}
	if err := s.ensureLocalLeaveBalanceAnchorLocked(v); err != nil {
		return LeaveBalance{}, err
	}
	for _, existing := range s.leaveBalances[v.TenantID] {
		if existing.EmployeeID == v.EmployeeID && existing.LeaveTypeID == v.LeaveTypeID && existing.EntitlementYear == v.EntitlementYear {
			return copyLeaveBalance(existing), nil
		}
	}
	return LeaveBalance{}, domain.NotFound("local leave balance anchor", v.ID)
}

func (s *Store) ensureLocalLeaveBalanceAnchorLocked(v LeaveBalance) error {
	for _, existing := range s.leaveBalances[v.TenantID] {
		if existing.EmployeeID == v.EmployeeID && existing.LeaveTypeID == v.LeaveTypeID && existing.EntitlementYear == v.EntitlementYear {
			return nil
		}
	}
	v.Source = "nexus"
	v.RemainingMinutes = 0
	v.SnapshotRemainingMinutes = 0
	v.GrantedMinutes = 0
	v.UsedMinutes = 0
	putNested(s.leaveBalances, v.TenantID, v.ID, copyLeaveBalance(v))
	return nil
}

// GetLeaveBalance 從儲存層取得請假 balance。
func (s *Store) GetLeaveBalance(_ context.Context, tenantID, id string) (LeaveBalance, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.leaveBalances, tenantID, id)
	if !ok {
		return LeaveBalance{}, false, nil
	}
	return copyLeaveBalance(v), true, nil
}

func (s *Store) GetLeaveBalanceForOverlay(_ context.Context, tenantID, employeeID, leaveTypeID string, asOf time.Time) (LeaveBalance, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := s.leaveBalancesForOverlayLocked(tenantID, employeeID, leaveTypeID, asOf)
	if len(items) == 0 {
		return LeaveBalance{}, false, nil
	}
	return copyLeaveBalance(items[0]), true, nil
}

func (s *Store) ListLeaveBalancesForOverlay(_ context.Context, tenantID, employeeID, leaveTypeID string, asOf time.Time) ([]LeaveBalance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := s.leaveBalancesForOverlayLocked(tenantID, employeeID, leaveTypeID, asOf)
	out := make([]LeaveBalance, len(items))
	for index := range items {
		out[index] = copyLeaveBalance(items[index])
	}
	return out, nil
}

func (s *Store) leaveBalancesForOverlayLocked(tenantID, employeeID, leaveTypeID string, asOf time.Time) []LeaveBalance {
	items := make([]LeaveBalance, 0)
	entitlementYear := asOf.In(time.FixedZone("Asia/Shanghai", 8*60*60)).Year()
	for _, item := range s.leaveBalances[tenantID] {
		if item.EmployeeID != employeeID || item.LeaveTypeID != leaveTypeID || item.EntitlementYear != entitlementYear {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})
	return items
}

// ListLeaveBalances 從儲存層列出請假 balances。
func (s *Store) ListLeaveBalances(_ context.Context, tenantID string) ([]LeaveBalance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.leaveBalances[tenantID], copyLeaveBalance)
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.Before(out[j].UpdatedAt) })
	return out, nil
}

func (s *Store) AppendLeaveBalanceEntry(_ context.Context, v LeaveBalanceEntry) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.leaveBalanceEntries[v.TenantID] {
		if existing.IdempotencyKey == v.IdempotencyKey {
			return false, nil
		}
	}
	putNested(s.leaveBalanceEntries, v.TenantID, v.ID, copyLeaveBalanceEntry(v))
	return true, nil
}

func (s *Store) AppendStandaloneLeaveBalanceEntry(_ context.Context, v LeaveBalanceEntry) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.leaveBalanceEntries[v.TenantID] {
		if existing.IdempotencyKey == v.IdempotencyKey {
			return false, nil
		}
	}
	putNested(s.leaveBalanceEntries, v.TenantID, v.ID, copyLeaveBalanceEntry(v))
	return true, nil
}

func (s *Store) ListLeaveBalanceEntries(_ context.Context, tenantID string) ([]LeaveBalanceEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.leaveBalanceEntries[tenantID], copyLeaveBalanceEntry)
	sort.Slice(out, func(i, j int) bool {
		if out[i].OccurredAt.Equal(out[j].OccurredAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].OccurredAt.Before(out[j].OccurredAt)
	})
	return out, nil
}

func (s *Store) ListLeaveBalanceEntriesByBalance(_ context.Context, tenantID, balanceID string) ([]LeaveBalanceEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LeaveBalanceEntry, 0)
	for _, entry := range s.leaveBalanceEntries[tenantID] {
		if entry.BalanceID == balanceID {
			out = append(out, copyLeaveBalanceEntry(entry))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OccurredAt.Equal(out[j].OccurredAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].OccurredAt.Before(out[j].OccurredAt)
	})
	return out, nil
}

// UpsertLeaveRequest 從儲存層處理 upsert 請假請求。
func (s *Store) UpsertLeaveRequest(_ context.Context, v LeaveRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(v.LeaveTypeID) == "" {
		v.LeaveTypeID = domain.StableLeaveTypeID(v.LeaveType)
	}
	if strings.TrimSpace(v.ReconciliationStatus) == "" {
		v.ReconciliationStatus = "not_required"
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = v.CreatedAt
	}
	putNested(s.leaveRequests, v.TenantID, v.ID, copyLeaveRequest(v))
	return nil
}

func (s *Store) UpsertLeaveRecord(_ context.Context, v LeaveRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.leaveRecords, v.TenantID, v.ID, copyLeaveRecord(v))
	return nil
}

func (s *Store) GetLeaveRecord(_ context.Context, tenantID, id string) (LeaveRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.leaveRecords[tenantID][id]
	return copyLeaveRecord(item), ok, nil
}

func (s *Store) ListLeaveRecords(_ context.Context, tenantID string) ([]LeaveRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.leaveRecords[tenantID], copyLeaveRecord)
	sort.Slice(out, func(i, j int) bool {
		if out[i].EventDate.Equal(out[j].EventDate) {
			return out[i].ID < out[j].ID
		}
		return out[i].EventDate.Before(out[j].EventDate)
	})
	return out, nil
}

func (s *Store) ListActiveLeaveRecordsByQuery(_ context.Context, tenantID string, employeeIDs []string, fromAt, toAt time.Time) ([]LeaveRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	employeeSet := make(map[string]struct{}, len(employeeIDs))
	for _, employeeID := range employeeIDs {
		employeeSet[employeeID] = struct{}{}
	}
	out := make([]LeaveRecord, 0)
	for _, item := range s.leaveRecords[tenantID] {
		if item.Status != "active" || item.DeletedAt != nil || !item.StartAt.Before(toAt) || !item.EndAt.After(fromAt) {
			continue
		}
		if len(employeeSet) > 0 {
			if _, ok := employeeSet[item.EmployeeID]; !ok {
				continue
			}
		}
		if item.Source == "ehrms" && item.MatchedRecordID != "" {
			continue
		}
		out = append(out, copyLeaveRecord(item))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartAt.Equal(out[j].StartAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].StartAt.Before(out[j].StartAt)
	})
	return out, nil
}

// GetLeaveRequest 從儲存層取得請假請求。
func (s *Store) GetLeaveRequest(_ context.Context, tenantID, id string) (LeaveRequest, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.leaveRequests, tenantID, id)
	if !ok {
		return LeaveRequest{}, false, nil
	}
	return copyLeaveRequest(v), true, nil
}

// GetLeaveRequestByFormInstanceID 從儲存層取得請假請求 by 表單實例 ID。
func (s *Store) GetLeaveRequestByFormInstanceID(_ context.Context, tenantID, formInstanceID string) (LeaveRequest, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.leaveRequests[tenantID] {
		if item.FormInstanceID == formInstanceID {
			return copyLeaveRequest(item), true, nil
		}
	}
	return LeaveRequest{}, false, nil
}

// ListLeaveRequests 從儲存層列出請假請求。
func (s *Store) ListLeaveRequests(_ context.Context, tenantID string) ([]LeaveRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.leaveRequests[tenantID], copyLeaveRequest)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// ListLeaveRequestsByQuery 從儲存層列出請假請求 by 查詢。
func (s *Store) ListLeaveRequestsByQuery(ctx context.Context, tenantID string, query domain.LeaveRequestQuery) ([]LeaveRequest, error) {
	items, err := s.ListLeaveRequests(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]LeaveRequest, 0, len(items))
	for _, item := range items {
		if memoryLeaveRequestMatches(item, query) {
			out = append(out, item)
		}
	}
	return out, nil
}

// ListLeaveRequestPageByQuery 從儲存層列出請假請求分頁 by 查詢。
func (s *Store) ListLeaveRequestPageByQuery(ctx context.Context, tenantID string, query domain.LeaveRequestQuery, page PageRequest) ([]LeaveRequest, int, error) {
	items, err := s.ListLeaveRequestsByQuery(ctx, tenantID, query)
	if err != nil {
		return nil, 0, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		switch page.Sort {
		case "created_at_asc":
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		default:
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
	})
	page = utils.NormalizePageRequest(page)
	total := len(items)
	return paginateMemory(items, page.Page, page.PageSize), total, nil
}

// UpsertAttendanceWorksite 從儲存層處理 upsert 考勤工作地點。
func (s *Store) UpsertAttendanceWorksite(_ context.Context, v AttendanceWorksite) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.attendanceWorksites, v.TenantID, v.ID, copyAttendanceWorksite(v))
	return nil
}

// GetAttendanceWorksite 從儲存層取得考勤工作地點。
func (s *Store) GetAttendanceWorksite(_ context.Context, tenantID, id string) (AttendanceWorksite, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.attendanceWorksites, tenantID, id)
	if !ok {
		return AttendanceWorksite{}, false, nil
	}
	return copyAttendanceWorksite(v), true, nil
}

// ListAttendanceWorksites 從儲存層列出考勤 worksites。
func (s *Store) ListAttendanceWorksites(_ context.Context, tenantID string) ([]AttendanceWorksite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.attendanceWorksites[tenantID], copyAttendanceWorksite)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// UpsertAttendanceClockRecord 從儲存層處理 upsert 考勤打卡 record。
func (s *Store) UpsertAttendanceClockRecord(_ context.Context, v AttendanceClockRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(v.ClientEventID) != "" {
		for _, item := range s.attendanceClockRecords[v.TenantID] {
			if item.ID != v.ID && item.ClientEventID == v.ClientEventID {
				return domain.Conflict("attendance clock client event already exists")
			}
		}
	}
	putNested(s.attendanceClockRecords, v.TenantID, v.ID, copyAttendanceClockRecord(v))
	return nil
}

// GetAttendanceClockRecordByClientEventID 依客戶端事件識別碼取得考勤打卡 record。
func (s *Store) GetAttendanceClockRecordByClientEventID(_ context.Context, tenantID, clientEventID string) (AttendanceClockRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.attendanceClockRecords[tenantID] {
		if clientEventID != "" && item.ClientEventID == clientEventID {
			return copyAttendanceClockRecord(item), true, nil
		}
	}
	return AttendanceClockRecord{}, false, nil
}

// GetEarliestAcceptedAttendanceClockIn 取得未作廢的最早 accepted 上班卡。
func (s *Store) GetEarliestAcceptedAttendanceClockIn(_ context.Context, tenantID, employeeID, workDate string) (AttendanceClockRecord, bool, error) {
	return s.getAcceptedAttendanceClockBoundary(tenantID, employeeID, workDate, "clock_in", false)
}

// GetLatestAcceptedAttendanceClockOut 取得未作廢的最晚 accepted 下班卡。
func (s *Store) GetLatestAcceptedAttendanceClockOut(_ context.Context, tenantID, employeeID, workDate string) (AttendanceClockRecord, bool, error) {
	return s.getAcceptedAttendanceClockBoundary(tenantID, employeeID, workDate, "clock_out", true)
}

// GetLatestAcceptedAttendanceClockRecord 取得未作廢的當日最新 accepted 打卡。
func (s *Store) GetLatestAcceptedAttendanceClockRecord(_ context.Context, tenantID, employeeID, workDate string) (AttendanceClockRecord, bool, error) {
	return s.getAcceptedAttendanceClockBoundary(tenantID, employeeID, workDate, "", true)
}

// getAcceptedAttendanceClockBoundary 依穩定排序取得 accepted 打卡邊界。
func (s *Store) getAcceptedAttendanceClockBoundary(tenantID, employeeID, workDate, direction string, latest bool) (AttendanceClockRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var boundary AttendanceClockRecord
	found := false
	for _, item := range s.attendanceClockRecords[tenantID] {
		if item.EmployeeID != employeeID || item.WorkDate != workDate || item.RecordStatus != "accepted" || item.Voided {
			continue
		}
		if direction != "" && item.Direction != direction {
			continue
		}
		if !found || attendanceClockRecordComesBefore(item, boundary) != latest {
			boundary = item
			found = true
		}
	}
	if !found {
		return AttendanceClockRecord{}, false, nil
	}
	return copyAttendanceClockRecord(boundary), true, nil
}

// attendanceClockRecordComesBefore 比較打卡時間並以建立時間及 ID 穩定決勝。
func attendanceClockRecordComesBefore(left, right AttendanceClockRecord) bool {
	if !left.ClockedAt.Equal(right.ClockedAt) {
		return left.ClockedAt.Before(right.ClockedAt)
	}
	if !left.CreatedAt.Equal(right.CreatedAt) {
		return left.CreatedAt.Before(right.CreatedAt)
	}
	return left.ID < right.ID
}

// ListAttendanceClockRecords 從儲存層列出考勤打卡 records。
func (s *Store) ListAttendanceClockRecords(_ context.Context, tenantID string, query domain.AttendanceClockRecordQuery) ([]AttendanceClockRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AttendanceClockRecord, 0, len(s.attendanceClockRecords[tenantID]))
	for _, item := range s.attendanceClockRecords[tenantID] {
		if !memoryClockRecordMatches(item, query) {
			continue
		}
		out = append(out, copyAttendanceClockRecord(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ClockedAt.After(out[j].ClockedAt) })
	return out, nil
}

// UpsertAttendanceDailySummary 從儲存層處理 upsert 考勤日彙總。
func (s *Store) UpsertAttendanceDailySummary(_ context.Context, v AttendanceDailySummary) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := attendanceDailySummaryKey(v.EmployeeID, v.WorkDate)
	for existingKey, item := range s.attendanceSummaries[v.TenantID] {
		if existingKey != key && item.EmployeeID == v.EmployeeID && item.WorkDate == v.WorkDate {
			return domain.Conflict("attendance daily summary already exists")
		}
		if existingKey != key && v.ExternalRef != "" && item.ExternalRef == v.ExternalRef {
			return domain.Conflict("attendance daily summary external_ref already exists")
		}
	}
	putNested(s.attendanceSummaries, v.TenantID, key, copyAttendanceDailySummary(v))
	return nil
}

func attendanceDailySummaryKey(employeeID, workDate string) string {
	return strings.TrimSpace(employeeID) + "|" + strings.TrimSpace(workDate)
}

// GetAttendanceDailySummaryByExternalRef 從儲存層取得考勤日彙總 by external ref。
func (s *Store) GetAttendanceDailySummaryByExternalRef(_ context.Context, tenantID, externalRef string) (AttendanceDailySummary, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.attendanceSummaries[tenantID] {
		if item.ExternalRef == externalRef {
			return copyAttendanceDailySummary(item), true, nil
		}
	}
	return AttendanceDailySummary{}, false, nil
}

// GetAttendanceDailySummaryByEmployeeDate 從儲存層取得考勤日彙總 by 員工日期。
func (s *Store) GetAttendanceDailySummaryByEmployeeDate(_ context.Context, tenantID, employeeID, workDate string) (AttendanceDailySummary, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.attendanceSummaries[tenantID] {
		if item.EmployeeID == employeeID && item.WorkDate == workDate {
			return copyAttendanceDailySummary(item), true, nil
		}
	}
	return AttendanceDailySummary{}, false, nil
}

// ListAttendanceDailySummaries 從儲存層列出考勤日彙總。
func (s *Store) ListAttendanceDailySummaries(_ context.Context, tenantID string, query domain.AttendanceDailySummaryQuery) ([]AttendanceDailySummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AttendanceDailySummary, 0, len(s.attendanceSummaries[tenantID]))
	for _, item := range s.attendanceSummaries[tenantID] {
		if !memoryAttendanceDailySummaryMatches(item, query) {
			continue
		}
		out = append(out, copyAttendanceDailySummary(item))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].WorkDate != out[j].WorkDate {
			return out[i].WorkDate < out[j].WorkDate
		}
		return out[i].EmployeeID < out[j].EmployeeID
	})
	return out, nil
}

func attendanceDayProjectionKey(employeeID, workDate string) string {
	return strings.TrimSpace(employeeID) + "|" + strings.TrimSpace(workDate)
}

func (s *Store) UpsertAttendanceDayProjection(_ context.Context, v AttendanceDayProjection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := attendanceDayProjectionKey(v.EmployeeID, v.WorkDate)
	putNested(s.attendanceDayProjections, v.TenantID, key, copyAttendanceDayProjection(v))
	return nil
}

func (s *Store) GetAttendanceDayProjection(_ context.Context, tenantID, employeeID, workDate string) (AttendanceDayProjection, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := getNested(s.attendanceDayProjections, tenantID, attendanceDayProjectionKey(employeeID, workDate))
	if !ok {
		return AttendanceDayProjection{}, false, nil
	}
	return copyAttendanceDayProjection(item), true, nil
}

func (s *Store) ListAttendanceDayProjections(_ context.Context, tenantID string, employeeIDs []string, fromDate, toDate string) ([]AttendanceDayProjection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	employeeSet := make(map[string]struct{}, len(employeeIDs))
	for _, employeeID := range employeeIDs {
		employeeSet[employeeID] = struct{}{}
	}
	out := make([]AttendanceDayProjection, 0)
	for _, item := range s.attendanceDayProjections[tenantID] {
		if len(employeeSet) > 0 {
			if _, ok := employeeSet[item.EmployeeID]; !ok {
				continue
			}
		}
		if fromDate != "" && item.WorkDate < fromDate || toDate != "" && item.WorkDate > toDate {
			continue
		}
		out = append(out, copyAttendanceDayProjection(item))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].WorkDate == out[j].WorkDate {
			return out[i].EmployeeID < out[j].EmployeeID
		}
		return out[i].WorkDate < out[j].WorkDate
	})
	return out, nil
}

// UpsertAttendanceCorrectionRequest 從儲存層處理 upsert 考勤 correction 請求。
func (s *Store) UpsertAttendanceCorrectionRequest(_ context.Context, v AttendanceCorrectionRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(v.CorrectionType) == "" {
		v.CorrectionType = "add_record"
	}
	putNested(s.attendanceCorrections, v.TenantID, v.ID, copyAttendanceCorrectionRequest(v))
	return nil
}

// GetAttendanceCorrectionRequest 從儲存層取得考勤 correction 請求。
func (s *Store) GetAttendanceCorrectionRequest(_ context.Context, tenantID, id string) (AttendanceCorrectionRequest, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.attendanceCorrections, tenantID, id)
	if !ok {
		return AttendanceCorrectionRequest{}, false, nil
	}
	return copyAttendanceCorrectionRequest(v), true, nil
}

// The in-memory transaction already serializes its clone-and-swap boundary, so
// this method has the same observable locking contract as the Postgres variant.
func (s *Store) GetAttendanceCorrectionRequestForUpdate(ctx context.Context, tenantID, id string) (AttendanceCorrectionRequest, bool, error) {
	return s.GetAttendanceCorrectionRequest(ctx, tenantID, id)
}

func (s *Store) ClaimAttendanceCorrectionReview(_ context.Context, tenantID, formInstanceID, reviewerID string, claimedAt time.Time) (AttendanceCorrectionRequest, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, item := range s.attendanceCorrections[tenantID] {
		if item.FormInstanceID != formInstanceID {
			continue
		}
		if item.Status != "pending" {
			return copyAttendanceCorrectionRequest(item), false, nil
		}
		item.Status = "reviewing"
		item.ReviewedByAccountID = reviewerID
		item.UpdatedAt = claimedAt
		putNested(s.attendanceCorrections, tenantID, id, copyAttendanceCorrectionRequest(item))
		return copyAttendanceCorrectionRequest(item), true, nil
	}
	return AttendanceCorrectionRequest{}, false, nil
}

// ListAttendanceCorrectionRequests 從儲存層列出考勤 correction 請求。
func (s *Store) ListAttendanceCorrectionRequests(_ context.Context, tenantID string, query domain.AttendanceCorrectionQuery) ([]AttendanceCorrectionRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AttendanceCorrectionRequest, 0, len(s.attendanceCorrections[tenantID]))
	for _, item := range s.attendanceCorrections[tenantID] {
		if !memoryCorrectionMatches(item, query) {
			continue
		}
		out = append(out, copyAttendanceCorrectionRequest(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// GetAttendanceCorrectionRequestByFormInstanceID 從儲存層取得考勤 correction 請求 by 表單實例 ID。
func (s *Store) GetAttendanceCorrectionRequestByFormInstanceID(_ context.Context, tenantID, formInstanceID string) (AttendanceCorrectionRequest, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.attendanceCorrections[tenantID] {
		if item.FormInstanceID == formInstanceID {
			return copyAttendanceCorrectionRequest(item), true, nil
		}
	}
	return AttendanceCorrectionRequest{}, false, nil
}

// UpsertOvertimeRequest 從儲存層處理 upsert 加班申請。
func (s *Store) UpsertOvertimeRequest(_ context.Context, v OvertimeRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.overtimeRequests, v.TenantID, v.ID, copyOvertimeRequest(v))
	return nil
}

// GetOvertimeRequest 從儲存層取得加班申請。
func (s *Store) GetOvertimeRequest(_ context.Context, tenantID, id string) (OvertimeRequest, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.overtimeRequests, tenantID, id)
	if !ok {
		return OvertimeRequest{}, false, nil
	}
	return copyOvertimeRequest(v), true, nil
}

// GetOvertimeRequestByFormInstanceID 從儲存層取得加班申請 by 表單實例 ID。
func (s *Store) GetOvertimeRequestByFormInstanceID(_ context.Context, tenantID, formInstanceID string) (OvertimeRequest, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.overtimeRequests[tenantID] {
		if item.FormInstanceID == formInstanceID {
			return copyOvertimeRequest(item), true, nil
		}
	}
	return OvertimeRequest{}, false, nil
}

// ListOvertimeRequestsByQuery 從儲存層列出加班申請 by 查詢。
func (s *Store) ListOvertimeRequestsByQuery(_ context.Context, tenantID string, query domain.OvertimeRequestQuery) ([]OvertimeRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]OvertimeRequest, 0, len(s.overtimeRequests[tenantID]))
	for _, item := range s.overtimeRequests[tenantID] {
		if !memoryOvertimeRequestMatches(item, query) {
			continue
		}
		out = append(out, copyOvertimeRequest(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// memoryOvertimeRequestMatches 處理 memory 加班申請 matches。
func memoryOvertimeRequestMatches(item OvertimeRequest, query domain.OvertimeRequestQuery) bool {
	if len(query.EmployeeIDs) > 0 {
		allowed := map[string]struct{}{}
		for _, id := range query.EmployeeIDs {
			allowed[id] = struct{}{}
		}
		if _, ok := allowed[item.EmployeeID]; !ok {
			return false
		}
	}
	if query.Status != "" && !strings.EqualFold(item.Status, query.Status) {
		return false
	}
	if query.FromDate != "" {
		if from, err := time.Parse(time.DateOnly, query.FromDate); err == nil && item.EndAt.Before(from) {
			return false
		}
	}
	if query.ToDate != "" {
		if to, err := time.Parse(time.DateOnly, query.ToDate); err == nil && item.StartAt.After(to.AddDate(0, 0, 1)) {
			return false
		}
	}
	return true
}

// memoryClockRecordMatches 處理 memory 打卡 record matches。
func memoryClockRecordMatches(item AttendanceClockRecord, query domain.AttendanceClockRecordQuery) bool {
	if query.EmployeeID != "" && item.EmployeeID != query.EmployeeID {
		return false
	}
	if len(query.EmployeeIDs) > 0 {
		allowed := memoryStringSet(query.EmployeeIDs)
		if _, ok := allowed[item.EmployeeID]; !ok {
			return false
		}
	}
	if query.FromDate != "" && item.WorkDate < query.FromDate {
		return false
	}
	if query.ToDate != "" && item.WorkDate > query.ToDate {
		return false
	}
	if query.Direction != "" && item.Direction != query.Direction {
		return false
	}
	if query.RecordStatus != "" && item.RecordStatus != query.RecordStatus {
		return false
	}
	if query.Source != "" && item.Source != query.Source {
		return false
	}
	return true
}

// memoryAttendanceDailySummaryMatches 處理 memory 考勤日彙總 matches。
func memoryAttendanceDailySummaryMatches(item AttendanceDailySummary, query domain.AttendanceDailySummaryQuery) bool {
	if query.EmployeeID != "" && item.EmployeeID != query.EmployeeID {
		return false
	}
	if len(query.EmployeeIDs) > 0 {
		allowed := memoryStringSet(query.EmployeeIDs)
		if _, ok := allowed[item.EmployeeID]; !ok {
			return false
		}
	}
	if query.FromDate != "" && item.WorkDate < query.FromDate {
		return false
	}
	if query.ToDate != "" && item.WorkDate > query.ToDate {
		return false
	}
	if query.Source != "" && item.Source != query.Source {
		return false
	}
	return true
}

// memoryCorrectionMatches 處理 memory correction matches。
func memoryCorrectionMatches(item AttendanceCorrectionRequest, query domain.AttendanceCorrectionQuery) bool {
	if query.EmployeeID != "" && item.EmployeeID != query.EmployeeID {
		return false
	}
	if query.FromDate != "" && item.WorkDate < query.FromDate {
		return false
	}
	if query.ToDate != "" && item.WorkDate > query.ToDate {
		return false
	}
	if query.Status != "" && item.Status != query.Status {
		return false
	}
	if query.Direction != "" && item.Direction != query.Direction {
		return false
	}
	return true
}

// UpsertFormDefinitionDraft 保存表單定義草稿並執行 revision 樂觀鎖。
func (s *Store) UpsertFormDefinitionDraft(_ context.Context, v domain.FormDefinitionDraft) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := getNested(s.formDefinitionDrafts, v.TenantID, v.ID)
	if ok {
		if v.Revision > 0 && existing.Revision != v.Revision {
			return domain.Conflict("form definition draft was modified concurrently")
		}
		v.Revision = existing.Revision + 1
	} else if v.Revision <= 0 {
		v.Revision = 1
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC()
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = v.CreatedAt
	}
	putNested(s.formDefinitionDrafts, v.TenantID, v.ID, copyFormDefinitionDraft(v))
	return nil
}

// GetFormDefinitionDraft 取得租戶內的表單定義草稿。
func (s *Store) GetFormDefinitionDraft(_ context.Context, tenantID, id string) (domain.FormDefinitionDraft, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.formDefinitionDrafts, tenantID, id)
	if !ok {
		return domain.FormDefinitionDraft{}, false, nil
	}
	return copyFormDefinitionDraft(v), true, nil
}

// GetFormDefinitionDraftByAgentCall 以 Agent run/tool call 實現冪等重試。
func (s *Store) GetFormDefinitionDraftByAgentCall(_ context.Context, tenantID, agentRunID, toolCallID string) (domain.FormDefinitionDraft, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.formDefinitionDrafts[tenantID] {
		if agentRunID != "" && toolCallID != "" && v.AgentRunID == agentRunID && v.ToolCallID == toolCallID {
			return copyFormDefinitionDraft(v), true, nil
		}
	}
	return domain.FormDefinitionDraft{}, false, nil
}

// ListFormDefinitionDrafts 列出指定擁有者與狀態的草稿。
func (s *Store) ListFormDefinitionDrafts(_ context.Context, tenantID, ownerAccountID, status string) ([]domain.FormDefinitionDraft, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]domain.FormDefinitionDraft, 0)
	for _, v := range s.formDefinitionDrafts[tenantID] {
		if ownerAccountID != "" && v.OwnerAccountID != ownerAccountID {
			continue
		}
		if status != "" && string(v.Status) != status {
			continue
		}
		items = append(items, copyFormDefinitionDraft(v))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items, nil
}

// UpsertFormTemplate 從儲存層處理 upsert 表單範本。
func (s *Store) UpsertFormTemplate(_ context.Context, v FormTemplate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(v.Status) == "" {
		v.Status = "published"
	}
	if v.CurrentVersion <= 0 {
		v.CurrentVersion = 1
	}
	if v.Status == "published" {
		v.PublishedVersion = v.CurrentVersion
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC()
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = v.CreatedAt
	}
	putNested(s.formTemplates, v.TenantID, v.ID, copyFormTemplate(v))
	versionKey := fmt.Sprintf("%s:%d", v.ID, v.CurrentVersion)
	versionStatus := formTemplateVersionStatus(v)
	if existing, exists := getNested(s.formTemplateVersions, v.TenantID, versionKey); !exists {
		version := FormTemplateVersion{
			ID: utils.NewID("ftv"), TenantID: v.TenantID, TemplateID: v.ID, Version: v.CurrentVersion,
			Schema: utils.CopyStringMap(v.Schema), Status: versionStatus, CreatedAt: v.UpdatedAt,
		}
		if versionStatus == "published" {
			publishedAt := v.UpdatedAt
			version.PublishedAt = &publishedAt
		}
		putNested(s.formTemplateVersions, v.TenantID, versionKey, version)
	} else if versionStatus == "published" && existing.Status != "published" {
		publishedAt := v.UpdatedAt
		existing.Status = "published"
		existing.PublishedAt = &publishedAt
		putNested(s.formTemplateVersions, v.TenantID, versionKey, existing)
	}
	return nil
}

func formTemplateVersionStatus(template FormTemplate) string {
	if template.Status == "archived" {
		return "archived"
	}
	if template.Status == "published" && template.PublishedVersion == template.CurrentVersion {
		return "published"
	}
	return "draft"
}

// GetFormTemplate 從儲存層取得表單範本。
func (s *Store) GetFormTemplate(_ context.Context, tenantID, id string) (FormTemplate, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.formTemplates, tenantID, id)
	if !ok {
		return FormTemplate{}, false, nil
	}
	return copyFormTemplate(v), true, nil
}

// GetFormTemplateForUpdate mirrors the row-locking contract; memory transactions already serialize writers.
func (s *Store) GetFormTemplateForUpdate(ctx context.Context, tenantID, id string) (FormTemplate, bool, error) {
	return s.GetFormTemplate(ctx, tenantID, id)
}

// GetFormTemplateByKey 從儲存層取得表單範本 by key。
func (s *Store) GetFormTemplateByKey(_ context.Context, tenantID, key string) (FormTemplate, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	bucket := s.formTemplates[tenantID]
	for _, v := range bucket {
		if v.Key == key {
			return copyFormTemplate(v), true, nil
		}
	}
	return FormTemplate{}, false, nil
}

// GetFormTemplateByKeyForUpdate mirrors the row-locking contract; memory transactions already serialize writers.
func (s *Store) GetFormTemplateByKeyForUpdate(ctx context.Context, tenantID, key string) (FormTemplate, bool, error) {
	return s.GetFormTemplateByKey(ctx, tenantID, key)
}

// ListFormTemplates 從儲存層列出表單範本。
func (s *Store) ListFormTemplates(_ context.Context, tenantID string) ([]FormTemplate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.formTemplates[tenantID], copyFormTemplate)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// InsertFormTemplateVersion 寫入不可變表單版本。
func (s *Store) InsertFormTemplateVersion(_ context.Context, v FormTemplateVersion) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%s:%d", v.TemplateID, v.Version)
	if existing, exists := getNested(s.formTemplateVersions, v.TenantID, key); exists {
		if v.Status == "published" && existing.Status != "published" {
			existing.Status = "published"
			if existing.PublishedAt == nil {
				existing.PublishedAt = v.PublishedAt
			}
			putNested(s.formTemplateVersions, v.TenantID, key, existing)
		}
		return nil
	}
	putNested(s.formTemplateVersions, v.TenantID, key, copyFormTemplateVersion(v))
	return nil
}

// GetFormTemplateVersion 依版本 ID 取得不可變快照。
func (s *Store) GetFormTemplateVersion(_ context.Context, tenantID, id string) (FormTemplateVersion, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, version := range s.formTemplateVersions[tenantID] {
		if version.ID == id {
			return copyFormTemplateVersion(version), true, nil
		}
	}
	return FormTemplateVersion{}, false, nil
}

// GetFormTemplateVersionByNumber 依模板與版本號取得不可變快照。
func (s *Store) GetFormTemplateVersionByNumber(_ context.Context, tenantID, templateID string, version int) (FormTemplateVersion, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.formTemplateVersions, tenantID, fmt.Sprintf("%s:%d", templateID, version))
	if !ok {
		return FormTemplateVersion{}, false, nil
	}
	return copyFormTemplateVersion(v), true, nil
}

// UpsertFormInstance 從儲存層處理 upsert 表單實例。Version > 0 時執行樂觀鎖檢查。
func (s *Store) UpsertFormInstance(_ context.Context, v FormInstance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(v.TemplateVersionID) == "" {
		template, ok := getNested(s.formTemplates, v.TenantID, v.TemplateID)
		if ok {
			versionNumber := template.PublishedVersion
			if versionNumber <= 0 {
				versionNumber = template.CurrentVersion
			}
			version, versionExists := getNested(s.formTemplateVersions, v.TenantID, fmt.Sprintf("%s:%d", v.TemplateID, versionNumber))
			if !versionExists {
				return fmt.Errorf("form template version %s:%d not found", v.TemplateID, versionNumber)
			}
			v.TemplateVersionID = version.ID
		}
	}
	existing, ok := getNested(s.formInstances, v.TenantID, v.ID)
	if ok {
		if v.Version > 0 && existing.Version != v.Version {
			return domain.Conflict("form instance was modified concurrently")
		}
		v.Version = existing.Version + 1
	} else {
		v.Version = 1
	}
	putNested(s.formInstances, v.TenantID, v.ID, copyFormInstance(v))
	return nil
}

// GetFormInstance 從儲存層取得表單實例。
func (s *Store) GetFormInstance(_ context.Context, tenantID, id string) (FormInstance, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.formInstances, tenantID, id)
	if !ok {
		return FormInstance{}, false, nil
	}
	return copyFormInstance(v), true, nil
}

// ListFormInstances 從儲存層列出表單實例。
func (s *Store) ListFormInstances(_ context.Context, tenantID string) ([]FormInstance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.formInstances[tenantID], copyFormInstance)
	sort.Slice(out, func(i, j int) bool { return out[i].SubmittedAt.Before(out[j].SubmittedAt) })
	return out, nil
}

// ListFormInstancesByQuery 從儲存層列出表單實例 by 查詢。
func (s *Store) ListFormInstancesByQuery(ctx context.Context, tenantID string, query domain.FormInstanceQuery) ([]FormInstance, error) {
	items, err := s.ListFormInstances(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	templateKeys := map[string]string{}
	templateNames := map[string]string{}
	applicantNames := map[string]string{}
	search := strings.TrimSpace(query.Search)
	if strings.TrimSpace(query.TemplateKey) != "" || search != "" {
		templates, err := s.ListFormTemplates(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		for _, template := range templates {
			templateKeys[template.ID] = template.Key
			templateNames[template.ID] = template.Name
		}
	}
	if search != "" {
		accounts, err := s.ListAccounts(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		for _, account := range accounts {
			applicantNames[account.ID] = account.DisplayName
		}
	}
	out := make([]FormInstance, 0, len(items))
	for _, item := range items {
		if memoryFormInstanceMatches(item, templateKeys[item.TemplateID], templateNames[item.TemplateID], applicantNames[item.ApplicantAccountID], query) {
			out = append(out, item)
		}
	}
	return out, nil
}

// ListFormInstancePageByQuery 從儲存層列出表單實例分頁 by 查詢。
func (s *Store) ListFormInstancePageByQuery(ctx context.Context, tenantID string, query domain.FormInstanceQuery, page PageRequest) ([]FormInstance, int, error) {
	items, err := s.ListFormInstancesByQuery(ctx, tenantID, query)
	if err != nil {
		return nil, 0, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		switch page.Sort {
		case "submitted_at_asc":
			return items[i].SubmittedAt.Before(items[j].SubmittedAt)
		default:
			return items[i].SubmittedAt.After(items[j].SubmittedAt)
		}
	})
	page = utils.NormalizePageRequest(page)
	total := len(items)
	return paginateMemory(items, page.Page, page.PageSize), total, nil
}

// ReplaceFormInstanceFieldValues 替換單一表單實例的可統計欄位投影。
func (s *Store) ReplaceFormInstanceFieldValues(_ context.Context, tenantID, formInstanceID string, values []FormInstanceFieldValue) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.formInstanceFieldValues, tenantID, formInstanceID, copyFormInstanceFieldValues(values))
	return nil
}

// ListFormInstanceFieldValues 列出單一表單實例的欄位投影。
func (s *Store) ListFormInstanceFieldValues(_ context.Context, tenantID, formInstanceID string) ([]FormInstanceFieldValue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	values, ok := getNested(s.formInstanceFieldValues, tenantID, formInstanceID)
	if !ok {
		return []FormInstanceFieldValue{}, nil
	}
	return copyFormInstanceFieldValues(values), nil
}

// DeleteFormInstance 從儲存層刪除表單實例。
func (s *Store) DeleteFormInstance(_ context.Context, tenantID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.formInstances[tenantID], id)
	delete(s.formInstanceFieldValues[tenantID], id)
	for fileID, file := range s.formInstanceFiles[tenantID] {
		if file.FormInstanceID == id {
			delete(s.formInstanceFiles[tenantID], fileID)
		}
	}
	return nil
}

// UpsertFormFileAsset persists form attachment metadata.
func (s *Store) UpsertFormFileAsset(_ context.Context, file domain.FormInstanceFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.formInstanceFiles, file.TenantID, file.ID, copyFormInstanceFile(file))
	return nil
}

// InsertFormInstanceFile records the form instance and field binding.
func (s *Store) InsertFormInstanceFile(_ context.Context, file domain.FormInstanceFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.formInstanceFiles, file.TenantID, file.ID, copyFormInstanceFile(file))
	return nil
}

// GetFormInstanceFile resolves one attachment for a form instance.
func (s *Store) GetFormInstanceFile(_ context.Context, tenantID, formInstanceID, fileID string) (domain.FormInstanceFile, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	file, ok := getNested(s.formInstanceFiles, tenantID, fileID)
	if !ok || file.FormInstanceID != formInstanceID {
		return domain.FormInstanceFile{}, false, nil
	}
	return copyFormInstanceFile(file), true, nil
}

// ListFormInstanceFiles lists attachments for a form instance.
func (s *Store) ListFormInstanceFiles(_ context.Context, tenantID, formInstanceID string) ([]domain.FormInstanceFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.FormInstanceFile, 0)
	for _, file := range s.formInstanceFiles[tenantID] {
		if file.FormInstanceID == formInstanceID {
			out = append(out, copyFormInstanceFile(file))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

// ListFormInstanceFilesByField lists attachments for one field.
func (s *Store) ListFormInstanceFilesByField(_ context.Context, tenantID, formInstanceID, fieldID string) ([]domain.FormInstanceFile, error) {
	items, err := s.ListFormInstanceFiles(context.Background(), tenantID, formInstanceID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.FormInstanceFile, 0, len(items))
	for _, file := range items {
		if file.FieldID == fieldID {
			out = append(out, file)
		}
	}
	return out, nil
}

// CountFormInstanceFilesByField returns how many files are bound to a field.
func (s *Store) CountFormInstanceFilesByField(ctx context.Context, tenantID, formInstanceID, fieldID string) (int, error) {
	items, err := s.ListFormInstanceFilesByField(ctx, tenantID, formInstanceID, fieldID)
	if err != nil {
		return 0, err
	}
	return len(items), nil
}

// MarkFormInstanceFilesAttached promotes draft attachments after form submit.
func (s *Store) MarkFormInstanceFilesAttached(_ context.Context, tenantID, formInstanceID string, updatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for fileID, file := range s.formInstanceFiles[tenantID] {
		if file.FormInstanceID != formInstanceID || file.State != "draft" {
			continue
		}
		file.State = "attached"
		file.UpdatedAt = updatedAt
		putNested(s.formInstanceFiles, tenantID, fileID, copyFormInstanceFile(file))
	}
	return nil
}

// DeleteDraftFormInstanceFile removes only an unattached draft binding.
func (s *Store) DeleteDraftFormInstanceFile(_ context.Context, tenantID, formInstanceID, fileID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, ok := getNested(s.formInstanceFiles, tenantID, fileID)
	if !ok || file.FormInstanceID != formInstanceID || file.State != "draft" {
		return false, nil
	}
	delete(s.formInstanceFiles[tenantID], fileID)
	return true, nil
}

// DeleteFormFileAsset removes in-memory form attachment metadata.
func (s *Store) DeleteFormFileAsset(_ context.Context, tenantID, fileID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.formInstanceFiles[tenantID], fileID)
	return nil
}

// UpsertPlatformTaskItem 從儲存層處理 upsert 平臺任務項目。
func (s *Store) UpsertPlatformTaskItem(_ context.Context, v PlatformTaskRecordItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := getNested(s.platformTaskItems, v.TenantID, v.ID); ok && current.AccountID != v.AccountID {
		return domain.Conflict("platform task item belongs to another account")
	}
	putNested(s.platformTaskItems, v.TenantID, v.ID, copyPlatformTaskRecordItem(v))
	return nil
}

// GetPlatformTaskItem 從儲存層取得平臺任務項目。
func (s *Store) GetPlatformTaskItem(_ context.Context, tenantID, accountID, id string) (PlatformTaskRecordItem, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.platformTaskItems, tenantID, id)
	if !ok || v.AccountID != accountID {
		return PlatformTaskRecordItem{}, false, nil
	}
	return copyPlatformTaskRecordItem(v), true, nil
}

// ListPlatformTaskItems 從儲存層列出平臺任務項目。
func (s *Store) ListPlatformTaskItems(_ context.Context, tenantID, accountID string, query domain.PlatformTasksQuery) ([]PlatformTaskRecordItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PlatformTaskRecordItem, 0)
	for _, v := range s.platformTaskItems[tenantID] {
		if v.AccountID != accountID {
			continue
		}
		if !memoryPlatformTaskWithinWindow(v.CreatedAt, query) {
			continue
		}
		if query.HasCursor && !memoryPlatformTaskAfterCursor(v.CreatedAt, v.ID, query) {
			continue
		}
		out = append(out, copyPlatformTaskRecordItem(v))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if query.PageSize > 0 && len(out) > query.PageSize {
		out = out[:query.PageSize]
	}
	return out, nil
}

// DeletePlatformTaskItem 從儲存層刪除平臺任務項目。
func (s *Store) DeletePlatformTaskItem(_ context.Context, tenantID, accountID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := getNested(s.platformTaskItems, tenantID, id); ok && current.AccountID == accountID {
		delete(s.platformTaskItems[tenantID], id)
	}
	return nil
}

// UpsertPlatformTaskTodo 從儲存層處理 upsert 平臺任務待辦。
func (s *Store) UpsertPlatformTaskTodo(_ context.Context, v PlatformTaskTodoRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := getNested(s.platformTaskTodos, v.TenantID, v.ID); ok && current.AccountID != v.AccountID {
		return domain.Conflict("platform task todo belongs to another account")
	}
	putNested(s.platformTaskTodos, v.TenantID, v.ID, copyPlatformTaskTodoRecord(v))
	return nil
}

// GetPlatformTaskTodo 從儲存層取得平臺任務待辦。
func (s *Store) GetPlatformTaskTodo(_ context.Context, tenantID, accountID, id string) (PlatformTaskTodoRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.platformTaskTodos, tenantID, id)
	if !ok || v.AccountID != accountID {
		return PlatformTaskTodoRecord{}, false, nil
	}
	return copyPlatformTaskTodoRecord(v), true, nil
}

// ListPlatformTaskTodos 從儲存層列出平臺任務待辦。
func (s *Store) ListPlatformTaskTodos(_ context.Context, tenantID, accountID string, query domain.PlatformTasksQuery) ([]PlatformTaskTodoRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PlatformTaskTodoRecord, 0)
	for _, v := range s.platformTaskTodos[tenantID] {
		if v.AccountID != accountID {
			continue
		}
		if !memoryPlatformTaskWithinWindow(v.CreatedAt, query) {
			continue
		}
		if query.HasCursor && !memoryPlatformTaskAfterCursor(v.CreatedAt, v.ID, query) {
			continue
		}
		out = append(out, copyPlatformTaskTodoRecord(v))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if query.PageSize > 0 && len(out) > query.PageSize {
		out = out[:query.PageSize]
	}
	return out, nil
}

// memoryPlatformTaskWithinWindow 判斷 created_at 是否落在查詢時間窗 [from, to) 內。
func memoryPlatformTaskWithinWindow(createdAt time.Time, query domain.PlatformTasksQuery) bool {
	if !query.From.IsZero() && createdAt.Before(query.From) {
		return false
	}
	if !query.To.IsZero() && !createdAt.Before(query.To) {
		return false
	}
	return true
}

// memoryPlatformTaskAfterCursor 判斷 (created_at, id) 是否落在遊標之後（倒序 keyset）。
func memoryPlatformTaskAfterCursor(createdAt time.Time, id string, query domain.PlatformTasksQuery) bool {
	return createdAt.Before(query.CursorCreatedAt) || (createdAt.Equal(query.CursorCreatedAt) && id < query.CursorID)
}

// DeletePlatformTaskTodo 從儲存層刪除平臺任務待辦。
func (s *Store) DeletePlatformTaskTodo(_ context.Context, tenantID, accountID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := getNested(s.platformTaskTodos, tenantID, id); ok && current.AccountID == accountID {
		delete(s.platformTaskTodos[tenantID], id)
	}
	return nil
}

// UpsertAgentRun 從儲存層處理 upsert agent 執行。
func (s *Store) UpsertAgentRun(_ context.Context, v AgentRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.agentRuns, v.TenantID, v.ID, copyAgentRun(v))
	return nil
}

// ListAgentRuns 從儲存層列出 agent 執行紀錄。
func (s *Store) ListAgentRuns(_ context.Context, tenantID string) ([]AgentRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.agentRuns[tenantID], copyAgentRun)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// ListAgentRunsByAccount 從儲存層列出 agent 執行紀錄 by 帳號。
func (s *Store) ListAgentRunsByAccount(ctx context.Context, tenantID, accountID string) ([]AgentRun, error) {
	items, err := s.ListAgentRuns(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]AgentRun, 0, len(items))
	for _, item := range items {
		if item.AccountID == accountID {
			out = append(out, item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// ListAgentRunPage 從儲存層列出 agent 執行分頁。
func (s *Store) ListAgentRunPage(ctx context.Context, tenantID string, page PageRequest) ([]AgentRun, int, error) {
	items, err := s.ListAgentRuns(ctx, tenantID)
	if err != nil {
		return nil, 0, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		switch page.Sort {
		case "created_at_asc":
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		default:
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
	})
	page = utils.NormalizePageRequest(page)
	total := len(items)
	return paginateMemory(items, page.Page, page.PageSize), total, nil
}

// ListAgentRunPageByAccount 從儲存層列出 agent 執行分頁 by 帳號。
func (s *Store) ListAgentRunPageByAccount(ctx context.Context, tenantID, accountID string, page PageRequest) ([]AgentRun, int, error) {
	items, err := s.ListAgentRunsByAccount(ctx, tenantID, accountID)
	if err != nil {
		return nil, 0, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		switch page.Sort {
		case "created_at_asc":
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		default:
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
	})
	page = utils.NormalizePageRequest(page)
	total := len(items)
	return paginateMemory(items, page.Page, page.PageSize), total, nil
}

// UpsertAgentModel 從儲存層處理 upsert agent 模型。
func (s *Store) UpsertAgentModel(_ context.Context, v AgentModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.agentModels, v.TenantID, v.ID, copyAgentModel(v))
	return nil
}

// GetAgentModel 從儲存層取得 agent 模型。
func (s *Store) GetAgentModel(_ context.Context, tenantID, id string) (AgentModel, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.agentModels, tenantID, id)
	if !ok {
		return AgentModel{}, false, nil
	}
	return copyAgentModel(v), true, nil
}

// ListAgentModels 從儲存層列出 agent 模型。
func (s *Store) ListAgentModels(_ context.Context, tenantID string) ([]AgentModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.agentModels[tenantID], copyAgentModel)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

// DeleteAgentModel 從儲存層刪除 agent 模型。
func (s *Store) DeleteAgentModel(_ context.Context, tenantID, id string) (AgentModel, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := getNested(s.agentModels, tenantID, id)
	if !ok {
		return AgentModel{}, false, nil
	}
	delete(s.agentModels[tenantID], id)
	return copyAgentModel(v), true, nil
}

// UpdateAgentModelTestResult 從儲存層更新模型測試結果。
func (s *Store) UpdateAgentModelTestResult(_ context.Context, tenantID, id, status, message string, testedAt time.Time) (AgentModel, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := getNested(s.agentModels, tenantID, id)
	if !ok {
		return AgentModel{}, false, nil
	}
	t := testedAt.UTC()
	v.LastTestedAt = &t
	v.LastTestStatus = status
	v.LastTestMessage = message
	v.UpdatedAt = testedAt.UTC()
	s.agentModels[tenantID][id] = copyAgentModel(v)
	return copyAgentModel(v), true, nil
}

// UpdateAgentModelSyncResult 從儲存層更新模型同步結果。
func (s *Store) UpdateAgentModelSyncResult(_ context.Context, tenantID, id string, status AgentModelSyncStatus, lastError, configHash string, syncedAt *time.Time, updatedAt time.Time) (AgentModel, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := getNested(s.agentModels, tenantID, id)
	if !ok {
		return AgentModel{}, false, nil
	}
	v.SyncStatus = status
	v.LastSyncError = lastError
	v.SyncedConfigHash = configHash
	v.LastSyncedAt = nil
	if syncedAt != nil {
		t := syncedAt.UTC()
		v.LastSyncedAt = &t
	}
	v.UpdatedAt = updatedAt.UTC()
	s.agentModels[tenantID][id] = copyAgentModel(v)
	return copyAgentModel(v), true, nil
}

// ListAgentDefinitionRefsByModel 列出目前引用模型的 agent（僅當前定義，不含歷史版本）。
func (s *Store) ListAgentDefinitionRefsByModel(_ context.Context, tenantID, modelID string) ([]domain.AgentDefinitionRef, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var refs []domain.AgentDefinitionRef
	for _, item := range s.agentDefinitions[tenantID] {
		if item.ModelID == modelID {
			refs = append(refs, domain.AgentDefinitionRef{ID: item.ID, Name: item.Name})
			continue
		}
		for _, member := range item.SubAgents {
			if member.ModelID == modelID {
				refs = append(refs, domain.AgentDefinitionRef{ID: item.ID, Name: item.Name})
				break
			}
		}
	}
	return refs, nil
}

// InsertAgentExternalTool stores one tenant-scoped external tool registration.
func (s *Store) InsertAgentExternalTool(_ context.Context, item AgentExternalTool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.agentExternalTools, item.TenantID, item.ID, copyAgentExternalTool(item))
	return nil
}

// GetAgentExternalTool returns one tenant-scoped external connection with its capabilities.
func (s *Store) GetAgentExternalTool(_ context.Context, tenantID, id string) (AgentExternalTool, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := getNested(s.agentExternalTools, tenantID, id)
	return copyAgentExternalTool(item), ok, nil
}

// ListAgentExternalTools returns external tools in newest-first order.
func (s *Store) ListAgentExternalTools(_ context.Context, tenantID string) ([]AgentExternalTool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.agentExternalTools[tenantID], copyAgentExternalTool)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// DeleteAgentExternalTool archives one tenant-scoped external tool registration.
func (s *Store) DeleteAgentExternalTool(_ context.Context, tenantID, id string) (AgentExternalTool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := getNested(s.agentExternalTools, tenantID, id)
	if !ok {
		return AgentExternalTool{}, false, nil
	}
	now := time.Now().UTC()
	item.Status = string(domain.ExternalToolConnectionStatusArchived)
	item.ArchivedAt = &now
	item.UpdatedAt = now
	for index := range item.Capabilities {
		item.Capabilities[index].Enabled = false
		item.Capabilities[index].ArchivedAt = &now
		item.Capabilities[index].UpdatedAt = now
	}
	putNested(s.agentExternalTools, tenantID, id, item)
	return copyAgentExternalTool(item), true, nil
}

// ReplaceAgentExternalToolCapabilities atomically replaces the current active catalogue.
func (s *Store) ReplaceAgentExternalToolCapabilities(_ context.Context, tenantID, id string, capabilities []domain.ExternalToolCapability) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := getNested(s.agentExternalTools, tenantID, id)
	if !ok {
		return nil
	}
	item.Capabilities = append([]domain.ExternalToolCapability(nil), capabilities...)
	putNested(s.agentExternalTools, tenantID, id, copyAgentExternalTool(item))
	return nil
}

// UpdateAgentExternalToolTestResult records the latest explicit endpoint check.
func (s *Store) UpdateAgentExternalToolTestResult(_ context.Context, tenantID, id, status, message string, testedAt time.Time) (AgentExternalTool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := getNested(s.agentExternalTools, tenantID, id)
	if !ok {
		return AgentExternalTool{}, false, nil
	}
	item.LastTestedAt = &testedAt
	item.LastTestStatus = status
	item.LastTestMessage = message
	item.UpdatedAt = testedAt
	putNested(s.agentExternalTools, tenantID, id, copyAgentExternalTool(item))
	return copyAgentExternalTool(item), true, nil
}

// GetAgentExternalToolCapability returns one enabled capability on an active connection.
func (s *Store) GetAgentExternalToolCapability(_ context.Context, tenantID, capabilityID string) (domain.ExternalToolCapability, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	capability, ok := s.agentExternalToolCapabilityLocked(tenantID, capabilityID)
	return copyExternalToolCapability(capability), ok, nil
}

// ListAgentExternalToolCapabilities lists enabled capabilities on one active connection.
func (s *Store) ListAgentExternalToolCapabilities(_ context.Context, tenantID, connectionID string) ([]domain.ExternalToolCapability, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	connection, ok := getNested(s.agentExternalTools, tenantID, connectionID)
	if !ok || connection.Status != string(domain.ExternalToolConnectionStatusActive) {
		return []domain.ExternalToolCapability{}, nil
	}
	out := make([]domain.ExternalToolCapability, 0, len(connection.Capabilities))
	for _, capability := range connection.Capabilities {
		if capability.Enabled && capability.ArchivedAt == nil {
			out = append(out, copyExternalToolCapability(capability))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ToolName == out[j].ToolName {
			return out[i].ID < out[j].ID
		}
		return out[i].ToolName < out[j].ToolName
	})
	return out, nil
}

// ListAgentExternalToolCapabilitiesByIDs batch-loads enabled capabilities on active connections.
func (s *Store) ListAgentExternalToolCapabilitiesByIDs(_ context.Context, tenantID string, capabilityIDs []string) ([]domain.ExternalToolCapability, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	wanted := make(map[string]struct{}, len(capabilityIDs))
	for _, id := range capabilityIDs {
		wanted[id] = struct{}{}
	}
	out := make([]domain.ExternalToolCapability, 0, len(wanted))
	for _, connection := range s.agentExternalTools[tenantID] {
		if connection.Status != string(domain.ExternalToolConnectionStatusActive) {
			continue
		}
		for _, capability := range connection.Capabilities {
			if _, ok := wanted[capability.ID]; ok && capability.Enabled && capability.ArchivedAt == nil {
				out = append(out, copyExternalToolCapability(capability))
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ToolName == out[j].ToolName {
			return out[i].ID < out[j].ID
		}
		return out[i].ToolName < out[j].ToolName
	})
	return out, nil
}

func (s *Store) agentExternalToolCapabilityLocked(tenantID, capabilityID string) (domain.ExternalToolCapability, bool) {
	for _, connection := range s.agentExternalTools[tenantID] {
		if connection.Status != string(domain.ExternalToolConnectionStatusActive) {
			continue
		}
		for _, capability := range connection.Capabilities {
			if capability.ID == capabilityID && capability.Enabled && capability.ArchivedAt == nil {
				return capability, true
			}
		}
	}
	return domain.ExternalToolCapability{}, false
}

// UpsertAgentDefinition 從儲存層處理 upsert agent 定義。
func (s *Store) UpsertAgentDefinition(_ context.Context, v AgentDefinition) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.agentDefinitions, v.TenantID, v.ID, copyAgentDefinition(v))
	return nil
}

// GetAgentDefinition 從儲存層取得 agent 定義。
func (s *Store) GetAgentDefinition(_ context.Context, tenantID, id string) (AgentDefinition, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.agentDefinitions, tenantID, id)
	if !ok {
		return AgentDefinition{}, false, nil
	}
	return copyAgentDefinition(v), true, nil
}

// ListAgentDefinitions 從儲存層列出 agent 定義。
func (s *Store) ListAgentDefinitions(_ context.Context, tenantID string) ([]AgentDefinition, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.agentDefinitions[tenantID], copyAgentDefinition)
	sortAgentDefinitions(out)
	return out, nil
}

// ListPublishedAgentDefinitions 從儲存層列出已發布 agent 定義。
func (s *Store) ListPublishedAgentDefinitions(ctx context.Context, tenantID string) ([]AgentDefinition, error) {
	items, err := s.ListAgentDefinitions(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]AgentDefinition, 0, len(items))
	for _, item := range items {
		if item.Status == domain.AgentDefinitionStatusPublished {
			out = append(out, item)
		}
	}
	return out, nil
}

// DeleteAgentDefinition 從儲存層刪除 agent 定義。
func (s *Store) DeleteAgentDefinition(_ context.Context, tenantID, id string) (AgentDefinition, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := getNested(s.agentDefinitions, tenantID, id)
	if !ok {
		return AgentDefinition{}, false, nil
	}
	delete(s.agentDefinitions[tenantID], id)
	return copyAgentDefinition(v), true, nil
}

// UpdateAgentDefinitionUsage 從儲存層更新 agent usage。
func (s *Store) UpdateAgentDefinitionUsage(_ context.Context, tenantID, id string, success bool, latencyMs int, prompt string, runAt time.Time) (AgentDefinition, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := getNested(s.agentDefinitions, tenantID, id)
	if !ok {
		return AgentDefinition{}, false, nil
	}
	totalBefore := v.Usage.TotalRuns
	v.Usage.TotalRuns++
	if success {
		v.Usage.SuccessRuns++
	} else {
		v.Usage.FailedRuns++
	}
	if latencyMs > 0 {
		v.Usage.AvgLatencyMs = int((int64(v.Usage.AvgLatencyMs)*totalBefore + int64(latencyMs)) / v.Usage.TotalRuns)
	}
	t := runAt.UTC()
	v.Usage.LastRunAt = &t
	prompt = strings.TrimSpace(prompt)
	if prompt != "" {
		v.Usage.TopPrompts = append([]string{prompt}, v.Usage.TopPrompts...)
		if len(v.Usage.TopPrompts) > 5 {
			v.Usage.TopPrompts = v.Usage.TopPrompts[:5]
		}
	}
	v.UpdatedAt = runAt.UTC()
	s.agentDefinitions[tenantID][id] = copyAgentDefinition(v)
	return copyAgentDefinition(v), true, nil
}

// InsertAgentDefinitionVersion 從儲存層新增 agent 版本。
func (s *Store) InsertAgentDefinitionVersion(_ context.Context, v AgentDefinitionVersion) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v = copyAgentDefinitionVersion(v)
	if strings.TrimSpace(v.ModelConfigChecksum) == "" {
		if model, ok := getNested(s.agentModels, v.TenantID, v.ModelID); ok {
			v.ModelConfigChecksum = domain.AgentModelSyncConfigHash(model)
		}
	}
	for index := range v.SubAgents {
		if strings.TrimSpace(v.SubAgents[index].ModelConfigChecksum) != "" {
			continue
		}
		if model, ok := getNested(s.agentModels, v.TenantID, v.SubAgents[index].ModelID); ok {
			v.SubAgents[index].ModelConfigChecksum = domain.AgentModelSyncConfigHash(model)
		}
	}
	putNested(s.agentDefinitionVersions, v.TenantID, agentDefinitionVersionKey(v.AgentID, v.Version), v)
	rootBindings := make([]domain.AgentRevisionExternalTool, 0, len(v.ExternalToolIDs))
	for ordinal, externalToolID := range v.ExternalToolIDs {
		capability, _ := s.agentExternalToolCapabilityLocked(v.TenantID, externalToolID)
		rootBindings = append(rootBindings, domain.AgentRevisionExternalTool{
			TenantID: v.TenantID, RevisionID: v.ID, ExternalToolID: externalToolID,
			ToolSchemaChecksum: capability.SchemaChecksum, Ordinal: ordinal, Config: map[string]any{},
		})
	}
	putNested(s.agentRevisionExternalTools, v.TenantID, v.ID, rootBindings)
	for _, member := range v.SubAgents {
		childRevisionID := v.ID + ":member:" + member.ID
		childBindings := make([]domain.AgentRevisionExternalTool, 0, len(member.ExternalToolIDs))
		for ordinal, externalToolID := range member.ExternalToolIDs {
			capability, _ := s.agentExternalToolCapabilityLocked(v.TenantID, externalToolID)
			childBindings = append(childBindings, domain.AgentRevisionExternalTool{
				TenantID: v.TenantID, RevisionID: childRevisionID, ExternalToolID: externalToolID,
				ToolSchemaChecksum: capability.SchemaChecksum, Ordinal: ordinal, Config: map[string]any{},
			})
		}
		putNested(s.agentRevisionExternalTools, v.TenantID, childRevisionID, childBindings)
	}
	return nil
}

// ListAgentRevisionExternalToolBindings returns the publish-time root capability snapshot.
func (s *Store) ListAgentRevisionExternalToolBindings(_ context.Context, tenantID, revisionID string) ([]domain.AgentRevisionExternalTool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items, _ := getNested(s.agentRevisionExternalTools, tenantID, revisionID)
	return copyAgentRevisionExternalTools(items), nil
}

// AppendExecutionStep allocates the next execution-local sequence while holding the store lock.
func (s *Store) AppendExecutionStep(_ context.Context, step domain.ExecutionStep) (domain.ExecutionStep, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := getNested(s.agentRuns, step.TenantID, step.ExecutionID); !ok {
		return domain.ExecutionStep{}, domain.NotFound("execution", step.ExecutionID)
	}
	if _, exists := getNested(s.agentExecutionSteps, step.TenantID, step.ID); exists {
		return domain.ExecutionStep{}, domain.Conflict("execution step already exists")
	}
	nextSequence := 1
	for _, existing := range s.agentExecutionSteps[step.TenantID] {
		if existing.ExecutionID == step.ExecutionID && existing.SequenceNo >= nextSequence {
			nextSequence = existing.SequenceNo + 1
		}
	}
	step.SequenceNo = nextSequence
	step.Status = domain.ExecutionStepStatusRunning
	step.OutputSummary = map[string]any{}
	step.InputTokens = 0
	step.CachedTokens = 0
	step.OutputTokens = 0
	step.CompletedAt = nil
	step.ErrorCode = ""
	putNested(s.agentExecutionSteps, step.TenantID, step.ID, copyExecutionStep(step))
	return copyExecutionStep(step), nil
}

// UpsertExecutionStep inserts a step or updates only its lifecycle fields.
func (s *Store) UpsertExecutionStep(_ context.Context, step domain.ExecutionStep) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := getNested(s.agentExecutionSteps, step.TenantID, step.ID)
	if !ok {
		putNested(s.agentExecutionSteps, step.TenantID, step.ID, copyExecutionStep(step))
		return nil
	}
	if existing.ExecutionID != step.ExecutionID {
		return domain.Conflict("execution step belongs to a different execution")
	}
	existing.Status = step.Status
	existing.InputSummary = step.InputSummary
	existing.OutputSummary = step.OutputSummary
	existing.InputTokens = step.InputTokens
	existing.CachedTokens = step.CachedTokens
	existing.OutputTokens = step.OutputTokens
	existing.StartedAt = step.StartedAt
	existing.CompletedAt = step.CompletedAt
	existing.ErrorCode = step.ErrorCode
	putNested(s.agentExecutionSteps, step.TenantID, step.ID, copyExecutionStep(existing))
	return nil
}

// GetExecutionStep returns one tenant-owned execution step.
func (s *Store) GetExecutionStep(_ context.Context, tenantID, id string) (domain.ExecutionStep, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	step, ok := getNested(s.agentExecutionSteps, tenantID, id)
	return copyExecutionStep(step), ok, nil
}

// ListExecutionSteps returns one execution's steps in sequence order.
func (s *Store) ListExecutionSteps(_ context.Context, tenantID, executionID string) ([]domain.ExecutionStep, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.ExecutionStep, 0)
	for _, step := range s.agentExecutionSteps[tenantID] {
		if step.ExecutionID == executionID {
			out = append(out, copyExecutionStep(step))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SequenceNo == out[j].SequenceNo {
			return out[i].ID < out[j].ID
		}
		return out[i].SequenceNo < out[j].SequenceNo
	})
	return out, nil
}

// UpsertAgentConfirmation creates an immutable confirmation action envelope.
func (s *Store) UpsertAgentConfirmation(_ context.Context, confirmation domain.AgentConfirmationRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := getNested(s.agentConfirmations, confirmation.TenantID, confirmation.ID); ok {
		if existing.AccountID != confirmation.AccountID || existing.ConversationID != confirmation.ConversationID ||
			existing.SegmentID != confirmation.SegmentID || existing.Kind != confirmation.Kind || existing.Action != confirmation.Action {
			return domain.Conflict("agent confirmation id already belongs to a different action")
		}
		return nil
	}
	putNested(s.agentConfirmations, confirmation.TenantID, confirmation.ID, copyAgentConfirmation(confirmation))
	return nil
}

// ListPendingAgentConfirmations restores only unexpired records on the current segment.
func (s *Store) ListPendingAgentConfirmations(_ context.Context, tenantID, accountID, conversationID, segmentID string, now time.Time) ([]domain.AgentConfirmationRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := getNested(s.agentSessions, tenantID, conversationID)
	if !ok || session.SegmentID != segmentID {
		return []domain.AgentConfirmationRecord{}, nil
	}
	out := make([]domain.AgentConfirmationRecord, 0)
	for _, confirmation := range s.agentConfirmations[tenantID] {
		if confirmation.AccountID == accountID && confirmation.ConversationID == conversationID &&
			confirmation.SegmentID == segmentID && confirmation.Status == domain.AgentConfirmationStatusPending &&
			confirmation.ExpiresAt.After(now) {
			out = append(out, copyAgentConfirmation(confirmation))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

// ClaimAgentConfirmation atomically expires or moves one pending action to executing.
func (s *Store) ClaimAgentConfirmation(_ context.Context, tenantID, accountID, id string, now time.Time) (domain.AgentConfirmationRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	confirmation, ok := getNested(s.agentConfirmations, tenantID, id)
	if !ok || confirmation.AccountID != accountID || confirmation.Status != domain.AgentConfirmationStatusPending {
		return domain.AgentConfirmationRecord{}, false, nil
	}
	session, ok := getNested(s.agentSessions, tenantID, confirmation.ConversationID)
	if !ok || session.SegmentID != confirmation.SegmentID {
		return domain.AgentConfirmationRecord{}, false, nil
	}
	confirmation.UpdatedAt = now.UTC()
	if !confirmation.ExpiresAt.After(now) {
		consumedAt := now.UTC()
		confirmation.Status = domain.AgentConfirmationStatusExpired
		confirmation.ConsumedAt = &consumedAt
	} else {
		confirmation.Status = domain.AgentConfirmationStatusExecuting
	}
	putNested(s.agentConfirmations, tenantID, id, copyAgentConfirmation(confirmation))
	return copyAgentConfirmation(confirmation), true, nil
}

// UpdateAgentConfirmation applies one legal settlement transition.
func (s *Store) UpdateAgentConfirmation(_ context.Context, confirmation domain.AgentConfirmationRecord) (domain.AgentConfirmationRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := getNested(s.agentConfirmations, confirmation.TenantID, confirmation.ID)
	if !ok || existing.AccountID != confirmation.AccountID || !existing.Status.CanTransitionTo(confirmation.Status) {
		return domain.AgentConfirmationRecord{}, false, nil
	}
	existing.ResultPayload = confirmation.ResultPayload
	existing.Status = confirmation.Status
	existing.LastError = confirmation.LastError
	existing.ConsumedAt = confirmation.ConsumedAt
	existing.UpdatedAt = confirmation.UpdatedAt
	putNested(s.agentConfirmations, confirmation.TenantID, confirmation.ID, copyAgentConfirmation(existing))
	return copyAgentConfirmation(existing), true, nil
}

// ListAgentDefinitionVersions 從儲存層列出 agent 版本。
func (s *Store) ListAgentDefinitionVersions(_ context.Context, tenantID, agentID string) ([]AgentDefinitionVersion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AgentDefinitionVersion, 0)
	for _, item := range s.agentDefinitionVersions[tenantID] {
		if item.AgentID == agentID {
			out = append(out, copyAgentDefinitionVersion(item))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Version > out[j].Version })
	return out, nil
}

// GetAgentDefinitionVersion 從儲存層取得 agent 版本。
func (s *Store) GetAgentDefinitionVersion(_ context.Context, tenantID, agentID string, version int) (AgentDefinitionVersion, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.agentDefinitionVersions, tenantID, agentDefinitionVersionKey(agentID, version))
	if !ok {
		return AgentDefinitionVersion{}, false, nil
	}
	return copyAgentDefinitionVersion(v), true, nil
}

// UpsertAgentSession 從儲存層處理 upsert agent 會話。
func (s *Store) UpsertAgentSession(_ context.Context, v AgentSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.ContextVersion <= 0 {
		v.ContextVersion = 1
	}
	putNested(s.agentSessions, v.TenantID, v.ID, copyAgentSession(v))
	return nil
}

// GetAgentSession 從儲存層取得 agent 會話。
func (s *Store) GetAgentSession(_ context.Context, tenantID, id string) (AgentSession, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.agentSessions, tenantID, id)
	if !ok {
		return AgentSession{}, false, nil
	}
	return copyAgentSession(v), true, nil
}

// GetAgentSessionForUpdate reads a session while the memory transaction holds the root lock.
func (s *Store) GetAgentSessionForUpdate(ctx context.Context, tenantID, id string) (AgentSession, bool, error) {
	return s.GetAgentSession(ctx, tenantID, id)
}

// ListAgentSessionsByAccount 從儲存層列出 account 的 agent 會話。
func (s *Store) ListAgentSessionsByAccount(_ context.Context, tenantID, accountID, status, agentID string, page domain.KeysetPage) ([]AgentSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AgentSession, 0)
	for _, item := range s.agentSessions[tenantID] {
		if item.AccountID != accountID {
			continue
		}
		if status != "" && string(item.Status) != status {
			continue
		}
		if agentID != "" && item.AgentID != agentID {
			continue
		}
		if page.HasCursor && !agentSessionAfterKeysetCursor(item, page) {
			continue
		}
		out = append(out, copyAgentSession(item))
	}
	sortAgentSessions(out)
	if page.Limit > 0 && len(out) > page.Limit {
		out = out[:page.Limit]
	}
	return out, nil
}

// agentSessionAfterKeysetCursor 保留排在 (created_at DESC, id DESC) 遊標之後的會話。
func agentSessionAfterKeysetCursor(item AgentSession, page domain.KeysetPage) bool {
	createdAt := item.CreatedAt.UTC()
	cursorAt := page.CursorCreatedAt.UTC()
	if createdAt.Equal(cursorAt) {
		return item.ID < page.CursorID
	}
	return createdAt.Before(cursorAt)
}

// ListAgentUsageByAccount returns a filtered account usage page.
func (s *Store) ListAgentUsageByAccount(_ context.Context, tenantID string, query domain.AgentAccountUsageQuery, page domain.PageRequest) ([]domain.AgentAccountUsage, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	all := s.agentUsageByAccountLocked(tenantID)
	filtered := make([]domain.AgentAccountUsage, 0, len(all))
	searchQuery := strings.ToLower(strings.TrimSpace(query.Query))
	for _, item := range all {
		if query.Status != "" && item.Status != query.Status {
			continue
		}
		haystack := strings.ToLower(item.DisplayName + " " + item.Email + " " + item.AccountID)
		if searchQuery != "" && !strings.Contains(haystack, searchQuery) {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return agentAccountUsageLess(filtered[i], filtered[j], page.Sort)
	})
	total := len(filtered)
	start := (page.Page - 1) * page.PageSize
	if start >= total {
		return []domain.AgentAccountUsage{}, total, nil
	}
	end := start + page.PageSize
	if end > total {
		end = total
	}
	return filtered[start:end], total, nil
}

// GetAgentUsageByAccount returns one tenant account's aggregate usage.
func (s *Store) GetAgentUsageByAccount(_ context.Context, tenantID, accountID string) (domain.AgentAccountUsage, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.agentUsageByAccountLocked(tenantID) {
		if item.AccountID == accountID {
			return item, true, nil
		}
	}
	return domain.AgentAccountUsage{}, false, nil
}

// GetAgentUsageSummary aggregates tenant-wide usage without applying list filters.
func (s *Store) GetAgentUsageSummary(_ context.Context, tenantID string) (domain.AgentUsageSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := s.agentUsageByAccountLocked(tenantID)
	summary := domain.AgentUsageSummary{UserCount: len(items)}
	for _, item := range items {
		if item.SessionCount > 0 || item.MessageCount > 0 {
			summary.UsersWithUsage++
		}
		summary.SessionCount += item.SessionCount
		summary.MessageCount += item.MessageCount
		summary.LLMCallCount += item.LLMCallCount
		summary.InputTokens += item.InputTokens
		summary.CachedTokens += item.CachedTokens
		summary.OutputTokens += item.OutputTokens
		summary.TotalTokens += item.TotalTokens
		summary.ActualTokens += item.ActualTokens
	}
	return summary, nil
}

// agentUsageByAccountLocked builds account aggregates while the caller holds a read lock.
func (s *Store) agentUsageByAccountLocked(tenantID string) []domain.AgentAccountUsage {

	out := make([]domain.AgentAccountUsage, 0, len(s.accounts[tenantID]))
	for _, account := range s.accounts[tenantID] {
		item := domain.AgentAccountUsage{
			AccountID:   account.ID,
			DisplayName: account.DisplayName,
			Email:       account.Email,
			Status:      account.Status,
		}
		sessionIDs := make(map[string]struct{})
		for _, session := range s.agentSessions[tenantID] {
			if session.AccountID != account.ID {
				continue
			}
			sessionIDs[session.ID] = struct{}{}
			item.SessionCount++
			activityAt := session.UpdatedAt
			if session.LastMessageAt != nil && session.LastMessageAt.After(activityAt) {
				activityAt = *session.LastMessageAt
			}
			if item.LastActiveAt == nil || activityAt.After(*item.LastActiveAt) {
				activityCopy := activityAt
				item.LastActiveAt = &activityCopy
			}
		}
		for _, message := range s.agentSessionMessages[tenantID] {
			if _, ok := sessionIDs[message.SessionID]; !ok {
				continue
			}
			item.MessageCount++
			if item.LastActiveAt == nil || message.CreatedAt.After(*item.LastActiveAt) {
				activityCopy := message.CreatedAt
				item.LastActiveAt = &activityCopy
			}
		}
		for _, run := range s.agentRuns[tenantID] {
			if run.AccountID != account.ID {
				continue
			}
			item.LLMCallCount += run.LLMCallCount
			item.InputTokens += run.InputTokens
			item.CachedTokens += run.CachedTokens
			item.OutputTokens += run.OutputTokens
			item.TotalTokens += run.TotalTokens
		}
		item.ActualTokens = item.TotalTokens - item.CachedTokens
		if item.ActualTokens < 0 {
			item.ActualTokens = 0
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SessionCount != out[j].SessionCount {
			return out[i].SessionCount > out[j].SessionCount
		}
		if out[i].MessageCount != out[j].MessageCount {
			return out[i].MessageCount > out[j].MessageCount
		}
		leftName := strings.ToLower(out[i].DisplayName)
		rightName := strings.ToLower(out[j].DisplayName)
		if leftName != rightName {
			return leftName < rightName
		}
		return out[i].AccountID < out[j].AccountID
	})
	return out
}

// agentAccountUsageLess applies the API's supported server-side ordering with stable name ties.
func agentAccountUsageLess(left, right domain.AgentAccountUsage, order string) bool {
	switch order {
	case "session_count_asc":
		if left.SessionCount != right.SessionCount {
			return left.SessionCount < right.SessionCount
		}
	case "session_count_desc", "usage_desc", "":
		if left.SessionCount != right.SessionCount {
			return left.SessionCount > right.SessionCount
		}
		if order == "usage_desc" || order == "" {
			if left.MessageCount != right.MessageCount {
				return left.MessageCount > right.MessageCount
			}
		}
	case "message_count_asc":
		if left.MessageCount != right.MessageCount {
			return left.MessageCount < right.MessageCount
		}
	case "message_count_desc":
		if left.MessageCount != right.MessageCount {
			return left.MessageCount > right.MessageCount
		}
	case "total_tokens_asc":
		if left.TotalTokens != right.TotalTokens {
			return left.TotalTokens < right.TotalTokens
		}
	case "total_tokens_desc":
		if left.TotalTokens != right.TotalTokens {
			return left.TotalTokens > right.TotalTokens
		}
	case "cached_tokens_asc":
		if left.CachedTokens != right.CachedTokens {
			return left.CachedTokens < right.CachedTokens
		}
	case "cached_tokens_desc":
		if left.CachedTokens != right.CachedTokens {
			return left.CachedTokens > right.CachedTokens
		}
	case "actual_tokens_asc":
		if left.ActualTokens != right.ActualTokens {
			return left.ActualTokens < right.ActualTokens
		}
	case "actual_tokens_desc":
		if left.ActualTokens != right.ActualTokens {
			return left.ActualTokens > right.ActualTokens
		}
	case "last_active_at_asc":
		if left.LastActiveAt == nil {
			return false
		}
		if right.LastActiveAt == nil {
			return true
		}
		if !left.LastActiveAt.Equal(*right.LastActiveAt) {
			return left.LastActiveAt.Before(*right.LastActiveAt)
		}
	case "last_active_at_desc":
		if left.LastActiveAt == nil {
			return false
		}
		if right.LastActiveAt == nil {
			return true
		}
		if !left.LastActiveAt.Equal(*right.LastActiveAt) {
			return left.LastActiveAt.After(*right.LastActiveAt)
		}
	}
	leftName, rightName := strings.ToLower(left.DisplayName), strings.ToLower(right.DisplayName)
	if leftName != rightName {
		return leftName < rightName
	}
	return left.AccountID < right.AccountID
}

// ListAgentUsageBySession returns paginated usage for one account's sessions.
func (s *Store) ListAgentUsageBySession(_ context.Context, tenantID, accountID string, page domain.PageRequest) ([]domain.AgentSessionUsage, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]domain.AgentSessionUsage, 0, len(s.agentSessions[tenantID]))
	for _, session := range s.agentSessions[tenantID] {
		if session.AccountID != accountID {
			continue
		}
		lastActiveAt := session.UpdatedAt
		if session.LastMessageAt != nil && session.LastMessageAt.After(lastActiveAt) {
			lastActiveAt = *session.LastMessageAt
		}
		item := domain.AgentSessionUsage{
			SessionID:    session.ID,
			AccountID:    session.AccountID,
			Title:        session.Title,
			Status:       session.Status,
			LastActiveAt: &lastActiveAt,
		}
		for _, message := range s.agentSessionMessages[tenantID] {
			if message.SessionID != session.ID {
				continue
			}
			item.MessageCount++
			if message.CreatedAt.After(*item.LastActiveAt) {
				activityCopy := message.CreatedAt
				item.LastActiveAt = &activityCopy
			}
		}
		for _, run := range s.agentRuns[tenantID] {
			if run.SessionID != session.ID {
				continue
			}
			item.LLMCallCount += run.LLMCallCount
			item.InputTokens += run.InputTokens
			item.CachedTokens += run.CachedTokens
			item.OutputTokens += run.OutputTokens
			item.TotalTokens += run.TotalTokens
			if run.UpdatedAt.After(*item.LastActiveAt) {
				activityCopy := run.UpdatedAt
				item.LastActiveAt = &activityCopy
			}
		}
		item.ActualTokens = item.TotalTokens - item.CachedTokens
		if item.ActualTokens < 0 {
			item.ActualTokens = 0
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].LastActiveAt != nil && out[j].LastActiveAt != nil && !out[i].LastActiveAt.Equal(*out[j].LastActiveAt) {
			return out[i].LastActiveAt.After(*out[j].LastActiveAt)
		}
		return out[i].SessionID > out[j].SessionID
	})
	total := len(out)
	start := (page.Page - 1) * page.PageSize
	if start > total {
		start = total
	}
	end := start + page.PageSize
	if end > total {
		end = total
	}
	pageItems := make([]domain.AgentSessionUsage, end-start)
	copy(pageItems, out[start:end])
	return pageItems, total, nil
}

// DeleteAgentSession 從儲存層刪除 agent 會話。
func (s *Store) DeleteAgentSession(_ context.Context, tenantID, id string) (AgentSession, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := getNested(s.agentSessions, tenantID, id)
	if !ok {
		return AgentSession{}, false, nil
	}
	delete(s.agentSessions[tenantID], id)
	for messageID, message := range s.agentSessionMessages[tenantID] {
		if message.SessionID == id {
			delete(s.agentSessionMessages[tenantID], messageID)
		}
	}
	for fileID, file := range s.agentSessionFiles[tenantID] {
		if file.SessionID == id {
			delete(s.agentSessionFiles[tenantID], fileID)
			delete(s.agentFileChunks[tenantID], fileID)
		}
	}
	return copyAgentSession(v), true, nil
}

// InsertAgentSessionMessage 從儲存層新增 agent 會話訊息。
func (s *Store) InsertAgentSessionMessage(_ context.Context, v AgentSessionMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.ContextVersion <= 0 {
		if session, ok := getNested(s.agentSessions, v.TenantID, v.SessionID); ok {
			v.ContextVersion = session.ContextVersion
		}
	}
	putNested(s.agentSessionMessages, v.TenantID, v.ID, copyAgentSessionMessage(v))
	return nil
}

// ListAgentSessionMessages 從儲存層列出 agent 會話訊息。
func (s *Store) ListAgentSessionMessages(_ context.Context, tenantID, sessionID string, page domain.KeysetPage) ([]AgentSessionMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AgentSessionMessage, 0)
	session, sessionOK := getNested(s.agentSessions, tenantID, sessionID)
	if !sessionOK {
		return out, nil
	}
	for _, item := range s.agentSessionMessages[tenantID] {
		if item.SessionID == sessionID && item.ContextVersion == session.ContextVersion {
			if page.HasCursor && !agentMessageAfterKeysetCursor(item, page) {
				continue
			}
			out = append(out, copyAgentSessionMessage(item))
		}
	}
	sortAgentSessionMessages(out)
	if page.Limit > 0 && len(out) > page.Limit {
		out = out[:page.Limit]
	}
	return out, nil
}

// agentMessageAfterKeysetCursor 保留排在 (created_at ASC, id ASC) 遊標之後的訊息。
func agentMessageAfterKeysetCursor(item AgentSessionMessage, page domain.KeysetPage) bool {
	createdAt := item.CreatedAt.UTC()
	cursorAt := page.CursorCreatedAt.UTC()
	if createdAt.Equal(cursorAt) {
		return item.ID > page.CursorID
	}
	return createdAt.After(cursorAt)
}

// ListRecentAgentSessionMessages 從儲存層列出最近 agent 會話訊息。
func (s *Store) ListRecentAgentSessionMessages(ctx context.Context, tenantID, sessionID string, limit int) ([]AgentSessionMessage, error) {
	items, err := s.ListAgentSessionMessages(ctx, tenantID, sessionID, domain.KeysetPage{})
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items, nil
}

// UpsertAgentFileAsset persists file metadata for a staged session file.
func (s *Store) UpsertAgentFileAsset(_ context.Context, file domain.AgentSessionFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.agentSessionFiles, file.TenantID, file.ID, copyAgentSessionFile(file))
	return nil
}

// InsertAgentFileChunks stores parsed text in source order.
func (s *Store) InsertAgentFileChunks(_ context.Context, tenantID, fileID string, chunks []string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.agentFileChunks[tenantID] == nil {
		s.agentFileChunks[tenantID] = map[string][]string{}
	}
	s.agentFileChunks[tenantID][fileID] = append([]string(nil), chunks...)
	return nil
}

// ListAgentFileChunks returns a defensive copy of parsed text chunks.
func (s *Store) ListAgentFileChunks(_ context.Context, tenantID, fileID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.agentFileChunks[tenantID][fileID]...), nil
}

// InsertAgentSessionFile records the current session and context binding.
func (s *Store) InsertAgentSessionFile(_ context.Context, file domain.AgentSessionFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.agentSessionFiles, file.TenantID, file.ID, copyAgentSessionFile(file))
	return nil
}

// GetCurrentAgentSessionFile resolves files only from the visible context version.
func (s *Store) GetCurrentAgentSessionFile(_ context.Context, tenantID, sessionID, fileID string) (domain.AgentSessionFile, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, sessionOK := getNested(s.agentSessions, tenantID, sessionID)
	file, fileOK := getNested(s.agentSessionFiles, tenantID, fileID)
	if !sessionOK || !fileOK || file.SessionID != sessionID || file.ContextVersion != session.ContextVersion {
		return domain.AgentSessionFile{}, false, nil
	}
	return copyAgentSessionFile(file), true, nil
}

// ListCurrentAgentSessionFiles lists files from the visible context version.
func (s *Store) ListCurrentAgentSessionFiles(_ context.Context, tenantID, sessionID string) ([]domain.AgentSessionFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := getNested(s.agentSessions, tenantID, sessionID)
	if !ok {
		return []domain.AgentSessionFile{}, nil
	}
	out := make([]domain.AgentSessionFile, 0)
	for _, file := range s.agentSessionFiles[tenantID] {
		if file.SessionID == sessionID && file.ContextVersion == session.ContextVersion {
			out = append(out, copyAgentSessionFile(file))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

// MarkAgentSessionFileAttached binds a draft file to one message and marks it attached.
func (s *Store) MarkAgentSessionFileAttached(_ context.Context, tenantID, sessionID, fileID, messageID string, ordinal int, updatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, sessionOK := getNested(s.agentSessions, tenantID, sessionID)
	file, fileOK := getNested(s.agentSessionFiles, tenantID, fileID)
	if !sessionOK || !fileOK || file.SessionID != sessionID || file.ContextVersion != session.ContextVersion || file.State != "draft" {
		return errors.New("agent session file not found")
	}
	message, messageOK := getNested(s.agentSessionMessages, tenantID, messageID)
	if !messageOK || message.SessionID != sessionID || message.ContextVersion != session.ContextVersion {
		return errors.New("agent session message not found")
	}
	for _, existing := range s.agentSessionFiles[tenantID] {
		if existing.SessionID == sessionID && existing.MessageID == messageID && existing.Ordinal != nil && *existing.Ordinal == ordinal && existing.ID != fileID {
			return errors.New("attachment ordinal already used")
		}
	}
	ordinalCopy := ordinal
	file.State = "attached"
	file.MessageID = messageID
	file.Ordinal = &ordinalCopy
	file.UpdatedAt = updatedAt
	putNested(s.agentSessionFiles, tenantID, fileID, copyAgentSessionFile(file))
	return nil
}

// ListCurrentAgentMessageAttachments returns attachment provenance for visible messages only.
func (s *Store) ListCurrentAgentMessageAttachments(_ context.Context, tenantID, sessionID string) ([]domain.AgentMessageAttachment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := getNested(s.agentSessions, tenantID, sessionID)
	if !ok {
		return []domain.AgentMessageAttachment{}, nil
	}
	out := make([]domain.AgentMessageAttachment, 0)
	for _, file := range s.agentSessionFiles[tenantID] {
		if file.SessionID != sessionID || file.State != "attached" || file.MessageID == "" || file.Ordinal == nil {
			continue
		}
		message, messageOK := getNested(s.agentSessionMessages, tenantID, file.MessageID)
		if !messageOK || message.SessionID != sessionID || message.ContextVersion != session.ContextVersion {
			continue
		}
		out = append(out, domain.AgentMessageAttachment{
			MessageID:          file.MessageID,
			ConversationFileID: file.ConversationFileID,
			Ordinal:            *file.Ordinal,
			File:               copyAgentSessionFile(file),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].MessageID == out[j].MessageID {
			return out[i].Ordinal < out[j].Ordinal
		}
		return out[i].MessageID < out[j].MessageID
	})
	return out, nil
}

// DeleteCurrentDraftAgentSessionFile removes only an unsent file from the visible context.
func (s *Store) DeleteCurrentDraftAgentSessionFile(_ context.Context, tenantID, sessionID, fileID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, sessionOK := getNested(s.agentSessions, tenantID, sessionID)
	file, fileOK := getNested(s.agentSessionFiles, tenantID, fileID)
	if !sessionOK || !fileOK || file.SessionID != sessionID || file.ContextVersion != session.ContextVersion || file.State != "draft" {
		return false, nil
	}
	delete(s.agentSessionFiles[tenantID], fileID)
	return true, nil
}

// DeleteAgentFileAsset removes in-memory metadata and chunks.
func (s *Store) DeleteAgentFileAsset(_ context.Context, tenantID, fileID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.agentSessionFiles[tenantID], fileID)
	delete(s.agentFileChunks[tenantID], fileID)
	return nil
}

// FailStaleAgentRunsBySession closes interrupted runs so they no longer lock the conversation.
func (s *Store) FailStaleAgentRunsBySession(_ context.Context, tenantID, sessionID string, staleBefore, failedAt time.Time, reason string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for id, item := range s.agentRuns[tenantID] {
		if item.SessionID != sessionID || !item.UpdatedAt.Before(staleBefore) {
			continue
		}
		if item.Status != string(domain.AgentRunStatusQueued) && item.Status != string(domain.AgentRunStatusRunning) {
			continue
		}
		item.Status = string(domain.AgentRunStatusFailed)
		if strings.TrimSpace(item.Answer) == "" {
			item.Answer = reason
		}
		item.UpdatedAt = failedAt
		s.agentRuns[tenantID][id] = copyAgentRun(item)
		count++
	}
	return count, nil
}

// CountActiveAgentRunsBySession 從儲存層統計會話中的未完成 agent run。
func (s *Store) CountActiveAgentRunsBySession(_ context.Context, tenantID, sessionID string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, item := range s.agentRuns[tenantID] {
		if item.SessionID != sessionID {
			continue
		}
		if item.Status == string(domain.AgentRunStatusQueued) || item.Status == string(domain.AgentRunStatusRunning) {
			count++
		}
	}
	return count, nil
}

// UpsertAgentMemory 從儲存層處理 upsert agent 記憶。
func (s *Store) UpsertAgentMemory(_ context.Context, v AgentMemory) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.Status == "" {
		v.Status = "active"
	}
	switch {
	case v.SessionID != "":
		v.Scope = "conversation"
		v.AgentID = ""
		if session, ok := getNested(s.agentSessions, v.TenantID, v.SessionID); ok {
			v.SegmentID = session.SegmentID
		}
	case v.AgentID != "":
		v.Scope = "agent"
		v.SessionID = ""
		v.SegmentID = ""
	default:
		v.Scope = "global"
		v.AgentID = ""
		v.SessionID = ""
		v.SegmentID = ""
	}
	if v.Status == "active" {
		for id, existing := range s.agentMemories[v.TenantID] {
			if existing.Status != "active" || existing.AccountID != v.AccountID || existing.Scope != v.Scope ||
				existing.AgentID != v.AgentID || existing.SessionID != v.SessionID || existing.SegmentID != v.SegmentID || existing.Key != v.Key {
				continue
			}
			v.ID = existing.ID
			v.CreatedAt = existing.CreatedAt
			s.agentMemories[v.TenantID][id] = copyAgentMemory(v)
			return nil
		}
	}
	putNested(s.agentMemories, v.TenantID, v.ID, copyAgentMemory(v))
	return nil
}

// GetAgentMemory 按租戶與 ID 取得原始記憶，讓服務層使用同一時鐘判斷單筆記憶是否過期。
func (s *Store) GetAgentMemory(_ context.Context, tenantID, id string) (AgentMemory, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.agentMemories, tenantID, id)
	if !ok {
		return AgentMemory{}, false, nil
	}
	return copyAgentMemory(v), true, nil
}

// ListAgentMemoriesByAccount 從儲存層列出 account 的 agent 記憶。
func (s *Store) ListAgentMemoriesByAccount(_ context.Context, tenantID, accountID, agentID, sessionID string, limit int) ([]AgentMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now().UTC()
	out := make([]AgentMemory, 0)
	currentSegmentID := ""
	if sessionID != "" {
		if session, ok := getNested(s.agentSessions, tenantID, sessionID); ok {
			currentSegmentID = session.SegmentID
		}
	}
	for _, item := range s.agentMemories[tenantID] {
		if item.AccountID != accountID || item.Status != "active" || agentMemoryExpired(item, now) {
			continue
		}
		scope := item.Scope
		if scope == "" {
			switch {
			case item.SessionID != "":
				scope = "conversation"
			case item.AgentID != "":
				scope = "agent"
			default:
				scope = "global"
			}
		}
		matchesScope := scope == "global" ||
			(scope == "agent" && agentID != "" && item.AgentID == agentID) ||
			(scope == "conversation" && sessionID != "" && item.SessionID == sessionID && item.SegmentID == currentSegmentID)
		if !matchesScope {
			continue
		}
		out = append(out, copyAgentMemory(item))
	}
	sortAgentMemories(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// DeleteAgentMemory 從儲存層刪除 agent 記憶。
func (s *Store) DeleteAgentMemory(_ context.Context, tenantID, id string) (AgentMemory, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := getNested(s.agentMemories, tenantID, id)
	if !ok {
		return AgentMemory{}, false, nil
	}
	v.Status = "superseded"
	s.agentMemories[tenantID][id] = copyAgentMemory(v)
	return copyAgentMemory(v), true, nil
}

func sortAgentDefinitions(items []AgentDefinition) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Status != items[j].Status {
			return items[i].Status < items[j].Status
		}
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
}

func sortAgentSessions(items []AgentSession) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
}

func sortAgentSessionMessages(items []AgentSessionMessage) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
}

func sortAgentMemories(items []AgentMemory) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Importance != items[j].Importance {
			return items[i].Importance > items[j].Importance
		}
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
}

func agentMemoryExpired(v AgentMemory, now time.Time) bool {
	return v.ExpiresAt != nil && !v.ExpiresAt.After(now)
}

func agentDefinitionVersionKey(agentID string, version int) string {
	return agentID + "\x00" + strconv.Itoa(version)
}

// UpsertNotification 從儲存層處理 upsert 系統通知。
func (s *Store) UpsertNotification(_ context.Context, v Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.notifications, v.TenantID, v.ID, copyNotification(v))
	return nil
}

// UpsertNotificationRecipient 從儲存層處理 upsert 通知投遞狀態。
func (s *Store) UpsertNotificationRecipient(_ context.Context, v NotificationRecipient) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.notificationRecipients, v.TenantID, notificationRecipientKey(v.NotificationID, v.AccountID), copyNotificationRecipient(v))
	return nil
}

// ListNotificationItems 從儲存層列出目前帳號可見的系統通知。
func (s *Store) ListNotificationItems(_ context.Context, tenantID, accountID string, query domain.NotificationListQuery) ([]NotificationItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now().UTC()
	out := make([]NotificationItem, 0)
	for _, recipient := range s.notificationRecipients[tenantID] {
		if recipient.AccountID != accountID || recipient.DeletedAt != nil {
			continue
		}
		notification, ok := s.notifications[tenantID][recipient.NotificationID]
		if !ok || !memoryNotificationVisible(notification, now) {
			continue
		}
		if query.Tone != "" && notification.Tone != query.Tone {
			continue
		}
		if query.UnreadOnly && recipient.ReadAt != nil {
			continue
		}
		if query.HasCursor && !memoryNotificationAfterCursor(notification, query) {
			continue
		}
		out = append(out, memoryNotificationItem(notification, recipient))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if query.Limit > 0 && len(out) > query.Limit {
		out = out[:query.Limit]
	}
	return out, nil
}

// CountUnreadNotifications 從儲存層統計目前帳號未讀通知數。
func (s *Store) CountUnreadNotifications(_ context.Context, tenantID, accountID string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now().UTC()
	count := 0
	for _, recipient := range s.notificationRecipients[tenantID] {
		if recipient.AccountID != accountID || recipient.DeletedAt != nil || recipient.ReadAt != nil {
			continue
		}
		notification, ok := s.notifications[tenantID][recipient.NotificationID]
		if ok && memoryNotificationVisible(notification, now) {
			count++
		}
	}
	return count, nil
}

// CountNotificationTones 從儲存層統計目前帳號可見通知的 tone 分佈。
func (s *Store) CountNotificationTones(_ context.Context, tenantID, accountID string) (NotificationToneCounts, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now().UTC()
	counts := NotificationToneCounts{}
	for _, recipient := range s.notificationRecipients[tenantID] {
		if recipient.AccountID != accountID || recipient.DeletedAt != nil {
			continue
		}
		notification, ok := s.notifications[tenantID][recipient.NotificationID]
		if !ok || !memoryNotificationVisible(notification, now) {
			continue
		}
		counts.All++
		switch notification.Tone {
		case string(domain.NotificationToneSuccess):
			counts.Success++
		case string(domain.NotificationToneInfo):
			counts.Info++
		case string(domain.NotificationToneWarning):
			counts.Warning++
		}
	}
	return counts, nil
}

// MarkNotificationRead 從儲存層將單筆通知標為已讀。
func (s *Store) MarkNotificationRead(_ context.Context, tenantID, accountID, notificationID string, readAt time.Time) (NotificationItem, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := notificationRecipientKey(notificationID, accountID)
	recipient, ok := s.notificationRecipients[tenantID][key]
	if !ok || recipient.DeletedAt != nil {
		return NotificationItem{}, false, nil
	}
	notification, ok := s.notifications[tenantID][notificationID]
	if !ok || !memoryNotificationVisible(notification, time.Now().UTC()) {
		return NotificationItem{}, false, nil
	}
	if recipient.ReadAt == nil {
		t := readAt.UTC()
		recipient.ReadAt = &t
		s.notificationRecipients[tenantID][key] = copyNotificationRecipient(recipient)
	}
	return memoryNotificationItem(notification, recipient), true, nil
}

// MarkAllNotificationsRead 從儲存層將目前帳號全部未讀通知標為已讀。
func (s *Store) MarkAllNotificationsRead(_ context.Context, tenantID, accountID string, readAt time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	updated := 0
	for key, recipient := range s.notificationRecipients[tenantID] {
		if recipient.AccountID != accountID || recipient.DeletedAt != nil || recipient.ReadAt != nil {
			continue
		}
		notification, ok := s.notifications[tenantID][recipient.NotificationID]
		if !ok || !memoryNotificationVisible(notification, now) {
			continue
		}
		t := readAt.UTC()
		recipient.ReadAt = &t
		s.notificationRecipients[tenantID][key] = copyNotificationRecipient(recipient)
		updated++
	}
	return updated, nil
}

// notificationRecipientKey 建立單筆通知送達狀態的 tenant 內 memory map key。
func notificationRecipientKey(notificationID, accountID string) string {
	return notificationID + "\x00" + accountID
}

// memoryNotificationVisible 以 SQL 查詢相同語義檢查通知是否仍可見。
func memoryNotificationVisible(item Notification, now time.Time) bool {
	return item.ExpiresAt == nil || item.ExpiresAt.After(now)
}

// memoryNotificationAfterCursor 只保留早於倒序遊標的通知列。
func memoryNotificationAfterCursor(item Notification, query domain.NotificationListQuery) bool {
	return item.CreatedAt.Before(query.CursorCreatedAt) || (item.CreatedAt.Equal(query.CursorCreatedAt) && item.ID < query.CursorID)
}

// memoryNotificationItem 合併通知內容與收件者已讀狀態。
func memoryNotificationItem(item Notification, recipient NotificationRecipient) NotificationItem {
	var readAt *time.Time
	if recipient.ReadAt != nil {
		t := recipient.ReadAt.UTC()
		readAt = &t
	}
	return NotificationItem{
		ID:         item.ID,
		Tone:       item.Tone,
		Category:   item.Category,
		Title:      item.Title,
		Body:       item.Body,
		StatusText: item.StatusText,
		LinkURL:    item.LinkURL,
		ReadAt:     readAt,
		CreatedAt:  item.CreatedAt.UTC(),
	}
}

// AppendAuditLog 從儲存層附加稽覈 log。
func (s *Store) AppendAuditLog(_ context.Context, v AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auditLogs[v.TenantID] = append(s.auditLogs[v.TenantID], copyAuditLog(v))
	return nil
}

// ListAuditLogs 從儲存層列出稽覈 logs。
func (s *Store) ListAuditLogs(_ context.Context, tenantID string) ([]AuditLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.auditLogs[tenantID]
	out := make([]AuditLog, 0, len(src))
	for _, v := range src {
		out = append(out, copyAuditLog(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// ListAuditLogFacetSources returns tenant-scoped, distinct, non-sensitive audit facet inputs.
func (s *Store) ListAuditLogFacetSources(_ context.Context, tenantID string) ([]domain.WorkspaceAuditLogFacetSource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := make(map[domain.WorkspaceAuditLogFacetSource]struct{})
	for _, log := range s.auditLogs[tenantID] {
		seen[domain.WorkspaceAuditLogFacetSource{
			ActorAccountID: log.ActorAccountID,
			Action:         log.Action,
			Resource:       log.Resource,
		}] = struct{}{}
	}
	out := make([]domain.WorkspaceAuditLogFacetSource, 0, len(seen))
	for source := range seen {
		out = append(out, source)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ActorAccountID != out[j].ActorAccountID {
			return out[i].ActorAccountID < out[j].ActorAccountID
		}
		if out[i].Resource != out[j].Resource {
			return out[i].Resource < out[j].Resource
		}
		return out[i].Action < out[j].Action
	})
	return out, nil
}

// ListAuditLogPage 從儲存層列出稽覈 log 分頁。
func (s *Store) ListAuditLogPage(ctx context.Context, tenantID string, page PageRequest) ([]AuditLog, int, error) {
	items, err := s.ListAuditLogs(ctx, tenantID)
	if err != nil {
		return nil, 0, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		switch page.Sort {
		case "created_at_asc":
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		default:
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
	})
	page = utils.NormalizePageRequest(page)
	total := len(items)
	return paginateMemory(items, page.Page, page.PageSize), total, nil
}

// ListAuditLogPageFiltered 從儲存層篩選並列出稽覈 log 分頁。
func (s *Store) ListAuditLogPageFiltered(_ context.Context, tenantID string, query domain.WorkspaceAuditLogQuery, page PageRequest) ([]AuditLog, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	accounts := s.accounts[tenantID]
	employees := s.employees[tenantID]
	from, hasFrom := auditLogFilterTime(query.From, false)
	to, hasTo := auditLogFilterTime(query.To, true)
	out := make([]AuditLog, 0, len(s.auditLogs[tenantID]))
	for _, log := range s.auditLogs[tenantID] {
		if hasFrom && log.CreatedAt.Before(from) {
			continue
		}
		if hasTo && !log.CreatedAt.Before(to) {
			continue
		}
		account := accounts[log.ActorAccountID]
		employee := employees[account.EmployeeID]
		if !auditLogOperatorMatches(log, account, employee, query.OperatorID) {
			continue
		}
		if !auditLogTypeMatches(log, query.Type) {
			continue
		}
		if !auditLogKeywordMatches(log, account, employee, query.Keyword) {
			continue
		}
		out = append(out, copyAuditLog(log))
	}
	sort.SliceStable(out, func(i, j int) bool {
		switch page.Sort {
		case "created_at_asc":
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		default:
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
	})
	page = utils.NormalizePageRequest(page)
	total := len(out)
	return paginateMemory(out, page.Page, page.PageSize), total, nil
}

func auditLogFilterTime(value string, endExclusive bool) (time.Time, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		if endExclusive {
			return parsed, true
		}
		return parsed, true
	}
	if parsed, err := time.Parse(time.DateOnly, trimmed); err == nil {
		if endExclusive {
			return parsed.AddDate(0, 0, 1), true
		}
		return parsed, true
	}
	return time.Time{}, false
}

func auditLogOperatorMatches(log AuditLog, account Account, employee Employee, value string) bool {
	needle := strings.ToLower(strings.TrimSpace(value))
	if needle == "" {
		return true
	}
	if needle == strings.ToLower(domain.WorkspaceAuditSystemOperatorID) {
		return strings.TrimSpace(log.ActorAccountID) == ""
	}
	for _, candidate := range []string{log.ActorAccountID, account.ID, account.EmployeeID, account.DisplayName, account.Email, employee.ID, employee.EmployeeNo, employee.Name} {
		if strings.ToLower(strings.TrimSpace(candidate)) == needle {
			return true
		}
	}
	return false
}

func auditLogTypeMatches(log AuditLog, value string) bool {
	needle := strings.ToLower(strings.TrimSpace(value))
	if needle == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{auditLogWorkspaceType(log), log.Resource, log.Action}, " "))
	return strings.Contains(haystack, needle)
}

func auditLogKeywordMatches(log AuditLog, account Account, employee Employee, value string) bool {
	needle := strings.ToLower(strings.TrimSpace(value))
	if needle == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		account.DisplayName,
		account.Email,
		employee.EmployeeNo,
		employee.Name,
		auditLogWorkspaceType(log),
		log.Action,
		log.Resource,
		log.Target,
		fmt.Sprint(log.Details),
	}, " "))
	return strings.Contains(haystack, needle)
}

// auditLogWorkspaceType mirrors the service projection so every advertised facet remains filterable.
func auditLogWorkspaceType(log AuditLog) string {
	text := strings.ToLower(strings.Join([]string{log.Resource, log.Action}, " "))
	switch {
	case strings.Contains(text, "employee"):
		return "員工管理"
	case strings.Contains(text, "org") || strings.Contains(text, "position"):
		return "組織架構"
	case strings.Contains(text, "attendance") || strings.Contains(text, "leave") || strings.Contains(text, "clock") || strings.Contains(text, "shift"):
		return "假勤制度"
	case strings.Contains(text, "form") || strings.Contains(text, "workflow"):
		return "表單設計"
	case strings.Contains(text, "iam") || strings.Contains(text, "authz") || strings.Contains(text, "permission") || strings.Contains(text, "admin"):
		return "管理員設定"
	default:
		return "系統"
	}
}

// GetPermissionVersion 從儲存層取得權限 version。
func (s *Store) GetPermissionVersion(_ context.Context, tenantID string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.permissionVersions[tenantID], nil
}

// IncrementPermissionVersion 從儲存層處理 increment 權限 version。
func (s *Store) IncrementPermissionVersion(_ context.Context, tenantID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.permissionVersions[tenantID]++
	return s.permissionVersions[tenantID], nil
}

// UpsertAuthzRelationshipTuple 從儲存層處理 upsert 授權關係 tuple。
func (s *Store) UpsertAuthzRelationshipTuple(_ context.Context, v AuthzRelationshipTuple) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.relationshipTuples[v.TenantID] == nil {
		s.relationshipTuples[v.TenantID] = map[string]AuthzRelationshipTuple{}
	}
	s.relationshipTuples[v.TenantID][relationshipTupleKey(v)] = v
	return nil
}

// DeleteAuthzRelationshipTuple 從儲存層刪除授權關係 tuple。
func (s *Store) DeleteAuthzRelationshipTuple(_ context.Context, v AuthzRelationshipTuple) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.relationshipTuples[v.TenantID], relationshipTupleKey(v))
	return nil
}

// ListAuthzRelationshipTuplesForObject 從儲存層列出授權關係 tuple for 物件。
func (s *Store) ListAuthzRelationshipTuplesForObject(_ context.Context, tenantID, objectType, objectID string) ([]AuthzRelationshipTuple, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.relationshipTuples[tenantID]
	out := make([]AuthzRelationshipTuple, 0)
	for _, v := range src {
		if v.ObjectType == objectType && v.ObjectID == objectID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Relation != out[j].Relation {
			return out[i].Relation < out[j].Relation
		}
		if out[i].SubjectType != out[j].SubjectType {
			return out[i].SubjectType < out[j].SubjectType
		}
		return out[i].SubjectID < out[j].SubjectID
	})
	return out, nil
}

// AppendIdentityProvisioningOutboxEvent 從儲存層附加身分開通 outbox 事件。
func (s *Store) AppendIdentityProvisioningOutboxEvent(_ context.Context, v IdentityProvisioningOutboxEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.NextAttemptAt.IsZero() {
		v.NextAttemptAt = v.CreatedAt
	}
	s.identityOutbox[v.TenantID] = append(s.identityOutbox[v.TenantID], v)
	return nil
}

// ClaimIdentityProvisioningOutboxEvents atomically leases due events to one worker.
func (s *Store) ClaimIdentityProvisioningOutboxEvents(_ context.Context, tenantID string, batchSize, maxRetries int, claimedAt, leaseUntil time.Time) ([]IdentityProvisioningOutboxEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	indices := make([]int, 0)
	for i, event := range s.identityOutbox[tenantID] {
		if event.RetryCount >= maxRetries {
			continue
		}
		due := event.Status == domain.IdentityProvisioningStatusPending && !event.NextAttemptAt.After(claimedAt)
		stale := event.Status == domain.IdentityProvisioningStatusProcessing && event.ClaimExpiresAt != nil && !event.ClaimExpiresAt.After(claimedAt)
		if due || stale {
			indices = append(indices, i)
		}
	}
	sort.Slice(indices, func(i, j int) bool {
		left, right := s.identityOutbox[tenantID][indices[i]], s.identityOutbox[tenantID][indices[j]]
		if left.NextAttemptAt.Equal(right.NextAttemptAt) {
			return left.CreatedAt.Before(right.CreatedAt)
		}
		return left.NextAttemptAt.Before(right.NextAttemptAt)
	})
	if batchSize < len(indices) {
		indices = indices[:batchSize]
	}
	out := make([]IdentityProvisioningOutboxEvent, 0, len(indices))
	for _, index := range indices {
		event := s.identityOutbox[tenantID][index]
		event.Status = domain.IdentityProvisioningStatusProcessing
		event.UpdatedAt = claimedAt
		expires := leaseUntil
		event.ClaimExpiresAt = &expires
		s.identityOutbox[tenantID][index] = event
		out = append(out, event)
	}
	return out, nil
}

// ListPendingIdentityProvisioningOutboxEvents 從儲存層列出 pending 身分開通 outbox 事件。
func (s *Store) ListPendingIdentityProvisioningOutboxEvents(_ context.Context, tenantID string) ([]IdentityProvisioningOutboxEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.identityOutbox[tenantID]
	out := make([]IdentityProvisioningOutboxEvent, 0, len(src))
	for _, v := range src {
		if v.Status == domain.IdentityProvisioningStatusPending {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// UpdateIdentityProvisioningOutboxEvent 從儲存層更新身分開通 outbox 事件。
func (s *Store) UpdateIdentityProvisioningOutboxEvent(_ context.Context, v IdentityProvisioningOutboxEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := s.identityOutbox[v.TenantID]
	for i := range events {
		if events[i].ID == v.ID {
			events[i] = v
			s.identityOutbox[v.TenantID] = events
			return nil
		}
	}
	return nil
}

// AppendOutboxEvent 從儲存層附加 outbox 事件。
func (s *Store) AppendOutboxEvent(_ context.Context, v OutboxEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v = normalizeMemoryOutboxEvent(v)
	if v.IdempotencyKey != "" {
		for _, existing := range s.outboxEvents[v.TenantID] {
			if existing.EventType == v.EventType && existing.IdempotencyKey == v.IdempotencyKey {
				return nil
			}
		}
	}
	s.outboxEvents[v.TenantID] = append(s.outboxEvents[v.TenantID], copyOutboxEvent(v))
	return nil
}

// normalizeMemoryOutboxEvent mirrors the defaults applied by PostgreSQL.
func normalizeMemoryOutboxEvent(v OutboxEvent) OutboxEvent {
	if v.PayloadVersion <= 0 {
		v.PayloadVersion = 1
	}
	if v.Status == "" {
		v.Status = domain.OutboxStatusPending
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC()
	}
	if v.NextAttemptAt.IsZero() {
		v.NextAttemptAt = v.CreatedAt
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = v.CreatedAt
	}
	if v.AttemptCount == 0 && v.RetryCount > 0 {
		v.AttemptCount = v.RetryCount
	}
	if v.MaxAttempts == nil {
		maxAttempts := domain.DefaultOutboxMaxAttempts
		v.MaxAttempts = &maxAttempts
	}
	return v
}

// ListOutboxEvents 從儲存層列出 outbox 事件。
func (s *Store) ListOutboxEvents(_ context.Context, tenantID string) ([]OutboxEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.outboxEvents[tenantID]
	out := make([]OutboxEvent, 0, len(src))
	for _, v := range src {
		out = append(out, copyOutboxEvent(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// GetOutboxEventByID 從儲存層依主鍵取得 outbox 事件。
func (s *Store) GetOutboxEventByID(_ context.Context, tenantID, id string) (OutboxEvent, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.outboxEvents[tenantID] {
		if v.ID == id {
			return copyOutboxEvent(v), true, nil
		}
	}
	return OutboxEvent{}, false, nil
}

// ListOutboxEventPage 從儲存層篩選並列出 outbox 事件分頁。
func (s *Store) ListOutboxEventPage(_ context.Context, tenantID string, query domain.OutboxEventQuery, page PageRequest) ([]OutboxEvent, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]OutboxEvent, 0, len(s.outboxEvents[tenantID]))
	for _, v := range s.outboxEvents[tenantID] {
		if !outboxEventMatchesQuery(v, query) {
			continue
		}
		out = append(out, copyOutboxEvent(v))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		switch page.Sort {
		case "created_at_asc":
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		default:
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
	})
	page = utils.NormalizePageRequest(page)
	total := len(out)
	return paginateMemory(out, page.Page, page.PageSize), total, nil
}

// outboxEventMatchesQuery 套用 outbox 管理查詢條件。
func outboxEventMatchesQuery(v OutboxEvent, query domain.OutboxEventQuery) bool {
	if status := strings.TrimSpace(query.Status); status != "" && v.Status != status {
		return false
	}
	if eventType := strings.TrimSpace(query.EventType); eventType != "" && v.EventType != eventType {
		return false
	}
	if lastError := strings.TrimSpace(query.LastError); lastError != "" && !strings.Contains(strings.ToLower(v.LastError), strings.ToLower(lastError)) {
		return false
	}
	if query.RetryCount != nil && v.RetryCount != *query.RetryCount {
		return false
	}
	if query.HasError != nil && (strings.TrimSpace(v.LastError) != "") != *query.HasError {
		return false
	}
	return true
}

// ClaimOutboxEvents atomically leases due or stale outbox events for a worker.
func (s *Store) ClaimOutboxEvents(_ context.Context, tenantID string, limit int, claimedAt, leaseUntil time.Time, claimOwner, claimToken string) ([]OutboxEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		return nil, nil
	}
	events := s.outboxEvents[tenantID]
	type ranked struct {
		idx int
		ev  OutboxEvent
	}
	candidates := make([]ranked, 0, len(events))
	for i, event := range events {
		event = normalizeMemoryOutboxEvent(event)
		events[i] = event
		if event.MaxAttempts != nil && *event.MaxAttempts > 0 && event.AttemptCount >= *event.MaxAttempts {
			continue
		}
		due := (event.Status == domain.OutboxStatusPending || event.Status == domain.OutboxStatusFailed) && !event.NextAttemptAt.After(claimedAt)
		stale := event.Status == domain.OutboxStatusProcessing && event.ClaimExpiresAt != nil && !event.ClaimExpiresAt.After(claimedAt)
		if due || stale {
			candidates = append(candidates, ranked{idx: i, ev: event})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if !candidates[i].ev.NextAttemptAt.Equal(candidates[j].ev.NextAttemptAt) {
			return candidates[i].ev.NextAttemptAt.Before(candidates[j].ev.NextAttemptAt)
		}
		if candidates[i].ev.CreatedAt.Equal(candidates[j].ev.CreatedAt) {
			return candidates[i].ev.ID < candidates[j].ev.ID
		}
		return candidates[i].ev.CreatedAt.Before(candidates[j].ev.CreatedAt)
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	out := make([]OutboxEvent, 0, len(candidates))
	for _, item := range candidates {
		events[item.idx].Status = domain.OutboxStatusProcessing
		events[item.idx].AttemptCount++
		events[item.idx].LastError = ""
		events[item.idx].ClaimOwner = claimOwner
		events[item.idx].ClaimToken = claimToken + ":" + events[item.idx].ID
		expires := leaseUntil.UTC()
		events[item.idx].ClaimExpiresAt = &expires
		attemptedAt := claimedAt.UTC()
		events[item.idx].LastAttemptAt = &attemptedAt
		events[item.idx].UpdatedAt = attemptedAt
		events[item.idx].ProcessedAt = nil
		events[item.idx].DeadLetteredAt = nil
		out = append(out, copyOutboxEvent(events[item.idx]))
	}
	s.outboxEvents[tenantID] = events
	return out, nil
}

// FinalizeOutboxEvent updates only the worker's currently fenced claim.
func (s *Store) FinalizeOutboxEvent(_ context.Context, v OutboxEvent) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := s.outboxEvents[v.TenantID]
	for i := range events {
		if events[i].ID == v.ID && events[i].Status == domain.OutboxStatusProcessing && v.ClaimToken != "" && events[i].ClaimToken == v.ClaimToken {
			v.ClaimOwner = ""
			v.ClaimToken = ""
			v.ClaimExpiresAt = nil
			events[i] = copyOutboxEvent(v)
			s.outboxEvents[v.TenantID] = events
			return true, nil
		}
	}
	return false, nil
}

// RetryOutboxEvent resets an operator-selected terminal failure for immediate dispatch.
func (s *Store) RetryOutboxEvent(_ context.Context, tenantID, id string, retriedAt time.Time) (OutboxEvent, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := s.outboxEvents[tenantID]
	for i := range events {
		if events[i].ID != id {
			continue
		}
		switch events[i].Status {
		case domain.OutboxStatusFailed, domain.OutboxStatusParked, domain.OutboxStatusDeadLettered:
		default:
			return OutboxEvent{}, false, nil
		}
		events[i].Status = domain.OutboxStatusPending
		events[i].RetryCount = 0
		events[i].AttemptCount = 0
		events[i].LastError = ""
		events[i].NextAttemptAt = retriedAt.UTC()
		events[i].ClaimOwner = ""
		events[i].ClaimToken = ""
		events[i].ClaimExpiresAt = nil
		events[i].LastAttemptAt = nil
		events[i].UpdatedAt = retriedAt.UTC()
		events[i].ProcessedAt = nil
		events[i].DeadLetteredAt = nil
		s.outboxEvents[tenantID] = events
		return copyOutboxEvent(events[i]), true, nil
	}
	return OutboxEvent{}, false, nil
}

// DeleteSucceededOutboxEventsBefore 從儲存層刪除已成功且早於 cutoff 的 outbox 事件。
func (s *Store) DeleteSucceededOutboxEventsBefore(_ context.Context, tenantID string, before time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := s.outboxEvents[tenantID]
	kept := make([]OutboxEvent, 0, len(events))
	var deleted int64
	for _, v := range events {
		if v.Status == "succeeded" && v.CreatedAt.Before(before) {
			deleted++
			continue
		}
		kept = append(kept, v)
	}
	s.outboxEvents[tenantID] = kept
	return deleted, nil
}

// AddAccountGroup 從儲存層處理 add 帳號羣組。
func (s *Store) AddAccountGroup(_ context.Context, tenantID, accountID, groupID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	accountBucket := s.accounts[tenantID]
	account, ok := accountBucket[accountID]
	if !ok {
		return nil
	}
	if !utils.ContainsString(account.UserGroupIDs, groupID) {
		account.UserGroupIDs = append(account.UserGroupIDs, groupID)
		accountBucket[accountID] = account
	}
	return nil
}

// RemoveAccountGroup 從儲存層處理 remove 帳號羣組。
func (s *Store) RemoveAccountGroup(_ context.Context, tenantID, accountID, groupID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	accountBucket := s.accounts[tenantID]
	account, ok := accountBucket[accountID]
	if !ok {
		return nil
	}
	next := make([]string, 0, len(account.UserGroupIDs))
	for _, id := range account.UserGroupIDs {
		if id != groupID {
			next = append(next, id)
		}
	}
	account.UserGroupIDs = next
	accountBucket[accountID] = account
	return nil
}

// groupMembershipKey 取得使用者羣組成員關係 key。
func groupMembershipKey(userGroupID, accountID string) string {
	return userGroupID + "\x00" + accountID
}

// membershipActiveAt 判斷羣組成員關係在指定時間是否有效。
func membershipActiveAt(v GroupMembership, at time.Time) bool {
	if !v.ValidFrom.IsZero() && v.ValidFrom.After(at) {
		return false
	}
	return v.ValidUntil == nil || at.Before(*v.ValidUntil)
}

// refreshGroupMembershipProjectionLocked rebuilds legacy arrays from the authoritative membership relation.
func (s *Store) refreshGroupMembershipProjectionLocked(tenantID, userGroupID, accountID string, at time.Time) {
	if group, ok := s.userGroups[tenantID][userGroupID]; ok {
		members := make([]string, 0)
		for _, membership := range s.groupMemberships[tenantID] {
			if membership.UserGroupID == userGroupID && membershipActiveAt(membership, at) {
				members = append(members, membership.AccountID)
			}
		}
		group.MemberAccountIDs = uniqueSortedStrings(members)
		group.Version++
		s.userGroups[tenantID][userGroupID] = group
	}
	if account, ok := s.accounts[tenantID][accountID]; ok {
		groups := make([]string, 0)
		for _, membership := range s.groupMemberships[tenantID] {
			if membership.AccountID == accountID && membershipActiveAt(membership, at) {
				groups = append(groups, membership.UserGroupID)
			}
		}
		account.UserGroupIDs = uniqueSortedStrings(groups)
		account.Version++
		s.accounts[tenantID][accountID] = account
	}
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// putNested 處理 put nested。
func putNested[T any](bucket map[string]map[string]T, tenantID, id string, value T) {
	sub, ok := bucket[tenantID]
	if !ok {
		sub = map[string]T{}
		bucket[tenantID] = sub
	}
	sub[id] = value
}

// getNested 取得 nested。
func getNested[T any](bucket map[string]map[string]T, tenantID, id string) (T, bool) {
	var zero T
	sub, ok := bucket[tenantID]
	if !ok {
		return zero, false
	}
	v, ok := sub[id]
	if !ok {
		return zero, false
	}
	return v, true
}

// copyNestedValues 複製 nested values。
func copyNestedValues[T any](bucket map[string]T, clone func(T) T) []T {
	if len(bucket) == 0 {
		return []T{}
	}
	out := make([]T, 0, len(bucket))
	for _, v := range bucket {
		out = append(out, clone(v))
	}
	return out
}

// identityKey 處理身分 key。
func identityKey(provider, subject string) string {
	return strings.TrimSpace(provider) + "\x00" + strings.TrimSpace(subject)
}

// nowUTC 處理 now utc。
func nowUTC() time.Time {
	return time.Now().UTC()
}
func (s *Store) UpsertWorkflowRun(_ context.Context, v domain.WorkflowRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.TemporalStartStatus == "" {
		v.TemporalStartStatus = domain.WorkflowTemporalStartStarted
	}
	if v.TemporalWorkflowID == "" {
		v.TemporalWorkflowID = domain.FormApprovalWorkflowID(v.TenantID, v.FormInstanceID)
	}
	if s.workflowRuns == nil {
		s.workflowRuns = map[string]map[string]domain.WorkflowRun{}
	}
	putNested(s.workflowRuns, v.TenantID, v.ID, copyWorkflowRun(v))
	return nil
}

func (s *Store) GetWorkflowRun(_ context.Context, tenantID, id string) (domain.WorkflowRun, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.workflowRuns, tenantID, id)
	return copyWorkflowRun(v), ok, nil
}

func (s *Store) GetWorkflowRunByFormInstance(_ context.Context, tenantID, formInstanceID string) (domain.WorkflowRun, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := s.workflowRuns[tenantID]
	var latest domain.WorkflowRun
	found := false
	for _, item := range items {
		if item.FormInstanceID != formInstanceID {
			continue
		}
		if !found || item.Version > latest.Version || (item.Version == latest.Version && item.UpdatedAt.After(latest.UpdatedAt)) {
			latest = item
			found = true
		}
	}
	return copyWorkflowRun(latest), found, nil
}

func (s *Store) ListWorkflowRunsByFormInstance(_ context.Context, tenantID, formInstanceID string) ([]domain.WorkflowRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.WorkflowRun, 0)
	for _, item := range s.workflowRuns[tenantID] {
		if item.FormInstanceID == formInstanceID {
			out = append(out, copyWorkflowRun(item))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Version == out[j].Version {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].Version < out[j].Version
	})
	return out, nil
}

func (s *Store) ListPendingWorkflowRuns(_ context.Context, tenantID string, staleBefore time.Time, limit int) ([]domain.WorkflowRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.WorkflowRun, 0)
	for _, item := range s.workflowRuns[tenantID] {
		if item.TemporalStartStatus == domain.WorkflowTemporalStartPending ||
			(item.TemporalStartStatus == domain.WorkflowTemporalStartStarting && !item.UpdatedAt.After(staleBefore)) {
			out = append(out, copyWorkflowRun(item))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.Before(out[j].UpdatedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) ClaimWorkflowRunTemporalStart(_ context.Context, tenantID, id string, claimedAt, staleBefore time.Time) (domain.WorkflowRun, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := getNested(s.workflowRuns, tenantID, id)
	if !ok || (item.TemporalStartStatus != domain.WorkflowTemporalStartPending &&
		!(item.TemporalStartStatus == domain.WorkflowTemporalStartStarting && !item.UpdatedAt.After(staleBefore))) {
		return domain.WorkflowRun{}, false, nil
	}
	item.TemporalStartStatus = domain.WorkflowTemporalStartStarting
	item.UpdatedAt = claimedAt
	putNested(s.workflowRuns, tenantID, id, copyWorkflowRun(item))
	return copyWorkflowRun(item), true, nil
}

func (s *Store) ReleaseWorkflowRunTemporalStart(_ context.Context, tenantID, id string, claimedAt, releasedAt time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := getNested(s.workflowRuns, tenantID, id)
	if !ok || item.TemporalStartStatus != domain.WorkflowTemporalStartStarting || !item.UpdatedAt.Equal(claimedAt) {
		return false, nil
	}
	item.TemporalStartStatus = domain.WorkflowTemporalStartPending
	item.UpdatedAt = releasedAt
	putNested(s.workflowRuns, tenantID, id, copyWorkflowRun(item))
	return true, nil
}

func (s *Store) MarkWorkflowRunTemporalStarted(_ context.Context, tenantID, id string, claimedAt time.Time, execution domain.FormApprovalWorkflowExecution, startedAt time.Time) (domain.WorkflowRun, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := getNested(s.workflowRuns, tenantID, id)
	if !ok || item.TemporalStartStatus != domain.WorkflowTemporalStartStarting || !item.UpdatedAt.Equal(claimedAt) {
		return domain.WorkflowRun{}, false, nil
	}
	item.TemporalStartStatus = domain.WorkflowTemporalStartStarted
	item.TemporalWorkflowID = execution.WorkflowID
	item.TemporalRunID = execution.RunID
	item.TemporalStartedAt = &startedAt
	item.UpdatedAt = startedAt
	putNested(s.workflowRuns, tenantID, id, copyWorkflowRun(item))
	return copyWorkflowRun(item), true, nil
}

func (s *Store) AbandonPendingWorkflowRunTemporalStart(_ context.Context, tenantID, id string, abandonedAt time.Time) (domain.WorkflowRun, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := getNested(s.workflowRuns, tenantID, id)
	if !ok || item.TemporalStartStatus != domain.WorkflowTemporalStartPending {
		return domain.WorkflowRun{}, false, nil
	}
	item.TemporalStartStatus = domain.WorkflowTemporalStartAbandoned
	item.UpdatedAt = abandonedAt
	putNested(s.workflowRuns, tenantID, id, copyWorkflowRun(item))
	return copyWorkflowRun(item), true, nil
}

func (s *Store) AbandonClaimedWorkflowRunTemporalStart(_ context.Context, tenantID, id string, claimedAt, abandonedAt time.Time) (domain.WorkflowRun, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := getNested(s.workflowRuns, tenantID, id)
	if !ok || item.TemporalStartStatus != domain.WorkflowTemporalStartStarting || !item.UpdatedAt.Equal(claimedAt) {
		return domain.WorkflowRun{}, false, nil
	}
	item.TemporalStartStatus = domain.WorkflowTemporalStartAbandoned
	item.UpdatedAt = abandonedAt
	putNested(s.workflowRuns, tenantID, id, copyWorkflowRun(item))
	return copyWorkflowRun(item), true, nil
}

func (s *Store) UpsertWorkflowStageInstance(_ context.Context, v domain.WorkflowStageInstance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.workflowStageInstances == nil {
		s.workflowStageInstances = map[string]map[string]domain.WorkflowStageInstance{}
	}
	putNested(s.workflowStageInstances, v.TenantID, v.ID, copyWorkflowStageInstance(v))
	return nil
}

func (s *Store) GetWorkflowStageInstance(_ context.Context, tenantID, id string) (domain.WorkflowStageInstance, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.workflowStageInstances, tenantID, id)
	return copyWorkflowStageInstance(v), ok, nil
}

func (s *Store) GetWorkflowStageInstanceForUpdate(ctx context.Context, tenantID, id string) (domain.WorkflowStageInstance, bool, error) {
	return s.GetWorkflowStageInstance(ctx, tenantID, id)
}

func (s *Store) ListWorkflowStageInstancesByRun(_ context.Context, tenantID, runID string) ([]domain.WorkflowStageInstance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.WorkflowStageInstance, 0)
	for _, item := range s.workflowStageInstances[tenantID] {
		if item.RunID == runID {
			out = append(out, copyWorkflowStageInstance(item))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	return out, nil
}

func (s *Store) UpsertWorkflowStageAssignee(_ context.Context, v domain.WorkflowStageAssignee) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.workflowStageAssignees == nil {
		s.workflowStageAssignees = map[string]map[string]domain.WorkflowStageAssignee{}
	}
	key := workflowAssigneeKey(v.StageInstanceID, v.AccountID)
	putNested(s.workflowStageAssignees, v.TenantID, key, copyWorkflowStageAssignee(v))
	return nil
}

func (s *Store) ListWorkflowStageAssignees(_ context.Context, tenantID, stageInstanceID string) ([]domain.WorkflowStageAssignee, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.WorkflowStageAssignee, 0)
	prefix := stageInstanceID + ":"
	for key, item := range s.workflowStageAssignees[tenantID] {
		if strings.HasPrefix(key, prefix) {
			out = append(out, copyWorkflowStageAssignee(item))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].AccountID < out[j].AccountID })
	return out, nil
}

func (s *Store) ListPendingAssigneeStageInstanceIDs(_ context.Context, tenantID, accountID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, item := range s.workflowStageAssignees[tenantID] {
		if item.AccountID != accountID || item.Status != domain.WorkflowAssigneeStatusPending {
			continue
		}
		if _, ok := seen[item.StageInstanceID]; ok {
			continue
		}
		seen[item.StageInstanceID] = struct{}{}
		out = append(out, item.StageInstanceID)
	}
	sort.Strings(out)
	return out, nil
}

func (s *Store) InsertWorkflowAction(_ context.Context, v domain.WorkflowAction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.workflowActions == nil {
		s.workflowActions = map[string][]domain.WorkflowAction{}
	}
	s.workflowActions[v.TenantID] = append(s.workflowActions[v.TenantID], copyWorkflowAction(v))
	return nil
}

func (s *Store) GetWorkflowActionByIdempotencyKey(_ context.Context, tenantID, runID, idempotencyKey string) (domain.WorkflowAction, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.workflowActions[tenantID] {
		if item.RunID == runID && item.IdempotencyKey == idempotencyKey && idempotencyKey != "" {
			return copyWorkflowAction(item), true, nil
		}
	}
	return domain.WorkflowAction{}, false, nil
}

func (s *Store) ListWorkflowActionsByRun(_ context.Context, tenantID, runID string) ([]domain.WorkflowAction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.WorkflowAction, 0)
	for _, item := range s.workflowActions[tenantID] {
		if item.RunID == runID {
			out = append(out, copyWorkflowAction(item))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func workflowAssigneeKey(stageInstanceID, accountID string) string {
	return stageInstanceID + ":" + accountID
}

func copyWorkflowRun(v domain.WorkflowRun) domain.WorkflowRun { return v }

func copyWorkflowStageInstance(v domain.WorkflowStageInstance) domain.WorkflowStageInstance {
	if len(v.Result) > 0 {
		next := make(map[string]any, len(v.Result))
		for key, value := range v.Result {
			next[key] = value
		}
		v.Result = next
	}
	return v
}

func copyWorkflowStageAssignee(v domain.WorkflowStageAssignee) domain.WorkflowStageAssignee { return v }

func copyWorkflowAction(v domain.WorkflowAction) domain.WorkflowAction { return v }
