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

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

// Store 定義儲存層的資料結構。
type Store struct {
	mu sync.RWMutex

	tenants map[string]Tenant

	accounts                map[string]map[string]Account
	userIdentities          map[string]map[string]UserIdentity
	userGroups              map[string]map[string]UserGroup
	groupMemberships        map[string]map[string]GroupMembership
	permissionSets          map[string]map[string]PermissionSet
	permissionPackages      map[string]PermissionPackage
	permissionSetTemplates  map[string]map[string]PermissionSetTemplate
	userGroupTemplates      map[string]map[string]UserGroupTemplate
	assumableRoleTemplates  map[string]map[string]AssumableRoleTemplate
	permissionImports       map[string]map[string]PermissionPackageImport
	permissionCatalog       map[string]map[string]PermissionCatalogItem
	menuItems               map[string]map[string]MenuItem
	permissionSetItems      map[string]map[string]PermissionSetItem
	assignments             map[string]map[string]PermissionSetAssignment
	dataScopes              map[string]map[string]DataScope
	fieldPolicies           map[string]map[string]FieldPolicy
	assumableRoles          map[string]map[string]AssumableRole
	roleSessions            map[string]map[string]AssumableRoleSession
	orgUnits                map[string]map[string]OrgUnit
	positions               map[string]map[string]Position
	employees               map[string]map[string]Employee
	employeeNoSequences     map[string]map[string]int
	employeeImports         map[string]map[string]EmployeeImportSession
	employmentContracts     map[string]map[string]EmploymentContract
	attendancePolicies      map[string]AttendancePolicy
	leaveBalances           map[string]map[string]LeaveBalance
	leaveRequests           map[string]map[string]LeaveRequest
	attendanceWorksites     map[string]map[string]AttendanceWorksite
	attendanceShifts        map[string]map[string]AttendanceShift
	attendanceAssignments   map[string]map[string]AttendanceShiftAssignment
	attendanceClockRecords  map[string]map[string]AttendanceClockRecord
	attendanceSummaries     map[string]map[string]AttendanceDailySummary
	attendanceCorrections   map[string]map[string]AttendanceCorrectionRequest
	overtimeRequests        map[string]map[string]OvertimeRequest
	formDefinitionDrafts    map[string]map[string]domain.FormDefinitionDraft
	formTemplates           map[string]map[string]FormTemplate
	formTemplateVersions    map[string]map[string]FormTemplateVersion
	formInstances           map[string]map[string]FormInstance
	formInstanceFieldValues map[string]map[string][]FormInstanceFieldValue
	workflowRuns            map[string]map[string]domain.WorkflowRun
	workflowStageInstances  map[string]map[string]domain.WorkflowStageInstance
	workflowStageAssignees  map[string]map[string]domain.WorkflowStageAssignee
	workflowActions         map[string][]domain.WorkflowAction
	platformTaskItems       map[string]map[string]PlatformTaskRecordItem
	platformTaskTodos       map[string]map[string]PlatformTaskTodoRecord
	agentRuns               map[string]map[string]AgentRun
	agentModels             map[string]map[string]AgentModel
	agentExternalTools      map[string]map[string]AgentExternalTool
	agentDefinitions        map[string]map[string]AgentDefinition
	agentDefinitionVersions map[string]map[string]AgentDefinitionVersion
	agentAudits             map[string][]AgentAudit
	knowledgeBases          map[string]map[string]KnowledgeBase
	knowledgeDocuments      map[string]map[string]KnowledgeDocument
	knowledgeDocumentChunks map[string]map[string]KnowledgeDocumentChunk
	agentSessions           map[string]map[string]AgentSession
	agentSessionMessages    map[string]map[string]AgentSessionMessage
	agentSessionFiles       map[string]map[string]domain.AgentSessionFile
	agentFileChunks         map[string]map[string][]string
	agentMessageAttachments map[string]map[string][]domain.AgentMessageAttachment
	agentMemories           map[string]map[string]AgentMemory
	notifications           map[string]map[string]Notification
	notificationRecipients  map[string]map[string]NotificationRecipient
	auditLogs               map[string][]AuditLog
	permissionVersions      map[string]int64
	identityOutbox          map[string][]IdentityProvisioningOutboxEvent
	outboxEvents            map[string][]OutboxEvent
	relationshipTuples      map[string]map[string]AuthzRelationshipTuple
	ehrmsSyncRuns           map[string]map[string]domain.EHRMSSyncRun
	ehrmsSyncRunSteps       map[string]map[string]domain.EHRMSSyncRunStep
	ehrmsSyncLocks          map[string]bool
}

// NewStore 建立儲存層。
func NewStore() *Store {
	return &Store{
		tenants:                 map[string]Tenant{},
		accounts:                map[string]map[string]Account{},
		userIdentities:          map[string]map[string]UserIdentity{},
		userGroups:              map[string]map[string]UserGroup{},
		groupMemberships:        map[string]map[string]GroupMembership{},
		permissionSets:          map[string]map[string]PermissionSet{},
		permissionPackages:      map[string]PermissionPackage{},
		permissionSetTemplates:  map[string]map[string]PermissionSetTemplate{},
		userGroupTemplates:      map[string]map[string]UserGroupTemplate{},
		assumableRoleTemplates:  map[string]map[string]AssumableRoleTemplate{},
		permissionImports:       map[string]map[string]PermissionPackageImport{},
		permissionCatalog:       map[string]map[string]PermissionCatalogItem{},
		menuItems:               map[string]map[string]MenuItem{},
		permissionSetItems:      map[string]map[string]PermissionSetItem{},
		assignments:             map[string]map[string]PermissionSetAssignment{},
		dataScopes:              map[string]map[string]DataScope{},
		fieldPolicies:           map[string]map[string]FieldPolicy{},
		assumableRoles:          map[string]map[string]AssumableRole{},
		roleSessions:            map[string]map[string]AssumableRoleSession{},
		orgUnits:                map[string]map[string]OrgUnit{},
		positions:               map[string]map[string]Position{},
		employees:               map[string]map[string]Employee{},
		employeeNoSequences:     map[string]map[string]int{},
		employeeImports:         map[string]map[string]EmployeeImportSession{},
		employmentContracts:     map[string]map[string]EmploymentContract{},
		attendancePolicies:      map[string]AttendancePolicy{},
		leaveBalances:           map[string]map[string]LeaveBalance{},
		leaveRequests:           map[string]map[string]LeaveRequest{},
		attendanceWorksites:     map[string]map[string]AttendanceWorksite{},
		attendanceShifts:        map[string]map[string]AttendanceShift{},
		attendanceAssignments:   map[string]map[string]AttendanceShiftAssignment{},
		attendanceClockRecords:  map[string]map[string]AttendanceClockRecord{},
		attendanceSummaries:     map[string]map[string]AttendanceDailySummary{},
		attendanceCorrections:   map[string]map[string]AttendanceCorrectionRequest{},
		overtimeRequests:        map[string]map[string]OvertimeRequest{},
		formDefinitionDrafts:    map[string]map[string]domain.FormDefinitionDraft{},
		formTemplates:           map[string]map[string]FormTemplate{},
		formTemplateVersions:    map[string]map[string]FormTemplateVersion{},
		formInstances:           map[string]map[string]FormInstance{},
		formInstanceFieldValues: map[string]map[string][]FormInstanceFieldValue{},
		workflowRuns:            map[string]map[string]domain.WorkflowRun{},
		workflowStageInstances:  map[string]map[string]domain.WorkflowStageInstance{},
		workflowStageAssignees:  map[string]map[string]domain.WorkflowStageAssignee{},
		workflowActions:         map[string][]domain.WorkflowAction{},
		platformTaskItems:       map[string]map[string]PlatformTaskRecordItem{},
		platformTaskTodos:       map[string]map[string]PlatformTaskTodoRecord{},
		agentRuns:               map[string]map[string]AgentRun{},
		agentModels:             map[string]map[string]AgentModel{},
		agentExternalTools:      map[string]map[string]AgentExternalTool{},
		agentDefinitions:        map[string]map[string]AgentDefinition{},
		agentDefinitionVersions: map[string]map[string]AgentDefinitionVersion{},
		agentAudits:             map[string][]AgentAudit{},
		knowledgeBases:          map[string]map[string]KnowledgeBase{},
		knowledgeDocuments:      map[string]map[string]KnowledgeDocument{},
		knowledgeDocumentChunks: map[string]map[string]KnowledgeDocumentChunk{},
		agentSessions:           map[string]map[string]AgentSession{},
		agentSessionMessages:    map[string]map[string]AgentSessionMessage{},
		agentSessionFiles:       map[string]map[string]domain.AgentSessionFile{},
		agentFileChunks:         map[string]map[string][]string{},
		agentMessageAttachments: map[string]map[string][]domain.AgentMessageAttachment{},
		agentMemories:           map[string]map[string]AgentMemory{},
		notifications:           map[string]map[string]Notification{},
		notificationRecipients:  map[string]map[string]NotificationRecipient{},
		auditLogs:               map[string][]AuditLog{},
		permissionVersions:      map[string]int64{},
		identityOutbox:          map[string][]IdentityProvisioningOutboxEvent{},
		outboxEvents:            map[string][]OutboxEvent{},
		relationshipTuples:      map[string]map[string]AuthzRelationshipTuple{},
		ehrmsSyncRuns:           map[string]map[string]domain.EHRMSSyncRun{},
		ehrmsSyncRunSteps:       map[string]map[string]domain.EHRMSSyncRunStep{},
		ehrmsSyncLocks:          map[string]bool{},
	}
}

// UpsertTenant 從儲存層處理 upsert 租戶。
func (s *Store) UpsertTenant(_ context.Context, v Tenant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tenants[v.ID] = copyTenant(v)
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

// UpsertUserGroup 從儲存層處理 upsert 使用者群組。Version > 0 時執行樂觀鎖檢查。
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

// GetUserGroup 從儲存層取得使用者群組。
func (s *Store) GetUserGroup(_ context.Context, tenantID, id string) (UserGroup, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.userGroups, tenantID, id)
	if !ok {
		return UserGroup{}, false, nil
	}
	return copyUserGroup(v), true, nil
}

// ListUserGroups 從儲存層列出使用者群組。
func (s *Store) ListUserGroups(_ context.Context, tenantID string) ([]UserGroup, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.userGroups[tenantID], copyUserGroup)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// DeleteUserGroup 從儲存層刪除使用者群組。
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

// UpsertGroupMembership 從儲存層處理 upsert 使用者群組成員關係。
func (s *Store) UpsertGroupMembership(_ context.Context, v GroupMembership) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := groupMembershipKey(v.UserGroupID, v.AccountID)
	if existing, ok := getNested(s.groupMemberships, v.TenantID, key); ok && v.ID == "" {
		v.ID = existing.ID
	}
	putNested(s.groupMemberships, v.TenantID, key, copyGroupMembership(v))
	return nil
}

// DeleteGroupMembership 從儲存層刪除使用者群組成員關係。
func (s *Store) DeleteGroupMembership(_ context.Context, tenantID, userGroupID, accountID string) (GroupMembership, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := groupMembershipKey(userGroupID, accountID)
	bucket := s.groupMemberships[tenantID]
	if bucket == nil {
		return GroupMembership{}, false, nil
	}
	v, ok := bucket[key]
	if !ok {
		return GroupMembership{}, false, nil
	}
	delete(bucket, key)
	return copyGroupMembership(v), true, nil
}

// GetGroupMembership 從儲存層取得使用者群組成員關係。
func (s *Store) GetGroupMembership(_ context.Context, tenantID, userGroupID, accountID string) (GroupMembership, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.groupMemberships, tenantID, groupMembershipKey(userGroupID, accountID))
	if !ok {
		return GroupMembership{}, false, nil
	}
	return copyGroupMembership(v), true, nil
}

// ListGroupMembershipsForGroup 從儲存層列出使用者群組成員關係。
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

// ListActiveGroupMembershipsForAccount 從儲存層列出帳號有效使用者群組成員關係。
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

// UpsertUserGroupTemplate 從儲存層處理 upsert 使用者群組模板。
func (s *Store) UpsertUserGroupTemplate(_ context.Context, v UserGroupTemplate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.userGroupTemplates, v.PackageID, v.TemplateKey, copyUserGroupTemplate(v))
	return nil
}

// ListUserGroupTemplates 從儲存層列出使用者群組模板。
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
	putNested(s.orgUnits, v.TenantID, v.ID, copyOrgUnit(v))
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
	putNested(s.employees, v.TenantID, v.ID, copyEmployee(v))
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

// NextEmployeeNo 從儲存層處理 next 員工 no。
func (s *Store) NextEmployeeNo(_ context.Context, tenantID, prefix string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	maxSeq := s.employeeNoSequences[tenantID][prefix]
	for _, employee := range s.employees[tenantID] {
		if seq, ok := memoryEmployeeNoSequence(employee.EmployeeNo, prefix); ok && seq > maxSeq {
			maxSeq = seq
		}
	}
	nextSeq := maxSeq + 1
	if s.employeeNoSequences[tenantID] == nil {
		s.employeeNoSequences[tenantID] = map[string]int{}
	}
	s.employeeNoSequences[tenantID][prefix] = nextSeq
	return fmt.Sprintf("%s%03d", prefix, nextSeq), nil
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
		if len(employeeAllowed) > 0 {
			if _, ok := employeeAllowed[item.ID]; !ok {
				continue
			}
		}
		if len(orgAllowed) > 0 {
			if _, ok := orgAllowed[item.OrgUnitID]; !ok {
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
func memoryFormInstanceMatches(item FormInstance, templateKey string, query domain.FormInstanceQuery) bool {
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

// memoryEmployeeNoSequence 處理 memory 員工 no sequence。
func memoryEmployeeNoSequence(employeeNo, prefix string) (int, bool) {
	employeeNo = strings.TrimSpace(employeeNo)
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || !strings.HasPrefix(employeeNo, prefix) {
		return 0, false
	}
	seq, err := strconv.Atoi(strings.TrimPrefix(employeeNo, prefix))
	if err != nil {
		return 0, false
	}
	return seq, true
}

// UpsertEmployeeImportSession 從儲存層處理 upsert 員工 import session。
func (s *Store) UpsertEmployeeImportSession(_ context.Context, v EmployeeImportSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.employeeImports, v.TenantID, v.ID, copyEmployeeImportSession(v))
	return nil
}

// GetEmployeeImportSession 從儲存層取得員工 import session。
func (s *Store) GetEmployeeImportSession(_ context.Context, tenantID, id string) (EmployeeImportSession, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.employeeImports, tenantID, id)
	if !ok {
		return EmployeeImportSession{}, false, nil
	}
	return copyEmployeeImportSession(v), true, nil
}

// UpsertEmploymentContract 從儲存層處理 upsert 員工合約。
func (s *Store) UpsertEmploymentContract(_ context.Context, v EmploymentContract) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.employmentContracts, v.TenantID, v.ID, copyEmploymentContract(v))
	return nil
}

// GetEmploymentContract 從儲存層取得員工合約。
func (s *Store) GetEmploymentContract(_ context.Context, tenantID, id string) (EmploymentContract, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.employmentContracts, tenantID, id)
	if !ok {
		return EmploymentContract{}, false, nil
	}
	return copyEmploymentContract(v), true, nil
}

// ListEmploymentContracts 從儲存層列出員工合約。
func (s *Store) ListEmploymentContracts(_ context.Context, tenantID string) ([]EmploymentContract, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.employmentContracts[tenantID], copyEmploymentContract)
	sortEmploymentContracts(out)
	return out, nil
}

// ListEmploymentContractsByEmployee 從儲存層列出員工合約 by 員工。
func (s *Store) ListEmploymentContractsByEmployee(_ context.Context, tenantID, employeeID string) ([]EmploymentContract, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]EmploymentContract, 0)
	for _, v := range s.employmentContracts[tenantID] {
		if v.EmployeeID == employeeID {
			out = append(out, copyEmploymentContract(v))
		}
	}
	sortEmploymentContracts(out)
	return out, nil
}

// sortEmploymentContracts 排序員工合約。
func sortEmploymentContracts(items []EmploymentContract) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].StartDate.Equal(items[j].StartDate) {
			if items[i].CreatedAt.Equal(items[j].CreatedAt) {
				return items[i].ID < items[j].ID
			}
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].StartDate.After(items[j].StartDate)
	})
}

// UpsertAttendancePolicy 從儲存層處理 upsert 考勤政策。
func (s *Store) UpsertAttendancePolicy(_ context.Context, v AttendancePolicy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attendancePolicies[v.TenantID] = copyAttendancePolicy(v)
	return nil
}

// GetAttendancePolicy 從儲存層取得考勤政策。
func (s *Store) GetAttendancePolicy(_ context.Context, tenantID string) (AttendancePolicy, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.attendancePolicies[tenantID]
	if !ok {
		return AttendancePolicy{}, false, nil
	}
	return copyAttendancePolicy(v), true, nil
}

// UpsertLeaveBalance 從儲存層處理 upsert 請假 balance。
func (s *Store) UpsertLeaveBalance(_ context.Context, v LeaveBalance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, existing := range s.leaveBalances[v.TenantID] {
		if existing.EmployeeID == v.EmployeeID && existing.LeaveType == v.LeaveType {
			v.ID = id
			break
		}
	}
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

// ListLeaveBalances 從儲存層列出請假 balances。
func (s *Store) ListLeaveBalances(_ context.Context, tenantID string) ([]LeaveBalance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.leaveBalances[tenantID], copyLeaveBalance)
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.Before(out[j].UpdatedAt) })
	return out, nil
}

// ReserveLeaveBalance 從儲存層保留請假 balance。
func (s *Store) ReserveLeaveBalance(_ context.Context, tenantID, employeeID, leaveType string, hours float64, updatedAt time.Time) (LeaveBalance, bool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	leaveType = strings.TrimSpace(leaveType)
	for id, balance := range s.leaveBalances[tenantID] {
		if balance.EmployeeID != employeeID || !strings.EqualFold(balance.LeaveType, leaveType) {
			continue
		}
		if balance.RemainingHours < hours {
			return copyLeaveBalance(balance), false, true, nil
		}
		balance.RemainingHours -= hours
		balance.UsedHours += hours
		balance.UpdatedAt = updatedAt
		s.leaveBalances[tenantID][id] = copyLeaveBalance(balance)
		return copyLeaveBalance(balance), true, true, nil
	}
	return LeaveBalance{}, false, false, nil
}

// ReleaseLeaveBalance 從儲存層釋放請假 balance。
func (s *Store) ReleaseLeaveBalance(_ context.Context, tenantID, employeeID, leaveType string, hours float64, updatedAt time.Time) (LeaveBalance, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	leaveType = strings.TrimSpace(leaveType)
	for id, balance := range s.leaveBalances[tenantID] {
		if balance.EmployeeID != employeeID || !strings.EqualFold(balance.LeaveType, leaveType) {
			continue
		}
		balance.RemainingHours += hours
		if balance.UsedHours > hours {
			balance.UsedHours -= hours
		} else {
			balance.UsedHours = 0
		}
		balance.UpdatedAt = updatedAt
		s.leaveBalances[tenantID][id] = copyLeaveBalance(balance)
		return copyLeaveBalance(balance), true, nil
	}
	return LeaveBalance{}, false, nil
}

// UpsertLeaveRequest 從儲存層處理 upsert 請假請求。
func (s *Store) UpsertLeaveRequest(_ context.Context, v LeaveRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.leaveRequests, v.TenantID, v.ID, copyLeaveRequest(v))
	return nil
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

// UpsertAttendanceShift 從儲存層處理 upsert 考勤班別。
func (s *Store) UpsertAttendanceShift(_ context.Context, v AttendanceShift) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.attendanceShifts, v.TenantID, v.ID, copyAttendanceShift(v))
	return nil
}

// GetAttendanceShift 從儲存層取得考勤班別。
func (s *Store) GetAttendanceShift(_ context.Context, tenantID, id string) (AttendanceShift, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.attendanceShifts, tenantID, id)
	if !ok {
		return AttendanceShift{}, false, nil
	}
	return copyAttendanceShift(v), true, nil
}

// ListAttendanceShifts 從儲存層列出考勤 shifts。
func (s *Store) ListAttendanceShifts(_ context.Context, tenantID string) ([]AttendanceShift, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.attendanceShifts[tenantID], copyAttendanceShift)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// UpsertAttendanceShiftAssignment 儲存員工班別指派。
func (s *Store) UpsertAttendanceShiftAssignment(_ context.Context, v AttendanceShiftAssignment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.attendanceAssignments, v.TenantID, v.ID, copyAttendanceShiftAssignment(v))
	return nil
}

// ListAttendanceShiftAssignments 列出租戶的員工班別指派。
func (s *Store) ListAttendanceShiftAssignments(_ context.Context, tenantID string) ([]AttendanceShiftAssignment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.attendanceAssignments[tenantID], copyAttendanceShiftAssignment)
	sort.Slice(out, func(i, j int) bool { return out[i].EffectiveFrom.After(out[j].EffectiveFrom) })
	return out, nil
}

// FindEffectiveAttendanceShiftAssignment 取得指定時點生效的員工排班。
func (s *Store) FindEffectiveAttendanceShiftAssignment(_ context.Context, tenantID, employeeID string, at time.Time) (AttendanceShiftAssignment, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var best AttendanceShiftAssignment
	found := false
	for _, item := range s.attendanceAssignments[tenantID] {
		if item.EmployeeID != employeeID || !strings.EqualFold(item.Status, "active") || item.EffectiveFrom.After(at) || (item.EffectiveTo != nil && item.EffectiveTo.Before(at)) {
			continue
		}
		if !found || item.EffectiveFrom.After(best.EffectiveFrom) {
			best, found = item, true
		}
	}
	if !found {
		return AttendanceShiftAssignment{}, false, nil
	}
	return copyAttendanceShiftAssignment(best), true, nil
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
	for _, item := range s.attendanceSummaries[v.TenantID] {
		if item.ID != v.ID && item.EmployeeID == v.EmployeeID && item.WorkDate == v.WorkDate {
			return domain.Conflict("attendance daily summary already exists")
		}
		if item.ID != v.ID && v.ExternalRef != "" && item.ExternalRef == v.ExternalRef {
			return domain.Conflict("attendance daily summary external_ref already exists")
		}
	}
	putNested(s.attendanceSummaries, v.TenantID, v.ID, copyAttendanceDailySummary(v))
	return nil
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

// GetFormDefinitionDraftByAgentCall 以 Agent run/tool call 实现幂等重试。
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

// ListFormDefinitionDrafts 列出指定拥有者与状态的草稿。
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
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC()
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = v.CreatedAt
	}
	putNested(s.formTemplates, v.TenantID, v.ID, copyFormTemplate(v))
	versionKey := fmt.Sprintf("%s:%d", v.ID, v.CurrentVersion)
	if _, exists := getNested(s.formTemplateVersions, v.TenantID, versionKey); !exists {
		version := FormTemplateVersion{
			ID: utils.NewID("ftv"), TenantID: v.TenantID, TemplateID: v.ID, Version: v.CurrentVersion,
			Schema: utils.CopyStringMap(v.Schema), Status: v.Status, CreatedAt: v.UpdatedAt,
		}
		if v.Status == "published" {
			publishedAt := v.UpdatedAt
			version.PublishedAt = &publishedAt
		}
		putNested(s.formTemplateVersions, v.TenantID, versionKey, version)
	}
	return nil
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
	if _, exists := getNested(s.formTemplateVersions, v.TenantID, key); exists {
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
			version, versionExists := getNested(s.formTemplateVersions, v.TenantID, fmt.Sprintf("%s:%d", v.TemplateID, template.CurrentVersion))
			if !versionExists {
				return fmt.Errorf("form template version %s:%d not found", v.TemplateID, template.CurrentVersion)
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
	if strings.TrimSpace(query.TemplateKey) != "" {
		templates, err := s.ListFormTemplates(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		for _, template := range templates {
			templateKeys[template.ID] = template.Key
		}
	}
	out := make([]FormInstance, 0, len(items))
	for _, item := range items {
		if memoryFormInstanceMatches(item, templateKeys[item.TemplateID], query) {
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
	return nil
}

// UpsertPlatformTaskItem 從儲存層處理 upsert 平台任務項目。
func (s *Store) UpsertPlatformTaskItem(_ context.Context, v PlatformTaskRecordItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := getNested(s.platformTaskItems, v.TenantID, v.ID); ok && current.AccountID != v.AccountID {
		return domain.Conflict("platform task item belongs to another account")
	}
	putNested(s.platformTaskItems, v.TenantID, v.ID, copyPlatformTaskRecordItem(v))
	return nil
}

// GetPlatformTaskItem 從儲存層取得平台任務項目。
func (s *Store) GetPlatformTaskItem(_ context.Context, tenantID, accountID, id string) (PlatformTaskRecordItem, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.platformTaskItems, tenantID, id)
	if !ok || v.AccountID != accountID {
		return PlatformTaskRecordItem{}, false, nil
	}
	return copyPlatformTaskRecordItem(v), true, nil
}

// ListPlatformTaskItems 從儲存層列出平台任務項目。
func (s *Store) ListPlatformTaskItems(_ context.Context, tenantID, accountID string) ([]PlatformTaskRecordItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PlatformTaskRecordItem, 0)
	for _, v := range s.platformTaskItems[tenantID] {
		if v.AccountID == accountID {
			out = append(out, copyPlatformTaskRecordItem(v))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].WorkDate == out[j].WorkDate {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].WorkDate > out[j].WorkDate
	})
	return out, nil
}

// DeletePlatformTaskItem 從儲存層刪除平台任務項目。
func (s *Store) DeletePlatformTaskItem(_ context.Context, tenantID, accountID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := getNested(s.platformTaskItems, tenantID, id); ok && current.AccountID == accountID {
		delete(s.platformTaskItems[tenantID], id)
	}
	return nil
}

// UpsertPlatformTaskTodo 從儲存層處理 upsert 平台任務待辦。
func (s *Store) UpsertPlatformTaskTodo(_ context.Context, v PlatformTaskTodoRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := getNested(s.platformTaskTodos, v.TenantID, v.ID); ok && current.AccountID != v.AccountID {
		return domain.Conflict("platform task todo belongs to another account")
	}
	putNested(s.platformTaskTodos, v.TenantID, v.ID, copyPlatformTaskTodoRecord(v))
	return nil
}

// GetPlatformTaskTodo 從儲存層取得平台任務待辦。
func (s *Store) GetPlatformTaskTodo(_ context.Context, tenantID, accountID, id string) (PlatformTaskTodoRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.platformTaskTodos, tenantID, id)
	if !ok || v.AccountID != accountID {
		return PlatformTaskTodoRecord{}, false, nil
	}
	return copyPlatformTaskTodoRecord(v), true, nil
}

// ListPlatformTaskTodos 從儲存層列出平台任務待辦。
func (s *Store) ListPlatformTaskTodos(_ context.Context, tenantID, accountID string) ([]PlatformTaskTodoRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PlatformTaskTodoRecord, 0)
	for _, v := range s.platformTaskTodos[tenantID] {
		if v.AccountID == accountID {
			out = append(out, copyPlatformTaskTodoRecord(v))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Status == out[j].Status {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].Status < out[j].Status
	})
	return out, nil
}

// DeletePlatformTaskTodo 從儲存層刪除平台任務待辦。
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

// CountAgentDefinitionsByModel 從儲存層統計使用模型的 agent。
func (s *Store) CountAgentDefinitionsByModel(_ context.Context, tenantID, modelID string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, item := range s.agentDefinitions[tenantID] {
		if item.ModelID == modelID {
			count++
			continue
		}
		for _, member := range item.SubAgents {
			if member.ModelID == modelID {
				count++
				break
			}
		}
	}
	for _, item := range s.agentDefinitionVersions[tenantID] {
		for _, member := range item.SubAgents {
			if member.ModelID == modelID {
				count++
				break
			}
		}
	}
	return count, nil
}

// InsertAgentExternalTool stores one tenant-scoped external tool registration.
func (s *Store) InsertAgentExternalTool(_ context.Context, item AgentExternalTool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.agentExternalTools, item.TenantID, item.ID, item)
	return nil
}

// ListAgentExternalTools returns external tools in newest-first order.
func (s *Store) ListAgentExternalTools(_ context.Context, tenantID string) ([]AgentExternalTool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.agentExternalTools[tenantID], func(v AgentExternalTool) AgentExternalTool { return v })
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// DeleteAgentExternalTool deletes one tenant-scoped external tool registration.
func (s *Store) DeleteAgentExternalTool(_ context.Context, tenantID, id string) (AgentExternalTool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := getNested(s.agentExternalTools, tenantID, id)
	if !ok {
		return AgentExternalTool{}, false, nil
	}
	delete(s.agentExternalTools[tenantID], id)
	return item, true, nil
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
	putNested(s.agentDefinitionVersions, v.TenantID, agentDefinitionVersionKey(v.AgentID, v.Version), copyAgentDefinitionVersion(v))
	return nil
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

// InsertAgentAudit 從儲存層新增 agent audit。
func (s *Store) InsertAgentAudit(_ context.Context, v AgentAudit) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentAudits[v.TenantID] = append(s.agentAudits[v.TenantID], copyAgentAudit(v))
	return nil
}

// ListAgentAudits 從儲存層列出 agent audit。
func (s *Store) ListAgentAudits(_ context.Context, tenantID string) ([]AgentAudit, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.agentAudits[tenantID]
	out := make([]AgentAudit, 0, len(src))
	for _, item := range src {
		out = append(out, copyAgentAudit(item))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
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
func (s *Store) ListAgentSessionsByAccount(_ context.Context, tenantID, accountID, status, agentID string) ([]AgentSession, error) {
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
		out = append(out, copyAgentSession(item))
	}
	sortAgentSessions(out)
	return out, nil
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
			delete(s.agentMessageAttachments[tenantID], messageID)
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
func (s *Store) ListAgentSessionMessages(_ context.Context, tenantID, sessionID string) ([]AgentSessionMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AgentSessionMessage, 0)
	session, sessionOK := getNested(s.agentSessions, tenantID, sessionID)
	if !sessionOK {
		return out, nil
	}
	for _, item := range s.agentSessionMessages[tenantID] {
		if item.SessionID == sessionID && item.ContextVersion == session.ContextVersion {
			out = append(out, copyAgentSessionMessage(item))
		}
	}
	sortAgentSessionMessages(out)
	return out, nil
}

// ListRecentAgentSessionMessages 從儲存層列出最近 agent 會話訊息。
func (s *Store) ListRecentAgentSessionMessages(ctx context.Context, tenantID, sessionID string, limit int) ([]AgentSessionMessage, error) {
	items, err := s.ListAgentSessionMessages(ctx, tenantID, sessionID)
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

// MarkAgentSessionFileAttached moves a draft file into persisted message history.
func (s *Store) MarkAgentSessionFileAttached(_ context.Context, tenantID, sessionID, fileID string, updatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, sessionOK := getNested(s.agentSessions, tenantID, sessionID)
	file, fileOK := getNested(s.agentSessionFiles, tenantID, fileID)
	if !sessionOK || !fileOK || file.SessionID != sessionID || file.ContextVersion != session.ContextVersion {
		return errors.New("agent session file not found")
	}
	file.State = "attached"
	file.UpdatedAt = updatedAt
	putNested(s.agentSessionFiles, tenantID, fileID, copyAgentSessionFile(file))
	return nil
}

// InsertAgentMessageAttachment records the exact message/file relation.
func (s *Store) InsertAgentMessageAttachment(_ context.Context, tenantID, messageID, fileID string, ordinal int, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, ok := getNested(s.agentSessionFiles, tenantID, fileID)
	if !ok {
		return errors.New("agent session file not found")
	}
	if s.agentMessageAttachments[tenantID] == nil {
		s.agentMessageAttachments[tenantID] = map[string][]domain.AgentMessageAttachment{}
	}
	items := s.agentMessageAttachments[tenantID][messageID]
	for _, item := range items {
		if item.File.ID == fileID {
			return nil
		}
	}
	s.agentMessageAttachments[tenantID][messageID] = append(items, domain.AgentMessageAttachment{
		MessageID: messageID, Ordinal: ordinal, File: copyAgentSessionFile(file),
	})
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
	for messageID, items := range s.agentMessageAttachments[tenantID] {
		message, messageOK := getNested(s.agentSessionMessages, tenantID, messageID)
		if !messageOK || message.SessionID != sessionID || message.ContextVersion != session.ContextVersion {
			continue
		}
		for _, item := range items {
			copyItem := item
			copyItem.File = copyAgentSessionFile(item.File)
			out = append(out, copyItem)
		}
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

// DeleteAgentFileAsset removes in-memory metadata, chunks, and stale attachment entries.
func (s *Store) DeleteAgentFileAsset(_ context.Context, tenantID, fileID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.agentSessionFiles[tenantID], fileID)
	delete(s.agentFileChunks[tenantID], fileID)
	for messageID, items := range s.agentMessageAttachments[tenantID] {
		filtered := items[:0]
		for _, item := range items {
			if item.File.ID != fileID {
				filtered = append(filtered, item)
			}
		}
		s.agentMessageAttachments[tenantID][messageID] = filtered
	}
	return nil
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
	for _, item := range s.agentMemories[tenantID] {
		if item.AccountID != accountID || agentMemoryExpired(item, now) {
			continue
		}
		if agentID != "" && item.AgentID != "" && item.AgentID != agentID {
			continue
		}
		if sessionID != "" && item.SessionID != sessionID {
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
	delete(s.agentMemories[tenantID], id)
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
		left := agentSessionSortTime(items[i])
		right := agentSessionSortTime(items[j])
		if left.Equal(right) {
			return items[i].ID > items[j].ID
		}
		return left.After(right)
	})
}

func agentSessionSortTime(v AgentSession) time.Time {
	if v.LastMessageAt != nil {
		return v.LastMessageAt.UTC()
	}
	return v.UpdatedAt.UTC()
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

// CountNotificationTones 從儲存層統計目前帳號可見通知的 tone 分布。
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

// memoryNotificationAfterCursor 只保留早於倒序游標的通知列。
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

// AppendAuditLog 從儲存層附加稽核 log。
func (s *Store) AppendAuditLog(_ context.Context, v AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auditLogs[v.TenantID] = append(s.auditLogs[v.TenantID], copyAuditLog(v))
	return nil
}

// ListAuditLogs 從儲存層列出稽核 logs。
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

// ListAuditLogPage 從儲存層列出稽核 log 分頁。
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

// ListAuditLogPageFiltered 從儲存層篩選並列出稽核 log 分頁。
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

func auditLogWorkspaceType(log AuditLog) string {
	text := strings.ToLower(strings.Join([]string{log.Resource, log.Action}, " "))
	switch {
	case strings.Contains(text, "employee"):
		return "員工管理"
	case strings.Contains(text, "org") || strings.Contains(text, "position"):
		return "組織架構"
	case strings.Contains(text, "attendance") || strings.Contains(text, "leave"):
		return "假勤制度"
	case strings.Contains(text, "form") || strings.Contains(text, "workflow"):
		return "表單設計"
	case strings.Contains(text, "iam") || strings.Contains(text, "authz") || strings.Contains(text, "admin"):
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
	s.identityOutbox[v.TenantID] = append(s.identityOutbox[v.TenantID], v)
	return nil
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
	s.outboxEvents[v.TenantID] = append(s.outboxEvents[v.TenantID], copyOutboxEvent(v))
	return nil
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

// ClaimOutboxEvents atomically claims dispatchable outbox events for a worker.
func (s *Store) ClaimOutboxEvents(_ context.Context, tenantID string, limit, maxRetries int) ([]OutboxEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		return nil, nil
	}
	if maxRetries <= 0 {
		maxRetries = 1
	}
	events := s.outboxEvents[tenantID]
	type ranked struct {
		idx int
		ev  OutboxEvent
	}
	candidates := make([]ranked, 0, len(events))
	for i, event := range events {
		if event.RetryCount >= maxRetries {
			continue
		}
		switch event.Status {
		case "pending", "failed":
			candidates = append(candidates, ranked{idx: i, ev: event})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
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
		events[item.idx].Status = "processing"
		events[item.idx].LastError = ""
		events[item.idx].ProcessedAt = nil
		out = append(out, copyOutboxEvent(events[item.idx]))
	}
	s.outboxEvents[tenantID] = events
	return out, nil
}

// UpdateOutboxEvent 從儲存層更新 outbox 事件處理狀態。
func (s *Store) UpdateOutboxEvent(_ context.Context, v OutboxEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := s.outboxEvents[v.TenantID]
	for i := range events {
		if events[i].ID == v.ID {
			events[i] = copyOutboxEvent(v)
			s.outboxEvents[v.TenantID] = events
			return nil
		}
	}
	return nil
}

// AddAccountGroup 從儲存層處理 add 帳號群組。
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

// RemoveAccountGroup 從儲存層處理 remove 帳號群組。
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

// groupMembershipKey 取得使用者群組成員關係 key。
func groupMembershipKey(userGroupID, accountID string) string {
	return userGroupID + "\x00" + accountID
}

// membershipActiveAt 判斷群組成員關係在指定時間是否有效。
func membershipActiveAt(v GroupMembership, at time.Time) bool {
	if !v.ValidFrom.IsZero() && v.ValidFrom.After(at) {
		return false
	}
	return v.ValidUntil == nil || !v.ValidUntil.Before(at)
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
