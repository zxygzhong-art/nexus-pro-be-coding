package service_test

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/platform/ehrms"
	"nexus-pro-api/internal/repository"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
	agentservice "nexus-pro-api/internal/service/agent"
)

type storeWithoutTenantTransaction struct {
	repository.Store
}

// TestWithinTenantTransactionRequiresTransactionalStore 驗證 within 租戶 transaction requires transactional 儲存層。
func TestWithinTenantTransactionRequiresTransactionalStore(t *testing.T) {
	called := false
	err := repository.WithinTenantTransaction(context.Background(), storeWithoutTenantTransaction{Store: memory.NewStore()}, "tenant-1", func(repository.Store) error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "does not support tenant transactions") {
		t.Fatalf("expected missing transaction support error, got %v", err)
	}
	if called {
		t.Fatal("transaction callback should not run without a tenant transactor")
	}
}

// TestRouteRiskPolicyUsesMatchedHTTPRoute 驗證風險分級只採用實際匹配的 HTTP 路由。
func TestRouteRiskPolicyUsesMatchedHTTPRoute(t *testing.T) {
	svc, ctx := newServiceFixture([]domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
		{Resource: "attendance.clock", Action: "import", Scope: "all"},
	})

	matched, err := svc.Authz().Check(ctx, domain.CheckRequest{
		ApplicationCode: "hr",
		ResourceType:    "employee",
		Action:          "import",
		RouteMethod:     http.MethodPost,
		RoutePath:       "/v1/hr/employees/import/preview",
	})
	if err != nil {
		t.Fatal(err)
	}
	if matched.RiskLevel != string(domain.RiskCritical) {
		t.Fatalf("expected matched import route to retain critical audit risk, got %+v", matched)
	}

	mismatched, err := svc.Authz().Check(ctx, domain.CheckRequest{
		ApplicationCode: "hr",
		ResourceType:    "employee",
		Action:          "import",
		RouteMethod:     http.MethodPost,
		RoutePath:       "/v1/hr/employees",
	})
	if err != nil {
		t.Fatal(err)
	}
	if mismatched.RiskLevel != string(domain.RiskNormal) {
		t.Fatalf("expected unmatched HTTP route to use normal risk, got %+v", mismatched)
	}

	attendanceImport, err := svc.Authz().Check(ctx, domain.CheckRequest{
		ApplicationCode: "attendance",
		ResourceType:    "clock",
		Action:          "import",
		RouteMethod:     http.MethodPost,
		RoutePath:       "/v1/attendance/ehrms/sync",
	})
	if err != nil {
		t.Fatal(err)
	}
	if attendanceImport.RiskLevel != string(domain.RiskCritical) {
		t.Fatalf("expected eHRMS attendance sync route to retain critical audit risk, got %+v", attendanceImport)
	}
}

// TestReadFacadesReturnForbiddenWhenReadPermissionMissing 驗證 facades 退回禁止 when read 權限 missing。
func TestReadFacadesReturnForbiddenWhenReadPermissionMissing(t *testing.T) {
	svc, ctx := newServiceFixture(nil)

	if _, err := svc.HR().QueryEmployees(ctx, domain.EmployeeQuery{}); err == nil {
		t.Fatal("expected employee query to be forbidden without hr.employee.read")
	} else if appErr, ok := domain.AsAppError(err); !ok || appErr.Status != 403 || appErr.ReasonCode != "menu_denied" {
		t.Fatalf("expected employee query menu_denied, got %v", err)
	}

	if _, err := svc.HR().EmployeeStats(ctx, domain.EmployeeQuery{}); err == nil {
		t.Fatal("expected employee stats to be forbidden without hr.employee.read")
	} else if appErr, ok := domain.AsAppError(err); !ok || appErr.Status != 403 || appErr.ReasonCode != "menu_denied" {
		t.Fatalf("expected employee stats menu_denied, got %v", err)
	}

	if _, err := svc.Attendance().ListLeaveBalances(ctx); err == nil {
		t.Fatal("expected leave balances to be forbidden without attendance.leave.read")
	} else if appErr, ok := domain.AsAppError(err); !ok || appErr.Status != 403 || appErr.ReasonCode != "menu_denied" {
		t.Fatalf("expected leave balance menu_denied, got %v", err)
	}
}

// TestEmployeeQuerySkipsSuccessfulReadAudit 驗證成功 query 不寫入操作稽覈，export 仍保留。
func TestEmployeeQuerySkipsSuccessfulReadAudit(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "read", Scope: "all"},
		{Resource: "hr.employee", Action: "export", Scope: "all"},
	})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:               "emp-1",
		TenantID:         "tenant-1",
		EmployeeNo:       "E0001",
		Name:             "Employee One",
		Status:           "active",
		EmploymentStatus: "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	})

	if _, err := svc.HR().QueryEmployees(ctx, domain.EmployeeQuery{}); err != nil {
		t.Fatal(err)
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findAuditLog(logs, "hr.employee.query"); ok {
		t.Fatalf("employee query should not write audit log, got %+v", logs)
	}

	if _, err := svc.HR().ExportEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}); err != nil {
		t.Fatal(err)
	}
	logs, err = store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findAuditLog(logs, "hr.employee.export"); !ok {
		t.Fatalf("employee export should keep audit log, got %+v", logs)
	}
}

// TestCheckRequiresSpecificTarget 驗證 requires specific target。
func TestCheckRequiresSpecificTarget(t *testing.T) {
	svc, ctx := newServiceFixture([]domain.Permission{
		{Resource: "hr.employee", Action: "read", Target: "emp-1"},
	})

	untargeted, err := svc.Authz().Check(ctx, domain.CheckRequest{Resource: "hr.employee", Action: "read"})
	if err != nil {
		t.Fatal(err)
	}
	if untargeted.Allowed {
		t.Fatalf("target-specific permission matched an untargeted request")
	}

	targeted, err := svc.Authz().Check(ctx, domain.CheckRequest{Resource: "hr.employee", Action: "read", Target: "emp-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !targeted.Allowed {
		t.Fatalf("target-specific permission did not match its target: %+v", targeted)
	}

	employeeTargeted, err := svc.Authz().Check(ctx, domain.CheckRequest{Resource: "hr.employee", Action: "read", TargetEmployeeID: "emp-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !employeeTargeted.Allowed {
		t.Fatalf("target-specific permission did not match target_employee_id: %+v", employeeTargeted)
	}

	resourceIDTargeted, err := svc.Authz().Check(ctx, domain.CheckRequest{Resource: "hr.employee", Action: "read", ResourceID: "emp-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !resourceIDTargeted.Allowed {
		t.Fatalf("target-specific permission did not match resource_id: %+v", resourceIDTargeted)
	}
}

// TestCreateAgentRunReturnsNoMatchWithoutBoundKnowledge verifies the fail-closed empty binding behavior.
func TestCreateAgentRunReturnsNoMatchWithoutBoundKnowledge(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-agent",
		TenantID: "tenant-1",
		Name:     "Agent Tool",
		Permissions: []domain.Permission{
			{Resource: "agent.run", Action: "create", Scope: "all"},
			{Resource: "agent.tool", Action: "call", Target: "knowledge.search", Scope: "all"},
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
	model := domain.AgentModel{
		ID: "model-1", TenantID: "tenant-1", Name: "Test Model", ModelName: "gpt-test",
		LiteLLMModel: "openai/gpt-test", Status: domain.AgentModelStatusActive,
		TimeoutSeconds: 30, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.UpsertAgentModel(context.Background(), model); err != nil {
		t.Fatal(err)
	}
	revision := domain.AgentDefinitionVersion{
		ID: "arev-1", TenantID: "tenant-1", AgentID: "agent-1", Version: 1,
		Name: "Test Agent", Category: domain.AgentCategoryWorkflow, Visibility: domain.AgentVisibilityAll,
		SystemPrompt: "Internal system prompt", ModelID: model.ID,
		ModelConfigChecksum: domain.AgentModelSyncConfigHash(model),
		TimeoutSeconds:      30, ConfigSchemaVersion: 1, Checksum: "test-checksum", CreatedAt: now,
	}
	if err := store.InsertAgentDefinitionVersion(context.Background(), revision); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgentDefinition(context.Background(), domain.AgentDefinition{
		ID: "agent-1", TenantID: "tenant-1", DraftRevisionID: revision.ID, PublishedRevisionID: revision.ID,
		Name: revision.Name, Category: revision.Category, ModelID: revision.ModelID, SystemPrompt: revision.SystemPrompt,
		Status: domain.AgentDefinitionStatusPublished, Visibility: revision.Visibility,
		TimeoutSeconds: revision.TimeoutSeconds, Version: 1, PublishedVersion: 1, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	run, err := agentservice.New(service.New(store)).CreateRun(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, domain.CreateAgentRunInput{AgentID: "agent-1", Prompt: "請"})
	if err != nil {
		t.Fatal(err)
	}
	if run.Prompt != "請" {
		t.Fatalf("expected persisted prompt to contain only the user input, got %q", run.Prompt)
	}
	if len(run.References) != 0 {
		t.Fatalf("expected no knowledge references, got %+v", run.References)
	}
	if !strings.Contains(run.Answer, "沒有匹配內容") {
		t.Fatalf("unexpected placeholder answer: %s", run.Answer)
	}
	if len(run.ToolDecisions) != 1 || !run.ToolDecisions[0].Allowed {
		t.Fatalf("expected authorized knowledge tool decision, got %+v", run.ToolDecisions)
	}
}

// TestCreateAgentRunRequiresToolPermission 驗證 agent 執行 requires 工具權限。
func TestCreateAgentRunRequiresToolPermission(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-agent-run",
		TenantID: "tenant-1",
		Name:     "Agent Run",
		Permissions: []domain.Permission{
			{Resource: "agent.run", Action: "create", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	account, _, _ := store.GetAccount(context.Background(), "tenant-1", "acct-1")
	account.DirectPermissionSetIDs = []string{"ps-agent-run"}
	_ = store.UpsertAccount(context.Background(), account)

	_, err := agentservice.New(service.New(store)).CreateRun(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, domain.CreateAgentRunInput{Prompt: "請假"})
	if err == nil {
		t.Fatal("expected agent tool gateway to reject missing tool permission")
	}
}

// TestAgentRunListRespectsOwnerScope 驗證 agent 執行列表 respects owner 範圍。
func TestAgentRunListRespectsOwnerScope(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-agent-own",
		TenantID: "tenant-1",
		Name:     "Own Agent Runs",
		Permissions: []domain.Permission{
			{Resource: "agent.run", Action: "read", Scope: "own"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-agent-all",
		TenantID: "tenant-1",
		Name:     "All Agent Runs",
		Permissions: []domain.Permission{
			{Resource: "agent.run", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-owner", TenantID: "tenant-1", EmployeeID: "emp-owner", Status: "active", DirectPermissionSetIDs: []string{"ps-agent-own"}, CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-other", TenantID: "tenant-1", EmployeeID: "emp-other", Status: "active", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-admin", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-agent-all"}, CreatedAt: now})
	_ = store.UpsertAgentRun(context.Background(), domain.AgentRun{ID: "run-owner", TenantID: "tenant-1", AccountID: "acct-owner", Mode: "qa", Prompt: "own", Status: "completed", CreatedAt: now, UpdatedAt: now})
	_ = store.UpsertAgentRun(context.Background(), domain.AgentRun{ID: "run-other", TenantID: "tenant-1", AccountID: "acct-other", Mode: "qa", Prompt: "other", Status: "completed", CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute)})

	svc := service.New(store)
	ownCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-owner"}
	runs, err := agentservice.New(svc).ListRuns(ownCtx)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != "run-owner" {
		t.Fatalf("expected own-scoped list to return only owner run, got %+v", runs)
	}
	page, err := agentservice.New(svc).ListRunPage(ownCtx, domain.PageRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != "run-owner" {
		t.Fatalf("expected own-scoped page to return only owner run, got %+v", page)
	}

	allRuns, err := agentservice.New(svc).ListRuns(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"})
	if err != nil {
		t.Fatal(err)
	}
	if len(allRuns) != 2 {
		t.Fatalf("expected all-scoped account to see all runs, got %+v", allRuns)
	}
}

// TestAuthzExplicitDenyWins 驗證授權 explicit deny wins。
func TestAuthzExplicitDenyWins(t *testing.T) {
	svc, ctx := newServiceFixture([]domain.Permission{
		{Resource: "hr.employee", Action: "read", Scope: "all"},
		{Resource: "hr.employee", Action: "read", Scope: "all", Effect: "deny"},
	})

	result, err := svc.Authz().Check(ctx, domain.CheckRequest{ApplicationCode: "hr", ResourceType: "employee", Action: "read", ResourceID: "emp-1"})
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

// TestPermissionRelationRequiresOpenFGA 驗證權限 relation requires OpenFGA。
func TestPermissionRelationRequiresOpenFGA(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-read",
		TenantID: "tenant-1",
		Name:     "Relationship Read",
		Permissions: []domain.Permission{
			{Resource: "agent.run", Action: "read", Scope: "all", Relation: "viewer"},
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
	req := domain.CheckRequest{Resource: "agent.run", ResourceID: "run-1", Action: "read"}

	denyChecker := &fixedRelationshipChecker{allowed: false}
	denied, err := service.New(store, service.Options{Relationships: denyChecker}).Authz().Check(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if denied.Allowed || denied.Reason != "relationship denied" {
		t.Fatalf("expected OpenFGA relationship to deny permission, got %+v", denied)
	}

	allowChecker := &fixedRelationshipChecker{allowed: true}
	allowed, err := service.New(store, service.Options{Relationships: allowChecker}).Authz().Check(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !allowed.Allowed {
		t.Fatalf("expected OpenFGA relationship to allow permission, got %+v", allowed)
	}
	if len(allowChecker.checks) != 1 || allowChecker.checks[0].Relation != "viewer" || allowChecker.checks[0].Object != "agent.run:run-1" {
		t.Fatalf("unexpected relationship check: %+v", allowChecker.checks)
	}
}

// TestAssumableRoleSessionPolicyCanOnlyShrink 驗證 assumable 角色 session 政策 can only shrink。
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
			{Resource: "hr.employee", Action: "read", Scope: "all"},
			{Resource: "hr.employee", Action: "export", Scope: "all"},
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
	session, err := svc.IAM().AssumeRole(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, "role-hr", domain.AssumeRoleInput{
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
	read, err := svc.Authz().Check(assumedCtx, domain.CheckRequest{Resource: "hr.employee", Action: "read"})
	if err != nil {
		t.Fatal(err)
	}
	if !read.Allowed || read.AssumedRole == nil || read.AssumedRole.SessionID != sessionID {
		t.Fatalf("expected read allowed through assumed role session, got %+v", read)
	}

	export, err := svc.Authz().Check(assumedCtx, domain.CheckRequest{Resource: "hr.employee", Action: "export"})
	if err != nil {
		t.Fatal(err)
	}
	if export.Allowed {
		t.Fatalf("expected session policy to shrink export permission, got %+v", export)
	}
}

// TestAssumableRoleDurationCannotExceedRoleOrGlobalMaximum 驗證 assumable 角色 duration cannot exceed 角色 or global maximum。
func TestAssumableRoleDurationCannotExceedRoleOrGlobalMaximum(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-assume",
		TenantID: "tenant-1",
		Name:     "Assume Roles",
		Permissions: []domain.Permission{
			{Resource: "iam.assumable_role", Action: "assume", Target: "role-hr", Scope: "all"},
			{Resource: "iam.assumable_role", Action: "create", Scope: "all"},
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
		PermissionBoundary:     map[string]any{"allow": []string{"hr.employee.*"}},
		SessionDurationSeconds: 3600,
		CreatedAt:              now,
	})
	svc := service.New(store)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	if _, err := svc.IAM().AssumeRole(ctx, "role-hr", domain.AssumeRoleInput{Reason: "too long", DurationMinutes: 120}); err == nil {
		t.Fatal("expected assume role request beyond role duration to fail")
	}
	if _, err := svc.IAM().CreateAssumableRole(ctx, domain.CreateAssumableRoleInput{
		Name:                   "Too Long",
		Trusted:                true,
		TrustPolicy:            map[string]any{"accounts": []string{"acct-1"}},
		PermissionBoundary:     map[string]any{"allow": []string{"hr.employee.*"}},
		SessionDurationSeconds: 13 * 60 * 60,
	}); err == nil {
		t.Fatal("expected role session duration above global maximum to fail")
	}
}

// TestAssumableRoleSessionBypassesAuthzSnapshot 驗證 assumable 角色 session bypasses 授權快照。
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
			{Resource: "hr.employee", Action: "read", Scope: "all"},
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
	session, err := svc.IAM().AssumeRole(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, "role-hr", domain.AssumeRoleInput{Reason: "test snapshot bypass"})
	if err != nil {
		t.Fatal(err)
	}
	assumedCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", AssumedRoleSessionID: session.SessionID}
	if result, err := svc.Authz().Check(assumedCtx, domain.CheckRequest{Resource: "hr.employee", Action: "read"}); err != nil || !result.Allowed {
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
	if _, err := svc.Authz().Check(assumedCtx, domain.CheckRequest{Resource: "hr.employee", Action: "read"}); err == nil {
		t.Fatal("expected revoked assumed role session to be rejected instead of served from cache")
	}
	if cache.gets != 1 || cache.sets != 1 {
		t.Fatalf("assumed role checks should bypass snapshot, got gets=%d sets=%d", cache.gets, cache.sets)
	}
}

// TestDirectAssumedRoleIDDoesNotGrantRolePermissions 驗證 direct assumed 角色 ID does not grant 角色權限。
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

	result, err := service.New(store).Authz().Check(
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

// TestTrustedAssumableRoleStillRequiresAssumePermission 驗證 trusted assumable 角色 still requires assume 權限。
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

	_, err := service.New(store).IAM().AssumeRole(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		"role-hr",
		domain.AssumeRoleInput{Reason: "test missing assume permission"},
	)
	if err == nil {
		t.Fatal("expected trusted role assumption to still require iam.assumable_role assume permission")
	}
}

// TestAccountActiveAssumableRoleIDDoesNotGrantRolePermissions 驗證帳號啟用中 assumable 角色 ID does not grant 角色權限。
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

	result, err := service.New(store).Authz().Check(
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

// TestResolveMeRequiresMeReadPermission 驗證 me requires me read 權限。
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

	_, err := service.New(store).Me().Resolve(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err == nil {
		t.Fatal("expected /me resolution to require me read permission")
	}
}

// TestEmployeeReadAppliesAssignmentDataScopeAndFieldPolicy 驗證員工 read applies 指派資料範圍 and 欄位政策。
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

	items, err := service.New(store).HR().ListEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
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

// TestDirectReportsScopeDerivesEmployeeIDs 驗證 direct reports 範圍 derives 員工 IDs。
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

	items, err := service.New(store).HR().ListEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "emp-2" {
		t.Fatalf("expected only direct report, got %+v", items)
	}
}

// TestSameRankDataScopesMergeEmployeeIDs 驗證 same rank 資料範圍 merge 員工 IDs。
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

	items, err := service.New(store).HR().ListEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "emp-2" || items[1].ID != "emp-3" {
		t.Fatalf("expected both same-rank direct reports, got %+v", items)
	}
}

// TestDepartmentSubtreeScopeDerivesOrgUnitIDs 驗證部門 subtree 範圍 derives 組織單位 IDs。
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

	items, err := service.New(store).HR().ListEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "emp-1" || items[1].ID != "emp-2" {
		t.Fatalf("expected root department subtree employees, got %+v", items)
	}
}

// TestOpenFGAScopeCheckCanFilterDepartmentSubtree 驗證 FGA scope check 可覆蓋 SQL 子樹派生。
func TestOpenFGAScopeCheckCanFilterDepartmentSubtree(t *testing.T) {
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
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", Name: "Root Employee", OrgUnitID: "ou-root", Status: "active", CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-2", TenantID: "tenant-1", Name: "Child Employee", OrgUnitID: "ou-child", Status: "active", CreatedAt: now.Add(time.Minute)})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-3", TenantID: "tenant-1", Name: "Other Employee", OrgUnitID: "ou-other", Status: "active", CreatedAt: now.Add(2 * time.Minute)})

	disabledChecker := &mappedRelationshipChecker{allowed: map[string]bool{
		relationshipCheckKey("account:acct-1", "member_recursive", "org_unit:ou-child"): true,
	}}
	disabledItems, err := service.New(store, service.Options{Relationships: disabledChecker}).HR().ListEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(disabledItems) != 2 || len(disabledChecker.checks) != 0 {
		t.Fatalf("expected default SQL scope to ignore FGA checker, items=%+v checks=%+v", disabledItems, disabledChecker.checks)
	}

	enabledChecker := &mappedRelationshipChecker{allowed: map[string]bool{
		relationshipCheckKey("account:acct-1", "member_recursive", "org_unit:ou-child"): true,
	}}
	enabledItems, err := service.New(store, service.Options{Relationships: enabledChecker, OpenFGAScopeChecks: true}).HR().ListEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(enabledItems) != 1 || enabledItems[0].ID != "emp-2" {
		t.Fatalf("expected FGA scope check to allow only child employee, got %+v", enabledItems)
	}
	if !relationshipCheckSeen(enabledChecker.checks, "member_recursive", "org_unit:ou-child") {
		t.Fatalf("expected subtree closure check, got %+v", enabledChecker.checks)
	}
}

// TestAssignedOrgUnitsScopeFiltersExactDepartments 驗證 assigned 組織單位範圍篩選 exact departments。
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

	items, err := service.New(store).HR().ListEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "emp-1" {
		t.Fatalf("expected only exact assigned org employee, got %+v", items)
	}
}

// TestEmployeeQueryKeywordMatchesLinkedAccount 驗證員工查詢 keyword matches linked 帳號。
func TestEmployeeQueryKeywordMatchesLinkedAccount(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-read",
		TenantID: "tenant-1",
		Name:     "Employee Read",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-reader", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-read"}, CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-linked", TenantID: "tenant-1", DisplayName: "Portal Login", Email: "login@example.com", EmployeeID: "emp-1", Status: "active", CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", Name: "Visible Employee", CompanyEmail: "employee@example.com", AccountID: "acct-linked", Status: "active", CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-2", TenantID: "tenant-1", Name: "Other Employee", CompanyEmail: "other@example.com", Status: "active", CreatedAt: now.Add(time.Minute)})

	page, err := service.New(store).HR().QueryEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-reader"}, domain.EmployeeQuery{Keyword: "portal login"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != "emp-1" {
		t.Fatalf("expected keyword to match linked account, got %+v", page)
	}
}

// TestEmployeeStatsRespectDepartmentSubtreeScope 驗證員工 stats respect 部門 subtree 範圍。
func TestEmployeeStatsRespectDepartmentSubtreeScope(t *testing.T) {
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
	_ = store.UpsertDataScope(context.Background(), domain.DataScope{ID: "ds-subtree", TenantID: "tenant-1", Code: "department_subtree", Name: "Department Subtree", ScopeType: "department_subtree", CreatedAt: now})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-1",
		TenantID:        "tenant-1",
		PrincipalType:   "account",
		PrincipalID:     "acct-1",
		PermissionSetID: "ps-read",
		Effect:          "allow",
		DataScopeID:     "ds-subtree",
		CreatedAt:       now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", EmployeeID: "emp-manager", Status: "active", CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-manager", TenantID: "tenant-1", Name: "Manager", OrgUnitID: "ou-root", Status: "active", EmploymentStatus: "active", CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-child", TenantID: "tenant-1", Name: "Child", OrgUnitID: "ou-child", Status: "onboarding", EmploymentStatus: "onboarding", CreatedAt: now.Add(time.Minute)})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-hidden", TenantID: "tenant-1", Name: "Hidden", OrgUnitID: "ou-other", Status: "resigned", EmploymentStatus: "resigned", CreatedAt: now.Add(2 * time.Minute)})

	stats, err := service.New(store).HR().EmployeeStats(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, domain.EmployeeQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 2 || stats.Active != 1 || stats.Onboarding != 1 || stats.Resigned != 0 {
		t.Fatalf("expected stats to exclude hidden department employees, got %+v", stats)
	}
}

// TestListOrgUnitsRespectsDepartmentSubtreeScope 驗證組織單位 respects 部門 subtree 範圍。
func TestListOrgUnitsRespectsDepartmentSubtreeScope(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-root", TenantID: "tenant-1", Name: "Root", Path: []string{"ou-root"}, CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-child", TenantID: "tenant-1", Name: "Child", ParentID: "ou-root", Path: []string{"ou-root", "ou-child"}, CreatedAt: now.Add(time.Minute)})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-other", TenantID: "tenant-1", Name: "Other", Path: []string{"ou-other"}, CreatedAt: now.Add(2 * time.Minute)})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-org-read",
		TenantID: "tenant-1",
		Name:     "Scoped Org Read",
		Permissions: []domain.Permission{
			{Resource: "hr.org_unit", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertDataScope(context.Background(), domain.DataScope{ID: "ds-subtree", TenantID: "tenant-1", Code: "department_subtree", Name: "Department Subtree", ScopeType: "department_subtree", CreatedAt: now})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-org",
		TenantID:        "tenant-1",
		PrincipalType:   "account",
		PrincipalID:     "acct-1",
		PermissionSetID: "ps-org-read",
		Effect:          "allow",
		DataScopeID:     "ds-subtree",
		CreatedAt:       now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", EmployeeID: "emp-manager", Status: "active", CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-manager", TenantID: "tenant-1", Name: "Manager", OrgUnitID: "ou-root", Status: "active", EmploymentStatus: "active", CreatedAt: now})

	page, err := service.New(store).HR().ListOrgUnitPage(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, domain.OrgUnitQuery{Page: 1, PageSize: 10, Sort: "created_at_asc"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 2 || page.Items[0].ID != "ou-root" || page.Items[1].ID != "ou-child" {
		t.Fatalf("expected org units to be scoped to manager subtree, got %+v", page)
	}
}

// TestEmployeeFieldPolicyHidesDenyFieldsAndBlocksWrites 驗證員工欄位政策 hides deny 欄位 and blocks writes。
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
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", RequestID: "req-field-policy", TraceID: "trace-field-policy"}

	items, err := svc.HR().ListEmployees(ctx)
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
	_, err = svc.HR().UpdateEmployee(ctx, "emp-1", domain.UpdateEmployeeInput{
		Phone:     &nextPhone,
		BasicInfo: map[string]any{"national_id": "B123456789"},
	})
	if err == nil || !strings.Contains(err.Error(), "employee field policy denied update") {
		t.Fatalf("expected field policy update error, got %v", err)
	}
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.ReasonCode != "field_denied" || len(appErr.FieldErrors) == 0 || appErr.FieldErrors[0].Code != "field_denied" {
		t.Fatalf("expected field_denied error code, got %v", err)
	}

	exported, err := svc.HR().ExportEmployees(ctx)
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
	if exportLog.TraceID != "trace-field-policy" || exportLog.Details["trace_id"] != "trace-field-policy" || exportLog.Details["request_id"] != "req-field-policy" || exportLog.Details["row_count"] != 1 {
		t.Fatalf("expected export audit trace and row count, got %+v", exportLog)
	}
	restricted, ok := exportLog.Details["restricted_fields"].(map[string][]string)
	if !ok || !stringSliceContains(restricted["hide"], "phone") || !stringSliceContains(restricted["deny"], "national_id") {
		t.Fatalf("expected export audit restricted fields, got %+v", exportLog.Details["restricted_fields"])
	}
}

// TestEmployeeExportAppliesPermissionScopedFieldPoliciesToJSONAndCSV 驗證員工 export applies 權限 scoped 欄位政策 to JSON and CSV。
func TestEmployeeExportAppliesPermissionScopedFieldPoliciesToJSONAndCSV(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-hr",
		TenantID: "tenant-1",
		Name:     "Employee Read Export",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
			{Resource: "hr.employee", Action: "export", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionCatalogItem(context.Background(), domain.PermissionCatalogItem{
		ID: "perm-export", TenantID: "tenant-1", Application: "hr", Resource: "hr.employee", Action: "export",
		PermissionType: domain.PermissionTypeAPI, Name: "Employee export", CreatedAt: now,
	})
	_ = store.UpsertFieldPolicy(context.Background(), domain.FieldPolicy{
		ID:              "fp-export-phone",
		TenantID:        "tenant-1",
		ApplicationCode: "hr",
		ResourceType:    "employee",
		FieldName:       "phone",
		Effect:          "hide",
		PermissionID:    "perm-export",
		CreatedAt:       now,
	})
	_ = store.UpsertFieldPolicy(context.Background(), domain.FieldPolicy{
		ID:              "fp-mask-company-email",
		TenantID:        "tenant-1",
		ApplicationCode: "hr",
		ResourceType:    "employee",
		FieldName:       "company_email",
		Effect:          "mask",
		CreatedAt:       now,
	})
	allowEmployeeSensitiveFieldsForPermission(t, store, now, "hr.employee.read")
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-hr"}, CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", EmployeeNo: "E0001", Name: "Employee One", CompanyEmail: "employee.one@example.com", Phone: "0912345678", Status: "active", CreatedAt: now})
	svc := service.New(store)

	page, err := svc.HR().QueryEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, domain.EmployeeQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Phone != "0912345678" || page.Items[0].CompanyEmail == "employee.one@example.com" {
		t.Fatalf("expected read permission to keep phone visible and company email masked, got %+v", page.Items)
	}

	exportCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	exported, err := svc.HR().ExportEmployees(exportCtx)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported) != 1 || exported[0].Phone != "" || exported[0].CompanyEmail != "employee.one@example.com" {
		t.Fatalf("expected JSON export to hide phone and keep company email unmasked, got %+v", exported)
	}
	raw, _, err := svc.HR().ExportEmployeesCSV(exportCtx, domain.EmployeeQuery{})
	if err != nil {
		t.Fatal(err)
	}
	csvBody := string(raw)
	if strings.Contains(csvBody, "電話") || strings.Contains(csvBody, "0912345678") || !strings.Contains(csvBody, "employee.one@example.com") || strings.Contains(csvBody, "***") {
		t.Fatalf("expected CSV export to omit hidden phone and keep company email unmasked, got %q", csvBody)
	}
}

// TestEmployeeExportCSVNeutralizesSpreadsheetFormulas 驗證員工 export CSV neutralizes spreadsheet formulas。
func TestEmployeeExportCSVNeutralizesSpreadsheetFormulas(t *testing.T) {
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
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:           "emp-1",
		TenantID:     "tenant-1",
		EmployeeNo:   "=HYPERLINK(http://evil.test)",
		Name:         "+SUM(1,1)",
		CompanyEmail: "@evil.test",
		Position:     "-cmd|' /C calc'!A0",
		Status:       "active",
		CreatedAt:    now,
	})

	raw, _, err := service.New(store).HR().ExportEmployeesCSV(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, domain.EmployeeQuery{})
	if err != nil {
		t.Fatal(err)
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(raw), "\ufeff")))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("expected header and one employee row, got %+v", records)
	}
	row := records[1]
	for _, index := range []int{0, 1, 2, 4} {
		if !strings.HasPrefix(row[index], "'") {
			t.Fatalf("expected formula-like cell %d to be neutralized, row=%+v", index, row)
		}
	}
}

// TestEmployeeExportUsesGrantedPermissionAndAuditsRisk 驗證匯出權限可直接執行且仍保留風險審計。
func TestEmployeeExportUsesGrantedPermissionAndAuditsRisk(t *testing.T) {
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

	items, err := svc.HR().ExportEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "emp-1" {
		t.Fatalf("expected granted export result, got %+v", items)
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Details["risk_level"] != string(domain.RiskCritical) || logs[0].Result != "allowed" {
		t.Fatalf("expected allowed critical-risk audit log, got %+v", logs)
	}
}

// TestEmployeeExportRejectsOversizedSyncResult 驗證員工 export rejects oversized sync 結果。
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

	_, err := service.New(store).HR().ExportEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if appErr, ok := domain.AsAppError(err); !ok || appErr.Code != "conflict" {
		t.Fatalf("expected oversized export conflict, got %v", err)
	}
}

// TestSelfScopedEmployeeReadOnlyReturnsCurrentEmployee 驗證 self scoped 員工 read only returns 目前員工。
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

	items, err := service.New(store).HR().ListEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "emp-1" {
		t.Fatalf("expected only current employee, got %+v", items)
	}
}

// TestSelfScopedLeaveCreateCannotTargetAnotherEmployee 驗證 self scoped 請假 create cannot target another 員工。
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

	_, err := service.New(store).Attendance().CreateLeaveRequest(
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

// TestSelfScopedLeaveReadOnlyReturnsCurrentEmployeeItems 驗證 self scoped 請假 read only returns 目前員工項目。
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
	_ = store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{ID: "lb-1", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", RemainingMinutes: 8 * 60, UpdatedAt: now})
	_ = store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{ID: "lb-2", TenantID: "tenant-1", EmployeeID: "emp-2", LeaveType: "annual", RemainingMinutes: 8 * 60, UpdatedAt: now})
	_ = store.UpsertLeaveRequest(context.Background(), domain.LeaveRequest{ID: "lr-1", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", RequestedMinutes: 8 * 60, Status: "pending", CreatedAt: now})
	_ = store.UpsertLeaveRequest(context.Background(), domain.LeaveRequest{ID: "lr-2", TenantID: "tenant-1", EmployeeID: "emp-2", LeaveType: "annual", RequestedMinutes: 8 * 60, Status: "pending", CreatedAt: now.Add(time.Minute)})

	svc := service.New(store)
	balances, err := svc.Attendance().ListLeaveBalances(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(balances) != 1 || balances[0].EmployeeID != "emp-1" {
		t.Fatalf("expected only current employee balance, got %+v", balances)
	}
	requests, err := svc.Attendance().ListLeaveRequests(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 || requests[0].EmployeeID != "emp-1" {
		t.Fatalf("expected only current employee request, got %+v", requests)
	}
	otherBalancePage, err := svc.Attendance().ListLeaveBalancePageByQuery(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		domain.LeaveBalanceQuery{EmployeeIDs: []string{"emp-2"}},
		domain.PageRequest{Page: 1, PageSize: 20},
	)
	if err != nil {
		t.Fatal(err)
	}
	if otherBalancePage.Total != 0 {
		t.Fatalf("expected explicit employee filter to stay inside self scope, got %+v", otherBalancePage)
	}
	otherRequestPage, err := svc.Attendance().ListLeaveRequestPageByQuery(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		domain.LeaveRequestQuery{EmployeeIDs: []string{"emp-2"}},
		domain.PageRequest{Page: 1, PageSize: 20},
	)
	if err != nil {
		t.Fatal(err)
	}
	if otherRequestPage.Total != 0 {
		t.Fatalf("expected explicit employee filter to stay inside self scope, got %+v", otherRequestPage)
	}
}

// TestAttendanceLeavePagesFilterEmployeeBeforePagination verifies employee detail queries are complete and isolated.
func TestAttendanceLeavePagesFilterEmployeeBeforePagination(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{{Resource: "attendance.leave", Action: "read", Scope: "all"}})
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	for _, employeeID := range []string{"emp-1", "emp-2"} {
		_ = store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{
			ID: employeeID + "-annual", TenantID: "tenant-1", EmployeeID: employeeID, LeaveType: "annual", RemainingMinutes: 8 * 60, UpdatedAt: now,
		})
		_ = store.UpsertLeaveRequest(context.Background(), domain.LeaveRequest{
			ID: employeeID + "-request", TenantID: "tenant-1", EmployeeID: employeeID, LeaveType: "annual", RequestedMinutes: 8 * 60, Status: "approved", CreatedAt: now,
		})
	}

	balancePage, err := svc.Attendance().ListLeaveBalancePageByQuery(
		ctx,
		domain.LeaveBalanceQuery{EmployeeIDs: []string{"emp-2"}},
		domain.PageRequest{Page: 1, PageSize: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	if balancePage.Total != 1 || len(balancePage.Items) != 1 || balancePage.Items[0].EmployeeID != "emp-2" {
		t.Fatalf("expected one filtered balance before pagination, got %+v", balancePage)
	}

	requestPage, err := svc.Attendance().ListLeaveRequestPageByQuery(
		ctx,
		domain.LeaveRequestQuery{EmployeeIDs: []string{"emp-2"}},
		domain.PageRequest{Page: 1, PageSize: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	if requestPage.Total != 1 || len(requestPage.Items) != 1 || requestPage.Items[0].EmployeeID != "emp-2" {
		t.Fatalf("expected one filtered request before pagination, got %+v", requestPage)
	}
}

// TestAttendanceClockRecordsAcceptedRejectedAndRepeated 驗證有效、拒絕及重複打卡都保留逐筆記錄。
func TestAttendanceClockRecordsAcceptedRejectedAndDuplicate(t *testing.T) {
	store, svc, employeeCtx, _, setNow := newAttendanceFixture(t)

	accepted, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_in",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
		LocationSource: "gps",
		DeviceID:       "phone-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.RecordStatus != "accepted" || accepted.RejectionReason != "" {
		t.Fatalf("expected accepted clock-in, got %+v", accepted)
	}
	if accepted.Latitude != 0 || accepted.Longitude != 0 || accepted.DeviceID != "" || accepted.DeviceInfo != nil {
		t.Fatalf("expected create response to hide clock location evidence, got %+v", accepted)
	}
	stored, ok, err := store.GetEarliestAcceptedAttendanceClockIn(context.Background(), "tenant-1", "emp-1", accepted.WorkDate)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected raw accepted clock-in evidence to be stored")
	}
	if got := stored.DeviceInfo["location_source"]; got != "gps" {
		t.Fatalf("expected stored location_source evidence, got %v", got)
	}

	setNow(attendanceFixtureClockOutTime())
	outside, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction: "clock_out",
		Latitude:  25.100000,
		Longitude: 121.700000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outside.RecordStatus != "rejected" || outside.RejectionReason != "outside_geofence" {
		t.Fatalf("expected outside geofence rejected attempt, got %+v", outside)
	}

	setNow(attendanceFixtureClockInTime().Add(30 * time.Minute))
	repeated, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction: "clock_in",
		Latitude:  25.033964,
		Longitude: 121.564468,
	})
	if err != nil || repeated.RecordStatus != "rejected" || repeated.RejectionReason != "duplicate" {
		t.Fatalf("expected repeated clock-in to be stored as rejected duplicate, got record=%+v err=%v", repeated, err)
	}

	setNow(attendanceFixtureClockOutTime())
	status, err := svc.Attendance().AttendanceClockStatus(employeeCtx)
	if err != nil {
		t.Fatal(err)
	}
	if status.NextAction != "clock_out" || status.ClockIn == nil || status.ClockOut != nil || status.PunchCount != 1 || status.CanClockIn {
		t.Fatalf("unexpected clock status: %+v", status)
	}
}

// TestAttendanceClockStatusProjectsAuthoritativeWorksites verifies only active geofences reach self-service clients.
func TestAttendanceClockStatusProjectsAuthoritativeWorksites(t *testing.T) {
	store, svc, employeeCtx, _, _ := newAttendanceFixture(t)
	now := attendanceFixtureClockInTime()
	for _, worksite := range []domain.AttendanceWorksite{
		{ID: "aws-2", TenantID: "tenant-1", Name: "Branch", Address: "No. 2, Branch Road", Latitude: 25.04, Longitude: 121.57, RadiusMeters: 350, Status: "active", CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute)},
		{ID: "aws-inactive", TenantID: "tenant-1", Name: "Closed", Address: "No. 3, Closed Road", Latitude: 25.05, Longitude: 121.58, RadiusMeters: 500, Status: "inactive", CreatedAt: now.Add(2 * time.Minute), UpdatedAt: now.Add(2 * time.Minute)},
	} {
		if err := store.UpsertAttendanceWorksite(context.Background(), worksite); err != nil {
			t.Fatal(err)
		}
	}

	status, err := svc.Attendance().AttendanceClockStatus(employeeCtx)
	if err != nil {
		t.Fatal(err)
	}
	if !status.RequireWorksite || status.Worksite == nil || status.Worksite.ID != "aws-2" {
		t.Fatalf("expected the current worksite policy and primary active worksite, got %+v", status)
	}
	if len(status.Worksites) != 2 || status.Worksites[0].ID != "aws-2" || status.Worksites[1].ID != "aws-1" {
		t.Fatalf("expected every active worksite and no inactive entries, got %+v", status.Worksites)
	}
	record, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction: "clock_in", Latitude: 25.04, Longitude: 121.57, AccuracyMeters: 10,
	})
	if err != nil || record.WorksiteID != "aws-2" || record.RecordStatus != "accepted" {
		t.Fatalf("expected multi-worksite submission to select the same nearest active geofence, record=%+v err=%v", record, err)
	}
	for _, worksite := range []domain.AttendanceWorksite{
		{ID: "aws-1", TenantID: "tenant-1", Name: "HQ", Latitude: 25.033964, Longitude: 121.564468, RadiusMeters: 200, Status: "inactive", CreatedAt: now, UpdatedAt: now.Add(3 * time.Minute)},
		{ID: "aws-2", TenantID: "tenant-1", Name: "Branch", Latitude: 25.04, Longitude: 121.57, RadiusMeters: 350, Status: "inactive", CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(3 * time.Minute)},
	} {
		if err := store.UpsertAttendanceWorksite(context.Background(), worksite); err != nil {
			t.Fatal(err)
		}
	}
	status, err = svc.Attendance().AttendanceClockStatus(employeeCtx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Worksite != nil || len(status.Worksites) != 0 || !status.RequireWorksite {
		t.Fatalf("expected required policy with an explicit empty active worksite set, got %+v", status)
	}
}

// TestAttendanceClockClientEventIDIsIdempotent 驗證網路重試不會重複建立原始打卡事件。
func TestAttendanceClockClientEventIDIsIdempotent(t *testing.T) {
	store, svc, employeeCtx, _, _ := newAttendanceFixture(t)
	input := domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_in",
		ClientEventID:  "punch-event-1",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	}
	first, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, input)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, input)
	if err != nil {
		t.Fatal(err)
	}
	if retried.ID != first.ID {
		t.Fatalf("idempotent retry created a different record: first=%s retry=%s", first.ID, retried.ID)
	}
	records, err := store.ListAttendanceClockRecords(context.Background(), "tenant-1", domain.AttendanceClockRecordQuery{EmployeeID: "emp-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("idempotent retry stored %d records, want 1", len(records))
	}
}

// TestAttendanceClockOneHourPlusApprovedLeaveCompletesDay 驗證工作一小時後請假可正常打下班卡。
func TestAttendanceClockOneHourPlusApprovedLeaveCompletesDay(t *testing.T) {
	store, svc, employeeCtx, _, setNow := newAttendanceFixture(t)
	clockInAt := attendanceFixtureClockInTime()
	if _, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_in",
		ClientEventID:  "one-hour-in",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	}); err != nil {
		t.Fatal(err)
	}
	request := domain.LeaveRequest{
		ID:               "leave-rest-of-day",
		TenantID:         "tenant-1",
		EmployeeID:       "emp-1",
		LeaveType:        "annual",
		LeaveTypeID:      "leave-type-annual",
		StartAt:          clockInAt.Add(time.Hour),
		EndAt:            clockInAt.Add(9 * time.Hour),
		RequestedMinutes: 7 * 60,
		Status:           "approved",
		CreatedAt:        clockInAt,
	}
	if err := store.UpsertLeaveRequest(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertLeaveRecord(context.Background(), domain.LeaveRecord{
		ID:                   "case-rest-of-day",
		TenantID:             "tenant-1",
		EmployeeID:           "emp-1",
		LeaveTypeID:          request.LeaveTypeID,
		BalanceID:            "balance-rest-of-day",
		EntitlementYear:      request.StartAt.Year(),
		Source:               "nexus",
		EventDate:            clockInAt,
		StartAt:              request.StartAt,
		EndAt:                request.EndAt,
		NetMinutes:           request.RequestedMinutes,
		Status:               "active",
		ReconciliationStatus: "not_required",
		UpdatedAt:            clockInAt,
	}); err != nil {
		t.Fatal(err)
	}
	setNow(clockInAt.Add(time.Hour))
	clockOut, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_out",
		ClientEventID:  "one-hour-out",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	})
	if err != nil || clockOut.RecordStatus != "accepted" {
		t.Fatalf("one-hour clock-out should be accepted, record=%+v err=%v", clockOut, err)
	}
	status, err := svc.Attendance().AttendanceClockStatus(employeeCtx)
	if err != nil {
		t.Fatal(err)
	}
	if status.DayStatus != "complete" || status.WorkedMinutes != 60 || status.WorkedMinutes+status.ApprovedLeaveMinutes != status.RequiredMinutes || status.ClockOut == nil {
		t.Fatalf("approved leave should complete the day projection, got %+v", status)
	}
}

// TestAttendanceClockKeepsEarlyAndFinalClockOut 驗證彈性工時不足由日投影標記，後續下班卡可更新尾卡。
func TestAttendanceClockRejectsInsufficientWorkHoursAndAllowsRetry(t *testing.T) {
	store, svc, employeeCtx, _, setNow := newAttendanceFixture(t)
	clockInAt := attendanceFixtureClockInTime()

	clockIn, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_in",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	})
	if err != nil {
		t.Fatal(err)
	}

	setNow(clockInAt.Add(11 * time.Second))
	tooShort, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_out",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tooShort.RecordStatus != "accepted" || tooShort.RejectionReason != "" {
		t.Fatalf("expected early clock-out to remain an accepted raw punch, got %+v", tooShort)
	}
	status, err := svc.Attendance().AttendanceClockStatus(employeeCtx)
	if err != nil {
		t.Fatal(err)
	}
	if status.NextAction != "complete" || status.ClockIn == nil || status.ClockOut == nil || status.DayStatus != "abnormal" || status.CanClockIn || status.CanClockOut {
		t.Fatalf("expected early clock-out to produce an abnormal day projection, got %+v", status)
	}
	records, err := store.ListAttendanceClockRecords(context.Background(), "tenant-1", domain.AttendanceClockRecordQuery{EmployeeID: "emp-1", FromDate: clockIn.WorkDate, ToDate: clockIn.WorkDate})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("expected accepted clock-in and abnormal audit attempt, got %+v", records)
	}

	setNow(clockInAt.Add(9 * time.Hour))
	clockOut, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_out",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	if clockOut.RecordStatus != "accepted" {
		t.Fatalf("expected retry to create accepted clock-out, got %+v", clockOut)
	}
	status, err = svc.Attendance().AttendanceClockStatus(employeeCtx)
	if err != nil {
		t.Fatal(err)
	}
	if status.NextAction != "complete" || status.ClockOut == nil || status.DayStatus != "complete" || status.CanClockIn || status.CanClockOut {
		t.Fatalf("expected completed status after valid retry, got %+v", status)
	}

	summary, err := svc.Attendance().AttendanceMonthlySummary(employeeCtx, "2026-06")
	if err != nil {
		t.Fatal(err)
	}
	if summary.WorkedMinutes != 480 {
		t.Fatalf("expected actual 8 monthly clock hours (480 minutes), got %+v", summary)
	}
}

// TestAttendanceMonthlySummaryUsesDailyProjection verifies selected-month totals and record counts.
func TestAttendanceMonthlySummaryUsesDailyProjection(t *testing.T) {
	_, svc, employeeCtx, _, setNow := newAttendanceFixture(t)
	clockInAt := attendanceFixtureClockInTime()

	if _, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_in",
		ClientEventID:  "monthly-summary-in",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	}); err != nil {
		t.Fatal(err)
	}
	setNow(clockInAt.Add(9 * time.Hour))
	if _, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_out",
		ClientEventID:  "monthly-summary-out",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	}); err != nil {
		t.Fatal(err)
	}

	summary, err := svc.Attendance().AttendanceMonthlySummary(employeeCtx, "2026-06")
	if err != nil {
		t.Fatal(err)
	}
	if summary.EmployeeID != "emp-1" || summary.Month != "2026-06" || summary.AttendanceDays != 1 || summary.WorkedMinutes != 480 || summary.RecordCount != 2 || summary.AbnormalDays != 0 {
		t.Fatalf("unexpected monthly attendance summary: %+v", summary)
	}
	if len(summary.Days) != 1 || summary.Days[0].WorkDate != "2026-06-10" || summary.Days[0].WorkedMinutes != 480 || summary.Days[0].RecordCount != 2 || summary.Days[0].DayStatus != "complete" {
		t.Fatalf("expected one projected calendar day, got %+v", summary.Days)
	}

	empty, err := svc.Attendance().AttendanceMonthlySummary(employeeCtx, "2026-05")
	if err != nil {
		t.Fatal(err)
	}
	if empty.AttendanceDays != 0 || empty.WorkedMinutes != 0 || empty.RecordCount != 0 || len(empty.Days) != 0 {
		t.Fatalf("expected selected-month filtering, got %+v", empty)
	}
	if _, err := svc.Attendance().AttendanceMonthlySummary(employeeCtx, "2026/06"); err == nil {
		t.Fatal("expected invalid month to be rejected")
	}
}

// TestAttendanceMonthlySummaryMarksPastOpenDayAbnormal verifies calendar totals use the elapsed-day boundary.
func TestAttendanceMonthlySummaryMarksPastOpenDayAbnormal(t *testing.T) {
	_, svc, employeeCtx, _, setNow := newAttendanceFixture(t)
	clockInAt := attendanceFixtureClockInTime()

	if _, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_in",
		ClientEventID:  "monthly-summary-expired-in",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	}); err != nil {
		t.Fatal(err)
	}
	setNow(clockInAt.Add(25 * time.Hour))

	summary, err := svc.Attendance().AttendanceMonthlySummary(employeeCtx, "2026-06")
	if err != nil {
		t.Fatal(err)
	}
	if summary.AttendanceDays != 1 || summary.WorkedMinutes != 0 || summary.RecordCount != 1 || summary.AbnormalDays != 1 {
		t.Fatalf("unexpected expired-open-day totals: %+v", summary)
	}
	if len(summary.Days) != 1 || summary.Days[0].DayStatus != "abnormal" || len(summary.Days[0].AnomalyReasons) != 1 || summary.Days[0].AnomalyReasons[0] != "missing_clock_out" {
		t.Fatalf("expected one abnormal calendar day with missing_clock_out, got %+v", summary.Days)
	}
}

// TestAttendanceMonthlySummaryIncludesLeaveOnlyCase verifies the report date
// set is the union of punches and reconciled leave facts, and that the shared
// projection is persisted for downstream workspace reads.
func TestAttendanceMonthlySummaryIncludesLeaveOnlyCase(t *testing.T) {
	store, svc, employeeCtx, _, _ := newAttendanceFixture(t)
	location := time.FixedZone("UTC+8", 8*60*60)
	start := time.Date(2026, 6, 11, 9, 0, 0, 0, location)
	end := time.Date(2026, 6, 11, 18, 0, 0, 0, location)
	if err := store.UpsertLeaveRecord(context.Background(), domain.LeaveRecord{
		ID: "case-leave-only", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveTypeID: "leave-type-annual",
		BalanceID: "balance-leave-only", EntitlementYear: 2026, Source: "nexus", EventDate: start,
		StartAt: start, EndAt: end, NetMinutes: 480, Status: "active", ReconciliationStatus: "not_required", UpdatedAt: start,
	}); err != nil {
		t.Fatal(err)
	}

	summary, err := svc.Attendance().AttendanceMonthlySummary(employeeCtx, "2026-06")
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Days) != 1 || summary.Days[0].WorkDate != "2026-06-11" || summary.Days[0].RecordCount != 0 || summary.Days[0].DayStatus != "complete" {
		t.Fatalf("expected one leave-only complete day, got %+v", summary.Days)
	}
	projection, ok, err := store.GetAttendanceDayProjection(context.Background(), "tenant-1", "emp-1", "2026-06-11")
	if err != nil || !ok {
		t.Fatalf("persisted projection missing: ok=%v err=%v", ok, err)
	}
	// The canonical case carries 480 source minutes, but the projection credits
	// only the 420 scheduled minutes after clipping the 09:00-18:00 envelope to
	// the effective 09:00-17:00 policy and removing the one-hour break.
	if projection.ApprovedLeaveMinutes != 420 || projection.PunchCount != 0 || projection.InputFingerprint == "" {
		t.Fatalf("unexpected persisted leave-only projection: %+v", projection)
	}
}

// TestAttendanceMonthlySummaryUsesPolicyEffectiveOnWorkDate protects historical
// projections from later policy publications.
func TestAttendanceMonthlySummaryUsesPolicyEffectiveOnWorkDate(t *testing.T) {
	store, svc, employeeCtx, _, _ := newAttendanceFixture(t)
	local := time.FixedZone("UTC+8", 8*60*60)
	versionOneAt := time.Date(2026, 6, 1, 0, 0, 0, 0, local)
	versionTwoAt := time.Date(2026, 7, 1, 0, 0, 0, 0, local)
	workTime := domain.AttendancePolicyWorkTime{ClockMode: "fixed", StandardStart: "09:00", StandardEnd: "17:00", BreakStart: "12:00", BreakEnd: "13:00"}
	if err := store.InsertAttendancePolicyVersion(context.Background(), domain.AttendancePolicy{TenantID: "tenant-1", Version: 1, EffectiveFrom: &versionOneAt, WorkTime: workTime, PublishedAt: versionOneAt}); err != nil {
		t.Fatal(err)
	}
	workTime.StandardEnd = "18:00"
	if err := store.InsertAttendancePolicyVersion(context.Background(), domain.AttendancePolicy{TenantID: "tenant-1", Version: 2, EffectiveFrom: &versionTwoAt, WorkTime: workTime, PublishedAt: versionTwoAt}); err != nil {
		t.Fatal(err)
	}
	for _, record := range []domain.AttendanceClockRecord{
		{ID: "historical-in", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-10", Direction: "clock_in", ClockedAt: time.Date(2026, 6, 10, 9, 0, 0, 0, local), RecordStatus: "accepted", Source: "geofence", CreatedAt: versionOneAt},
		{ID: "historical-out", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-10", Direction: "clock_out", ClockedAt: time.Date(2026, 6, 10, 17, 0, 0, 0, local), RecordStatus: "accepted", Source: "geofence", CreatedAt: versionOneAt},
	} {
		if err := store.UpsertAttendanceClockRecord(context.Background(), record); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Attendance().AttendanceMonthlySummary(employeeCtx, "2026-06"); err != nil {
		t.Fatal(err)
	}
	projection, ok, err := store.GetAttendanceDayProjection(context.Background(), "tenant-1", "emp-1", "2026-06-10")
	if err != nil || !ok {
		t.Fatalf("historical projection missing: ok=%v err=%v", ok, err)
	}
	if projection.PolicyVersion != 1 || projection.RequiredMinutes != 420 || projection.DayStatus != "complete" {
		t.Fatalf("historical projection used the wrong policy: %+v", projection)
	}
}

// TestAttendanceClockUsesElapsedHoursInsteadOfFixedClockOutTime 驗證彈性打卡只依實際工時判定。
func TestAttendanceClockUsesElapsedHoursInsteadOfFixedClockOutTime(t *testing.T) {
	_, svc, employeeCtx, _, setNow := newAttendanceFixture(t)
	clockInAt := attendanceFixtureClockInTime()

	if _, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_in",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	}); err != nil {
		t.Fatal(err)
	}

	setNow(clockInAt.Add(3*time.Hour + 44*time.Minute))
	early, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_out",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	if early.RecordStatus != "accepted" || early.RejectionReason != "" {
		t.Fatalf("expected short flexible clock-out to remain accepted raw evidence, got %+v", early)
	}
	status, err := svc.Attendance().AttendanceClockStatus(employeeCtx)
	if err != nil {
		t.Fatal(err)
	}
	if status.NextAction != "complete" || status.ClockOut == nil || status.DayStatus != "abnormal" || status.CanClockIn || status.CanClockOut {
		t.Fatalf("expected early clock-out to remain visible as an abnormal day, got %+v", status)
	}

	setNow(clockInAt.Add(9 * time.Hour))
	valid, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_out",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	if valid.RecordStatus != "accepted" {
		t.Fatalf("expected clock-out after eight elapsed hours to be accepted, got %+v", valid)
	}
}

// TestAttendanceClockFixedModeRecordsLateAndEarlyPunchesAsAcceptedAnomalies 驗證固定打卡遲到早退仍會完成打卡。
func TestAttendanceClockFixedModeRecordsLateAndEarlyPunchesAsAcceptedAnomalies(t *testing.T) {
	store, svc, employeeCtx, _, setNow := newAttendanceFixture(t)
	clockInAt := attendanceFixtureClockInTime()
	effectiveFrom := attendanceFixtureWorkDateStart()
	if err := store.InsertAttendancePolicyVersion(context.Background(), domain.AttendancePolicy{
		TenantID: "tenant-1",
		WorkTime: domain.AttendancePolicyWorkTime{
			RequireWorksite: true,
			ClockMode:       "fixed",
			StandardStart:   "09:00",
			StandardEnd:     "18:00",
			BreakStart:      "12:00",
			BreakEnd:        "13:00",
			Weekend:         "週六、週日",
			CycleStart:      "1 日",
			CycleEnd:        "本月 月底（最後一日）",
		},
		Version:       1,
		EffectiveFrom: &effectiveFrom,
		PublishedAt:   effectiveFrom,
	}); err != nil {
		t.Fatal(err)
	}

	setNow(clockInAt.Add(30 * time.Minute))
	late, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_in",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	if late.RecordStatus != "accepted" || late.RejectionReason != "outside_time_window" {
		t.Fatalf("expected fixed-mode late clock-in to be recorded as anomaly, got %+v", late)
	}
	secondIn, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_in",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	})
	if err != nil || secondIn.RecordStatus != "rejected" || secondIn.RejectionReason != "duplicate" {
		t.Fatalf("expected repeated fixed-mode clock-in to be rejected, got record=%+v err=%v", secondIn, err)
	}

	setNow(clockInAt.Add(8 * time.Hour))
	early, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_out",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	if early.RecordStatus != "accepted" || early.RejectionReason != "outside_time_window" {
		t.Fatalf("expected fixed-mode early clock-out to be recorded as anomaly, got %+v", early)
	}
	status, err := svc.Attendance().AttendanceClockStatus(employeeCtx)
	if err != nil {
		t.Fatal(err)
	}
	if status.NextAction != "complete" || status.ClockIn == nil || status.ClockOut == nil || status.DayStatus != "abnormal" || status.CanClockIn || status.CanClockOut {
		t.Fatalf("expected fixed-mode punches to produce an abnormal daily projection, got %+v", status)
	}

	setNow(clockInAt.Add(9 * time.Hour))
	finalOut, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_out",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	})
	if err != nil || finalOut.RecordStatus != "accepted" {
		t.Fatalf("expected later fixed-mode clock-out to be stored, got record=%+v err=%v", finalOut, err)
	}
}

// TestAttendanceClockFlexibleBoundsKeepAbnormalClockOutRetryable 驗證彈性範圍異常上班不可重複、異常下班可重打。
func TestAttendanceClockFlexibleBoundsKeepAbnormalClockOutRetryable(t *testing.T) {
	store, svc, employeeCtx, _, setNow := newAttendanceFixture(t)
	base := attendanceFixtureClockInTime()
	effectiveFrom := attendanceFixtureWorkDateStart()
	if err := store.InsertAttendancePolicyVersion(context.Background(), domain.AttendancePolicy{
		TenantID: "tenant-1",
		WorkTime: domain.AttendancePolicyWorkTime{
			RequireWorksite:         true,
			ClockMode:               "flexible",
			FlexibleClockInEarliest: "08:00",
			FlexibleClockOutLatest:  "20:00",
			StandardStart:           "09:00",
			StandardEnd:             "18:00",
			BreakStart:              "12:00",
			BreakEnd:                "13:00",
			Weekend:                 "週六、週日",
			CycleStart:              "1 日",
			CycleEnd:                "本月 月底（最後一日）",
		},
		Version:       1,
		EffectiveFrom: &effectiveFrom,
		PublishedAt:   effectiveFrom,
	}); err != nil {
		t.Fatal(err)
	}

	setNow(base.Add(-90 * time.Minute))
	earlyIn, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_in",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	if earlyIn.RecordStatus != "accepted" || earlyIn.RejectionReason != "outside_time_window" {
		t.Fatalf("expected early flexible clock-in to be recorded as anomaly, got %+v", earlyIn)
	}
	secondIn, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_in",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	})
	if err != nil || secondIn.RecordStatus != "rejected" || secondIn.RejectionReason != "duplicate" {
		t.Fatalf("expected repeated flexible clock-in to be rejected, got record=%+v err=%v", secondIn, err)
	}

	setNow(base.Add(11*time.Hour + 30*time.Minute))
	lateOut, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_out",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	if lateOut.RecordStatus != "accepted" || lateOut.RejectionReason != "outside_time_window" {
		t.Fatalf("expected late flexible clock-out to remain accepted raw evidence, got %+v", lateOut)
	}
	status, err := svc.Attendance().AttendanceClockStatus(employeeCtx)
	if err != nil {
		t.Fatal(err)
	}
	if status.NextAction != "complete" || status.ClockIn == nil || status.ClockOut == nil || status.DayStatus != "abnormal" || status.CanClockIn || status.CanClockOut {
		t.Fatalf("expected flexible bounds anomaly in daily projection, got %+v", status)
	}
}

// TestAttendanceClockResetsNormalWorkDateAfterMidnight verifies clock status uses the calendar work date from policy only.
func TestAttendanceClockResetsNormalWorkDateAfterMidnight(t *testing.T) {
	_, svc, employeeCtx, _, setNow := newAttendanceFixture(t)
	if _, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_in",
		ClientEventID:  "normal-day-in",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	}); err != nil {
		t.Fatal(err)
	}
	setNow(attendanceFixtureClockOutTime())
	if _, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_out",
		ClientEventID:  "normal-day-out",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	}); err != nil {
		t.Fatal(err)
	}

	setNow(attendanceFixtureClockOutTime().Add(8 * time.Hour))
	status, err := svc.Attendance().AttendanceClockStatus(employeeCtx)
	if err != nil {
		t.Fatal(err)
	}
	if status.WorkDate != "2026-06-11" || status.ClockIn != nil || status.ClockOut != nil || status.NextAction != "clock_in" || !status.CanClockIn || status.CanClockOut {
		t.Fatalf("expected a fresh normal work date after midnight, got %+v", status)
	}

	clockOut, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_out",
		ClientEventID:  "normal-next-day-out",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	if clockOut.WorkDate != "2026-06-11" || clockOut.RecordStatus != "rejected" || clockOut.RejectionReason != "invalid_sequence" {
		t.Fatalf("expected next-day clock-out to require a new clock-in, got %+v", clockOut)
	}
}

// TestAttendanceClockRecordSkipsGeofenceWhenPolicyDisablesIt 驗證關閉地點校驗後不要求工作地點或定位範圍。
func TestAttendanceClockRecordSkipsGeofenceWhenPolicyDisablesIt(t *testing.T) {
	store, svc, employeeCtx, _, _ := newAttendanceFixture(t)
	effectiveFrom := attendanceFixtureWorkDateStart()
	if err := store.InsertAttendancePolicyVersion(context.Background(), domain.AttendancePolicy{
		TenantID: "tenant-1",
		WorkTime: domain.AttendancePolicyWorkTime{
			RequireWorksite: false,
			StandardStart:   "09:00",
			StandardEnd:     "18:00",
			BreakStart:      "12:00",
			BreakEnd:        "13:00",
			Weekend:         "週六、週日",
			CycleStart:      "1 日",
			CycleEnd:        "本月 月底（最後一日）",
		},
		Version:       1,
		EffectiveFrom: &effectiveFrom,
		PublishedAt:   effectiveFrom,
	}); err != nil {
		t.Fatal(err)
	}

	record, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_in",
		Latitude:       0,
		Longitude:      0,
		AccuracyMeters: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.RecordStatus != "accepted" || record.WorksiteID != "" {
		t.Fatalf("expected accepted clock-in without worksite, got %+v", record)
	}
}

// TestAttendanceClockReadIncludesAssociatedWorksiteDetails verifies self-service reads expose only the linked place label.
func TestAttendanceClockReadIncludesAssociatedWorksiteDetails(t *testing.T) {
	_, svc, employeeCtx, _, _ := newAttendanceFixture(t)
	if _, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_in",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	}); err != nil {
		t.Fatal(err)
	}

	page, err := svc.Attendance().ListAttendanceClockRecordPage(employeeCtx, domain.AttendanceClockRecordQuery{EmployeeID: "emp-1"}, domain.PageRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("expected one self-service clock record, got %+v", page)
	}
	item := page.Items[0]
	if item.WorksiteID != "aws-1" || item.WorksiteName != "HQ" || item.WorksiteAddress != "No. 1, HQ Road" {
		t.Fatalf("expected linked worksite display details, got %+v", item)
	}
}

// TestAttendanceClockReadAppliesFieldPolicyToGPSAndDeviceInfo 驗證考勤打卡 read applies 欄位政策 to gps and device info。
func TestAttendanceClockReadAppliesFieldPolicyToGPSAndDeviceInfo(t *testing.T) {
	store, svc, employeeCtx, adminCtx, _ := newAttendanceFixture(t)

	created, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_in",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
		LocationSource: "gps",
		DeviceID:       "phone-1",
		DeviceInfo:     map[string]any{"os": "ios"},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, ok, err := store.GetEarliestAcceptedAttendanceClockIn(context.Background(), "tenant-1", "emp-1", created.WorkDate)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || raw.Latitude == 0 || raw.Longitude == 0 || raw.DeviceID == "" || raw.DeviceInfo["location_source"] != "gps" {
		t.Fatalf("expected raw clock evidence to remain in storage, ok=%v record=%+v", ok, raw)
	}

	page, err := svc.Attendance().ListAttendanceClockRecordPage(adminCtx, domain.AttendanceClockRecordQuery{EmployeeID: "emp-1"}, domain.PageRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("expected one visible clock record, got %+v", page)
	}
	item := page.Items[0]
	if item.Latitude != 0 || item.Longitude != 0 || item.AccuracyMeters != 0 || item.DistanceMeters != 0 || item.DeviceID != "" || item.DeviceInfo != nil {
		t.Fatalf("expected clock read to hide GPS and device evidence by default, got %+v", item)
	}
}

// TestAttendanceClockReadAllowsGPSAndDeviceInfoByPermissionFieldPolicy 驗證考勤打卡 read allows gps and device info by 權限欄位政策。
func TestAttendanceClockReadAllowsGPSAndDeviceInfoByPermissionFieldPolicy(t *testing.T) {
	store, svc, employeeCtx, adminCtx, _ := newAttendanceFixture(t)
	allowAttendanceClockSensitiveFieldsForPermission(t, store, attendanceFixtureClockInTime(), "attendance.clock.read")

	_, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_in",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
		LocationSource: "gps",
		DeviceID:       "phone-1",
		DeviceInfo:     map[string]any{"os": "ios"},
	})
	if err != nil {
		t.Fatal(err)
	}

	page, err := svc.Attendance().ListAttendanceClockRecordPage(adminCtx, domain.AttendanceClockRecordQuery{EmployeeID: "emp-1"}, domain.PageRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("expected one visible clock record, got %+v", page)
	}
	item := page.Items[0]
	if item.Latitude != 25.033964 || item.Longitude != 121.564468 || item.AccuracyMeters != 12 || item.DeviceID != "phone-1" || item.DeviceInfo["location_source"] != "gps" || item.DeviceInfo["os"] != "ios" {
		t.Fatalf("expected explicit allow policy to reveal clock GPS and device evidence, got %+v", item)
	}
}

// TestAttendanceClockRejectsSequenceAndLowAccuracy 驗證考勤打卡 rejects sequence and low accuracy。
func TestAttendanceClockRejectsSequenceAndLowAccuracy(t *testing.T) {
	_, svc, employeeCtx, _, setNow := newAttendanceFixture(t)

	setNow(attendanceFixtureClockOutTime())
	invalidSequence, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction: "clock_out",
		Latitude:  25.033964,
		Longitude: 121.564468,
	})
	if err != nil {
		t.Fatal(err)
	}
	if invalidSequence.RecordStatus != "rejected" || invalidSequence.RejectionReason != "invalid_sequence" {
		t.Fatalf("expected clock-out before clock-in rejection, got %+v", invalidSequence)
	}

	_, svc, employeeCtx, _, setNow = newAttendanceFixture(t)
	setNow(attendanceFixtureClockInTime())
	lowAccuracy, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_in",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 250,
	})
	if err != nil {
		t.Fatal(err)
	}
	if lowAccuracy.RecordStatus != "rejected" || lowAccuracy.RejectionReason != "low_location_accuracy" {
		t.Fatalf("expected low accuracy rejection, got %+v", lowAccuracy)
	}
}

// TestAttendanceClockSelfScopeCannotTargetAnotherEmployee 驗證考勤打卡 self 範圍 cannot target another 員工。
func TestAttendanceClockSelfScopeCannotTargetAnotherEmployee(t *testing.T) {
	_, svc, employeeCtx, adminCtx, _ := newAttendanceFixture(t)

	_, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		EmployeeID: "emp-2",
		Direction:  "clock_in",
		Latitude:   25.033964,
		Longitude:  121.564468,
	})
	if err == nil {
		t.Fatal("expected self-scoped account to be forbidden from clocking another employee")
	}

	_, err = svc.Attendance().CreateAttendanceClockRecord(adminCtx, domain.CreateAttendanceClockRecordInput{
		EmployeeID: "emp-1",
		Direction:  "clock_in",
		Latitude:   25.033964,
		Longitude:  121.564468,
	})
	if err == nil {
		t.Fatal("expected admin account to use correction flow instead of direct employee clocking")
	}
}

// TestAttendanceCorrectionApproveCreatesManualRecordAndRejectDoesNot 驗證考勤 correction 覈準 creates manual record and 駁回 does not。
func TestAttendanceCorrectionApproveCreatesManualRecordAndRejectDoesNot(t *testing.T) {
	store, svc, employeeCtx, adminCtx, _ := newAttendanceFixture(t)
	requestedAt := attendanceFixtureClockInTime().Format(time.RFC3339)

	pending, err := svc.Attendance().CreateAttendanceCorrection(employeeCtx, domain.CreateAttendanceCorrectionInput{
		Direction:          "clock_in",
		RequestedClockedAt: requestedAt,
		Reason:             "forgot to clock in",
	})
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != "pending" || pending.FormInstanceID == "" {
		t.Fatalf("expected pending correction with form evidence, got %+v", pending)
	}

	approved, err := svc.Attendance().ApproveAttendanceCorrection(adminCtx, pending.ID, domain.ReviewAttendanceCorrectionInput{Reason: "verified"})
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != "approved" || approved.ClockRecordID == "" {
		t.Fatalf("expected approved correction with manual record, got %+v", approved)
	}
	record, ok, err := store.GetEarliestAcceptedAttendanceClockIn(context.Background(), "tenant-1", "emp-1", approved.WorkDate)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || record.Source != "manual_correction" || record.CorrectionRequestID != approved.ID {
		t.Fatalf("expected accepted manual correction record, got ok=%v record=%+v", ok, record)
	}
	if record.WorksiteID != "" || record.LocationCaptured || record.Latitude != 0 || record.Longitude != 0 {
		t.Fatalf("manual correction must not fabricate worksite or GPS evidence: %+v", record)
	}

	rejected, err := svc.Attendance().CreateAttendanceCorrection(employeeCtx, domain.CreateAttendanceCorrectionInput{
		Direction:          "clock_out",
		RequestedClockedAt: attendanceFixtureClockOutTime().Format(time.RFC3339),
		Reason:             "forgot to clock out",
	})
	if err != nil {
		t.Fatal(err)
	}
	rejected, err = svc.Attendance().RejectAttendanceCorrection(adminCtx, rejected.ID, domain.ReviewAttendanceCorrectionInput{Reason: "not enough evidence"})
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != "rejected" || rejected.ClockRecordID != "" {
		t.Fatalf("expected rejected correction without record, got %+v", rejected)
	}
	if _, ok, err := store.GetLatestAcceptedAttendanceClockOut(context.Background(), "tenant-1", "emp-1", rejected.WorkDate); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("rejecting a correction should not create an accepted clock-out record")
	}
}

// TestAttendanceCorrectionConcurrentApprovalClaimsPendingOnce verifies the
// transaction-level row lock prevents two reviewers from applying the same
// correction side effect twice.
func TestAttendanceCorrectionConcurrentApprovalClaimsPendingOnce(t *testing.T) {
	store, svc, employeeCtx, adminCtx, _ := newAttendanceFixture(t)
	pending, err := svc.Attendance().CreateAttendanceCorrection(employeeCtx, domain.CreateAttendanceCorrectionInput{
		Direction: "clock_in", RequestedClockedAt: attendanceFixtureClockInTime().Format(time.RFC3339), Reason: "forgot to clock in",
	})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, reviewErr := svc.Attendance().ApproveAttendanceCorrection(adminCtx, pending.ID, domain.ReviewAttendanceCorrectionInput{Reason: "verified"})
			errs <- reviewErr
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	successes := 0
	for reviewErr := range errs {
		if reviewErr == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent approval successes = %d, want exactly one", successes)
	}
	records, err := store.ListAttendanceClockRecords(context.Background(), "tenant-1", domain.AttendanceClockRecordQuery{EmployeeID: "emp-1"})
	if err != nil {
		t.Fatal(err)
	}
	created := 0
	for _, record := range records {
		if record.CorrectionRequestID == pending.ID {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("manual records for correction = %d, want one", created)
	}
}

// TestAttendanceCorrectionReplaceVoidsTargetAndCreatesReplacement 驗證誤卡保留審計但不再參與首尾投影。
func TestAttendanceCorrectionReplaceVoidsTargetAndCreatesReplacement(t *testing.T) {
	store, svc, employeeCtx, adminCtx, _ := newAttendanceFixture(t)
	target, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_in",
		ClientEventID:  "mistaken-clock-in",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	replacementAt := attendanceFixtureClockInTime().Add(15 * time.Minute)
	pending, err := svc.Attendance().CreateAttendanceCorrection(employeeCtx, domain.CreateAttendanceCorrectionInput{
		CorrectionType:      "replace_record",
		TargetClockRecordID: target.ID,
		Direction:           "clock_in",
		RequestedClockedAt:  replacementAt.Format(time.RFC3339),
		Reason:              "selected the wrong punch time",
	})
	if err != nil {
		t.Fatal(err)
	}
	approved, err := svc.Attendance().ApproveAttendanceCorrection(adminCtx, pending.ID, domain.ReviewAttendanceCorrectionInput{Reason: "verified"})
	if err != nil {
		t.Fatal(err)
	}
	if approved.ReplacementClockRecordID == "" || approved.TargetClockRecordID != target.ID {
		t.Fatalf("replacement audit links missing: %+v", approved)
	}
	records, err := store.ListAttendanceClockRecords(context.Background(), "tenant-1", domain.AttendanceClockRecordQuery{EmployeeID: "emp-1"})
	if err != nil {
		t.Fatal(err)
	}
	var original, replacement domain.AttendanceClockRecord
	for _, record := range records {
		switch record.ID {
		case target.ID:
			original = record
		case approved.ReplacementClockRecordID:
			replacement = record
		}
	}
	if !original.Voided || original.VoidReason != "verified" || replacement.RecordStatus != "accepted" {
		t.Fatalf("unexpected replace result: original=%+v replacement=%+v", original, replacement)
	}
	boundary, ok, err := store.GetEarliestAcceptedAttendanceClockIn(context.Background(), "tenant-1", "emp-1", target.WorkDate)
	if err != nil || !ok || boundary.ID != replacement.ID {
		t.Fatalf("effective boundary should use replacement, ok=%v err=%v record=%+v", ok, err, boundary)
	}
}

// TestAttendanceCorrectionReplaceShortHoursClockOut avoids add-record conflicts by replacing the bound accepted event.
func TestAttendanceCorrectionReplaceShortHoursClockOut(t *testing.T) {
	store, svc, employeeCtx, adminCtx, setNow := newAttendanceFixture(t)
	clockIn, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_in",
		ClientEventID:  "short-hours-clock-in",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	setNow(attendanceFixtureClockOutTime())
	clockOut, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_out",
		ClientEventID:  "short-hours-clock-out",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	})
	if err != nil {
		t.Fatal(err)
	}

	pending, err := svc.Attendance().CreateAttendanceCorrection(employeeCtx, domain.CreateAttendanceCorrectionInput{
		CorrectionType:      "replace_record",
		TargetClockRecordID: clockOut.ID,
		Direction:           "clock_out",
		RequestedClockedAt:  attendanceFixtureClockOutTime().Add(30 * time.Minute).Format(time.RFC3339),
		Reason:              "replace insufficient-hours clock out",
	})
	if err != nil {
		t.Fatal(err)
	}
	approved, err := svc.Attendance().ApproveAttendanceCorrection(adminCtx, pending.ID, domain.ReviewAttendanceCorrectionInput{Reason: "verified short-hours correction"})
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != "approved" || approved.ReplacementClockRecordID == "" {
		t.Fatalf("expected replacement approval without sequence conflict, got %+v", approved)
	}
	if original, ok, err := store.GetEarliestAcceptedAttendanceClockIn(context.Background(), "tenant-1", "emp-1", clockIn.WorkDate); err != nil || !ok || original.ID != clockIn.ID {
		t.Fatalf("clock-in boundary changed unexpectedly, ok=%v err=%v record=%+v", ok, err, original)
	}
	if replacement, ok, err := store.GetLatestAcceptedAttendanceClockOut(context.Background(), "tenant-1", "emp-1", clockOut.WorkDate); err != nil || !ok || replacement.ID != approved.ReplacementClockRecordID {
		t.Fatalf("expected replacement clock-out boundary, ok=%v err=%v record=%+v", ok, err, replacement)
	}
}

// TestAttendanceCorrectionTargetRecordOwnership rejects a target owned by another employee before request creation.
func TestAttendanceCorrectionTargetRecordOwnership(t *testing.T) {
	store, svc, employeeCtx, _, _ := newAttendanceFixture(t)
	now := attendanceFixtureClockInTime()
	foreign := domain.AttendanceClockRecord{
		ID:           "acr-foreign",
		TenantID:     "tenant-1",
		EmployeeID:   "emp-2",
		WorksiteID:   "aws-1",
		WorkDate:     "2026-06-10",
		Direction:    "clock_in",
		ClockedAt:    now,
		RecordStatus: "accepted",
		Source:       "geofence",
		CreatedAt:    now,
	}
	if err := store.UpsertAttendanceClockRecord(context.Background(), foreign); err != nil {
		t.Fatal(err)
	}

	_, err := svc.Attendance().CreateAttendanceCorrection(employeeCtx, domain.CreateAttendanceCorrectionInput{
		CorrectionType:      "replace_record",
		TargetClockRecordID: foreign.ID,
		Direction:           "clock_in",
		RequestedClockedAt:  now.Add(15 * time.Minute).Format(time.RFC3339),
		Reason:              "must not replace another employee record",
	})
	if err == nil {
		t.Fatal("expected another employee's target record to stay inaccessible")
	}
	if appErr, ok := domain.AsAppError(err); !ok || appErr.Status != http.StatusNotFound {
		t.Fatalf("expected ownership-safe not found, got %v", err)
	}
}

// TestCreateLeaveRequestReservesLeaveBalance 驗證請假請求 reserves 請假 balance。
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
	_ = store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{ID: "lb-1", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", RemainingMinutes: 16 * 60, UpdatedAt: now})

	created, err := newDirectAttendanceWorkflowService(t, store, now, "leave-request").Attendance().CreateLeaveRequest(
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
	balance := effectiveLeaveBalanceForTest(t, store, "lb-1")
	if created.RequestedMinutes != 7*60 || balance.RemainingMinutes != 9*60 {
		t.Fatalf("expected policy-derived 7 hours and remaining balance 9, request=%+v balance=%+v", created, balance)
	}
}

// TestLeaveWorkflowReviewUpdatesRequestAndBalance 驗證請假流程審核 updates 請求 and balance。
func TestLeaveWorkflowReviewUpdatesRequestAndBalance(t *testing.T) {
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
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-workflow-reviewer",
		TenantID: "tenant-1",
		Name:     "Workflow Reviewer",
		Permissions: []domain.Permission{
			{Resource: "workflow.form_instance", Action: "read", Scope: "all"},
			{Resource: "workflow.form_instance", Action: "update", Scope: "all"},
			{Resource: "workflow.form_instance", Action: "approve", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-employee",
		TenantID:               "tenant-1",
		DisplayName:            "Employee One",
		EmployeeID:             "emp-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-self"},
		CreatedAt:              now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-reviewer",
		TenantID:               "tenant-1",
		DisplayName:            "Reviewer",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-workflow-reviewer"},
		CreatedAt:              now,
	})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", Name: "Employee One", Status: "active", CreatedAt: now})
	_ = store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{ID: "lb-1", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", RemainingMinutes: 24 * 60, UpdatedAt: now})
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID:        "ft-leave",
		TenantID:  "tenant-1",
		Key:       "leave-request",
		Name:      "請假申請單",
		Schema:    workflowEnabledTemplateSchema("acct-reviewer"),
		CreatedAt: now,
	})
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return now.Add(time.Hour) }})
	employeeCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-employee"}
	reviewerCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-reviewer"}

	approvedRequest, err := svc.Attendance().CreateLeaveRequest(employeeCtx, domain.CreateLeaveRequestInput{
		LeaveType: "annual",
		StartAt:   "2026-06-10",
		EndAt:     "2026-06-11",
		Hours:     8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Workflow().ApproveForm(reviewerCtx, approvedRequest.FormInstanceID, domain.ApproveFormInput{}); err != nil {
		t.Fatal(err)
	}
	storedApproved, ok, err := store.GetLeaveRequest(context.Background(), "tenant-1", approvedRequest.ID)
	if err != nil || !ok {
		t.Fatalf("approved leave request missing ok=%v err=%v", ok, err)
	}
	if storedApproved.Status != "approved" {
		t.Fatalf("expected approved leave request, got %+v", storedApproved)
	}
	balance := effectiveLeaveBalanceForTest(t, store, "lb-1")
	if balance.RemainingMinutes != 17*60 {
		t.Fatalf("approval should keep Nexus-consumed minutes deducted from the effective balance, got %v", balance.RemainingMinutes)
	}

	rejectedRequest, err := svc.Attendance().CreateLeaveRequest(employeeCtx, domain.CreateLeaveRequestInput{
		LeaveType: "annual",
		StartAt:   "2026-06-12",
		EndAt:     "2026-06-13",
		Hours:     8,
	})
	if err != nil {
		t.Fatal(err)
	}
	later := now.Add(30 * time.Minute)
	_ = store.InsertAttendancePolicyVersion(context.Background(), domain.AttendancePolicy{
		TenantID: "tenant-1", Version: rejectedRequest.PolicyVersion + 1,
		WorkTime:      domain.AttendancePolicyWorkTime{StandardStart: "09:00", StandardEnd: "18:00", BreakStart: "12:00", BreakEnd: "13:00"},
		EffectiveFrom: &later, PublishedAt: later,
	})
	if _, err := svc.Workflow().RejectForm(reviewerCtx, rejectedRequest.FormInstanceID, domain.RejectFormInput{Reason: "missing attachment"}); err != nil {
		t.Fatal(err)
	}
	storedRejected, ok, err := store.GetLeaveRequest(context.Background(), "tenant-1", rejectedRequest.ID)
	if err != nil || !ok {
		t.Fatalf("rejected leave request missing ok=%v err=%v", ok, err)
	}
	if storedRejected.Status != "rejected" {
		t.Fatalf("expected rejected leave request, got %+v", storedRejected)
	}
	balance = effectiveLeaveBalanceForTest(t, store, "lb-1")
	if balance.RemainingMinutes != 17*60 {
		t.Fatalf("rejection should release reserved minutes, got %v", balance.RemainingMinutes)
	}
}

// TestPunchFixFormSubmitCreatesLinkedCorrection 驗證通用表單提交會建立補卡業務記錄。
func TestPunchFixFormSubmitCreatesLinkedCorrection(t *testing.T) {
	store, svc, employeeCtx, _, _ := newAttendanceFixture(t)
	now := attendanceFixtureClockInTime()
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID:        "ft-punch-fix",
		TenantID:  "tenant-1",
		Key:       "punch-fix",
		Name:      "HR-005 補卡單",
		Schema:    workflowEnabledTemplateSchema("acct-admin"),
		CreatedAt: now,
	})

	submitted, err := svc.Workflow().SubmitForm(employeeCtx, domain.SubmitFormInput{
		TemplateKey: "punch-fix",
		Payload: map[string]any{
			"correction_type":      "add_record",
			"direction":            "clock_in",
			"requested_clocked_at": now.Format(time.RFC3339),
			"reason":               "forgot to clock in",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request, ok, err := store.GetAttendanceCorrectionRequestByFormInstanceID(context.Background(), "tenant-1", submitted.ID)
	if err != nil || !ok {
		t.Fatalf("linked correction missing ok=%v err=%v", ok, err)
	}
	if request.EmployeeID != "emp-1" || request.FormInstanceID != submitted.ID || request.Status != "pending" || request.EffectStatus != "not_applied" {
		t.Fatalf("unexpected linked correction: %+v", request)
	}
	if submitted.Payload["linked_resource_id"] != request.ID || submitted.Payload["linked_resource_type"] != "attendance.clock_correction" {
		t.Fatalf("expected form payload to link correction record, got %+v", submitted.Payload)
	}
}

// TestCorrectionWorkflowApproveCreatesClockRecord 驗證補卡單走 workflow 審批也會產生打卡記錄。
func TestCorrectionWorkflowApproveCreatesClockRecord(t *testing.T) {
	store, svc, employeeCtx, _, _ := newAttendanceFixture(t)
	now := attendanceFixtureClockInTime()
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-workflow-reviewer",
		TenantID: "tenant-1",
		Name:     "Workflow Reviewer",
		Permissions: []domain.Permission{
			{Resource: "workflow.form_instance", Action: "read", Scope: "all"},
			{Resource: "workflow.form_instance", Action: "update", Scope: "all"},
			{Resource: "workflow.form_instance", Action: "approve", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-reviewer",
		TenantID:               "tenant-1",
		DisplayName:            "Reviewer",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-workflow-reviewer"},
		CreatedAt:              now,
	})
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID:        "ft-punch-fix",
		TenantID:  "tenant-1",
		Key:       "punch-fix",
		Name:      "HR-005 補卡單",
		Schema:    workflowEnabledTemplateSchema("acct-reviewer"),
		CreatedAt: now,
	})
	reviewerCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-reviewer"}

	pending, err := svc.Attendance().CreateAttendanceCorrection(employeeCtx, domain.CreateAttendanceCorrectionInput{
		Direction:          "clock_in",
		RequestedClockedAt: now.Format(time.RFC3339),
		Reason:             "forgot to clock in",
	})
	if err != nil {
		t.Fatal(err)
	}
	startWorkflowRunForTest(t, svc, store, "tenant-1", pending.FormInstanceID, "acct-employee")
	if _, err := svc.Workflow().ApproveForm(reviewerCtx, pending.FormInstanceID, domain.ApproveFormInput{Reason: "verified"}); err != nil {
		t.Fatal(err)
	}
	stored, ok, err := store.GetAttendanceCorrectionRequest(context.Background(), "tenant-1", pending.ID)
	if err != nil || !ok {
		t.Fatalf("correction missing ok=%v err=%v", ok, err)
	}
	if stored.Status != "approved" || stored.ClockRecordID == "" {
		t.Fatalf("expected workflow-approved correction with clock record, got %+v", stored)
	}
	record, ok, err := store.GetEarliestAcceptedAttendanceClockIn(context.Background(), "tenant-1", "emp-1", stored.WorkDate)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || record.Source != "manual_correction" || record.CorrectionRequestID != stored.ID {
		t.Fatalf("expected accepted manual correction record via workflow, got ok=%v record=%+v", ok, record)
	}
}

// TestCorrectionWorkflowRejectMarksRejectedWithoutRecord 驗證補卡單 workflow 駁回不產生打卡記錄。
func TestCorrectionWorkflowRejectMarksRejectedWithoutRecord(t *testing.T) {
	store, svc, employeeCtx, _, _ := newAttendanceFixture(t)
	now := attendanceFixtureClockInTime()
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-workflow-reviewer",
		TenantID: "tenant-1",
		Name:     "Workflow Reviewer",
		Permissions: []domain.Permission{
			{Resource: "workflow.form_instance", Action: "read", Scope: "all"},
			{Resource: "workflow.form_instance", Action: "update", Scope: "all"},
			{Resource: "workflow.form_instance", Action: "approve", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-reviewer",
		TenantID:               "tenant-1",
		DisplayName:            "Reviewer",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-workflow-reviewer"},
		CreatedAt:              now,
	})
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID:        "ft-punch-fix",
		TenantID:  "tenant-1",
		Key:       "punch-fix",
		Name:      "HR-005 補卡單",
		Schema:    workflowEnabledTemplateSchema("acct-reviewer"),
		CreatedAt: now,
	})
	reviewerCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-reviewer"}

	pending, err := svc.Attendance().CreateAttendanceCorrection(employeeCtx, domain.CreateAttendanceCorrectionInput{
		Direction:          "clock_in",
		RequestedClockedAt: now.Format(time.RFC3339),
		Reason:             "forgot to clock in",
	})
	if err != nil {
		t.Fatal(err)
	}
	startWorkflowRunForTest(t, svc, store, "tenant-1", pending.FormInstanceID, "acct-employee")
	if _, err := svc.Workflow().RejectForm(reviewerCtx, pending.FormInstanceID, domain.RejectFormInput{Reason: "not enough evidence"}); err != nil {
		t.Fatal(err)
	}
	stored, ok, err := store.GetAttendanceCorrectionRequest(context.Background(), "tenant-1", pending.ID)
	if err != nil || !ok {
		t.Fatalf("correction missing ok=%v err=%v", ok, err)
	}
	if stored.Status != "rejected" || stored.ClockRecordID != "" {
		t.Fatalf("expected workflow-rejected correction without record, got %+v", stored)
	}
	if _, ok, err := store.GetEarliestAcceptedAttendanceClockIn(context.Background(), "tenant-1", "emp-1", stored.WorkDate); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("workflow rejection should not create an accepted clock record")
	}
}

// TestOvertimeWorkflowReviewUpdatesRequestAndCreditsBalance 驗證加班流程審核更新申請並累積補休餘額。
func TestOvertimeWorkflowReviewUpdatesRequestAndCreditsBalance(t *testing.T) {
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
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-workflow-reviewer",
		TenantID: "tenant-1",
		Name:     "Workflow Reviewer",
		Permissions: []domain.Permission{
			{Resource: "workflow.form_instance", Action: "read", Scope: "all"},
			{Resource: "workflow.form_instance", Action: "update", Scope: "all"},
			{Resource: "workflow.form_instance", Action: "approve", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-employee",
		TenantID:               "tenant-1",
		DisplayName:            "Employee One",
		EmployeeID:             "emp-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-self"},
		CreatedAt:              now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-reviewer",
		TenantID:               "tenant-1",
		DisplayName:            "Reviewer",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-workflow-reviewer"},
		CreatedAt:              now,
	})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", Name: "Employee One", Status: "active", CreatedAt: now})
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID:        "ft-overtime",
		TenantID:  "tenant-1",
		Key:       "overtime-approval",
		Name:      "加班覈準申請單",
		Schema:    workflowEnabledTemplateSchema("acct-reviewer"),
		CreatedAt: now,
	})
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return now.Add(time.Hour) }})
	employeeCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-employee"}
	reviewerCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-reviewer"}

	approvedRequest, err := svc.Attendance().CreateOvertimeRequest(employeeCtx, domain.CreateOvertimeRequestInput{
		StartAt:          "2026-06-12T18:00:00Z",
		EndAt:            "2026-06-12T21:00:00Z",
		Hours:            3,
		OvertimeType:     "weekday",
		CompensationType: "leave",
		Reason:           "release",
	})
	if err != nil {
		t.Fatal(err)
	}
	if approvedRequest.Status != "pending_approval" || approvedRequest.FormInstanceID == "" {
		t.Fatalf("expected pending overtime request with form evidence, got %+v", approvedRequest)
	}
	if _, err := svc.Workflow().ApproveForm(reviewerCtx, approvedRequest.FormInstanceID, domain.ApproveFormInput{}); err != nil {
		t.Fatal(err)
	}
	storedApproved, ok, err := store.GetOvertimeRequest(context.Background(), "tenant-1", approvedRequest.ID)
	if err != nil || !ok {
		t.Fatalf("approved overtime request missing ok=%v err=%v", ok, err)
	}
	if storedApproved.Status != "approved" {
		t.Fatalf("expected approved overtime request, got %+v", storedApproved)
	}
	balances, err := store.ListLeaveBalances(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	compensatory := 0
	compensatoryBalanceID := ""
	for _, balance := range balances {
		if balance.EmployeeID == "emp-1" && balance.LeaveType == "compensatory" {
			compensatoryBalanceID = balance.ID
		}
	}
	if compensatoryBalanceID != "" {
		compensatory = effectiveLeaveBalanceForTest(t, store, compensatoryBalanceID).RemainingMinutes
	}
	if compensatory != 3*60 {
		t.Fatalf("expected 180 compensatory minutes credited, got %v", compensatory)
	}
	rawAnchor, ok, err := store.GetLeaveBalance(t.Context(), "tenant-1", compensatoryBalanceID)
	if err != nil || !ok || rawAnchor.Source != "nexus" || rawAnchor.EntitlementYear != approvedRequest.StartAt.Year() || rawAnchor.RemainingMinutes != 0 || rawAnchor.GrantedMinutes != 0 {
		t.Fatalf("overtime credit must not mutate anchor snapshot, ok=%v err=%v anchor=%+v", ok, err, rawAnchor)
	}
	entries, err := store.ListLeaveBalanceEntries(t.Context(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	creditCount := 0
	for _, entry := range entries {
		if entry.EntryType == "overtime_credit" {
			creditCount++
			if entry.LeaveRecordID != "" || entry.AmountMinutes != 3*60 {
				t.Fatalf("credit must be a standalone minute-exact balance entry: %+v", entry)
			}
		}
	}
	if creditCount != 1 {
		t.Fatalf("expected one idempotent overtime credit, got %+v", entries)
	}

	rejectedRequest, err := svc.Attendance().CreateOvertimeRequest(employeeCtx, domain.CreateOvertimeRequestInput{
		StartAt: "2026-06-13T18:00:00Z",
		EndAt:   "2026-06-13T20:00:00Z",
		Hours:   2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Workflow().RejectForm(reviewerCtx, rejectedRequest.FormInstanceID, domain.RejectFormInput{Reason: "no need"}); err != nil {
		t.Fatal(err)
	}
	storedRejected, ok, err := store.GetOvertimeRequest(context.Background(), "tenant-1", rejectedRequest.ID)
	if err != nil || !ok {
		t.Fatalf("rejected overtime request missing ok=%v err=%v", ok, err)
	}
	if storedRejected.Status != "rejected" {
		t.Fatalf("expected rejected overtime request, got %+v", storedRejected)
	}
	balances, err = store.ListLeaveBalances(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	compensatory = 0
	for _, balance := range balances {
		if balance.EmployeeID == "emp-1" && balance.LeaveType == "compensatory" {
			compensatory = effectiveLeaveBalanceForTest(t, store, balance.ID).RemainingMinutes
		}
	}
	if compensatory != 3*60 {
		t.Fatalf("rejection should not change compensatory minutes, got %v", compensatory)
	}
}

// TestCreateLeaveRequestFallsBackFromInsufficientLeaveBalance verifies the form remains creatable.
func TestCreateLeaveRequestFallsBackFromInsufficientLeaveBalance(t *testing.T) {
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
	_ = store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{ID: "lb-1", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", RemainingMinutes: 4 * 60, UpdatedAt: now})

	created, err := newDirectAttendanceWorkflowService(t, store, now, "leave-request").Attendance().CreateLeaveRequest(
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
	if created.RuleSnapshot["requires_balance"] != true || created.EvaluationSnapshot["balance_required"] != false || created.EvaluationSnapshot["balance_fallback_reason"] != "insufficient_balance" {
		t.Fatalf("expected a no-balance fallback request with an auditable policy snapshot, got %+v", created)
	}
	if requests, err := store.ListLeaveRequests(context.Background(), "tenant-1"); err != nil || len(requests) != 1 {
		t.Fatalf("expected one leave request to be created, got %+v", requests)
	}
	forms, err := store.ListFormInstances(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(forms) != 1 {
		t.Fatalf("expected one form instance to be created, got %+v", forms)
	}
	balance, ok, err := store.GetLeaveBalance(context.Background(), "tenant-1", "lb-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("leave balance was not found")
	}
	if balance.RemainingMinutes != 4*60 {
		t.Fatalf("expected remaining balance to stay 240 minutes, got %v", balance.RemainingMinutes)
	}
}

// TestWorkflowDraftLifecycleAndPlatformProjection 驗證流程草稿生命週期 and 平臺 projection。
func TestWorkflowDraftLifecycleAndPlatformProjection(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-workflow-self",
		TenantID: "tenant-1",
		Name:     "Workflow Self Service",
		Permissions: []domain.Permission{
			{Resource: "workflow.form_instance", Action: "read", Scope: "self"},
			{Resource: "workflow.form_instance", Action: "create", Scope: "self"},
			{Resource: "workflow.form_instance", Action: "submit", Scope: "self"},
			{Resource: "workflow.form_instance", Action: "update", Scope: "self"},
			{Resource: "workflow.form_instance", Action: "delete", Scope: "self"},
			{Resource: "platform.forms", Action: "read", Scope: "self"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-self",
		TenantID:               "tenant-1",
		DisplayName:            "Self User",
		EmployeeID:             "emp-self",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-workflow-self"},
		CreatedAt:              now,
	})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:                "emp-self",
		TenantID:          "tenant-1",
		Name:              "Self User",
		AccountID:         "acct-self",
		ManagerEmployeeID: "emp-manager",
		Status:            "active",
		CreatedAt:         now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:          "acct-manager",
		TenantID:    "tenant-1",
		DisplayName: "Manager User",
		EmployeeID:  "emp-manager",
		Status:      "active",
		CreatedAt:   now,
	})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:        "emp-manager",
		TenantID:  "tenant-1",
		Name:      "Manager User",
		AccountID: "acct-manager",
		Status:    "active",
		CreatedAt: now,
	})
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID:        "ft-leave",
		TenantID:  "tenant-1",
		Key:       "leave-request",
		Name:      "請假申請單",
		Schema:    workflowEnabledTemplateSchema(),
		CreatedAt: now,
	})
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return now.Add(time.Hour) }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-self"}

	draft, err := svc.Workflow().SaveFormDraft(ctx, domain.SaveFormDraftInput{
		TemplateKey: "leave-request",
		Payload:     map[string]any{"reason": "draft leave"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Status != "draft" {
		t.Fatalf("expected draft status, got %+v", draft)
	}
	updated, err := svc.Workflow().UpdateFormDraft(ctx, draft.ID, domain.UpdateFormDraftInput{
		Payload: map[string]any{"reason": "updated leave"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Payload["reason"] != "updated leave" {
		t.Fatalf("expected updated payload, got %+v", updated.Payload)
	}
	forms, err := svc.Platform().Forms(ctx, domain.PlatformFormsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if forms.Drafts.Total != 1 || len(forms.Drafts.Items) != 1 || forms.Drafts.Items[0].ID != draft.ID || forms.Drafts.Items[0].Summary != "updated leave" {
		t.Fatalf("expected draft projection, got %+v", forms.Drafts)
	}

	submitted, err := svc.Workflow().SubmitForm(ctx, domain.SubmitFormInput{
		TemplateKey: draft.ID,
		Payload:     map[string]any{"reason": "submitted leave"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if submitted.ID != draft.ID || submitted.Status != "in_review" || submitted.Payload["reason"] != "submitted leave" {
		t.Fatalf("expected submitted draft, got %+v", submitted)
	}
	forms, err = svc.Platform().Forms(ctx, domain.PlatformFormsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if forms.Drafts.Total != 0 || forms.Applications.Total != 1 || len(forms.Applications.Items) != 1 || forms.Applications.Items[0].ID != draft.ID {
		t.Fatalf("expected one application and no drafts, got applications=%+v drafts=%+v", forms.Applications, forms.Drafts)
	}
	if submitted.TemplateVersionID == "" {
		t.Fatal("expected submitted form to bind an immutable template version")
	}
	template, ok, err := store.GetFormTemplate(context.Background(), "tenant-1", submitted.TemplateID)
	if err != nil || !ok {
		t.Fatalf("template lookup failed ok=%v err=%v", ok, err)
	}
	template.CurrentVersion = 2
	template.UpdatedAt = now.Add(2 * time.Hour)
	if err := store.UpsertFormTemplate(context.Background(), template); err != nil {
		t.Fatal(err)
	}
	currentVersion, ok, err := store.GetFormTemplateVersionByNumber(context.Background(), "tenant-1", submitted.TemplateID, 2)
	if err != nil || !ok {
		t.Fatalf("updated template version lookup failed ok=%v err=%v", ok, err)
	}
	if currentVersion.ID == submitted.TemplateVersionID {
		t.Fatalf("expected template update to create a distinct version, got %q", currentVersion.ID)
	}

	duplicate, err := svc.Workflow().DuplicateForm(ctx, submitted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Status != "draft" || duplicate.ID == submitted.ID || duplicate.Payload["reason"] != "submitted leave" {
		t.Fatalf("expected duplicated draft, got %+v", duplicate)
	}
	if duplicate.TemplateVersionID == "" || duplicate.TemplateVersionID != submitted.TemplateVersionID {
		t.Fatalf("expected duplicate to preserve source template version, source=%q duplicate=%q", submitted.TemplateVersionID, duplicate.TemplateVersionID)
	}
	persistedDuplicate, ok, err := store.GetFormInstance(context.Background(), "tenant-1", duplicate.ID)
	if err != nil || !ok {
		t.Fatalf("duplicate lookup failed ok=%v err=%v", ok, err)
	}
	if persistedDuplicate.TemplateVersionID != submitted.TemplateVersionID {
		t.Fatalf("expected persisted duplicate to preserve source template version, source=%q duplicate=%q", submitted.TemplateVersionID, persistedDuplicate.TemplateVersionID)
	}
	exported, err := svc.Workflow().ExportForm(ctx, submitted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if exported.FileName == "" || !strings.Contains(string(exported.Body), "submitted leave") {
		t.Fatalf("expected exported JSON to include submitted payload, got name=%q body=%s", exported.FileName, string(exported.Body))
	}
	detail, err := svc.Workflow().GetFormInstanceDetail(ctx, submitted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.ID != submitted.ID || detail.TemplateKey != "leave-request" || detail.TemplateName != "請假申請單" || detail.Payload["reason"] != "submitted leave" {
		t.Fatalf("expected renderable submitted form detail, got %+v", detail)
	}
	cancelled, err := svc.Workflow().CancelForm(ctx, submitted.ID, domain.CancelFormInput{Reason: "no longer needed"})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("expected cancelled status, got %+v", cancelled)
	}
	reviewQueue, err := svc.Workflow().ReviewQueue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviewQueue.AlreadyReviewed) != 1 || len(reviewQueue.AlreadyReviewed[0].ReviewLog) != 1 || reviewQueue.AlreadyReviewed[0].ReviewLog[0].Type != "cancel" {
		t.Fatalf("expected withdraw to project as the public cancel action, got %+v", reviewQueue.AlreadyReviewed)
	}
	deleted, err := svc.Workflow().DeleteFormDraft(ctx, duplicate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.ID != duplicate.ID {
		t.Fatalf("expected deleted draft, got %+v", deleted)
	}
	if _, ok, err := store.GetFormInstance(context.Background(), "tenant-1", duplicate.ID); err != nil || ok {
		t.Fatalf("expected duplicate draft to be removed ok=%v err=%v", ok, err)
	}
}

// TestWorkflowReviewQueueAndRejectForm 驗證流程審核佇列 and 駁回表單。
func TestWorkflowReviewQueueAndRejectForm(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-workflow-admin",
		TenantID: "tenant-1",
		Name:     "Workflow Admin",
		Permissions: []domain.Permission{
			{Resource: "workflow.form_instance", Action: "read", Scope: "all"},
			{Resource: "workflow.form_instance", Action: "update", Scope: "all"},
			{Resource: "workflow.form_instance", Action: "approve", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-admin",
		TenantID:               "tenant-1",
		DisplayName:            "Admin Reviewer",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-workflow-admin"},
		CreatedAt:              now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-workflow-applicant",
		TenantID: "tenant-1",
		Name:     "Workflow Applicant",
		Permissions: []domain.Permission{
			{Resource: "workflow.form_instance", Action: "submit", Scope: "self"},
			{Resource: "workflow.form_instance", Action: "read", Scope: "self"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:        "emp-applicant",
		TenantID:  "tenant-1",
		Name:      "Applicant One",
		AccountID: "acct-applicant",
		Status:    "active",
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-applicant",
		TenantID:               "tenant-1",
		DisplayName:            "Applicant One",
		EmployeeID:             "emp-applicant",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-workflow-applicant"},
		CreatedAt:              now,
	})
	originalSchema := workflowEnabledTemplateSchema("acct-admin")
	originalDesign := originalSchema["workspace_design"].(map[string]any)
	originalDesign["form_kind"] = "custom"
	originalDesign["fields"] = []map[string]any{{
		"id": "frozen_reason", "type": "textarea", "label": "原始事由", "placeholder": "填寫原始事由", "required": true,
	}}
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID:        "ft-leave",
		TenantID:  "tenant-1",
		Key:       "leave-request",
		Name:      "請假申請單",
		Schema:    originalSchema,
		CreatedAt: now,
	})
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return now.Add(time.Hour) }})
	applicantCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-applicant"}
	submitted, err := svc.Workflow().SubmitForm(applicantCtx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload:     map[string]any{"desc": "申請一天特休", "frozen_reason": "申請一天特休", "notify_account_ids": []any{"acct-admin"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	currentSchema := workflowEnabledTemplateSchema("acct-admin")
	currentDesign := currentSchema["workspace_design"].(map[string]any)
	currentDesign["form_kind"] = "hybrid"
	currentDesign["fields"] = []map[string]any{{
		"id": "current_reason", "type": "textarea", "label": "當前事由", "placeholder": "填寫當前事由", "required": true,
	}}
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID: "ft-leave", TenantID: "tenant-1", Key: "leave-request", Name: "請假申請單",
		Schema: currentSchema, Status: "published", CurrentVersion: 2, CreatedAt: now, UpdatedAt: now.Add(2 * time.Hour),
	})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}

	queue, err := svc.Workflow().ReviewQueue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.PendingReview) != 1 || len(queue.Notified) != 1 {
		t.Fatalf("expected one pending and notified item, got %+v", queue)
	}
	if queue.PendingReview[0].Title != "請假申請單" || queue.PendingReview[0].StatusText != "審核中" || queue.PendingReview[0].Desc != "表單已提交，等待審批處理。" {
		t.Fatalf("unexpected review projection: %+v", queue.PendingReview[0])
	}
	if item := queue.PendingReview[0]; item.TemplateKey != "leave-request" || item.FormKind != "custom" || len(item.Fields) != 1 || item.Fields[0].ID != "frozen_reason" || item.Instance.ID != submitted.ID {
		t.Fatalf("expected review contract from frozen template version, got %+v", item)
	}
	if queue.PendingReview[0].Time != "2026-06-10T09:00:00Z" {
		t.Fatalf("expected RFC3339 UTC review time, got %q", queue.PendingReview[0].Time)
	}

	rejected, err := svc.Workflow().RejectForm(ctx, submitted.ID, domain.RejectFormInput{Reason: "missing attachment"})
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != "rejected" || rejected.ApprovedBy != "acct-admin" {
		t.Fatalf("expected rejected form instance, got %+v", rejected)
	}
	review, _ := rejected.Payload["_review"].(map[string]any)
	if review["type"] != "reject" || review["comment"] != "missing attachment" {
		t.Fatalf("expected rejection metadata in payload, got %+v", rejected.Payload)
	}
	if review["time"] != "2026-06-10T09:00:00Z" {
		t.Fatalf("expected RFC3339 UTC review log time, got %v", review["time"])
	}

	queue, err = svc.Workflow().ReviewQueue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.PendingReview) != 0 || len(queue.AlreadyReviewed) != 1 {
		t.Fatalf("expected rejected item to move to reviewed bucket, got %+v", queue)
	}
	if got := queue.AlreadyReviewed[0].ReviewLog; len(got) != 1 || got[0].Type != "reject" || got[0].Comment != "missing attachment" {
		t.Fatalf("unexpected review log: %+v", got)
	}
	applicantNotifications, err := svc.Notifications().ListNotifications(
		applicantCtx,
		domain.NotificationListQuery{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(applicantNotifications.Items) != 1 {
		t.Fatalf("expected one rejection notification, got %+v", applicantNotifications)
	}
	if item := applicantNotifications.Items[0]; item.StatusText != "已駁回" || item.Title != "你的「請假申請單」已駁回" || item.Body != "由 Admin Reviewer 已駁回這筆申請。 審核意見：missing attachment" {
		t.Fatalf("unexpected rejection notification copy: %+v", item)
	}

	_ = store.UpsertFormInstance(context.Background(), domain.FormInstance{
		ID: "fi-missing-version", TenantID: "tenant-1", TemplateID: "ft-leave", TemplateVersionID: "ftv-missing",
		ApplicantAccountID: "acct-applicant", Status: "submitted", SubmittedAt: now, UpdatedAt: now,
	})
	if _, err := svc.Workflow().ReviewQueue(ctx); err == nil {
		t.Fatal("expected review queue to return a missing template version error")
	}
}

// TestWorkflowBulkReviewFormsReturnsPerItemResults 驗證流程批次審核表單 returns per 項目結果。
func TestWorkflowBulkReviewFormsReturnsPerItemResults(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-workflow-admin",
		TenantID: "tenant-1",
		Name:     "Workflow Admin",
		Permissions: []domain.Permission{
			{Resource: "workflow.form_instance", Action: "read", Scope: "all"},
			{Resource: "workflow.form_instance", Action: "update", Scope: "all"},
			{Resource: "workflow.form_instance", Action: "approve", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-admin",
		TenantID:               "tenant-1",
		DisplayName:            "Admin Reviewer",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-workflow-admin"},
		CreatedAt:              now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:          "acct-applicant",
		TenantID:    "tenant-1",
		DisplayName: "Applicant One",
		Status:      "active",
		CreatedAt:   now,
	})
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID:        "ft-general",
		TenantID:  "tenant-1",
		Key:       "general",
		Name:      "通用簽呈",
		Schema:    workflowEnabledTemplateSchema("acct-admin"),
		CreatedAt: now,
	})
	for _, item := range []domain.FormInstance{
		{ID: "fi-approve", TenantID: "tenant-1", TemplateID: "ft-general", ApplicantAccountID: "acct-applicant", Status: "submitted", SubmittedAt: now, UpdatedAt: now},
		{ID: "fi-return", TenantID: "tenant-1", TemplateID: "ft-general", ApplicantAccountID: "acct-applicant", Status: "submitted", SubmittedAt: now, UpdatedAt: now},
		{ID: "fi-direct-return", TenantID: "tenant-1", TemplateID: "ft-general", ApplicantAccountID: "acct-applicant", Status: "submitted", SubmittedAt: now, UpdatedAt: now},
	} {
		_ = store.UpsertFormInstance(context.Background(), item)
	}
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return now.Add(time.Hour) }})
	for _, id := range []string{"fi-approve", "fi-return", "fi-direct-return"} {
		startWorkflowRunForTest(t, svc, store, "tenant-1", id, "acct-applicant")
	}
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}

	approved, err := svc.Workflow().BulkReviewForms(ctx, domain.BulkReviewFormsInput{
		FormInstanceIDs: []string{"fi-approve", "fi-missing"},
		Action:          "approve",
		Reason:          "looks good",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(approved.Results) != 2 || !approved.Results[0].Success || approved.Results[1].Success || approved.Results[1].Code != "not_found" {
		t.Fatalf("unexpected approve batch result: %+v", approved.Results)
	}
	approveInstance, ok, err := store.GetFormInstance(context.Background(), "tenant-1", "fi-approve")
	if err != nil || !ok {
		t.Fatalf("approved instance lookup failed ok=%v err=%v", ok, err)
	}
	if approveInstance.Status != "approved" || approveInstance.ApprovedBy != "acct-admin" {
		t.Fatalf("expected approved instance, got %+v", approveInstance)
	}
	review, _ := approveInstance.Payload["_review"].(map[string]any)
	if review["type"] != "approve" || review["comment"] != "looks good" {
		t.Fatalf("expected approve review metadata, got payload=%+v", approveInstance.Payload)
	}

	returned, err := svc.Workflow().BulkReviewForms(ctx, domain.BulkReviewFormsInput{
		FormInstanceIDs: []string{"fi-return"},
		Action:          "return",
		Reason:          "please add attachment",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(returned.Results) != 1 || !returned.Results[0].Success || returned.Results[0].Action != "return" {
		t.Fatalf("unexpected return batch result: %+v", returned.Results)
	}
	returnInstance, ok, err := store.GetFormInstance(context.Background(), "tenant-1", "fi-return")
	if err != nil || !ok {
		t.Fatalf("returned instance lookup failed ok=%v err=%v", ok, err)
	}
	review, _ = returnInstance.Payload["_review"].(map[string]any)
	if returnInstance.Status != "returned" || review["type"] != "return" || review["comment"] != "please add attachment" {
		t.Fatalf("expected returned review metadata, got status=%s payload=%+v", returnInstance.Status, returnInstance.Payload)
	}

	directReturn, err := svc.Workflow().ReturnForm(ctx, "fi-direct-return", domain.ReturnFormInput{Reason: "please update approver"})
	if err != nil {
		t.Fatal(err)
	}
	review, _ = directReturn.Payload["_review"].(map[string]any)
	if directReturn.Status != "returned" || review["type"] != "return" || review["comment"] != "please update approver" {
		t.Fatalf("expected direct return metadata, got status=%s payload=%+v", directReturn.Status, directReturn.Payload)
	}
}

// TestWorkflowNotificationsFollowSubmitAndReview 驗證流程提交與審核會寫入系統通知。
func TestWorkflowNotificationsFollowSubmitAndReview(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-workflow-applicant",
		TenantID: "tenant-1",
		Name:     "Workflow Applicant",
		Permissions: []domain.Permission{
			{Resource: "workflow.form_instance", Action: "submit", Scope: "self"},
			{Resource: "workflow.form_instance", Action: "read", Scope: "self"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-workflow-admin",
		TenantID: "tenant-1",
		Name:     "Workflow Admin",
		Permissions: []domain.Permission{
			{Resource: "workflow.form_instance", Action: "read", Scope: "all"},
			{Resource: "workflow.form_instance", Action: "approve", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-applicant",
		TenantID:               "tenant-1",
		DisplayName:            "Applicant One",
		EmployeeID:             "emp-applicant",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-workflow-applicant"},
		CreatedAt:              now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-admin",
		TenantID:               "tenant-1",
		DisplayName:            "Admin Reviewer",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-workflow-admin"},
		CreatedAt:              now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:          "acct-observer",
		TenantID:    "tenant-1",
		DisplayName: "Observer",
		Status:      "active",
		CreatedAt:   now,
	})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:        "emp-applicant",
		TenantID:  "tenant-1",
		Name:      "Applicant One",
		AccountID: "acct-applicant",
		Status:    "active",
		CreatedAt: now,
	})
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID:        "ft-general",
		TenantID:  "tenant-1",
		Key:       "general",
		Name:      "通用簽呈",
		Schema:    workflowEnabledTemplateSchema("acct-admin"),
		CreatedAt: now,
	})
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return now.Add(time.Hour) }})

	submitted, err := svc.Workflow().SubmitForm(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-applicant"},
		domain.SubmitFormInput{
			TemplateKey: "general",
			Payload: map[string]any{
				"desc":               "請協助查看附件",
				"notify_account_ids": []any{"acct-observer", "acct-applicant", "acct-missing"},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	adminNotifications, err := svc.Notifications().ListNotifications(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"},
		domain.NotificationListQuery{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if adminNotifications.UnreadCount != 1 || len(adminNotifications.Items) != 1 {
		t.Fatalf("expected one approver notification, got %+v", adminNotifications)
	}
	if item := adminNotifications.Items[0]; item.StatusText != "待處理" || item.LinkURL != "/notifications?reviewId="+submitted.ID || !strings.Contains(item.Body, "Applicant One 提交了") || strings.Contains(item.Body, "Applicant One提交了") {
		t.Fatalf("unexpected submit notification: %+v", item)
	}

	observerNotifications, err := svc.Notifications().ListNotifications(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-observer"},
		domain.NotificationListQuery{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if observerNotifications.UnreadCount != 0 || len(observerNotifications.Items) != 0 {
		t.Fatalf("observer should not receive workflow pending notification, got %+v", observerNotifications)
	}

	applicantBeforeReview, err := svc.Notifications().ListNotifications(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-applicant"},
		domain.NotificationListQuery{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if applicantBeforeReview.UnreadCount != 0 || len(applicantBeforeReview.Items) != 0 {
		t.Fatalf("submit notification should not echo to applicant, got %+v", applicantBeforeReview)
	}

	if _, err := svc.Workflow().ApproveForm(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"},
		submitted.ID,
		domain.ApproveFormInput{Reason: "looks good"},
	); err != nil {
		t.Fatal(err)
	}
	applicantAfterReview, err := svc.Notifications().ListNotifications(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-applicant"},
		domain.NotificationListQuery{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if applicantAfterReview.UnreadCount != 1 || len(applicantAfterReview.Items) != 1 {
		t.Fatalf("expected one applicant review-result notification, got %+v", applicantAfterReview)
	}
	if item := applicantAfterReview.Items[0]; item.Tone != "success" || item.StatusText != "已覈準" || item.LinkURL != "/forms?applicationId="+submitted.ID || !strings.Contains(item.Body, "由 Admin Reviewer 已覈準") || !strings.Contains(item.Body, "looks good") {
		t.Fatalf("unexpected review notification: %+v", item)
	}
}

// TestWorkflowFormInstanceReadSelfScopeOnlyReturnsOwnItems 驗證流程表單實例 read self 範圍 only returns own 項目。
func TestWorkflowFormInstanceReadSelfScopeOnlyReturnsOwnItems(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-workflow-self",
		TenantID: "tenant-1",
		Name:     "Workflow Self",
		Permissions: []domain.Permission{
			{Resource: "workflow.form_instance", Action: "read", Scope: "self"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-self",
		TenantID:               "tenant-1",
		DisplayName:            "Self User",
		EmployeeID:             "emp-self",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-workflow-self"},
		CreatedAt:              now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:          "acct-other",
		TenantID:    "tenant-1",
		DisplayName: "Other User",
		Status:      "active",
		CreatedAt:   now,
	})
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID:        "ft-general",
		TenantID:  "tenant-1",
		Key:       "general",
		Name:      "通用簽呈",
		CreatedAt: now,
	})
	for _, item := range []domain.FormInstance{
		{ID: "fi-self", TenantID: "tenant-1", TemplateID: "ft-general", ApplicantAccountID: "acct-self", Status: "submitted", SubmittedAt: now, UpdatedAt: now},
		{ID: "fi-other", TenantID: "tenant-1", TemplateID: "ft-general", ApplicantAccountID: "acct-other", Status: "submitted", SubmittedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute)},
	} {
		_ = store.UpsertFormInstance(context.Background(), item)
	}
	svc := service.New(store)

	page, err := svc.Workflow().ListFormInstancePage(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-self"}, domain.FormInstanceQuery{}, domain.PageRequest{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != "fi-self" {
		t.Fatalf("expected self scope to return only own form instance, got %+v", page)
	}
}

// TestEmployeeAggregateCreatePatchAndDetail 驗證員工 aggregate create patch and detail。
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
	allowEmployeeSensitiveFieldsForPermission(t, store, now, "hr.employee.read")
	svc := service.New(store)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	created, err := svc.HR().CreateEmployee(ctx, domain.CreateEmployeeInput{
		EmployeeNo:            "E1001",
		Name:                  "Ada Chen",
		CompanyEmail:          "ada@example.com",
		OrgUnitID:             "ou-1",
		Position:              "Engineer",
		Category:              "full_time",
		HireDate:              "2026-06-01",
		BasicInfo:             map[string]any{"nationality_type": "local", "national_id": "A123456789"},
		EmploymentInfo:        map[string]any{"job_level": "senior"},
		EducationMilitaryInfo: map[string]any{"degree": "master", "school": "NTU"},
		ContactInfo:           validContactInfo(),
		InsuranceInfo:         validInsuranceInfo(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.InternalExperiences) != 1 || !created.InternalExperiences[0].Current {
		t.Fatalf("expected initial current experience, got %+v", created.InternalExperiences)
	}

	newPhone := "0911222333"
	updated, err := svc.HR().UpdateEmployee(ctx, created.ID, domain.UpdateEmployeeInput{
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

	detail, err := svc.HR().GetEmployee(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.BasicInfo["national_id"] != "A123456789" || detail.InsuranceInfo["labor_insurance_salary"] != "45800" {
		t.Fatalf("expected detail sections, got %+v", detail)
	}
}

// TestEmployeeCreateAccountPolicyCreatesAccountsAndEvents 驗證員工 create 帳號政策 creates 帳號 and 事件。
func TestEmployeeCreateAccountPolicyCreatesAccountsAndEvents(t *testing.T) {
	provisioner := &recordingIdentityProvisioner{}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "create", Scope: "all"},
	}, service.Options{IdentityProvisioner: provisioner})

	pendingInput := validEmployeeInput("E1901", "Pending Invite", "pending.invite@example.com")
	pendingInput.AccountPolicy = "create_pending_invite"
	pending, err := svc.HR().CreateEmployee(ctx, pendingInput)
	if err != nil {
		t.Fatal(err)
	}
	if pending.AccountID == "" {
		t.Fatalf("expected pending invite account link, got %+v", pending)
	}
	pendingAccount, ok, err := store.GetAccount(context.Background(), "tenant-1", pending.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || pendingAccount.EmployeeID != pending.ID || pendingAccount.Status != "pending_invite" {
		t.Fatalf("expected pending invite account linked to employee, got %+v", pendingAccount)
	}
	pendingIdentities, err := store.ListUserIdentities(context.Background(), "tenant-1", pending.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pendingIdentities) != 1 || pendingIdentities[0].Provider != domain.IdentityProviderKeycloak || pendingIdentities[0].Subject != "kc-"+pending.AccountID {
		t.Fatalf("expected pending invite keycloak identity binding, got %+v", pendingIdentities)
	}
	if pending.InternalExperiences[0].Status != "active" {
		t.Fatalf("expected initial experience to snapshot status, got %+v", pending.InternalExperiences)
	}

	activeInput := validEmployeeInput("E1902", "Active Account", "active.account@example.com")
	activeInput.AccountPolicy = "create_active"
	activeInput.BasicInfo["national_id"] = "A223456789"
	active, err := svc.HR().CreateEmployee(ctx, activeInput)
	if err != nil {
		t.Fatal(err)
	}
	activeAccount, ok, err := store.GetAccount(context.Background(), "tenant-1", active.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || activeAccount.EmployeeID != active.ID || activeAccount.Status != "active" {
		t.Fatalf("expected active account linked to employee, got %+v", activeAccount)
	}
	if len(provisioner.inputs) != 2 || !provisioner.inputs[0].SendInvite || provisioner.inputs[1].SendInvite {
		t.Fatalf("expected provisioning calls for pending invite and active accounts, got %+v", provisioner.inputs)
	}

	events, err := store.ListOutboxEvents(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if !hasBusinessOutboxEvent(events, string(domain.EventEmployeeCreated)) || !hasBusinessOutboxEvent(events, string(domain.EventEmployeeInvited)) {
		t.Fatalf("expected employee created and invited events, got %+v", events)
	}
}

// TestEmployeeAccountChangesEmitOpenFGATupleOutbox 驗證員工帳號 changes emit OpenFGA tuple outbox。
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
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-1", TenantID: "tenant-1", Name: "HQ", Path: []string{"ou-1"}, CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-2", TenantID: "tenant-1", Name: "Branch", Path: []string{"ou-2"}, CreatedAt: now})
	svc := service.New(store)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-hr"}

	input := validEmployeeInput("E2001", "Relationship Owner", "relationship.owner@example.com")
	input.AccountID = "acct-old"
	created, err := svc.HR().CreateEmployee(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	tuples, err := store.ListAuthzRelationshipTuplesForObject(context.Background(), "tenant-1", "hr.employee", created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !relationshipTupleExists(tuples, "owner", "account", "acct-old") || !relationshipTupleExists(tuples, "org", "org_unit", "ou-1") {
		t.Fatalf("expected owner and org tuple for created employee, got %+v", tuples)
	}
	orgTuples, err := store.ListAuthzRelationshipTuplesForObject(context.Background(), "tenant-1", "org_unit", "ou-1")
	if err != nil {
		t.Fatal(err)
	}
	if !relationshipTupleExists(orgTuples, "member", "account", "acct-old") {
		t.Fatalf("expected old account to be org unit member, got %+v", orgTuples)
	}

	newAccountID := "acct-new"
	if _, err := svc.HR().UpdateEmployee(ctx, created.ID, domain.UpdateEmployeeInput{AccountID: &newAccountID}); err != nil {
		t.Fatal(err)
	}
	tuples, err = store.ListAuthzRelationshipTuplesForObject(context.Background(), "tenant-1", "hr.employee", created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !relationshipTupleExists(tuples, "owner", "account", "acct-new") || relationshipTupleExists(tuples, "owner", "account", "acct-old") || !relationshipTupleExists(tuples, "org", "org_unit", "ou-1") {
		t.Fatalf("expected owner tuple to move to new account, got %+v", tuples)
	}
	orgTuples, err = store.ListAuthzRelationshipTuplesForObject(context.Background(), "tenant-1", "org_unit", "ou-1")
	if err != nil {
		t.Fatal(err)
	}
	if !relationshipTupleExists(orgTuples, "member", "account", "acct-new") || relationshipTupleExists(orgTuples, "member", "account", "acct-old") {
		t.Fatalf("expected org unit member tuple to move to new account, got %+v", orgTuples)
	}

	newOrgUnitID := "ou-2"
	if _, err := svc.HR().UpdateEmployee(ctx, created.ID, domain.UpdateEmployeeInput{OrgUnitID: &newOrgUnitID}); err != nil {
		t.Fatal(err)
	}
	tuples, err = store.ListAuthzRelationshipTuplesForObject(context.Background(), "tenant-1", "hr.employee", created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !relationshipTupleExists(tuples, "org", "org_unit", "ou-2") || relationshipTupleExists(tuples, "org", "org_unit", "ou-1") {
		t.Fatalf("expected employee org tuple to move to new org unit, got %+v", tuples)
	}
	oldOrgTuples, err := store.ListAuthzRelationshipTuplesForObject(context.Background(), "tenant-1", "org_unit", "ou-1")
	if err != nil {
		t.Fatal(err)
	}
	newOrgTuples, err := store.ListAuthzRelationshipTuplesForObject(context.Background(), "tenant-1", "org_unit", "ou-2")
	if err != nil {
		t.Fatal(err)
	}
	if relationshipTupleExists(oldOrgTuples, "member", "account", "acct-new") || !relationshipTupleExists(newOrgTuples, "member", "account", "acct-new") {
		t.Fatalf("expected org unit member tuple to move to new department, old=%+v new=%+v", oldOrgTuples, newOrgTuples)
	}
	events, err := store.ListOutboxEvents(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if !hasBusinessOutboxEvent(events, string(domain.EventOpenFGARelationshipWrite)) || !hasBusinessOutboxEvent(events, string(domain.EventOpenFGARelationshipDelete)) {
		t.Fatalf("expected OpenFGA write/delete outbox events, got %+v", events)
	}
}

// TestOpenFGABackfillIsIdempotent 驗證 OpenFGA backfill 可重跑且不重複追加 outbox。
func TestOpenFGABackfillIsIdempotent(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-manager", TenantID: "tenant-1", EmployeeID: "emp-manager", Status: "active", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-user", TenantID: "tenant-1", EmployeeID: "emp-user", Status: "active", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-disabled", TenantID: "tenant-1", Status: "disabled", CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-root", TenantID: "tenant-1", Name: "Root", Path: []string{"ou-root"}, CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-child", TenantID: "tenant-1", Name: "Child", ParentID: "ou-root", Path: []string{"ou-root", "ou-child"}, CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-manager", TenantID: "tenant-1", Name: "Manager", OrgUnitID: "ou-root", AccountID: "acct-manager", Status: "active", CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-user", TenantID: "tenant-1", Name: "User", OrgUnitID: "ou-child", AccountID: "acct-user", ManagerEmployeeID: "emp-manager", Status: "active", CreatedAt: now})
	_ = store.UpsertUserGroup(context.Background(), domain.UserGroup{ID: "ug-1", TenantID: "tenant-1", Name: "Group", MemberAccountIDs: []string{"acct-user"}, CreatedAt: now})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})

	first, err := svc.OpenFGABackfillTuples(context.Background(), service.OpenFGABackfillInput{TenantID: "tenant-1"})
	if err != nil {
		t.Fatal(err)
	}
	if first.DesiredTuples == 0 || first.CreatedTuples != first.DesiredTuples || first.OutboxEvents != first.CreatedTuples {
		t.Fatalf("unexpected first backfill result: %+v", first)
	}
	employeeTuples, err := store.ListAuthzRelationshipTuplesForObject(context.Background(), "tenant-1", "hr.employee", "emp-user")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		relation    string
		subjectType string
		subjectID   string
	}{
		{relation: "owner", subjectType: "account", subjectID: "acct-user"},
		{relation: "manager", subjectType: "account", subjectID: "acct-manager"},
		{relation: "org", subjectType: "org_unit", subjectID: "ou-child"},
	} {
		if !relationshipTupleExists(employeeTuples, want.relation, want.subjectType, want.subjectID) {
			t.Fatalf("expected employee tuple %+v in %+v", want, employeeTuples)
		}
	}
	tenantTuples, err := store.ListAuthzRelationshipTuplesForObject(context.Background(), "tenant-1", "tenant", "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if !relationshipTupleExists(tenantTuples, "member", "account", "acct-user") || relationshipTupleExists(tenantTuples, "member", "account", "acct-disabled") {
		t.Fatalf("expected only active tenant members, got %+v", tenantTuples)
	}
	events, err := store.ListOutboxEvents(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != first.OutboxEvents {
		t.Fatalf("expected one outbox event per created tuple, result=%+v events=%d", first, len(events))
	}

	second, err := svc.OpenFGABackfillTuples(context.Background(), service.OpenFGABackfillInput{TenantID: "tenant-1"})
	if err != nil {
		t.Fatal(err)
	}
	if second.CreatedTuples != 0 || second.SkippedTuples != second.DesiredTuples {
		t.Fatalf("expected second backfill to skip all tuples, got %+v", second)
	}
	eventsAfterSecond, err := store.ListOutboxEvents(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(eventsAfterSecond) != len(events) {
		t.Fatalf("expected second backfill not to append outbox events, before=%d after=%d", len(events), len(eventsAfterSecond))
	}
}

// TestEmployeeCreateRejectsDuplicateUniqueFields 驗證員工 create rejects duplicate unique 欄位。
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
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-1", TenantID: "tenant-1", Name: "HQ", Path: []string{"ou-1"}, CreatedAt: now})
	store.UpsertEmployee(context.Background(), domain.Employee{
		ID:            "emp-existing",
		TenantID:      "tenant-1",
		EmployeeNo:    "E1001",
		Name:          "Existing Employee",
		CompanyEmail:  "duplicate@example.com",
		PersonalEmail: "personal.duplicate@example.com",
		AccountID:     "acct-linked",
		Status:        "active",
		BasicInfo:     map[string]any{"national_id": "A123456789"},
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	svc := service.New(store)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	input := validEmployeeInput("E1001", "Duplicate Employee", "duplicate@example.com")
	input.AccountID = "acct-linked"
	input.PersonalEmail = "PERSONAL.DUPLICATE@example.com"
	_, err := svc.HR().CreateEmployee(ctx, input)
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
	for _, field := range []string{"employee_no", "company_email", "personal_email", "account_id", "national_id"} {
		if codes[field] != "unique" {
			t.Fatalf("expected %s unique error, got %+v", field, appErr.FieldErrors)
		}
	}
}

// TestEmployeeStatusTransitionHandlesEmptyEmploymentInfo 驗證員工狀態轉換 handles 空值任職 info。
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
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	leaveTarget := domain.Employee{ID: "emp-leave", TenantID: "tenant-1", Name: "Leave Target", CompanyEmail: "leave.target@example.com", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now}
	_ = store.UpsertEmployee(context.Background(), leaveTarget)
	_, err := svc.HR().TransitionEmployeeStatus(ctx, leaveTarget.ID, domain.StatusTransitionInput{Status: "leave_suspended"})
	if appErr, ok := domain.AsAppError(err); !ok || appErr.Code != "validation_failed" {
		t.Fatalf("expected missing leave dates validation, got %v", err)
	}
	_, err = svc.HR().TransitionEmployeeStatus(ctx, leaveTarget.ID, domain.StatusTransitionInput{Status: "leave_suspended", StartDate: "2026-06-10", EndDate: "2026-06-20"})
	if appErr, ok := domain.AsAppError(err); !ok || appErr.Code != "validation_failed" || !fieldErrorsContain(appErr.FieldErrors, "reason") {
		t.Fatalf("expected missing leave reason validation, got %v", err)
	}

	resignTarget := domain.Employee{ID: "emp-resign-target", TenantID: "tenant-1", Name: "Resign Target", CompanyEmail: "resign.target@example.com", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now}
	_ = store.UpsertEmployee(context.Background(), resignTarget)
	if resignTarget.EmploymentInfo != nil {
		t.Fatalf("test setup expected empty employment info, got %+v", resignTarget.EmploymentInfo)
	}
	transitioned, err := svc.HR().TransitionEmployeeStatus(ctx, resignTarget.ID, domain.StatusTransitionInput{
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

// TestEmployeeStatusWritesRequireTransitionPath 驗證員工狀態 writes require 轉換 path。
func TestEmployeeStatusWritesRequireTransitionPath(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "update", Scope: "all"},
		{Resource: "hr.employee", Action: "update_status", Scope: "all"},
	})
	now := time.Now().UTC()
	employee := domain.Employee{ID: "emp-status-write", TenantID: "tenant-1", Name: "Status Write", CompanyEmail: "status.write@example.com", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now}
	if err := store.UpsertEmployee(context.Background(), employee); err != nil {
		t.Fatal(err)
	}

	status := "resigned"
	if _, err := svc.HR().UpdateEmployee(ctx, employee.ID, domain.UpdateEmployeeInput{Status: &status}); err == nil {
		t.Fatal("expected patch status to require status-transition")
	}
	if _, err := svc.HR().UpdateEmployeeStatus(ctx, employee.ID, "leave_suspended"); err == nil {
		t.Fatal("expected direct leave_suspended status to require status-transition")
	}
}

// TestSyncEHRMSEmployeesCreatesEmployeesAndDepartments 驗證 eHRMS 員工同步會更新既有部門並建立員工。
func TestSyncEHRMSEmployeesCreatesEmployeesAndDepartments(t *testing.T) {
	rows := []domain.EHRMSEmployeeRecord{{
		"員工編號":     "IKM001",
		"中文姓名":     "測試員工",
		"英文姓名":     "Test Employee",
		"email":    "test.employee@ikala.ai",
		"到職日期":     "2026/06/01",
		"在職狀態":     "在職",
		"部門代碼":     "M0101",
		"部門中文名稱":   "Nexus",
		"職務代碼":     "0704",
		"職務中文名稱":   "工程師",
		"身份類別名稱":   "一般員工",
		"身份證號":     "A123456789",
		"學校名稱(中文)": "Nexus University",
	}}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
		{Resource: "hr.employee", Action: "read", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{rows: rows}})
	seedOrgUnitCodes(t, store, "tenant-1", "M0101")

	result, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{})
	if err != nil {
		t.Fatalf("%v result=%+v", err, result)
	}
	if result.Fetched != 1 || result.Created != 1 || result.Updated != 0 || result.DepartmentsUpserted != 1 || result.PositionsUpserted != 1 || result.Mode != "upsert" {
		t.Fatalf("unexpected eHRMS sync result: %+v", result)
	}
	unit := mustOrgUnitByCode(t, store, "tenant-1", "M0101")
	if unit.Name != "Nexus" {
		t.Fatalf("expected eHRMS department to be upserted, unit=%+v", unit)
	}
	position := mustPositionByCode(t, store, "tenant-1", "0704")
	employee, ok, err := store.GetEmployeeByEmployeeNo(context.Background(), "tenant-1", "IKM001")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected eHRMS employee to be created")
	}
	if employee.Name != "測試員工" || employee.CompanyEmail != "test.employee@ikala.ai" || employee.OrgUnitID != unit.ID || employee.Position != "工程師" || employee.PositionID != position.ID || employee.Status != "active" || employee.Category != "full_time" {
		t.Fatalf("unexpected eHRMS employee mapping: %+v", employee)
	}
	if employee.AccountID == "" {
		t.Fatal("expected eHRMS sync to create pending_invite account from email")
	}
	account, ok, err := store.GetAccount(context.Background(), "tenant-1", employee.AccountID)
	if err != nil || !ok || account.Email != "test.employee@ikala.ai" || account.Status != "pending_invite" {
		t.Fatalf("expected pending_invite account for email SSO invite, ok=%v account=%+v err=%v", ok, account, err)
	}
	if employee.BasicInfo["national_id"] != "A123456789" || employee.EmploymentInfo["position"] != "工程師" || employee.EducationMilitaryInfo["school_name"] != "Nexus University" {
		t.Fatalf("expected eHRMS profile sections to be preserved, got basic=%+v employment=%+v education=%+v", employee.BasicInfo, employee.EmploymentInfo, employee.EducationMilitaryInfo)
	}
	if len(employee.InternalExperiences) != 1 || !employee.InternalExperiences[0].Current || employee.InternalExperiences[0].OrgUnitID != unit.ID || employee.InternalExperiences[0].Reason != "eHRMS sync" {
		t.Fatalf("expected initial eHRMS internal experience to be persisted, got %+v", employee.InternalExperiences)
	}
}

// TestSyncEHRMSEmployeesPersistsDepartmentChangeExperience verifies department changes are recorded once.
func TestSyncEHRMSEmployeesPersistsDepartmentChangeExperience(t *testing.T) {
	client := &fakeEHRMSClient{rows: []domain.EHRMSEmployeeRecord{{
		"員工編號":   "EHRMS-HISTORY-001",
		"中文姓名":   "歷程測試員工",
		"到職日期":   "2026/06/01",
		"在職狀態":   "在職",
		"部門代碼":   "C01",
		"部門中文名稱": "Corporate",
		"職務代碼":   "0704",
		"職務中文名稱": "工程師",
	}}}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
		{Resource: "hr.employee", Action: "read", Scope: "all"},
	}, service.Options{EHRMSClient: client})
	seedOrgUnitCodes(t, store, ctx.TenantID, "C01", "C02")

	if _, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{}); err != nil {
		t.Fatal(err)
	}
	originalUnit := mustOrgUnitByCode(t, store, ctx.TenantID, "C01")

	client.rows[0]["部門代碼"] = "C02"
	client.rows[0]["部門中文名稱"] = "People"
	if _, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{}); err != nil {
		t.Fatal(err)
	}
	updatedUnit := mustOrgUnitByCode(t, store, ctx.TenantID, "C02")
	employee, ok, err := store.GetEmployeeByEmployeeNo(context.Background(), ctx.TenantID, "EHRMS-HISTORY-001")
	if err != nil || !ok {
		t.Fatalf("expected synced employee, ok=%v err=%v", ok, err)
	}
	if len(employee.InternalExperiences) != 2 {
		t.Fatalf("expected department change to persist a second internal experience, got %+v", employee.InternalExperiences)
	}
	previous := employee.InternalExperiences[0]
	current := employee.InternalExperiences[1]
	if previous.Current || previous.EndDate == nil || previous.OrgUnitID != originalUnit.ID {
		t.Fatalf("expected previous department experience to be closed, got %+v", previous)
	}
	if !current.Current || current.EndDate != nil || current.OrgUnitID != updatedUnit.ID || current.Reason != "eHRMS sync" {
		t.Fatalf("expected current department experience to be persisted, got %+v", current)
	}

	if _, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{}); err != nil {
		t.Fatal(err)
	}
	employee, ok, err = store.GetEmployeeByEmployeeNo(context.Background(), ctx.TenantID, "EHRMS-HISTORY-001")
	if err != nil || !ok {
		t.Fatalf("expected synced employee after idempotency check, ok=%v err=%v", ok, err)
	}
	if len(employee.InternalExperiences) != 2 {
		t.Fatalf("expected unchanged eHRMS sync not to duplicate internal experiences, got %+v", employee.InternalExperiences)
	}
}

// TestSyncEHRMSEmployeesRepairsLegacyRawCatalogReferences verifies an upsert rewrites old code-based references.
func TestSyncEHRMSEmployeesRepairsLegacyRawCatalogReferences(t *testing.T) {
	rows := []domain.EHRMSEmployeeRecord{{
		"員工編號":   "LEGACY001",
		"中文姓名":   "Legacy Employee",
		"在職狀態":   "在職",
		"部門代碼":   "C01",
		"部門中文名稱": "Corporate",
		"職務代碼":   "0704",
		"職務中文名稱": "Engineer",
	}}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
		{Resource: "hr.employee", Action: "read", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{rows: rows}})
	seedOrgUnitCodes(t, store, ctx.TenantID, "C01")
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID:               "emp-legacy-raw-refs",
		TenantID:         ctx.TenantID,
		EmployeeNo:       "LEGACY001",
		Name:             "Legacy Employee",
		OrgUnitID:        "C01",
		PositionID:       "0704",
		Position:         "Engineer",
		Status:           "active",
		EmploymentStatus: "active",
		EmploymentInfo:   map[string]any{"org_unit_id": "C01", "org_unit_code": "C01", "position_id": "0704", "position_code": "0704"},
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{Mode: "upsert"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 1 || result.Failed != 0 {
		t.Fatalf("expected one repaired employee, got %+v", result)
	}
	unit := mustOrgUnitByCode(t, store, ctx.TenantID, "C01")
	position := mustPositionByCode(t, store, ctx.TenantID, "0704")
	updated, ok, err := store.GetEmployeeByEmployeeNo(context.Background(), ctx.TenantID, "LEGACY001")
	if err != nil || !ok {
		t.Fatalf("expected repaired employee, ok=%v err=%v", ok, err)
	}
	if updated.OrgUnitID != unit.ID || updated.PositionID != position.ID {
		t.Fatalf("expected tenant-scoped references, employee=%+v unit=%+v position=%+v", updated, unit, position)
	}
	if updated.EmploymentInfo["org_unit_id"] != unit.ID || updated.EmploymentInfo["position_id"] != position.ID {
		t.Fatalf("expected profile projection to use tenant-scoped references, got %+v", updated.EmploymentInfo)
	}
}

// TestSyncEHRMSEmployeesReusesSameTenantLegacyCatalogIDs verifies upgrades do not duplicate legacy catalogs.
func TestSyncEHRMSEmployeesReusesSameTenantLegacyCatalogIDs(t *testing.T) {
	rows := []domain.EHRMSEmployeeRecord{{
		"員工編號":   "LEGACY002",
		"中文姓名":   "Legacy Catalog Employee",
		"在職狀態":   "在職",
		"部門代碼":   "C01",
		"部門中文名稱": "Corporate",
		"職務代碼":   "0704",
		"職務中文名稱": "Engineer",
	}}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
		{Resource: "hr.employee", Action: "read", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{rows: rows}})
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := store.UpsertOrgUnit(context.Background(), domain.OrgUnit{
		ID: "C01", TenantID: ctx.TenantID, Code: "C01", Name: "Legacy Corporate", Path: []string{"C01"}, Source: "ehrms", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPosition(context.Background(), domain.Position{
		ID: "0704", TenantID: ctx.TenantID, Code: "0704", Name: "Legacy Engineer", Status: string(domain.PositionStatusActive), Source: "ehrms", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-legacy-catalog", TenantID: ctx.TenantID, EmployeeNo: "LEGACY002", Name: "Legacy Catalog Employee",
		OrgUnitID: "C01", PositionID: "0704", Position: "Engineer", Status: "active", EmploymentStatus: "active",
		EmploymentInfo: map[string]any{"org_unit_id": "C01", "org_unit_code": "C01", "position_id": "0704", "position_code": "0704"},
		CreatedAt:      now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{Mode: "upsert"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 1 || result.Failed != 0 {
		t.Fatalf("expected the legacy employee to update, got %+v", result)
	}
	unit := mustOrgUnitByCode(t, store, ctx.TenantID, "C01")
	position := mustPositionByCode(t, store, ctx.TenantID, "0704")
	if unit.ID != "C01" || position.ID != "0704" {
		t.Fatalf("expected same-tenant legacy IDs to remain compatible, unit=%+v position=%+v", unit, position)
	}
	updated, ok, err := store.GetEmployeeByEmployeeNo(context.Background(), ctx.TenantID, "LEGACY002")
	if err != nil || !ok || updated.OrgUnitID != unit.ID || updated.PositionID != position.ID {
		t.Fatalf("expected employee references to reuse legacy catalog IDs, ok=%v employee=%+v err=%v", ok, updated, err)
	}
	units, err := store.ListOrgUnits(context.Background(), ctx.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	matchingUnits := 0
	for _, item := range units {
		if item.Code == "C01" {
			matchingUnits++
		}
	}
	if matchingUnits != 1 {
		t.Fatalf("expected one org row for business code C01, got %d", matchingUnits)
	}
}

// TestSyncEHRMSEmployeesRequiresImportPermissionEvenWhenApproved 驗證 eHRMS 員工 requires import 權限 even when approved。
func TestSyncEHRMSEmployeesRequiresImportPermissionEvenWhenApproved(t *testing.T) {
	rows := []domain.EHRMSEmployeeRecord{{
		"員工編號":     "IKM-NOAUTH",
		"中文姓名":     "未授權同步",
		"到職日期":     "2026/06/01",
		"在職狀態":     "在職",
		"部門代碼":     "M0101",
		"部門中文名稱":   "Nexus",
		"職務中文名稱":   "工程師",
		"身份類別名稱":   "一般員工",
		"身份證號":     "C123456789",
		"學校名稱(中文)": "Nexus University",
	}}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "read", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{rows: rows}})

	_, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{})
	if err == nil {
		t.Fatal("expected eHRMS sync to require hr.employee.import permission")
	}
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Status != 403 || appErr.ReasonCode != "button_denied" {
		t.Fatalf("expected button_denied forbidden error, got %v", err)
	}
	employees, err := store.ListEmployees(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(employees) != 0 {
		t.Fatalf("unauthorized eHRMS sync should not write employees, got %+v", employees)
	}
}

// TestSyncEHRMSEmployeesClearsLocalEmailWhenUpstreamEmpty 驗證上游無 email 時以 EHRMS 覆蓋清空本機 email。
func TestSyncEHRMSEmployeesClearsLocalEmailWhenUpstreamEmpty(t *testing.T) {
	rows := []domain.EHRMSEmployeeRecord{{
		"員工編號":   "IKM002",
		"中文姓名":   "更新員工",
		"到職日期":   "2026/06/01",
		"在職狀態":   "留職停薪",
		"部門代碼":   "M0202",
		"部門中文名稱": "People",
		"職務代碼":   "0801",
		"職務中文名稱": "專員",
		"身份類別名稱": "時薪員工",
		"身份證號":   "B123456789",
	}}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{rows: rows}})
	seedOrgUnitCodes(t, store, "tenant-1", "M0202")
	now := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID:               "emp-existing",
		TenantID:         "tenant-1",
		EmployeeNo:       "IKM002",
		Name:             "舊員工",
		CompanyEmail:     "local@example.com",
		Status:           "active",
		EmploymentStatus: "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{})
	if err != nil {
		t.Fatalf("%v result=%+v", err, result)
	}
	if result.Created != 0 || result.Updated != 1 {
		t.Fatalf("expected one eHRMS update, got %+v", result)
	}
	employee, ok, err := store.GetEmployee(context.Background(), "tenant-1", "emp-existing")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected existing employee to remain")
	}
	if employee.Name != "更新員工" || employee.CompanyEmail != "" || employee.Status != "leave_suspended" || employee.Category != "part_time" {
		t.Fatalf("unexpected updated employee: %+v", employee)
	}
	if employee.AccountID != "" {
		t.Fatalf("expected no account when upstream email is empty, got account_id=%s", employee.AccountID)
	}
}

// TestSyncEHRMSEmployeesOverwritesLocalEmailAndCreatesPendingInvite 驗證上游 email 覆蓋本機並建立 pending_invite + Keycloak invite。
func TestSyncEHRMSEmployeesOverwritesLocalEmailAndCreatesPendingInvite(t *testing.T) {
	provisioner := &recordingIdentityProvisioner{}
	rows := []domain.EHRMSEmployeeRecord{{
		"員工編號":   "IKM003",
		"中文姓名":   "覆蓋員工",
		"email":  "ehrms@ikala.ai",
		"到職日期":   "2026/06/01",
		"在職狀態":   "在職",
		"部門代碼":   "M0303",
		"部門中文名稱": "Ops",
		"職務代碼":   "0704",
		"職務中文名稱": "工程師",
		"身份類別名稱": "一般員工",
		"身份證號":   "C123456789",
	}}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{rows: rows}, IdentityProvisioner: provisioner})
	seedOrgUnitCodes(t, store, "tenant-1", "M0303")
	now := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID:               "emp-overwrite",
		TenantID:         "tenant-1",
		EmployeeNo:       "IKM003",
		Name:             "舊員工",
		CompanyEmail:     "local@example.com",
		Status:           "active",
		EmploymentStatus: "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{})
	if err != nil {
		t.Fatalf("%v result=%+v", err, result)
	}
	if result.Updated != 1 {
		t.Fatalf("expected one update, got %+v", result)
	}
	employee, ok, err := store.GetEmployee(context.Background(), "tenant-1", "emp-overwrite")
	if err != nil || !ok {
		t.Fatalf("expected employee, ok=%v err=%v", ok, err)
	}
	if employee.CompanyEmail != "ehrms@ikala.ai" || employee.AccountID == "" {
		t.Fatalf("expected EHRMS email overwrite and account, got %+v", employee)
	}
	account, ok, err := store.GetAccount(context.Background(), "tenant-1", employee.AccountID)
	if err != nil || !ok || account.Email != "ehrms@ikala.ai" || account.Status != "pending_invite" {
		t.Fatalf("expected pending_invite account, ok=%v account=%+v err=%v", ok, account, err)
	}
	if len(provisioner.inputs) != 1 || !provisioner.inputs[0].SendInvite || provisioner.inputs[0].Email != "ehrms@ikala.ai" {
		t.Fatalf("expected Keycloak invite provisioning, got %+v", provisioner.inputs)
	}
	identities, err := store.ListUserIdentities(context.Background(), "tenant-1", employee.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(identities) != 1 || identities[0].Email != "ehrms@ikala.ai" {
		t.Fatalf("expected keycloak identity binding, got %+v", identities)
	}
}

// TestSyncEHRMSEmployeesSkipsEmployeesWithoutLocalPosition only upserts rows whose job code exists in positions.
func TestSyncEHRMSEmployeesSkipsEmployeesWithoutLocalPosition(t *testing.T) {
	rows := []domain.EHRMSEmployeeRecord{
		{
			"員工編號": "E1", "中文姓名": "有崗位", "email": "with-pos@ikala.ai",
			"到職日期": "2026/06/01", "在職狀態": "在職",
			"部門代碼": "C01", "部門中文名稱": "Corporate",
			"職務代碼": "0704", "職務中文名稱": "工程師", "身份類別名稱": "一般員工",
		},
		{
			"員工編號": "E2", "中文姓名": "無崗位", "email": "no-pos@ikala.ai",
			"到職日期": "2026/06/01", "在職狀態": "在職",
			"部門代碼": "C01", "部門中文名稱": "Corporate",
			"職務代碼": "9999", "職務中文名稱": "未知職", "身份類別名稱": "一般員工",
		},
		{
			"員工編號": "E3", "中文姓名": "缺職務碼", "email": "empty-pos@ikala.ai",
			"到職日期": "2026/06/01", "在職狀態": "在職",
			"部門代碼": "C01", "部門中文名稱": "Corporate",
			"職務中文名稱": "只有名稱", "身份類別名稱": "一般員工",
		},
	}
	positionRows := []domain.EHRMSPositionRecord{
		{"job_code": "0704", "job_title_zh": "工程師", "job_title_en": "Engineer"},
	}
	departmentRows := []domain.EHRMSDepartmentRecord{
		{"code": "C01", "name": "Corporate", "closed": "false"},
	}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{
		rows: rows, positionRows: positionRows, departmentRows: departmentRows,
	}})
	seedOrgUnitCodes(t, store, "tenant-1", "C01")

	result, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 3 || result.Created != 1 || result.Updated != 0 || result.Skipped != 2 || result.Failed != 0 {
		t.Fatalf("unexpected sync result: %+v", result)
	}
	if _, ok, err := store.GetEmployeeByEmployeeNo(context.Background(), "tenant-1", "E1"); err != nil || !ok {
		t.Fatalf("expected employee with local position to sync, ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.GetEmployeeByEmployeeNo(context.Background(), "tenant-1", "E2"); err != nil || ok {
		t.Fatalf("expected missing-position employee to be skipped, ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.GetEmployeeByEmployeeNo(context.Background(), "tenant-1", "E3"); err != nil || ok {
		t.Fatalf("expected empty position_code employee to be skipped, ok=%v err=%v", ok, err)
	}
	skippedNotFound, skippedRequired := false, false
	for _, item := range result.Results {
		if item.Action != "skipped" {
			continue
		}
		if item.Code == "not_found" {
			skippedNotFound = true
		}
		if item.Code == "required" {
			skippedRequired = true
		}
	}
	if !skippedNotFound || !skippedRequired {
		t.Fatalf("expected skipped not_found and required results, got %+v", result.Results)
	}
}

// TestSyncEHRMSEmployeesFailsInvalidRowsAndWritesValidOnes verifies row failures without aborting the batch.
func TestSyncEHRMSEmployeesSkipsInvalidRowsAndWritesValidOnes(t *testing.T) {
	rows := []domain.EHRMSEmployeeRecord{
		{
			"員工編號":   "E1",
			"中文姓名":   "員工一",
			"email":  "dup@ikala.ai",
			"到職日期":   "2026/06/01",
			"在職狀態":   "在職",
			"部門代碼":   "C01",
			"部門中文名稱": "Corporate",
			"職務代碼":   "0704",
			"職務中文名稱": "工程師",
			"身份類別名稱": "一般員工",
			"身份證號":   "A123456789",
		},
		{
			"員工編號":   "E2",
			"中文姓名":   "員工二",
			"email":  "dup@ikala.ai",
			"到職日期":   "2026/06/01",
			"在職狀態":   "在職",
			"部門代碼":   "C01",
			"部門中文名稱": "Corporate",
			"職務代碼":   "0901",
			"職務中文名稱": "經理",
			"身份類別名稱": "一般員工",
			"身份證號":   "A223456789",
		},
		{
			"員工編號":   "",
			"中文姓名":   "缺編號",
			"email":  "missing-no@ikala.ai",
			"到職日期":   "2026/06/01",
			"在職狀態":   "在職",
			"部門代碼":   "C01",
			"部門中文名稱": "Corporate",
			"職務代碼":   "0704",
			"職務中文名稱": "工程師",
			"身份類別名稱": "一般員工",
			"身份證號":   "A323456789",
		},
		{
			"員工編號":   "E3",
			"中文姓名":   "員工三",
			"email":  "ok@ikala.ai",
			"到職日期":   "2026/06/01",
			"在職狀態":   "在職",
			"部門代碼":   "C01",
			"部門中文名稱": "Corporate",
			"職務代碼":   "0704",
			"職務中文名稱": "工程師",
			"身份類別名稱": "一般員工",
			"身份證號":   "A423456789",
		},
	}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{rows: rows}})
	seedOrgUnitCodes(t, store, "tenant-1", "C01")

	result, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{})
	if err != nil {
		t.Fatalf("expected invalid rows to be skipped, got %v", err)
	}
	if result.Fetched != 4 || result.Created != 2 || result.Updated != 0 || result.Skipped != 0 || result.Failed != 2 {
		t.Fatalf("unexpected sync result: %+v", result)
	}
	foundDup := false
	foundRequired := false
	for _, rowErr := range result.RowErrors {
		if rowErr.Field == "company_email" && rowErr.Code == "duplicate_in_file" {
			foundDup = true
		}
		if rowErr.Field == "employee_no" && rowErr.Code == "required" {
			foundRequired = true
		}
	}
	if !foundDup || !foundRequired {
		t.Fatalf("expected duplicate_in_file and required row errors, got %+v", result.RowErrors)
	}
	failed := 0
	for _, item := range result.Results {
		if item.Action == "failed" {
			failed++
		}
	}
	if failed != 2 {
		t.Fatalf("expected 2 failed results, got %+v", result.Results)
	}
	employees, err := store.ListEmployees(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(employees) != 2 {
		t.Fatalf("expected valid rows to be written, got %+v", employees)
	}
	nos := map[string]bool{}
	for _, employee := range employees {
		nos[employee.EmployeeNo] = true
	}
	if !nos["E1"] || !nos["E3"] {
		t.Fatalf("expected E1 and E3 to be written, got %+v", employees)
	}
}

// TestSyncEHRMSEmployeesHidesUpstreamFetchDetails 驗證 eHRMS 員工 hides upstream fetch details。
func TestSyncEHRMSEmployeesHidesUpstreamFetchDetails(t *testing.T) {
	_, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{err: errors.New("upstream 500: token=secret-value")}})

	_, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{})
	if err == nil {
		t.Fatal("expected eHRMS fetch failure")
	}
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Code != "bad_request" || appErr.Message != "fetch eHRMS employees failed" {
		t.Fatalf("expected sanitized bad_request, got %v", err)
	}
	if strings.Contains(err.Error(), "secret-value") || strings.Contains(err.Error(), "upstream 500") {
		t.Fatalf("eHRMS fetch error leaked upstream detail: %v", err)
	}
}

// TestSyncEHRMSEmployeesMapsEnglishAliasesToDatabase 驗證 eHRMS 英文別名欄位會落到對應資料表。
func TestSyncEHRMSEmployeesMapsEnglishAliasesToDatabase(t *testing.T) {
	rows := []domain.EHRMSEmployeeRecord{{
		"emp_id":          "IK0028",
		"gender":          "女",
		"name_en":         "Test IK0028",
		"name_zh":         "測試員工IK0028",
		"birthday":        "1990/01/01",
		"job_code":        "0901",
		"dept_code":       "M010102",
		"education":       "-",
		"hire_date":       "2008/07/01",
		"last_name":       "IK0028",
		"first_name":      "Test",
		"shift_attr":      "固定班",
		"shift_name":      "正常班",
		"leave_group":     "-",
		"national_id":     "A580392764",
		"nationality":     "中華民國",
		"passport_no":     "-",
		"school_zh":       "Nexus University",
		"work_status":     "離職",
		"dept_name_en":    "MarTech Div.(TW)-KOL Radar E2E-Sales 2(已關閉)",
		"dept_name_zh":    "行銷科技事業處-網紅行銷事業-業務二(已關閉)",
		"job_title_en":    "Manager",
		"job_title_zh":    "經理",
		"identity_type":   "一般員工",
		"probation_end":   "2008/09/28",
		"clock_required":  "是",
		"direct_indirect": "間接",
		"seniority_start": "2008/07/01",
	}}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
		{Resource: "hr.employee", Action: "read", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{rows: rows}})
	seedOrgUnitCodes(t, store, "tenant-1", "M010102")

	result, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 || result.DepartmentsUpserted != 1 || result.PositionsUpserted != 1 {
		t.Fatalf("unexpected eHRMS sync result: %+v", result)
	}

	unit := mustOrgUnitByCode(t, store, "tenant-1", "M010102")
	if unit.Code != "M010102" {
		t.Fatalf("expected org unit from dept_code, got unit=%+v", unit)
	}
	if unit.Name != "行銷科技事業處-網紅行銷事業-業務二" || unit.NameEN != "MarTech Div.(TW)-KOL Radar E2E-Sales 2" || !unit.Closed || unit.Source != "ehrms" || unit.UpdatedAt.IsZero() {
		t.Fatalf("expected eHRMS org metadata with closed status and cleaned name, got %+v", unit)
	}

	employee, ok, err := store.GetEmployeeByEmployeeNo(context.Background(), "tenant-1", "IK0028")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected employee to be created")
	}
	if employee.Name != "測試員工IK0028" || employee.OrgUnitID != unit.ID || employee.Position != "經理" || employee.Status != "resigned" {
		t.Fatalf("unexpected employee hot fields: %+v", employee)
	}
	if employee.PositionID == "" {
		t.Fatalf("expected position to be linked in positions table, employee=%+v", employee)
	}
	if employee.BasicInfo["gender"] != "女" || employee.BasicInfo["national_id"] != "A580392764" {
		t.Fatalf("unexpected basic_info: %+v", employee.BasicInfo)
	}
	if employee.BasicInfo["passport_no"] != nil && employee.BasicInfo["passport_no"] != "" {
		t.Fatalf("expected placeholder passport_no to be empty, got %+v", employee.BasicInfo["passport_no"])
	}
	if employee.EmploymentInfo["org_unit_name"] == nil || employee.EmploymentInfo["position_code"] != "0901" {
		t.Fatalf("unexpected employment_info: %+v", employee.EmploymentInfo)
	}
	if employee.EmploymentInfo["leave_group"] != nil && employee.EmploymentInfo["leave_group"] != "" {
		t.Fatalf("expected placeholder leave_group to be empty, got %+v", employee.EmploymentInfo["leave_group"])
	}
	if employee.EducationMilitaryInfo["highest_education"] != nil && employee.EducationMilitaryInfo["highest_education"] != "" {
		t.Fatalf("expected placeholder education to be empty, got %+v", employee.EducationMilitaryInfo["highest_education"])
	}
	if employee.EducationMilitaryInfo["school_name"] != "Nexus University" {
		t.Fatalf("expected school_zh to map to school_name, got %+v", employee.EducationMilitaryInfo)
	}

	position, ok, err := store.GetPosition(context.Background(), "tenant-1", employee.PositionID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || position.Name != "經理" || position.Code != "0901" {
		t.Fatalf("expected position record for job_code, got ok=%v position=%+v", ok, position)
	}
	if position.NameEN != "Manager" || position.Source != "ehrms" {
		t.Fatalf("expected eHRMS position metadata, got %+v", position)
	}
}

// TestSyncEHRMSOrgUnitsUpsertsDepartments 驗證單獨同步組織單位（僅未關閉部門）。
func TestSyncEHRMSOrgUnitsUpsertsDepartments(t *testing.T) {
	departmentRows := []domain.EHRMSDepartmentRecord{
		{"code": "C01", "name": "Corporate", "name_en": "Corporate EN", "parent_code": "", "closed": "false", "manager_job_code": "1502", "manager_job_title": "董事長"},
		{"code": "C0101", "name": "Sales", "name_en": "Sales EN", "parent_code": "C01", "closed": "false", "manager_job_code": "1502"},
		{"code": "C0102", "name": "Ops", "name_en": "Ops EN", "parent_code": "C01", "closed": "false", "manager_job_code": "0901", "manager_job_title": "經理"},
		{"code": "C0199", "name": "Empty Closed", "name_en": "Empty Closed EN", "parent_code": "C01", "closed": "true", "manager_job_code": "0501", "manager_job_title": "實習生"},
	}
	positionRows := []domain.EHRMSPositionRecord{
		{"job_code": "1502", "job_title_zh": "董事長", "job_title_en": "Chairman"},
		{"job_code": "0901", "job_title_zh": "經理", "job_title_en": "Manager"},
	}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.org_unit", Action: "create", Scope: "all"},
		{Resource: "hr.position", Action: "create", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{departmentRows: departmentRows, positionRows: positionRows}})

	result, err := svc.HR().SyncEHRMSOrgUnits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 4 || result.Upserted != 3 {
		t.Fatalf("unexpected org sync result: %+v", result)
	}
	parent := mustOrgUnitByCode(t, store, "tenant-1", "C01")
	sameJobChild := mustOrgUnitByCode(t, store, "tenant-1", "C0101")
	ownJobChild := mustOrgUnitByCode(t, store, "tenant-1", "C0102")
	if sameJobChild.ParentID != parent.ID || sameJobChild.Source != "ehrms" {
		t.Fatalf("expected synced child org unit, unit=%+v", sameJobChild)
	}
	units, err := store.ListOrgUnits(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, unit := range units {
		if unit.Code == "C0199" {
			t.Fatalf("expected closed department to be skipped by org unit sync, got %+v", unit)
		}
	}
	if _, ok, err := store.GetPositionByCode(context.Background(), "tenant-1", "0501"); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("expected closed-department manager job not to be absorbed during org sync")
	}
	chairman := mustPositionByCode(t, store, "tenant-1", "1502")
	manager := mustPositionByCode(t, store, "tenant-1", "0901")
	if parent.ManagerPositionID != chairman.ID {
		t.Fatalf("expected parent manager position %s, got %q", chairman.ID, parent.ManagerPositionID)
	}
	if sameJobChild.ManagerPositionID != "" {
		t.Fatalf("expected same-as-parent manager job to inherit (empty), got %q", sameJobChild.ManagerPositionID)
	}
	if ownJobChild.ManagerPositionID != manager.ID {
		t.Fatalf("expected distinct child manager position %s, got %q", manager.ID, ownJobChild.ManagerPositionID)
	}
}

// TestSyncEHRMSEmployeesBuildsOrgHierarchyAndPositions 驗證員工同步只刷新 DB 既有部門並建立崗位目錄。
func TestSyncEHRMSEmployeesBuildsOrgHierarchyAndPositions(t *testing.T) {
	rows := []domain.EHRMSEmployeeRecord{
		{
			"員工編號":   "E1",
			"中文姓名":   "員工一",
			"email":  "e1@ikala.ai",
			"到職日期":   "2026/06/01",
			"在職狀態":   "在職",
			"部門代碼":   "C01",
			"部門中文名稱": "Corporate",
			"部門英文名稱": "Corporate EN",
			"職務代碼":   "0901",
			"職務中文名稱": "經理",
			"職務英文名稱": "Manager",
			"身份類別名稱": "一般員工",
			"身份證號":   "A123456789",
		},
		{
			"員工編號":   "E2",
			"中文姓名":   "員工二",
			"email":  "e2@ikala.ai",
			"到職日期":   "2026/06/01",
			"在職狀態":   "在職",
			"部門代碼":   "C0101",
			"部門中文名稱": "Sales",
			"部門英文名稱": "Sales EN",
			"職務代碼":   "0704",
			"職務中文名稱": "工程師",
			"職務英文名稱": "Engineer",
			"身份類別名稱": "一般員工",
			"身份證號":   "A223456789",
		},
		{
			"員工編號":   "E3",
			"中文姓名":   "未知部門員工",
			"email":  "e3@ikala.ai",
			"到職日期":   "2026/06/01",
			"在職狀態":   "在職",
			"部門代碼":   "C0199",
			"部門中文名稱": "Empty Closed",
			"職務代碼":   "0704",
			"職務中文名稱": "工程師",
			"身份類別名稱": "一般員工",
			"身份證號":   "A323456789",
		},
	}
	departmentRows := []domain.EHRMSDepartmentRecord{
		{"code": "C01", "name": "Corporate", "name_en": "Corporate EN", "parent_code": "", "closed": "false", "depth": "0"},
		{"code": "C0101", "name": "Sales", "name_en": "Sales EN", "parent_code": "C01", "closed": "false", "depth": "1"},
		{"code": "C0199", "name": "Empty Closed", "name_en": "Empty Closed EN", "parent_code": "C01", "closed": "true", "depth": "1", "headcount": "0"},
	}
	positionRows := []domain.EHRMSPositionRecord{
		{"job_code": "0901", "job_title_zh": "經理", "job_title_en": "Manager"},
		{"job_code": "0704", "job_title_zh": "工程師", "job_title_en": "Engineer"},
		{"job_code": "0501", "job_title_zh": "實習生", "job_title_en": "Intern"},
	}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
		{Resource: "hr.employee", Action: "read", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{rows: rows, departmentRows: departmentRows, positionRows: positionRows}})
	seedOrgUnitCodes(t, store, "tenant-1", "C01", "C0101")

	result, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 2 || result.Created != 2 || result.DepartmentsUpserted != 2 || result.PositionsUpserted != 3 {
		t.Fatalf("unexpected sync counts: %+v", result)
	}
	parent := mustOrgUnitByCode(t, store, "tenant-1", "C01")
	child := mustOrgUnitByCode(t, store, "tenant-1", "C0101")
	if child.ParentID != parent.ID {
		t.Fatalf("expected child org unit parent, got unit=%+v", child)
	}
	if child.NameEN != "Sales EN" || child.Source != "ehrms" || child.UpdatedAt.IsZero() || child.Closed {
		t.Fatalf("expected child org metadata, got %+v", child)
	}
	for _, unit := range mustListOrgUnits(t, store, "tenant-1") {
		if unit.Code == "C0199" {
			t.Fatalf("expected unknown/closed department to stay out of employee sync, got %+v", unit)
		}
	}
	if _, ok, err := store.GetEmployeeByEmployeeNo(context.Background(), "tenant-1", "E3"); err != nil || ok {
		t.Fatalf("expected employee outside local departments to be ignored, ok=%v err=%v", ok, err)
	}
	position := mustPositionByCode(t, store, "tenant-1", "0704")
	if position.Name != "工程師" || position.NameEN != "Engineer" || position.Source != "ehrms" {
		t.Fatalf("expected synced position, got position=%+v", position)
	}
	intern := mustPositionByCode(t, store, "tenant-1", "0501")
	if intern.Name != "實習生" {
		t.Fatalf("expected position from /positions without employees, position=%+v", intern)
	}
}

// TestSyncEHRMSAttendanceUpsertsDailySummaries 驗證 eHRMS 考勤同步 writes 日彙總 without GPS 打卡。
func TestSyncEHRMSAttendanceUpsertsDailySummaries(t *testing.T) {
	syncNow := time.Date(2026, 6, 20, 8, 0, 0, 0, time.UTC)
	queries := make([]domain.EHRMSAttendanceQuery, 0)
	balanceQueries := make([]domain.EHRMSAttendanceQuery, 0)
	detailQueries := make([]domain.EHRMSAttendanceQuery, 0)
	rows := []domain.EHRMSAttendanceRecord{{
		"emp_id":           "IKM017",
		"date":             "2026/06/10",
		"shift_start":      "09:00",
		"shift_end":        "18:00",
		"shift_hours":      "8",
		"daily_hours":      "8",
		"clock_hours":      "8",
		"clock_start":      "2026-06-10 09:01",
		"clock_end":        "18:02:00",
		"attend_start":     "09:00",
		"attend_end":       "18:00",
		"attend_hours":     "8",
		"attend_counted":   "V",
		"leave_type":       "特休",
		"leave_start":      "13:00",
		"leave_end":        "15:00",
		"leave_hours":      "2",
		"leave_counted":    "是",
		"leave2_type":      "病假",
		"leave2_start":     "16:00",
		"leave2_end":       "17:00",
		"leave2_hours":     "1",
		"leave2_counted":   "1",
		"overtime_start":   "18:30",
		"overtime_end":     "20:00",
		"overtime_hours":   "1.5",
		"overtime_counted": "true",
		"name_zh":          "測試員工",
	}, {
		"emp_id":      "MISSING",
		"date":        "2026-06-10",
		"shift_start": "09:00",
		"shift_end":   "18:00",
		"clock_hours": "8",
	}}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "attendance.clock", Action: "import", Scope: "all"},
		{Resource: "attendance.clock", Action: "read", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{
		attendanceRows: rows, attendanceQueries: &queries,
		leaveBalanceQueries: &balanceQueries, leaveDetailQueries: &detailQueries,
	}, Now: func() time.Time { return syncNow }})
	now := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID:               "emp-ehrms",
		TenantID:         "tenant-1",
		EmployeeNo:       "IKM017",
		Name:             "測試員工",
		Status:           "active",
		EmploymentStatus: "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-other-tenant", TenantID: "tenant-2", EmployeeNo: "OTHER001", Name: "Other Tenant", Status: "active", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.Attendance().SyncEHRMSAttendance(ctx, domain.EHRMSAttendanceSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 1 || result.Created != 1 || result.Updated != 0 || result.Skipped != 0 || result.Failed != 0 ||
		result.Mode != "upsert" || result.Start != "2026-01-01" || result.End != "2027-01-01" {
		t.Fatalf("unexpected eHRMS attendance sync result: %+v", result)
	}
	if len(queries) != 1 || queries[0].EmployeeID != "IKM017" || queries[0].Start != "2026-01-01" ||
		queries[0].End != "2027-01-01" || queries[0].Year != "2026" {
		t.Fatalf("expected one current-tenant employee query with annual bounds, got %+v", queries)
	}
	if len(balanceQueries) != 1 || len(detailQueries) != 1 ||
		balanceQueries[0].EmployeeID != "IKM017" || detailQueries[0].EmployeeID != "IKM017" ||
		detailQueries[0].Year != "2026" {
		t.Fatalf("expected attendance, leave balance, and leave detail queries for the current tenant employee, balances=%+v details=%+v", balanceQueries, detailQueries)
	}
	summaries, err := store.ListAttendanceDailySummaries(context.Background(), "tenant-1", domain.AttendanceDailySummaryQuery{EmployeeID: "emp-ehrms", FromDate: "2026-06-10", ToDate: "2026-06-10"})
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected one daily summary, got %+v", summaries)
	}
	got := summaries[0]
	if got.EmployeeID != "emp-ehrms" || got.WorkDate != "2026-06-10" || got.ShiftStart != "09:00" || got.ShiftEnd != "18:00" || got.ClockHours != 8 || got.ExternalRef != "IKM017:2026-06-10" || got.Source != "ehrms" {
		t.Fatalf("unexpected daily summary mapping: %+v", got)
	}
	if got.ClockStart != "09:01" || got.ClockEnd != "18:02" || got.ShiftHours != 8 || got.DailyHours != 8 {
		t.Fatalf("unexpected clock/shift mapping: %+v", got)
	}
	if got.Payload["name_zh"] != "測試員工" || got.Payload["clock_start"] != "2026-06-10 09:01" || got.Payload["leave_type"] != "特休" {
		t.Fatalf("expected payload to preserve unused upstream fields, got %+v", got.Payload)
	}
	clocks, err := store.ListAttendanceClockRecords(context.Background(), "tenant-1", domain.AttendanceClockRecordQuery{EmployeeID: "emp-ehrms"})
	if err != nil {
		t.Fatal(err)
	}
	if len(clocks) != 0 {
		t.Fatalf("eHRMS daily summaries must not create GPS clock records, got %+v", clocks)
	}
	leaves, err := store.ListLeaveRequests(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(leaves) != 0 {
		t.Fatalf("eHRMS daily summaries must not create leave requests, got %+v", leaves)
	}
	overtimes, err := store.ListOvertimeRequestsByQuery(context.Background(), "tenant-1", domain.OvertimeRequestQuery{EmployeeIDs: []string{"emp-ehrms"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(overtimes) != 0 {
		t.Fatalf("eHRMS daily summaries must not create overtime requests, got %+v", overtimes)
	}

	result, err = svc.Attendance().SyncEHRMSAttendance(ctx, domain.EHRMSAttendanceSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 0 || result.Updated != 1 || result.Skipped != 0 {
		t.Fatalf("expected idempotent upsert on second sync, got %+v", result)
	}
}

func TestSyncEHRMSAttendanceUsesBoundedDayWindow(t *testing.T) {
	syncNow := time.Date(2026, 6, 20, 8, 0, 0, 0, time.UTC)
	syncLocation := time.FixedZone("Asia/Shanghai", 8*60*60)
	attendanceQueries := []domain.EHRMSAttendanceQuery{}
	balanceQueries := []domain.EHRMSAttendanceQuery{}
	detailQueries := []domain.EHRMSAttendanceQuery{}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "attendance.clock", Action: "import", Scope: "all"},
	}, service.Options{
		EHRMSClient: fakeEHRMSClient{
			attendanceRows: []domain.EHRMSAttendanceRecord{
				{"emp_id": "IKM017", "date": "2026-06-19", "shift_hours": "8", "daily_hours": "8", "clock_hours": "8"},
				{"emp_id": "IKM017", "date": "2026-06-20", "shift_hours": "8", "daily_hours": "8", "clock_hours": "8"},
			},
			attendanceQueries: &attendanceQueries,
			leaveBalances: []domain.EHRMSLeaveBalanceRecord{{
				"emp_id": "IKM017", "year": "2026", "leave_type": "annual",
				"unit": "hours", "quota": "80", "used": "8", "remaining": "72",
			}},
			leaveBalanceQueries: &balanceQueries,
			leaveDetails: []domain.EHRMSLeaveDetailRecord{
				{"record_id": "LEAVE-OLD", "emp_id": "IKM017", "date": "2026-06-19", "leave_type": "annual", "start": "09:00", "end": "10:00", "hours": "1"},
				{"record_id": "LEAVE-TODAY", "emp_id": "IKM017", "date": "2026-06-20", "leave_type": "annual", "start": "09:00", "end": "10:00", "hours": "1"},
			},
			leaveDetailQueries: &detailQueries,
		},
		Now: func() time.Time { return syncNow },
	})
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-ehrms", TenantID: "tenant-1", EmployeeNo: "IKM017",
		Name: "測試員工", Status: "active", EmploymentStatus: "active",
		CreatedAt: syncNow, UpdatedAt: syncNow,
	}); err != nil {
		t.Fatal(err)
	}
	for _, record := range []domain.LeaveRecord{
		{
			ID: "leave-before-window", TenantID: "tenant-1", EmployeeID: "emp-ehrms",
			Source: "ehrms", LeaveTypeID: domain.StableLeaveTypeID("annual"), Status: "active",
			StartAt:   time.Date(2026, 6, 19, 9, 0, 0, 0, syncLocation),
			EndAt:     time.Date(2026, 6, 19, 10, 0, 0, 0, syncLocation),
			UpdatedAt: syncNow,
		},
		{
			ID: "leave-after-window", TenantID: "tenant-1", EmployeeID: "emp-ehrms",
			Source: "ehrms", LeaveTypeID: domain.StableLeaveTypeID("annual"), Status: "active",
			StartAt:   time.Date(2026, 6, 21, 9, 0, 0, 0, syncLocation),
			EndAt:     time.Date(2026, 6, 21, 10, 0, 0, 0, syncLocation),
			UpdatedAt: syncNow,
		},
	} {
		if err := store.UpsertLeaveRecord(context.Background(), record); err != nil {
			t.Fatal(err)
		}
	}

	result, err := svc.Attendance().SyncEHRMSAttendance(ctx, domain.EHRMSAttendanceSyncInput{
		Start: "2026-06-20", End: "2026-06-21", SkipLeaveTypes: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Start != "2026-06-20" || result.End != "2026-06-21" ||
		result.Fetched != 1 || result.LeaveBalancesFetched != 1 || result.LeaveDetailsFetched != 1 ||
		result.LeaveTypesFetched != 0 {
		t.Fatalf("unexpected bounded sync result: %+v", result)
	}
	for name, queries := range map[string][]domain.EHRMSAttendanceQuery{
		"attendance": attendanceQueries,
		"balance":    balanceQueries,
		"detail":     detailQueries,
	} {
		if len(queries) != 1 || queries[0].Start != "2026-06-20" ||
			queries[0].End != "2026-06-21" || queries[0].Year != "2026" {
			t.Fatalf("%s query did not use daily bounds: %+v", name, queries)
		}
	}
	summaries, err := store.ListAttendanceDailySummaries(context.Background(), "tenant-1", domain.AttendanceDailySummaryQuery{EmployeeID: "emp-ehrms"})
	if err != nil || len(summaries) != 1 || summaries[0].WorkDate != "2026-06-20" {
		t.Fatalf("expected only today's attendance, summaries=%+v err=%v", summaries, err)
	}
	leaveRecords, err := store.ListLeaveRecords(context.Background(), "tenant-1")
	if err != nil || len(leaveRecords) != 3 {
		t.Fatalf("expected today's detail plus untouched records outside the window, records=%+v err=%v", leaveRecords, err)
	}
	byID := map[string]domain.LeaveRecord{}
	for _, record := range leaveRecords {
		byID[record.ID] = record
	}
	if byID["leave-before-window"].DeletedAt != nil || byID["leave-after-window"].DeletedAt != nil {
		t.Fatalf("bounded sync must not tombstone records outside today: %+v", byID)
	}
	todayFound := false
	for _, record := range leaveRecords {
		if record.SourcePayload["record_id"] == "LEAVE-TODAY" {
			todayFound = true
		}
	}
	if !todayFound {
		t.Fatalf("expected today's leave detail, records=%+v", leaveRecords)
	}
}

type attendanceConcurrencyClient struct {
	fakeEHRMSClient
	active    int32
	maxActive int32
	started   chan struct{}
	release   chan struct{}
}

func (c *attendanceConcurrencyClient) ListAttendance(ctx context.Context, query domain.EHRMSAttendanceQuery) ([]domain.EHRMSAttendanceRecord, error) {
	current := atomic.AddInt32(&c.active, 1)
	defer atomic.AddInt32(&c.active, -1)
	for {
		maximum := atomic.LoadInt32(&c.maxActive)
		if current <= maximum || atomic.CompareAndSwapInt32(&c.maxActive, maximum, current) {
			break
		}
	}
	c.started <- struct{}{}
	select {
	case <-c.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return ehrms.NormalizeAttendanceRecords([]domain.EHRMSAttendanceRecord{{
		"emp_id":      query.EmployeeID,
		"date":        "2026-06-10",
		"shift_start": "09:00",
		"shift_end":   "18:00",
		"shift_hours": "8",
		"daily_hours": "8",
		"clock_hours": "8",
	}}), nil
}

func TestSyncEHRMSAttendanceFetchesAtMostTenEmployeesConcurrently(t *testing.T) {
	const employeeCount = 20
	client := &attendanceConcurrencyClient{
		started: make(chan struct{}, employeeCount),
		release: make(chan struct{}),
	}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "attendance.clock", Action: "import", Scope: "all"},
	}, service.Options{
		EHRMSClient: client,
		Now: func() time.Time {
			return time.Date(2026, 6, 20, 8, 0, 0, 0, time.UTC)
		},
	})
	for index := range employeeCount {
		employeeNo := fmt.Sprintf("IKM%03d", index)
		if err := store.UpsertEmployee(context.Background(), domain.Employee{
			ID:               "emp-" + employeeNo,
			TenantID:         "tenant-1",
			EmployeeNo:       employeeNo,
			Name:             employeeNo,
			Status:           "active",
			EmploymentStatus: "active",
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}

	type syncResult struct {
		response domain.EHRMSAttendanceSyncResponse
		err      error
	}
	done := make(chan syncResult, 1)
	go func() {
		response, err := svc.Attendance().SyncEHRMSAttendance(ctx, domain.EHRMSAttendanceSyncInput{})
		done <- syncResult{response: response, err: err}
	}()

	for range 10 {
		select {
		case <-client.started:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for ten concurrent employee fetches")
		}
	}
	select {
	case <-client.started:
		t.Fatal("more than ten employee fetches started concurrently")
	case <-time.After(25 * time.Millisecond):
	}
	close(client.release)

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.response.Fetched != employeeCount || result.response.Created != employeeCount {
			t.Fatalf("unexpected concurrent sync result: %+v", result.response)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for concurrent attendance sync")
	}
	if got := atomic.LoadInt32(&client.maxActive); got != 10 {
		t.Fatalf("maximum concurrent employee fetches = %d, want 10", got)
	}
}

func TestSyncEHRMSAttendanceLinksCrossDayLeaveDetailToDailySegment(t *testing.T) {
	syncNow := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "attendance.clock", Action: "import", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{
		leaveTypes: []domain.EHRMSLeaveTypeRecord{{
			"code": "annual", "kind": "item", "name_zh": "特休假", "unit": "hours",
		}},
		attendanceRows: []domain.EHRMSAttendanceRecord{{
			"emp_id": "IKM018", "date": "2026-07-09",
			"shift_start": "09:00", "shift_end": "17:00", "shift_hours": "7",
			"daily_hours": "0", "clock_hours": "0",
			"leave_type": "特休假", "leave_start": "2026-07-06 09:00",
			"leave_end": "17:00", "leave_hours": "7.00", "leave_counted": "V",
		}},
		leaveBalances: []domain.EHRMSLeaveBalanceRecord{{
			"emp_id": "IKM018", "year": "2026", "leave_type": "特休假",
			"leave_code": "annual", "unit": "hours", "quota": "140", "used": "28", "remaining": "112",
		}},
		leaveDetails: []domain.EHRMSLeaveDetailRecord{{
			"record_id": "ehrms-cross-day-1", "emp_id": "IKM018", "date": "2026-07-09",
			"leave_type": "特休假", "leave_code": "annual",
			"start": "2026-07-06 09:00", "end": "2026-07-09 17:00", "hours": "28",
		}},
	}, Now: func() time.Time { return syncNow }})
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-ehrms-leave", TenantID: "tenant-1", EmployeeNo: "IKM018",
		Name: "測試員工IKM018", Status: "active", EmploymentStatus: "active",
		CreatedAt: syncNow, UpdatedAt: syncNow,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.Attendance().SyncEHRMSAttendance(ctx, domain.EHRMSAttendanceSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 || result.LeaveDetailsCreated != 1 || result.LeaveDetailsFailed != 0 {
		t.Fatalf("unexpected eHRMS sync result: %+v", result)
	}

	segments, err := store.ListAttendanceDailyLeaveSegments(
		context.Background(), "tenant-1", "emp-ehrms-leave", "2026-07-09", "2026-07-09",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 1 {
		t.Fatalf("expected one normalized daily leave segment, got %+v", segments)
	}
	segment := segments[0]
	if segment.DailySource != "ehrms" || segment.SegmentNo != 1 || segment.LeaveTypeID != "annual" || segment.SourceLeaveType != "特休假" ||
		segment.Minutes != 420 || !segment.Counted || !segment.TimeInferred ||
		segment.LinkStatus != "matched" || segment.LeaveRecordID == "" ||
		segment.MatchBasis != "employee+type+exact_interval" {
		t.Fatalf("unexpected linked daily leave segment: %+v", segment)
	}
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	if segment.StartAt == nil || segment.EndAt == nil ||
		segment.StartAt.In(location).Format("2006-01-02 15:04") != "2026-07-09 09:00" ||
		segment.EndAt.In(location).Format("2006-01-02 15:04") != "2026-07-09 17:00" {
		t.Fatalf("cross-day leave must be clipped to the attendance work date: %+v", segment)
	}
	leaveRecord, ok, err := store.GetLeaveRecord(context.Background(), "tenant-1", segment.LeaveRecordID)
	if err != nil || !ok {
		t.Fatalf("linked eHRMS leave record missing, ok=%v err=%v", ok, err)
	}
	if leaveRecord.Source != "ehrms" || leaveRecord.EmployeeID != "emp-ehrms-leave" ||
		leaveRecord.StartAt.In(location).Format("2006-01-02 15:04") != "2026-07-06 09:00" ||
		leaveRecord.EndAt.In(location).Format("2006-01-02 15:04") != "2026-07-09 17:00" {
		t.Fatalf("daily segment must point to the unsplit eHRMS leave detail: %+v", leaveRecord)
	}
	ehrmsDay, ok, err := store.GetAttendanceDailyRecord(
		context.Background(), "tenant-1", "emp-ehrms-leave", "2026-07-09", "ehrms",
	)
	if err != nil || !ok {
		t.Fatalf("unified eHRMS daily record missing, ok=%v err=%v", ok, err)
	}
	if ehrmsDay.ScheduledMinutes != 420 || ehrmsDay.RequiredMinutes != 0 ||
		ehrmsDay.WorkedMinutes != 0 || ehrmsDay.CreditedLeaveMinutes != 420 {
		t.Fatalf("unexpected unified eHRMS daily record: %+v", ehrmsDay)
	}
	localDay, ok, err := store.GetAttendanceDailyRecord(
		context.Background(), "tenant-1", "emp-ehrms-leave", "2026-07-09", "local",
	)
	if err != nil || !ok {
		t.Fatalf("unified local daily record missing, ok=%v err=%v", ok, err)
	}
	if localDay.CreditedLeaveMinutes != 420 || localDay.InputFingerprint == "" {
		t.Fatalf("unexpected unified local daily record: %+v", localDay)
	}
	reconciliation, ok, err := store.GetAttendanceDailyReconciliation(
		context.Background(), "tenant-1", "emp-ehrms-leave", "2026-07-09",
	)
	if err != nil || !ok {
		t.Fatalf("daily reconciliation missing, ok=%v err=%v", ok, err)
	}
	if reconciliation.Status != "mismatch" || reconciliation.LocalFingerprint == "" ||
		reconciliation.EHRMSFingerprint == "" || len(reconciliation.Differences) == 0 {
		t.Fatalf("unexpected daily reconciliation: %+v", reconciliation)
	}
}

// TestSyncEHRMSAttendanceUpsertsLeaveBalancesAndDetails 驗證 eHRMS 假別餘額與明細同步。
func TestSyncEHRMSAttendanceUpsertsLeaveBalancesAndDetails(t *testing.T) {
	syncNow := time.Date(2026, 6, 20, 8, 0, 0, 0, time.UTC)
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "attendance.clock", Action: "import", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{
		leaveBalances: []domain.EHRMSLeaveBalanceRecord{{
			"emp_id":      "IKM017",
			"year":        "2026",
			"leave_type":  "annual",
			"unit":        "days",
			"quota":       "10",
			"used":        "2",
			"remaining":   "8",
			"grant_start": "2026-01-01",
			"expire_date": "2026-12-31",
		}, {
			"emp_id":      "IKM017",
			"year":        "2025",
			"leave_type":  "annual",
			"unit":        "days",
			"quota":       "10",
			"used":        "1",
			"remaining":   "9",
			"grant_start": "2025-01-01",
			"expire_date": "2025-12-31",
		}, {
			"emp_id":     "IKM017",
			"leave_type": "加班",
			"unit":       "hours",
			"used":       "12",
		}},
		leaveDetails: []domain.EHRMSLeaveDetailRecord{{
			"emp_id":     "IKM017",
			"date":       "2026-06-11",
			"leave_type": "annual",
			"start":      "09:00",
			"end":        "13:00",
			"hours":      "4",
		}, {
			"emp_id":     "IKM017",
			"date":       "2026-06-12",
			"leave_type": "出勤時數",
			"start":      "09:00",
			"end":        "18:00",
			"hours":      "8",
		}},
	}, Now: func() time.Time { return syncNow }})
	now := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID:               "emp-ehrms",
		TenantID:         "tenant-1",
		EmployeeNo:       "IKM017",
		Name:             "測試員工",
		Status:           "active",
		EmploymentStatus: "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.Attendance().SyncEHRMSAttendance(ctx, domain.EHRMSAttendanceSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.LeaveBalancesFetched != 3 || result.LeaveBalancesUpserted != 2 || result.LeaveBalancesSkipped != 1 || result.LeaveDetailsFetched != 2 || result.LeaveDetailsCreated != 1 || result.LeaveDetailsUpdated != 0 || result.LeaveDetailsSkipped != 1 {
		t.Fatalf("unexpected eHRMS leave sync result: %+v", result)
	}
	balances, err := store.ListLeaveBalances(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(balances) != 2 {
		t.Fatalf("expected historical and current leave balances, got %+v", balances)
	}
	var current domain.LeaveBalance
	for _, balance := range balances {
		if balance.EntitlementYear == 2026 {
			current = balance
		}
	}
	if current.EmployeeID != "emp-ehrms" || current.LeaveType != "annual" || current.GrantedMinutes != 70*60 || current.UsedMinutes != 14*60 || current.RemainingMinutes != 56*60 || current.Source != "ehrms" {
		t.Fatalf("unexpected current leave balance mapping: %+v", current)
	}
	requests, err := store.ListLeaveRequests(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 0 {
		t.Fatalf("eHRMS facts must not be inserted as Nexus requests, got %+v", requests)
	}
	allLeaveRecords, err := store.ListLeaveRecords(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	externalRecords := make([]domain.LeaveRecord, 0)
	for _, record := range allLeaveRecords {
		if record.Source == "ehrms" {
			externalRecords = append(externalRecords, record)
		}
	}
	if len(externalRecords) != 1 {
		t.Fatalf("expected one independent eHRMS leave fact, got %+v", externalRecords)
	}
	got := externalRecords[0]
	if got.EmployeeID != "emp-ehrms" || got.LeaveTypeID != domain.StableLeaveTypeID("annual") || got.NetMinutes != 240 || got.StartAt.Format("15:04") != "09:00" || got.EndAt.Format("15:04") != "13:00" {
		t.Fatalf("unexpected eHRMS leave fact mapping: %+v", got)
	}

	result, err = svc.Attendance().SyncEHRMSAttendance(ctx, domain.EHRMSAttendanceSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.LeaveDetailsCreated != 0 || result.LeaveDetailsUpdated != 1 || result.LeaveDetailsSkipped != 1 || result.LeaveBalancesUpserted != 2 || result.LeaveBalancesSkipped != 1 {
		t.Fatalf("expected idempotent leave sync, got %+v", result)
	}
}

// TestSyncEHRMSAttendanceRejectsUnknownLeaveCodes verifies unknown upstream codes fail without a catalog match.
func TestSyncEHRMSAttendanceRejectsUnknownLeaveCodes(t *testing.T) {
	syncNow := time.Date(2026, 6, 20, 8, 0, 0, 0, time.UTC)
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "attendance.clock", Action: "import", Scope: "all"},
		{Resource: "attendance.leave", Action: "read", Scope: "all"},
		{Resource: "attendance.leave", Action: "update", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{
		leaveBalances: []domain.EHRMSLeaveBalanceRecord{{
			"emp_id": "IKM-MAP", "year": "2026", "leave_type": "Wellness Leave", "unit": "days", "quota": "1", "remaining": "1",
			"grant_start": "2026-01-01", "expire_date": "2026-12-31",
		}},
	}, Now: func() time.Time { return syncNow }})
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-map", TenantID: "tenant-1", EmployeeNo: "IKM-MAP", Name: "Mapping Employee",
		Status: "active", EmploymentStatus: "active", CreatedAt: syncNow, UpdatedAt: syncNow,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.Attendance().SyncEHRMSAttendance(ctx, domain.EHRMSAttendanceSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.LeaveBalancesFailed != 1 || result.LeaveBalancesUpserted != 0 || len(result.RowErrors) != 1 || result.RowErrors[0].Code != "unknown_leave_type" {
		t.Fatalf("expected unknown leave code to fail against tenant catalog, got %+v", result)
	}

	result, err = svc.Attendance().SyncEHRMSAttendance(ctx, domain.EHRMSAttendanceSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.LeaveBalancesFailed != 1 || result.LeaveBalancesUpserted != 0 {
		t.Fatalf("expected unknown leave code to keep failing without catalog row, got %+v", result)
	}
}

// TestValidateAttendancePolicyDoesNotRewriteLinkedLeaveStorage verifies work-time
// validation no longer accepts or projects a local leave catalog.
func TestValidateAttendancePolicyDoesNotRewriteLinkedLeaveStorage(t *testing.T) {
	now := time.Date(2026, 6, 20, 8, 0, 0, 0, time.UTC)
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "attendance.leave", Action: "read", Scope: "all"},
		{Resource: "attendance.leave", Action: "update", Scope: "all"},
	}, service.Options{Now: func() time.Time { return now }})
	if err := store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{
		ID: "lb-linked", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", LeaveTypeID: "lt_annual",
		RemainingMinutes: 7 * 60, EntitlementYear: 2026, Source: "ehrms", UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	policy, err := svc.Attendance().CurrentAttendancePolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	validation, err := svc.Attendance().ValidateAttendancePolicy(ctx, domain.UpdateAttendancePolicyInput{
		BaseVersion: policy.Version, WorkTime: policy.WorkTime,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !validation.Valid || len(validation.Issues) != 0 {
		t.Fatalf("expected work-time-only policy validation, got %+v", validation)
	}
}

// TestSyncEHRMSAttendanceCountsInvalidRows 驗證 eHRMS 考勤同步 counts invalid rows without aborting batch。
func TestSyncEHRMSAttendanceCountsInvalidRows(t *testing.T) {
	rows := []domain.EHRMSAttendanceRecord{{
		"emp_id":      "IKM018",
		"date":        "bad-date",
		"shift_start": "bad-time",
		"clock_hours": "oops",
	}}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "attendance.clock", Action: "import", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{attendanceRows: rows}})
	if err := store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-invalid", TenantID: "tenant-1", EmployeeNo: "IKM018", Name: "Invalid", Status: "active", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.Attendance().SyncEHRMSAttendance(ctx, domain.EHRMSAttendanceSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 1 || result.Failed != 1 || len(result.RowErrors) == 0 {
		t.Fatalf("expected invalid row to be counted, got %+v", result)
	}
}

// TestSyncEHRMSAttendanceHidesUpstreamFetchDetails 驗證 eHRMS 考勤 hides upstream fetch details。
func TestSyncEHRMSAttendanceHidesUpstreamFetchDetails(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "attendance.clock", Action: "import", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{attendanceErr: errors.New("upstream 500: token=secret-value")}})
	if err := store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-fetch", TenantID: "tenant-1", EmployeeNo: "IKM-FETCH", Name: "Fetch", Status: "active", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	_, err := svc.Attendance().SyncEHRMSAttendance(ctx, domain.EHRMSAttendanceSyncInput{})
	if err == nil {
		t.Fatal("expected eHRMS attendance fetch failure")
	}
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Code != "bad_request" || appErr.Message != "fetch eHRMS attendance failed" {
		t.Fatalf("expected sanitized bad_request, got %v", err)
	}
	if strings.Contains(err.Error(), "secret-value") || strings.Contains(err.Error(), "upstream 500") {
		t.Fatalf("eHRMS attendance fetch error leaked upstream detail: %v", err)
	}
}

// TestEmployeeReinstatementRequiresTransitionAndKeepsDeletedTerminal 驗證員工 reinstatement requires 轉換 and keeps deleted terminal。
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
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", RequestID: "req-reinstate"}
	if _, err := svc.HR().TransitionEmployeeStatus(ctx, "emp-resign", domain.StatusTransitionInput{Status: "resigned", Reason: "left", EndDate: "2026-06-30"}); err != nil {
		t.Fatal(err)
	}
	linked, ok, err := store.GetAccount(context.Background(), "tenant-1", "acct-linked")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || linked.Status != "disabled" {
		t.Fatalf("expected resignation to disable linked account, got %+v", linked)
	}
	if _, err := svc.HR().UpdateEmployeeStatus(ctx, "emp-resign", "active"); err == nil {
		t.Fatal("expected direct status patch to reject resigned employee reactivation")
	}
	_, err = svc.HR().TransitionEmployeeStatus(ctx, "emp-resign", domain.StatusTransitionInput{Status: "active", Reason: "rehired"})
	if appErr, ok := domain.AsAppError(err); !ok || appErr.Code != "validation_failed" || len(appErr.FieldErrors) == 0 {
		t.Fatalf("expected reinstatement start_date validation, got %v", err)
	}
	reinstated, err := svc.HR().TransitionEmployeeStatus(ctx, "emp-resign", domain.StatusTransitionInput{Status: "active", Reason: "rehired", StartDate: "2026-07-01"})
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
	events, err := store.ListOutboxEvents(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if !hasBusinessOutboxEvent(events, "employee.reinstated") {
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
	if _, err := svc.HR().DeleteEmployee(ctx, "emp-delete"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.HR().TransitionEmployeeStatus(ctx, "emp-delete", domain.StatusTransitionInput{Status: "active"}); err == nil {
		t.Fatal("expected deleted employee reactivation to be rejected")
	}
}

// TestBatchDeleteEmployeesUsesPerEmployeeResultsAndAudits 驗證批次 delete 員工 uses per 員工結果 and audits。
func TestBatchDeleteEmployeesUsesPerEmployeeResultsAndAudits(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "delete", Scope: "all"},
	})
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	ctx.RequestID = "req-batch-delete"
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-linked", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:               "emp-linked",
		TenantID:         "tenant-1",
		Name:             "Linked Employee",
		AccountID:        "acct-linked",
		Status:           "active",
		EmploymentStatus: "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:               "emp-deleted",
		TenantID:         "tenant-1",
		Name:             "Already Deleted",
		Status:           "deleted",
		EmploymentStatus: "deleted",
		CreatedAt:        now.Add(time.Minute),
		UpdatedAt:        now.Add(time.Minute),
	})

	result, err := svc.HR().BatchDeleteEmployees(ctx, domain.BatchDeleteEmployeesInput{
		EmployeeIDs: []string{"emp-linked", "emp-deleted"},
		Reason:      "cleanup",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 2 || !result.Results[0].Success || result.Results[0].Action != "soft_deleted_account_disabled" || result.Results[1].Success {
		t.Fatalf("unexpected batch delete result: %+v", result)
	}
	if result.Results[1].Code != "conflict" {
		t.Fatalf("expected already deleted row to fail with conflict, got %+v", result.Results[1])
	}
	linked, ok, err := store.GetAccount(context.Background(), "tenant-1", "acct-linked")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || linked.Status != "disabled" {
		t.Fatalf("expected linked account to be disabled, got %+v", linked)
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	deleteLog, ok := findAuditLog(logs, "hr.employee.delete")
	if !ok || deleteLog.Target != "emp-linked" || deleteLog.Result == "" || deleteLog.Details["account_disabled"] != true {
		t.Fatalf("expected per-employee delete audit, got %+v", logs)
	}
	batchLog, ok := findAuditLog(logs, "hr.employee.batch_delete")
	if !ok || batchLog.TraceID != "req-batch-delete" || batchLog.Result != "partial_success" || batchLog.Details["reason"] != "cleanup" {
		t.Fatalf("expected batch delete audit, got %+v", logs)
	}
	succeeded, _ := batchLog.Details["succeeded_employee_ids"].([]string)
	failed, _ := batchLog.Details["failed_employee_ids"].([]string)
	if !stringSliceContains(succeeded, "emp-linked") || !stringSliceContains(failed, "emp-deleted") {
		t.Fatalf("expected batch audit to record succeeded and failed employee ids, got %+v", batchLog.Details)
	}
}

// TestDeleteEmployeeRejectsAlreadyDeleted 驗證員工 rejects already deleted。
func TestDeleteEmployeeRejectsAlreadyDeleted(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "delete", Scope: "all"},
	})
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:               "emp-deleted-single",
		TenantID:         "tenant-1",
		Name:             "Already Deleted",
		Status:           "deleted",
		EmploymentStatus: "deleted",
		CreatedAt:        now,
		UpdatedAt:        now,
	})

	_, err := svc.HR().DeleteEmployee(ctx, "emp-deleted-single")
	if appErr, ok := domain.AsAppError(err); !ok || appErr.Code != "conflict" {
		t.Fatalf("expected conflict for already deleted employee, got %v", err)
	}
	events, err := store.ListOutboxEvents(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no offboard event for rejected delete, got %+v", events)
	}
}

// TestAuditServiceRequiresAuditReadPermission 驗證稽覈服務 requires 稽覈 read 權限。
func TestAuditServiceRequiresAuditReadPermission(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	svc := service.New(store)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	if _, err := svc.Audit().ListLogPage(ctx, domain.PageRequest{}); err == nil {
		t.Fatal("expected audit log read to require audit permission")
	}

	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-audit",
		TenantID: "tenant-1",
		Name:     "Audit",
		Permissions: []domain.Permission{
			{Resource: "audit.log", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-audit"}, CreatedAt: now})
	if _, err := svc.Audit().ListLogPage(ctx, domain.PageRequest{}); err != nil {
		t.Fatalf("expected audit log read with permission to succeed, got %v", err)
	}
}

// TestEmployeePreviewCreateDoesNotPersist 驗證員工 preview create does not persist。
func TestEmployeePreviewCreateDoesNotPersist(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "create", Scope: "all"},
	})
	input := validEmployeeInput("E3001", "Preview Person", "preview.person@example.com")

	preview, err := svc.HR().PreviewCreateEmployee(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Valid || preview.Employee.CompanyEmail != input.CompanyEmail || len(preview.FieldErrors) != 0 {
		t.Fatalf("expected valid preview response, got %+v", preview)
	}
	employees, err := store.ListEmployees(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(employees) != 0 {
		t.Fatalf("preview must not persist employees, got %+v", employees)
	}

	invalid := input
	invalid.Name = ""
	invalidPreview, err := svc.HR().PreviewCreateEmployee(ctx, invalid)
	if err != nil {
		t.Fatal(err)
	}
	if invalidPreview.Valid || !fieldErrorsContain(invalidPreview.FieldErrors, "name") {
		t.Fatalf("expected validation errors in preview response, got %+v", invalidPreview)
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findAuditLog(logs, "hr.employee.create"); ok {
		t.Fatalf("preview should not write employee create audit event, got %+v", logs)
	}
}

// TestEmployeeAvatarUploadReplaceAndDelete 驗證員工 avatar upload replace and delete。
func TestEmployeeAvatarUploadReplaceAndDelete(t *testing.T) {
	objects := &recordingObjectStore{}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "update", Scope: "all"},
	}, service.Options{ObjectStore: objects})
	now := time.Now().UTC()
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:               "emp-avatar",
		TenantID:         "tenant-1",
		EmployeeNo:       "E4001",
		Name:             "Avatar Person",
		CompanyEmail:     "avatar.person@example.com",
		Status:           "active",
		EmploymentStatus: "active",
		BasicInfo:        map[string]any{"avatar_object_key": "employee-avatars/tenant-1/emp-avatar/old.png"},
		CreatedAt:        now,
		UpdatedAt:        now,
	})

	updated, err := svc.HR().UpdateEmployeeAvatar(ctx, "emp-avatar", domain.EmployeeAvatarInput{
		Filename:    "photo.png",
		ContentType: "image/png",
		Content:     testPNGBytes(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects.keys) != 1 || !strings.HasPrefix(objects.keys[0], "employee-avatars/tenant-1/emp-avatar/") {
		t.Fatalf("expected avatar object key under tenant/employee prefix, got %+v", objects.keys)
	}
	if updated.BasicInfo["avatar_object_key"] != objects.keys[0] {
		t.Fatalf("expected avatar key on employee, got %+v", updated.BasicInfo)
	}
	if len(objects.deleted) != 1 || objects.deleted[0] != "employee-avatars/tenant-1/emp-avatar/old.png" {
		t.Fatalf("expected old avatar object to be deleted, got %+v", objects.deleted)
	}

	deleted, err := svc.HR().DeleteEmployeeAvatar(ctx, "emp-avatar")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := deleted.BasicInfo["avatar"]; ok {
		t.Fatalf("expected avatar metadata to be removed, got %+v", deleted.BasicInfo)
	}
	if _, ok := deleted.BasicInfo["avatar_object_key"]; ok {
		t.Fatalf("expected avatar object key to be removed, got %+v", deleted.BasicInfo)
	}
	if len(objects.deleted) != 2 || objects.deleted[1] != objects.keys[0] {
		t.Fatalf("expected current avatar object to be deleted, got %+v", objects.deleted)
	}
}

// TestEmployeeAvatarRejectsForgedContentType 驗證員工 avatar rejects forged content type。
func TestEmployeeAvatarRejectsForgedContentType(t *testing.T) {
	objects := &recordingObjectStore{}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "update", Scope: "all"},
	}, service.Options{ObjectStore: objects})
	now := time.Now().UTC()
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:               "emp-avatar",
		TenantID:         "tenant-1",
		EmployeeNo:       "E4002",
		Name:             "Forged Avatar",
		CompanyEmail:     "forged.avatar@example.com",
		Status:           "active",
		EmploymentStatus: "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	})

	_, err := svc.HR().UpdateEmployeeAvatar(ctx, "emp-avatar", domain.EmployeeAvatarInput{
		Filename:    "photo.png",
		ContentType: "image/png",
		Content:     []byte("not really a png"),
	})
	if err == nil || !strings.Contains(err.Error(), "valid image") {
		t.Fatalf("expected forged avatar content to be rejected, got %v", err)
	}
	if len(objects.keys) != 0 || len(objects.deleted) != 0 {
		t.Fatalf("invalid avatar should not touch object store, keys=%+v deleted=%+v", objects.keys, objects.deleted)
	}
}

// TestInviteDeletedOrResignedEmployeeFails 驗證 deleted or resigned 員工 fails。
func TestInviteDeletedOrResignedEmployeeFails(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "invite", Scope: "all"},
	})
	now := time.Now().UTC()
	for _, item := range []domain.Employee{
		{ID: "emp-resigned", TenantID: "tenant-1", EmployeeNo: "E6001", Name: "Resigned", CompanyEmail: "resigned@example.com", Status: "resigned", EmploymentStatus: "resigned", CreatedAt: now, UpdatedAt: now},
		{ID: "emp-deleted", TenantID: "tenant-1", EmployeeNo: "E6002", Name: "Deleted", CompanyEmail: "deleted@example.com", Status: "deleted", EmploymentStatus: "deleted", CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.UpsertEmployee(context.Background(), item); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"emp-resigned", "emp-deleted"} {
		_, err := svc.HR().InviteEmployee(ctx, id, domain.InviteEmployeeInput{})
		if appErr, ok := domain.AsAppError(err); !ok || appErr.Code != "conflict" {
			t.Fatalf("expected conflict for %s invite, got %v", id, err)
		}
	}
}

// TestInviteEmployeeRejectsDuplicateAccountEmail 驗證員工 rejects duplicate 帳號 email。
func TestInviteEmployeeRejectsDuplicateAccountEmail(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "invite", Scope: "all"},
	})
	now := time.Now().UTC()
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-existing", TenantID: "tenant-1", Email: "taken@example.com", Status: "active", CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:               "emp-invite",
		TenantID:         "tenant-1",
		EmployeeNo:       "E6100",
		Name:             "Invite Target",
		CompanyEmail:     "invite.target@example.com",
		Status:           "active",
		EmploymentStatus: "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	})

	_, err := svc.HR().InviteEmployee(ctx, "emp-invite", domain.InviteEmployeeInput{Email: "TAKEN@example.com"})
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Code != "validation_failed" || len(appErr.FieldErrors) == 0 || appErr.FieldErrors[0].Code != "unique" {
		t.Fatalf("expected duplicate invite email validation failure, got %v", err)
	}
}

// TestInviteEmployeeProvisionKeycloakIdentity 驗證員工 provision Keycloak 身分。
func TestInviteEmployeeProvisionKeycloakIdentity(t *testing.T) {
	provisioner := &recordingIdentityProvisioner{}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "invite", Scope: "all"},
	}, service.Options{IdentityProvisioner: provisioner})
	now := time.Now().UTC()
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:               "emp-keycloak-invite",
		TenantID:         "tenant-1",
		EmployeeNo:       "E6200",
		Name:             "Keycloak Invite",
		CompanyEmail:     "keycloak.invite@example.com",
		Status:           "active",
		EmploymentStatus: "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	})

	invited, err := svc.HR().InviteEmployee(ctx, "emp-keycloak-invite", domain.InviteEmployeeInput{})
	if err != nil {
		t.Fatal(err)
	}
	if invited.AccountID == "" || len(provisioner.inputs) != 1 || !provisioner.inputs[0].SendInvite {
		t.Fatalf("expected invite to provision one keycloak user, employee=%+v inputs=%+v", invited, provisioner.inputs)
	}
	identities, err := store.ListUserIdentities(context.Background(), "tenant-1", invited.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(identities) != 1 || identities[0].Subject != "kc-"+invited.AccountID || identities[0].Email != "keycloak.invite@example.com" {
		t.Fatalf("expected keycloak identity binding for invited employee, got %+v", identities)
	}
}

// TestEmployeeUpdateRespectsDataScope 驗證員工 update respects 資料範圍。
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
	_, err := service.New(store).HR().UpdateEmployee(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		"emp-2",
		domain.UpdateEmployeeInput{Name: &newName},
	)
	if err == nil {
		t.Fatal("expected scoped update to reject another employee")
	}
}

// TestCreatePermissionSetAssignmentRejectsDanglingReferences 驗證權限集合指派 rejects dangling references。
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
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-admin",
		TenantID: "tenant-1",
		Name:     "Admin",
		Permissions: []domain.Permission{
			{Resource: "iam.permission_set_assignment", Action: "create", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-admin", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-admin"}, CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-target", TenantID: "tenant-1", Status: "active", CreatedAt: now})

	svc := service.New(store)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}
	_, err := svc.IAM().CreatePermissionSetAssignment(ctx, domain.CreatePermissionSetAssignmentInput{
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

	_, err = svc.IAM().CreatePermissionSetAssignment(ctx, domain.CreatePermissionSetAssignmentInput{
		PrincipalType:   "account",
		PrincipalID:     "missing-account",
		PermissionSetID: "ps-read",
	})
	if err == nil || !strings.Contains(err.Error(), "account") {
		t.Fatalf("expected missing account to be rejected, got %v", err)
	}

	_, err = svc.IAM().CreatePermissionSetAssignment(ctx, domain.CreatePermissionSetAssignmentInput{
		PrincipalType:   "external",
		PrincipalID:     "acct-target",
		PermissionSetID: "ps-read",
	})
	if err == nil || !strings.Contains(err.Error(), "principal_type") {
		t.Fatalf("expected unsupported principal type to be rejected, got %v", err)
	}

	_, err = svc.IAM().CreatePermissionSetAssignment(ctx, domain.CreatePermissionSetAssignmentInput{
		PrincipalType:   "account",
		PrincipalID:     "acct-target",
		PermissionSetID: "ps-read",
		ConditionID:     "condition-not-implemented",
	})
	if err == nil || !strings.Contains(err.Error(), "condition_id is not supported") {
		t.Fatalf("expected unsupported condition to be rejected instead of becoming unconditional, got %v", err)
	}
}

// TestLegacyConditionalAllowDoesNotGrantUnconditionalAccess 驗證既有 conditional allow 在缺少 evaluator 時保持不生效。
func TestLegacyConditionalAllowDoesNotGrantUnconditionalAccess(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-read", TenantID: "tenant-1", Name: "Read", CreatedAt: now,
		Permissions: []domain.Permission{{Resource: "hr.employee", Action: "read", Scope: "all"}},
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-target", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID: "assign-conditional", TenantID: "tenant-1", PrincipalType: "account", PrincipalID: "acct-target",
		PermissionSetID: "ps-read", Effect: "allow", ConditionID: "legacy-condition", CreatedAt: now,
	})

	result, err := service.New(store).Authz().Check(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-target"},
		domain.CheckRequest{Resource: "hr.employee", Action: "read"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed {
		t.Fatalf("expected unevaluated conditional allow to stay inactive, got %+v", result)
	}
}

// TestCustomConditionRejectsUnsupportedExpressionAndLegacyScopeFailsClosed 驗證 custom scope 只接受可執行的結構化條件。
func TestCustomConditionRejectsUnsupportedExpressionAndLegacyScopeFailsClosed(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-admin", TenantID: "tenant-1", Name: "Policy Admin", CreatedAt: now,
		Permissions: []domain.Permission{{Resource: "iam.data_scope", Action: "create", Scope: "all"}},
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-read", TenantID: "tenant-1", Name: "Read", CreatedAt: now,
		Permissions: []domain.Permission{{Resource: "hr.employee", Action: "read", Scope: "all"}},
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-admin", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-admin"}, CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-user", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	svc := service.New(store)

	_, err := svc.IAM().CreateDataScope(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}, domain.CreateDataScopeInput{
		Code: "unsupported", Name: "Unsupported", ScopeType: "custom_condition", Params: map[string]any{"expression": "employee.status == 'active'"},
	})
	if err == nil || !strings.Contains(err.Error(), "support only") {
		t.Fatalf("expected expression-only custom scope to be rejected, got %v", err)
	}

	_ = store.UpsertDataScope(context.Background(), domain.DataScope{
		ID: "ds-legacy", TenantID: "tenant-1", Code: "legacy", Name: "Legacy", ScopeType: "custom_condition",
		Params: map[string]any{"expression": "employee.status == 'active'"}, CreatedAt: now,
	})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID: "assign-legacy", TenantID: "tenant-1", PrincipalType: "account", PrincipalID: "acct-user",
		PermissionSetID: "ps-read", Effect: "allow", DataScopeID: "ds-legacy", CreatedAt: now,
	})
	result, err := svc.Authz().Check(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-user"},
		domain.CheckRequest{Resource: "hr.employee", Action: "read"},
	)
	if err == nil || result.Allowed {
		t.Fatalf("expected legacy ineffective custom scope to fail closed, result=%+v err=%v", result, err)
	}
}

// TestMissingAssignmentDataScopeDoesNotBecomeUnscopedGrant 驗證 missing 指派資料範圍 does not become unscoped grant。
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

	_, err := service.New(store).HR().ListEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err == nil || !strings.Contains(err.Error(), "data scope") {
		t.Fatalf("expected dangling data scope to fail closed, got %v", err)
	}
}

// TestEmployeeOptionsOnlyIncludeVisibleDepartments 驗證員工選項 only include 可見 departments。
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

	options, err := service.New(store).HR().EmployeeOptions(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
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

// TestEmployeeOptionsDepartmentSubtreeIncludesEmptyOrgUnits 驗證員工選項部門 subtree includes 空值組織單位。
func TestEmployeeOptionsDepartmentSubtreeIncludesEmptyOrgUnits(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-root", TenantID: "tenant-1", Name: "Root", Path: []string{"ou-root"}, CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-filled", TenantID: "tenant-1", Name: "Filled", ParentID: "ou-root", Path: []string{"ou-root", "ou-filled"}, CreatedAt: now.Add(time.Minute)})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-empty", TenantID: "tenant-1", Name: "Empty", ParentID: "ou-root", Path: []string{"ou-root", "ou-empty"}, CreatedAt: now.Add(2 * time.Minute)})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-hidden", TenantID: "tenant-1", Name: "Hidden", Path: []string{"ou-hidden"}, CreatedAt: now.Add(3 * time.Minute)})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-read",
		TenantID: "tenant-1",
		Name:     "Scoped Employee Read",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertDataScope(context.Background(), domain.DataScope{ID: "ds-subtree", TenantID: "tenant-1", Code: "department_subtree", Name: "Department Subtree", ScopeType: "department_subtree", CreatedAt: now})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "assign-1",
		TenantID:        "tenant-1",
		PrincipalType:   "account",
		PrincipalID:     "acct-1",
		PermissionSetID: "ps-read",
		Effect:          "allow",
		DataScopeID:     "ds-subtree",
		CreatedAt:       now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", EmployeeID: "emp-manager", Status: "active", CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-manager", TenantID: "tenant-1", Name: "Manager", OrgUnitID: "ou-root", Position: "Manager", Status: "active", CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-filled", TenantID: "tenant-1", Name: "Filled Employee", OrgUnitID: "ou-filled", Position: "Engineer", Status: "active", CreatedAt: now.Add(time.Minute)})

	options, err := service.New(store).HR().EmployeeOptions(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(options.Departments))
	for _, unit := range options.Departments {
		got = append(got, unit.ID)
	}
	slices.Sort(got)
	if strings.Join(got, ",") != "ou-empty,ou-filled,ou-root" {
		t.Fatalf("expected subtree departments including empty org unit, got %+v", got)
	}
}

// TestCreateOrgUnitPathDoesNotDuplicateParent 驗證組織單位 path does not duplicate parent。
func TestCreateOrgUnitPathDoesNotDuplicateParent(t *testing.T) {
	svc, ctx := newServiceFixture([]domain.Permission{
		{Resource: "hr.org_unit", Action: "create", Scope: "all"},
	})
	root, err := svc.HR().CreateOrgUnit(ctx, domain.CreateOrgUnitInput{Name: "Root"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.HR().CreateOrgUnit(ctx, domain.CreateOrgUnitInput{Name: "Child", ParentID: root.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(child.Path, "/"), root.ID+"/"+child.ID; got != want {
		t.Fatalf("unexpected child path: got %q want %q", got, want)
	}
}

// TestPlatformTaskMutationsPersistAndProject 驗證平臺任務 mutations persist and project。
func TestPlatformTaskMutationsPersistAndProject(t *testing.T) {
	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-platform-task",
		TenantID: "tenant-1",
		Name:     "Platform Task",
		Permissions: []domain.Permission{
			{Resource: "me", Action: "read", Scope: "self"},
			{Resource: "me", Action: "create", Scope: "self"},
			{Resource: "me", Action: "update", Scope: "self"},
			{Resource: "me", Action: "delete", Scope: "self"},
			{Resource: "attendance.clock", Action: "read", Scope: "self"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		DisplayName:            "Task Tester",
		EmployeeID:             "emp-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-platform-task"},
		CreatedAt:              now,
	})

	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	item, err := svc.Platform().CreateTaskItem(ctx, domain.CreatePlatformTaskItemInput{
		WorkDate: "2026-07-01",
		Title:    "Implement task API",
		Category: "Backend",
		Product:  "Nexus",
		Hours:    1.5,
		Note:     "wire frontend",
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Title != "Implement task API" || item.Hours != 1.5 {
		t.Fatalf("unexpected created task item: %+v", item)
	}

	tasks, err := svc.Platform().Tasks(ctx, domain.PlatformTasksQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if tasks.ClockSummary == nil {
		t.Fatal("expected authorized task projection to include clock summary")
	}
	record := findPlatformTaskRecord(t, tasks.Records, "2026/07/01")
	if record.TotalHours != 1.5 || len(record.Items) != 1 || record.Items[0].ID != item.ID {
		t.Fatalf("expected task item to project into 2026/07/01 record, got %+v", record)
	}

	updatedTitle := "Implement task API v2"
	updatedHours := 2.0
	updated, err := svc.Platform().UpdateTaskItem(ctx, item.ID, domain.UpdatePlatformTaskItemInput{
		Title: &updatedTitle,
		Hours: &updatedHours,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != updatedTitle || updated.Hours != updatedHours {
		t.Fatalf("unexpected updated task item: %+v", updated)
	}

	todo, err := svc.Platform().CreateTaskTodo(ctx, domain.CreatePlatformTaskTodoInput{
		Text:    "Convert me",
		DueDate: "2026-07-02",
	})
	if err != nil {
		t.Fatal(err)
	}
	if todo.Done || todo.Date != "07/02" || todo.WorkDate != "2026/07/02" || todo.DueDate != "2026/07/02" {
		t.Fatalf("unexpected created todo: %+v", todo)
	}

	converted, err := svc.Platform().ConvertTaskTodo(ctx, todo.ID, domain.ConvertPlatformTaskTodoInput{
		WorkDate: "2026-07-02",
		Hours:    0.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if converted.Title != "Convert me" || converted.Category != "待辦" || converted.Hours != 0.5 {
		t.Fatalf("unexpected converted item: %+v", converted)
	}

	tasks, err = svc.Platform().Tasks(ctx, domain.PlatformTasksQuery{})
	if err != nil {
		t.Fatal(err)
	}
	convertedRecord := findPlatformTaskRecord(t, tasks.Records, "2026/07/02")
	if convertedRecord.TotalHours != 0.5 || len(convertedRecord.Items) != 1 || convertedRecord.Items[0].ID != converted.ID {
		t.Fatalf("expected converted todo to project as a task item, got %+v", convertedRecord)
	}
	projectedTodo := findPlatformTaskTodo(t, tasks.Todos, todo.ID)
	if !projectedTodo.Done {
		t.Fatalf("expected converted todo to be done, got %+v", projectedTodo)
	}

	if err := store.UpsertAgentRun(context.Background(), domain.AgentRun{
		ID:        "run-platform-readonly",
		TenantID:  "tenant-1",
		AccountID: "acct-1",
		Mode:      "assistant_chat",
		Prompt:    "Summarize my tasks",
		Status:    string(domain.AgentRunStatusCompleted),
		CreatedAt: now.Add(2 * time.Hour),
		UpdatedAt: now.Add(2 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	tasks, err = svc.Platform().Tasks(ctx, domain.PlatformTasksQuery{})
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range tasks.Records {
		for _, item := range record.Items {
			if item.ID == "run-platform-readonly" || item.Source == "agent_run" {
				t.Fatalf("agent run must not project as a task item: %+v", item)
			}
		}
	}
	for _, todo := range tasks.Todos {
		if todo.ID == "todo-run-platform-readonly" || todo.Source == "agent_run" {
			t.Fatalf("agent run must not project as a task todo: %+v", todo)
		}
	}

	deletedTodo, err := svc.Platform().DeleteTaskTodo(ctx, todo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if deletedTodo.ID != todo.ID {
		t.Fatalf("unexpected deleted todo: %+v", deletedTodo)
	}
	deletedItem, err := svc.Platform().DeleteTaskItem(ctx, updated.ID)
	if err != nil {
		t.Fatal(err)
	}
	if deletedItem.ID != updated.ID {
		t.Fatalf("unexpected deleted task item: %+v", deletedItem)
	}
}

// TestPlatformWorkspaceOrganizationManagerUpdatePersistsHierarchy 驗證平臺工作區 organization 主管 update persists hierarchy。
func TestPlatformWorkspaceOrganizationManagerUpdatePersistsHierarchy(t *testing.T) {
	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-1", TenantID: "tenant-1", Name: "HQ", Path: []string{"ou-1"}, CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-workspace-org",
		TenantID: "tenant-1",
		Name:     "Workspace Org",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
			{Resource: "hr.employee", Action: "update", Scope: "all"},
			{Resource: "hr.org_unit", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		DisplayName:            "Workspace Admin",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-workspace-org"},
		CreatedAt:              now,
	})
	for _, employee := range []domain.Employee{
		{ID: "emp-manager", TenantID: "tenant-1", EmployeeNo: "E1001", Name: "Manager One", CompanyEmail: "manager@example.com", OrgUnitID: "ou-1", Position: "Manager", Status: "active", EmploymentStatus: "active", CreatedAt: now},
		{ID: "emp-report", TenantID: "tenant-1", EmployeeNo: "E1002", Name: "Report Two", CompanyEmail: "report@example.com", OrgUnitID: "ou-1", Position: "Engineer", Status: "active", EmploymentStatus: "active", CreatedAt: now.Add(time.Minute)},
		{ID: "emp-leaf", TenantID: "tenant-1", EmployeeNo: "E1003", Name: "Leaf Three", CompanyEmail: "leaf@example.com", OrgUnitID: "ou-1", ManagerEmployeeID: "emp-report", Position: "Engineer", Status: "active", EmploymentStatus: "active", CreatedAt: now.Add(2 * time.Minute)},
	} {
		_ = store.UpsertEmployee(context.Background(), employee)
	}
	svc := service.New(store, service.Options{Now: func() time.Time { return now.Add(time.Hour) }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	organization, err := svc.Workspace().UpdateWorkspaceOrganizationManager(ctx, "E1002", domain.UpdateWorkspaceOrganizationManagerInput{ParentID: stringPtr("E1001")})
	if err != nil {
		if appErr, ok := domain.AsAppError(err); ok {
			t.Fatalf("%s fields=%+v", appErr.Message, appErr.FieldErrors)
		}
		t.Fatal(err)
	}
	report, ok, err := store.GetEmployee(context.Background(), "tenant-1", "emp-report")
	if err != nil || !ok {
		t.Fatalf("report lookup failed ok=%v err=%v", ok, err)
	}
	if report.ManagerEmployeeID != "emp-manager" {
		t.Fatalf("expected report manager to persist, got %+v", report)
	}
	row := findWorkspaceOrganizationRow(t, organization.Rows, "E1002")
	if row.ParentID != "E1001" || row.Level != 2 {
		t.Fatalf("expected refreshed organization row to point at E1001, got %+v", row)
	}
	if !row.ShowInOrgChart {
		t.Fatalf("expected organization rows to be visible by default, got %+v", row)
	}

	organization, err = svc.Workspace().UpdateWorkspaceOrganizationVisibility(ctx, "E1002", domain.UpdateWorkspaceOrganizationVisibilityInput{ShowInOrgChart: boolPtr(false)})
	if err != nil {
		t.Fatal(err)
	}
	report, ok, err = store.GetEmployee(context.Background(), "tenant-1", "emp-report")
	if err != nil || !ok {
		t.Fatalf("report lookup after visibility update failed ok=%v err=%v", ok, err)
	}
	if report.ShowInOrgChart {
		t.Fatalf("expected organization chart visibility to persist, got %+v", report)
	}
	row = findWorkspaceOrganizationRow(t, organization.Rows, "E1002")
	if row.ShowInOrgChart {
		t.Fatalf("expected refreshed organization row to be hidden, got %+v", row)
	}

	if _, err := svc.Workspace().UpdateWorkspaceOrganizationManager(ctx, "E1001", domain.UpdateWorkspaceOrganizationManagerInput{ParentID: stringPtr("E1003")}); err == nil {
		t.Fatal("expected manager cycle to be rejected")
	}
}

// TestPlatformWorkspaceFormDesignMutationsPersistTemplateSchema 驗證平臺工作區表單 design mutations persist 範本 schema。
func TestPlatformWorkspaceFormDesignMutationsPersistTemplateSchema(t *testing.T) {
	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	currentNow := now
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-workspace-forms",
		TenantID: "tenant-1",
		Name:     "Workspace Forms",
		Permissions: []domain.Permission{
			{Resource: "workflow.form_template", Action: "read", Scope: "all"},
			{Resource: "workflow.form_template", Action: "create", Scope: "all"},
			{Resource: "workflow.form_template", Action: "update", Scope: "all"},
			{Resource: "workflow.form_template", Action: "delete", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-forms",
		TenantID:               "tenant-1",
		DisplayName:            "Forms Admin",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-workspace-forms"},
		CreatedAt:              now,
	})
	svc := service.New(store, service.Options{Now: func() time.Time { return currentNow }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-forms"}
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID:       "ft-leave-legacy",
		TenantID: "tenant-1",
		Key:      "leave-request",
		Name:     "請假申請單",
		Schema: map[string]any{
			"type": "object",
			"workspace_design": map[string]any{
				"enabled": true,
				"fields": []any{
					map[string]any{"id": "subject", "type": "text", "label": "申請主旨", "required": true},
					map[string]any{"id": "needed_at", "type": "datetime", "label": "需求日期", "required": true},
					map[string]any{"id": "description", "type": "textarea", "label": "申請說明", "required": true},
				},
				"stages": []any{
					map[string]any{"id": "stage-manager", "type": "approver", "label": "主管", "detail": "主管審核", "config": map[string]any{"role": "manager"}},
				},
			},
		},
		CreatedAt: now,
	})

	legacyDesign, err := svc.Workspace().WorkspaceFormDesign(ctx)
	if err != nil {
		t.Fatal(err)
	}
	legacyLeave := findPlatformFormDesignForm(t, legacyDesign.Forms, "leave-request")
	legacyFieldIDs := make([]string, 0, len(legacyLeave.Fields))
	for _, field := range legacyLeave.Fields {
		legacyFieldIDs = append(legacyFieldIDs, field.ID)
	}
	for _, requiredID := range []string{"proxy", "leave_type", "start_at", "end_at", "hours", "reason"} {
		if !slices.Contains(legacyFieldIDs, requiredID) {
			t.Fatalf("expected legacy leave fallback to include %s, got %v", requiredID, legacyFieldIDs)
		}
	}
	if slices.Contains(legacyFieldIDs, "subject") {
		t.Fatalf("expected legacy leave fallback to exclude generic subject field, got %v", legacyFieldIDs)
	}

	design, err := svc.Workspace().CreateWorkspaceFormDesign(ctx, domain.SaveWorkspaceFormDesignInput{
		ID:       "custom-approval",
		Icon:     "🧪",
		Name:     "Custom Approval",
		Category: "行政相關",
		Desc:     "first draft",
		Enabled:  boolPtr(true),
		Fields: []domain.PlatformFormBuilderField{
			{ID: "field-subject", Type: "text", Label: "Subject", Placeholder: "Subject", Required: true},
		},
		Stages: []domain.PlatformFormBuilderStage{
			{ID: "stage-manager", Type: "approver", Label: "Manager", Detail: "Manager approves", Config: map[string]any{"role": "manager"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	created := findPlatformFormDesignForm(t, design.Forms, "custom-approval")
	if created.Name != "Custom Approval" || created.Icon != "🧪" || !created.Enabled || created.Flow != "Manager" {
		t.Fatalf("unexpected created form projection: %+v", created)
	}
	if created.UpdatedBy != "Forms Admin" {
		t.Fatalf("expected updater display name, got %q", created.UpdatedBy)
	}
	template, ok, err := store.GetFormTemplateByKey(context.Background(), "tenant-1", "custom-approval")
	if err != nil || !ok {
		t.Fatalf("template lookup failed ok=%v err=%v", ok, err)
	}
	if workspaceDesignFlag(t, template.Schema, "enabled") != true || workspaceDesignFlag(t, template.Schema, "deleted") != false {
		t.Fatalf("expected enabled non-deleted schema, got %+v", template.Schema)
	}

	currentNow = now.Add(time.Hour)
	nextName := "Custom Approval v2"
	disabled := false
	nextDesc := "second draft"
	design, err = svc.Workspace().UpdateWorkspaceFormDesign(ctx, "custom-approval", domain.UpdateWorkspaceFormDesignInput{
		Name:    &nextName,
		Desc:    &nextDesc,
		Enabled: &disabled,
		Stages: &[]domain.PlatformFormBuilderStage{
			{ID: "stage-finance", Type: "approver", Label: "Finance", Detail: "Finance approves", Config: map[string]any{"role": "finance"}},
			{ID: "stage-hr", Type: "notify", Label: "HR", Detail: "Notify HR", Config: map[string]any{"role": "hr"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	updated := findPlatformFormDesignForm(t, design.Forms, "custom-approval")
	if updated.Name != nextName || updated.Desc != nextDesc || updated.Enabled || updated.Flow != "Finance → HR" {
		t.Fatalf("unexpected updated form projection: %+v", updated)
	}

	design, err = svc.Workspace().DeleteWorkspaceFormDesign(ctx, "custom-approval")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := platformFormDesignFormByID(design.Forms, "custom-approval"); ok {
		t.Fatalf("expected soft-deleted form to disappear from projection, got %+v", design.Forms)
	}
	template, ok, err = store.GetFormTemplateByKey(context.Background(), "tenant-1", "custom-approval")
	if err != nil || !ok {
		t.Fatalf("soft-deleted template lookup failed ok=%v err=%v", ok, err)
	}
	if workspaceDesignFlag(t, template.Schema, "deleted") != true {
		t.Fatalf("expected template to be soft-deleted, got %+v", template.Schema)
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findAuditLog(logs, "platform.workspace.form_design.delete"); !ok {
		t.Fatalf("expected form design delete audit log, got %+v", logs)
	}
}

// findWorkspaceOrganizationRow 驗證 find 工作區 organization 列。
func findWorkspaceOrganizationRow(t *testing.T, rows []domain.WorkspaceOrganizationRow, id string) domain.WorkspaceOrganizationRow {
	t.Helper()
	for _, row := range rows {
		if row.ID == id {
			return row
		}
	}
	t.Fatalf("missing organization row %s in %+v", id, rows)
	return domain.WorkspaceOrganizationRow{}
}

// findPlatformFormDesignForm 驗證 find 平臺表單 design 表單。
func findPlatformFormDesignForm(t *testing.T, forms []domain.PlatformFormDesignForm, id string) domain.PlatformFormDesignForm {
	t.Helper()
	form, ok := platformFormDesignFormByID(forms, id)
	if !ok {
		t.Fatalf("missing form design %s in %+v", id, forms)
	}
	return form
}

// platformFormDesignFormByID 驗證平臺表單 design 表單 by ID。
func platformFormDesignFormByID(forms []domain.PlatformFormDesignForm, id string) (domain.PlatformFormDesignForm, bool) {
	for _, form := range forms {
		if form.ID == id {
			return form, true
		}
	}
	return domain.PlatformFormDesignForm{}, false
}

// workspaceDesignFlag 驗證工作區 design flag。
func workspaceDesignFlag(t *testing.T, schema map[string]any, key string) bool {
	t.Helper()
	workspace, ok := schema["workspace_design"].(map[string]any)
	if !ok {
		t.Fatalf("missing workspace_design in schema %+v", schema)
	}
	value, ok := workspace[key].(bool)
	if !ok {
		t.Fatalf("missing boolean %s in workspace_design %+v", key, workspace)
	}
	return value
}

// stringPtr 驗證字串 ptr。
func stringPtr(value string) *string {
	return &value
}

// boolPtr 驗證布林值 ptr。
func boolPtr(value bool) *bool {
	return &value
}

// findPlatformTaskRecord 驗證 find 平臺任務 record。
func findPlatformTaskRecord(t *testing.T, records []domain.PlatformTaskRecord, date string) domain.PlatformTaskRecord {
	t.Helper()
	for _, record := range records {
		if record.Date == date {
			return record
		}
	}
	t.Fatalf("missing task record for %s in %+v", date, records)
	return domain.PlatformTaskRecord{}
}

// findPlatformTaskTodo 驗證 find 平臺任務待辦。
func findPlatformTaskTodo(t *testing.T, todos []domain.PlatformTaskTodo, id string) domain.PlatformTaskTodo {
	t.Helper()
	for _, todo := range todos {
		if todo.ID == id {
			return todo
		}
	}
	t.Fatalf("missing task todo %s in %+v", id, todos)
	return domain.PlatformTaskTodo{}
}

// newServiceFixture 驗證服務 fixture。
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

// newEmployeeFeatureFixture 驗證員工 feature fixture。
func newEmployeeFeatureFixture(t *testing.T, permissions []domain.Permission, options ...service.Options) (*memory.Store, *service.Service, domain.RequestContext) {
	t.Helper()
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-1", TenantID: "tenant-1", Name: "HQ", Path: []string{"ou-1"}, CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:          "ps-employee-feature",
		TenantID:    "tenant-1",
		Name:        "Employee Feature",
		Permissions: permissions,
		CreatedAt:   now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		DisplayName:            "Feature Tester",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-employee-feature"},
		CreatedAt:              now,
	})
	return store, service.New(store, options...), domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
}

// newAttendanceFixture 驗證考勤 fixture。
func newAttendanceFixture(t *testing.T) (*memory.Store, *service.Service, domain.RequestContext, domain.RequestContext, func(time.Time)) {
	t.Helper()
	now := attendanceFixtureClockInTime()
	currentNow := now
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-attendance-self",
		TenantID: "tenant-1",
		Name:     "Attendance Self",
		Permissions: []domain.Permission{
			{Resource: "attendance.clock", Action: "read", Scope: "self"},
			{Resource: "attendance.clock", Action: "create", Scope: "self"},
			{Resource: "attendance.correction", Action: "read", Scope: "self"},
			{Resource: "attendance.correction", Action: "create", Scope: "self"},
			{Resource: "workflow.form_instance", Action: "submit", Scope: "self"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-attendance-admin",
		TenantID: "tenant-1",
		Name:     "Attendance Admin",
		Permissions: []domain.Permission{
			{Resource: "attendance.clock", Action: "read", Scope: "all"},
			{Resource: "attendance.correction", Action: "read", Scope: "all"},
			{Resource: "attendance.correction", Action: "approve", Scope: "all"},
			{Resource: "attendance.correction", Action: "update", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-employee",
		TenantID:               "tenant-1",
		EmployeeID:             "emp-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-attendance-self"},
		CreatedAt:              now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-admin",
		TenantID:               "tenant-1",
		EmployeeID:             "emp-admin",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-attendance-admin"},
		CreatedAt:              now,
	})
	for _, employee := range []domain.Employee{
		{ID: "emp-1", TenantID: "tenant-1", AccountID: "acct-employee", ManagerEmployeeID: "emp-admin", Name: "Employee One", Status: "active", CreatedAt: now},
		{ID: "emp-2", TenantID: "tenant-1", Name: "Employee Two", Status: "active", CreatedAt: now},
		{ID: "emp-admin", TenantID: "tenant-1", AccountID: "acct-admin", Name: "Attendance Admin", Position: "HR", Status: "active", CreatedAt: now},
	} {
		_ = store.UpsertEmployee(context.Background(), employee)
	}
	_ = store.UpsertAttendanceWorksite(context.Background(), domain.AttendanceWorksite{
		ID:           "aws-1",
		TenantID:     "tenant-1",
		Name:         "HQ",
		Address:      "No. 1, HQ Road",
		Latitude:     25.033964,
		Longitude:    121.564468,
		RadiusMeters: 200,
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return currentNow }})
	setNow := func(next time.Time) { currentNow = next.UTC().Truncate(time.Second) }
	return store, svc,
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-employee"},
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"},
		setNow
}

// attendanceFixtureClockInTime 驗證考勤 fixture 打卡 in 時間。
func attendanceFixtureClockInTime() time.Time {
	return time.Date(2026, 6, 10, 1, 0, 0, 0, time.UTC)
}

// attendanceFixtureWorkDateStart returns the instant at which a policy must
// become effective to govern the whole fixture work date. Attendance policy
// resolution is deliberately anchored to the work-date boundary, not the
// punch time or the latest published version.
func attendanceFixtureWorkDateStart() time.Time {
	return time.Date(2026, 6, 10, 0, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
}

// attendanceFixtureClockOutTime 驗證考勤 fixture 打卡 out 時間。
func attendanceFixtureClockOutTime() time.Time {
	return time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC)
}

type recordingObjectStore struct {
	keys    []string
	deleted []string
}

type recordingIdentityProvisioner struct {
	inputs []domain.IdentityProvisioningInput
	err    error
}

// EnsureUser 驗證使用者。
func (p *recordingIdentityProvisioner) EnsureUser(_ context.Context, input domain.IdentityProvisioningInput) (domain.ProvisionedIdentity, error) {
	p.inputs = append(p.inputs, input)
	if p.err != nil {
		return domain.ProvisionedIdentity{}, p.err
	}
	return domain.ProvisionedIdentity{Provider: domain.IdentityProviderKeycloak, Subject: "kc-" + input.AccountID, Email: input.Email}, nil
}

type fakeEHRMSClient struct {
	rows                []domain.EHRMSEmployeeRecord
	departmentRows      []domain.EHRMSDepartmentRecord
	positionRows        []domain.EHRMSPositionRecord
	attendanceRows      []domain.EHRMSAttendanceRecord
	attendanceQueries   *[]domain.EHRMSAttendanceQuery
	leaveTypes          []domain.EHRMSLeaveTypeRecord
	leaveBalances       []domain.EHRMSLeaveBalanceRecord
	leaveBalanceQueries *[]domain.EHRMSAttendanceQuery
	leaveDetails        []domain.EHRMSLeaveDetailRecord
	leaveDetailQueries  *[]domain.EHRMSAttendanceQuery
	err                 error
	departmentsErr      error
	positionsErr        error
	attendanceErr       error
	leaveTypesErr       error
	leaveBalanceErr     error
	leaveDetailErr      error
}

// ListEmployees 驗證員工。
func (c fakeEHRMSClient) ListEmployees(context.Context) ([]domain.EHRMSEmployeeRecord, error) {
	return ehrms.NormalizeEmployeeRecords(c.rows), c.err
}

// ListDepartments 驗證部門。
func (c fakeEHRMSClient) ListDepartments(context.Context) ([]domain.EHRMSDepartmentRecord, error) {
	if len(c.departmentRows) > 0 || c.departmentsErr != nil {
		return ehrms.NormalizeDepartmentRecords(c.departmentRows), c.departmentsErr
	}
	return ehrmsDepartmentsFromEmployees(c.rows), c.departmentsErr
}

// ListPositions 驗證崗位。
func (c fakeEHRMSClient) ListPositions(context.Context) ([]domain.EHRMSPositionRecord, error) {
	if len(c.positionRows) > 0 || c.positionsErr != nil {
		return ehrms.NormalizePositionRecords(c.positionRows), c.positionsErr
	}
	return ehrmsPositionsFromEmployees(c.rows), c.positionsErr
}

// ListAttendance 驗證考勤。
func (c fakeEHRMSClient) ListAttendance(_ context.Context, query domain.EHRMSAttendanceQuery) ([]domain.EHRMSAttendanceRecord, error) {
	if c.attendanceQueries != nil {
		*c.attendanceQueries = append(*c.attendanceQueries, query)
	}
	if c.attendanceErr != nil {
		return nil, c.attendanceErr
	}
	rows := ehrms.NormalizeAttendanceRecords(c.attendanceRows)
	if query.EmployeeID == "" {
		return rows, nil
	}
	filtered := make([]domain.EHRMSAttendanceRecord, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row["員工編號"]) == query.EmployeeID {
			filtered = append(filtered, row)
		}
	}
	return filtered, nil
}

// ListLeaveTypes 驗證假別目錄。
func (c fakeEHRMSClient) ListLeaveTypes(context.Context) ([]domain.EHRMSLeaveTypeRecord, error) {
	if c.leaveTypesErr != nil {
		return nil, c.leaveTypesErr
	}
	return ehrms.NormalizeLeaveTypeRecords(c.leaveTypes), nil
}

// ListLeaveBalances 驗證假別餘額（對應上游 /leave-entitlement）。
func (c fakeEHRMSClient) ListLeaveBalances(_ context.Context, query domain.EHRMSAttendanceQuery) ([]domain.EHRMSLeaveBalanceRecord, error) {
	if c.leaveBalanceQueries != nil {
		*c.leaveBalanceQueries = append(*c.leaveBalanceQueries, query)
	}
	if c.leaveBalanceErr != nil {
		return nil, c.leaveBalanceErr
	}
	rows := ehrms.NormalizeLeaveBalanceRecords(c.leaveBalances)
	if query.EmployeeID == "" {
		return rows, nil
	}
	filtered := make([]domain.EHRMSLeaveBalanceRecord, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row["員工編號"]) == query.EmployeeID {
			filtered = append(filtered, row)
		}
	}
	return filtered, nil
}

// ListLeaveDetails 驗證假別明細（對應上游 /leave）。
func (c fakeEHRMSClient) ListLeaveDetails(_ context.Context, query domain.EHRMSAttendanceQuery) ([]domain.EHRMSLeaveDetailRecord, error) {
	if c.leaveDetailQueries != nil {
		*c.leaveDetailQueries = append(*c.leaveDetailQueries, query)
	}
	if c.leaveDetailErr != nil {
		return nil, c.leaveDetailErr
	}
	rows := ehrms.NormalizeLeaveDetailRecords(c.leaveDetails)
	if query.EmployeeID == "" {
		return rows, nil
	}
	filtered := make([]domain.EHRMSLeaveDetailRecord, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row["員工編號"]) == query.EmployeeID {
			filtered = append(filtered, row)
		}
	}
	return filtered, nil
}

func ehrmsDepartmentsFromEmployees(rows []domain.EHRMSEmployeeRecord) []domain.EHRMSDepartmentRecord {
	normalized := ehrms.NormalizeEmployeeRecords(rows)
	byCode := map[string]domain.EHRMSDepartmentRecord{}
	codes := map[string]struct{}{}
	for _, row := range normalized {
		code := strings.TrimSpace(firstNonEmpty(row["部門代碼"], row["dept_code"]))
		if code == "" {
			continue
		}
		codes[code] = struct{}{}
		byCode[code] = domain.EHRMSDepartmentRecord{
			"部門代碼":   code,
			"部門中文名稱": firstNonEmpty(row["部門中文名稱"], row["dept_name_zh"]),
			"部門英文名稱": firstNonEmpty(row["部門英文名稱"], row["dept_name_en"]),
		}
	}
	out := make([]domain.EHRMSDepartmentRecord, 0, len(byCode))
	for code, record := range byCode {
		parent := ""
		for length := len(code) - 1; length > 0; length-- {
			prefix := code[:length]
			if _, ok := codes[prefix]; ok {
				parent = prefix
				break
			}
		}
		if parent != "" {
			record["上級部門代碼"] = parent
		}
		out = append(out, record)
	}
	return out
}

func ehrmsPositionsFromEmployees(rows []domain.EHRMSEmployeeRecord) []domain.EHRMSPositionRecord {
	normalized := ehrms.NormalizeEmployeeRecords(rows)
	byCode := map[string]domain.EHRMSPositionRecord{}
	for _, row := range normalized {
		code := strings.TrimSpace(firstNonEmpty(row["職務代碼"], row["job_code"]))
		if code == "" {
			continue
		}
		if _, ok := byCode[code]; ok {
			continue
		}
		byCode[code] = domain.EHRMSPositionRecord{
			"職務代碼":   code,
			"職務中文名稱": firstNonEmpty(row["職務中文名稱"], row["job_title_zh"]),
			"職務英文名稱": firstNonEmpty(row["職務英文名稱"], row["job_title_en"]),
		}
	}
	out := make([]domain.EHRMSPositionRecord, 0, len(byCode))
	for _, record := range byCode {
		out = append(out, record)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// PutObject 驗證 put 物件。
func (s *recordingObjectStore) PutObject(_ context.Context, key string, _ string, _ []byte) error {
	s.keys = append(s.keys, key)
	return nil
}

func (s *recordingObjectStore) GetObject(_ context.Context, key string) ([]byte, error) {
	return nil, fmt.Errorf("object %s not recorded for download", key)
}

// DeleteObject 驗證物件。
func (s *recordingObjectStore) DeleteObject(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return nil
}

// Provider 驗證提供者。
func (s *recordingObjectStore) Provider() string {
	return "test"
}

// Bucket 驗證 bucket。
func (s *recordingObjectStore) Bucket() string {
	return "imports"
}

// validEmployeeInput 驗證有效員工輸入。
func validEmployeeInput(employeeNo, name, email string) domain.CreateEmployeeInput {
	return domain.CreateEmployeeInput{
		EmployeeNo:            employeeNo,
		Name:                  name,
		CompanyEmail:          email,
		OrgUnitID:             "ou-1",
		Position:              "Engineer",
		Category:              "full_time",
		Status:                "active",
		EmploymentStatus:      "active",
		HireDate:              "2026-06-01",
		BasicInfo:             map[string]any{"nationality_type": "local", "national_id": "A123456789"},
		EmploymentInfo:        map[string]any{"org_unit_id": "ou-1", "position": "Engineer", "category": "full_time"},
		EducationMilitaryInfo: map[string]any{"highest_education": "master", "school": "NTU"},
		ContactInfo:           validContactInfo(),
		InsuranceInfo:         validInsuranceInfo(),
	}
}

// validContactInfo 驗證有效聯絡 info。
func validContactInfo() map[string]any {
	return map[string]any{
		"mobile_phone":               "0911222333",
		"address":                    "Taipei",
		"emergency_contact_relation": "spouse",
		"emergency_contact_name":     "Emergency Contact",
		"emergency_contact_phone":    "0922333444",
	}
}

// validInsuranceInfo 驗證有效保險 info。
func validInsuranceInfo() map[string]any {
	return map[string]any{
		"labor_insurance_date":    "2026-06-01",
		"labor_insurance_level":   "L1",
		"labor_insurance_salary":  "45800",
		"health_insurance_date":   "2026-06-01",
		"health_insurance_level":  "H1",
		"health_insurance_amount": "826",
	}
}

type recordingAuthzSnapshot struct {
	values      map[string]domain.CheckResult
	expirations map[string]time.Time
	ttls        []time.Duration
	now         func() time.Time
	gets        int
	sets        int
}

// GetAuthzSnapshot 驗證授權快照。
func (s *recordingAuthzSnapshot) GetAuthzSnapshot(_ context.Context, key string) (domain.CheckResult, bool, error) {
	s.gets++
	if expiresAt, ok := s.expirations[key]; ok && !expiresAt.After(s.currentTime()) {
		delete(s.values, key)
		delete(s.expirations, key)
		return domain.CheckResult{}, false, nil
	}
	result, ok := s.values[key]
	return result, ok, nil
}

// SetAuthzSnapshot 驗證集合授權快照。
func (s *recordingAuthzSnapshot) SetAuthzSnapshot(_ context.Context, key string, result domain.CheckResult, ttl time.Duration) error {
	s.sets++
	if s.values == nil {
		s.values = map[string]domain.CheckResult{}
	}
	if s.expirations == nil {
		s.expirations = map[string]time.Time{}
	}
	s.values[key] = result
	s.expirations[key] = s.currentTime().Add(ttl)
	s.ttls = append(s.ttls, ttl)
	return nil
}

// currentTime 回傳快照測試使用的可控時鐘。
func (s *recordingAuthzSnapshot) currentTime() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// InvalidateTenant 驗證 invalidate 租戶。
func (s *recordingAuthzSnapshot) InvalidateTenant(_ context.Context, tenantID string) error {
	for key := range s.values {
		if strings.Contains(key, tenantID) {
			delete(s.values, key)
			delete(s.expirations, key)
		}
	}
	return nil
}

type fixedRelationshipChecker struct {
	allowed bool
	checks  []domain.RelationshipCheck
}

// CheckRelationship 驗證關係。
func (c *fixedRelationshipChecker) CheckRelationship(_ context.Context, check domain.RelationshipCheck) (bool, error) {
	c.checks = append(c.checks, check)
	return c.allowed, nil
}

type mappedRelationshipChecker struct {
	allowed map[string]bool
	checks  []domain.RelationshipCheck
	err     error
}

// CheckRelationship 驗證 map 型關係 checker。
func (c *mappedRelationshipChecker) CheckRelationship(_ context.Context, check domain.RelationshipCheck) (bool, error) {
	c.checks = append(c.checks, check)
	if c.err != nil {
		return false, c.err
	}
	return c.allowed[relationshipCheckKey(check.Subject, check.Relation, check.Object)], nil
}

func relationshipCheckKey(subject, relation, object string) string {
	return subject + "|" + relation + "|" + object
}

func relationshipCheckSeen(checks []domain.RelationshipCheck, relation, object string) bool {
	for _, check := range checks {
		if check.Relation == relation && check.Object == object {
			return true
		}
	}
	return false
}

// hasBusinessOutboxEvent 驗證 business outbox 事件。
func hasBusinessOutboxEvent(events []domain.OutboxEvent, eventType string) bool {
	for _, event := range events {
		if event.EventType == eventType {
			return true
		}
	}
	return false
}

// relationshipTupleExists 驗證 relationship tuple 是否存在。
func relationshipTupleExists(tuples []domain.AuthzRelationshipTuple, relation, subjectType, subjectID string) bool {
	for _, tuple := range tuples {
		if tuple.Relation == relation && tuple.SubjectType == subjectType && tuple.SubjectID == subjectID {
			return true
		}
	}
	return false
}

// findAuditLog 驗證 find 稽覈 log。
func findAuditLog(logs []domain.AuditLog, action string) (domain.AuditLog, bool) {
	for _, log := range logs {
		if log.Action == action {
			return log, true
		}
	}
	return domain.AuditLog{}, false
}

// allowEmployeeSensitiveFieldsForPermission 驗證員工 sensitive 欄位 for 權限。
func allowEmployeeSensitiveFieldsForPermission(t *testing.T, store *memory.Store, now time.Time, permissionID string) {
	t.Helper()
	for _, field := range []string{
		"personal_email",
		"phone",
		"mobile_phone",
		"address",
		"communication_address",
		"emergency_contact_name",
		"emergency_name",
		"emergency_contact_phone",
		"emergency_phone",
		"national_id",
		"passport_no",
		"arc_no",
		"tax_id",
		"work_permit_no",
		"insurance_info",
		"labor_insurance_salary",
		"health_insurance_amount",
	} {
		if err := store.UpsertFieldPolicy(context.Background(), domain.FieldPolicy{
			ID:              "fp-allow-" + strings.ReplaceAll(permissionID, ".", "-") + "-" + field,
			TenantID:        "tenant-1",
			ApplicationCode: "hr",
			ResourceType:    "employee",
			FieldName:       field,
			Effect:          "allow",
			PermissionID:    permissionID,
			CreatedAt:       now,
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// allowAttendanceClockSensitiveFieldsForPermission 驗證考勤打卡 sensitive 欄位 for 權限。
func allowAttendanceClockSensitiveFieldsForPermission(t *testing.T, store *memory.Store, now time.Time, permissionID string) {
	t.Helper()
	for _, field := range []string{
		"latitude",
		"longitude",
		"accuracy_meters",
		"distance_meters",
		"device_id",
		"device_info",
		"location_source",
	} {
		if err := store.UpsertFieldPolicy(context.Background(), domain.FieldPolicy{
			ID:              "fp-allow-" + strings.ReplaceAll(permissionID, ".", "-") + "-" + field,
			TenantID:        "tenant-1",
			ApplicationCode: "attendance",
			ResourceType:    "clock",
			FieldName:       field,
			Effect:          "allow",
			PermissionID:    permissionID,
			CreatedAt:       now,
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// stringSliceContains 驗證字串 slice contains。
func stringSliceContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

// fieldErrorsContain 驗證欄位錯誤 contain。
func fieldErrorsContain(fields []domain.FieldError, expectedField string) bool {
	for _, field := range fields {
		if field.Field == expectedField {
			return true
		}
	}
	return false
}

// testPNGBytes 驗證 png bytes。
func testPNGBytes() []byte {
	return []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0}
}
