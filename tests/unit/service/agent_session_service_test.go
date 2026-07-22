package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
	agentservice "nexus-pro-api/internal/service/agent"
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
			return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "已記住。"})
		},
	}
	svc := service.New(store, service.Options{
		Now:              func() time.Time { return now },
		AgentChatRuntime: runtime,
	})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	run, err := agentservice.New(svc).Chat(ctx, domain.AgentChatInput{Message: "記住我喜歡特休"}, func(context.Context, domain.AgentChatEvent) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if run.SessionID == "" {
		t.Fatalf("expected run session id, got %+v", run)
	}

	sessionPage, err := agentservice.New(svc).ListSessions(ctx, domain.ListAgentSessionsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	sessions := sessionPage.Items
	if len(sessions) != 1 || sessions[0].ID != run.SessionID || sessions[0].Status != domain.AgentSessionStatusActive {
		t.Fatalf("expected one active session, got %+v", sessions)
	}
	if sessions[0].Title == "" || !strings.Contains(sessions[0].Title, "記住我喜歡特休") {
		t.Fatalf("expected session title from first message, got %+v", sessions[0])
	}

	messagePage, err := agentservice.New(svc).ListSessionMessages(ctx, run.SessionID, domain.ListAgentSessionMessagesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	messages := messagePage.Items
	if len(messages) != 2 || messages[0].Role != domain.AgentMessageRoleUser || messages[1].Role != domain.AgentMessageRoleAssistant {
		t.Fatalf("expected user and assistant messages, got %+v", messages)
	}
	if messages[0].Content != "記住我喜歡特休" || messages[1].Content != "已記住。" {
		t.Fatalf("unexpected persisted message content: %+v", messages)
	}

	memories, err := agentservice.New(svc).ListMemories(ctx, domain.ListAgentMemoriesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 1 || memories[0].Source != domain.AgentMemorySourceAuto || memories[0].Key != "preference" || !strings.Contains(memories[0].Content, "特休") {
		t.Fatalf("expected auto preference memory, got %+v", memories)
	}
	if memories[0].Scope != "global" || memories[0].SourceMessageID != messages[0].ID || memories[0].Status != "active" {
		t.Fatalf("expected memory provenance to bind the persisted user message, got %+v", memories[0])
	}
}

// TestAgentSessionListKeysetPagination verifies created_at,id keyset paging keeps filters working.
func TestAgentSessionListKeysetPagination(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "read", Scope: "all"},
		{Resource: "agent.run", Action: "create", Scope: "all"},
		{Resource: "agent.run", Action: "update", Scope: "all"},
	})
	current := now
	svc := service.New(store, service.Options{Now: func() time.Time { return current }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		current = now.Add(time.Duration(i) * time.Minute)
		session, err := agentservice.New(svc).CreateSession(ctx, domain.CreateAgentSessionInput{Title: fmt.Sprintf("session-%d", i)})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, session.ID)
	}

	first, err := agentservice.New(svc).ListSessions(ctx, domain.ListAgentSessionsQuery{PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.Items[0].ID != ids[2] || first.Items[1].ID != ids[1] {
		t.Fatalf("expected newest sessions first, got %+v", first.Items)
	}
	if first.NextCursor == "" {
		t.Fatal("expected next cursor on a full first page")
	}

	second, err := agentservice.New(svc).ListSessions(ctx, domain.ListAgentSessionsQuery{PageSize: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].ID != ids[0] {
		t.Fatalf("expected the remaining oldest session, got %+v", second.Items)
	}
	if second.NextCursor != "" {
		t.Fatalf("expected no cursor after the last page, got %q", second.NextCursor)
	}

	if _, err := agentservice.New(svc).ListSessions(ctx, domain.ListAgentSessionsQuery{Cursor: "!!not-a-cursor!!"}); err == nil {
		t.Fatal("expected an invalid cursor to be rejected")
	}

	if _, err := agentservice.New(svc).UpdateSession(ctx, ids[2], domain.UpdateAgentSessionInput{Status: ptrTo(string(domain.AgentSessionStatusArchived))}); err != nil {
		t.Fatal(err)
	}
	archived, err := agentservice.New(svc).ListSessions(ctx, domain.ListAgentSessionsQuery{Status: "archived"})
	if err != nil {
		t.Fatal(err)
	}
	if len(archived.Items) != 1 || archived.Items[0].ID != ids[2] {
		t.Fatalf("expected status filter to keep working with paging, got %+v", archived.Items)
	}
}

// TestAgentSessionMessageListKeysetPagination verifies ascending created_at,id keyset paging
// and that the visible context version filter still applies.
func TestAgentSessionMessageListKeysetPagination(t *testing.T) {
	now := time.Date(2026, 7, 9, 13, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "read", Scope: "all"},
		{Resource: "agent.run", Action: "create", Scope: "all"},
	})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	session, err := agentservice.New(svc).CreateSession(ctx, domain.CreateAgentSessionInput{Title: "paged messages"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := store.InsertAgentSessionMessage(context.Background(), domain.AgentSessionMessage{
			ID:             fmt.Sprintf("amsg-%d", i),
			TenantID:       "tenant-1",
			SessionID:      session.ID,
			Role:           domain.AgentMessageRoleUser,
			Content:        fmt.Sprintf("message-%d", i),
			ContextVersion: 1,
			CreatedAt:      now.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}
	// A message from a non-visible context version must stay hidden.
	if err := store.InsertAgentSessionMessage(context.Background(), domain.AgentSessionMessage{
		ID:             "amsg-hidden",
		TenantID:       "tenant-1",
		SessionID:      session.ID,
		Role:           domain.AgentMessageRoleUser,
		Content:        "hidden",
		ContextVersion: 99,
		CreatedAt:      now.Add(3 * time.Second),
	}); err != nil {
		t.Fatal(err)
	}

	first, err := agentservice.New(svc).ListSessionMessages(ctx, session.ID, domain.ListAgentSessionMessagesQuery{PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.Items[0].ID != "amsg-0" || first.Items[1].ID != "amsg-1" {
		t.Fatalf("expected oldest messages first, got %+v", first.Items)
	}
	if first.NextCursor == "" {
		t.Fatal("expected next cursor on a full first page")
	}

	second, err := agentservice.New(svc).ListSessionMessages(ctx, session.ID, domain.ListAgentSessionMessagesQuery{PageSize: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].ID != "amsg-2" {
		t.Fatalf("expected the remaining newest message, got %+v", second.Items)
	}
	if second.NextCursor != "" {
		t.Fatalf("expected no cursor after the last page, got %q", second.NextCursor)
	}

	if _, err := agentservice.New(svc).ListSessionMessages(ctx, session.ID, domain.ListAgentSessionMessagesQuery{Cursor: "!!not-a-cursor!!"}); err == nil {
		t.Fatal("expected an invalid cursor to be rejected")
	}
}

func ptrTo(v string) *string {
	return &v
}
