package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func TestMeProjectionIntersectsOrthogonalWildcardsAtConcreteRoute(t *testing.T) {
	svc, ctx := assumedProjectionEdgeFixture(t,
		[]domain.Permission{{ApplicationCode: domain.AppHR, ResourceType: "*", Action: domain.ActionRead, Scope: domain.ScopeAll}},
		[]domain.Permission{{ApplicationCode: "*", ResourceType: domain.ResourceEmployee, Action: "*", Scope: domain.ScopeAll}},
		map[string]any{"allow": []string{"hr.employee.read"}},
		service.Options{},
	)
	decision, err := svc.Authz().Check(ctx, domain.CheckRequest{Resource: "hr.employee", Action: domain.ActionRead})
	if err != nil || !decision.Allowed {
		t.Fatalf("expected concrete route to satisfy both orthogonal wildcards, decision=%+v err=%v", decision, err)
	}
	me, err := svc.Me().Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, permission := range me.EffectivePermissions {
		if permission.ApplicationCode == domain.AppHR && permission.ResourceType == domain.ResourceEmployee && permission.Action == domain.ActionRead {
			count++
		}
		if projectionTestPermissionHasWildcard(permission) {
			t.Fatalf("projection must not expose a wildcard capability: %+v", permission)
		}
	}
	if count != 1 {
		t.Fatalf("expected one concrete hr.employee.read projection, got %+v", me.EffectivePermissions)
	}
}

func TestMeProjectionKeepsAllowedTargetWhenDifferentTargetIsDenied(t *testing.T) {
	svc, ctx := assumedProjectionEdgeFixture(t,
		[]domain.Permission{
			{Resource: "agent.tool", Action: domain.ActionCall, Target: "get_my_profile", Scope: domain.ScopeAll},
			{Resource: "agent.tool", Action: domain.ActionCall, Target: "list_employees", Scope: domain.ScopeAll, Effect: "deny"},
		},
		[]domain.Permission{{Resource: "agent.tool", Action: domain.ActionCall, Target: "get_my_profile", Scope: domain.ScopeAll}},
		map[string]any{"allow": []string{"agent.tool.call"}},
		service.Options{},
	)
	decision, err := svc.Authz().Check(ctx, domain.CheckRequest{Resource: "agent.tool", Action: domain.ActionCall, Target: "get_my_profile"})
	if err != nil || !decision.Allowed {
		t.Fatalf("expected unrelated target deny not to block the requested tool, decision=%+v err=%v", decision, err)
	}
	me, err := svc.Me().Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, permission := range me.EffectivePermissions {
		if permission.ApplicationCode == domain.AppAgent && permission.ResourceType == domain.ResourceTool &&
			permission.Action == domain.ActionCall && permission.Target == "get_my_profile" {
			return
		}
	}
	t.Fatalf("expected allowed target to remain in projection, got %+v", me.EffectivePermissions)
}

func TestMeProjectionAppliesNormalAccountDataScopeAndConditions(t *testing.T) {
	now := time.Date(2026, 7, 16, 2, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAuthzScopeTenant(store, now)
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:          "ps-me",
		TenantID:    "tenant-1",
		Name:        "Me",
		Permissions: []domain.Permission{{Resource: "me", Action: domain.ActionRead, Scope: domain.ScopeSelf}},
		CreatedAt:   now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:          "ps-employee",
		TenantID:    "tenant-1",
		Name:        "Scoped employee",
		Permissions: []domain.Permission{{Resource: "hr.employee", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "hr.employees"}},
		CreatedAt:   now,
	})
	_ = store.UpsertDataScope(context.Background(), domain.DataScope{
		ID:        "ds-org",
		TenantID:  "tenant-1",
		Code:      "assigned_org",
		Name:      "Assigned org",
		ScopeType: string(domain.ScopeAssignedOrgUnits),
		Params:    map[string]any{"org_unit_ids": []string{"ou-a"}},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-account-org",
		TenantID:        "tenant-1",
		PrincipalType:   "account",
		PrincipalID:     "acct-1",
		PermissionSetID: "ps-employee",
		Effect:          "allow",
		DataScopeID:     "ds-org",
		CreatedAt:       now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		EmployeeID:             "emp-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-me"},
		CreatedAt:              now,
	})

	me, err := service.New(store).Me().Resolve(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, permission := range me.EffectivePermissions {
		if permission.ApplicationCode != domain.AppHR || permission.ResourceType != domain.ResourceEmployee || permission.Action != domain.ActionRead {
			continue
		}
		if permission.Scope != domain.ScopeAssignedOrgUnits ||
			!authzScopeHasString(authzScopeStringsFromAny(permission.Conditions["org_unit_ids"]), "ou-a") {
			t.Fatalf("expected authoritative normal-account scope and conditions, got %+v", permission)
		}
		if authzScopeHasString(me.EffectiveMenuKeys, "hr.employees") {
			t.Fatalf("scoped normal account must not expose tenant-wide HR menu, got %+v", me.EffectiveMenuKeys)
		}
		return
	}
	t.Fatalf("expected scoped normal-account permission, got %+v", me.EffectivePermissions)
}

func TestMeProjectionDoesNotAdvertiseNormalRelationOrDerivedObjectScope(t *testing.T) {
	for _, test := range []struct {
		name       string
		permission domain.Permission
		dataScope  *domain.DataScope
	}{
		{
			name: "relation",
			permission: domain.Permission{
				Resource: "hr.employee", Action: domain.ActionRead, Scope: domain.ScopeAll, Relation: "viewer", MenuKey: "hr.employees",
			},
		},
		{
			name:       "assignment_object_scope",
			permission: domain.Permission{Resource: "hr.employee", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "hr.employees"},
			dataScope: &domain.DataScope{
				ID: "ds-object", TenantID: "tenant-1", Code: "object", Name: "Object", ScopeType: string(domain.ScopeObject),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			now := time.Date(2026, 7, 16, 2, 0, 0, 0, time.UTC)
			store := memory.NewStore()
			seedAuthzScopeTenant(store, now)
			_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
				ID:          "ps-me",
				TenantID:    "tenant-1",
				Name:        "Me",
				Permissions: []domain.Permission{{Resource: "me", Action: domain.ActionRead, Scope: domain.ScopeSelf}},
				CreatedAt:   now,
			})
			_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
				ID:          "ps-object",
				TenantID:    "tenant-1",
				Name:        "Object",
				Permissions: []domain.Permission{test.permission},
				CreatedAt:   now,
			})
			account := domain.Account{
				ID: "acct-1", TenantID: "tenant-1", EmployeeID: "emp-1", Status: "active", DirectPermissionSetIDs: []string{"ps-me"}, CreatedAt: now,
			}
			if test.dataScope == nil {
				account.DirectPermissionSetIDs = append(account.DirectPermissionSetIDs, "ps-object")
			} else {
				test.dataScope.CreatedAt = now
				_ = store.UpsertDataScope(context.Background(), *test.dataScope)
				_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
					ID: "assign-object", TenantID: "tenant-1", PrincipalType: "account", PrincipalID: "acct-1", PermissionSetID: "ps-object", Effect: "allow", DataScopeID: test.dataScope.ID, CreatedAt: now,
				})
			}
			_ = store.UpsertAccount(context.Background(), account)
			svc := service.New(store, service.Options{Relationships: &fixedRelationshipChecker{allowed: true}})
			decision, err := svc.Authz().Check(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, domain.CheckRequest{
				Resource: "hr.employee", ResourceID: "emp-2", Action: domain.ActionRead,
			})
			if err != nil || !decision.Allowed {
				t.Fatalf("expected per-object backend decision to remain allowed, decision=%+v err=%v", decision, err)
			}
			me, err := svc.Me().Resolve(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
			if err != nil {
				t.Fatal(err)
			}
			for _, permission := range me.EffectivePermissions {
				if permission.ApplicationCode == domain.AppHR && permission.ResourceType == domain.ResourceEmployee && permission.Action == domain.ActionRead {
					t.Fatalf("object-only access must not be advertised as broad: %+v", permission)
				}
			}
		})
	}
}

func TestMeProjectionKeepsBroadGrantWhenSameRouteAlsoHasObjectScope(t *testing.T) {
	objectPermission := domain.Permission{Resource: "hr.employee", Action: domain.ActionRead, Scope: domain.ScopeObject}
	broadPermission := domain.Permission{Resource: "hr.employee", Action: domain.ActionRead, Scope: domain.ScopeAll}
	for _, test := range []struct {
		name        string
		permissions []domain.Permission
	}{
		{name: "object_before_all", permissions: []domain.Permission{objectPermission, broadPermission}},
		{name: "all_before_object", permissions: []domain.Permission{broadPermission, objectPermission}},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, ctx := assumedProjectionEdgeFixture(t,
				test.permissions,
				[]domain.Permission{broadPermission},
				map[string]any{"allow": []string{"hr.employee.read"}},
				service.Options{},
			)
			me, err := svc.Me().Resolve(ctx)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, permission := range me.EffectivePermissions {
				if permission.ApplicationCode == domain.AppHR && permission.ResourceType == domain.ResourceEmployee && permission.Action == domain.ActionRead {
					count++
					if permission.Scope != domain.ScopeAll {
						t.Fatalf("expected authoritative broad scope, got %+v", permission)
					}
				}
			}
			if count != 1 {
				t.Fatalf("expected one projected employee read permission, got %+v", me.EffectivePermissions)
			}
		})
	}
}

func TestMeProjectionKeepsPlainGrantWhenSameRouteAlsoHasRelation(t *testing.T) {
	relationPermission := domain.Permission{Resource: "hr.employee", Action: domain.ActionRead, Scope: domain.ScopeAll, Relation: "viewer"}
	plainPermission := domain.Permission{Resource: "hr.employee", Action: domain.ActionRead, Scope: domain.ScopeAll}
	for _, test := range []struct {
		name        string
		permissions []domain.Permission
	}{
		{name: "relation_before_plain", permissions: []domain.Permission{relationPermission, plainPermission}},
		{name: "plain_before_relation", permissions: []domain.Permission{plainPermission, relationPermission}},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, ctx := assumedProjectionEdgeFixture(t,
				test.permissions,
				[]domain.Permission{plainPermission},
				map[string]any{"allow": []string{"hr.employee.read"}},
				service.Options{},
			)
			me, err := svc.Me().Resolve(ctx)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, permission := range me.EffectivePermissions {
				if permission.ApplicationCode == domain.AppHR && permission.ResourceType == domain.ResourceEmployee && permission.Action == domain.ActionRead {
					count++
				}
			}
			if count != 1 {
				t.Fatalf("expected plain grant to survive relation candidate order, got %+v", me.EffectivePermissions)
			}
		})
	}
}

func TestMeProjectionPreservesMultipleMenuAssociationsForOneRoute(t *testing.T) {
	overviewPermission := domain.Permission{Resource: "hr.employee", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "workspace.overview"}
	employeesPermission := domain.Permission{Resource: "hr.employee", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "hr.employees"}
	plainPermission := domain.Permission{Resource: "hr.employee", Action: domain.ActionRead, Scope: domain.ScopeAll}
	for _, test := range []struct {
		name        string
		permissions []domain.Permission
	}{
		{name: "overview_before_employees", permissions: []domain.Permission{overviewPermission, employeesPermission}},
		{name: "employees_before_overview", permissions: []domain.Permission{employeesPermission, overviewPermission}},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, ctx := assumedProjectionEdgeFixture(t,
				test.permissions,
				[]domain.Permission{plainPermission},
				map[string]any{"allow": []string{"hr.employee.read"}},
				service.Options{},
			)
			me, err := svc.Me().Resolve(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, menuKey := range []string{"workspace.overview", "hr.employees"} {
				if !authzScopeHasString(me.EffectiveMenuKeys, menuKey) {
					t.Fatalf("expected menu association %s to survive grant order, got %+v", menuKey, me.EffectiveMenuKeys)
				}
			}
		})
	}
}

func TestAgentUsageProjectionRequiresExplicitTenantWideScopeOnBothSides(t *testing.T) {
	usageUnscoped := domain.Permission{Resource: "agent.usage", Action: domain.ActionRead, MenuKey: "agents.usage"}
	usageAll := domain.Permission{Resource: "agent.usage", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "agents.usage"}
	svc, ctx := assumedProjectionEdgeFixture(t,
		[]domain.Permission{usageUnscoped},
		[]domain.Permission{usageAll},
		map[string]any{"allow": []string{"agent.usage.read"}},
		service.Options{},
	)
	decision, err := svc.Authz().Check(ctx, domain.CheckRequest{Resource: "agent.usage", Action: domain.ActionRead})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.Reason != "data scope denied" {
		t.Fatalf("unscoped base permission must not be elevated by an assumed All scope, got %+v", decision)
	}
	me, err := svc.Me().Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, permission := range me.EffectivePermissions {
		if permission.ApplicationCode == domain.AppAgent && permission.ResourceType == domain.ResourceUsage && permission.Action == domain.ActionRead {
			t.Fatalf("unscoped usage must not be advertised as a client capability: %+v", permission)
		}
	}
	if authzScopeHasString(me.EffectiveMenuKeys, "agents.usage") {
		t.Fatalf("unscoped usage menu must remain hidden, got %+v", me.EffectiveMenuKeys)
	}
	_, err = svc.Agent().ListAccountUsage(ctx, domain.AgentAccountUsageQuery{}, domain.PageRequest{})
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Status != 403 || appErr.ReasonCode != "data_scope_denied" {
		t.Fatalf("usage service must fail closed with stable reason, got %v", err)
	}
}

func TestAgentUsageProjectionRejectsBoundaryNarrowedPersonalScope(t *testing.T) {
	usageAll := domain.Permission{Resource: "agent.usage", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "agents.usage"}
	svc, ctx := assumedProjectionEdgeFixture(t,
		[]domain.Permission{usageAll},
		[]domain.Permission{usageAll},
		map[string]any{"allow": []string{"agent.usage.read"}, "scope": string(domain.ScopeSelf)},
		service.Options{},
	)
	decision, err := svc.Authz().Check(ctx, domain.CheckRequest{Resource: "agent.usage", Action: domain.ActionRead})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.Reason != "data scope denied" {
		t.Fatalf("personal boundary scope must not authorize tenant-wide usage, got %+v", decision)
	}
	me, err := svc.Me().Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, permission := range me.EffectivePermissions {
		if permission.ApplicationCode == domain.AppAgent && permission.ResourceType == domain.ResourceUsage && permission.Action == domain.ActionRead {
			t.Fatalf("boundary-narrowed usage must not be advertised: %+v", permission)
		}
	}
	if authzScopeHasString(me.EffectiveMenuKeys, "agents.usage") {
		t.Fatalf("boundary-narrowed usage menu must remain hidden, got %+v", me.EffectiveMenuKeys)
	}
}

func TestMeProjectionUsesAuthoritativeRouteRiskLevel(t *testing.T) {
	usageTenant := domain.Permission{Resource: "agent.usage", Action: domain.ActionRead, Scope: domain.ScopeTenant}
	svc, ctx := assumedProjectionEdgeFixture(t,
		[]domain.Permission{usageTenant},
		[]domain.Permission{usageTenant},
		map[string]any{"allow": []string{"agent.usage.read"}},
		service.Options{},
	)
	me, err := svc.Me().Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, permission := range me.EffectivePermissions {
		if permission.ApplicationCode == domain.AppAgent && permission.ResourceType == domain.ResourceUsage && permission.Action == domain.ActionRead {
			if permission.RiskLevel != string(domain.RiskHigh) {
				t.Fatalf("expected authoritative high route risk, got %+v", permission)
			}
			return
		}
	}
	t.Fatalf("expected scoped usage permission, got %+v", me.EffectivePermissions)
}

func TestMeProjectionCachesDepartmentSubtreeResolutionAcrossMaterializedRoutes(t *testing.T) {
	now := time.Date(2026, 7, 16, 2, 0, 0, 0, time.UTC)
	baseStore := memory.NewStore()
	seedAuthzScopeTenant(baseStore, now)
	_ = baseStore.UpsertOrgUnit(context.Background(), domain.OrgUnit{
		ID: "ou-root", TenantID: "tenant-1", Name: "Root", Path: []string{"ou-root"}, CreatedAt: now,
	})
	_ = baseStore.UpsertOrgUnit(context.Background(), domain.OrgUnit{
		ID: "ou-child", TenantID: "tenant-1", Name: "Child", ParentID: "ou-root", Path: []string{"ou-root", "ou-child"}, CreatedAt: now,
	})
	_ = baseStore.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-1", TenantID: "tenant-1", Name: "Employee", OrgUnitID: "ou-root", Status: "active", CreatedAt: now,
	})
	_ = baseStore.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-me", TenantID: "tenant-1", Name: "Me",
		Permissions: []domain.Permission{{Resource: "me", Action: domain.ActionRead, Scope: domain.ScopeSelf}}, CreatedAt: now,
	})
	_ = baseStore.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-hr-wildcard", TenantID: "tenant-1", Name: "HR wildcard",
		Permissions: []domain.Permission{{ApplicationCode: domain.AppHR, ResourceType: "*", Action: domain.ActionRead, Scope: domain.ScopeAll}}, CreatedAt: now,
	})
	_ = baseStore.UpsertDataScope(context.Background(), domain.DataScope{
		ID: "ds-subtree", TenantID: "tenant-1", Code: "subtree", Name: "Department subtree", ScopeType: string(domain.ScopeDepartmentSubtree), CreatedAt: now,
	})
	_ = baseStore.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID: "assign-subtree", TenantID: "tenant-1", PrincipalType: "account", PrincipalID: "acct-1",
		PermissionSetID: "ps-hr-wildcard", Effect: "allow", DataScopeID: "ds-subtree", CreatedAt: now,
	})
	_ = baseStore.UpsertAccount(context.Background(), domain.Account{
		ID: "acct-1", TenantID: "tenant-1", EmployeeID: "emp-1", Status: "active", DirectPermissionSetIDs: []string{"ps-me"}, CreatedAt: now,
	})

	store := &projectionScopeCountingStore{Store: baseStore}
	me, err := service.New(store).Me().Resolve(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	projectedHRReads := 0
	for _, permission := range me.EffectivePermissions {
		if permission.ApplicationCode == domain.AppHR && permission.Action == domain.ActionRead {
			projectedHRReads++
		}
	}
	if projectedHRReads < 2 {
		t.Fatalf("fixture must materialize multiple HR routes, got %+v", me.EffectivePermissions)
	}
	if store.listOrgUnitCalls != 1 {
		t.Fatalf("department subtree lookup must be cached across routes, calls=%d", store.listOrgUnitCalls)
	}
}

func TestMeProjectionDeduplicatesLegacyAndTypedAPIAtCanonicalRoute(t *testing.T) {
	svc, ctx := assumedProjectionEdgeFixture(t,
		[]domain.Permission{{Resource: "audit.log", Action: domain.ActionRead, Scope: domain.ScopeAll}},
		[]domain.Permission{{PermissionType: domain.PermissionTypeAPI, Resource: "audit.audit_log", Action: domain.ActionRead, Scope: domain.ScopeAll}},
		map[string]any{"allow": []string{"audit.audit_log.read"}},
		service.Options{},
	)
	me, err := svc.Me().Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, permission := range me.EffectivePermissions {
		if permission.ApplicationCode == domain.AppAudit && permission.ResourceType == domain.ResourceType("audit_log") && permission.Action == domain.ActionRead {
			count++
			if permission.PermissionType != domain.PermissionTypeAPI {
				t.Fatalf("canonical duplicate should retain the explicit API type, got %+v", permission)
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected one canonical audit API permission, got %+v", me.EffectivePermissions)
	}
}

func assumedProjectionEdgeFixture(
	t *testing.T,
	basePermissions []domain.Permission,
	assumedPermissions []domain.Permission,
	boundary map[string]any,
	options service.Options,
) (*service.Service, domain.RequestContext) {
	t.Helper()
	now := time.Date(2026, 7, 16, 2, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAuthzScopeTenant(store, now)
	basePermissions = append([]domain.Permission{
		{Resource: "me", Action: domain.ActionRead, Scope: domain.ScopeAll},
		{Resource: "iam.assumable_role", Action: domain.ActionAssume, Target: "role-hr", Scope: domain.ScopeAll},
	}, basePermissions...)
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-base-edge", TenantID: "tenant-1", Name: "Base", Permissions: basePermissions, CreatedAt: now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-role-edge", TenantID: "tenant-1", Name: "Role", Permissions: assumedPermissions, CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID: "acct-1", TenantID: "tenant-1", EmployeeID: "emp-1", Status: "active", DirectPermissionSetIDs: []string{"ps-base-edge"}, CreatedAt: now,
	})
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID:                     "role-hr",
		TenantID:               "tenant-1",
		Name:                   "Projection role",
		Trusted:                true,
		TrustPolicy:            map[string]any{"accounts": []string{"acct-1"}},
		PermissionSetIDs:       []string{"ps-role-edge"},
		PermissionBoundary:     boundary,
		SessionDurationSeconds: 3600,
		CreatedAt:              now,
	})
	svc := service.New(store, options)
	session := assumeScopeTestRole(t, svc)
	return svc, domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", AssumedRoleSessionID: session.SessionID}
}

func projectionTestPermissionHasWildcard(permission domain.Permission) bool {
	return permission.ApplicationCode == "" || permission.ApplicationCode == "*" ||
		permission.ResourceType == "" || permission.ResourceType == "*" ||
		permission.Action == "" || permission.Action == "*"
}

type projectionScopeCountingStore struct {
	*memory.Store
	listOrgUnitCalls int
}

func (s *projectionScopeCountingStore) ListOrgUnits(ctx context.Context, tenantID string) ([]domain.OrgUnit, error) {
	s.listOrgUnitCalls++
	return s.Store.ListOrgUnits(ctx, tenantID)
}
