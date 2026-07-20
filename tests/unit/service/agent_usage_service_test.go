package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
	agentservice "nexus-pro-api/internal/service/agent"
)

// TestAgentUsageAggregatesRetainedMessagesByTenantAccount verifies the management usage contract.
func TestAgentUsageAggregatesRetainedMessagesByTenantAccount(t *testing.T) {
	now := time.Date(2026, 7, 15, 2, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentUsageTenant(t, store, now, "tenant-1", "acct-admin", true)
	seedAgentUsageTenant(t, store, now, "tenant-2", "acct-other", false)
	for _, account := range []domain.Account{
		{ID: "acct-user", TenantID: "tenant-1", DisplayName: "Alice", Email: "alice@example.com", Status: "active", CreatedAt: now},
		{ID: "acct-zero", TenantID: "tenant-1", DisplayName: "Zero", Email: "zero@example.com", Status: "disabled", CreatedAt: now},
	} {
		if err := store.UpsertAccount(context.Background(), account); err != nil {
			t.Fatal(err)
		}
	}

	lastMessageAt := now.Add(5 * time.Minute)
	for _, session := range []domain.AgentSession{
		{ID: "session-active", TenantID: "tenant-1", AccountID: "acct-user", Status: domain.AgentSessionStatusActive, ContextVersion: 2, LastMessageAt: &lastMessageAt, CreatedAt: now, UpdatedAt: lastMessageAt},
		{ID: "session-archived", TenantID: "tenant-1", AccountID: "acct-user", Status: domain.AgentSessionStatusArchived, ContextVersion: 1, CreatedAt: now, UpdatedAt: now.Add(time.Minute)},
		{ID: "session-other", TenantID: "tenant-2", AccountID: "acct-other", Status: domain.AgentSessionStatusActive, ContextVersion: 1, CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.UpsertAgentSession(context.Background(), session); err != nil {
			t.Fatal(err)
		}
	}
	for index, message := range []domain.AgentSessionMessage{
		{ID: "msg-old-user", TenantID: "tenant-1", SessionID: "session-active", Role: domain.AgentMessageRoleUser, ContextVersion: 1, CreatedAt: now.Add(time.Minute)},
		{ID: "msg-current-assistant", TenantID: "tenant-1", SessionID: "session-active", Role: domain.AgentMessageRoleAssistant, ContextVersion: 2, CreatedAt: lastMessageAt},
		{ID: "msg-tool", TenantID: "tenant-1", SessionID: "session-archived", Role: domain.AgentMessageRoleTool, ContextVersion: 1, CreatedAt: now.Add(2 * time.Minute)},
		{ID: "msg-other", TenantID: "tenant-2", SessionID: "session-other", Role: domain.AgentMessageRoleUser, ContextVersion: 1, CreatedAt: now},
	} {
		message.Content = "message"
		if err := store.InsertAgentSessionMessage(context.Background(), message); err != nil {
			t.Fatalf("insert message %d: %v", index, err)
		}
	}
	for _, run := range []domain.AgentRun{
		{
			ID: "run-active", TenantID: "tenant-1", AccountID: "acct-user", SessionID: "session-active",
			Status: string(domain.AgentRunStatusCompleted), LLMCallCount: 2, InputTokens: 100, CachedTokens: 40,
			OutputTokens: 20, TotalTokens: 120, UsageComplete: true, CreatedAt: now, UpdatedAt: lastMessageAt,
		},
		{
			ID: "run-archived", TenantID: "tenant-1", AccountID: "acct-user", SessionID: "session-archived",
			Status: string(domain.AgentRunStatusCompleted), LLMCallCount: 1, InputTokens: 50,
			OutputTokens: 10, TotalTokens: 60, UsageComplete: true, CreatedAt: now, UpdatedAt: now.Add(2 * time.Minute),
		},
		{
			ID: "run-other", TenantID: "tenant-2", AccountID: "acct-other", SessionID: "session-other",
			Status: string(domain.AgentRunStatusCompleted), LLMCallCount: 1, TotalTokens: 999,
			UsageComplete: true, CreatedAt: now, UpdatedAt: now,
		},
	} {
		if err := store.UpsertAgentRun(context.Background(), run); err != nil {
			t.Fatal(err)
		}
	}

	usage, err := agentservice.New(service.New(store)).ListAccountUsage(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"},
		domain.AgentAccountUsageQuery{},
		domain.PageRequest{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Total != 3 || usage.Summary.UserCount != 3 || usage.Summary.UsersWithUsage != 1 {
		t.Fatalf("unexpected account summary: %+v", usage)
	}
	if usage.Page != 1 || usage.PageSize != domain.DefaultPageSize {
		t.Fatalf("unexpected overview page metadata: %+v", usage)
	}
	if usage.Summary.SessionCount != 2 || usage.Summary.MessageCount != 3 {
		t.Fatalf("unexpected usage totals: %+v", usage.Summary)
	}
	if usage.Summary.LLMCallCount != 3 || usage.Summary.TotalTokens != 180 || usage.Summary.CachedTokens != 40 || usage.Summary.ActualTokens != 140 {
		t.Fatalf("unexpected token totals: %+v", usage.Summary)
	}
	if len(usage.Items) != 3 || usage.Items[0].AccountID != "acct-user" {
		t.Fatalf("expected used account first, got %+v", usage.Items)
	}
	used := usage.Items[0]
	if used.SessionCount != 2 || used.MessageCount != 3 || used.LastActiveAt == nil || !used.LastActiveAt.Equal(lastMessageAt) {
		t.Fatalf("unexpected retained usage row: %+v", used)
	}
	if used.InputTokens != 150 || used.OutputTokens != 30 || used.TotalTokens != 180 || used.ActualTokens != 140 {
		t.Fatalf("unexpected account token usage: %+v", used)
	}
	detail, err := agentservice.New(service.New(store)).ListAccountSessionUsage(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"},
		"acct-user",
		domain.PageRequest{Page: 1, PageSize: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Account.AccountID != "acct-user" || detail.Total != 2 || detail.Page != 1 || detail.PageSize != 1 {
		t.Fatalf("unexpected session page: %+v", detail)
	}
	if len(detail.Items) != 1 || detail.Items[0].SessionID != "session-active" {
		t.Fatalf("expected newest account session only, got %+v", detail.Items)
	}
	activeSession := detail.Items[0]
	if activeSession.MessageCount != 2 || activeSession.TotalTokens != 120 || activeSession.CachedTokens != 40 || activeSession.ActualTokens != 80 {
		t.Fatalf("unexpected active session usage: %+v", activeSession)
	}
	secondPage, err := agentservice.New(service.New(store)).ListAccountSessionUsage(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"},
		"acct-user",
		domain.PageRequest{Page: 2, PageSize: 1},
	)
	if err != nil || len(secondPage.Items) != 1 || secondPage.Items[0].SessionID != "session-archived" {
		t.Fatalf("unexpected second session page: %+v err=%v", secondPage, err)
	}
	for _, item := range usage.Items {
		if item.AccountID == "acct-other" {
			t.Fatalf("cross-tenant account leaked into usage: %+v", usage.Items)
		}
	}

	filtered, err := agentservice.New(service.New(store)).ListAccountUsage(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"},
		domain.AgentAccountUsageQuery{Query: "ALICE@EXAMPLE", Status: "active"},
		domain.PageRequest{Page: 1, PageSize: 1, Sort: "total_tokens_desc"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Total != 1 || len(filtered.Items) != 1 || filtered.Items[0].AccountID != "acct-user" {
		t.Fatalf("expected server-filtered Alice page, got %+v", filtered)
	}
	if filtered.Summary.UserCount != 3 || filtered.Summary.TotalTokens != 180 {
		t.Fatalf("expected tenant totals to remain independent of list filters, got %+v", filtered.Summary)
	}

	ascending, err := agentservice.New(service.New(store)).ListAccountUsage(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"},
		domain.AgentAccountUsageQuery{},
		domain.PageRequest{Page: 2, PageSize: 2, Sort: "total_tokens_asc"},
	)
	if err != nil || ascending.Total != 3 || len(ascending.Items) != 1 || ascending.Items[0].AccountID != "acct-user" {
		t.Fatalf("expected server-ordered second page, got %+v err=%v", ascending, err)
	}
}

// TestAgentUsageRejectsUnsupportedFilters keeps dynamic SQL inputs on a fixed allowlist.
func TestAgentUsageRejectsUnsupportedFilters(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 7, 15, 2, 0, 0, 0, time.UTC)
	seedAgentUsageTenant(t, store, now, "tenant-1", "acct-admin", true)
	svc := agentservice.New(service.New(store))
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}

	for name, input := range map[string]struct {
		query domain.AgentAccountUsageQuery
		page  domain.PageRequest
	}{
		"status": {query: domain.AgentAccountUsageQuery{Status: "deleted"}},
		"sort":   {page: domain.PageRequest{Sort: "total_tokens;drop table accounts"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := svc.ListAccountUsage(ctx, input.query, input.page)
			appErr, ok := domain.AsAppError(err)
			if !ok || appErr.Status != 400 {
				t.Fatalf("expected validation error, got %v", err)
			}
		})
	}
}

// TestAgentSessionUsageRejectsUnknownAccounts keeps tenant account lookup explicit.
func TestAgentSessionUsageRejectsUnknownAccounts(t *testing.T) {
	now := time.Date(2026, 7, 15, 2, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentUsageTenant(t, store, now, "tenant-1", "acct-admin", true)

	_, err := agentservice.New(service.New(store)).ListAccountSessionUsage(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"},
		"missing-account",
		domain.PageRequest{},
	)
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Status != 404 {
		t.Fatalf("expected missing account to return 404, got %v", err)
	}
}

// TestAgentUsageRequiresDedicatedTenantWideRead rejects definition-only and account-scoped usage grants.
func TestAgentUsageRequiresDedicatedTenantWideRead(t *testing.T) {
	now := time.Date(2026, 7, 15, 2, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name                string
		permission          domain.Permission
		expectDataScopeDeny bool
	}{
		{name: "tenant_wide_definition_read", permission: domain.Permission{Resource: "agent.definition", Action: domain.ActionRead, Scope: domain.ScopeAll}},
		{name: "unscoped_usage_read", permission: domain.Permission{Resource: "agent.usage", Action: domain.ActionRead}, expectDataScopeDeny: true},
		{name: "self_scoped_usage_read", permission: domain.Permission{Resource: "agent.usage", Action: domain.ActionRead, Scope: domain.ScopeSelf}, expectDataScopeDeny: true},
		{name: "own_scoped_usage_read", permission: domain.Permission{Resource: "agent.usage", Action: domain.ActionRead, Scope: domain.ScopeOwn}, expectDataScopeDeny: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := memory.NewStore()
			seedAgentUsageTenant(t, store, now, "tenant-1", "acct-runtime", false)
			if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
				ID:          "ps-runtime",
				TenantID:    "tenant-1",
				Name:        "Usage Boundary Test",
				Permissions: []domain.Permission{test.permission},
				CreatedAt:   now,
			}); err != nil {
				t.Fatal(err)
			}
			account, ok, err := store.GetAccount(context.Background(), "tenant-1", "acct-runtime")
			if err != nil || !ok {
				t.Fatalf("get account: ok=%v err=%v", ok, err)
			}
			if test.permission.Scope == domain.ScopeSelf || test.permission.Scope == domain.ScopeOwn {
				account.EmployeeID = "emp-runtime"
			}
			account.DirectPermissionSetIDs = []string{"ps-runtime"}
			if err := store.UpsertAccount(context.Background(), account); err != nil {
				t.Fatal(err)
			}

			svc := agentservice.New(service.New(store))
			ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-runtime"}
			for endpoint, call := range map[string]func() error{
				"overview": func() error {
					_, err := svc.ListAccountUsage(ctx, domain.AgentAccountUsageQuery{}, domain.PageRequest{})
					return err
				},
				"sessions": func() error {
					_, err := svc.ListAccountSessionUsage(ctx, "acct-runtime", domain.PageRequest{})
					return err
				},
			} {
				err := call()
				appErr, ok := domain.AsAppError(err)
				if !ok || appErr.Status != 403 {
					t.Fatalf("expected %s forbidden for %s, got %v", endpoint, test.name, err)
				}
				if test.expectDataScopeDeny && appErr.ReasonCode != "data_scope_denied" {
					t.Fatalf("expected %s data_scope_denied for %s, got %+v", endpoint, test.name, appErr)
				}
			}
		})
	}
}

// TestAgentUsageAcceptsExplicitTenantScope keeps the dedicated permission usable for tenant managers.
func TestAgentUsageAcceptsExplicitTenantScope(t *testing.T) {
	now := time.Date(2026, 7, 15, 2, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentUsageTenant(t, store, now, "tenant-1", "acct-manager", false)
	if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:          "ps-usage-tenant",
		TenantID:    "tenant-1",
		Name:        "Tenant Usage Reader",
		Permissions: []domain.Permission{{Resource: "agent.usage", Action: domain.ActionRead, Scope: domain.ScopeTenant}},
		CreatedAt:   now,
	}); err != nil {
		t.Fatal(err)
	}
	account, ok, err := store.GetAccount(context.Background(), "tenant-1", "acct-manager")
	if err != nil || !ok {
		t.Fatalf("get account: ok=%v err=%v", ok, err)
	}
	account.DirectPermissionSetIDs = []string{"ps-usage-tenant"}
	if err := store.UpsertAccount(context.Background(), account); err != nil {
		t.Fatal(err)
	}

	usage, err := agentservice.New(service.New(store)).ListAccountUsage(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-manager"},
		domain.AgentAccountUsageQuery{},
		domain.PageRequest{},
	)
	if err != nil || usage.Total != 1 {
		t.Fatalf("expected explicit tenant usage read to succeed, usage=%+v err=%v", usage, err)
	}
}

// seedAgentUsageTenant creates a tenant account and optional workspace Agent management permission.
func seedAgentUsageTenant(t *testing.T, store *memory.Store, now time.Time, tenantID, accountID string, canManage bool) {
	t.Helper()
	if err := store.UpsertTenant(context.Background(), domain.Tenant{ID: tenantID, Name: tenantID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	permissionSetIDs := []string(nil)
	if canManage {
		permissionSetID := "ps-agent-admin"
		if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
			ID:          permissionSetID,
			TenantID:    tenantID,
			Name:        "Agent Admin",
			Permissions: []domain.Permission{{Resource: "agent.usage", Action: domain.ActionRead, Scope: domain.ScopeAll}},
			CreatedAt:   now,
		}); err != nil {
			t.Fatal(err)
		}
		permissionSetIDs = []string{permissionSetID}
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID:                     accountID,
		TenantID:               tenantID,
		DisplayName:            accountID,
		Email:                  accountID + "@example.com",
		Status:                 "active",
		DirectPermissionSetIDs: permissionSetIDs,
		CreatedAt:              now,
	}); err != nil {
		t.Fatal(err)
	}
}
