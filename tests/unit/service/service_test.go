package service_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"nexus-pro-be/internal/domain"
	authzpkg "nexus-pro-be/internal/domain/authz"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func TestCheckRequiresSpecificTarget(t *testing.T) {
	svc, ctx := newServiceFixture([]domain.Permission{
		{Resource: "hr.employee", Action: "read", Target: "emp-1"},
	})

	untargeted, err := svc.Check(ctx, domain.CheckRequest{Resource: "hr.employee", Action: "read"})
	if err != nil {
		t.Fatal(err)
	}
	if untargeted.Allowed {
		t.Fatalf("target-specific permission matched an untargeted request")
	}

	targeted, err := svc.Check(ctx, domain.CheckRequest{Resource: "hr.employee", Action: "read", Target: "emp-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !targeted.Allowed {
		t.Fatalf("target-specific permission did not match its target: %+v", targeted)
	}

	employeeTargeted, err := svc.Check(ctx, domain.CheckRequest{Resource: "hr.employee", Action: "read", TargetEmployeeID: "emp-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !employeeTargeted.Allowed {
		t.Fatalf("target-specific permission did not match target_employee_id: %+v", employeeTargeted)
	}

	resourceIDTargeted, err := svc.Check(ctx, domain.CheckRequest{Resource: "hr.employee", Action: "read", ResourceID: "emp-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !resourceIDTargeted.Allowed {
		t.Fatalf("target-specific permission did not match resource_id: %+v", resourceIDTargeted)
	}
}

func TestCreateAgentRunPreservesMultibyteReferenceSnippet(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-agent",
		TenantID: "tenant-1",
		Name:     "Agent Tool",
		Permissions: []domain.Permission{
			{Resource: "agent.tool", Action: "call", Target: "knowledge.search", Scope: "all"},
			{Resource: "agent.knowledge_article", Action: "read", Scope: "all"},
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
	_ = store.UpsertKnowledgeArticle(context.Background(), domain.KnowledgeArticle{
		ID:        "ka-1",
		TenantID:  "tenant-1",
		Title:     "请假制度",
		Content:   "A" + strings.Repeat("请", 121),
		CreatedAt: now,
	})

	run, err := service.New(store).CreateAgentRun(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, domain.CreateAgentRunInput{Prompt: "请"})
	if err != nil {
		t.Fatal(err)
	}
	if len(run.References) != 1 {
		t.Fatalf("expected one knowledge reference, got %d", len(run.References))
	}

	snippet := run.References[0].Snippet
	if !utf8.ValidString(snippet) {
		t.Fatalf("snippet is not valid UTF-8: %q", snippet)
	}
	if want := "A" + strings.Repeat("请", 119) + "..."; snippet != want {
		t.Fatalf("unexpected snippet: got %q want %q", snippet, want)
	}
	if len(run.ToolDecisions) != 1 || !run.ToolDecisions[0].Allowed {
		t.Fatalf("expected authorized knowledge tool decision, got %+v", run.ToolDecisions)
	}
}

func TestCreateAgentRunRequiresToolPermission(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	_ = store.UpsertKnowledgeArticle(context.Background(), domain.KnowledgeArticle{
		ID:        "ka-1",
		TenantID:  "tenant-1",
		Title:     "请假制度",
		Content:   "请假制度内容",
		CreatedAt: now,
	})

	_, err := service.New(store).CreateAgentRun(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, domain.CreateAgentRunInput{Prompt: "请假"})
	if err == nil {
		t.Fatal("expected agent tool gateway to reject missing tool permission")
	}
}

func TestCreateAgentRunFiltersUnauthorizedKnowledge(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-agent",
		TenantID: "tenant-1",
		Name:     "Agent Tool",
		Permissions: []domain.Permission{
			{Resource: "agent.tool", Action: "call", Target: "knowledge.search", Scope: "all"},
			{Resource: "agent.knowledge_article", Action: "read", Target: "ka-allowed", Scope: "all"},
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
	_ = store.UpsertKnowledgeArticle(context.Background(), domain.KnowledgeArticle{
		ID:        "ka-allowed",
		TenantID:  "tenant-1",
		Title:     "请假制度公开版",
		Content:   "请假公开制度",
		CreatedAt: now,
	})
	_ = store.UpsertKnowledgeArticle(context.Background(), domain.KnowledgeArticle{
		ID:        "ka-denied",
		TenantID:  "tenant-1",
		Title:     "请假制度敏感版",
		Content:   "请假敏感薪资字段",
		CreatedAt: now.Add(time.Minute),
	})

	run, err := service.New(store).CreateAgentRun(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, domain.CreateAgentRunInput{Prompt: "请假"})
	if err != nil {
		t.Fatal(err)
	}
	if len(run.References) != 1 || run.References[0].Title != "请假制度公开版" {
		t.Fatalf("expected only authorized knowledge reference, got %+v", run.References)
	}
	if strings.Contains(run.Answer, "敏感") {
		t.Fatalf("unauthorized knowledge leaked into answer: %s", run.Answer)
	}
}

func TestAuthzExplicitDenyWins(t *testing.T) {
	svc, ctx := newServiceFixture([]domain.Permission{
		{Resource: "hr.employee", Action: "read", Scope: "all"},
		{Resource: "hr.employee", Action: "read", Scope: "all", Effect: "deny"},
	})

	result, err := svc.Check(ctx, domain.CheckRequest{ApplicationCode: "hr", ResourceType: "employee", Action: "read", ResourceID: "emp-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed {
		t.Fatalf("expected explicit deny to win, got %+v", result)
	}
	if result.Reason != "explicit deny" {
		t.Fatalf("unexpected reason: %q", result.Reason)
	}
	if len(result.MissingPermissions) != 1 || result.MissingPermissions[0] != "hr.employee.read" {
		t.Fatalf("unexpected missing permissions: %+v", result.MissingPermissions)
	}
}

func TestPermissionRelationRequiresOpenFGA(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-read",
		TenantID: "tenant-1",
		Name:     "Relationship Read",
		Permissions: []domain.Permission{
			{Resource: "agent.knowledge_article", Action: "read", Scope: "all", Relation: "viewer"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-read"},
		CreatedAt:              now,
	})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	req := domain.CheckRequest{Resource: "agent.knowledge_article", ResourceID: "ka-1", Action: "read"}

	denyChecker := &fixedRelationshipChecker{allowed: false}
	denied, err := service.New(store, service.Options{Relationships: denyChecker}).Check(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if denied.Allowed || denied.Reason != "relationship denied" {
		t.Fatalf("expected OpenFGA relationship to deny permission, got %+v", denied)
	}

	allowChecker := &fixedRelationshipChecker{allowed: true}
	allowed, err := service.New(store, service.Options{Relationships: allowChecker}).Check(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !allowed.Allowed {
		t.Fatalf("expected OpenFGA relationship to allow permission, got %+v", allowed)
	}
	if len(allowChecker.checks) != 1 || allowChecker.checks[0].Relation != "viewer" || allowChecker.checks[0].Object != "agent.knowledge_article:ka-1" {
		t.Fatalf("unexpected relationship check: %+v", allowChecker.checks)
	}
}

func TestAssumableRoleSessionPolicyCanOnlyShrink(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-assume",
		TenantID: "tenant-1",
		Name:     "Assume Roles",
		Permissions: []domain.Permission{
			{Resource: "iam.assumable_role", Action: "assume", Target: "role-hr", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-role-hr",
		TenantID: "tenant-1",
		Name:     "HR Role",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
			{Resource: "hr.employee", Action: "export", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-assume"},
		CreatedAt:              now,
	})
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID:                     "role-hr",
		TenantID:               "tenant-1",
		Name:                   "HR Role",
		Trusted:                true,
		TrustPolicy:            map[string]any{"accounts": []string{"acct-1"}},
		PermissionSetIDs:       []string{"ps-role-hr"},
		PermissionBoundary:     map[string]any{"allow": []string{"hr.employee.*"}},
		SessionDurationSeconds: 3600,
		CreatedAt:              now,
	})
	svc := service.New(store)
	session, err := svc.AssumeRole(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, "role-hr", domain.AssumeRoleInput{
		Reason:          "test temporary HR read",
		DurationMinutes: 10,
		SessionPolicy:   map[string]any{"allow": []string{"hr.employee.read"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionID := session.SessionID
	if sessionID == "" {
		t.Fatalf("expected session id, got %+v", session)
	}

	assumedCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", AssumedRoleSessionID: sessionID}
	read, err := svc.Check(assumedCtx, domain.CheckRequest{Resource: "hr.employee", Action: "read"})
	if err != nil {
		t.Fatal(err)
	}
	if !read.Allowed || read.AssumedRole == nil || read.AssumedRole.SessionID != sessionID {
		t.Fatalf("expected read allowed through assumed role session, got %+v", read)
	}

	export, err := svc.Check(assumedCtx, domain.CheckRequest{Resource: "hr.employee", Action: "export"})
	if err != nil {
		t.Fatal(err)
	}
	if export.Allowed {
		t.Fatalf("expected session policy to shrink export permission, got %+v", export)
	}
}

func TestAssumableRoleSessionBypassesAuthzSnapshot(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-assume",
		TenantID: "tenant-1",
		Name:     "Assume Roles",
		Permissions: []domain.Permission{
			{Resource: "iam.assumable_role", Action: "assume", Target: "role-hr", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-role-hr",
		TenantID: "tenant-1",
		Name:     "HR Role",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-assume"},
		CreatedAt:              now,
	})
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID:                     "role-hr",
		TenantID:               "tenant-1",
		Name:                   "HR Role",
		Trusted:                true,
		TrustPolicy:            map[string]any{"accounts": []string{"acct-1"}},
		PermissionSetIDs:       []string{"ps-role-hr"},
		PermissionBoundary:     map[string]any{"allow": []string{"hr.employee.*"}},
		SessionDurationSeconds: 3600,
		CreatedAt:              now,
	})
	cache := &recordingAuthzSnapshot{values: map[string]domain.CheckResult{}}
	svc := service.New(store, service.Options{AuthzSnapshot: cache})
	session, err := svc.AssumeRole(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, "role-hr", domain.AssumeRoleInput{Reason: "test snapshot bypass"})
	if err != nil {
		t.Fatal(err)
	}
	assumedCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", AssumedRoleSessionID: session.SessionID}
	if result, err := svc.Check(assumedCtx, domain.CheckRequest{Resource: "hr.employee", Action: "read"}); err != nil || !result.Allowed {
		t.Fatalf("expected assumed role read before revocation, result=%+v err=%v", result, err)
	}
	if cache.gets != 1 || cache.sets != 1 {
		t.Fatalf("expected only the assume permission check to use snapshot, got gets=%d sets=%d", cache.gets, cache.sets)
	}
	revokedAt := time.Now().UTC()
	_ = store.UpsertAssumableRoleSession(context.Background(), domain.AssumableRoleSession{
		ID:              session.SessionID,
		TenantID:        "tenant-1",
		AccountID:       "acct-1",
		AssumableRoleID: "role-hr",
		ExpiresAt:       now.Add(time.Hour),
		RevokedAt:       &revokedAt,
		CreatedAt:       now,
	})
	if _, err := svc.Check(assumedCtx, domain.CheckRequest{Resource: "hr.employee", Action: "read"}); err == nil {
		t.Fatal("expected revoked assumed role session to be rejected instead of served from cache")
	}
	if cache.gets != 1 || cache.sets != 1 {
		t.Fatalf("assumed role checks should bypass snapshot, got gets=%d sets=%d", cache.gets, cache.sets)
	}
}

func TestDirectAssumedRoleIDDoesNotGrantRolePermissions(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:        "acct-1",
		TenantID:  "tenant-1",
		Status:    "active",
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-role-hr",
		TenantID: "tenant-1",
		Name:     "HR Role",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID:                 "role-hr",
		TenantID:           "tenant-1",
		Name:               "HR Role",
		Trusted:            true,
		TrustPolicy:        map[string]any{"accounts": []string{"acct-1"}},
		PermissionBoundary: map[string]any{"allow": []string{"hr.employee.*"}},
		PermissionSetIDs:   []string{"ps-role-hr"},
		CreatedAt:          now,
	})

	result, err := service.New(store).Check(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", AssumedRoleID: "role-hr"},
		domain.CheckRequest{Resource: "hr.employee", Action: "read"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed {
		t.Fatalf("direct assumed role id should not grant role permissions, got %+v", result)
	}
}

func TestTrustedAssumableRoleStillRequiresAssumePermission(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:        "acct-1",
		TenantID:  "tenant-1",
		Status:    "active",
		CreatedAt: now,
	})
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID:                 "role-hr",
		TenantID:           "tenant-1",
		Name:               "HR Role",
		Trusted:            true,
		TrustPolicy:        map[string]any{"accounts": []string{"acct-1"}},
		PermissionBoundary: map[string]any{"allow": []string{"hr.employee.*"}},
		CreatedAt:          now,
	})

	_, err := service.New(store).AssumeRole(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		"role-hr",
		domain.AssumeRoleInput{Reason: "test missing assume permission"},
	)
	if err == nil {
		t.Fatal("expected trusted role assumption to still require iam.assumable_role assume permission")
	}
}

func TestAccountActiveAssumableRoleIDDoesNotGrantRolePermissions(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                    "acct-1",
		TenantID:              "tenant-1",
		Status:                "active",
		ActiveAssumableRoleID: "role-hr",
		CreatedAt:             now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-role-hr",
		TenantID: "tenant-1",
		Name:     "HR Role",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID:                 "role-hr",
		TenantID:           "tenant-1",
		Name:               "HR Role",
		Trusted:            true,
		TrustPolicy:        map[string]any{"accounts": []string{"acct-1"}},
		PermissionBoundary: map[string]any{"allow": []string{"hr.employee.*"}},
		PermissionSetIDs:   []string{"ps-role-hr"},
		CreatedAt:          now,
	})

	result, err := service.New(store).Check(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		domain.CheckRequest{Resource: "hr.employee", Action: "read"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed || result.AssumedRole != nil {
		t.Fatalf("account active role id should not grant read without a session, got %+v", result)
	}
}

func TestResolveMeRequiresMeReadPermission(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:        "acct-1",
		TenantID:  "tenant-1",
		Status:    "active",
		CreatedAt: now,
	})

	_, err := service.New(store).ResolveMe(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err == nil {
		t.Fatal("expected /me resolution to require me read permission")
	}
}

func TestEmployeeReadAppliesAssignmentDataScopeAndFieldPolicy(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-read",
		TenantID: "tenant-1",
		Name:     "Scoped Employee Read",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertDataScope(context.Background(), domain.DataScope{
		ID:        "ds-reports",
		TenantID:  "tenant-1",
		Code:      "direct_reports",
		Name:      "Direct Reports",
		ScopeType: "direct_reports",
		Params:    map[string]any{"employee_ids": []string{"emp-2"}},
		CreatedAt: now,
	})
	_ = store.UpsertFieldPolicy(context.Background(), domain.FieldPolicy{
		ID:              "fp-mask-no",
		TenantID:        "tenant-1",
		ApplicationCode: "hr",
		ResourceType:    "employee",
		FieldName:       "employee_no",
		Effect:          "mask",
		CreatedAt:       now,
	})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-1",
		TenantID:        "tenant-1",
		PrincipalType:   "account",
		PrincipalID:     "acct-1",
		PermissionSetID: "ps-read",
		Effect:          "allow",
		DataScopeID:     "ds-reports",
		CreatedAt:       now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", EmployeeID: "emp-1", Status: "active", CreatedAt: now})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", EmployeeNo: "E0001", Name: "Employee One", Status: "active", CreatedAt: now})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-2", TenantID: "tenant-1", EmployeeNo: "E0002", Name: "Employee Two", Status: "active", CreatedAt: now.Add(time.Minute)})

	items, err := service.New(store).ListEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "emp-2" {
		t.Fatalf("expected only scoped employee, got %+v", items)
	}
	if items[0].EmployeeNo != "***" {
		t.Fatalf("expected employee number to be masked, got %+v", items[0])
	}
}

func TestDirectReportsScopeDerivesEmployeeIDs(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-read",
		TenantID: "tenant-1",
		Name:     "Scoped Employee Read",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertDataScope(context.Background(), domain.DataScope{ID: "ds-reports", TenantID: "tenant-1", Code: "direct_reports", Name: "Direct Reports", ScopeType: "direct_reports", CreatedAt: now})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-1",
		TenantID:        "tenant-1",
		PrincipalType:   "account",
		PrincipalID:     "acct-1",
		PermissionSetID: "ps-read",
		Effect:          "allow",
		DataScopeID:     "ds-reports",
		CreatedAt:       now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", EmployeeID: "emp-1", Status: "active", CreatedAt: now})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", Name: "Manager", Status: "active", CreatedAt: now})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-2", TenantID: "tenant-1", Name: "Report", ManagerEmployeeID: "emp-1", Status: "active", CreatedAt: now.Add(time.Minute)})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-3", TenantID: "tenant-1", Name: "Other", Status: "active", CreatedAt: now.Add(2 * time.Minute)})

	items, err := service.New(store).ListEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "emp-2" {
		t.Fatalf("expected only direct report, got %+v", items)
	}
}

func TestSameRankDataScopesMergeEmployeeIDs(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-read",
		TenantID: "tenant-1",
		Name:     "Scoped Employee Read",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertDataScope(context.Background(), domain.DataScope{ID: "ds-report-2", TenantID: "tenant-1", Code: "direct_reports", Name: "Direct Report 2", ScopeType: "direct_reports", Params: map[string]any{"employee_ids": []string{"emp-2"}}, CreatedAt: now})
	_ = store.UpsertDataScope(context.Background(), domain.DataScope{ID: "ds-report-3", TenantID: "tenant-1", Code: "direct_reports", Name: "Direct Report 3", ScopeType: "direct_reports", Params: map[string]any{"employee_ids": []string{"emp-3"}}, CreatedAt: now})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-1",
		TenantID:        "tenant-1",
		PrincipalType:   "account",
		PrincipalID:     "acct-1",
		PermissionSetID: "ps-read",
		Effect:          "allow",
		DataScopeID:     "ds-report-2",
		CreatedAt:       now,
	})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-2",
		TenantID:        "tenant-1",
		PrincipalType:   "account",
		PrincipalID:     "acct-1",
		PermissionSetID: "ps-read",
		Effect:          "allow",
		DataScopeID:     "ds-report-3",
		CreatedAt:       now.Add(time.Minute),
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", EmployeeID: "emp-1", Status: "active", CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", Name: "Manager", Status: "active", CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-2", TenantID: "tenant-1", Name: "Report 2", Status: "active", CreatedAt: now.Add(time.Minute)})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-3", TenantID: "tenant-1", Name: "Report 3", Status: "active", CreatedAt: now.Add(2 * time.Minute)})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-4", TenantID: "tenant-1", Name: "Other", Status: "active", CreatedAt: now.Add(3 * time.Minute)})

	items, err := service.New(store).ListEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "emp-2" || items[1].ID != "emp-3" {
		t.Fatalf("expected both same-rank direct reports, got %+v", items)
	}
}

func TestDepartmentSubtreeScopeDerivesOrgUnitIDs(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-root", TenantID: "tenant-1", Name: "Root", Path: []string{"ou-root"}, CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-child", TenantID: "tenant-1", Name: "Child", ParentID: "ou-root", Path: []string{"ou-root", "ou-child"}, CreatedAt: now.Add(time.Minute)})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-other", TenantID: "tenant-1", Name: "Other", Path: []string{"ou-other"}, CreatedAt: now.Add(2 * time.Minute)})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-read",
		TenantID: "tenant-1",
		Name:     "Scoped Employee Read",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertDataScope(context.Background(), domain.DataScope{ID: "ds-dept", TenantID: "tenant-1", Code: "department_subtree", Name: "Department", ScopeType: "department_subtree", CreatedAt: now})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-1",
		TenantID:        "tenant-1",
		PrincipalType:   "account",
		PrincipalID:     "acct-1",
		PermissionSetID: "ps-read",
		Effect:          "allow",
		DataScopeID:     "ds-dept",
		CreatedAt:       now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", EmployeeID: "emp-1", Status: "active", CreatedAt: now})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", Name: "Root Employee", OrgUnitID: "ou-root", Status: "active", CreatedAt: now})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-2", TenantID: "tenant-1", Name: "Child Employee", OrgUnitID: "ou-child", Status: "active", CreatedAt: now.Add(time.Minute)})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-3", TenantID: "tenant-1", Name: "Other Employee", OrgUnitID: "ou-other", Status: "active", CreatedAt: now.Add(2 * time.Minute)})

	items, err := service.New(store).ListEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "emp-1" || items[1].ID != "emp-2" {
		t.Fatalf("expected root department subtree employees, got %+v", items)
	}
}

func TestAssignedOrgUnitsScopeFiltersExactDepartments(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-read",
		TenantID: "tenant-1",
		Name:     "Assigned Org Read",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertDataScope(context.Background(), domain.DataScope{
		ID:        "ds-assigned",
		TenantID:  "tenant-1",
		Code:      "assigned_hr_orgs",
		Name:      "Assigned HR Orgs",
		ScopeType: "assigned_org_units",
		Params:    map[string]any{"org_unit_ids": []string{"ou-allowed"}},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-1",
		TenantID:        "tenant-1",
		PrincipalType:   "account",
		PrincipalID:     "acct-1",
		PermissionSetID: "ps-read",
		Effect:          "allow",
		DataScopeID:     "ds-assigned",
		CreatedAt:       now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", Name: "Allowed", OrgUnitID: "ou-allowed", Status: "active", CreatedAt: now})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-2", TenantID: "tenant-1", Name: "Child Is Not Included", OrgUnitID: "ou-child", Status: "active", CreatedAt: now.Add(time.Minute)})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-3", TenantID: "tenant-1", Name: "Other", OrgUnitID: "ou-other", Status: "active", CreatedAt: now.Add(2 * time.Minute)})

	items, err := service.New(store).ListEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "emp-1" {
		t.Fatalf("expected only exact assigned org employee, got %+v", items)
	}
}

func TestEmployeeFieldPolicyHidesDenyFieldsAndBlocksWrites(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-hr",
		TenantID: "tenant-1",
		Name:     "Employee Admin",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
			{Resource: "hr.employee", Action: "update", Scope: "all"},
			{Resource: "hr.employee", Action: "export", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertFieldPolicy(context.Background(), domain.FieldPolicy{
		ID:              "fp-hide-phone",
		TenantID:        "tenant-1",
		ApplicationCode: "hr",
		ResourceType:    "employee",
		FieldName:       "phone",
		Effect:          "hide",
		CreatedAt:       now,
	})
	_ = store.UpsertFieldPolicy(context.Background(), domain.FieldPolicy{
		ID:              "fp-deny-national",
		TenantID:        "tenant-1",
		ApplicationCode: "hr",
		ResourceType:    "employee",
		FieldName:       "national_id",
		Effect:          "deny",
		CreatedAt:       now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-hr"},
		CreatedAt:              now,
	})
	store.UpsertEmployee(context.Background(), domain.Employee{
		ID:           "emp-1",
		TenantID:     "tenant-1",
		EmployeeNo:   "E0001",
		Name:         "Employee One",
		CompanyEmail: "one@example.com",
		Phone:        "0912345678",
		Status:       "active",
		BasicInfo:    map[string]any{"national_id": "A123456789", "birthday": "1990-01-01"},
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	svc := service.New(store)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", RequestID: "req-field-policy", ApprovalConfirmed: true}

	items, err := svc.ListEmployees(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one employee, got %+v", items)
	}
	if items[0].Phone != "" {
		t.Fatalf("expected hidden phone to be empty, got %+v", items[0].Phone)
	}
	if _, ok := items[0].BasicInfo["national_id"]; ok {
		t.Fatalf("expected denied national_id to be removed, got %+v", items[0].BasicInfo)
	}

	nextPhone := "0999999999"
	_, err = svc.UpdateEmployee(ctx, "emp-1", domain.UpdateEmployeeInput{
		Phone:     &nextPhone,
		BasicInfo: map[string]any{"national_id": "B123456789"},
	})
	if err == nil || !strings.Contains(err.Error(), "employee field policy denied update") {
		t.Fatalf("expected field policy update error, got %v", err)
	}
	appErr, ok := domain.AsAppError(err)
	if !ok || len(appErr.FieldErrors) == 0 || appErr.FieldErrors[0].Code != "field_denied" {
		t.Fatalf("expected field_denied error code, got %v", err)
	}

	exported, err := svc.ExportEmployees(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported) != 1 || exported[0].Phone != "" {
		t.Fatalf("expected export to hide phone, got %+v", exported)
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	exportLog, ok := findAuditLog(logs, "hr.employee.export")
	if !ok {
		t.Fatalf("expected export audit log, got %+v", logs)
	}
	if exportLog.TraceID != "req-field-policy" || exportLog.Details["row_count"] != 1 {
		t.Fatalf("expected export audit trace and row count, got %+v", exportLog)
	}
	restricted, ok := exportLog.Details["restricted_fields"].(map[string][]string)
	if !ok || !stringSliceContains(restricted["hide"], "phone") || !stringSliceContains(restricted["deny"], "national_id") {
		t.Fatalf("expected export audit restricted fields, got %+v", exportLog.Details["restricted_fields"])
	}
}

func TestEmployeeExportRequiresApprovalAndAudits(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-export",
		TenantID: "tenant-1",
		Name:     "Employee Export",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "export", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-export"},
		CreatedAt:              now,
	})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", EmployeeNo: "E0001", Name: "Employee One", Status: "active", CreatedAt: now})
	svc := service.New(store)

	_, err := svc.ExportEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err == nil {
		t.Fatal("expected export to require approval confirmation")
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Details["requires_approval"] != true {
		t.Fatalf("expected approval audit log, got %+v", logs)
	}

	items, err := svc.ExportEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "emp-1" {
		t.Fatalf("expected confirmed export result, got %+v", items)
	}
}

func TestEmployeeExportRejectsOversizedSyncResult(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-export",
		TenantID: "tenant-1",
		Name:     "Employee Export",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "export", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-export"},
		CreatedAt:              now,
	})
	for i := 0; i < 5001; i++ {
		store.UpsertEmployee(context.Background(), domain.Employee{
			ID:               fmt.Sprintf("emp-%04d", i),
			TenantID:         "tenant-1",
			EmployeeNo:       fmt.Sprintf("E%04d", i),
			Name:             fmt.Sprintf("Employee %04d", i),
			CompanyEmail:     fmt.Sprintf("employee%04d@example.com", i),
			Status:           "active",
			EmploymentStatus: "active",
			CreatedAt:        now.Add(time.Duration(i) * time.Second),
			UpdatedAt:        now.Add(time.Duration(i) * time.Second),
		})
	}

	_, err := service.New(store).ExportEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true})
	if appErr, ok := domain.AsAppError(err); !ok || appErr.Code != "conflict" {
		t.Fatalf("expected oversized export conflict, got %v", err)
	}
}

func TestSelfScopedEmployeeReadOnlyReturnsCurrentEmployee(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-self",
		TenantID: "tenant-1",
		Name:     "Self Service",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "self"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		DisplayName:            "Employee One",
		EmployeeID:             "emp-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-self"},
		CreatedAt:              now,
	})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", Name: "Employee One", Status: "active", CreatedAt: now})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-2", TenantID: "tenant-1", Name: "Employee Two", Status: "active", CreatedAt: now.Add(time.Minute)})

	items, err := service.New(store).ListEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "emp-1" {
		t.Fatalf("expected only current employee, got %+v", items)
	}
}

func TestSelfScopedLeaveCreateCannotTargetAnotherEmployee(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-self",
		TenantID: "tenant-1",
		Name:     "Self Service",
		Permissions: []domain.Permission{
			{Resource: "attendance.leave", Action: "create", Scope: "self"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		DisplayName:            "Employee One",
		EmployeeID:             "emp-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-self"},
		CreatedAt:              now,
	})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", Name: "Employee One", Status: "active", CreatedAt: now})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-2", TenantID: "tenant-1", Name: "Employee Two", Status: "active", CreatedAt: now.Add(time.Minute)})

	_, err := service.New(store).CreateLeaveRequest(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		domain.CreateLeaveRequestInput{
			EmployeeID: "emp-2",
			LeaveType:  "annual",
			StartAt:    "2026-06-10",
			EndAt:      "2026-06-11",
			Hours:      8,
		},
	)
	if err == nil {
		t.Fatal("expected self-scoped account to be forbidden from creating leave for another employee")
	}
}

func TestSelfScopedLeaveReadOnlyReturnsCurrentEmployeeItems(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-self",
		TenantID: "tenant-1",
		Name:     "Self Service",
		Permissions: []domain.Permission{
			{Resource: "attendance.leave", Action: "read", Scope: "self"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		DisplayName:            "Employee One",
		EmployeeID:             "emp-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-self"},
		CreatedAt:              now,
	})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", Name: "Employee One", Status: "active", CreatedAt: now})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-2", TenantID: "tenant-1", Name: "Employee Two", Status: "active", CreatedAt: now.Add(time.Minute)})
	_ = store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{ID: "lb-1", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", RemainingHours: 8, UpdatedAt: now})
	_ = store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{ID: "lb-2", TenantID: "tenant-1", EmployeeID: "emp-2", LeaveType: "annual", RemainingHours: 8, UpdatedAt: now})
	_ = store.UpsertLeaveRequest(context.Background(), domain.LeaveRequest{ID: "lr-1", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", Hours: 8, Status: "pending", CreatedAt: now})
	_ = store.UpsertLeaveRequest(context.Background(), domain.LeaveRequest{ID: "lr-2", TenantID: "tenant-1", EmployeeID: "emp-2", LeaveType: "annual", Hours: 8, Status: "pending", CreatedAt: now.Add(time.Minute)})

	svc := service.New(store)
	balances, err := svc.ListLeaveBalances(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(balances) != 1 || balances[0].EmployeeID != "emp-1" {
		t.Fatalf("expected only current employee balance, got %+v", balances)
	}
	requests, err := svc.ListLeaveRequests(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 || requests[0].EmployeeID != "emp-1" {
		t.Fatalf("expected only current employee request, got %+v", requests)
	}
}

func TestCreateLeaveRequestReservesLeaveBalance(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-self",
		TenantID: "tenant-1",
		Name:     "Self Service",
		Permissions: []domain.Permission{
			{Resource: "attendance.leave", Action: "create", Scope: "self"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		DisplayName:            "Employee One",
		EmployeeID:             "emp-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-self"},
		CreatedAt:              now,
	})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", Name: "Employee One", Status: "active", CreatedAt: now})
	_ = store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{ID: "lb-1", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", RemainingHours: 16, UpdatedAt: now})

	created, err := service.New(store).CreateLeaveRequest(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		domain.CreateLeaveRequestInput{
			LeaveType: "annual",
			StartAt:   "2026-06-10",
			EndAt:     "2026-06-11",
			Hours:     8,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != "pending_approval" {
		t.Fatalf("unexpected leave request status: %s", created.Status)
	}
	balance, ok, err := store.GetLeaveBalance(context.Background(), "tenant-1", "lb-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("leave balance was not found")
	}
	if balance.RemainingHours != 8 {
		t.Fatalf("expected remaining balance 8, got %v", balance.RemainingHours)
	}
}

func TestCreateLeaveRequestRejectsInsufficientLeaveBalance(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-self",
		TenantID: "tenant-1",
		Name:     "Self Service",
		Permissions: []domain.Permission{
			{Resource: "attendance.leave", Action: "create", Scope: "self"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		DisplayName:            "Employee One",
		EmployeeID:             "emp-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-self"},
		CreatedAt:              now,
	})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", Name: "Employee One", Status: "active", CreatedAt: now})
	_ = store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{ID: "lb-1", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", RemainingHours: 4, UpdatedAt: now})

	_, err := service.New(store).CreateLeaveRequest(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		domain.CreateLeaveRequestInput{
			LeaveType: "annual",
			StartAt:   "2026-06-10",
			EndAt:     "2026-06-11",
			Hours:     8,
		},
	)
	if err == nil {
		t.Fatal("expected insufficient leave balance error")
	}
	if requests, err := store.ListLeaveRequests(context.Background(), "tenant-1"); err != nil || len(requests) != 0 {
		t.Fatalf("expected no leave request to be created, got %+v", requests)
	}
	forms, err := store.ListFormInstances(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(forms) != 0 {
		t.Fatalf("expected no form instance to be created, got %+v", forms)
	}
	balance, ok, err := store.GetLeaveBalance(context.Background(), "tenant-1", "lb-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("leave balance was not found")
	}
	if balance.RemainingHours != 4 {
		t.Fatalf("expected remaining balance to stay 4, got %v", balance.RemainingHours)
	}
}

func TestEmployeeAggregateCreatePatchAndDetail(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-1", TenantID: "tenant-1", Name: "HQ", Path: []string{"ou-1"}, CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-hr",
		TenantID: "tenant-1",
		Name:     "HR",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "create", Scope: "all"},
			{Resource: "hr.employee", Action: "read", Scope: "all"},
			{Resource: "hr.employee", Action: "update", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-hr"}, CreatedAt: now})
	svc := service.New(store)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	created, err := svc.CreateEmployee(ctx, domain.CreateEmployeeInput{
		EmployeeNo:            "E1001",
		Name:                  "Ada Chen",
		CompanyEmail:          "ada@example.com",
		OrgUnitID:             "ou-1",
		Position:              "Engineer",
		Category:              "full_time",
		HireDate:              "2026-06-01",
		BasicInfo:             map[string]any{"nationality_type": "local", "national_id": "A123456789"},
		EmploymentInfo:        map[string]any{"job_level": "senior"},
		EducationMilitaryInfo: map[string]any{"degree": "master"},
		ContactInfo:           map[string]any{"address": "Taipei"},
		InsuranceInfo:         map[string]any{"labor_insurance_salary": "45800"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.InternalExperiences) != 1 || !created.InternalExperiences[0].Current {
		t.Fatalf("expected initial current experience, got %+v", created.InternalExperiences)
	}

	newPhone := "0911222333"
	updated, err := svc.UpdateEmployee(ctx, created.ID, domain.UpdateEmployeeInput{
		Phone:       &newPhone,
		ContactInfo: map[string]any{"mobile_phone": newPhone},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ContactInfo["address"] != "Taipei" || updated.ContactInfo["mobile_phone"] != newPhone {
		t.Fatalf("expected patch to merge contact info, got %+v", updated.ContactInfo)
	}
	if updated.EducationMilitaryInfo["degree"] != "master" {
		t.Fatalf("expected untouched section to remain, got %+v", updated.EducationMilitaryInfo)
	}

	detail, err := svc.GetEmployee(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.BasicInfo["national_id"] != "A123456789" || detail.InsuranceInfo["labor_insurance_salary"] != "45800" {
		t.Fatalf("expected detail sections, got %+v", detail)
	}
}

func TestEmployeeAccountChangesEmitOpenFGATupleOutbox(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-hr",
		TenantID: "tenant-1",
		Name:     "HR",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "create", Scope: "all"},
			{Resource: "hr.employee", Action: "update", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-hr", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-hr"}, CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-old", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-new", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	svc := service.New(store)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-hr"}

	created, err := svc.CreateEmployee(ctx, domain.CreateEmployeeInput{
		EmployeeNo:   "E2001",
		Name:         "Relationship Owner",
		CompanyEmail: "relationship.owner@example.com",
		AccountID:    "acct-old",
		HireDate:     "2026-06-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	tuples, err := store.ListAuthzRelationshipTuplesForObject(context.Background(), "tenant-1", "hr.employee", created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tuples) != 1 || tuples[0].Relation != "owner" || tuples[0].SubjectID != "acct-old" {
		t.Fatalf("expected owner tuple for old account, got %+v", tuples)
	}

	newAccountID := "acct-new"
	if _, err := svc.UpdateEmployee(ctx, created.ID, domain.UpdateEmployeeInput{AccountID: &newAccountID}); err != nil {
		t.Fatal(err)
	}
	tuples, err = store.ListAuthzRelationshipTuplesForObject(context.Background(), "tenant-1", "hr.employee", created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tuples) != 1 || tuples[0].Relation != "owner" || tuples[0].SubjectID != "acct-new" {
		t.Fatalf("expected owner tuple to move to new account, got %+v", tuples)
	}
	events, err := store.ListAuthzOutboxEvents(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if !hasOutboxEvent(events, string(domain.EventOpenFGARelationshipWrite)) || !hasOutboxEvent(events, string(domain.EventOpenFGARelationshipDelete)) {
		t.Fatalf("expected OpenFGA write/delete outbox events, got %+v", events)
	}
}

func TestEmployeeCreateRejectsDuplicateUniqueFields(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-hr",
		TenantID: "tenant-1",
		Name:     "HR",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "create", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-hr"}, CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-linked", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	store.UpsertEmployee(context.Background(), domain.Employee{
		ID:           "emp-existing",
		TenantID:     "tenant-1",
		EmployeeNo:   "E1001",
		Name:         "Existing Employee",
		CompanyEmail: "duplicate@example.com",
		AccountID:    "acct-linked",
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	svc := service.New(store)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	_, err := svc.CreateEmployee(ctx, domain.CreateEmployeeInput{
		EmployeeNo:   "E1001",
		Name:         "Duplicate Employee",
		CompanyEmail: "duplicate@example.com",
		AccountID:    "acct-linked",
	})
	if err == nil {
		t.Fatal("expected duplicate unique fields to fail")
	}
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Code != "validation_failed" {
		t.Fatalf("expected validation_failed, got %v", err)
	}
	codes := map[string]string{}
	for _, field := range appErr.FieldErrors {
		codes[field.Field] = field.Code
	}
	for _, field := range []string{"employee_no", "company_email", "account_id"} {
		if codes[field] != "unique" {
			t.Fatalf("expected %s unique error, got %+v", field, appErr.FieldErrors)
		}
	}
}

func TestEmployeeStatusTransitionHandlesEmptyEmploymentInfo(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-hr",
		TenantID: "tenant-1",
		Name:     "HR",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "create", Scope: "all"},
			{Resource: "hr.employee", Action: "status_transition", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-hr"}, CreatedAt: now})
	svc := service.New(store)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}

	leaveTarget, err := svc.CreateEmployee(ctx, domain.CreateEmployeeInput{
		Name:         "Leave Target",
		CompanyEmail: "leave.target@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.TransitionEmployeeStatus(ctx, leaveTarget.ID, domain.StatusTransitionInput{Status: "leave_suspended"})
	if appErr, ok := domain.AsAppError(err); !ok || appErr.Code != "validation_failed" {
		t.Fatalf("expected missing leave dates validation, got %v", err)
	}

	resignTarget, err := svc.CreateEmployee(ctx, domain.CreateEmployeeInput{
		Name:         "Resign Target",
		CompanyEmail: "resign.target@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resignTarget.EmploymentInfo != nil {
		t.Fatalf("test setup expected empty employment info, got %+v", resignTarget.EmploymentInfo)
	}
	transitioned, err := svc.TransitionEmployeeStatus(ctx, resignTarget.ID, domain.StatusTransitionInput{
		Status:  "resigned",
		Reason:  "left voluntarily",
		EndDate: "2026-06-30",
	})
	if err != nil {
		t.Fatal(err)
	}
	if transitioned.EmploymentStatus != "resigned" || transitioned.EmploymentInfo["transition_reason"] != "left voluntarily" {
		t.Fatalf("expected resigned transition details, got %+v", transitioned)
	}
}

func TestEmployeeImportPreviewConfirmAndStatusTransition(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-1", TenantID: "tenant-1", Name: "HQ", Path: []string{"ou-1"}, CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-hr",
		TenantID: "tenant-1",
		Name:     "HR",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "import", Scope: "all"},
			{Resource: "hr.employee", Action: "read", Scope: "all"},
			{Resource: "hr.employee", Action: "status_transition", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-hr"}, CreatedAt: now})
	objects := &recordingObjectStore{}
	svc := service.New(store, service.Options{ObjectStore: objects})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}

	session, err := svc.PreviewEmployeeImport(ctx, domain.EmployeeImportPreviewInput{
		Filename: "employees.csv",
		Content:  "員工編號,姓名,Email,部門,職位,類別,電話,狀態,到職日期\nE2001,Partial Wu,partial@example.com,ou-1,HRBP,全職,0911000222,在職,2026-06-01\nE2001,Duplicate,dup@example.com,ou-1,HRBP,全職,0911000333,在職,2026-06-01\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.Summary["valid"] != 1 || session.Summary["invalid"] != 1 {
		t.Fatalf("expected one valid and one invalid row, got %+v", session.Summary)
	}
	if len(objects.keys) != 1 || objects.keys[0] != session.ObjectKey {
		t.Fatalf("expected import file to be stored through object store, keys=%+v session=%+v", objects.keys, session)
	}

	_, err = svc.ConfirmEmployeeImport(ctx, session.ID, domain.EmployeeImportConfirmInput{Mode: "create"})
	if err == nil {
		t.Fatal("expected all-or-nothing import confirmation to reject invalid preview rows")
	}
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Code != "import_validation_failed" || len(appErr.RowErrors) == 0 {
		t.Fatalf("expected import_validation_failed with row errors, got %v", err)
	}
	storedEmployees, err := store.ListEmployees(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, employee := range storedEmployees {
		if employee.CompanyEmail == "partial@example.com" || employee.CompanyEmail == "dup@example.com" {
			t.Fatalf("invalid import should not partially write employees, got %+v", storedEmployees)
		}
	}

	session, err = svc.PreviewEmployeeImport(ctx, domain.EmployeeImportPreviewInput{
		Filename: "employees.csv",
		Content:  "員工編號,姓名,Email,部門,職位,類別,電話,狀態,到職日期\n,Lina Wu,lina@example.com,ou-1,HRBP,約聘,0911000222,在職,2026-06-01\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := svc.ConfirmEmployeeImport(ctx, session.ID, domain.EmployeeImportConfirmInput{Mode: "create"})
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Summary["confirmed"] != 1 {
		t.Fatalf("expected one confirmed import, got %+v", confirmed.Summary)
	}

	var imported domain.Employee
	storedEmployees, err = store.ListEmployees(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range storedEmployees {
		if item.CompanyEmail == "lina@example.com" {
			imported = item
			break
		}
	}
	if imported.ID == "" {
		t.Fatal("imported employee was not written")
	}
	if imported.EmployeeNo != "IKL001" || imported.Category != "contractor" {
		t.Fatalf("expected generated employee number and normalized category, got %+v", imported)
	}

	transitioned, err := svc.TransitionEmployeeStatus(ctx, imported.ID, domain.StatusTransitionInput{
		Status:  "resigned",
		Reason:  "contract ended",
		EndDate: "2026-06-30",
	})
	if err != nil {
		t.Fatal(err)
	}
	if transitioned.EmploymentStatus != "resigned" || transitioned.ResignDate == nil {
		t.Fatalf("expected resigned employee with resign date, got %+v", transitioned)
	}
	if len(transitioned.InternalExperiences) < 2 {
		t.Fatalf("expected status transition history, got %+v", transitioned.InternalExperiences)
	}
}

func TestEmployeeImportConfirmRevalidatesBatchDuplicates(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-hr",
		TenantID: "tenant-1",
		Name:     "HR",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "import", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-hr"}, CreatedAt: now})
	session := domain.EmployeeImportSession{
		ID:        "eimp-test",
		TenantID:  "tenant-1",
		Filename:  "employees.csv",
		Status:    "previewed",
		CreatedAt: now,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
		Rows: []domain.EmployeeImportRow{
			{RowNumber: 2, Valid: true, Employee: domain.CreateEmployeeInput{Name: "One", CompanyEmail: "dup@example.com", AccountID: "acct-dup", Status: "active"}},
			{RowNumber: 3, Valid: true, Employee: domain.CreateEmployeeInput{Name: "Two", CompanyEmail: "dup@example.com", AccountID: "acct-dup", Status: "active"}},
		},
	}
	if err := store.UpsertEmployeeImportSession(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	_, err := service.New(store).ConfirmEmployeeImport(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}, session.ID, domain.EmployeeImportConfirmInput{Mode: "create"})
	if err == nil {
		t.Fatal("expected duplicate batch to fail all-or-nothing import confirmation")
	}
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Code != "import_validation_failed" {
		t.Fatalf("expected import_validation_failed, got %v", err)
	}
	if len(appErr.RowErrors) < 2 {
		t.Fatalf("expected duplicate email and account row errors, got %+v", appErr.RowErrors)
	}
	storedEmployees, err := store.ListEmployees(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(storedEmployees) != 0 {
		t.Fatalf("duplicate batch should not write partial employees, got %+v", storedEmployees)
	}
}

func TestEmployeeReinstatementRequiresTransitionAndKeepsDeletedTerminal(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-hr",
		TenantID: "tenant-1",
		Name:     "HR",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
			{Resource: "hr.employee", Action: "update_status", Scope: "all"},
			{Resource: "hr.employee", Action: "status_transition", Scope: "all"},
			{Resource: "hr.employee", Action: "delete", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-hr"}, CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-linked", TenantID: "tenant-1", EmployeeID: "emp-resign", Status: "active", CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-resign", TenantID: "tenant-1", Name: "Resign", CompanyEmail: "resign@example.com", AccountID: "acct-linked", Status: "active", EmploymentStatus: "active", EmploymentInfo: map[string]any{"resign_reason": "legacy", "resign_date": "2026-06-30"}, CreatedAt: now, UpdatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-delete", TenantID: "tenant-1", Name: "Delete", CompanyEmail: "delete@example.com", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	svc := service.New(store)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", RequestID: "req-reinstate", ApprovalConfirmed: true}
	if _, err := svc.TransitionEmployeeStatus(ctx, "emp-resign", domain.StatusTransitionInput{Status: "resigned", Reason: "left", EndDate: "2026-06-30"}); err != nil {
		t.Fatal(err)
	}
	linked, ok, err := store.GetAccount(context.Background(), "tenant-1", "acct-linked")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || linked.Status != "disabled" {
		t.Fatalf("expected resignation to disable linked account, got %+v", linked)
	}
	if _, err := svc.UpdateEmployeeStatus(ctx, "emp-resign", "active"); err == nil {
		t.Fatal("expected direct status patch to reject resigned employee reactivation")
	}
	_, err = svc.TransitionEmployeeStatus(ctx, "emp-resign", domain.StatusTransitionInput{Status: "active", Reason: "rehired"})
	if appErr, ok := domain.AsAppError(err); !ok || appErr.Code != "validation_failed" || len(appErr.FieldErrors) == 0 {
		t.Fatalf("expected reinstatement start_date validation, got %v", err)
	}
	reinstated, err := svc.TransitionEmployeeStatus(ctx, "emp-resign", domain.StatusTransitionInput{Status: "active", Reason: "rehired", StartDate: "2026-07-01"})
	if err != nil {
		t.Fatal(err)
	}
	if reinstated.EmploymentStatus != "active" || reinstated.ResignDate != nil {
		t.Fatalf("expected active reinstated employee with cleared resign date, got %+v", reinstated)
	}
	if reinstated.EmploymentInfo["transition_type"] != "reinstatement" {
		t.Fatalf("expected reinstatement transition details, got %+v", reinstated.EmploymentInfo)
	}
	if _, ok := reinstated.EmploymentInfo["resign_reason"]; ok {
		t.Fatalf("expected resignation fields to be cleared, got %+v", reinstated.EmploymentInfo)
	}
	if len(reinstated.InternalExperiences) < 2 || reinstated.InternalExperiences[len(reinstated.InternalExperiences)-1].Reason != "rehired" {
		t.Fatalf("expected reinstatement to append internal experience, got %+v", reinstated.InternalExperiences)
	}
	linked, ok, err = store.GetAccount(context.Background(), "tenant-1", "acct-linked")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || linked.Status != "active" {
		t.Fatalf("expected reinstatement to reactivate linked account, got %+v", linked)
	}
	events, err := store.ListAuthzOutboxEvents(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if !hasOutboxEvent(events, "employee.reinstated") {
		t.Fatalf("expected employee.reinstated event, got %+v", events)
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	reinstateLog, ok := findAuditLog(logs, "hr.employee.reinstate")
	if !ok {
		t.Fatalf("expected reinstatement audit log, got %+v", logs)
	}
	if reinstateLog.TraceID != "req-reinstate" || reinstateLog.Details["transition_type"] != "reinstatement" || reinstateLog.Details["data_scope"] == nil {
		t.Fatalf("expected reinstatement audit details with trace and authz context, got %+v", reinstateLog)
	}
	if _, err := svc.DeleteEmployee(ctx, "emp-delete"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.TransitionEmployeeStatus(ctx, "emp-delete", domain.StatusTransitionInput{Status: "active"}); err == nil {
		t.Fatal("expected deleted employee reactivation to be rejected")
	}
}

func TestEmployeeImportXLSXPreservesManagerEmployeeID(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-1", TenantID: "tenant-1", Name: "HQ", Path: []string{"ou-1"}, CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-hr",
		TenantID: "tenant-1",
		Name:     "HR",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "import", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-hr"}, CreatedAt: now})

	svc := service.New(store)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}
	content := minimalEmployeeImportXLSX(t, [][]string{
		{"員工編號", "姓名", "Email", "部門", "職位", "類別", "電話", "狀態", "到職日期", "主管員工ID"},
		{"E2002", "Mina Chen", "mina@example.com", "ou-1", "HRBP", "全職", "0911000444", "在職", "2026-06-01", "E1000"},
	})

	session, err := svc.PreviewEmployeeImport(ctx, domain.EmployeeImportPreviewInput{
		Filename: "employees.xlsx",
		Content:  content,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Rows) != 1 {
		t.Fatalf("expected one preview row, got %d", len(session.Rows))
	}
	row := session.Rows[0]
	if row.Employee.ManagerEmployeeID != "E1000" {
		t.Fatalf("expected manager employee id from xlsx column J, got %+v", row.Employee)
	}
	if got := row.Employee.EmploymentInfo["manager_employee_id"]; got != "E1000" {
		t.Fatalf("expected employment info manager_employee_id from xlsx column J, got %v", got)
	}
	if got := row.Input["主管員工ID"]; got != "E1000" {
		t.Fatalf("expected input manager column from xlsx column J, got %q", got)
	}
}

func TestEmployeeUpdateRespectsDataScope(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-self-update",
		TenantID: "tenant-1",
		Name:     "Self Update",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "update", Scope: "self"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", EmployeeID: "emp-1", Status: "active", DirectPermissionSetIDs: []string{"ps-self-update"}, CreatedAt: now})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", Name: "Employee One", CompanyEmail: "one@example.com", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-2", TenantID: "tenant-1", Name: "Employee Two", CompanyEmail: "two@example.com", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})

	newName := "Changed"
	_, err := service.New(store).UpdateEmployee(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		"emp-2",
		domain.UpdateEmployeeInput{Name: &newName},
	)
	if err == nil {
		t.Fatal("expected scoped update to reject another employee")
	}
}

func TestCreatePermissionSetAssignmentRejectsDanglingReferences(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-read",
		TenantID: "tenant-1",
		Name:     "Read",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-admin", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-target", TenantID: "tenant-1", Status: "active", CreatedAt: now})

	svc := service.New(store)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}
	_, err := svc.CreatePermissionSetAssignment(ctx, domain.CreatePermissionSetAssignmentInput{
		PrincipalType:   "account",
		PrincipalID:     "acct-target",
		PermissionSetID: "ps-read",
		DataScopeID:     "missing-scope",
	})
	if err == nil || !strings.Contains(err.Error(), "data scope") {
		t.Fatalf("expected missing data scope to be rejected, got %v", err)
	}
	assignments, err := store.ListPermissionSetAssignments(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 0 {
		t.Fatalf("expected rejected assignment to avoid writes, got %+v", assignments)
	}

	_, err = svc.CreatePermissionSetAssignment(ctx, domain.CreatePermissionSetAssignmentInput{
		PrincipalType:   "account",
		PrincipalID:     "missing-account",
		PermissionSetID: "ps-read",
	})
	if err == nil || !strings.Contains(err.Error(), "account") {
		t.Fatalf("expected missing account to be rejected, got %v", err)
	}

	_, err = svc.CreatePermissionSetAssignment(ctx, domain.CreatePermissionSetAssignmentInput{
		PrincipalType:   "external",
		PrincipalID:     "acct-target",
		PermissionSetID: "ps-read",
	})
	if err == nil || !strings.Contains(err.Error(), "principal_type") {
		t.Fatalf("expected unsupported principal type to be rejected, got %v", err)
	}
}

func TestMissingAssignmentDataScopeDoesNotBecomeUnscopedGrant(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-read",
		TenantID: "tenant-1",
		Name:     "Read",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-1",
		TenantID:        "tenant-1",
		PrincipalType:   "account",
		PrincipalID:     "acct-1",
		PermissionSetID: "ps-read",
		Effect:          "allow",
		DataScopeID:     "missing-scope",
		CreatedAt:       now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", Name: "Employee One", Status: "active", CreatedAt: now})

	_, err := service.New(store).ListEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err == nil || !strings.Contains(err.Error(), "data scope") {
		t.Fatalf("expected dangling data scope to fail closed, got %v", err)
	}
}

func TestEmployeeOptionsOnlyIncludeVisibleDepartments(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-root", TenantID: "tenant-1", Name: "Root", Path: []string{"ou-root"}, CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-child", TenantID: "tenant-1", Name: "Child", ParentID: "ou-root", Path: []string{"ou-root", "ou-child"}, CreatedAt: now.Add(time.Minute)})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-other", TenantID: "tenant-1", Name: "Other", Path: []string{"ou-other"}, CreatedAt: now.Add(2 * time.Minute)})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-read",
		TenantID: "tenant-1",
		Name:     "Scoped Employee Read",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertDataScope(context.Background(), domain.DataScope{
		ID:        "ds-visible",
		TenantID:  "tenant-1",
		Code:      "direct_reports",
		Name:      "Visible Reports",
		ScopeType: "direct_reports",
		Params:    map[string]any{"employee_ids": []string{"emp-2"}},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-1",
		TenantID:        "tenant-1",
		PrincipalType:   "account",
		PrincipalID:     "acct-1",
		PermissionSetID: "ps-read",
		Effect:          "allow",
		DataScopeID:     "ds-visible",
		CreatedAt:       now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", EmployeeID: "emp-1", Status: "active", CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", Name: "Manager", OrgUnitID: "ou-root", Position: "Manager", Status: "active", CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-2", TenantID: "tenant-1", Name: "Visible Report", OrgUnitID: "ou-child", Position: "Engineer", Status: "active", CreatedAt: now.Add(time.Minute)})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-3", TenantID: "tenant-1", Name: "Hidden", OrgUnitID: "ou-other", Position: "Finance", Status: "active", CreatedAt: now.Add(2 * time.Minute)})

	options, err := service.New(store).EmployeeOptions(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Departments) != 1 || options.Departments[0].ID != "ou-child" {
		t.Fatalf("expected only visible department, got %+v", options.Departments)
	}
	if len(options.Positions) != 1 || options.Positions[0] != "Engineer" {
		t.Fatalf("expected only visible positions, got %+v", options.Positions)
	}
}

func newServiceFixture(permissions []domain.Permission) (*service.Service, domain.RequestContext) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:          "ps-test",
		TenantID:    "tenant-1",
		Name:        "Test Permission Set",
		Permissions: permissions,
		CreatedAt:   now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		DisplayName:            "Test Account",
		EmployeeID:             "emp-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-test"},
		CreatedAt:              now,
	})
	return service.New(store), domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
}

type recordingObjectStore struct {
	keys []string
}

func (s *recordingObjectStore) PutObject(_ context.Context, key string, _ string, _ []byte) error {
	s.keys = append(s.keys, key)
	return nil
}

type recordingAuthzSnapshot struct {
	values map[string]domain.CheckResult
	gets   int
	sets   int
}

func (s *recordingAuthzSnapshot) GetAuthzSnapshot(_ context.Context, key string) (domain.CheckResult, bool, error) {
	s.gets++
	result, ok := s.values[key]
	return result, ok, nil
}

func (s *recordingAuthzSnapshot) SetAuthzSnapshot(_ context.Context, key string, result domain.CheckResult, _ time.Duration) error {
	s.sets++
	s.values[key] = result
	return nil
}

func (s *recordingAuthzSnapshot) InvalidateTenant(_ context.Context, tenantID string) error {
	for key := range s.values {
		if strings.Contains(key, tenantID) {
			delete(s.values, key)
		}
	}
	return nil
}

type fixedRelationshipChecker struct {
	allowed bool
	checks  []authzpkg.RelationshipCheck
}

func (c *fixedRelationshipChecker) CheckRelationship(_ context.Context, check authzpkg.RelationshipCheck) (bool, error) {
	c.checks = append(c.checks, check)
	return c.allowed, nil
}

func hasOutboxEvent(events []domain.AuthzOutboxEvent, eventType string) bool {
	for _, event := range events {
		if event.EventType == eventType {
			return true
		}
	}
	return false
}

func findAuditLog(logs []domain.AuditLog, action string) (domain.AuditLog, bool) {
	for _, log := range logs {
		if log.Action == action {
			return log, true
		}
	}
	return domain.AuditLog{}, false
}

func stringSliceContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func minimalEmployeeImportXLSX(t *testing.T, rows [][]string) string {
	t.Helper()
	var values []string
	for _, row := range rows {
		values = append(values, row...)
	}

	var shared bytes.Buffer
	shared.WriteString(`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	for _, value := range values {
		shared.WriteString("<si><t>")
		if err := xml.EscapeText(&shared, []byte(value)); err != nil {
			t.Fatal(err)
		}
		shared.WriteString("</t></si>")
	}
	shared.WriteString("</sst>")

	var sheet bytes.Buffer
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	index := 0
	for rowIndex, row := range rows {
		fmt.Fprintf(&sheet, `<row r="%d">`, rowIndex+1)
		for colIndex := range row {
			ref := string(rune('A'+colIndex)) + fmt.Sprint(rowIndex+1)
			fmt.Fprintf(&sheet, `<c r="%s" t="s"><v>%d</v></c>`, ref, index)
			index++
		}
		sheet.WriteString("</row>")
	}
	sheet.WriteString("</sheetData></worksheet>")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, data []byte) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	add("xl/sharedStrings.xml", shared.Bytes())
	add("xl/worksheets/sheet1.xml", sheet.Bytes())
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
