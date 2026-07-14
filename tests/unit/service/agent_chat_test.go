package service_test

import (
	"context"
	"encoding/json"
	"errors"
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
			if _, ok := req.Tools["knowledge.search"]; !ok {
				t.Fatalf("tool catalog entry knowledge.search is missing from runtime tools: %+v", req.Tools)
			}
			if !strings.Contains(req.Message, "Known facts:") || !strings.Contains(req.Message, "User: 帮我看一下资料") {
				t.Fatalf("expected runtime message to retain assembled context, got %q", req.Message)
			}
			return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "您好，已完成分析。"})
		},
	}
	svc := service.New(store, service.Options{
		Now:              func() time.Time { return now },
		AgentChatRuntime: runtime,
	})
	events := []domain.AgentChatEvent{}

	run, err := svc.Agent().Chat(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		domain.AgentChatInput{Message: "帮我看一下资料", Mode: "assistant_chat"},
		func(_ context.Context, event domain.AgentChatEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != string(domain.AgentRunStatusCompleted) || run.Answer != "您好，已完成分析。" || run.Mode != "assistant_chat" || run.Prompt != "帮我看一下资料" {
		t.Fatalf("unexpected run: %+v", run)
	}
	if len(events) != 3 || events[0].Event != domain.AgentChatEventSession || events[1].Event != domain.AgentChatEventMessageDelta || events[2].Event != domain.AgentChatEventDone {
		t.Fatalf("unexpected events: %+v", events)
	}
	stored, err := store.ListAgentRunsByAccount(context.Background(), "tenant-1", "acct-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].ID != run.ID || stored[0].Answer != run.Answer || stored[0].Prompt != "帮我看一下资料" {
		t.Fatalf("expected persisted completed run, got %+v", stored)
	}
}

func TestAgentChatRecommendationUsesVisibleAssistantCatalog(t *testing.T) {
	now := time.Date(2026, 7, 13, 11, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
	})
	for _, agent := range []domain.AgentDefinition{
		{
			ID: "agent-payroll", TenantID: "tenant-1", Name: "薪资助理", Description: "协助薪资查询与差异说明",
			Category: domain.AgentCategoryAnalytics, Status: domain.AgentDefinitionStatusPublished,
			Visibility: domain.AgentVisibilityAll, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "agent-draft", TenantID: "tenant-1", Name: "未发布助理", Description: "不应出现在推荐上下文",
			Category: domain.AgentCategoryWorkflow, Status: domain.AgentDefinitionStatusDraft,
			Visibility: domain.AgentVisibilityAll, CreatedAt: now, UpdatedAt: now,
		},
	} {
		if err := store.UpsertAgentDefinition(context.Background(), agent); err != nil {
			t.Fatal(err)
		}
	}
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		if req.Mode != "assistant_recommendation" || req.AgentName != "助理推荐" {
			t.Fatalf("unexpected recommendation runtime request: %+v", req)
		}
		if !strings.Contains(req.Message, "薪资助理") || strings.Contains(req.Message, "未发布助理") {
			t.Fatalf("recommendation prompt did not contain only visible published assistants: %s", req.Message)
		}
		if len(req.Tools) != 0 {
			t.Fatalf("recommendation mode should not expose business tools: %+v", req.Tools)
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "建议使用薪资助理。"})
	}}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	run, err := svc.Agent().Chat(ctx, domain.AgentChatInput{Message: "谁能解释薪资差异？", Mode: "assistant_recommendation"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.Answer != "建议使用薪资助理。" || run.SessionID == "" {
		t.Fatalf("unexpected recommendation run: %+v", run)
	}
	session, ok, err := store.GetAgentSession(context.Background(), "tenant-1", run.SessionID)
	if err != nil || !ok {
		t.Fatalf("expected recommendation session, ok=%v err=%v", ok, err)
	}
	if session.Title != "谁能解释薪资差异？" {
		t.Fatalf("expected user text to remain the conversation title, got %q", session.Title)
	}
}

func TestAgentChatRecommendationFallsBackToVisibleCatalog(t *testing.T) {
	now := time.Date(2026, 7, 13, 11, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
	})
	for _, agent := range []domain.AgentDefinition{
		{
			ID: "agent-leave", TenantID: "tenant-1", Name: "请假流程助理", Description: "协助员工请假申请与流程说明",
			Category: domain.AgentCategoryWorkflow, Status: domain.AgentDefinitionStatusPublished,
			Visibility: domain.AgentVisibilityAll, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "agent-sales", TenantID: "tenant-1", Name: "业务分析助理", Description: "分析销售与业绩报表",
			Category: domain.AgentCategoryAnalytics, Status: domain.AgentDefinitionStatusPublished,
			Visibility: domain.AgentVisibilityAll, CreatedAt: now, UpdatedAt: now,
		},
	} {
		if err := store.UpsertAgentDefinition(context.Background(), agent); err != nil {
			t.Fatal(err)
		}
	}
	runtime := fakeAgentChatRuntime{run: func(context.Context, service.AgentChatRuntimeRequest, service.AgentChatEmitFunc) error {
		return errors.New("model unavailable")
	}}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime})
	events := []domain.AgentChatEvent{}

	run, err := svc.Agent().Chat(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		domain.AgentChatInput{Message: "请推荐处理员工请假的助理", Mode: "assistant_recommendation"},
		func(_ context.Context, event domain.AgentChatEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != string(domain.AgentRunStatusCompleted) || !strings.Contains(run.Answer, "请假流程助理") {
		t.Fatalf("expected completed catalog fallback, got %+v", run)
	}
	if len(events) != 3 || events[1].Event != domain.AgentChatEventMessageDelta || events[2].Event != domain.AgentChatEventDone {
		t.Fatalf("unexpected fallback events: %+v", events)
	}
	messages, err := store.ListAgentSessionMessages(context.Background(), "tenant-1", run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[1].Role != domain.AgentMessageRoleAssistant || messages[1].Content != run.Answer {
		t.Fatalf("expected persisted fallback answer, got %+v", messages)
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
		AgentChatRuntime: runtime,
	})

	if _, err := svc.Agent().Chat(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
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
		AgentChatRuntime: runtime,
	})

	run, err := svc.Agent().Chat(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
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
		Now: func() time.Time { return now },
		AgentChatRuntime: fakeAgentChatRuntime{run: func(context.Context, service.AgentChatRuntimeRequest, service.AgentChatEmitFunc) error {
			t.Fatal("runtime should not be called while an active run exists")
			return nil
		}},
	})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
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

func TestAgentChatUsesSessionBoundAgentAndRejectsAgentSwitch(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
	})
	if err := store.UpsertAgentModel(context.Background(), domain.AgentModel{
		ID: "model-bound", TenantID: "tenant-1", Name: "Bound Model", ModelName: "gpt-bound",
		LiteLLMModel: "openai/gpt-bound", Status: domain.AgentModelStatusActive, TimeoutSeconds: 45,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgentDefinition(context.Background(), domain.AgentDefinition{
		ID: "agent-bound", TenantID: "tenant-1", Name: "Bound Agent", ModelID: "model-bound",
		SystemPrompt: "Bound system prompt", Status: domain.AgentDefinitionStatusPublished,
		Visibility: domain.AgentVisibilityAll, TimeoutSeconds: 30, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgentSession(context.Background(), domain.AgentSession{
		ID: "session-bound", TenantID: "tenant-1", AccountID: "acct-1", AgentID: "agent-bound",
		Title: "Bound conversation", Status: domain.AgentSessionStatusActive, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	runtimeCalls := 0
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		runtimeCalls++
		if req.ModelName != "openai/gpt-bound" || !strings.Contains(req.Message, "Bound system prompt") {
			t.Fatalf("session did not restore its bound agent config: %+v", req)
		}
		deadline, ok := ctx.Deadline()
		remaining := time.Until(deadline)
		if !ok || remaining < 44*time.Second || remaining > 46*time.Second {
			t.Fatalf("expected the model's 45 second timeout to win over the legacy agent value")
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "bound answer"})
	}}})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	run, err := svc.Agent().Chat(ctx, domain.AgentChatInput{SessionID: "session-bound", Message: "continue"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.AgentID != "agent-bound" || runtimeCalls != 1 {
		t.Fatalf("expected bound agent run, got run=%+v calls=%d", run, runtimeCalls)
	}
	if _, err := svc.Agent().Chat(ctx, domain.AgentChatInput{SessionID: "session-bound", AgentID: "other-agent", Message: "switch"}, nil); err == nil {
		t.Fatal("expected switching the agent bound to an existing session to fail")
	}
	if runtimeCalls != 1 {
		t.Fatalf("runtime ran for a mismatched agent: %d", runtimeCalls)
	}
}

func TestAgentChatClearContextDropsPreviousHistoryFromPrompt(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "read", Scope: "all"},
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
		AgentChatRuntime: runtime,
	})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	first, err := svc.Agent().Chat(ctx, domain.AgentChatInput{Message: "first question"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cleared, err := svc.Agent().ClearSessionContext(ctx, first.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.ContextVersion != 2 || cleared.LastMessageAt != nil {
		t.Fatalf("expected clear to advance the visible context partition, got %+v", cleared)
	}
	visible, err := svc.Agent().ListSessionMessages(ctx, first.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 0 {
		t.Fatalf("expected messages before clear to be invisible, got %+v", visible)
	}
	if _, err := svc.Agent().Chat(ctx, domain.AgentChatInput{SessionID: first.SessionID, Message: "second question"}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestAgentChatFilesAreBoundToMessagesAndHiddenAfterClear(t *testing.T) {
	now := time.Date(2026, 7, 14, 11, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "read", Scope: "all"},
		{Resource: "agent.run", Action: "create", Scope: "all"},
	})
	objectStore := service.NewMemoryObjectStore()
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		if !strings.Contains(req.Message, "quarterly revenue is 42") {
			t.Fatalf("expected parsed attachment content in runtime context, got %q", req.Message)
		}
		if !strings.Contains(req.Message, "untrusted user-provided data") {
			t.Fatalf("expected attachment content to be marked untrusted, got %q", req.Message)
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "file reviewed"})
	}}
	svc := service.New(store, service.Options{
		Now: func() time.Time { return now }, AgentChatRuntime: runtime, ObjectStore: objectStore,
	})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	session, err := svc.Agent().CreateSession(ctx, domain.CreateAgentSessionInput{Title: "File review"})
	if err != nil {
		t.Fatal(err)
	}
	file, err := svc.Agent().UploadSessionFile(ctx, session.ID, domain.UploadAgentSessionFileInput{
		Filename: "report.txt", ContentType: "text/plain", Content: []byte("quarterly revenue is 42"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if file.State != "draft" || file.ContextVersion != 1 {
		t.Fatalf("unexpected staged file: %+v", file)
	}
	download, err := svc.Agent().DownloadSessionFile(ctx, session.ID, file.ID)
	if err != nil || string(download.Content) != "quarterly revenue is 42" {
		t.Fatalf("unexpected staged download: content=%q err=%v", download.Content, err)
	}
	if _, err := svc.Agent().Chat(ctx, domain.AgentChatInput{
		SessionID: session.ID, Message: "review this", AttachmentIDs: []string{file.ID},
	}, nil); err != nil {
		t.Fatal(err)
	}
	messages, err := svc.Agent().ListSessionMessages(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || len(messages[0].Attachments) != 1 || messages[0].Attachments[0].ID != file.ID {
		t.Fatalf("expected turn-level attachment provenance, got %+v", messages)
	}
	rawMessage, err := json.Marshal(messages[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawMessage), "object_key") || strings.Contains(string(rawMessage), file.ObjectKey) {
		t.Fatalf("storage key must not be serialized through message attachments: %s", rawMessage)
	}
	if _, err := svc.Agent().ClearSessionContext(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	messages, err = svc.Agent().ListSessionMessages(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected cleared messages to be invisible, got %+v", messages)
	}
	files, err := svc.Agent().ListSessionFiles(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("expected cleared files to be invisible, got %+v", files)
	}
	if _, err := svc.Agent().DownloadSessionFile(ctx, session.ID, file.ID); err == nil {
		t.Fatal("expected old context file download to be hidden")
	}
}

// TestAgentChatClearCannotSplitAnActiveRunAcrossContextVersions verifies clear waits for a complete turn boundary.
func TestAgentChatClearCannotSplitAnActiveRunAcrossContextVersions(t *testing.T) {
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
		{Resource: "agent.run", Action: "read", Scope: "all"},
	})
	started := make(chan struct{})
	release := make(chan struct{})
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, _ service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		close(started)
		<-release
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "done"})
	}}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	session, err := svc.Agent().CreateSession(ctx, domain.CreateAgentSessionInput{Title: "active turn"})
	if err != nil {
		t.Fatal(err)
	}
	type chatResult struct {
		run domain.AgentRun
		err error
	}
	result := make(chan chatResult, 1)
	go func() {
		run, err := svc.Agent().Chat(ctx, domain.AgentChatInput{SessionID: session.ID, Message: "hello"}, nil)
		result <- chatResult{run: run, err: err}
	}()
	<-started
	if _, err := svc.Agent().ClearSessionContext(ctx, session.ID); err == nil {
		t.Fatal("expected clear to reject while the session has an active run")
	}
	close(release)
	completed := <-result
	if completed.err != nil || completed.run.Status != string(domain.AgentRunStatusCompleted) {
		t.Fatalf("expected the active turn to complete atomically, run=%+v err=%v", completed.run, completed.err)
	}
	cleared, err := svc.Agent().ClearSessionContext(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.ContextVersion != session.ContextVersion+1 {
		t.Fatalf("expected context version to advance after the turn, got %+v", cleared)
	}
	messages, err := svc.Agent().ListSessionMessages(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected the completed old-version turn to be hidden, got %+v", messages)
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
