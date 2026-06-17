package memory

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"nexus-pro-be/internal/utils"
)

type Store struct {
	mu sync.RWMutex

	tenants map[string]Tenant

	accounts            map[string]map[string]Account
	userGroups          map[string]map[string]UserGroup
	permissionSets      map[string]map[string]PermissionSet
	assignments         map[string]map[string]PermissionSetAssignment
	dataScopes          map[string]map[string]DataScope
	fieldPolicies       map[string]map[string]FieldPolicy
	assumableRoles      map[string]map[string]AssumableRole
	roleSessions        map[string]map[string]AssumableRoleSession
	orgUnits            map[string]map[string]OrgUnit
	employees           map[string]map[string]Employee
	employeeNoSequences map[string]map[string]int
	employeeImports     map[string]map[string]EmployeeImportSession
	leaveBalances       map[string]map[string]LeaveBalance
	leaveRequests       map[string]map[string]LeaveRequest
	formTemplates       map[string]map[string]FormTemplate
	formInstances       map[string]map[string]FormInstance
	knowledgeArticles   map[string]map[string]KnowledgeArticle
	agentRuns           map[string]map[string]AgentRun
	auditLogs           map[string][]AuditLog
	permissionVersions  map[string]int64
	authzOutbox         map[string][]AuthzOutboxEvent
	relationshipTuples  map[string]map[string]AuthzRelationshipTuple
}

func NewStore() *Store {
	return &Store{
		tenants:             map[string]Tenant{},
		accounts:            map[string]map[string]Account{},
		userGroups:          map[string]map[string]UserGroup{},
		permissionSets:      map[string]map[string]PermissionSet{},
		assignments:         map[string]map[string]PermissionSetAssignment{},
		dataScopes:          map[string]map[string]DataScope{},
		fieldPolicies:       map[string]map[string]FieldPolicy{},
		assumableRoles:      map[string]map[string]AssumableRole{},
		roleSessions:        map[string]map[string]AssumableRoleSession{},
		orgUnits:            map[string]map[string]OrgUnit{},
		employees:           map[string]map[string]Employee{},
		employeeNoSequences: map[string]map[string]int{},
		employeeImports:     map[string]map[string]EmployeeImportSession{},
		leaveBalances:       map[string]map[string]LeaveBalance{},
		leaveRequests:       map[string]map[string]LeaveRequest{},
		formTemplates:       map[string]map[string]FormTemplate{},
		formInstances:       map[string]map[string]FormInstance{},
		knowledgeArticles:   map[string]map[string]KnowledgeArticle{},
		agentRuns:           map[string]map[string]AgentRun{},
		auditLogs:           map[string][]AuditLog{},
		permissionVersions:  map[string]int64{},
		authzOutbox:         map[string][]AuthzOutboxEvent{},
		relationshipTuples:  map[string]map[string]AuthzRelationshipTuple{},
	}
}

func (s *Store) UpsertTenant(_ context.Context, v Tenant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tenants[v.ID] = copyTenant(v)
	return nil
}

func (s *Store) GetTenant(_ context.Context, id string) (Tenant, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.tenants[id]
	if !ok {
		return Tenant{}, false, nil
	}
	return copyTenant(v), true, nil
}

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

func (s *Store) UpsertAccount(_ context.Context, v Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.accounts, v.TenantID, v.ID, copyAccount(v))
	return nil
}

func (s *Store) GetAccount(_ context.Context, tenantID, id string) (Account, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.accounts, tenantID, id)
	if !ok {
		return Account{}, false, nil
	}
	return copyAccount(v), true, nil
}

func (s *Store) ListAccounts(_ context.Context, tenantID string) ([]Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.accounts[tenantID], copyAccount)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) UpsertUserGroup(_ context.Context, v UserGroup) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.userGroups, v.TenantID, v.ID, copyUserGroup(v))
	return nil
}

func (s *Store) GetUserGroup(_ context.Context, tenantID, id string) (UserGroup, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.userGroups, tenantID, id)
	if !ok {
		return UserGroup{}, false, nil
	}
	return copyUserGroup(v), true, nil
}

func (s *Store) ListUserGroups(_ context.Context, tenantID string) ([]UserGroup, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.userGroups[tenantID], copyUserGroup)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) UpsertPermissionSet(_ context.Context, v PermissionSet) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.permissionSets, v.TenantID, v.ID, copyPermissionSet(v))
	return nil
}

func (s *Store) GetPermissionSet(_ context.Context, tenantID, id string) (PermissionSet, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.permissionSets, tenantID, id)
	if !ok {
		return PermissionSet{}, false, nil
	}
	return copyPermissionSet(v), true, nil
}

func (s *Store) ListPermissionSets(_ context.Context, tenantID string) ([]PermissionSet, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.permissionSets[tenantID], copyPermissionSet)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) UpsertPermissionSetAssignment(_ context.Context, v PermissionSetAssignment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.assignments, v.TenantID, v.ID, copyPermissionSetAssignment(v))
	return nil
}

func (s *Store) ListPermissionSetAssignments(_ context.Context, tenantID string) ([]PermissionSetAssignment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.assignments[tenantID], copyPermissionSetAssignment)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

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

func (s *Store) UpsertDataScope(_ context.Context, v DataScope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.dataScopes, v.TenantID, v.ID, copyDataScope(v))
	return nil
}

func (s *Store) GetDataScope(_ context.Context, tenantID, id string) (DataScope, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.dataScopes, tenantID, id)
	if !ok {
		return DataScope{}, false, nil
	}
	return copyDataScope(v), true, nil
}

func (s *Store) GetDataScopeByCode(_ context.Context, tenantID, code string) (DataScope, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.dataScopes[tenantID] {
		if v.Code == code {
			return copyDataScope(v), true, nil
		}
	}
	return DataScope{}, false, nil
}

func (s *Store) ListDataScopes(_ context.Context, tenantID string) ([]DataScope, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.dataScopes[tenantID], copyDataScope)
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

func (s *Store) UpsertFieldPolicy(_ context.Context, v FieldPolicy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.fieldPolicies, v.TenantID, v.ID, copyFieldPolicy(v))
	return nil
}

func (s *Store) ListFieldPolicies(_ context.Context, tenantID, applicationCode, resourceType string) ([]FieldPolicy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]FieldPolicy, 0)
	for _, v := range s.fieldPolicies[tenantID] {
		if v.ApplicationCode == applicationCode && v.ResourceType == resourceType {
			out = append(out, copyFieldPolicy(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FieldName < out[j].FieldName })
	return out, nil
}

func (s *Store) UpsertAssumableRole(_ context.Context, v AssumableRole) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.assumableRoles, v.TenantID, v.ID, copyAssumableRole(v))
	return nil
}

func (s *Store) GetAssumableRole(_ context.Context, tenantID, id string) (AssumableRole, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.assumableRoles, tenantID, id)
	if !ok {
		return AssumableRole{}, false, nil
	}
	return copyAssumableRole(v), true, nil
}

func (s *Store) ListAssumableRoles(_ context.Context, tenantID string) ([]AssumableRole, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.assumableRoles[tenantID], copyAssumableRole)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) UpsertAssumableRoleSession(_ context.Context, v AssumableRoleSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.roleSessions, v.TenantID, v.ID, copyAssumableRoleSession(v))
	return nil
}

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

func (s *Store) UpsertOrgUnit(_ context.Context, v OrgUnit) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.orgUnits, v.TenantID, v.ID, copyOrgUnit(v))
	return nil
}

func (s *Store) GetOrgUnit(_ context.Context, tenantID, id string) (OrgUnit, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.orgUnits, tenantID, id)
	if !ok {
		return OrgUnit{}, false, nil
	}
	return copyOrgUnit(v), true, nil
}

func (s *Store) ListOrgUnits(_ context.Context, tenantID string) ([]OrgUnit, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.orgUnits[tenantID], copyOrgUnit)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) UpsertEmployee(_ context.Context, v Employee) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.employees, v.TenantID, v.ID, copyEmployee(v))
	return nil
}

func (s *Store) GetEmployee(_ context.Context, tenantID, id string) (Employee, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.employees, tenantID, id)
	if !ok {
		return Employee{}, false, nil
	}
	return copyEmployee(v), true, nil
}

func (s *Store) GetEmployeeByEmployeeNo(_ context.Context, tenantID, employeeNo string) (Employee, bool, error) {
	return s.getEmployeeBy(tenantID, func(v Employee) bool {
		return v.EmployeeNo == strings.TrimSpace(employeeNo)
	})
}

func (s *Store) GetEmployeeByCompanyEmail(_ context.Context, tenantID, companyEmail string) (Employee, bool, error) {
	email := strings.ToLower(strings.TrimSpace(companyEmail))
	return s.getEmployeeBy(tenantID, func(v Employee) bool {
		return strings.ToLower(strings.TrimSpace(v.CompanyEmail)) == email
	})
}

func (s *Store) GetEmployeeByAccountID(_ context.Context, tenantID, accountID string) (Employee, bool, error) {
	accountID = strings.TrimSpace(accountID)
	return s.getEmployeeBy(tenantID, func(v Employee) bool {
		return v.AccountID == accountID
	})
}

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

func (s *Store) ListEmployees(_ context.Context, tenantID string) ([]Employee, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.employees[tenantID], copyEmployee)
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) ListEmployeesByQuery(ctx context.Context, tenantID string, query EmployeeQuery) ([]Employee, error) {
	items, err := s.ListEmployees(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	items = filterMemoryEmployeesByQuery(items, query)
	sortMemoryEmployees(items, query.Sort)
	return items, nil
}

func (s *Store) ListEmployeePageByQuery(ctx context.Context, tenantID string, query EmployeeQuery) ([]Employee, int, error) {
	items, err := s.ListEmployeesByQuery(ctx, tenantID, query)
	if err != nil {
		return nil, 0, err
	}
	total := len(items)
	query = normalizeMemoryEmployeeQuery(query)
	return paginateMemory(items, query.Page, query.PageSize), total, nil
}

func (s *Store) CountEmployeesByQuery(ctx context.Context, tenantID string, query EmployeeQuery) (int, error) {
	items, err := s.ListEmployeesByQuery(ctx, tenantID, query)
	if err != nil {
		return 0, err
	}
	return len(items), nil
}

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

func filterMemoryEmployeesByQuery(items []Employee, query EmployeeQuery) []Employee {
	out := make([]Employee, 0, len(items))
	query = normalizeMemoryEmployeeQuery(query)
	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))
	for _, item := range items {
		status := utils.FirstNonEmpty(item.EmploymentStatus, item.Status)
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
			haystack := strings.ToLower(strings.Join([]string{item.Name, item.CompanyEmail, item.PersonalEmail, item.EmployeeNo, item.Phone}, " "))
			if !strings.Contains(haystack, keyword) {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

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

func normalizeMemoryEmployeeStatus(value string) string {
	switch strings.TrimSpace(value) {
	case "在職", "active":
		return "active"
	case "試用中", "probation":
		return "probation"
	case "留停", "on-leave", "leave_suspended":
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

func sortMemoryEmployees(items []Employee, sortKey string) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
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

func memoryTimeValue(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

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

func (s *Store) UpsertEmployeeImportSession(_ context.Context, v EmployeeImportSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.employeeImports, v.TenantID, v.ID, copyEmployeeImportSession(v))
	return nil
}

func (s *Store) GetEmployeeImportSession(_ context.Context, tenantID, id string) (EmployeeImportSession, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.employeeImports, tenantID, id)
	if !ok {
		return EmployeeImportSession{}, false, nil
	}
	return copyEmployeeImportSession(v), true, nil
}

func (s *Store) UpsertLeaveBalance(_ context.Context, v LeaveBalance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.leaveBalances, v.TenantID, v.ID, copyLeaveBalance(v))
	return nil
}

func (s *Store) GetLeaveBalance(_ context.Context, tenantID, id string) (LeaveBalance, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.leaveBalances, tenantID, id)
	if !ok {
		return LeaveBalance{}, false, nil
	}
	return copyLeaveBalance(v), true, nil
}

func (s *Store) ListLeaveBalances(_ context.Context, tenantID string) ([]LeaveBalance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.leaveBalances[tenantID], copyLeaveBalance)
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.Before(out[j].UpdatedAt) })
	return out, nil
}

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
		balance.UpdatedAt = updatedAt
		s.leaveBalances[tenantID][id] = copyLeaveBalance(balance)
		return copyLeaveBalance(balance), true, true, nil
	}
	return LeaveBalance{}, false, false, nil
}

func (s *Store) UpsertLeaveRequest(_ context.Context, v LeaveRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.leaveRequests, v.TenantID, v.ID, copyLeaveRequest(v))
	return nil
}

func (s *Store) GetLeaveRequest(_ context.Context, tenantID, id string) (LeaveRequest, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.leaveRequests, tenantID, id)
	if !ok {
		return LeaveRequest{}, false, nil
	}
	return copyLeaveRequest(v), true, nil
}

func (s *Store) ListLeaveRequests(_ context.Context, tenantID string) ([]LeaveRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.leaveRequests[tenantID], copyLeaveRequest)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) UpsertFormTemplate(_ context.Context, v FormTemplate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.formTemplates, v.TenantID, v.ID, copyFormTemplate(v))
	return nil
}

func (s *Store) GetFormTemplate(_ context.Context, tenantID, id string) (FormTemplate, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.formTemplates, tenantID, id)
	if !ok {
		return FormTemplate{}, false, nil
	}
	return copyFormTemplate(v), true, nil
}

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

func (s *Store) ListFormTemplates(_ context.Context, tenantID string) ([]FormTemplate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.formTemplates[tenantID], copyFormTemplate)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) UpsertFormInstance(_ context.Context, v FormInstance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.formInstances, v.TenantID, v.ID, copyFormInstance(v))
	return nil
}

func (s *Store) GetFormInstance(_ context.Context, tenantID, id string) (FormInstance, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.formInstances, tenantID, id)
	if !ok {
		return FormInstance{}, false, nil
	}
	return copyFormInstance(v), true, nil
}

func (s *Store) ListFormInstances(_ context.Context, tenantID string) ([]FormInstance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.formInstances[tenantID], copyFormInstance)
	sort.Slice(out, func(i, j int) bool { return out[i].SubmittedAt.Before(out[j].SubmittedAt) })
	return out, nil
}

func (s *Store) UpsertKnowledgeArticle(_ context.Context, v KnowledgeArticle) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.knowledgeArticles, v.TenantID, v.ID, copyKnowledgeArticle(v))
	return nil
}

func (s *Store) ListKnowledgeArticles(_ context.Context, tenantID string) ([]KnowledgeArticle, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.knowledgeArticles[tenantID], copyKnowledgeArticle)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) UpsertAgentRun(_ context.Context, v AgentRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.agentRuns, v.TenantID, v.ID, copyAgentRun(v))
	return nil
}

func (s *Store) GetAgentRun(_ context.Context, tenantID, id string) (AgentRun, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.agentRuns, tenantID, id)
	if !ok {
		return AgentRun{}, false, nil
	}
	return copyAgentRun(v), true, nil
}

func (s *Store) ListAgentRuns(_ context.Context, tenantID string) ([]AgentRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.agentRuns[tenantID], copyAgentRun)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

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

func (s *Store) AppendAuditLog(_ context.Context, v AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auditLogs[v.TenantID] = append(s.auditLogs[v.TenantID], copyAuditLog(v))
	return nil
}

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

func (s *Store) GetPermissionVersion(_ context.Context, tenantID string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.permissionVersions[tenantID], nil
}

func (s *Store) IncrementPermissionVersion(_ context.Context, tenantID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.permissionVersions[tenantID]++
	return s.permissionVersions[tenantID], nil
}

func (s *Store) UpsertAuthzRelationshipTuple(_ context.Context, v AuthzRelationshipTuple) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.relationshipTuples[v.TenantID] == nil {
		s.relationshipTuples[v.TenantID] = map[string]AuthzRelationshipTuple{}
	}
	s.relationshipTuples[v.TenantID][relationshipTupleKey(v)] = v
	return nil
}

func (s *Store) DeleteAuthzRelationshipTuple(_ context.Context, v AuthzRelationshipTuple) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.relationshipTuples[v.TenantID], relationshipTupleKey(v))
	return nil
}

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

func (s *Store) AppendAuthzOutboxEvent(_ context.Context, v AuthzOutboxEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authzOutbox[v.TenantID] = append(s.authzOutbox[v.TenantID], copyAuthzOutboxEvent(v))
	return nil
}

func (s *Store) ListAuthzOutboxEvents(_ context.Context, tenantID string) ([]AuthzOutboxEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.authzOutbox[tenantID]
	out := make([]AuthzOutboxEvent, 0, len(src))
	for _, v := range src {
		out = append(out, copyAuthzOutboxEvent(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) UpdateAuthzOutboxEvent(_ context.Context, v AuthzOutboxEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := s.authzOutbox[v.TenantID]
	for i := range events {
		if events[i].ID == v.ID {
			events[i] = copyAuthzOutboxEvent(v)
			s.authzOutbox[v.TenantID] = events
			return nil
		}
	}
	return nil
}

func (s *Store) RemoveAccountGroup(_ context.Context, tenantID, accountID, groupID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	accountBucket := s.accounts[tenantID]
	account, ok := accountBucket[accountID]
	if !ok {
		return nil
	}
	account.UserGroupIDs = utils.RemoveString(account.UserGroupIDs, groupID)
	accountBucket[accountID] = account
	return nil
}

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

func putNested[T any](bucket map[string]map[string]T, tenantID, id string, value T) {
	sub, ok := bucket[tenantID]
	if !ok {
		sub = map[string]T{}
		bucket[tenantID] = sub
	}
	sub[id] = value
}

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

func nowUTC() time.Time {
	return time.Now().UTC()
}
