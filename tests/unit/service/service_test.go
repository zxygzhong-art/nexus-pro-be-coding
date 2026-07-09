package service_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/platform/ehrms"
	"nexus-pro-be/internal/repository"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
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

// TestRouteApprovalPolicyUsesMatchedHTTPRoute 驗證路由核准政策 uses matched HTTP 路由。
func TestRouteApprovalPolicyUsesMatchedHTTPRoute(t *testing.T) {
	svc, ctx := newServiceFixture([]domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
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
	if !matched.RequiresApproval || matched.ApprovalReason != "route_policy" {
		t.Fatalf("expected matched import route to require approval, got %+v", matched)
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
	if mismatched.RequiresApproval {
		t.Fatalf("unexpected approval requirement for unmatched HTTP route: %+v", mismatched)
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
	if !attendanceImport.RequiresApproval || attendanceImport.RiskLevel != string(domain.RiskHigh) || attendanceImport.ApprovalReason != "route_policy" {
		t.Fatalf("expected eHRMS attendance sync route to require high-risk approval, got %+v", attendanceImport)
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

// TestEmployeeQuerySkipsSuccessfulReadAudit 驗證成功 query 不寫入操作稽核，export 仍保留。
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

	if _, err := svc.HR().ExportEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}); err != nil {
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

// TestCreateAgentRunReturnsPlaceholderWithoutKnowledgeArticles 驗證 agent 執行在沒有知識文章表時回傳占位回答。
func TestCreateAgentRunReturnsPlaceholderWithoutKnowledgeArticles(t *testing.T) {
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

	run, err := service.New(store).Agent().CreateRun(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}, domain.CreateAgentRunInput{Prompt: "请"})
	if err != nil {
		t.Fatal(err)
	}
	if len(run.References) != 0 {
		t.Fatalf("expected no knowledge references, got %+v", run.References)
	}
	if !strings.Contains(run.Answer, "没有可检索的知识库内容") {
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

	_, err := service.New(store).Agent().CreateRun(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}, domain.CreateAgentRunInput{Prompt: "请假"})
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
	runs, err := svc.Agent().ListRuns(ownCtx)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != "run-owner" {
		t.Fatalf("expected own-scoped list to return only owner run, got %+v", runs)
	}
	page, err := svc.Agent().ListRunPage(ownCtx, domain.PageRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != "run-owner" {
		t.Fatalf("expected own-scoped page to return only owner run, got %+v", page)
	}

	allRuns, err := svc.Agent().ListRuns(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"})
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
	session, err := svc.IAM().AssumeRole(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}, "role-hr", domain.AssumeRoleInput{
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
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}

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
	session, err := svc.IAM().AssumeRole(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}, "role-hr", domain.AssumeRoleInput{Reason: "test snapshot bypass"})
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

	page, err := service.New(store).HR().ListOrgUnitPage(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, domain.PageRequest{Page: 1, PageSize: 10, Sort: "created_at_asc"})
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
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", RequestID: "req-field-policy", TraceID: "trace-field-policy", ApprovalConfirmed: true}

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
	_ = store.UpsertFieldPolicy(context.Background(), domain.FieldPolicy{
		ID:              "fp-export-phone",
		TenantID:        "tenant-1",
		ApplicationCode: "hr",
		ResourceType:    "employee",
		FieldName:       "phone",
		Effect:          "hide",
		PermissionID:    "hr.employee.export",
		CreatedAt:       now,
	})
	allowEmployeeSensitiveFieldsForPermission(t, store, now, "hr.employee.read")
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-hr"}, CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", EmployeeNo: "E0001", Name: "Employee One", Phone: "0912345678", Status: "active", CreatedAt: now})
	svc := service.New(store)

	page, err := svc.HR().QueryEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, domain.EmployeeQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Phone != "0912345678" {
		t.Fatalf("expected read permission to keep phone visible, got %+v", page.Items)
	}

	exportCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}
	exported, err := svc.HR().ExportEmployees(exportCtx)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported) != 1 || exported[0].Phone != "" {
		t.Fatalf("expected JSON export to hide phone, got %+v", exported)
	}
	raw, _, err := svc.HR().ExportEmployeesCSV(exportCtx, domain.EmployeeQuery{})
	if err != nil {
		t.Fatal(err)
	}
	csvBody := string(raw)
	if strings.Contains(csvBody, "電話") || strings.Contains(csvBody, "0912345678") {
		t.Fatalf("expected CSV export to omit hidden phone column/value, got %q", csvBody)
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

	raw, _, err := service.New(store).HR().ExportEmployeesCSV(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}, domain.EmployeeQuery{})
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

// TestEmployeeExportRequiresApprovalAndAudits 驗證員工 export requires 核准 and audits。
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

	_, err := svc.HR().ExportEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
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

	items, err := svc.HR().ExportEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "emp-1" {
		t.Fatalf("expected confirmed export result, got %+v", items)
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

	_, err := service.New(store).HR().ExportEmployees(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true})
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
	_ = store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{ID: "lb-1", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", RemainingHours: 8, UpdatedAt: now})
	_ = store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{ID: "lb-2", TenantID: "tenant-1", EmployeeID: "emp-2", LeaveType: "annual", RemainingHours: 8, UpdatedAt: now})
	_ = store.UpsertLeaveRequest(context.Background(), domain.LeaveRequest{ID: "lr-1", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", Hours: 8, Status: "pending", CreatedAt: now})
	_ = store.UpsertLeaveRequest(context.Background(), domain.LeaveRequest{ID: "lr-2", TenantID: "tenant-1", EmployeeID: "emp-2", LeaveType: "annual", Hours: 8, Status: "pending", CreatedAt: now.Add(time.Minute)})

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
}

// TestAttendanceShiftAssignmentRejectsOverlappingActiveRange 驗證考勤班別指派 rejects overlapping 啟用中 range。
func TestAttendanceShiftAssignmentRejectsOverlappingActiveRange(t *testing.T) {
	store, svc, _, adminCtx, _ := newAttendanceFixture(t)
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-attendance-admin",
		TenantID: "tenant-1",
		Name:     "Attendance Admin",
		Permissions: []domain.Permission{
			{Resource: "attendance.shift_assignment", Action: "create", Scope: "all"},
			{Resource: "attendance.shift_assignment", Action: "read", Scope: "all"},
		},
		CreatedAt: attendanceFixtureClockInTime(),
	})

	created, err := svc.Attendance().CreateAttendanceShiftAssignment(adminCtx, domain.CreateAttendanceShiftAssignmentInput{
		EmployeeID:    "emp-1",
		ShiftID:       "ash-1",
		WorksiteID:    "aws-1",
		EffectiveFrom: "2026-06-01T00:00:00Z",
		EffectiveTo:   "2026-06-08T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.EmployeeID != "emp-1" || created.Status != "active" {
		t.Fatalf("unexpected non-overlapping assignment: %+v", created)
	}

	_, err = svc.Attendance().CreateAttendanceShiftAssignment(adminCtx, domain.CreateAttendanceShiftAssignmentInput{
		EmployeeID:    "emp-1",
		ShiftID:       "ash-1",
		WorksiteID:    "aws-1",
		EffectiveFrom: "2026-06-10T00:00:00Z",
	})
	if err == nil {
		t.Fatal("expected overlapping active shift assignment to be rejected")
	}
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Status != 400 || !strings.Contains(appErr.Message, "overlaps existing assignment") {
		t.Fatalf("expected overlap bad request, got %v", err)
	}

	assignments, err := store.ListAttendanceShiftAssignments(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 3 {
		t.Fatalf("overlapping assignment should not be stored, got %+v", assignments)
	}
}

// TestAttendanceClockRecordsAcceptedRejectedAndDuplicate 驗證考勤打卡 records accepted rejected and duplicate。
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
	stored, ok, err := store.GetAcceptedAttendanceClockRecord(context.Background(), "tenant-1", "emp-1", accepted.WorkDate, "clock_in")
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
	duplicate, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction: "clock_in",
		Latitude:  25.033964,
		Longitude: 121.564468,
	})
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.RecordStatus != "rejected" || duplicate.RejectionReason != "duplicate" {
		t.Fatalf("expected duplicate rejected attempt, got %+v", duplicate)
	}

	setNow(attendanceFixtureClockOutTime())
	status, err := svc.Attendance().AttendanceClockStatus(employeeCtx)
	if err != nil {
		t.Fatal(err)
	}
	if status.NextAction != "clock_out" || status.ClockIn == nil || status.ClockOut != nil {
		t.Fatalf("unexpected clock status: %+v", status)
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
	raw, ok, err := store.GetAcceptedAttendanceClockRecord(context.Background(), "tenant-1", "emp-1", created.WorkDate, "clock_in")
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

// TestAttendanceClockRejectsWindowSequenceAndLowAccuracy 驗證考勤打卡 rejects window sequence and low accuracy。
func TestAttendanceClockRejectsWindowSequenceAndLowAccuracy(t *testing.T) {
	_, svc, employeeCtx, _, setNow := newAttendanceFixture(t)

	setNow(time.Date(2026, 6, 9, 23, 30, 0, 0, time.UTC))
	outsideWindow, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction: "clock_in",
		Latitude:  25.033964,
		Longitude: 121.564468,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outsideWindow.WorkDate != "2026-06-10" || outsideWindow.RecordStatus != "rejected" || outsideWindow.RejectionReason != "outside_time_window" {
		t.Fatalf("expected local-date outside-window rejection, got %+v", outsideWindow)
	}

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

// TestAttendanceClockSupportsOvernightShiftWorkDate 驗證考勤打卡 supports overnight 班別 work 日期。
func TestAttendanceClockSupportsOvernightShiftWorkDate(t *testing.T) {
	store, svc, employeeCtx, _, setNow := newAttendanceFixture(t)
	now := attendanceFixtureClockInTime()
	_ = store.UpsertAttendanceShift(context.Background(), domain.AttendanceShift{
		ID:            "ash-1",
		TenantID:      "tenant-1",
		Name:          "Night Shift",
		ClockInStart:  "22:00",
		ClockInEnd:    "23:59",
		ClockOutStart: "05:00",
		ClockOutEnd:   "07:00",
		Status:        "active",
		CreatedAt:     now,
		UpdatedAt:     now,
	})

	setNow(time.Date(2026, 6, 10, 14, 30, 0, 0, time.UTC))
	clockIn, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_in",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	if clockIn.WorkDate != "2026-06-10" || clockIn.RecordStatus != "accepted" {
		t.Fatalf("expected accepted night clock-in on 2026-06-10, got %+v", clockIn)
	}

	setNow(time.Date(2026, 6, 10, 22, 0, 0, 0, time.UTC))
	clockOut, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
		Direction:      "clock_out",
		Latitude:       25.033964,
		Longitude:      121.564468,
		AccuracyMeters: 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	if clockOut.WorkDate != clockIn.WorkDate || clockOut.RecordStatus != "accepted" {
		t.Fatalf("expected accepted night clock-out on same work date, got in=%+v out=%+v", clockIn, clockOut)
	}
	status, err := svc.Attendance().AttendanceClockStatus(employeeCtx)
	if err != nil {
		t.Fatal(err)
	}
	if status.WorkDate != "2026-06-10" || status.NextAction != "complete" || status.ClockIn == nil || status.ClockOut == nil {
		t.Fatalf("expected completed night-shift status for previous work date, got %+v", status)
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

// TestAttendanceCorrectionApproveCreatesManualRecordAndRejectDoesNot 驗證考勤 correction 核准 creates manual record and 駁回 does not。
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
	record, ok, err := store.GetAcceptedAttendanceClockRecord(context.Background(), "tenant-1", "emp-1", approved.WorkDate, "clock_in")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || record.Source != "manual_correction" || record.CorrectionRequestID != approved.ID {
		t.Fatalf("expected accepted manual correction record, got ok=%v record=%+v", ok, record)
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
	if _, ok, err := store.GetAcceptedAttendanceClockRecord(context.Background(), "tenant-1", "emp-1", rejected.WorkDate, "clock_out"); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("rejecting a correction should not create an accepted clock-out record")
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
	_ = store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{ID: "lb-1", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", RemainingHours: 16, UpdatedAt: now})

	created, err := service.New(store).Attendance().CreateLeaveRequest(
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
	_ = store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{ID: "lb-1", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", RemainingHours: 24, UpdatedAt: now})
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
	startWorkflowRunForTest(t, svc, store, "tenant-1", approvedRequest.FormInstanceID, "acct-employee")
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
	balance, ok, err := store.GetLeaveBalance(context.Background(), "tenant-1", "lb-1")
	if err != nil || !ok {
		t.Fatalf("leave balance missing ok=%v err=%v", ok, err)
	}
	if balance.RemainingHours != 16 {
		t.Fatalf("approval should keep reserved hours deducted, got %v", balance.RemainingHours)
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
	startWorkflowRunForTest(t, svc, store, "tenant-1", rejectedRequest.FormInstanceID, "acct-employee")
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
	balance, ok, err = store.GetLeaveBalance(context.Background(), "tenant-1", "lb-1")
	if err != nil || !ok {
		t.Fatalf("leave balance missing ok=%v err=%v", ok, err)
	}
	if balance.RemainingHours != 16 {
		t.Fatalf("rejection should release reserved hours, got %v", balance.RemainingHours)
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
	record, ok, err := store.GetAcceptedAttendanceClockRecord(context.Background(), "tenant-1", "emp-1", stored.WorkDate, "clock_in")
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
	if _, ok, err := store.GetAcceptedAttendanceClockRecord(context.Background(), "tenant-1", "emp-1", stored.WorkDate, "clock_in"); err != nil {
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
		Name:      "加班核准申請單",
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
	startWorkflowRunForTest(t, svc, store, "tenant-1", approvedRequest.FormInstanceID, "acct-employee")
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
	compensatory := 0.0
	for _, balance := range balances {
		if balance.EmployeeID == "emp-1" && balance.LeaveType == "compensatory" {
			compensatory = balance.RemainingHours
		}
	}
	if compensatory != 3 {
		t.Fatalf("expected 3 compensatory hours credited, got %v", compensatory)
	}

	rejectedRequest, err := svc.Attendance().CreateOvertimeRequest(employeeCtx, domain.CreateOvertimeRequestInput{
		StartAt: "2026-06-13T18:00:00Z",
		EndAt:   "2026-06-13T20:00:00Z",
		Hours:   2,
	})
	if err != nil {
		t.Fatal(err)
	}
	startWorkflowRunForTest(t, svc, store, "tenant-1", rejectedRequest.FormInstanceID, "acct-employee")
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
	compensatory = 0.0
	for _, balance := range balances {
		if balance.EmployeeID == "emp-1" && balance.LeaveType == "compensatory" {
			compensatory = balance.RemainingHours
		}
	}
	if compensatory != 3 {
		t.Fatalf("rejection should not change compensatory hours, got %v", compensatory)
	}
}

// TestCreateLeaveRequestRejectsInsufficientLeaveBalance 驗證請假請求 rejects insufficient 請假 balance。
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

	_, err := service.New(store).Attendance().CreateLeaveRequest(
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

// TestWorkflowDraftLifecycleAndPlatformProjection 驗證流程草稿生命週期 and 平台 projection。
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
		Name:      "请假申请单",
		Schema:    workflowEnabledTemplateSchema(),
		CreatedAt: now,
	})
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return now.Add(time.Hour) }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-self"}

	draft, err := svc.Workflow().SaveFormDraft(ctx, domain.SaveFormDraftInput{
		TemplateKey: "leave-request",
		Payload:     map[string]any{"desc": "draft leave"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Status != "draft" {
		t.Fatalf("expected draft status, got %+v", draft)
	}
	updated, err := svc.Workflow().UpdateFormDraft(ctx, draft.ID, domain.UpdateFormDraftInput{
		Payload: map[string]any{"desc": "updated leave"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Payload["desc"] != "updated leave" {
		t.Fatalf("expected updated payload, got %+v", updated.Payload)
	}
	forms, err := svc.Platform().Forms(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(forms.Drafts) != 1 || forms.Drafts[0].ID != draft.ID || forms.Drafts[0].Summary != "updated leave" {
		t.Fatalf("expected draft projection, got %+v", forms.Drafts)
	}

	submitted, err := svc.Workflow().SubmitForm(ctx, domain.SubmitFormInput{
		TemplateKey: draft.ID,
		Payload:     map[string]any{"desc": "submitted leave"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if submitted.ID != draft.ID || submitted.Status != "in_review" || submitted.Payload["desc"] != "submitted leave" {
		t.Fatalf("expected submitted draft, got %+v", submitted)
	}
	forms, err = svc.Platform().Forms(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(forms.Drafts) != 0 || len(forms.Applications) != 1 || forms.Applications[0].ID != draft.ID {
		t.Fatalf("expected one application and no drafts, got applications=%+v drafts=%+v", forms.Applications, forms.Drafts)
	}

	duplicate, err := svc.Workflow().DuplicateForm(ctx, submitted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Status != "draft" || duplicate.ID == submitted.ID || duplicate.Payload["desc"] != "submitted leave" {
		t.Fatalf("expected duplicated draft, got %+v", duplicate)
	}
	exported, err := svc.Workflow().ExportForm(ctx, submitted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if exported.FileName == "" || !strings.Contains(string(exported.Body), "submitted leave") {
		t.Fatalf("expected exported JSON to include submitted payload, got name=%q body=%s", exported.FileName, string(exported.Body))
	}
	cancelled, err := svc.Workflow().CancelForm(ctx, submitted.ID, domain.CancelFormInput{Reason: "no longer needed"})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("expected cancelled status, got %+v", cancelled)
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
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID:        "ft-leave",
		TenantID:  "tenant-1",
		Key:       "leave-request",
		Name:      "请假申请单",
		Schema:    workflowEnabledTemplateSchema("acct-admin"),
		CreatedAt: now,
	})
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return now.Add(time.Hour) }})
	applicantCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-applicant"}
	submitted, err := svc.Workflow().SubmitForm(applicantCtx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload:     map[string]any{"desc": "申请一天特休", "notify_account_ids": []any{"acct-admin"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}

	queue, err := svc.Workflow().ReviewQueue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.PendingReview) != 1 || len(queue.Notified) != 1 {
		t.Fatalf("expected one pending and notified item, got %+v", queue)
	}
	if queue.PendingReview[0].Title != "请假申请单" || queue.PendingReview[0].Desc != "申请一天特休" {
		t.Fatalf("unexpected review projection: %+v", queue.PendingReview[0])
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
		Name:      "通用签呈",
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
		Name:      "通用签呈",
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
	if item := adminNotifications.Items[0]; item.StatusText != "待處理" || item.LinkURL != "/notifications?reviewId="+submitted.ID || !strings.Contains(item.Body, "通用签呈") {
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
	if item := applicantAfterReview.Items[0]; item.Tone != "success" || item.StatusText != "已核准" || item.LinkURL != "/forms?applicationId="+submitted.ID || !strings.Contains(item.Body, "looks good") {
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
		Name:      "通用签呈",
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
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}

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
	ctx.ApprovalConfirmed = true
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

// TestEmployeeImportPreviewConfirmAndStatusTransition 驗證員工 import preview confirm and 狀態轉換。
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

	session, err := svc.HR().PreviewEmployeeImport(ctx, domain.EmployeeImportPreviewInput{
		Filename: "employees.csv",
		Content:  "員工編號,姓名,Email,部門,職位,類別,電話,狀態,到職日期,主管員工ID\nE2001,Partial Wu,partial@example.com,ou-1,HRBP,全職,0911000222,在職,2026-06-01,\nE2001,Duplicate,dup@example.com,ou-1,HRBP,全職,0911000333,在職,2026-06-01,\n",
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
	if session.ObjectProvider != "test" || session.ObjectBucket != "imports" || session.SizeBytes == 0 || session.SHA256 == "" || session.CreatedByAccountID != "acct-1" {
		t.Fatalf("expected object metadata on preview session, got %+v", session)
	}

	_, err = svc.HR().ConfirmEmployeeImport(ctx, session.ID, domain.EmployeeImportConfirmInput{Mode: "create"})
	if err == nil {
		t.Fatal("expected all-or-nothing import confirmation to reject invalid preview rows")
	}
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Code != "import_validation_failed" || len(appErr.RowErrors) == 0 {
		t.Fatalf("expected import_validation_failed with row errors, got %v", err)
	}
	failedSession, ok, err := store.GetEmployeeImportSession(context.Background(), "tenant-1", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || failedSession.Status != "failed_validation" || failedSession.Summary["failed"] != 2 {
		t.Fatalf("expected failed validation session summary, got ok=%v session=%+v", ok, failedSession)
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if failedLog, ok := findAuditLog(logs, "hr.employee.import.confirm_failed"); !ok || failedLog.Details["object_key"] != failedSession.ObjectKey {
		t.Fatalf("expected failed import audit with object metadata, got %+v", logs)
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

	session, err = svc.HR().PreviewEmployeeImport(ctx, domain.EmployeeImportPreviewInput{
		Filename: "employees.csv",
		Content:  "員工編號,姓名,Email,部門,職位,類別,電話,狀態,到職日期,主管員工ID\n,Lina Wu,lina@example.com,ou-1,HRBP,約聘,0911000222,在職,2026-06-01,\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := svc.HR().ConfirmEmployeeImport(ctx, session.ID, domain.EmployeeImportConfirmInput{Mode: "create"})
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Summary["confirmed"] != 1 {
		t.Fatalf("expected one confirmed import, got %+v", confirmed.Summary)
	}
	events, err := store.ListOutboxEvents(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if !hasBusinessOutboxEvent(events, string(domain.EventEmployeeImported)) {
		t.Fatalf("expected employee.imported outbox event, got %+v", events)
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

	transitioned, err := svc.HR().TransitionEmployeeStatus(ctx, imported.ID, domain.StatusTransitionInput{
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

// TestEmployeeImportConfirmProvisionsKeycloakIdentityForAccountPolicy 驗證員工 import confirm provisions Keycloak 身分 for 帳號政策。
func TestEmployeeImportConfirmProvisionsKeycloakIdentityForAccountPolicy(t *testing.T) {
	provisioner := &recordingIdentityProvisioner{}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
	}, service.Options{IdentityProvisioner: provisioner})
	ctx.ApprovalConfirmed = true

	session, err := svc.HR().PreviewEmployeeImport(ctx, domain.EmployeeImportPreviewInput{
		Filename: "employees.csv",
		Content:  "員工編號,姓名,Email,部門,職位,類別,電話,狀態,到職日期,主管員工ID,帳號策略\nE2101,Import Login,import.login@example.com,ou-1,HRBP,全職,0911000444,在職,2026-06-01,,create_pending_invite\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := svc.HR().ConfirmEmployeeImport(ctx, session.ID, domain.EmployeeImportConfirmInput{Mode: "create"})
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Summary["confirmed"] != 1 || len(provisioner.inputs) != 1 || !provisioner.inputs[0].SendInvite {
		t.Fatalf("expected import confirmation to provision one invited keycloak user, summary=%+v inputs=%+v", confirmed.Summary, provisioner.inputs)
	}

	employee, ok, err := store.GetEmployeeByEmployeeNo(context.Background(), "tenant-1", "E2101")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || employee.AccountID == "" {
		t.Fatalf("expected imported employee with account id, ok=%v employee=%+v", ok, employee)
	}
	account, ok, err := store.GetAccount(context.Background(), "tenant-1", employee.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || account.Status != "pending_invite" || account.EmployeeID != employee.ID {
		t.Fatalf("expected pending invite account from import, ok=%v account=%+v", ok, account)
	}
	identities, err := store.ListUserIdentities(context.Background(), "tenant-1", employee.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(identities) != 1 || identities[0].Subject != "kc-"+employee.AccountID || identities[0].Email != "import.login@example.com" {
		t.Fatalf("expected keycloak identity binding for imported employee, got %+v", identities)
	}
}

// TestEmployeeImportPreviewCleansObjectWhenSessionPersistenceFails 驗證員工 import preview cleans 物件 when session persistence fails。
func TestEmployeeImportPreviewCleansObjectWhenSessionPersistenceFails(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	base := memory.NewStore()
	store := &failingEmployeeImportSessionStore{Store: base}
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
	objects := &recordingObjectStore{}
	svc := service.New(store, service.Options{ObjectStore: objects})

	_, err := svc.HR().PreviewEmployeeImport(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}, domain.EmployeeImportPreviewInput{
		Filename: "employees.csv",
		Content:  "員工編號,姓名,Email,部門,職位,類別,電話,狀態,到職日期,主管員工ID\nE9001,Cleanup Target,cleanup@example.com,ou-1,HRBP,全職,0911000222,在職,2026-06-01,\n",
	})
	if err == nil || !strings.Contains(err.Error(), "session persistence failed") {
		t.Fatalf("expected session persistence failure, got %v", err)
	}
	if len(objects.keys) != 1 || len(objects.deleted) != 1 || objects.deleted[0] != objects.keys[0] {
		t.Fatalf("expected stored import object to be deleted on failure, keys=%+v deleted=%+v", objects.keys, objects.deleted)
	}
}

// TestEmployeeImportConfirmRevalidatesBatchDuplicates 驗證員工 import confirm revalidates 批次 duplicates。
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
	_, err := service.New(store).HR().ConfirmEmployeeImport(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}, session.ID, domain.EmployeeImportConfirmInput{Mode: "create"})
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

// TestSyncEHRMSEmployeesCreatesEmployeesAndDepartments 驗證 eHRMS 員工 creates 員工 and departments。
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
	ctx.ApprovalConfirmed = true

	result, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{})
	if err != nil {
		t.Fatalf("%v result=%+v", err, result)
	}
	if result.Fetched != 1 || result.Created != 1 || result.Updated != 0 || result.DepartmentsUpserted != 1 || result.PositionsUpserted != 1 || result.Mode != "upsert" {
		t.Fatalf("unexpected eHRMS sync result: %+v", result)
	}
	unit, ok, err := store.GetOrgUnit(context.Background(), "tenant-1", "M0101")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || unit.Name != "Nexus" {
		t.Fatalf("expected eHRMS department to be upserted, ok=%v unit=%+v", ok, unit)
	}
	employee, ok, err := store.GetEmployeeByEmployeeNo(context.Background(), "tenant-1", "IKM001")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected eHRMS employee to be created")
	}
	if employee.Name != "測試員工" || employee.CompanyEmail != "test.employee@ikala.ai" || employee.OrgUnitID != "M0101" || employee.Position != "工程師" || employee.PositionID != "0704" || employee.Status != "active" || employee.Category != "full_time" {
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
	ctx.ApprovalConfirmed = true

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
		"職務中文名稱": "專員",
		"身份類別名稱": "時薪員工",
		"身份證號":   "B123456789",
	}}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{rows: rows}})
	ctx.ApprovalConfirmed = true
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
	ctx.ApprovalConfirmed = true
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

// TestSyncEHRMSEmployeesFailsEntireBatchOnDuplicateEmail 驗證批次內重複 email 整批失敗。
func TestSyncEHRMSEmployeesFailsEntireBatchOnDuplicateEmail(t *testing.T) {
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
	}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{rows: rows}})
	ctx.ApprovalConfirmed = true

	result, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{})
	if err == nil {
		t.Fatal("expected duplicate email batch to fail")
	}
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Code != "import_validation_failed" {
		t.Fatalf("expected import_validation_failed, got %v", err)
	}
	if result.Failed != 2 {
		t.Fatalf("expected failed=2, got %+v", result)
	}
	foundDup := false
	for _, rowErr := range result.RowErrors {
		if rowErr.Field == "company_email" && rowErr.Code == "duplicate_in_file" {
			foundDup = true
			break
		}
	}
	if !foundDup {
		t.Fatalf("expected company_email duplicate_in_file, got %+v", result.RowErrors)
	}
	employees, err := store.ListEmployees(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(employees) != 0 {
		t.Fatalf("duplicate email batch should not write employees, got %+v", employees)
	}
}

// TestSyncEHRMSEmployeesHidesUpstreamFetchDetails 驗證 eHRMS 員工 hides upstream fetch details。
func TestSyncEHRMSEmployeesHidesUpstreamFetchDetails(t *testing.T) {
	_, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{err: errors.New("upstream 500: token=secret-value")}})
	ctx.ApprovalConfirmed = true

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
	ctx.ApprovalConfirmed = true

	result, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 || result.DepartmentsUpserted != 1 || result.PositionsUpserted != 1 {
		t.Fatalf("unexpected eHRMS sync result: %+v", result)
	}

	unit, ok, err := store.GetOrgUnit(context.Background(), "tenant-1", "M010102")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || unit.Code != "M010102" {
		t.Fatalf("expected org unit from dept_code, got ok=%v unit=%+v", ok, unit)
	}
	if unit.NameEN != "MarTech Div.(TW)-KOL Radar E2E-Sales 2(已關閉)" || unit.Source != "ehrms" || unit.UpdatedAt.IsZero() {
		t.Fatalf("expected eHRMS org metadata, got %+v", unit)
	}

	employee, ok, err := store.GetEmployeeByEmployeeNo(context.Background(), "tenant-1", "IK0028")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected employee to be created")
	}
	if employee.Name != "測試員工IK0028" || employee.OrgUnitID != "M010102" || employee.Position != "經理" || employee.Status != "resigned" {
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

// TestSyncEHRMSEmployeesBuildsOrgHierarchyAndPositions 驗證 eHRMS 同步會建立組織層級與崗位目錄。
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
	ctx.ApprovalConfirmed = true

	result, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.DepartmentsUpserted != 3 || result.PositionsUpserted != 3 {
		t.Fatalf("unexpected sync counts: %+v", result)
	}
	child, ok, err := store.GetOrgUnit(context.Background(), "tenant-1", "C0101")
	if err != nil || !ok || child.ParentID != "C01" {
		t.Fatalf("expected child org unit parent, got ok=%v unit=%+v err=%v", ok, child, err)
	}
	if child.NameEN != "Sales EN" || child.Source != "ehrms" || child.UpdatedAt.IsZero() || child.Closed {
		t.Fatalf("expected child org metadata, got %+v", child)
	}
	closed, ok, err := store.GetOrgUnit(context.Background(), "tenant-1", "C0199")
	if err != nil || !ok || !closed.Closed || closed.ParentID != "C01" {
		t.Fatalf("expected empty closed department upserted, ok=%v unit=%+v err=%v", ok, closed, err)
	}
	position, ok, err := store.GetPosition(context.Background(), "tenant-1", "0704")
	if err != nil || !ok || position.Name != "工程師" || position.NameEN != "Engineer" || position.Source != "ehrms" {
		t.Fatalf("expected synced position, got ok=%v position=%+v err=%v", ok, position, err)
	}
	intern, ok, err := store.GetPosition(context.Background(), "tenant-1", "0501")
	if err != nil || !ok || intern.Name != "實習生" {
		t.Fatalf("expected position from /positions without employees, ok=%v position=%+v err=%v", ok, intern, err)
	}
}

// TestSyncEHRMSAttendanceUpsertsDailySummaries 驗證 eHRMS 考勤同步 writes 日彙總 without GPS 打卡。
func TestSyncEHRMSAttendanceUpsertsDailySummaries(t *testing.T) {
	rows := []domain.EHRMSAttendanceRecord{{
		"emp_id":           "IKM017",
		"date":             "2026-06-10",
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
	}, service.Options{EHRMSClient: fakeEHRMSClient{attendanceRows: rows}})
	ctx.ApprovalConfirmed = true
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
	if result.Fetched != 2 || result.Created != 1 || result.Updated != 0 || result.Skipped != 1 || result.Failed != 0 || result.Mode != "upsert" {
		t.Fatalf("unexpected eHRMS attendance sync result: %+v", result)
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
	if got.ClockStart != "09:01" || got.ClockEnd != "18:02" || got.AttendStart != "09:00" || got.AttendEnd != "18:00" || got.AttendHours != 8 || !got.AttendCounted {
		t.Fatalf("unexpected clock/attend mapping: %+v", got)
	}
	if got.LeaveType != "特休" || got.LeaveStart != "13:00" || got.LeaveEnd != "15:00" || got.LeaveHours != 2 || !got.LeaveCounted {
		t.Fatalf("unexpected leave mapping: %+v", got)
	}
	if got.Leave2Type != "病假" || got.Leave2Start != "16:00" || got.Leave2End != "17:00" || got.Leave2Hours != 1 || !got.Leave2Counted {
		t.Fatalf("unexpected second leave mapping: %+v", got)
	}
	if got.OvertimeStart != "18:30" || got.OvertimeEnd != "20:00" || got.OvertimeHours != 1.5 || !got.OvertimeCounted {
		t.Fatalf("unexpected overtime mapping: %+v", got)
	}
	if got.Payload["name_zh"] != "測試員工" || got.Payload["clock_start"] != "2026-06-10 09:01" || got.Payload["leave_type"] != "特休" {
		t.Fatalf("expected normalized payload to preserve source fields, got %+v", got.Payload)
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
	if result.Created != 0 || result.Updated != 1 || result.Skipped != 1 {
		t.Fatalf("expected idempotent upsert on second sync, got %+v", result)
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
	_, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "attendance.clock", Action: "import", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{attendanceRows: rows}})
	ctx.ApprovalConfirmed = true

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
	_, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "attendance.clock", Action: "import", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{attendanceErr: errors.New("upstream 500: token=secret-value")}})
	ctx.ApprovalConfirmed = true

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

// TestEmployeeImportConfirmSupportsUpdateAndUpsert 驗證員工 import confirm supports update and upsert。
func TestEmployeeImportConfirmSupportsUpdateAndUpsert(t *testing.T) {
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
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-hr"}, CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:               "emp-existing",
		TenantID:         "tenant-1",
		EmployeeNo:       "E3001",
		Name:             "Original",
		CompanyEmail:     "original@example.com",
		Phone:            "0900000000",
		OrgUnitID:        "ou-1",
		Position:         "HRBP",
		Category:         "full_time",
		Status:           "active",
		EmploymentStatus: "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	svc := service.New(store)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}

	updateSession, err := svc.HR().PreviewEmployeeImport(ctx, domain.EmployeeImportPreviewInput{
		Filename: "employees.csv",
		Content:  "員工編號,姓名,Email,部門,職位,類別,電話,狀態,到職日期,主管員工ID\nE3001,Updated,original@example.com,ou-1,People Lead,全職,0911000999,在職,2026-06-01,\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	updatedSession, err := svc.HR().ConfirmEmployeeImport(ctx, updateSession.ID, domain.EmployeeImportConfirmInput{Mode: "update"})
	if err != nil {
		t.Fatal(err)
	}
	if updatedSession.Summary["updated"] != 1 || updatedSession.Summary["created"] != 0 {
		t.Fatalf("expected update import summary, got %+v", updatedSession.Summary)
	}
	updated, ok, err := store.GetEmployee(context.Background(), "tenant-1", "emp-existing")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || updated.Phone != "0911000999" || updated.Position != "People Lead" {
		t.Fatalf("expected existing employee to be updated, got ok=%v employee=%+v", ok, updated)
	}

	upsertSession, err := svc.HR().PreviewEmployeeImport(ctx, domain.EmployeeImportPreviewInput{
		Filename: "employees.csv",
		Content:  "員工編號,姓名,Email,部門,職位,類別,電話,狀態,到職日期,主管員工ID\nE3001,Updated Again,original@example.com,ou-1,HR Manager,全職,0911000888,在職,2026-06-01,\nE3002,New Hire,new.hire@example.com,ou-1,Recruiter,全職,0911000777,在職,2026-06-01,\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	upsertedSession, err := svc.HR().ConfirmEmployeeImport(ctx, upsertSession.ID, domain.EmployeeImportConfirmInput{Mode: "upsert"})
	if err != nil {
		t.Fatal(err)
	}
	if upsertedSession.Summary["updated"] != 1 || upsertedSession.Summary["created"] != 1 {
		t.Fatalf("expected upsert import summary, got %+v", upsertedSession.Summary)
	}
	employees, err := store.ListEmployees(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(employees) != 2 {
		t.Fatalf("expected one updated and one created employee, got %+v", employees)
	}
}

// TestEmployeeImportConfirmEnforcesAssignedOrgScope 驗證員工 import confirm enforces assigned 組織範圍。
func TestEmployeeImportConfirmEnforcesAssignedOrgScope(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-allowed", TenantID: "tenant-1", Name: "Allowed", Path: []string{"ou-allowed"}, CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-blocked", TenantID: "tenant-1", Name: "Blocked", Path: []string{"ou-blocked"}, CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-import",
		TenantID: "tenant-1",
		Name:     "Scoped Import",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "import", Scope: "all"},
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
		PermissionSetID: "ps-import",
		Effect:          "allow",
		DataScopeID:     "ds-assigned",
		CreatedAt:       now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:               "emp-blocked",
		TenantID:         "tenant-1",
		EmployeeNo:       "E9001",
		Name:             "Blocked",
		CompanyEmail:     "blocked@example.com",
		OrgUnitID:        "ou-blocked",
		Position:         "Engineer",
		Category:         "full_time",
		Status:           "active",
		EmploymentStatus: "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	svc := service.New(store)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}

	session, err := svc.HR().PreviewEmployeeImport(ctx, domain.EmployeeImportPreviewInput{
		Filename: "employees.csv",
		Content:  "員工編號,姓名,Email,部門,職位,類別,電話,狀態,到職日期,主管員工ID\nE9001,Move Into Scope,blocked@example.com,ou-allowed,Engineer,全職,0911000000,在職,2026-06-01,\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.HR().ConfirmEmployeeImport(ctx, session.ID, domain.EmployeeImportConfirmInput{Mode: "update"})
	if err == nil {
		t.Fatal("expected scoped import confirmation to reject out-of-scope employee update")
	}
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Code != "import_validation_failed" || !rowErrorsContain(appErr.RowErrors, "authz_scope") {
		t.Fatalf("expected authz_scope import validation error, got %v", err)
	}
	stored, ok, err := store.GetEmployee(context.Background(), "tenant-1", "emp-blocked")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || stored.OrgUnitID != "ou-blocked" || stored.Name != "Blocked" {
		t.Fatalf("out-of-scope import should not mutate employee, got ok=%v employee=%+v", ok, stored)
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
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", RequestID: "req-reinstate", ApprovalConfirmed: true}
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
	ctx.ApprovalConfirmed = true
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
	ctx.ApprovalConfirmed = true
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

// TestAuditServiceRequiresAuditReadPermission 驗證稽核服務 requires 稽核 read 權限。
func TestAuditServiceRequiresAuditReadPermission(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	svc := service.New(store)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}

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

// TestEmployeeImportXLSXPreservesManagerEmployeeID 驗證員工 import XLSX preserves 主管員工 ID。
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
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:               "emp-manager",
		TenantID:         "tenant-1",
		EmployeeNo:       "E1000",
		Name:             "Manager Chen",
		CompanyEmail:     "manager@example.com",
		Status:           "active",
		EmploymentStatus: "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	})

	svc := service.New(store)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}
	content := minimalEmployeeImportXLSX(t, [][]string{
		{"員工編號", "姓名", "Email", "部門", "職位", "類別", "電話", "狀態", "到職日期", "主管員工ID"},
		{"E2002", "Mina Chen", "mina@example.com", "ou-1", "HRBP", "全職", "0911000444", "在職", "2026-06-01", "emp-manager"},
	})

	session, err := svc.HR().PreviewEmployeeImport(ctx, domain.EmployeeImportPreviewInput{
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
	if row.Employee.ManagerEmployeeID != "emp-manager" {
		t.Fatalf("expected manager employee id from xlsx column J, got %+v", row.Employee)
	}
	if got := row.Employee.EmploymentInfo["manager_employee_id"]; got != "emp-manager" {
		t.Fatalf("expected employment info manager_employee_id from xlsx column J, got %v", got)
	}
	if got := row.Input["主管員工ID"]; got != "emp-manager" {
		t.Fatalf("expected input manager column from xlsx column J, got %q", got)
	}
	if !row.Valid {
		t.Fatalf("expected import_minimal profile to accept 10-column xlsx row, got errors %+v", row.Errors)
	}

	missingManagerContent := minimalEmployeeImportXLSX(t, [][]string{
		{"員工編號", "姓名", "Email", "部門", "職位", "類別", "電話", "狀態", "到職日期", "主管員工ID"},
		{"E2003", "Missing Manager", "missing.manager@example.com", "ou-1", "HRBP", "全職", "0911000445", "在職", "2026-06-01", "emp-missing"},
	})
	missingSession, err := svc.HR().PreviewEmployeeImport(ctx, domain.EmployeeImportPreviewInput{
		Filename: "employees.xlsx",
		Content:  missingManagerContent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(missingSession.Rows) != 1 || missingSession.Rows[0].Valid || !rowErrorsContain(missingSession.Rows[0].Errors, "manager_employee_id") {
		t.Fatalf("expected missing manager employee id to invalidate import row, got %+v", missingSession.Rows)
	}
}

// TestEmployeeImportPreviewRejectsOversizedXLSXEntry 驗證員工 import preview rejects oversized XLSX entry。
func TestEmployeeImportPreviewRejectsOversizedXLSXEntry(t *testing.T) {
	_, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
	})
	ctx.ApprovalConfirmed = true
	content := oversizedEmployeeImportXLSX(t)
	if len(content) > 10<<20 {
		t.Fatalf("test workbook should stay below upload byte limit, got %d bytes", len(content))
	}

	_, err := svc.HR().PreviewEmployeeImport(ctx, domain.EmployeeImportPreviewInput{
		Filename: "employees.xlsx",
		Content:  content,
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected oversized xlsx entry error, got %v", err)
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

// TestEmployeeImportTemplateHeaders 驗證員工 import 範本 headers。
func TestEmployeeImportTemplateHeaders(t *testing.T) {
	_, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "read", Scope: "all"},
	})
	expected := []string{"員工編號", "姓名", "Email", "部門", "職位", "類別", "電話", "狀態", "到職日期", "主管員工ID", "帳號策略"}

	rawCSV, filename, contentType, err := svc.HR().EmployeeImportTemplate(ctx, "csv")
	if err != nil {
		t.Fatal(err)
	}
	if filename != "employee-import-template.csv" || !strings.HasPrefix(contentType, "text/csv") {
		t.Fatalf("unexpected csv template metadata filename=%q content_type=%q", filename, contentType)
	}
	if !bytes.HasPrefix(rawCSV, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("expected csv template to include UTF-8 BOM")
	}
	csvHeaders := strings.Split(strings.TrimSpace(strings.TrimPrefix(string(rawCSV), "\ufeff")), ",")
	if !equalStrings(csvHeaders, expected) || csvHeaders[10] != "帳號策略" {
		t.Fatalf("unexpected csv headers: %+v", csvHeaders)
	}

	rawXLSX, filename, contentType, err := svc.HR().EmployeeImportTemplate(ctx, "xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if filename != "employee-import-template.xlsx" || contentType != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("unexpected xlsx template metadata filename=%q content_type=%q", filename, contentType)
	}
	xlsxHeaders := xlsxSharedStrings(t, rawXLSX)
	if !equalStrings(xlsxHeaders, expected) || xlsxHeaders[10] != "帳號策略" {
		t.Fatalf("unexpected xlsx headers: %+v", xlsxHeaders)
	}
}

// TestEmployeeExportApprovalInstanceMatchesFilters 驗證員工 export 核准實例 matches 篩選。
func TestEmployeeExportApprovalInstanceMatchesFilters(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "export", Scope: "all"},
	})
	now := time.Now().UTC()
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:               "emp-export",
		TenantID:         "tenant-1",
		EmployeeNo:       "E5001",
		Name:             "Export Person",
		CompanyEmail:     "export.person@example.com",
		Status:           "active",
		EmploymentStatus: "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	_ = store.UpsertFormInstance(context.Background(), domain.FormInstance{
		ID:                 "fi-export-good",
		TenantID:           "tenant-1",
		ApplicantAccountID: ctx.AccountID,
		Status:             "approved",
		Payload: map[string]any{
			"application_code": "hr",
			"resource_type":    "employee",
			"action":           "export",
			"filters":          map[string]any{"employment_status": "active"},
		},
		SubmittedAt: now,
		UpdatedAt:   now,
	})
	ctx.ApprovalInstanceID = "fi-export-good"
	items, err := svc.HR().ExportEmployees(ctx, domain.EmployeeQuery{EmploymentStatus: "active"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "emp-export" {
		t.Fatalf("expected approved export result, got %+v", items)
	}

	_ = store.UpsertFormInstance(context.Background(), domain.FormInstance{
		ID:                 "fi-export-generic",
		TenantID:           "tenant-1",
		ApplicantAccountID: ctx.AccountID,
		Status:             "approved",
		Payload: map[string]any{
			"application_code": "hr",
			"resource_type":    "employee",
			"action":           "export",
		},
		SubmittedAt: now,
		UpdatedAt:   now,
	})
	ctx.ApprovalInstanceID = "fi-export-generic"
	_, err = svc.HR().ExportEmployees(ctx, domain.EmployeeQuery{EmploymentStatus: "active"})
	if err == nil || !strings.Contains(err.Error(), "approval filters do not match request") {
		t.Fatalf("expected unfiltered approval to fail filtered export, got %v", err)
	}

	_ = store.UpsertFormInstance(context.Background(), domain.FormInstance{
		ID:                 "fi-export-bad",
		TenantID:           "tenant-1",
		ApplicantAccountID: ctx.AccountID,
		Status:             "approved",
		Payload: map[string]any{
			"application_code": "hr",
			"resource_type":    "employee",
			"action":           "export",
			"filters":          map[string]any{"employment_status": "resigned"},
		},
		SubmittedAt: now,
		UpdatedAt:   now,
	})
	ctx.ApprovalInstanceID = "fi-export-bad"
	_, err = svc.HR().ExportEmployees(ctx, domain.EmployeeQuery{EmploymentStatus: "active"})
	if err == nil || !strings.Contains(err.Error(), "approval filters do not match request") {
		t.Fatalf("expected filter-mismatched approval to fail, got %v", err)
	}
}

// TestApprovalInstanceMustBelongToCurrentAccount 驗證核准實例 must belong to 目前帳號。
func TestApprovalInstanceMustBelongToCurrentAccount(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "export", Scope: "all"},
	})
	now := time.Now().UTC()
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:               "emp-export-other-approval",
		TenantID:         "tenant-1",
		EmployeeNo:       "E5009",
		Name:             "Export Other Approval",
		CompanyEmail:     "export.other.approval@example.com",
		Status:           "active",
		EmploymentStatus: "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	_ = store.UpsertFormInstance(context.Background(), domain.FormInstance{
		ID:                 "fi-export-other-account",
		TenantID:           "tenant-1",
		ApplicantAccountID: "acct-other",
		Status:             "approved",
		Payload: map[string]any{
			"application_code": "hr",
			"resource_type":    "employee",
			"action":           "export",
			"filters":          map[string]any{"employment_status": "active"},
		},
		SubmittedAt: now,
		UpdatedAt:   now,
	})

	ctx.ApprovalInstanceID = "fi-export-other-account"
	_, err := svc.HR().ExportEmployees(ctx, domain.EmployeeQuery{EmploymentStatus: "active"})
	if err == nil || !strings.Contains(err.Error(), "approval instance does not belong to current account") {
		t.Fatalf("expected approval owner mismatch to fail, got %v", err)
	}
}

// TestEmployeeStatusTransitionApprovalInstanceRequiresTarget 驗證員工狀態轉換核准實例 requires target。
func TestEmployeeStatusTransitionApprovalInstanceRequiresTarget(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "status_transition", Scope: "all"},
	})
	now := time.Now().UTC()
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:               "emp-status-target",
		TenantID:         "tenant-1",
		EmployeeNo:       "E5002",
		Name:             "Status Target",
		CompanyEmail:     "status.target@example.com",
		Status:           "active",
		EmploymentStatus: "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	_ = store.UpsertFormInstance(context.Background(), domain.FormInstance{
		ID:                 "fi-status-generic",
		TenantID:           "tenant-1",
		ApplicantAccountID: ctx.AccountID,
		Status:             "approved",
		Payload: map[string]any{
			"application_code": "hr",
			"resource_type":    "employee",
			"action":           "status_transition",
		},
		SubmittedAt: now,
		UpdatedAt:   now,
	})

	ctx.ApprovalInstanceID = "fi-status-generic"
	_, err := svc.HR().TransitionEmployeeStatus(ctx, "emp-status-target", domain.StatusTransitionInput{
		Status:  "resigned",
		Reason:  "left",
		EndDate: "2026-06-30",
	})
	if err == nil || !strings.Contains(err.Error(), "approval target does not match request") {
		t.Fatalf("expected unbound approval target to fail, got %v", err)
	}

	_ = store.UpsertFormInstance(context.Background(), domain.FormInstance{
		ID:                 "fi-status-target",
		TenantID:           "tenant-1",
		ApplicantAccountID: ctx.AccountID,
		Status:             "approved",
		Payload: map[string]any{
			"application_code": "hr",
			"resource_type":    "employee",
			"action":           "status_transition",
			"resource_id":      "emp-status-target",
		},
		SubmittedAt: now,
		UpdatedAt:   now,
	})
	ctx.ApprovalInstanceID = "fi-status-target"
	transitioned, err := svc.HR().TransitionEmployeeStatus(ctx, "emp-status-target", domain.StatusTransitionInput{
		Status:  "resigned",
		Reason:  "left",
		EndDate: "2026-06-30",
	})
	if err != nil {
		t.Fatal(err)
	}
	if transitioned.EmploymentStatus != "resigned" {
		t.Fatalf("expected approved target transition, got %+v", transitioned)
	}
}

// TestInviteDeletedOrResignedEmployeeFails 驗證 deleted or resigned 員工 fails。
func TestInviteDeletedOrResignedEmployeeFails(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "invite", Scope: "all"},
	})
	ctx.ApprovalConfirmed = true
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
	ctx.ApprovalConfirmed = true
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
	ctx.ApprovalConfirmed = true
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
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin", ApprovalConfirmed: true}
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
	if strings.Join(got, ",") != "ou-root,ou-filled,ou-empty" {
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

// TestUpdateOrgUnitManagerPosition 驗證組織單位可綁定主管崗。
func TestUpdateOrgUnitManagerPosition(t *testing.T) {
	svc, ctx := newServiceFixture([]domain.Permission{
		{Resource: "hr.org_unit", Action: "create", Scope: "all"},
		{Resource: "hr.org_unit", Action: "update", Scope: "all"},
		{Resource: "hr.org_unit", Action: "read", Scope: "all"},
		{Resource: "hr.position", Action: "create", Scope: "all"},
		{Resource: "hr.position", Action: "read", Scope: "all"},
	})
	root, err := svc.HR().CreateOrgUnit(ctx, domain.CreateOrgUnitInput{Name: "Root"})
	if err != nil {
		t.Fatal(err)
	}
	position, err := svc.HR().CreatePosition(ctx, domain.CreatePositionInput{
		Code: "ROOT-HEAD", Name: "Root Head", OrgUnitID: root.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	managerPositionID := position.ID
	updated, err := svc.HR().UpdateOrgUnit(ctx, root.ID, domain.UpdateOrgUnitInput{
		ManagerPositionID: &managerPositionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ManagerPositionID != position.ID {
		t.Fatalf("expected manager position %s, got %s", position.ID, updated.ManagerPositionID)
	}
}

// TestPlatformTaskMutationsPersistAndProject 驗證平台任務 mutations persist and project。
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

	tasks, err := svc.Platform().Tasks(ctx)
	if err != nil {
		t.Fatal(err)
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

	tasks, err = svc.Platform().Tasks(ctx)
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
	tasks, err = svc.Platform().Tasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	agentRecord := findPlatformTaskRecord(t, tasks.Records, "2026/07/01")
	var agentItem domain.PlatformTaskItem
	for _, item := range agentRecord.Items {
		if item.ID == "run-platform-readonly" {
			agentItem = item
		}
	}
	if !agentItem.ReadOnly || agentItem.Source != "agent_run" {
		t.Fatalf("expected agent run task item to be read-only, got %+v", agentItem)
	}
	agentTodo := findPlatformTaskTodo(t, tasks.Todos, "todo-run-platform-readonly")
	if !agentTodo.ReadOnly || agentTodo.Source != "agent_run" {
		t.Fatalf("expected agent run todo to be read-only, got %+v", agentTodo)
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

// TestPlatformWorkspaceOrganizationManagerUpdatePersistsHierarchy 驗證平台工作區 organization 主管 update persists hierarchy。
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

	if _, err := svc.Workspace().UpdateWorkspaceOrganizationManager(ctx, "E1001", domain.UpdateWorkspaceOrganizationManagerInput{ParentID: stringPtr("E1003")}); err == nil {
		t.Fatal("expected manager cycle to be rejected")
	}
}

// TestPlatformWorkspaceFormDesignMutationsPersistTemplateSchema 驗證平台工作區表單 design mutations persist 範本 schema。
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
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-forms", ApprovalConfirmed: true}

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

// findPlatformFormDesignForm 驗證 find 平台表單 design 表單。
func findPlatformFormDesignForm(t *testing.T, forms []domain.PlatformFormDesignForm, id string) domain.PlatformFormDesignForm {
	t.Helper()
	form, ok := platformFormDesignFormByID(forms, id)
	if !ok {
		t.Fatalf("missing form design %s in %+v", id, forms)
	}
	return form
}

// platformFormDesignFormByID 驗證平台表單 design 表單 by ID。
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

// findPlatformTaskRecord 驗證 find 平台任務 record。
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

// findPlatformTaskTodo 驗證 find 平台任務待辦。
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
		{ID: "emp-1", TenantID: "tenant-1", Name: "Employee One", Status: "active", CreatedAt: now},
		{ID: "emp-2", TenantID: "tenant-1", Name: "Employee Two", Status: "active", CreatedAt: now},
		{ID: "emp-admin", TenantID: "tenant-1", Name: "Attendance Admin", Status: "active", CreatedAt: now},
	} {
		_ = store.UpsertEmployee(context.Background(), employee)
	}
	_ = store.UpsertAttendanceWorksite(context.Background(), domain.AttendanceWorksite{
		ID:           "aws-1",
		TenantID:     "tenant-1",
		Name:         "HQ",
		Latitude:     25.033964,
		Longitude:    121.564468,
		RadiusMeters: 200,
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	_ = store.UpsertAttendanceShift(context.Background(), domain.AttendanceShift{
		ID:            "ash-1",
		TenantID:      "tenant-1",
		Name:          "Day Shift",
		ClockInStart:  "08:00",
		ClockInEnd:    "10:00",
		ClockOutStart: "17:00",
		ClockOutEnd:   "19:00",
		Status:        "active",
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	for _, employeeID := range []string{"emp-1", "emp-2"} {
		_ = store.UpsertAttendanceShiftAssignment(context.Background(), domain.AttendanceShiftAssignment{
			ID:            "asa-" + employeeID,
			TenantID:      "tenant-1",
			EmployeeID:    employeeID,
			ShiftID:       "ash-1",
			WorksiteID:    "aws-1",
			EffectiveFrom: now.Add(-24 * time.Hour),
			Status:        "active",
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	}
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return currentNow }})
	setNow := func(next time.Time) { currentNow = next.UTC().Truncate(time.Second) }
	return store, svc,
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-employee"},
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin", ApprovalConfirmed: true},
		setNow
}

// attendanceFixtureClockInTime 驗證考勤 fixture 打卡 in 時間。
func attendanceFixtureClockInTime() time.Time {
	return time.Date(2026, 6, 10, 1, 0, 0, 0, time.UTC)
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
	rows           []domain.EHRMSEmployeeRecord
	departmentRows []domain.EHRMSDepartmentRecord
	positionRows   []domain.EHRMSPositionRecord
	attendanceRows []domain.EHRMSAttendanceRecord
	err            error
	departmentsErr error
	positionsErr   error
	attendanceErr  error
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
func (c fakeEHRMSClient) ListAttendance(context.Context) ([]domain.EHRMSAttendanceRecord, error) {
	return ehrms.NormalizeAttendanceRecords(c.attendanceRows), c.attendanceErr
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

type failingEmployeeImportSessionStore struct {
	*memory.Store
}

// WithTenantTransaction 驗證租戶 transaction。
func (s *failingEmployeeImportSessionStore) WithTenantTransaction(_ context.Context, _ string, fn func(repository.Store) error) error {
	return fn(s)
}

// UpsertEmployeeImportSession 驗證 upsert 員工 import session。
func (s *failingEmployeeImportSessionStore) UpsertEmployeeImportSession(_ context.Context, _ domain.EmployeeImportSession) error {
	return fmt.Errorf("session persistence failed")
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
	values map[string]domain.CheckResult
	gets   int
	sets   int
}

// GetAuthzSnapshot 驗證授權快照。
func (s *recordingAuthzSnapshot) GetAuthzSnapshot(_ context.Context, key string) (domain.CheckResult, bool, error) {
	s.gets++
	result, ok := s.values[key]
	return result, ok, nil
}

// SetAuthzSnapshot 驗證集合授權快照。
func (s *recordingAuthzSnapshot) SetAuthzSnapshot(_ context.Context, key string, result domain.CheckResult, _ time.Duration) error {
	s.sets++
	s.values[key] = result
	return nil
}

// InvalidateTenant 驗證 invalidate 租戶。
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

// findAuditLog 驗證 find 稽核 log。
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

// rowErrorsContain 驗證列錯誤 contain。
func rowErrorsContain(fields []domain.RowError, expectedField string) bool {
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

// equalStrings 驗證 equal 字串。
func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// xlsxSharedStrings 驗證 XLSX shared 字串。
func xlsxSharedStrings(t *testing.T, raw []byte) []string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range reader.File {
		if file.Name != "xl/sharedStrings.xml" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatal(err)
		}
		var parsed struct {
			Items []struct {
				Text string `xml:"t"`
			} `xml:"si"`
		}
		if err := xml.Unmarshal(buf.Bytes(), &parsed); err != nil {
			t.Fatal(err)
		}
		values := make([]string, 0, len(parsed.Items))
		for _, item := range parsed.Items {
			values = append(values, item.Text)
		}
		return values
	}
	t.Fatal("xl/sharedStrings.xml not found")
	return nil
}

// minimalEmployeeImportXLSX 驗證 minimal 員工 import XLSX。
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

// oversizedEmployeeImportXLSX 驗證 oversized 員工 import XLSX。
func oversizedEmployeeImportXLSX(t *testing.T) string {
	t.Helper()
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
	add("xl/sharedStrings.xml", []byte(`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>員工編號</t></si></sst>`))
	oversizedSheet := `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>` + strings.Repeat(" ", (10<<20)+1) + `</sheetData></worksheet>`
	add("xl/worksheets/sheet1.xml", []byte(oversizedSheet))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
