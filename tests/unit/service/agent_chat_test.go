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

func TestAgentChatUsesInjectedRuntimeAndPersistsRun(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "me", Action: "read", Scope: "all"},
		{Resource: "agent.run", Action: "create", Scope: "all"},
		{Resource: "agent.tool", Action: "call", Target: "get_my_profile", Scope: "all"},
	})
	runtime := fakeAgentChatRuntime{
		run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
			if req.RequestContext.TenantID != "tenant-1" || req.RequestContext.AccountID != "acct-1" {
				t.Fatalf("unexpected request context: %+v", req.RequestContext)
			}
			if req.RunID == "" || req.SessionID == "" {
				t.Fatalf("expected run and session ids, got %+v", req)
			}
			return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "您好，已完成分析。"})
		},
	}
	svc := service.New(store, service.Options{
		Now:              func() time.Time { return now },
		AgentChatEnabled: true,
		AgentChatRuntime: runtime,
	})
	events := []domain.AgentChatEvent{}

	run, err := svc.Agent().Chat(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true},
		domain.AgentChatInput{Message: "帮我看一下资料", Mode: "assistant_chat"},
		func(_ context.Context, event domain.AgentChatEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != string(domain.AgentRunStatusCompleted) || run.Answer != "您好，已完成分析。" || run.Mode != "assistant_chat" {
		t.Fatalf("unexpected run: %+v", run)
	}
	if len(events) != 3 || events[0].Event != domain.AgentChatEventSession || events[1].Event != domain.AgentChatEventMessageDelta || events[2].Event != domain.AgentChatEventDone {
		t.Fatalf("unexpected events: %+v", events)
	}
	stored, err := store.ListAgentRunsByAccount(context.Background(), "tenant-1", "acct-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].ID != run.ID || stored[0].Answer != run.Answer {
		t.Fatalf("expected persisted completed run, got %+v", stored)
	}
}

func TestAgentChatReadOnlyToolsUseRequestContext(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "me", Action: "read", Scope: "all"},
		{Resource: "agent.run", Action: "create", Scope: "all"},
		{Resource: "agent.tool", Action: "call", Target: "get_my_profile", Scope: "all"},
	})
	runtime := fakeAgentChatRuntime{
		run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
			profile, err := req.Tools["get_my_profile"](ctx, map[string]any{
				"tenant_id":  "attacker-tenant",
				"account_id": "attacker-account",
			})
			if err != nil {
				return err
			}
			account, _ := profile["account"].(map[string]any)
			if account["id"] != "acct-1" {
				t.Fatalf("tool trusted forged args instead of request context: %+v", profile)
			}
			return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "ok"})
		},
	}
	svc := service.New(store, service.Options{
		Now:              func() time.Time { return now },
		AgentChatEnabled: true,
		AgentChatRuntime: runtime,
	})

	if _, err := svc.Agent().Chat(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true},
		domain.AgentChatInput{Message: "我的资料"},
		func(context.Context, domain.AgentChatEvent) error { return nil },
	); err != nil {
		t.Fatal(err)
	}
}

func TestAgentChatToolRequiresAgentToolPermission(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
	})
	runtime := fakeAgentChatRuntime{
		run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
			_, err := req.Tools["get_my_profile"](ctx, nil)
			return err
		},
	}
	svc := service.New(store, service.Options{
		Now:              func() time.Time { return now },
		AgentChatEnabled: true,
		AgentChatRuntime: runtime,
	})

	run, err := svc.Agent().Chat(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true},
		domain.AgentChatInput{Message: "我的资料"},
		func(context.Context, domain.AgentChatEvent) error { return nil },
	)
	if err == nil {
		t.Fatal("expected missing tool permission to fail chat")
	}
	if run.Status != string(domain.AgentRunStatusFailed) || run.Answer == "" {
		t.Fatalf("expected failed run to be persisted with error answer, got %+v", run)
	}
}

func TestAgentChatBlocksActiveRunInSameSession(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
	})
	svc := service.New(store, service.Options{
		Now:              func() time.Time { return now },
		AgentChatEnabled: true,
		AgentChatRuntime: fakeAgentChatRuntime{run: func(context.Context, service.AgentChatRuntimeRequest, service.AgentChatEmitFunc) error {
			t.Fatal("runtime should not be called while an active run exists")
			return nil
		}},
	})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}
	session, err := svc.Agent().CreateSession(ctx, domain.CreateAgentSessionInput{Title: "Busy"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgentRun(context.Background(), domain.AgentRun{
		ID:        "active-run",
		TenantID:  "tenant-1",
		AccountID: "acct-1",
		SessionID: session.ID,
		Mode:      "assistant_chat",
		Prompt:    "busy",
		Status:    string(domain.AgentRunStatusRunning),
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Agent().Chat(ctx, domain.AgentChatInput{SessionID: session.ID, Message: "hi"}, nil); err == nil {
		t.Fatal("expected active run conflict")
	}
}

func TestAgentChatClearContextDropsPreviousHistoryFromPrompt(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
	})
	call := 0
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		call++
		if call == 2 {
			if len(req.History) != 0 {
				t.Fatalf("expected history after clear to be empty, got %+v", req.History)
			}
			if strings.Contains(req.Message, "first question") || strings.Contains(req.Message, "first answer") {
				t.Fatalf("prompt retained history before clear: %s", req.Message)
			}
		}
		answer := "first answer"
		if call == 2 {
			answer = "second answer"
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: answer})
	}}
	svc := service.New(store, service.Options{
		Now:              func() time.Time { return now },
		AgentChatEnabled: true,
		AgentChatRuntime: runtime,
	})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}
	first, err := svc.Agent().Chat(ctx, domain.AgentChatInput{Message: "first question"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Agent().ClearSessionContext(ctx, first.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Agent().Chat(ctx, domain.AgentChatInput{SessionID: first.SessionID, Message: "second question"}, nil); err != nil {
		t.Fatal(err)
	}
}

type fakeAgentChatRuntime struct {
	run func(context.Context, service.AgentChatRuntimeRequest, service.AgentChatEmitFunc) error
}

func (f fakeAgentChatRuntime) RunAgentChat(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
	return f.run(ctx, req, emit)
}

func seedAgentChatAccount(t *testing.T, store *memory.Store, now time.Time, permissions []domain.Permission) {
	t.Helper()
	if err := store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:          "ps-agent-chat",
		TenantID:    "tenant-1",
		Name:        "Agent Chat",
		Permissions: permissions,
		CreatedAt:   now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", Name: "Employee One", AccountID: "acct-1", Status: "active", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		DisplayName:            "Agent User",
		Email:                  "agent@example.com",
		EmployeeID:             "emp-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-agent-chat"},
		CreatedAt:              now,
	}); err != nil {
		t.Fatal(err)
	}
}
