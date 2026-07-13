package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func TestAssumedRoleIntersectsNormalDepartmentSubtreeWithAssignedOrgUnits(t *testing.T) {
	now := authzScopeTestNow()
	store := memory.NewStore()
	seedAuthzScopeTenant(store, now)
	seedAuthzScopeAssumePermissionSet(store, now)
	seedAuthzScopeEmployeeReadPermissionSet(store, now)
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-root", TenantID: "tenant-1", Name: "Root", Path: []string{"ou-root"}, CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-child", TenantID: "tenant-1", Name: "Child", ParentID: "ou-root", Path: []string{"ou-root", "ou-child"}, CreatedAt: now.Add(time.Minute)})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-other", TenantID: "tenant-1", Name: "Other", Path: []string{"ou-other"}, CreatedAt: now.Add(2 * time.Minute)})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-manager", TenantID: "tenant-1", Name: "Manager", OrgUnitID: "ou-root", Status: "active", CreatedAt: now})
	_ = store.UpsertDataScope(context.Background(), domain.DataScope{ID: "ds-subtree", TenantID: "tenant-1", Code: "subtree", Name: "Subtree", ScopeType: "department_subtree", CreatedAt: now})
	_ = store.UpsertDataScope(context.Background(), domain.DataScope{
		ID:        "ds-assigned-child",
		TenantID:  "tenant-1",
		Code:      "assigned_child",
		Name:      "Assigned Child",
		ScopeType: "assigned_org_units",
		Params:    map[string]any{"org_unit_ids": []string{"ou-child"}},
		CreatedAt: now,
	})
	_ = store.UpsertUserGroup(context.Background(), domain.UserGroup{ID: "grp-hr", TenantID: "tenant-1", Name: "HR", CreatedAt: now})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-group-subtree",
		TenantID:        "tenant-1",
		PrincipalType:   "user_group",
		PrincipalID:     "grp-hr",
		PermissionSetID: "ps-employee-read",
		Effect:          "allow",
		DataScopeID:     "ds-subtree",
		CreatedAt:       now,
	})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-role-assigned",
		TenantID:        "tenant-1",
		PrincipalType:   "assumable_role",
		PrincipalID:     "role-hr",
		PermissionSetID: "ps-employee-read",
		Effect:          "allow",
		DataScopeID:     "ds-assigned-child",
		CreatedAt:       now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		EmployeeID:             "emp-manager",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-assume"},
		UserGroupIDs:           []string{"grp-hr"},
		CreatedAt:              now,
	})
	seedAuthzScopeAssumableRole(store, now, map[string]any{"allow": []string{"hr.employee.read"}})

	svc := service.New(store)
	session := assumeScopeTestRole(t, svc)
	result, err := svc.Authz().Check(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", AssumedRoleSessionID: session.SessionID},
		domain.CheckRequest{Resource: "hr.employee", Action: "read"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Allowed {
		t.Fatalf("expected normal and assumed scopes to intersect, got %+v", result)
	}
	if result.Scope != domain.ScopeCustomCondition {
		t.Fatalf("expected mixed scope intersection to use custom_condition, got %+v", result)
	}
	orgIDs := authzScopeStringsFromAny(result.Conditions["org_unit_ids"])
	if len(orgIDs) != 1 || orgIDs[0] != "ou-child" {
		t.Fatalf("expected subtree AND assigned org units to keep only child org, got %+v", result.Conditions)
	}
}

func TestAssumedRolePermissionBoundaryDenyOverridesDirectExport(t *testing.T) {
	now := authzScopeTestNow()
	store := memory.NewStore()
	seedAuthzScopeTenant(store, now)
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-base",
		TenantID: "tenant-1",
		Name:     "Base",
		Permissions: []domain.Permission{
			{Resource: "iam.assumable_role", Action: "assume", Target: "role-hr", Scope: "all"},
			{Resource: "hr.employee", Action: "export", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-role-export",
		TenantID: "tenant-1",
		Name:     "Role Export",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "export", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-base"}, CreatedAt: now})
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID:                     "role-hr",
		TenantID:               "tenant-1",
		Name:                   "HR",
		Trusted:                true,
		TrustPolicy:            map[string]any{"accounts": []string{"acct-1"}},
		PermissionSetIDs:       []string{"ps-role-export"},
		PermissionBoundary:     map[string]any{"allow": []string{"hr.employee.*"}, "deny": []string{"hr.employee.export"}},
		SessionDurationSeconds: 3600,
		CreatedAt:              now,
	})

	svc := service.New(store)
	session := assumeScopeTestRole(t, svc)
	result, err := svc.Authz().Check(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", AssumedRoleSessionID: session.SessionID},
		domain.CheckRequest{Resource: "hr.employee", Action: "export"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed || result.Reason != "explicit deny" {
		t.Fatalf("expected permission boundary deny to reject export, got %+v", result)
	}
}

func TestNormalUserGroupScopesStillUnionWithoutAssumedSession(t *testing.T) {
	now := authzScopeTestNow()
	store := memory.NewStore()
	seedAuthzScopeTenant(store, now)
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-root", TenantID: "tenant-1", Name: "Root", Path: []string{"ou-root"}, CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", Name: "Employee", OrgUnitID: "ou-root", Status: "active", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-tenant-read",
		TenantID: "tenant-1",
		Name:     "Tenant Read",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "tenant"},
		},
		CreatedAt: now,
	})
	seedAuthzScopeEmployeeReadPermissionSet(store, now)
	_ = store.UpsertDataScope(context.Background(), domain.DataScope{ID: "ds-dept", TenantID: "tenant-1", Code: "dept", Name: "Department", ScopeType: "department", CreatedAt: now})
	_ = store.UpsertUserGroup(context.Background(), domain.UserGroup{ID: "grp-dept", TenantID: "tenant-1", Name: "Dept", CreatedAt: now})
	_ = store.UpsertUserGroup(context.Background(), domain.UserGroup{ID: "grp-tenant", TenantID: "tenant-1", Name: "Tenant", PermissionSetIDs: []string{"ps-tenant-read"}, CreatedAt: now})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-group-dept",
		TenantID:        "tenant-1",
		PrincipalType:   "user_group",
		PrincipalID:     "grp-dept",
		PermissionSetID: "ps-employee-read",
		Effect:          "allow",
		DataScopeID:     "ds-dept",
		CreatedAt:       now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:           "acct-1",
		TenantID:     "tenant-1",
		EmployeeID:   "emp-1",
		Status:       "active",
		UserGroupIDs: []string{"grp-dept", "grp-tenant"},
		CreatedAt:    now,
	})

	result, err := service.New(store).Authz().Check(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		domain.CheckRequest{Resource: "hr.employee", Action: "read"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Allowed || result.Scope != domain.ScopeTenant {
		t.Fatalf("expected no-session normal scopes to union to tenant, got %+v", result)
	}
}

func TestAssumedRoleOrgUnitIntersectionEmptyDenies(t *testing.T) {
	now := authzScopeTestNow()
	store := memory.NewStore()
	seedAuthzScopeTenant(store, now)
	seedAuthzScopeAssumePermissionSet(store, now)
	seedAuthzScopeEmployeeReadPermissionSet(store, now)
	_ = store.UpsertDataScope(context.Background(), domain.DataScope{
		ID:        "ds-normal",
		TenantID:  "tenant-1",
		Code:      "normal_ou",
		Name:      "Normal OU",
		ScopeType: "assigned_org_units",
		Params:    map[string]any{"org_unit_ids": []string{"ou-a"}},
		CreatedAt: now,
	})
	_ = store.UpsertDataScope(context.Background(), domain.DataScope{
		ID:        "ds-assumed",
		TenantID:  "tenant-1",
		Code:      "assumed_ou",
		Name:      "Assumed OU",
		ScopeType: "assigned_org_units",
		Params:    map[string]any{"org_unit_ids": []string{"ou-b"}},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-account-normal",
		TenantID:        "tenant-1",
		PrincipalType:   "account",
		PrincipalID:     "acct-1",
		PermissionSetID: "ps-employee-read",
		Effect:          "allow",
		DataScopeID:     "ds-normal",
		CreatedAt:       now,
	})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-role-assumed",
		TenantID:        "tenant-1",
		PrincipalType:   "assumable_role",
		PrincipalID:     "role-hr",
		PermissionSetID: "ps-employee-read",
		Effect:          "allow",
		DataScopeID:     "ds-assumed",
		CreatedAt:       now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-assume"}, CreatedAt: now})
	seedAuthzScopeAssumableRole(store, now, map[string]any{"allow": []string{"hr.employee.read"}})

	svc := service.New(store)
	session := assumeScopeTestRole(t, svc)
	result, err := svc.Authz().Check(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", AssumedRoleSessionID: session.SessionID},
		domain.CheckRequest{Resource: "hr.employee", Action: "read"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed || !strings.Contains(result.Reason, "scope intersection empty") {
		t.Fatalf("expected empty org_unit_ids intersection to deny with readable reason, got %+v", result)
	}
}

func TestMeEffectivePermissionsApplyAssumedRoleBoundary(t *testing.T) {
	now := authzScopeTestNow()
	store := memory.NewStore()
	seedAuthzScopeTenant(store, now)
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-base",
		TenantID: "tenant-1",
		Name:     "Base",
		Permissions: []domain.Permission{
			{Resource: "iam.assumable_role", Action: "assume", Target: "role-hr", Scope: "all"},
			{Resource: "me", Action: "read", Scope: "all", MenuKey: "workbench"},
			{Resource: "hr.employee", Action: "read", Scope: "all", MenuKey: "hr.employees"},
			{Resource: "hr.employee", Action: "export", Scope: "all", MenuKey: "hr.employee_export"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-role",
		TenantID: "tenant-1",
		Name:     "Role",
		Permissions: []domain.Permission{
			{Resource: "me", Action: "read", Scope: "all", MenuKey: "workbench"},
			{Resource: "hr.employee", Action: "read", Scope: "all", MenuKey: "hr.employees"},
			{Resource: "hr.employee", Action: "export", Scope: "all", MenuKey: "hr.employee_export"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-base"}, CreatedAt: now})
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID:                     "role-hr",
		TenantID:               "tenant-1",
		Name:                   "HR",
		Trusted:                true,
		TrustPolicy:            map[string]any{"accounts": []string{"acct-1"}},
		PermissionSetIDs:       []string{"ps-role"},
		PermissionBoundary:     map[string]any{"allow": []string{"platform.me.read", "hr.employee.*"}, "deny": []string{"hr.employee.export"}},
		SessionDurationSeconds: 3600,
		CreatedAt:              now,
	})

	svc := service.New(store)
	session := assumeScopeTestRole(t, svc)
	me, err := svc.Me().Resolve(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", AssumedRoleSessionID: session.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	if authzScopeHasPermissionAction(me.EffectivePermissions, "hr.employee", "export") {
		t.Fatalf("expected boundary-denied export to be absent from effective_permissions, got %+v", me.EffectivePermissions)
	}
	if !authzScopeHasPermissionAction(me.EffectivePermissions, "hr.employee", "read") {
		t.Fatalf("expected boundary-allowed read to remain in effective_permissions, got %+v", me.EffectivePermissions)
	}
	if authzScopeHasString(me.EffectiveMenuKeys, "hr.employee_export") {
		t.Fatalf("expected boundary-denied export menu to be hidden, got %+v", me.EffectiveMenuKeys)
	}
}

func authzScopeTestNow() time.Time {
	return time.Date(2026, 7, 7, 9, 0, 0, 0, time.UTC)
}

func seedAuthzScopeTenant(store *memory.Store, now time.Time) {
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
}

func seedAuthzScopeAssumePermissionSet(store *memory.Store, now time.Time) {
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-assume",
		TenantID: "tenant-1",
		Name:     "Assume",
		Permissions: []domain.Permission{
			{Resource: "iam.assumable_role", Action: "assume", Target: "role-hr", Scope: "all"},
		},
		CreatedAt: now,
	})
}

func seedAuthzScopeEmployeeReadPermissionSet(store *memory.Store, now time.Time) {
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-employee-read",
		TenantID: "tenant-1",
		Name:     "Employee Read",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
}

func seedAuthzScopeAssumableRole(store *memory.Store, now time.Time, boundary map[string]any) {
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID:                     "role-hr",
		TenantID:               "tenant-1",
		Name:                   "HR",
		Trusted:                true,
		TrustPolicy:            map[string]any{"accounts": []string{"acct-1"}},
		PermissionBoundary:     boundary,
		SessionDurationSeconds: 3600,
		CreatedAt:              now,
	})
}

func assumeScopeTestRole(t *testing.T, svc *service.Service) domain.AssumeRoleResponse {
	t.Helper()
	session, err := svc.IAM().AssumeRole(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		"role-hr",
		domain.AssumeRoleInput{Reason: "scope intersection test"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if session.SessionID == "" {
		t.Fatalf("expected session id, got %+v", session)
	}
	return session
}

func authzScopeStringsFromAny(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func authzScopeHasPermissionAction(perms []domain.Permission, resource, action string) bool {
	for _, perm := range perms {
		if perm.Resource == resource && string(perm.Action) == action {
			return true
		}
	}
	return false
}

func authzScopeHasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
