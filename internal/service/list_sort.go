package service

type comparators[T any] map[string]func(a, b T) bool

func sortBy[T any](items []T, sortKey string, cmps comparators[T], defaultKey string) []T {
	out := make([]T, len(items))
	copy(out, items)
	less := cmps[sortKey]
	if less == nil {
		less = cmps[defaultKey]
	}
	if less != nil {
		sortSlice(out, less)
	}
	return out
}

var userGroupComparators = comparators[UserGroup]{
	"created_at_asc":  func(a, b UserGroup) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b UserGroup) bool { return a.CreatedAt.After(b.CreatedAt) },
	"name_asc":        func(a, b UserGroup) bool { return a.Name < b.Name },
}

func sortUserGroups(items []UserGroup, sort string) []UserGroup {
	return sortBy(items, sort, userGroupComparators, "created_at_desc")
}

var permissionSetComparators = comparators[PermissionSet]{
	"created_at_asc":  func(a, b PermissionSet) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b PermissionSet) bool { return a.CreatedAt.After(b.CreatedAt) },
	"name_asc":        func(a, b PermissionSet) bool { return a.Name < b.Name },
}

func sortPermissionSets(items []PermissionSet, sort string) []PermissionSet {
	return sortBy(items, sort, permissionSetComparators, "created_at_desc")
}

var permissionSetAssignmentComparators = comparators[PermissionSetAssignment]{
	"created_at_asc":  func(a, b PermissionSetAssignment) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b PermissionSetAssignment) bool { return a.CreatedAt.After(b.CreatedAt) },
}

func sortPermissionSetAssignments(items []PermissionSetAssignment, sort string) []PermissionSetAssignment {
	return sortBy(items, sort, permissionSetAssignmentComparators, "created_at_desc")
}

var dataScopeComparators = comparators[DataScope]{
	"created_at_asc":  func(a, b DataScope) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b DataScope) bool { return a.CreatedAt.After(b.CreatedAt) },
	"name_asc":        func(a, b DataScope) bool { return a.Name < b.Name },
}

func sortDataScopes(items []DataScope, sort string) []DataScope {
	return sortBy(items, sort, dataScopeComparators, "created_at_desc")
}

var fieldPolicyComparators = comparators[FieldPolicy]{
	"created_at_asc":  func(a, b FieldPolicy) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b FieldPolicy) bool { return a.CreatedAt.After(b.CreatedAt) },
	"field_asc":       func(a, b FieldPolicy) bool { return a.FieldName < b.FieldName },
}

func sortFieldPolicies(items []FieldPolicy, sort string) []FieldPolicy {
	return sortBy(items, sort, fieldPolicyComparators, "created_at_desc")
}

var assumableRoleComparators = comparators[AssumableRole]{
	"created_at_asc":  func(a, b AssumableRole) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b AssumableRole) bool { return a.CreatedAt.After(b.CreatedAt) },
	"name_asc":        func(a, b AssumableRole) bool { return a.Name < b.Name },
}

func sortAssumableRoles(items []AssumableRole, sort string) []AssumableRole {
	return sortBy(items, sort, assumableRoleComparators, "created_at_desc")
}

var formTemplateComparators = comparators[FormTemplate]{
	"created_at_asc":  func(a, b FormTemplate) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b FormTemplate) bool { return a.CreatedAt.After(b.CreatedAt) },
	"key_asc":         func(a, b FormTemplate) bool { return a.Key < b.Key },
}

func sortFormTemplates(items []FormTemplate, sort string) []FormTemplate {
	return sortBy(items, sort, formTemplateComparators, "created_at_desc")
}

var leaveBalanceComparators = comparators[LeaveBalance]{
	"updated_at_asc":  func(a, b LeaveBalance) bool { return a.UpdatedAt.Before(b.UpdatedAt) },
	"updated_at_desc": func(a, b LeaveBalance) bool { return a.UpdatedAt.After(b.UpdatedAt) },
	"employee_id_asc": func(a, b LeaveBalance) bool { return a.EmployeeID < b.EmployeeID },
}

func sortLeaveBalances(items []LeaveBalance, sort string) []LeaveBalance {
	return sortBy(items, sort, leaveBalanceComparators, "updated_at_desc")
}

var leaveRequestComparators = comparators[LeaveRequest]{
	"created_at_asc":  func(a, b LeaveRequest) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b LeaveRequest) bool { return a.CreatedAt.After(b.CreatedAt) },
	"start_at_asc":    func(a, b LeaveRequest) bool { return a.StartAt.Before(b.StartAt) },
}

func sortLeaveRequests(items []LeaveRequest, sort string) []LeaveRequest {
	return sortBy(items, sort, leaveRequestComparators, "created_at_desc")
}

var orgUnitComparators = comparators[OrgUnit]{
	"created_at_asc":  func(a, b OrgUnit) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b OrgUnit) bool { return a.CreatedAt.After(b.CreatedAt) },
	"name_asc":        func(a, b OrgUnit) bool { return a.Name < b.Name },
}

func sortOrgUnits(items []OrgUnit, sort string) []OrgUnit {
	return sortBy(items, sort, orgUnitComparators, "created_at_desc")
}
