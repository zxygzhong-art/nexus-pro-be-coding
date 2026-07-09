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

func TestAgentSessionChatPersistsMessagesAndAutoMemory(t *testing.T) {
	now := time.Date(2026, 7, 9, 11, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "read", Scope: "all"},
		{Resource: "agent.run", Action: "create", Scope: "all"},
		{Resource: "agent.run", Action: "update", Scope: "all"},
		{Resource: "agent.run", Action: "delete", Scope: "all"},
	})
	runtime := fakeAgentChatRuntime{
		run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
			if req.SessionID == "" {
				t.Fatal("expected chat request to include session id")
			}
			if !strings.Contains(req.Message, "Known facts:") {
				t.Fatalf("expected runtime message to include memory context, got %q", req.Message)
			}
			return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "已记住。"})
		},
	}
	svc := service.New(store, service.Options{
		Now:              func() time.Time { return now },
		AgentChatEnabled: true,
		AgentChatRuntime: runtime,
	})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}

	run, err := svc.Agent().Chat(ctx, domain.AgentChatInput{Message: "记住我喜欢特休"}, func(context.Context, domain.AgentChatEvent) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if run.SessionID == "" {
		t.Fatalf("expected run session id, got %+v", run)
	}

	sessions, err := svc.Agent().ListSessions(ctx, domain.ListAgentSessionsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].ID != run.SessionID || sessions[0].Status != domain.AgentSessionStatusActive {
		t.Fatalf("expected one active session, got %+v", sessions)
	}
	if sessions[0].Title == "" || !strings.Contains(sessions[0].Title, "记住我喜欢特休") {
		t.Fatalf("expected session title from first message, got %+v", sessions[0])
	}

	messages, err := svc.Agent().ListSessionMessages(ctx, run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Role != domain.AgentMessageRoleUser || messages[1].Role != domain.AgentMessageRoleAssistant {
		t.Fatalf("expected user and assistant messages, got %+v", messages)
	}
	if messages[0].Content != "记住我喜欢特休" || messages[1].Content != "已记住。" {
		t.Fatalf("unexpected persisted message content: %+v", messages)
	}

	memories, err := svc.Agent().ListMemories(ctx, domain.ListAgentMemoriesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 1 || memories[0].Source != domain.AgentMemorySourceAuto || memories[0].Key != "preference" || !strings.Contains(memories[0].Content, "特休") {
		t.Fatalf("expected auto preference memory, got %+v", memories)
	}
}
