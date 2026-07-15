package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func TestAssumableRoleTrustPolicyEmitsOpenFGATupleDiff(t *testing.T) {
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-iam-admin",
		TenantID: "tenant-1",
		Name:     "IAM Admin",
		Permissions: []domain.Permission{
			{Resource: "iam.assumable_role", Action: "create", Scope: "all"},
			{Resource: "iam.assumable_role", Action: "update", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-admin",
		TenantID:               "tenant-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-iam-admin"},
		CreatedAt:              now,
	})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}

	role, err := svc.IAM().CreateAssumableRole(ctx, domain.CreateAssumableRoleInput{
		Name:               "Ops Role",
		Trusted:            true,
		TrustPolicy:        map[string]any{"accounts": []string{"acct-old"}, "user_groups": []string{"ug-old"}},
		PermissionBoundary: map[string]any{"allow": []string{"hr.employee.*"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	tuples, err := store.ListAuthzRelationshipTuplesForObject(context.Background(), "tenant-1", "assumable_role", role.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !relationshipTupleExists(tuples, "tenant", "tenant", "tenant-1") ||
		!relationshipTupleExists(tuples, "trusted_user", "account", "acct-old") ||
		!relationshipTupleExists(tuples, "trusted_group", "user_group#member", "ug-old") {
		t.Fatalf("expected created assumable role trust tuples, got %+v", tuples)
	}

	_, err = svc.IAM().UpdateAssumableRole(ctx, role.ID, domain.UpdateAssumableRoleInput{
		TrustPolicy: map[string]any{"account_ids": []any{"acct-new"}, "user_group_ids": []any{"ug-new"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	tuples, err = store.ListAuthzRelationshipTuplesForObject(context.Background(), "tenant-1", "assumable_role", role.ID)
	if err != nil {
		t.Fatal(err)
	}
	if relationshipTupleExists(tuples, "trusted_user", "account", "acct-old") ||
		relationshipTupleExists(tuples, "trusted_group", "user_group#member", "ug-old") ||
		!relationshipTupleExists(tuples, "trusted_user", "account", "acct-new") ||
		!relationshipTupleExists(tuples, "trusted_group", "user_group#member", "ug-new") {
		t.Fatalf("expected updated trust tuple diff, got %+v", tuples)
	}
	events, err := store.ListOutboxEvents(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if countOutboxEvents(events, string(domain.EventOpenFGARelationshipWrite)) == 0 ||
		countOutboxEvents(events, string(domain.EventOpenFGARelationshipDelete)) == 0 {
		t.Fatalf("expected OpenFGA write/delete events for trust policy diff, got %+v", events)
	}
}

func TestAssumeRoleOpenFGACanAssumeGate(t *testing.T) {
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAssumableRoleFGAFixture(t, store, now)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	checkKey := relationshipCheckKey("account:acct-1", "can_assume", "assumable_role:role-hr")

	allowChecker := &mappedRelationshipChecker{allowed: map[string]bool{checkKey: true}}
	if _, err := service.New(store, service.Options{Relationships: allowChecker, OpenFGAScopeChecks: true}).IAM().AssumeRole(ctx, "role-hr", domain.AssumeRoleInput{Reason: "trusted group"}); err != nil {
		t.Fatal(err)
	}
	if !relationshipCheckSeen(allowChecker.checks, "can_assume", "assumable_role:role-hr") {
		t.Fatalf("expected can_assume OpenFGA check, got %+v", allowChecker.checks)
	}

	denyChecker := &mappedRelationshipChecker{allowed: map[string]bool{}}
	if _, err := service.New(store, service.Options{Relationships: denyChecker, OpenFGAScopeChecks: true}).IAM().AssumeRole(ctx, "role-hr", domain.AssumeRoleInput{Reason: "fga denied"}); err == nil {
		t.Fatal("expected OpenFGA can_assume denial")
	}

	errorChecker := &mappedRelationshipChecker{allowed: map[string]bool{}, err: errors.New("model not ready")}
	if _, err := service.New(store, service.Options{Relationships: errorChecker, OpenFGAScopeChecks: true}).IAM().AssumeRole(ctx, "role-hr", domain.AssumeRoleInput{Reason: "fail closed"}); err == nil {
		t.Fatal("expected OpenFGA error to deny role assumption")
	}

	disabledChecker := &mappedRelationshipChecker{allowed: map[string]bool{}}
	if _, err := service.New(store, service.Options{Relationships: disabledChecker, OpenFGAScopeChecks: false}).IAM().AssumeRole(ctx, "role-hr", domain.AssumeRoleInput{Reason: "switch off"}); err != nil {
		t.Fatalf("expected switch-off behavior to ignore OpenFGA, got %v", err)
	}
	if len(disabledChecker.checks) != 0 {
		t.Fatalf("expected no OpenFGA checks when switch is off, got %+v", disabledChecker.checks)
	}
}

func TestAgentToolOpenFGACanRunGate(t *testing.T) {
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	canRunKey := relationshipCheckKey("account:acct-1", "can_run", "agent_tool:knowledge.search")

	deniedStore := memory.NewStore()
	seedAgentToolFGAFixture(t, deniedStore, now, true)
	denyChecker := &mappedRelationshipChecker{allowed: map[string]bool{}}
	if _, err := service.New(deniedStore, service.Options{Relationships: denyChecker, OpenFGAScopeChecks: true}).Agent().CreateRun(ctx, domain.CreateAgentRunInput{Prompt: "search"}); err == nil {
		t.Fatal("expected agent_tool can_run denial")
	}
	if !relationshipCheckSeen(denyChecker.checks, "can_run", "agent_tool:knowledge.search") {
		t.Fatalf("expected can_run check, got %+v", denyChecker.checks)
	}

	errorStore := memory.NewStore()
	seedAgentToolFGAFixture(t, errorStore, now, true)
	errorChecker := &mappedRelationshipChecker{allowed: map[string]bool{}, err: errors.New("model not ready")}
	if _, err := service.New(errorStore, service.Options{Relationships: errorChecker, OpenFGAScopeChecks: true}).Agent().CreateRun(ctx, domain.CreateAgentRunInput{Prompt: "search"}); err == nil {
		t.Fatal("expected OpenFGA can_run error to fail closed")
	}

	allowedStore := memory.NewStore()
	seedAgentToolFGAFixture(t, allowedStore, now, true)
	allowedChecker := &mappedRelationshipChecker{allowed: map[string]bool{canRunKey: true}}
	run, err := service.New(allowedStore, service.Options{Relationships: allowedChecker, OpenFGAScopeChecks: true}).Agent().CreateRun(ctx, domain.CreateAgentRunInput{Prompt: "search"})
	if err != nil {
		t.Fatal(err)
	}
	if len(run.ToolDecisions) != 1 || run.ToolDecisions[0].RiskLevel != string(domain.RiskHigh) {
		t.Fatalf("expected allowed high-risk tool to retain audit risk level, got %+v", run.ToolDecisions)
	}

	disabledStore := memory.NewStore()
	seedAgentToolFGAFixture(t, disabledStore, now, false)
	disabledChecker := &mappedRelationshipChecker{allowed: map[string]bool{}}
	if _, err := service.New(disabledStore, service.Options{Relationships: disabledChecker, OpenFGAScopeChecks: false}).Agent().CreateRun(ctx, domain.CreateAgentRunInput{Prompt: "search"}); err != nil {
		t.Fatalf("expected switch-off behavior to ignore OpenFGA, got %v", err)
	}
	if len(disabledChecker.checks) != 0 {
		t.Fatalf("expected no OpenFGA checks when switch is off, got %+v", disabledChecker.checks)
	}
}

func TestOpenFGABackfillIncludesAssumableRoleAndAgentToolTuples(t *testing.T) {
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	_ = store.UpsertUserGroup(context.Background(), domain.UserGroup{ID: "ug-1", TenantID: "tenant-1", Name: "Trusted", MemberAccountIDs: []string{"acct-1"}, CreatedAt: now})
	seedActiveGroupMembership(t, store, "tenant-1", "ug-1", "acct-1", now)
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID:                 "role-hr",
		TenantID:           "tenant-1",
		Name:               "HR",
		Trusted:            true,
		TrustPolicy:        map[string]any{"accounts": []string{"acct-1"}, "user_groups": []string{"ug-1"}},
		PermissionBoundary: map[string]any{"allow": []string{"hr.employee.*"}},
		CreatedAt:          now,
	})

	result, err := service.New(store, service.Options{Now: func() time.Time { return now }}).OpenFGABackfillTuples(context.Background(), service.OpenFGABackfillInput{TenantID: "tenant-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.CreatedTuples == 0 {
		t.Fatalf("expected backfill to create tuples, got %+v", result)
	}
	roleTuples, err := store.ListAuthzRelationshipTuplesForObject(context.Background(), "tenant-1", "assumable_role", "role-hr")
	if err != nil {
		t.Fatal(err)
	}
	if !relationshipTupleExists(roleTuples, "tenant", "tenant", "tenant-1") ||
		!relationshipTupleExists(roleTuples, "trusted_user", "account", "acct-1") ||
		!relationshipTupleExists(roleTuples, "trusted_group", "user_group#member", "ug-1") {
		t.Fatalf("expected assumable_role backfill tuples, got %+v", roleTuples)
	}
	for _, toolID := range []string{"knowledge.search", "form.get_capabilities", "form.simulate_workflow"} {
		toolTuples, err := store.ListAuthzRelationshipTuplesForObject(context.Background(), "tenant-1", "agent_tool", toolID)
		if err != nil {
			t.Fatal(err)
		}
		if !relationshipTupleExists(toolTuples, "tenant", "tenant", "tenant-1") {
			t.Fatalf("expected agent_tool %q tenant backfill tuple, got %+v", toolID, toolTuples)
		}
	}
}

func TestOpenFGATenantAdminGrantIsIdempotent(t *testing.T) {
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-admin", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})

	first, err := svc.OpenFGAGrantTenantAdmin(context.Background(), service.OpenFGAGrantTenantAdminInput{TenantID: "tenant-1", AccountID: "acct-admin"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.OpenFGAGrantTenantAdmin(context.Background(), service.OpenFGAGrantTenantAdminInput{TenantID: "tenant-1", AccountID: "acct-admin"})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Created || first.Skipped || !second.Skipped || second.Created {
		t.Fatalf("expected idempotent tenant admin grant, first=%+v second=%+v", first, second)
	}
	tuples, err := store.ListAuthzRelationshipTuplesForObject(context.Background(), "tenant-1", "tenant", "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if !relationshipTupleExists(tuples, "admin", "account", "acct-admin") {
		t.Fatalf("expected tenant admin tuple, got %+v", tuples)
	}
}

func seedAssumableRoleFGAFixture(t *testing.T, store *memory.Store, now time.Time) {
	t.Helper()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-assume",
		TenantID: "tenant-1",
		Name:     "Assume",
		Permissions: []domain.Permission{
			{Resource: "iam.assumable_role", Action: "assume", Target: "role-hr", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertUserGroup(context.Background(), domain.UserGroup{ID: "ug-trusted", TenantID: "tenant-1", Name: "Trusted", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		Status:                 "active",
		UserGroupIDs:           []string{"ug-trusted"},
		DirectPermissionSetIDs: []string{"ps-assume"},
		CreatedAt:              now,
	})
	seedActiveGroupMembership(t, store, "tenant-1", "ug-trusted", "acct-1", now)
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID:                 "role-hr",
		TenantID:           "tenant-1",
		Name:               "HR Role",
		Trusted:            true,
		TrustPolicy:        map[string]any{"user_groups": []string{"ug-trusted"}},
		PermissionBoundary: map[string]any{"allow": []string{"hr.employee.*"}},
		CreatedAt:          now,
	})
}

func seedAgentToolFGAFixture(t *testing.T, store *memory.Store, now time.Time, highRiskTool bool) {
	t.Helper()
	risk := ""
	if highRiskTool {
		risk = string(domain.RiskHigh)
	}
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-agent",
		TenantID: "tenant-1",
		Name:     "Agent",
		Permissions: []domain.Permission{
			{Resource: "agent.run", Action: "create", Scope: "all"},
			{Resource: "agent.tool", Action: "call", Target: "knowledge.search", Scope: "all", RiskLevel: risk},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-agent"},
		CreatedAt:              now,
	})
}
