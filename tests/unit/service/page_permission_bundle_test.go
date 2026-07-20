package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

// TestExplicitPageActionsAuthorizeAndPopulateMeResponse 驗證明確 action 授權與 /me 導覽輸出形成閉環。
func TestExplicitPageActionsAuthorizeAndPopulateMeResponse(t *testing.T) {
	now := pagePermissionTestNow()
	store := pagePermissionTestStore(now)
	pagePermissionUpsertSet(store, now, "ps-employees-page", []domain.Permission{
		pagePermissionAPI("workbench", "me", domain.ActionRead),
		pagePermissionAPI("hr.employees", "hr.employee", domain.ActionRead),
		pagePermissionAPI("hr.employees", "hr.employee", domain.ActionCreate),
		pagePermissionAPI("hr.employees", "hr.employee", domain.ActionExport),
		pagePermissionAPI("hr.employees", "hr.org_unit", domain.ActionRead),
		pagePermissionAPI("hr.employees", "hr.position", domain.ActionRead),
	})
	pagePermissionUpsertAccount(store, now, []string{"ps-employees-page"})
	svc := service.New(store)
	ctx := pagePermissionTestContext()

	for _, check := range []domain.CheckRequest{
		{Resource: "me", Action: domain.ActionRead},
		{Resource: "hr.employee", Action: domain.ActionCreate},
		{Resource: "hr.employee", Action: domain.ActionExport},
		{Resource: "hr.org_unit", Action: domain.ActionRead},
		{Resource: "hr.position", Action: domain.ActionRead},
	} {
		result, err := svc.Authz().Check(ctx, check)
		if err != nil {
			t.Fatal(err)
		}
		if !result.Allowed {
			t.Fatalf("expected explicit action to allow %s:%s, got %+v", check.Resource, check.Action, result)
		}
	}
	for _, check := range []domain.CheckRequest{
		{Resource: "hr.org_unit", Action: domain.ActionCreate},
		{Resource: "audit.audit_log", Action: domain.ActionRead},
	} {
		result, err := svc.Authz().Check(ctx, check)
		if err != nil {
			t.Fatal(err)
		}
		if result.Allowed {
			t.Fatalf("expected unrelated operation %s:%s to stay denied, got %+v", check.Resource, check.Action, result)
		}
	}

	me, err := svc.Me().Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !pagePermissionHasMenuKey(me.EffectiveMenuKeys, "hr.employees") {
		t.Fatalf("expected canonical page key in /me, got %+v", me.EffectiveMenuKeys)
	}
	if !pagePermissionHasPermission(me.EffectivePermissions, domain.PermissionTypeAPI, "hr.employee", domain.ActionCreate) {
		t.Fatalf("expected explicit API grant in /me, got %+v", me.EffectivePermissions)
	}
}

// TestPageMenuGrantDoesNotAuthorizeAPIWithoutExplicitAction 驗證 menu 僅控制導覽，不會合成 API 權限。
func TestPageMenuGrantDoesNotAuthorizeAPIWithoutExplicitAction(t *testing.T) {
	now := pagePermissionTestNow()
	store := pagePermissionTestStore(now)
	pagePermissionUpsertSet(store, now, "ps-menu-only", []domain.Permission{
		pagePermissionMenu("iam.members"),
	})
	pagePermissionUpsertAccount(store, now, []string{"ps-menu-only"})

	result, err := service.New(store).Authz().Check(pagePermissionTestContext(), domain.CheckRequest{
		Resource: "iam.permission_set_assignment",
		Action:   domain.ActionCreate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed {
		t.Fatalf("expected menu-only grant not to authorize API action, got %+v", result)
	}
}

// TestEmployeeBaseFormAssistantPermissionsStayMinimal verifies employees can run visible Agents without authoring or admin grants.
func TestEmployeeBaseFormAssistantPermissionsStayMinimal(t *testing.T) {
	content := service.DefaultHRPermissionPackageContent()
	var employeeBase *domain.PermissionSetTemplateContent
	for index := range content.PermissionSetTemplates {
		if content.PermissionSetTemplates[index].TemplateKey == "hr_employee_base" {
			employeeBase = &content.PermissionSetTemplates[index]
			break
		}
	}
	if employeeBase == nil {
		t.Fatal("hr_employee_base permission template is missing")
	}
	authoringTools := map[string]struct{}{
		"form.get_capabilities": {}, "form.get_data_source_schema": {}, "form.create_draft": {},
		"form.update_draft": {}, "form.validate_draft": {}, "form.preview_draft": {}, "form.simulate_workflow": {},
	}
	for _, permission := range employeeBase.Permissions {
		switch permission.Resource {
		case "agent.definition", "agent.model", "workflow.form_definition_draft":
			t.Fatalf("employee base must not include authoring or Agent administration permission: %+v", permission)
		case "agent.run":
			if permission.Action != domain.ActionRead && permission.Action != domain.ActionCreate {
				t.Fatalf("employee base must only read or create its Agent runtime state: %+v", permission)
			}
		case "agent.tool":
			if _, forbidden := authoringTools[permission.Target]; forbidden {
				t.Fatalf("employee base must not call form definition authoring tool %q", permission.Target)
			}
		}
	}

	now := pagePermissionTestNow()
	store := pagePermissionTestStore(now)
	pagePermissionUpsertSet(store, now, "ps-employee-base", employeeBase.Permissions)
	pagePermissionUpsertAccount(store, now, []string{"ps-employee-base"})
	svc := service.New(store)
	ctx := pagePermissionTestContext()
	for _, check := range []domain.CheckRequest{
		{Resource: "agent.run", Action: domain.ActionRead},
		{Resource: "agent.run", Action: domain.ActionCreate},
		{Resource: "agent.tool", Action: domain.ActionCall, Target: "list_published_form_templates"},
		{Resource: "agent.tool", Action: domain.ActionCall, Target: "get_published_form_template"},
		{Resource: "agent.tool", Action: domain.ActionCall, Target: "create_form_draft"},
		{Resource: "agent.tool", Action: domain.ActionCall, Target: "update_form_draft"},
		{Resource: "agent.tool", Action: domain.ActionCall, Target: "preview_form_submission"},
	} {
		result, err := svc.Authz().Check(ctx, check)
		if err != nil {
			t.Fatal(err)
		}
		if !result.Allowed {
			t.Fatalf("expected employee Form Assistant permission %s:%s:%s, got %+v", check.Resource, check.Action, check.Target, result)
		}
	}
	for _, check := range []domain.CheckRequest{
		{Resource: "agent.run", Action: domain.ActionUpdate},
		{Resource: "agent.run", Action: domain.ActionDelete},
		{Resource: "agent.definition", Action: domain.ActionRead},
		{Resource: "agent.model", Action: domain.ActionRead},
		{Resource: "workflow.form_definition_draft", Action: domain.ActionCreate},
		{Resource: "agent.tool", Action: domain.ActionCall, Target: "form.create_draft"},
	} {
		result, err := svc.Authz().Check(ctx, check)
		if err != nil {
			t.Fatal(err)
		}
		if result.Allowed {
			t.Fatalf("expected employee permission to remain denied for %s:%s:%s, got %+v", check.Resource, check.Action, check.Target, result)
		}
	}
	me, err := svc.Me().Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !pagePermissionHasMenuKey(me.EffectiveMenuKeys, "agents.runs") {
		t.Fatalf("expected Agent runtime read permission to expose agents.runs, got %+v", me.EffectiveMenuKeys)
	}
}

// TestCanonicalWorkspacePageActionsDeriveNavigation 驗證每個 canonical workspace 頁面的明確 action 可推導導覽。
func TestCanonicalWorkspacePageActionsDeriveNavigation(t *testing.T) {
	tests := []struct {
		menuKey         string
		primaryResource string
		resource        string
		action          domain.Action
	}{
		{menuKey: "workspace.overview", primaryResource: "hr.employee", resource: "hr.employee", action: domain.ActionRead},
		{menuKey: "hr.employees", primaryResource: "hr.employee", resource: "hr.employee", action: domain.ActionCreate},
		{menuKey: "hr.org_units", primaryResource: "hr.org_unit", resource: "hr.org_unit", action: domain.ActionUpdate},
		{menuKey: "hr.positions", primaryResource: "hr.position", resource: "hr.position", action: domain.ActionDelete},
		{menuKey: "hr.organization", primaryResource: "hr.employee", resource: "hr.employee", action: domain.ActionUpdate},
		{menuKey: "hr.turnover", primaryResource: "hr.employee", resource: "hr.employee", action: domain.ActionExport},
		{menuKey: "attendance.overview", primaryResource: "attendance.clock", resource: "attendance.clock", action: domain.ActionImport},
		{menuKey: "attendance.clock", primaryResource: "attendance.clock", resource: "attendance.correction", action: domain.ActionCreate},
		{menuKey: "attendance.leave_policy", primaryResource: "attendance.leave", resource: "attendance.leave", action: domain.ActionUpdate},
		{menuKey: "workflow.forms", primaryResource: "workflow.form_template", resource: "workflow.form_template", action: domain.ActionDelete},
		{menuKey: "agents.models", primaryResource: "agent.model", resource: "agent.model", action: domain.ActionDelete},
		{menuKey: "agents.definitions", primaryResource: "agent.definition", resource: "agent.definition", action: domain.ActionUpdate},
		{menuKey: "agents.usage", primaryResource: "agent.usage", resource: "agent.usage", action: domain.ActionRead},
		{menuKey: "iam.members", primaryResource: "iam.permission_set_assignment", resource: "iam.permission_set_assignment", action: domain.ActionCreate},
		{menuKey: "iam.user_groups", primaryResource: "iam.user_group", resource: "iam.user_group", action: domain.ActionDelete},
		{menuKey: "iam.permission_sets", primaryResource: "iam.permission_set", resource: "iam.permission_set", action: domain.ActionUpdate},
		{menuKey: "iam.assignments", primaryResource: "iam.permission_set_assignment", resource: "iam.permission_set_assignment", action: domain.ActionDelete},
		{menuKey: "iam.assumable_roles", primaryResource: "iam.assumable_role", resource: "iam.assumable_role", action: domain.ActionAssume},
		{menuKey: "iam.policies", primaryResource: "iam.data_scope", resource: "iam.field_policy", action: domain.ActionDelete},
		{menuKey: "audit.logs", primaryResource: "audit.audit_log", resource: "audit.audit_log", action: domain.ActionRead},
		{menuKey: "workbench", primaryResource: "me", resource: "me", action: domain.ActionRead},
		{menuKey: "agents.runs", primaryResource: "agent.run", resource: "agent.run", action: domain.ActionCreate},
		{menuKey: "workflow.instances", primaryResource: "workflow.form_instance", resource: "workflow.form_instance", action: domain.ActionApprove},
	}
	for _, test := range tests {
		t.Run(test.menuKey, func(t *testing.T) {
			now := pagePermissionTestNow()
			store := pagePermissionTestStore(now)
			pagePermissionUpsertSet(store, now, "ps-page", []domain.Permission{
				pagePermissionAPI("workbench", "me", domain.ActionRead),
				pagePermissionAPI(test.menuKey, test.primaryResource, domain.ActionRead),
				pagePermissionAPI(test.menuKey, test.resource, test.action),
			})
			pagePermissionUpsertAccount(store, now, []string{"ps-page"})
			result, err := service.New(store).Authz().Check(pagePermissionTestContext(), domain.CheckRequest{
				Resource: test.resource,
				Action:   test.action,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !result.Allowed {
				t.Fatalf("expected explicit %s action to allow %s:%s, got %+v", test.menuKey, test.resource, test.action, result)
			}
			primaryRead, err := service.New(store).Authz().Check(pagePermissionTestContext(), domain.CheckRequest{
				Resource: test.primaryResource,
				Action:   domain.ActionRead,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !primaryRead.Allowed {
				t.Fatalf("expected %s to include primary read %s:read, got %+v", test.menuKey, test.primaryResource, primaryRead)
			}
			me, err := service.New(store).Me().Resolve(pagePermissionTestContext())
			if err != nil {
				t.Fatal(err)
			}
			if !pagePermissionHasMenuKey(me.EffectiveMenuKeys, test.menuKey) {
				t.Fatalf("expected explicit primary read to expose %s, got %+v", test.menuKey, me.EffectiveMenuKeys)
			}
		})
	}
}

// TestAgentUsageMenuRequiresDedicatedTenantWideRead keeps definition readers out of account usage navigation.
func TestAgentUsageMenuRequiresDedicatedTenantWideRead(t *testing.T) {
	tests := []struct {
		name    string
		scopes  []domain.Scope
		visible bool
	}{
		{name: "unscoped", scopes: []domain.Scope{""}},
		{name: "self", scopes: []domain.Scope{domain.ScopeSelf}},
		{name: "own", scopes: []domain.Scope{domain.ScopeOwn}},
		{name: "all", scopes: []domain.Scope{domain.ScopeAll}, visible: true},
		{name: "tenant", scopes: []domain.Scope{domain.ScopeTenant}, visible: true},
		{name: "system_without_assumed_session", scopes: []domain.Scope{domain.ScopeSystem}},
		{name: "normal_union_all_and_self", scopes: []domain.Scope{domain.ScopeAll, domain.ScopeSelf}, visible: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := pagePermissionTestNow()
			store := pagePermissionTestStore(now)
			permissions := []domain.Permission{
				pagePermissionAPI("workbench", "me", domain.ActionRead),
				pagePermissionMenu("agents.usage"),
			}
			for _, scope := range test.scopes {
				permissions = append(permissions, domain.Permission{
					Resource:       "agent.usage",
					Action:         domain.ActionRead,
					Scope:          scope,
					PermissionType: domain.PermissionTypeAPI,
					MenuKey:        "agents.usage",
				})
			}
			pagePermissionUpsertSet(store, now, "ps-usage", permissions)
			pagePermissionUpsertAccount(store, now, []string{"ps-usage"})

			me, err := service.New(store).Me().Resolve(pagePermissionTestContext())
			if err != nil {
				t.Fatal(err)
			}
			if got := pagePermissionHasMenuKey(me.EffectiveMenuKeys, "agents.usage"); got != test.visible {
				t.Fatalf("expected agents.usage visibility %v for scopes %v, got keys %+v", test.visible, test.scopes, me.EffectiveMenuKeys)
			}
			if got := pagePermissionHasPermission(me.EffectivePermissions, domain.PermissionTypeMenu, "agents.usage", domain.ActionRead); got != test.visible {
				t.Fatalf("expected agents.usage menu grant visibility %v for scopes %v, got permissions %+v", test.visible, test.scopes, me.EffectivePermissions)
			}
		})
	}
}

// TestAgentDefinitionReadDoesNotExposeUsageMenu keeps Agent configuration separate from account PII and usage.
func TestAgentDefinitionReadDoesNotExposeUsageMenu(t *testing.T) {
	now := pagePermissionTestNow()
	store := pagePermissionTestStore(now)
	pagePermissionUpsertSet(store, now, "ps-definition-reader", []domain.Permission{
		pagePermissionAPI("workbench", "me", domain.ActionRead),
		pagePermissionAPI("agents.usage", "agent.definition", domain.ActionRead),
		pagePermissionMenu("agents.usage"),
	})
	pagePermissionUpsertAccount(store, now, []string{"ps-definition-reader"})

	me, err := service.New(store).Me().Resolve(pagePermissionTestContext())
	if err != nil {
		t.Fatal(err)
	}
	if pagePermissionHasMenuKey(me.EffectiveMenuKeys, "agents.usage") {
		t.Fatalf("expected definition read not to expose agents.usage, got %+v", me.EffectiveMenuKeys)
	}
	if pagePermissionHasPermission(me.EffectivePermissions, domain.PermissionTypeMenu, "agents.usage", domain.ActionRead) {
		t.Fatalf("expected stale agents.usage menu grant to be removed, got %+v", me.EffectivePermissions)
	}
}

// TestWorkspaceMenusRejectPersonalScopes keeps self-service permissions out of the tenant management navigation.
func TestWorkspaceMenusRejectPersonalScopes(t *testing.T) {
	now := pagePermissionTestNow()
	store := pagePermissionTestStore(now)
	pagePermissionUpsertSet(store, now, "ps-self-service", []domain.Permission{
		pagePermissionAPI("workbench", "me", domain.ActionRead),
		{Resource: "hr.employee", Action: domain.ActionRead, Scope: domain.ScopeSelf, MenuKey: "hr.employees"},
		{Resource: "attendance.clock", Action: domain.ActionRead, Scope: domain.ScopeSelf, MenuKey: "attendance.clock"},
		{Resource: "attendance.leave", Action: domain.ActionRead, Scope: domain.ScopeSelf, MenuKey: "attendance.leave"},
	})
	pagePermissionUpsertAccount(store, now, []string{"ps-self-service"})

	me, err := service.New(store).Me().Resolve(pagePermissionTestContext())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"hr.employees", "attendance.clock", "attendance.leave_policy"} {
		if pagePermissionHasMenuKey(me.EffectiveMenuKeys, key) {
			t.Fatalf("expected personal scope to hide workspace key %s, got %+v", key, me.EffectiveMenuKeys)
		}
	}
	if !pagePermissionHasMenuKey(me.EffectiveMenuKeys, "attendance.leave") {
		t.Fatalf("expected personal leave navigation to remain available, got %+v", me.EffectiveMenuKeys)
	}
}

// TestAgentUsageMenuUsesFinalAssumedRoleScope validates normal and assumed scope intersection before showing usage.
func TestAgentUsageMenuUsesFinalAssumedRoleScope(t *testing.T) {
	tests := []struct {
		name          string
		assumedScope  domain.Scope
		expectedScope domain.Scope
		allowed       bool
		visible       bool
	}{
		{name: "self", assumedScope: domain.ScopeSelf, expectedScope: domain.ScopeSelf},
		{name: "tenant", assumedScope: domain.ScopeTenant, expectedScope: domain.ScopeTenant, allowed: true, visible: true},
		{name: "all", assumedScope: domain.ScopeAll, expectedScope: domain.ScopeAll, allowed: true, visible: true},
		{name: "system", assumedScope: domain.ScopeSystem, expectedScope: domain.ScopeSystem, allowed: true, visible: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := pagePermissionTestNow()
			store := pagePermissionTestStore(now)
			pagePermissionUpsertSet(store, now, "ps-base", []domain.Permission{
				{Resource: "iam.assumable_role", Action: domain.ActionAssume, Target: "role-usage", Scope: domain.ScopeAll},
				pagePermissionAPI("workbench", "me", domain.ActionRead),
				pagePermissionAPI("agents.usage", "agent.usage", domain.ActionRead),
				pagePermissionMenu("agents.usage"),
			})
			pagePermissionUpsertSet(store, now, "ps-role-usage", []domain.Permission{
				pagePermissionAPI("workbench", "me", domain.ActionRead),
				{
					Resource:       "agent.usage",
					Action:         domain.ActionRead,
					Scope:          test.assumedScope,
					PermissionType: domain.PermissionTypeAPI,
					MenuKey:        "agents.usage",
				},
				pagePermissionMenu("agents.usage"),
			})
			pagePermissionUpsertAccount(store, now, []string{"ps-base"})
			_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
				ID:                     "role-usage",
				TenantID:               "tenant-1",
				Name:                   "Usage Scope Role",
				PermissionSetIDs:       []string{"ps-role-usage"},
				Trusted:                true,
				TrustPolicy:            map[string]any{"accounts": []string{"acct-1"}},
				PermissionBoundary:     map[string]any{"allow": []string{"platform.me.read", "agent.usage.read"}},
				SessionDurationSeconds: 3600,
				CreatedAt:              now,
			})
			svc := service.New(store, service.Options{Now: func() time.Time { return now }})
			session, err := svc.IAM().AssumeRole(pagePermissionTestContext(), "role-usage", domain.AssumeRoleInput{Reason: "usage scope test"})
			if err != nil {
				t.Fatal(err)
			}
			ctx := pagePermissionTestContext()
			ctx.AssumedRoleSessionID = session.SessionID
			decision, err := svc.Authz().Check(ctx, domain.CheckRequest{Resource: "agent.usage", Action: domain.ActionRead})
			if err != nil {
				t.Fatal(err)
			}
			if decision.Allowed != test.allowed || (test.allowed && decision.Scope != test.expectedScope) {
				t.Fatalf("expected allowed=%v final decision scope %s, got %+v", test.allowed, test.expectedScope, decision)
			}
			if !test.allowed && decision.Reason != "data scope denied" {
				t.Fatalf("expected non-tenant usage scope to fail closed, got %+v", decision)
			}

			me, err := svc.Me().Resolve(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if got := pagePermissionHasMenuKey(me.EffectiveMenuKeys, "agents.usage"); got != test.visible {
				t.Fatalf("expected agents.usage visibility %v for assumed scope %s, got keys %+v", test.visible, test.assumedScope, me.EffectiveMenuKeys)
			}
			if got := pagePermissionHasPermission(me.EffectivePermissions, domain.PermissionTypeMenu, "agents.usage", domain.ActionRead); got != test.visible {
				t.Fatalf("expected agents.usage menu grant visibility %v for assumed scope %s, got permissions %+v", test.visible, test.assumedScope, me.EffectivePermissions)
			}
		})
	}
}

// TestControlPermissionTypesCannotDirectlyAuthorizeAPI 驗證 menu、field、scope 不能因 resource/action 同名而直接命中 API。
func TestControlPermissionTypesCannotDirectlyAuthorizeAPI(t *testing.T) {
	for _, permissionType := range []domain.PermissionType{
		domain.PermissionTypeMenu,
		domain.PermissionTypeField,
		domain.PermissionTypeScope,
	} {
		t.Run(string(permissionType), func(t *testing.T) {
			now := pagePermissionTestNow()
			store := pagePermissionTestStore(now)
			pagePermissionUpsertSet(store, now, "ps-control", []domain.Permission{{
				Resource:       "attendance.clock",
				Action:         domain.ActionRead,
				Scope:          domain.ScopeAll,
				PermissionType: permissionType,
				MenuKey:        "custom.unmapped",
			}})
			pagePermissionUpsertAccount(store, now, []string{"ps-control"})

			result, err := service.New(store).Authz().Check(pagePermissionTestContext(), domain.CheckRequest{
				Resource: "attendance.clock",
				Action:   domain.ActionRead,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Allowed {
				t.Fatalf("expected %s permission not to authorize attendance.clock API, got %+v", permissionType, result)
			}
		})
	}
}

// TestLegacyMenuAliasesNeverExpandIntoAPIAccess 驗證舊 menu alias 只供導航 canonical 化，不能授予 API。
func TestLegacyMenuAliasesNeverExpandIntoAPIAccess(t *testing.T) {
	tests := []struct {
		name     string
		menuKey  string
		resource string
		actions  []domain.Action
	}{
		{name: "leave", menuKey: "attendance.leave", resource: "attendance.leave", actions: []domain.Action{domain.ActionUpdate}},
		{name: "reporting", menuKey: "hr.reporting", resource: "hr.employee", actions: []domain.Action{domain.ActionUpdate}},
		{name: "attendance_parent", menuKey: "attendance", resource: "attendance.clock", actions: []domain.Action{domain.ActionImport, domain.ActionExport}},
		{name: "audit", menuKey: "audit", resource: "audit.audit_log", actions: []domain.Action{domain.ActionRead}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := pagePermissionTestNow()
			store := pagePermissionTestStore(now)
			pagePermissionUpsertSet(store, now, "ps-alias", []domain.Permission{pagePermissionMenu(test.menuKey)})
			pagePermissionUpsertAccount(store, now, []string{"ps-alias"})
			svc := service.New(store)
			for _, action := range test.actions {
				result, err := svc.Authz().Check(pagePermissionTestContext(), domain.CheckRequest{Resource: test.resource, Action: action})
				if err != nil {
					t.Fatal(err)
				}
				if result.Allowed {
					t.Fatalf("expected legacy menu %s not to authorize %s:%s, got %+v", test.menuKey, test.resource, action, result)
				}
			}
		})
	}
}

// TestExplicitActionDenyOverridesAllow 驗證 deny assignment 會覆蓋相同的明確 action allow。
func TestExplicitActionDenyOverridesAllow(t *testing.T) {
	now := pagePermissionTestNow()
	store := pagePermissionTestStore(now)
	pagePermissionUpsertSet(store, now, "ps-page-allow", []domain.Permission{
		pagePermissionAPI("workbench", "me", domain.ActionRead),
		pagePermissionAPI("hr.employees", "hr.employee", domain.ActionRead),
		pagePermissionAPI("hr.employees", "hr.employee", domain.ActionCreate),
	})
	pagePermissionUpsertSet(store, now, "ps-page-deny", []domain.Permission{
		pagePermissionAPI("hr.employees", "hr.employee", domain.ActionCreate),
	})
	pagePermissionUpsertAccount(store, now, []string{"ps-page-allow"})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-page-deny",
		TenantID:        "tenant-1",
		PrincipalType:   string(domain.PrincipalTypeAccount),
		PrincipalID:     "acct-1",
		PermissionSetID: "ps-page-deny",
		Effect:          string(domain.EffectDeny),
		CreatedAt:       now,
	})

	svc := service.New(store)
	result, err := svc.Authz().Check(pagePermissionTestContext(), domain.CheckRequest{
		Resource: "hr.employee",
		Action:   domain.ActionCreate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed || result.Reason != "explicit deny" {
		t.Fatalf("expected explicit deny to override action allow, got %+v", result)
	}
	me, err := svc.Me().Resolve(pagePermissionTestContext())
	if err != nil {
		t.Fatalf("page deny must not deny the shared /me bootstrap: %v", err)
	}
	if !pagePermissionHasMenuKey(me.EffectiveMenuKeys, "hr.employees") {
		t.Fatalf("expected page to remain visible while primary read survives, got %+v", me.EffectiveMenuKeys)
	}
}

// TestPageHidesWhenPrimaryReadIsDenied 驗證 write 尚可執行時，主 read deny 仍會隱藏頁面。
func TestPageHidesWhenPrimaryReadIsDenied(t *testing.T) {
	now := pagePermissionTestNow()
	store := pagePermissionTestStore(now)
	pagePermissionUpsertSet(store, now, "ps-page-allow", []domain.Permission{
		pagePermissionAPI("workbench", "me", domain.ActionRead),
		pagePermissionAPI("hr.employees", "hr.employee", domain.ActionRead),
		pagePermissionAPI("hr.employees", "hr.employee", domain.ActionCreate),
	})
	pagePermissionUpsertSet(store, now, "ps-read-deny", []domain.Permission{{
		Resource: "hr.employee", Action: domain.ActionRead, Scope: domain.ScopeAll, PermissionType: domain.PermissionTypeAPI,
	}})
	pagePermissionUpsertAccount(store, now, []string{"ps-page-allow"})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-read-deny",
		TenantID:        "tenant-1",
		PrincipalType:   string(domain.PrincipalTypeAccount),
		PrincipalID:     "acct-1",
		PermissionSetID: "ps-read-deny",
		Effect:          string(domain.EffectDeny),
		CreatedAt:       now,
	})
	svc := service.New(store)
	ctx := pagePermissionTestContext()

	create, err := svc.Authz().Check(ctx, domain.CheckRequest{Resource: "hr.employee", Action: domain.ActionCreate})
	if err != nil {
		t.Fatal(err)
	}
	if !create.Allowed {
		t.Fatalf("expected non-denied create to remain allowed, got %+v", create)
	}
	read, err := svc.Authz().Check(ctx, domain.CheckRequest{Resource: "hr.employee", Action: domain.ActionRead})
	if err != nil {
		t.Fatal(err)
	}
	if read.Allowed {
		t.Fatalf("expected primary read to be denied, got %+v", read)
	}
	me, err := svc.Me().Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pagePermissionHasMenuKey(me.EffectiveMenuKeys, "hr.employees") {
		t.Fatalf("expected page key hidden without primary read, got %+v", me.EffectiveMenuKeys)
	}
	if !pagePermissionHasPermission(me.EffectivePermissions, domain.PermissionTypeAPI, "hr.employee", domain.ActionCreate) {
		t.Fatalf("expected surviving create permission in /me, got %+v", me.EffectivePermissions)
	}
}

// TestExplicitPageActionPreservesAssignmentDataScope 驗證明確 action 沿用 assignment data scope。
func TestExplicitPageActionPreservesAssignmentDataScope(t *testing.T) {
	now := pagePermissionTestNow()
	store := pagePermissionTestStore(now)
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-1", TenantID: "tenant-1", Name: "OU 1", Path: []string{"ou-1"}, CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", Name: "Employee 1", OrgUnitID: "ou-1", Status: "active", CreatedAt: now})
	_ = store.UpsertDataScope(context.Background(), domain.DataScope{ID: "ds-department", TenantID: "tenant-1", Code: "department", Name: "Department", ScopeType: string(domain.ScopeDepartment), CreatedAt: now})
	pagePermissionUpsertSet(store, now, "ps-page", []domain.Permission{pagePermissionAPI("hr.employees", "hr.employee", domain.ActionRead)})
	pagePermissionUpsertAccount(store, now, nil)
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-page-scope",
		TenantID:        "tenant-1",
		PrincipalType:   string(domain.PrincipalTypeAccount),
		PrincipalID:     "acct-1",
		PermissionSetID: "ps-page",
		Effect:          string(domain.EffectAllow),
		DataScopeID:     "ds-department",
		CreatedAt:       now,
	})

	result, err := service.New(store).Authz().Check(pagePermissionTestContext(), domain.CheckRequest{
		Resource: "hr.employee",
		Action:   domain.ActionRead,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Allowed || result.Scope != domain.ScopeDepartment {
		t.Fatalf("expected explicit permission to keep department scope, got %+v", result)
	}
}

// TestExplicitPageActionPreservesAssumedRoleBoundary 驗證明確 action 不繞過 assumed-role permission boundary。
func TestExplicitPageActionPreservesAssumedRoleBoundary(t *testing.T) {
	now := pagePermissionTestNow()
	store := pagePermissionTestStore(now)
	pagePermissionUpsertSet(store, now, "ps-base", []domain.Permission{
		{Resource: "iam.assumable_role", Action: domain.ActionAssume, Target: "role-page", Scope: domain.ScopeAll},
		pagePermissionAPI("workbench", "me", domain.ActionRead),
		pagePermissionAPI("hr.employees", "hr.employee", domain.ActionRead),
		pagePermissionAPI("hr.employees", "hr.employee", domain.ActionCreate),
	})
	pagePermissionUpsertSet(store, now, "ps-role-page", []domain.Permission{
		pagePermissionAPI("workbench", "me", domain.ActionRead),
		pagePermissionAPI("hr.employees", "hr.employee", domain.ActionRead),
		pagePermissionAPI("hr.employees", "hr.employee", domain.ActionCreate),
	})
	pagePermissionUpsertAccount(store, now, []string{"ps-base"})
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID:                     "role-page",
		TenantID:               "tenant-1",
		Name:                   "Page Reader",
		PermissionSetIDs:       []string{"ps-role-page"},
		Trusted:                true,
		TrustPolicy:            map[string]any{"accounts": []string{"acct-1"}},
		PermissionBoundary:     map[string]any{"allow": []string{"platform.me.read", "hr.employee.read"}},
		SessionDurationSeconds: 3600,
		CreatedAt:              now,
	})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	session, err := svc.IAM().AssumeRole(pagePermissionTestContext(), "role-page", domain.AssumeRoleInput{Reason: "page boundary test"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := pagePermissionTestContext()
	ctx.AssumedRoleSessionID = session.SessionID

	read, err := svc.Authz().Check(ctx, domain.CheckRequest{Resource: "hr.employee", Action: domain.ActionRead})
	if err != nil {
		t.Fatal(err)
	}
	if !read.Allowed {
		t.Fatalf("expected boundary-allowed page read, got %+v", read)
	}
	create, err := svc.Authz().Check(ctx, domain.CheckRequest{Resource: "hr.employee", Action: domain.ActionCreate})
	if err != nil {
		t.Fatal(err)
	}
	if create.Allowed {
		t.Fatalf("expected boundary to reject explicit create, got %+v", create)
	}
	me, err := svc.Me().Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pagePermissionHasPermission(me.EffectivePermissions, domain.PermissionTypeAPI, "hr.employee", domain.ActionCreate) {
		t.Fatalf("expected boundary-denied create to be absent from /me, got %+v", me.EffectivePermissions)
	}
	if !pagePermissionHasMenuKey(me.EffectiveMenuKeys, "hr.employees") {
		t.Fatalf("expected page to remain visible while read survives boundary, got %+v", me.EffectiveMenuKeys)
	}
}

// TestPageBoundaryWithOnlyWriteHidesPage 驗證 boundary 僅放行 write 時不會把 write 誤當頁面可見依據。
func TestPageBoundaryWithOnlyWriteHidesPage(t *testing.T) {
	now := pagePermissionTestNow()
	store := pagePermissionTestStore(now)
	pagePermissionUpsertSet(store, now, "ps-base", []domain.Permission{
		{Resource: "iam.assumable_role", Action: domain.ActionAssume, Target: "role-write", Scope: domain.ScopeAll},
		pagePermissionAPI("workbench", "me", domain.ActionRead),
		pagePermissionAPI("hr.employees", "hr.employee", domain.ActionRead),
		pagePermissionAPI("hr.employees", "hr.employee", domain.ActionCreate),
	})
	pagePermissionUpsertSet(store, now, "ps-role-page", []domain.Permission{
		pagePermissionAPI("workbench", "me", domain.ActionRead),
		pagePermissionAPI("hr.employees", "hr.employee", domain.ActionRead),
		pagePermissionAPI("hr.employees", "hr.employee", domain.ActionCreate),
	})
	pagePermissionUpsertAccount(store, now, []string{"ps-base"})
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID:                     "role-write",
		TenantID:               "tenant-1",
		Name:                   "Page Writer",
		PermissionSetIDs:       []string{"ps-role-page"},
		Trusted:                true,
		TrustPolicy:            map[string]any{"accounts": []string{"acct-1"}},
		PermissionBoundary:     map[string]any{"allow": []string{"platform.me.read", "hr.employee.create"}},
		SessionDurationSeconds: 3600,
		CreatedAt:              now,
	})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	session, err := svc.IAM().AssumeRole(pagePermissionTestContext(), "role-write", domain.AssumeRoleInput{Reason: "write-only boundary test"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := pagePermissionTestContext()
	ctx.AssumedRoleSessionID = session.SessionID

	create, err := svc.Authz().Check(ctx, domain.CheckRequest{Resource: "hr.employee", Action: domain.ActionCreate})
	if err != nil {
		t.Fatal(err)
	}
	if !create.Allowed {
		t.Fatalf("expected boundary-allowed create, got %+v", create)
	}
	me, err := svc.Me().Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pagePermissionHasMenuKey(me.EffectiveMenuKeys, "hr.employees") {
		t.Fatalf("expected page hidden when boundary removes primary read, got %+v", me.EffectiveMenuKeys)
	}
	if pagePermissionHasPermission(me.EffectivePermissions, domain.PermissionTypeMenu, "hr.employees", domain.ActionRead) {
		t.Fatalf("expected original menu hidden when boundary removes primary read, got %+v", me.EffectivePermissions)
	}
}

// pagePermissionTestNow 回傳固定測試時間。
func pagePermissionTestNow() time.Time {
	return time.Date(2030, 7, 10, 9, 0, 0, 0, time.UTC)
}

// pagePermissionTestStore 建立頁面授權測試 tenant。
func pagePermissionTestStore(now time.Time) *memory.Store {
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	return store
}

// pagePermissionUpsertSet 寫入測試 permission set。
func pagePermissionUpsertSet(store *memory.Store, now time.Time, id string, permissions []domain.Permission) {
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: id, TenantID: "tenant-1", Name: id, Permissions: permissions, CreatedAt: now,
	})
}

// pagePermissionUpsertAccount 寫入測試帳號。
func pagePermissionUpsertAccount(store *memory.Store, now time.Time, permissionSetIDs []string) {
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID: "acct-1", TenantID: "tenant-1", EmployeeID: "emp-1", Status: "active", DirectPermissionSetIDs: permissionSetIDs, CreatedAt: now,
	})
}

// pagePermissionMenu 建立只代表頁面的 menu grant。
func pagePermissionMenu(menuKey string) domain.Permission {
	return domain.Permission{
		Resource: menuKey, Action: domain.ActionRead, Scope: domain.ScopeAll, PermissionType: domain.PermissionTypeMenu, MenuKey: menuKey,
	}
}

// pagePermissionAPI 建立帶 menuKey 的明確 API action，供授權與導覽推導測試共用。
func pagePermissionAPI(menuKey, resource string, action domain.Action) domain.Permission {
	return domain.Permission{
		Resource: resource, Action: action, Scope: domain.ScopeAll, PermissionType: domain.PermissionTypeAPI, MenuKey: menuKey,
	}
}

// pagePermissionTestContext 建立頁面授權測試 request context。
func pagePermissionTestContext() domain.RequestContext {
	return domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
}

// pagePermissionHasPermission 檢查 /me 是否含指定型別與 resource/action。
func pagePermissionHasPermission(permissions []domain.Permission, permissionType domain.PermissionType, resource string, action domain.Action) bool {
	for _, permission := range permissions {
		if permission.PermissionType == permissionType && permission.Resource == resource && permission.Action == action {
			return true
		}
	}
	return false
}

// pagePermissionHasMenuKey 檢查 /me 是否含指定 menu key。
func pagePermissionHasMenuKey(keys []string, expected string) bool {
	for _, key := range keys {
		if key == expected {
			return true
		}
	}
	return false
}
