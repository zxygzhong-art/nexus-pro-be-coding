package utils

import "nexus-pro-be/internal/domain"

type comparators[T any] map[string]func(a, b T) bool

func sortBy[T any](items []T, sortKey string, cmps comparators[T], defaultKey string) []T {
	out := make([]T, len(items))
	copy(out, items)
	less := cmps[sortKey]
	if less == nil {
		less = cmps[defaultKey]
	}
	if less != nil {
		SortSlice(out, less)
	}
	return out
}

var userGroupComparators = comparators[domain.UserGroup]{
	"created_at_asc":  func(a, b domain.UserGroup) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b domain.UserGroup) bool { return a.CreatedAt.After(b.CreatedAt) },
	"name_asc":        func(a, b domain.UserGroup) bool { return a.Name < b.Name },
}

// SortUserGroups applies the user-group list sort contract.
func SortUserGroups(items []domain.UserGroup, sort string) []domain.UserGroup {
	return sortBy(items, sort, userGroupComparators, "created_at_desc")
}

var permissionSetComparators = comparators[domain.PermissionSet]{
	"created_at_asc":  func(a, b domain.PermissionSet) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b domain.PermissionSet) bool { return a.CreatedAt.After(b.CreatedAt) },
	"name_asc":        func(a, b domain.PermissionSet) bool { return a.Name < b.Name },
}

// SortPermissionSets applies the permission-set list sort contract.
func SortPermissionSets(items []domain.PermissionSet, sort string) []domain.PermissionSet {
	return sortBy(items, sort, permissionSetComparators, "created_at_desc")
}

var permissionSetAssignmentComparators = comparators[domain.PermissionSetAssignment]{
	"created_at_asc":  func(a, b domain.PermissionSetAssignment) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b domain.PermissionSetAssignment) bool { return a.CreatedAt.After(b.CreatedAt) },
}

// SortPermissionSetAssignments applies the assignment list sort contract.
func SortPermissionSetAssignments(items []domain.PermissionSetAssignment, sort string) []domain.PermissionSetAssignment {
	return sortBy(items, sort, permissionSetAssignmentComparators, "created_at_desc")
}

var dataScopeComparators = comparators[domain.DataScope]{
	"created_at_asc":  func(a, b domain.DataScope) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b domain.DataScope) bool { return a.CreatedAt.After(b.CreatedAt) },
	"name_asc":        func(a, b domain.DataScope) bool { return a.Name < b.Name },
}

// SortDataScopes applies the data-scope list sort contract.
func SortDataScopes(items []domain.DataScope, sort string) []domain.DataScope {
	return sortBy(items, sort, dataScopeComparators, "created_at_desc")
}

var fieldPolicyComparators = comparators[domain.FieldPolicy]{
	"created_at_asc":  func(a, b domain.FieldPolicy) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b domain.FieldPolicy) bool { return a.CreatedAt.After(b.CreatedAt) },
	"field_asc":       func(a, b domain.FieldPolicy) bool { return a.FieldName < b.FieldName },
}

// SortFieldPolicies applies the field-policy list sort contract.
func SortFieldPolicies(items []domain.FieldPolicy, sort string) []domain.FieldPolicy {
	return sortBy(items, sort, fieldPolicyComparators, "created_at_desc")
}

var assumableRoleComparators = comparators[domain.AssumableRole]{
	"created_at_asc":  func(a, b domain.AssumableRole) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b domain.AssumableRole) bool { return a.CreatedAt.After(b.CreatedAt) },
	"name_asc":        func(a, b domain.AssumableRole) bool { return a.Name < b.Name },
}

// SortAssumableRoles applies the assumable-role list sort contract.
func SortAssumableRoles(items []domain.AssumableRole, sort string) []domain.AssumableRole {
	return sortBy(items, sort, assumableRoleComparators, "created_at_desc")
}

var formTemplateComparators = comparators[domain.FormTemplate]{
	"created_at_asc":  func(a, b domain.FormTemplate) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b domain.FormTemplate) bool { return a.CreatedAt.After(b.CreatedAt) },
	"key_asc":         func(a, b domain.FormTemplate) bool { return a.Key < b.Key },
}

// SortFormTemplates applies the form-template list sort contract.
func SortFormTemplates(items []domain.FormTemplate, sort string) []domain.FormTemplate {
	return sortBy(items, sort, formTemplateComparators, "created_at_desc")
}

var leaveBalanceComparators = comparators[domain.LeaveBalance]{
	"updated_at_asc":  func(a, b domain.LeaveBalance) bool { return a.UpdatedAt.Before(b.UpdatedAt) },
	"updated_at_desc": func(a, b domain.LeaveBalance) bool { return a.UpdatedAt.After(b.UpdatedAt) },
	"employee_id_asc": func(a, b domain.LeaveBalance) bool { return a.EmployeeID < b.EmployeeID },
}

// SortLeaveBalances applies the leave-balance list sort contract.
func SortLeaveBalances(items []domain.LeaveBalance, sort string) []domain.LeaveBalance {
	return sortBy(items, sort, leaveBalanceComparators, "updated_at_desc")
}

var orgUnitComparators = comparators[domain.OrgUnit]{
	"created_at_asc":  func(a, b domain.OrgUnit) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b domain.OrgUnit) bool { return a.CreatedAt.After(b.CreatedAt) },
	"name_asc":        func(a, b domain.OrgUnit) bool { return a.Name < b.Name },
}

// SortOrgUnits applies the organization-unit list sort contract.
func SortOrgUnits(items []domain.OrgUnit, sort string) []domain.OrgUnit {
	return sortBy(items, sort, orgUnitComparators, "created_at_desc")
}
