package utils

import "nexus-pro-be/internal/domain"

type comparators[T any] map[string]func(a, b T) bool

// sortBy 排序by。
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

// SortUserGroups 排序使用者群組。
func SortUserGroups(items []domain.UserGroup, sort string) []domain.UserGroup {
	return sortBy(items, sort, userGroupComparators, "created_at_desc")
}

var permissionSetComparators = comparators[domain.PermissionSet]{
	"created_at_asc":  func(a, b domain.PermissionSet) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b domain.PermissionSet) bool { return a.CreatedAt.After(b.CreatedAt) },
	"name_asc":        func(a, b domain.PermissionSet) bool { return a.Name < b.Name },
}

// SortPermissionSets 排序權限集合。
func SortPermissionSets(items []domain.PermissionSet, sort string) []domain.PermissionSet {
	return sortBy(items, sort, permissionSetComparators, "created_at_desc")
}

var permissionSetAssignmentComparators = comparators[domain.PermissionSetAssignment]{
	"created_at_asc":  func(a, b domain.PermissionSetAssignment) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b domain.PermissionSetAssignment) bool { return a.CreatedAt.After(b.CreatedAt) },
}

// SortPermissionSetAssignments 排序權限集合指派。
func SortPermissionSetAssignments(items []domain.PermissionSetAssignment, sort string) []domain.PermissionSetAssignment {
	return sortBy(items, sort, permissionSetAssignmentComparators, "created_at_desc")
}

var iamAccountProjectionComparators = comparators[domain.IamAccountProjection]{
	"created_at_asc":  func(a, b domain.IamAccountProjection) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b domain.IamAccountProjection) bool { return a.CreatedAt.After(b.CreatedAt) },
	"name_asc":        func(a, b domain.IamAccountProjection) bool { return a.DisplayName < b.DisplayName },
}

// SortIamAccountProjections 排序 IAM 帳號投影。
func SortIamAccountProjections(items []domain.IamAccountProjection, sort string) []domain.IamAccountProjection {
	return sortBy(items, sort, iamAccountProjectionComparators, "created_at_desc")
}

var dataScopeComparators = comparators[domain.DataScope]{
	"created_at_asc":  func(a, b domain.DataScope) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b domain.DataScope) bool { return a.CreatedAt.After(b.CreatedAt) },
	"name_asc":        func(a, b domain.DataScope) bool { return a.Name < b.Name },
}

// SortDataScopes 排序資料範圍。
func SortDataScopes(items []domain.DataScope, sort string) []domain.DataScope {
	return sortBy(items, sort, dataScopeComparators, "created_at_desc")
}

var fieldPolicyComparators = comparators[domain.FieldPolicy]{
	"created_at_asc":  func(a, b domain.FieldPolicy) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b domain.FieldPolicy) bool { return a.CreatedAt.After(b.CreatedAt) },
	"field_asc":       func(a, b domain.FieldPolicy) bool { return a.FieldName < b.FieldName },
}

// SortFieldPolicies 排序欄位政策。
func SortFieldPolicies(items []domain.FieldPolicy, sort string) []domain.FieldPolicy {
	return sortBy(items, sort, fieldPolicyComparators, "created_at_desc")
}

var assumableRoleComparators = comparators[domain.AssumableRole]{
	"created_at_asc":  func(a, b domain.AssumableRole) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b domain.AssumableRole) bool { return a.CreatedAt.After(b.CreatedAt) },
	"name_asc":        func(a, b domain.AssumableRole) bool { return a.Name < b.Name },
}

// SortAssumableRoles 排序assumable 角色。
func SortAssumableRoles(items []domain.AssumableRole, sort string) []domain.AssumableRole {
	return sortBy(items, sort, assumableRoleComparators, "created_at_desc")
}

var formTemplateComparators = comparators[domain.FormTemplate]{
	"created_at_asc":  func(a, b domain.FormTemplate) bool { return a.CreatedAt.Before(b.CreatedAt) },
	"created_at_desc": func(a, b domain.FormTemplate) bool { return a.CreatedAt.After(b.CreatedAt) },
	"key_asc":         func(a, b domain.FormTemplate) bool { return a.Key < b.Key },
}

// SortFormTemplates 排序表單範本。
func SortFormTemplates(items []domain.FormTemplate, sort string) []domain.FormTemplate {
	return sortBy(items, sort, formTemplateComparators, "created_at_desc")
}

var leaveBalanceComparators = comparators[domain.LeaveBalance]{
	"updated_at_asc":  func(a, b domain.LeaveBalance) bool { return a.UpdatedAt.Before(b.UpdatedAt) },
	"updated_at_desc": func(a, b domain.LeaveBalance) bool { return a.UpdatedAt.After(b.UpdatedAt) },
	"employee_id_asc": func(a, b domain.LeaveBalance) bool { return a.EmployeeID < b.EmployeeID },
}

// SortLeaveBalances 排序請假 balances。
func SortLeaveBalances(items []domain.LeaveBalance, sort string) []domain.LeaveBalance {
	return sortBy(items, sort, leaveBalanceComparators, "updated_at_desc")
}

var orgUnitComparators = comparators[domain.OrgUnit]{
	"created_at_asc":  orgUnitLess(func(a, b domain.OrgUnit) bool { return a.CreatedAt.Before(b.CreatedAt) }),
	"created_at_desc": orgUnitLess(func(a, b domain.OrgUnit) bool { return a.CreatedAt.After(b.CreatedAt) }),
	"name_asc":        orgUnitLess(func(a, b domain.OrgUnit) bool { return a.Name < b.Name }),
	"code_asc":        orgUnitLess(orgUnitCodeThenName),
}

func orgUnitCodeThenName(a, b domain.OrgUnit) bool {
	if a.Code != b.Code {
		return a.Code < b.Code
	}
	return a.Name < b.Name
}

func orgUnitLess(secondary func(a, b domain.OrgUnit) bool) func(a, b domain.OrgUnit) bool {
	return func(a, b domain.OrgUnit) bool {
		if a.Closed != b.Closed {
			return !a.Closed
		}
		return secondary(a, b)
	}
}

// SortOrgUnits 排序組織單位。
func SortOrgUnits(items []domain.OrgUnit, sort string) []domain.OrgUnit {
	return sortBy(items, sort, orgUnitComparators, "code_asc")
}
