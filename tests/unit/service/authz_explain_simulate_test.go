package service_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func TestAuthzExplainIncludesAllowChainAndDenySources(t *testing.T) {
	now := authzExplainTestNow()
	store := memory.NewStore()
	seedAuthzExplainTenant(store, now)
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-export-allow",
		TenantID: "tenant-1",
		Name:     "Export Allow",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "export", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-export-deny",
		TenantID: "tenant-1",
		Name:     "Export Deny",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "export", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertUserGroup(context.Background(), domain.UserGroup{ID: "group.hr-managers", TenantID: "tenant-1", Name: "HR Managers", PermissionSetIDs: []string{"ps-export-allow"}, CreatedAt: now})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-account-deny",
		TenantID:        "tenant-1",
		PrincipalType:   "account",
		PrincipalID:     "acct-1",
		PermissionSetID: "ps-export-deny",
		Effect:          "deny",
		CreatedAt:       now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", UserGroupIDs: []string{"group.hr-managers"}, CreatedAt: now})

	result, err := service.New(store, service.Options{Now: func() time.Time { return now }}).Authz().Explain(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		domain.CheckRequest{Resource: "hr.employee", Action: "export"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision.Allowed || result.Decision.Reason != "explicit deny" {
		t.Fatalf("expected explicit deny decision, got %+v", result.Decision)
	}
	if !authzExplainHasGrant(result.EvaluatedGrants, "user_group", "group.hr-managers", "ps-export-allow", "allow", true, "") {
		t.Fatalf("expected matched allow grant from user group, got %+v", result.EvaluatedGrants)
	}
	if !authzExplainHasGrant(result.EvaluatedGrants, "account", "acct-1", "ps-export-deny", "deny", true, "explicit_deny") {
		t.Fatalf("expected matched explicit deny grant from account assignment, got %+v", result.EvaluatedGrants)
	}
	if !authzExplainHasString(result.DenySources, "account:acct-1:ps-export-deny") {
		t.Fatalf("expected deny source to include account assignment, got %+v", result.DenySources)
	}
}

func TestAuthzExplainShowsBoundaryExcludedGrant(t *testing.T) {
	now := authzExplainTestNow()
	store := memory.NewStore()
	seedAuthzExplainTenant(store, now)
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-base",
		TenantID: "tenant-1",
		Name:     "Base",
		Permissions: []domain.Permission{
			{Resource: "iam.assumable_role", Action: "assume", Target: "role-support", Scope: "all"},
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
		ID:                     "role-support",
		TenantID:               "tenant-1",
		Name:                   "Support",
		Trusted:                true,
		TrustPolicy:            map[string]any{"accounts": []string{"acct-1"}},
		PermissionSetIDs:       []string{"ps-role-export"},
		PermissionBoundary:     map[string]any{"allow": []string{"hr.employee.*"}, "deny": []string{"hr.employee.export"}},
		SessionDurationSeconds: 3600,
		CreatedAt:              now,
	})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	session, err := svc.IAM().AssumeRole(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true},
		"role-support",
		domain.AssumeRoleInput{Reason: "explain boundary"},
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := svc.Authz().Explain(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", AssumedRoleSessionID: session.SessionID},
		domain.CheckRequest{Resource: "hr.employee", Action: "export"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision.Allowed || result.Decision.Reason != "explicit deny" {
		t.Fatalf("expected boundary explicit deny, got %+v", result.Decision)
	}
	if !authzExplainHasGrant(result.EvaluatedGrants, "direct", "ps-base", "ps-base", "allow", true, "permission_boundary") {
		t.Fatalf("expected direct grant excluded by boundary, got %+v", result.EvaluatedGrants)
	}
	if !authzExplainHasBoundaryEffect(result.BoundaryEffects, "hr.employee.export", "deny") {
		t.Fatalf("expected boundary deny effect, got %+v", result.BoundaryEffects)
	}
}

func TestAuthzSimulateAddUserGroupFlipsAllowedWithoutAuthzWrites(t *testing.T) {
	now := authzExplainTestNow()
	store := memory.NewStore()
	seedAuthzExplainTenant(store, now)
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-export",
		TenantID: "tenant-1",
		Name:     "Export",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "export"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertUserGroup(context.Background(), domain.UserGroup{ID: "group-export", TenantID: "tenant-1", Name: "Exporters", PermissionSetIDs: []string{"ps-export"}, CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	beforeCounts := authzStoreCounts(t, store)

	result, err := service.New(store, service.Options{Now: func() time.Time { return now }}).Authz().Simulate(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		domain.AuthzSimulationRequest{
			Check: domain.CheckRequest{Resource: "hr.employee", Action: "export"},
			Overrides: domain.AuthzSimulationOverrides{
				AddUserGroups: []string{"group-export"},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Before.Allowed {
		t.Fatalf("expected before decision denied, got %+v", result.Before)
	}
	if !result.After.Allowed {
		t.Fatalf("expected after decision allowed, got %+v", result.After)
	}
	if !result.Diff.AllowedChanged || result.Diff.BeforeAllowed || !result.Diff.AfterAllowed {
		t.Fatalf("unexpected allowed diff: %+v", result.Diff)
	}
	if !authzExplainHasString(result.Diff.AddedMatchedBy, "group:group-export:ps-export") {
		t.Fatalf("expected added matched source for simulated group, got %+v", result.Diff)
	}
	if !authzExplainHasString(result.Diff.AddedMatchedPermissions, "hr.employee.export") {
		t.Fatalf("expected added matched permission, got %+v", result.Diff)
	}
	afterCounts := authzStoreCounts(t, store)
	if beforeCounts.permissionSets != afterCounts.permissionSets ||
		beforeCounts.assignments != afterCounts.assignments ||
		beforeCounts.groupMemberships != afterCounts.groupMemberships ||
		beforeCounts.outboxEvents != afterCounts.outboxEvents {
		t.Fatalf("simulate should not mutate authz config or outbox, before=%+v after=%+v", beforeCounts, afterCounts)
	}
	if afterCounts.auditLogs != beforeCounts.auditLogs+1 {
		t.Fatalf("simulate should write exactly one audit log, before=%+v after=%+v", beforeCounts, afterCounts)
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if logs[len(logs)-1].Action != "iam.authz.simulate" {
		t.Fatalf("expected simulate audit action, got %+v", logs[len(logs)-1])
	}
}

func TestAuthzExplainDecisionMatchesCheck(t *testing.T) {
	now := authzExplainTestNow()
	store := memory.NewStore()
	seedAuthzExplainTenant(store, now)
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-read",
		TenantID: "tenant-1",
		Name:     "Read",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-deny-export",
		TenantID: "tenant-1",
		Name:     "Deny Export",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "export", Effect: "deny", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-read", "ps-deny-export"}, CreatedAt: now})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	for _, req := range []domain.CheckRequest{
		{Resource: "hr.employee", Action: "read"},
		{Resource: "hr.employee", Action: "export"},
		{Resource: "hr.employee", Action: "delete"},
	} {
		check, err := svc.Authz().Check(ctx, req)
		if err != nil {
			t.Fatalf("check failed for %+v: %v", req, err)
		}
		explain, err := svc.Authz().Explain(ctx, req)
		if err != nil {
			t.Fatalf("explain failed for %+v: %v", req, err)
		}
		if !reflect.DeepEqual(check, explain.Decision) {
			t.Fatalf("explain decision drifted from check for %+v\ncheck=%+v\nexplain=%+v", req, check, explain.Decision)
		}
	}
}

// authzExplainTestNow 保持測試 session 在 memory store 的即時 clock 下仍為 active。
func authzExplainTestNow() time.Time {
	return time.Now().UTC().Truncate(time.Second)
}

func seedAuthzExplainTenant(store *memory.Store, now time.Time) {
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
}

func authzExplainHasGrant(grants []domain.AuthzEvaluatedGrant, source, sourceID, permissionSetID, effect string, matched bool, excludedBy string) bool {
	for _, grant := range grants {
		if grant.Source != source || grant.SourceID != sourceID || grant.PermissionSetID != permissionSetID || grant.Effect != effect || grant.Matched != matched {
			continue
		}
		if excludedBy == "" {
			return grant.ExcludedBy == nil
		}
		return grant.ExcludedBy != nil && *grant.ExcludedBy == excludedBy
	}
	return false
}

func authzExplainHasBoundaryEffect(effects []domain.AuthzBoundaryEffect, permission, effect string) bool {
	for _, item := range effects {
		if item.Source == "permission_boundary" && item.Permission == permission && item.Effect == effect && item.Matched {
			return true
		}
	}
	return false
}

func authzExplainHasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type authzStoreCountSnapshot struct {
	permissionSets   int
	assignments      int
	groupMemberships int
	auditLogs        int
	outboxEvents     int
}

func authzStoreCounts(t *testing.T, store *memory.Store) authzStoreCountSnapshot {
	t.Helper()
	permissionSets, err := store.ListPermissionSets(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	assignments, err := store.ListPermissionSetAssignments(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	memberships, err := store.ListGroupMembershipsForGroup(context.Background(), "tenant-1", "group-export")
	if err != nil {
		t.Fatal(err)
	}
	auditLogs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	outboxEvents, err := store.ListOutboxEvents(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	return authzStoreCountSnapshot{
		permissionSets:   len(permissionSets),
		assignments:      len(assignments),
		groupMemberships: len(memberships),
		auditLogs:        len(auditLogs),
		outboxEvents:     len(outboxEvents),
	}
}
