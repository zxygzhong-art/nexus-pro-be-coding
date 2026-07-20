package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
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
	seedActiveGroupMembership(t, store, "tenant-1", "grp-hr", "acct-1", now)
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
	seedActiveGroupMembership(t, store, "tenant-1", "grp-dept", "acct-1", now)
	seedActiveGroupMembership(t, store, "tenant-1", "grp-tenant", "acct-1", now)

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

// TestAssumedRoleAllScopeIsIntersectionIdentity verifies that all preserves the final assumed-role scope.
func TestAssumedRoleAllScopeIsIntersectionIdentity(t *testing.T) {
	tests := []struct {
		name         string
		assumedScope domain.Scope
	}{
		{name: "self", assumedScope: domain.ScopeSelf},
		{name: "tenant", assumedScope: domain.ScopeTenant},
		{name: "all", assumedScope: domain.ScopeAll},
		{name: "system", assumedScope: domain.ScopeSystem},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := authzScopeTestNow()
			store := memory.NewStore()
			seedAuthzScopeTenant(store, now)
			_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
				ID:       "ps-base",
				TenantID: "tenant-1",
				Name:     "Base",
				Permissions: []domain.Permission{
					{Resource: "iam.assumable_role", Action: domain.ActionAssume, Target: "role-hr", Scope: domain.ScopeAll},
					{Resource: "hr.employee", Action: domain.ActionRead, Scope: domain.ScopeAll},
				},
				CreatedAt: now,
			})
			_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
				ID:       "ps-role",
				TenantID: "tenant-1",
				Name:     "Role",
				Permissions: []domain.Permission{
					{Resource: "hr.employee", Action: domain.ActionRead, Scope: test.assumedScope},
				},
				CreatedAt: now,
			})
			_ = store.UpsertAccount(context.Background(), domain.Account{
				ID:                     "acct-1",
				TenantID:               "tenant-1",
				EmployeeID:             "emp-1",
				Status:                 "active",
				DirectPermissionSetIDs: []string{"ps-base"},
				CreatedAt:              now,
			})
			_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
				ID:                     "role-hr",
				TenantID:               "tenant-1",
				Name:                   "HR",
				Trusted:                true,
				TrustPolicy:            map[string]any{"accounts": []string{"acct-1"}},
				PermissionSetIDs:       []string{"ps-role"},
				PermissionBoundary:     map[string]any{"allow": []string{"hr.employee.read"}},
				SessionDurationSeconds: 3600,
				CreatedAt:              now,
			})

			svc := service.New(store)
			session := assumeScopeTestRole(t, svc)
			result, err := svc.Authz().Check(
				domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", AssumedRoleSessionID: session.SessionID},
				domain.CheckRequest{Resource: "hr.employee", Action: domain.ActionRead},
			)
			if err != nil {
				t.Fatal(err)
			}
			if !result.Allowed || result.Scope != test.assumedScope {
				t.Fatalf("expected all AND %s to keep %s, got %+v", test.assumedScope, test.assumedScope, result)
			}
		})
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
		PermissionBoundary:     map[string]any{"allow": []string{"hr.employee.*"}, "deny": []string{"hr.employee.export"}},
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

// TestMeProjectionUsesTheSameWildcardAndAuditAliasSemanticsAsAuthorization
// protects the caller projection from diverging from an allowed audit request.
func TestMeProjectionUsesTheSameWildcardAndAuditAliasSemanticsAsAuthorization(t *testing.T) {
	now := authzScopeTestNow()
	store := memory.NewStore()
	seedAuthzScopeTenant(store, now)
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-base-admin",
		TenantID: "tenant-1",
		Name:     "Base Wildcard Admin",
		Permissions: []domain.Permission{
			{Resource: "*", Action: "*", Scope: domain.ScopeAll},
		},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-audit-reader",
		TenantID: "tenant-1",
		Name:     "Audit Reader",
		Permissions: []domain.Permission{
			{Resource: "me", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "workbench"},
			{Resource: "audit.log", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "audit"},
			{Resource: "audit.audit_log", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "audit"},
			{Resource: "iam.permission_set", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "iam.permission_sets"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-base-admin"},
		CreatedAt:              now,
	})
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID:                     "role-hr",
		TenantID:               "tenant-1",
		Name:                   "Audit Reader",
		Trusted:                true,
		TrustPolicy:            map[string]any{"accounts": []string{"acct-1"}},
		PermissionSetIDs:       []string{"ps-audit-reader"},
		PermissionBoundary:     map[string]any{"allow": []string{"audit.log.read"}},
		SessionDurationSeconds: 3600,
		CreatedAt:              now,
	})

	svc := service.New(store)
	session := assumeScopeTestRole(t, svc)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", AssumedRoleSessionID: session.SessionID}
	for _, request := range []domain.CheckRequest{
		{Resource: "audit.log", Action: domain.ActionRead},
		{ApplicationCode: domain.AppAudit, ResourceType: "audit_log", Action: domain.ActionRead},
	} {
		decision, err := svc.Authz().Check(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		if !decision.Allowed {
			t.Fatalf("expected audit request %+v to be allowed, got %+v", request, decision)
		}
	}

	me, err := svc.Me().Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	auditReadCount := 0
	var hasIAMRead, hasWildcard bool
	for _, permission := range me.EffectivePermissions {
		switch {
		case permission.ApplicationCode == domain.AppAudit && permission.ResourceType == "audit_log" && permission.Action == domain.ActionRead:
			auditReadCount++
			if permission.Resource != "audit.audit_log" {
				t.Fatalf("expected canonical audit resource identity, got %+v", permission)
			}
		case permission.ApplicationCode == domain.AppIAM:
			hasIAMRead = true
		case permission.Resource == "*" || permission.Action == "*":
			hasWildcard = true
		}
	}
	if auditReadCount != 1 || !authzScopeHasString(me.EffectiveMenuKeys, "audit.logs") {
		t.Fatalf("expected canonical audit permission and menu projection, got permissions=%+v menus=%+v", me.EffectivePermissions, me.EffectiveMenuKeys)
	}
	if hasIAMRead || hasWildcard {
		t.Fatalf("expected boundary projection not to leak IAM or wildcard access, got %+v", me.EffectivePermissions)
	}
}

func TestMeProjectionNarrowsPermissionScopeToAuthoritativeDecision(t *testing.T) {
	tests := []struct {
		name          string
		baseScope     domain.Scope
		assumedScope  domain.Scope
		expectedScope domain.Scope
		menuVisible   bool
	}{
		{name: "base_self_assumed_all", baseScope: domain.ScopeSelf, assumedScope: domain.ScopeAll, expectedScope: domain.ScopeSelf},
		{name: "base_all_assumed_self", baseScope: domain.ScopeAll, assumedScope: domain.ScopeSelf, expectedScope: domain.ScopeSelf},
		{name: "base_tenant_assumed_all", baseScope: domain.ScopeTenant, assumedScope: domain.ScopeAll, expectedScope: domain.ScopeTenant, menuVisible: true},
		{name: "base_all_assumed_system", baseScope: domain.ScopeAll, assumedScope: domain.ScopeSystem, expectedScope: domain.ScopeSystem, menuVisible: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := authzScopeTestNow()
			store := memory.NewStore()
			seedAuthzScopeTenant(store, now)
			_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
				ID:       "ps-base",
				TenantID: "tenant-1",
				Name:     "Base",
				Permissions: []domain.Permission{
					{Resource: "iam.assumable_role", Action: domain.ActionAssume, Target: "role-hr", Scope: domain.ScopeAll},
					{Resource: "me", Action: domain.ActionRead, Scope: domain.ScopeAll},
					{Resource: "hr.employee", Action: domain.ActionRead, Scope: test.baseScope, MenuKey: "hr.employees"},
				},
				CreatedAt: now,
			})
			_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
				ID:       "ps-role",
				TenantID: "tenant-1",
				Name:     "Role",
				Permissions: []domain.Permission{
					{Resource: "hr.employee", Action: domain.ActionRead, Scope: test.assumedScope, MenuKey: "hr.employees"},
				},
				CreatedAt: now,
			})
			_ = store.UpsertAccount(context.Background(), domain.Account{
				ID:                     "acct-1",
				TenantID:               "tenant-1",
				EmployeeID:             "emp-1",
				Status:                 "active",
				DirectPermissionSetIDs: []string{"ps-base"},
				CreatedAt:              now,
			})
			_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
				ID:                     "role-hr",
				TenantID:               "tenant-1",
				Name:                   "Scoped Role",
				Trusted:                true,
				TrustPolicy:            map[string]any{"accounts": []string{"acct-1"}},
				PermissionSetIDs:       []string{"ps-role"},
				PermissionBoundary:     map[string]any{"allow": []string{"hr.employee.read"}},
				SessionDurationSeconds: 3600,
				CreatedAt:              now,
			})

			svc := service.New(store)
			session := assumeScopeTestRole(t, svc)
			ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", AssumedRoleSessionID: session.SessionID}
			decision, err := svc.Authz().Check(ctx, domain.CheckRequest{Resource: "hr.employee", Action: domain.ActionRead})
			if err != nil {
				t.Fatal(err)
			}
			if !decision.Allowed || decision.Scope != test.expectedScope {
				t.Fatalf("expected authoritative scope %s, got %+v", test.expectedScope, decision)
			}

			me, err := svc.Me().Resolve(ctx)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, permission := range me.EffectivePermissions {
				if permission.ApplicationCode != domain.AppHR || permission.ResourceType != domain.ResourceEmployee || permission.Action != domain.ActionRead {
					continue
				}
				count++
				if permission.Scope != test.expectedScope {
					t.Fatalf("projection must not exceed authoritative scope %s, got %+v", test.expectedScope, me.EffectivePermissions)
				}
			}
			if count != 1 {
				t.Fatalf("expected one narrowed HR read permission, got %+v", me.EffectivePermissions)
			}
			if got := authzScopeHasString(me.EffectiveMenuKeys, "hr.employees"); got != test.menuVisible {
				t.Fatalf("expected hr.employees visibility %v for scope %s, got %+v", test.menuVisible, test.expectedScope, me.EffectiveMenuKeys)
			}
		})
	}
}

func TestMeProjectionUsesCustomScopeForConditionalIntersection(t *testing.T) {
	now := authzScopeTestNow()
	store := memory.NewStore()
	seedAuthzScopeTenant(store, now)
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-base",
		TenantID: "tenant-1",
		Name:     "Base Self",
		Permissions: []domain.Permission{
			{Resource: "iam.assumable_role", Action: domain.ActionAssume, Target: "role-hr", Scope: domain.ScopeAll},
			{Resource: "me", Action: domain.ActionRead, Scope: domain.ScopeAll},
			{Resource: "hr.employee", Action: domain.ActionRead, Scope: domain.ScopeSelf, MenuKey: "hr.employees"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:          "ps-role",
		TenantID:    "tenant-1",
		Name:        "Assigned Org Reader",
		Permissions: []domain.Permission{{Resource: "hr.employee", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "hr.employees"}},
		CreatedAt:   now,
	})
	_ = store.UpsertDataScope(context.Background(), domain.DataScope{
		ID:        "ds-role-orgs",
		TenantID:  "tenant-1",
		Code:      "role_orgs",
		Name:      "Role Orgs",
		ScopeType: string(domain.ScopeAssignedOrgUnits),
		Params:    map[string]any{"org_unit_ids": []string{"ou-a"}},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-role-orgs",
		TenantID:        "tenant-1",
		PrincipalType:   "assumable_role",
		PrincipalID:     "role-hr",
		PermissionSetID: "ps-role",
		Effect:          "allow",
		DataScopeID:     "ds-role-orgs",
		CreatedAt:       now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		EmployeeID:             "emp-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-base"},
		CreatedAt:              now,
	})
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID:                     "role-hr",
		TenantID:               "tenant-1",
		Name:                   "Conditional Role",
		Trusted:                true,
		TrustPolicy:            map[string]any{"accounts": []string{"acct-1"}},
		PermissionBoundary:     map[string]any{"allow": []string{"hr.employee.read"}},
		SessionDurationSeconds: 3600,
		CreatedAt:              now,
	})

	svc := service.New(store)
	session := assumeScopeTestRole(t, svc)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", AssumedRoleSessionID: session.SessionID}
	decision, err := svc.Authz().Check(ctx, domain.CheckRequest{Resource: "hr.employee", Action: domain.ActionRead})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed || decision.Scope != domain.ScopeCustomCondition {
		t.Fatalf("expected conditional intersection to resolve to custom_condition, got %+v", decision)
	}
	me, err := svc.Me().Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, permission := range me.EffectivePermissions {
		if permission.ApplicationCode != domain.AppHR || permission.ResourceType != domain.ResourceEmployee || permission.Action != domain.ActionRead {
			continue
		}
		count++
		if permission.Scope != domain.ScopeCustomCondition {
			t.Fatalf("expected conditional projection to stay custom_condition, got %+v", me.EffectivePermissions)
		}
		if !authzScopeHasString(authzScopeStringsFromAny(permission.Conditions["employee_ids"]), "emp-1") ||
			!authzScopeHasString(authzScopeStringsFromAny(permission.Conditions["org_unit_ids"]), "ou-a") {
			t.Fatalf("expected projected custom conditions to match authorization, got %+v", permission.Conditions)
		}
	}
	if count != 1 {
		t.Fatalf("expected one condition-narrowed permission, got %+v", me.EffectivePermissions)
	}
	if authzScopeHasString(me.EffectiveMenuKeys, "hr.employees") {
		t.Fatalf("custom scope must not expose tenant-wide HR menu, got %+v", me.EffectiveMenuKeys)
	}
}

func TestMeProjectionMaterializesWildcardBeforeApplyingConcreteBoundaryDeny(t *testing.T) {
	now := authzScopeTestNow()
	store := &fieldPolicyCountingStore{Store: memory.NewStore()}
	seedAuthzScopeTenant(store.Store, now)
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-base-broad",
		TenantID: "tenant-1",
		Name:     "Base Broad HR",
		Permissions: []domain.Permission{
			{Resource: "me", Action: domain.ActionRead, Scope: domain.ScopeAll},
			{Resource: "iam.assumable_role", Action: domain.ActionAssume, Target: "role-hr", Scope: domain.ScopeAll},
			{Resource: "hr.*", Action: "*", Scope: domain.ScopeAll},
			{PermissionType: domain.PermissionTypeMenu, Resource: "hr.employees", Action: domain.ActionRead, MenuKey: "hr.employees"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:          "ps-role-broad",
		TenantID:    "tenant-1",
		Name:        "Assumed Broad HR",
		Permissions: []domain.Permission{{Resource: "hr.*", Action: "*", Scope: domain.ScopeAll}},
		CreatedAt:   now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-base-broad"},
		CreatedAt:              now,
	})
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID:                     "role-hr",
		TenantID:               "tenant-1",
		Name:                   "Broad HR Except Export",
		Trusted:                true,
		TrustPolicy:            map[string]any{"accounts": []string{"acct-1"}},
		PermissionSetIDs:       []string{"ps-role-broad"},
		PermissionBoundary:     map[string]any{"allow": []string{"hr.*"}, "deny": []string{"hr.employee.export"}},
		SessionDurationSeconds: 3600,
		CreatedAt:              now,
	})

	svc := service.New(store)
	session := assumeScopeTestRole(t, svc)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", AssumedRoleSessionID: session.SessionID}
	read, err := svc.Authz().Check(ctx, domain.CheckRequest{Resource: "hr.employee", Action: domain.ActionRead})
	if err != nil || !read.Allowed {
		t.Fatalf("expected concrete HR read to remain allowed, decision=%+v err=%v", read, err)
	}
	export, err := svc.Authz().Check(ctx, domain.CheckRequest{Resource: "hr.employee", Action: domain.ActionExport})
	if err != nil {
		t.Fatal(err)
	}
	if export.Allowed {
		t.Fatalf("expected concrete export deny to remain authoritative, got %+v", export)
	}

	fieldPolicyCallsBeforeMe := store.fieldPolicyCalls
	me, err := svc.Me().Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if delta := store.fieldPolicyCalls - fieldPolicyCallsBeforeMe; delta != 1 {
		t.Fatalf("expected one field-policy query for /me bootstrap and none per projected permission/menu, got %d", delta)
	}
	var hasRead, hasExport, hasWildcard bool
	for _, permission := range me.EffectivePermissions {
		if permission.ApplicationCode == domain.AppHR && permission.ResourceType == domain.ResourceEmployee {
			hasRead = hasRead || permission.Action == domain.ActionRead
			hasExport = hasExport || permission.Action == domain.ActionExport
		}
		hasWildcard = hasWildcard || permission.ApplicationCode == "*" || permission.ResourceType == "*" || permission.Action == "*"
	}
	if !hasRead || hasExport || hasWildcard {
		t.Fatalf("expected concrete allow projection without denied export or wildcard, got %+v", me.EffectivePermissions)
	}
	if !authzScopeHasString(me.EffectiveMenuKeys, "hr.employees") {
		t.Fatalf("expected explicit menu to survive until concrete permission validation, got %+v", me.EffectiveMenuKeys)
	}
}

func TestMeProjectionDoesNotAdvertiseObjectRelationAsBroadAccess(t *testing.T) {
	now := authzScopeTestNow()
	store := memory.NewStore()
	seedAuthzScopeTenant(store, now)
	relationPermission := domain.Permission{
		Resource: "hr.employee",
		Action:   domain.ActionRead,
		Scope:    domain.ScopeObject,
		Relation: "viewer",
		MenuKey:  "hr.employees",
	}
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-base-relation",
		TenantID: "tenant-1",
		Name:     "Base Object Reader",
		Permissions: []domain.Permission{
			{Resource: "me", Action: domain.ActionRead, Scope: domain.ScopeAll},
			{Resource: "iam.assumable_role", Action: domain.ActionAssume, Target: "role-hr", Scope: domain.ScopeAll},
			relationPermission,
		},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:          "ps-role-relation",
		TenantID:    "tenant-1",
		Name:        "Assumed Object Reader",
		Permissions: []domain.Permission{relationPermission},
		CreatedAt:   now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-base-relation"},
		CreatedAt:              now,
	})
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID:                     "role-hr",
		TenantID:               "tenant-1",
		Name:                   "Object Reader",
		Trusted:                true,
		TrustPolicy:            map[string]any{"accounts": []string{"acct-1"}},
		PermissionSetIDs:       []string{"ps-role-relation"},
		PermissionBoundary:     map[string]any{"allow": []string{"hr.employee.read"}},
		SessionDurationSeconds: 3600,
		CreatedAt:              now,
	})

	svc := service.New(store, service.Options{Relationships: &fixedRelationshipChecker{allowed: true}})
	session := assumeScopeTestRole(t, svc)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", AssumedRoleSessionID: session.SessionID}
	decision, err := svc.Authz().Check(ctx, domain.CheckRequest{Resource: "hr.employee", ResourceID: "emp-2", Action: domain.ActionRead})
	if err != nil || !decision.Allowed {
		t.Fatalf("expected concrete object relationship check to allow, decision=%+v err=%v", decision, err)
	}
	me, err := svc.Me().Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, permission := range me.EffectivePermissions {
		if permission.ApplicationCode == domain.AppHR && permission.ResourceType == domain.ResourceEmployee && permission.Action == domain.ActionRead {
			t.Fatalf("object-only relation must not be advertised as broad /me access: %+v", permission)
		}
	}
	if authzScopeHasString(me.EffectiveMenuKeys, "hr.employees") {
		t.Fatalf("object-only relation must not expose a tenant-wide menu, got %+v", me.EffectiveMenuKeys)
	}
}

func TestMeProjectionRejectsInvalidOrForeignAssumedSessionWithoutBaseFallback(t *testing.T) {
	now := authzScopeTestNow()
	store := memory.NewStore()
	seedAuthzScopeTenant(store, now)
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:          "ps-base-me",
		TenantID:    "tenant-1",
		Name:        "Base Me",
		Permissions: []domain.Permission{{Resource: "me", Action: domain.ActionRead, Scope: domain.ScopeAll}},
		CreatedAt:   now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-base-me"}, CreatedAt: now})
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{ID: "role-foreign", TenantID: "tenant-1", Name: "Foreign", CreatedAt: now})
	_ = store.UpsertAssumableRoleSession(context.Background(), domain.AssumableRoleSession{
		ID:              "synthetic-foreign-session",
		TenantID:        "tenant-1",
		AccountID:       "acct-other",
		AssumableRoleID: "role-foreign",
		ExpiresAt:       now.Add(time.Hour),
		CreatedAt:       now,
	})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})

	for name, sessionID := range map[string]string{
		"invalid": "synthetic-inactive-session",
		"foreign": "synthetic-foreign-session",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Me().Resolve(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", AssumedRoleSessionID: sessionID})
			appErr, ok := domain.AsAppError(err)
			if !ok || (appErr.Status != 404 && appErr.Status != 403) {
				t.Fatalf("expected supplied assumed session validation to fail closed, got %v", err)
			}
		})
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

type fieldPolicyCountingStore struct {
	*memory.Store
	fieldPolicyCalls int
}

func (s *fieldPolicyCountingStore) ListFieldPolicies(ctx context.Context, tenantID, applicationCode, resourceType string) ([]domain.FieldPolicy, error) {
	s.fieldPolicyCalls++
	return s.Store.ListFieldPolicies(ctx, tenantID, applicationCode, resourceType)
}
