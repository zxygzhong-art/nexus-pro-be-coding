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

func TestAgentAdminCreatesPublishesTrialsAndRollsBackAgent(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentAdminAccount(t, store, now)
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin", ApprovalConfirmed: true}

	model, err := svc.Agent().CreateModel(ctx, domain.CreateAgentModelInput{
		Name:         "GPT 4.1",
		Provider:     "openai",
		ModelName:    "gpt-4.1",
		LiteLLMModel: "openai/gpt-4.1",
		IsDefault:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if model.Status != domain.AgentModelStatusActive || !model.IsDefault || model.TimeoutSeconds <= 0 {
		t.Fatalf("unexpected model defaults: %+v", model)
	}

	agent, err := svc.Agent().CreateDefinition(ctx, domain.CreateAgentDefinitionInput{
		Name:         "HR Helper",
		Description:  "Answers HR questions",
		ModelID:      model.ID,
		SystemPrompt: "You are an HR helper.",
		Tools:        []string{"get_my_profile"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if agent.Status != domain.AgentDefinitionStatusDraft || agent.Version != 1 {
		t.Fatalf("unexpected draft agent: %+v", agent)
	}

	updatedPrompt := "You are a careful HR helper."
	status := string(domain.AgentDefinitionStatusPublished)
	agent, err = svc.Agent().UpdateDefinition(ctx, agent.ID, domain.UpdateAgentDefinitionInput{
		SystemPrompt: &updatedPrompt,
		Status:       &status,
		VersionNote:  "publish careful prompt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if agent.Status != domain.AgentDefinitionStatusPublished || agent.Version != 2 {
		t.Fatalf("expected published version 2, got %+v", agent)
	}

	trial, err := svc.Agent().Trial(ctx, agent.ID, domain.AgentTrialInput{Message: "How do I request leave?"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(trial.Reply, "HR Helper") || trial.ModelName != "gpt-4.1" {
		t.Fatalf("unexpected trial reply: %+v", trial)
	}
	if len(trial.ToolsUsed) != 0 {
		t.Fatalf("mock trial should not claim tools were used, got %+v", trial.ToolsUsed)
	}
	agent, _, err = store.GetAgentDefinition(context.Background(), "tenant-1", agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Usage.TotalRuns != 1 || agent.Usage.SuccessRuns != 1 || agent.Usage.LastRunAt == nil {
		t.Fatalf("expected usage updated after trial, got %+v", agent.Usage)
	}

	rolledBack, err := svc.Agent().RollbackDefinition(ctx, agent.ID, domain.RollbackAgentDefinitionInput{Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.Version != 3 || rolledBack.SystemPrompt != "You are an HR helper." {
		t.Fatalf("expected rollback to create version 3 from v1, got %+v", rolledBack)
	}
}

func TestAgentAdminTrialReportsOnlyActuallyUsedTools(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentAdminAccount(t, store, now)
	runtime := agentAdminFakeRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		tool, ok := req.Tools["get_my_profile"]
		if !ok {
			t.Fatalf("expected get_my_profile to be available in trial tools: %+v", req.Tools)
		}
		if _, ok := req.Tools["workspace_insights"]; ok {
			t.Fatalf("trial exposed a tool not configured on the agent: %+v", req.Tools)
		}
		_ = tool
		if err := emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventToolResult, Name: "get_my_profile", Status: "ok"}); err != nil {
			return err
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "profile checked"})
	}}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin", ApprovalConfirmed: true}

	model, err := svc.Agent().CreateModel(ctx, domain.CreateAgentModelInput{
		Name:         "GPT 4.1",
		ModelName:    "gpt-4.1",
		LiteLLMModel: "openai/gpt-4.1",
		IsDefault:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	status := string(domain.AgentDefinitionStatusPublished)
	agent, err := svc.Agent().CreateDefinition(ctx, domain.CreateAgentDefinitionInput{
		Name:    "HR Helper",
		ModelID: model.ID,
		Tools:   []string{"get_my_profile", "my_leave_balances"},
		Status:  status,
	})
	if err != nil {
		t.Fatal(err)
	}

	trial, err := svc.Agent().Trial(ctx, agent.ID, domain.AgentTrialInput{Message: "Who am I?"})
	if err != nil {
		t.Fatal(err)
	}
	if trial.Reply != "profile checked" {
		t.Fatalf("unexpected trial reply: %+v", trial)
	}
	if len(trial.ToolsUsed) != 1 || trial.ToolsUsed[0] != "get_my_profile" {
		t.Fatalf("expected only actually used tool, got %+v", trial.ToolsUsed)
	}
}

func TestAgentAdminBlocksDeletingModelInUse(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentAdminAccount(t, store, now)
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin", ApprovalConfirmed: true}

	model, err := svc.Agent().CreateModel(ctx, domain.CreateAgentModelInput{Name: "Default", ModelName: "gpt-4.1", LiteLLMModel: "openai/gpt-4.1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Agent().CreateDefinition(ctx, domain.CreateAgentDefinitionInput{Name: "Agent", ModelID: model.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Agent().DeleteModel(ctx, model.ID); err == nil {
		t.Fatal("expected deleting a model used by an agent to be blocked")
	}
}

func TestAgentAdminImportBundleNormalizesAndSnapshots(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentAdminAccount(t, store, now)
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin", ApprovalConfirmed: true}

	result, err := svc.Agent().ImportBundle(ctx, domain.AgentBundle{
		Models: []domain.AgentModel{
			{
				ID:              "model-primary",
				Name:            "Primary",
				ModelName:       "gpt-4.1",
				LiteLLMModel:    "openai/gpt-4.1",
				FallbackModelID: "model-fallback",
			},
			{
				ID:           "model-fallback",
				Name:         "Fallback",
				ModelName:    "gpt-4.1-mini",
				LiteLLMModel: "openai/gpt-4.1-mini",
			},
		},
		Agents: []domain.AgentDefinition{
			{
				ID:           "agent-imported",
				Name:         "Imported Agent",
				ModelID:      "model-primary",
				SystemPrompt: "Use verified tools only.",
				Tools:        []string{"get_my_profile", "get_my_profile"},
				Status:       domain.AgentDefinitionStatusPublished,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Models != 2 || result.Agents != 1 {
		t.Fatalf("unexpected import result: %+v", result)
	}
	agent, err := svc.Agent().GetDefinition(ctx, "agent-imported")
	if err != nil {
		t.Fatal(err)
	}
	if agent.TenantID != "tenant-1" || agent.Version != 1 || len(agent.Tools) != 1 {
		t.Fatalf("expected normalized imported agent, got %+v", agent)
	}
	versions, err := svc.Agent().ListDefinitionVersions(ctx, "agent-imported")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].Note != "imported bundle" {
		t.Fatalf("expected imported version snapshot, got %+v", versions)
	}
}

func TestAgentAdminImportBundleRejectsInvalidAgentWithoutPartialWrite(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentAdminAccount(t, store, now)
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin", ApprovalConfirmed: true}

	_, err := svc.Agent().ImportBundle(ctx, domain.AgentBundle{
		Models: []domain.AgentModel{{
			ID:           "model-imported",
			Name:         "Imported",
			ModelName:    "gpt-4.1",
			LiteLLMModel: "openai/gpt-4.1",
		}},
		Agents: []domain.AgentDefinition{{
			ID:           "agent-bad-tool",
			Name:         "Bad Tool Agent",
			ModelID:      "model-imported",
			SystemPrompt: "bad",
			Tools:        []string{"unknown_tool"},
		}},
	})
	if err == nil {
		t.Fatal("expected invalid tool to reject import")
	}
	models, err := svc.Agent().ListModels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 0 {
		t.Fatalf("expected failed import to roll back models, got %+v", models)
	}
}

func seedAgentAdminAccount(t *testing.T, store *memory.Store, now time.Time) {
	t.Helper()
	if err := store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-agent-admin",
		TenantID: "tenant-1",
		Name:     "Agent Admin",
		Permissions: []domain.Permission{
			{Resource: "agent.model", Action: "read", Scope: "all"},
			{Resource: "agent.model", Action: "create", Scope: "all"},
			{Resource: "agent.model", Action: "update", Scope: "all"},
			{Resource: "agent.model", Action: "delete", Scope: "all"},
			{Resource: "agent.definition", Action: "read", Scope: "all"},
			{Resource: "agent.definition", Action: "create", Scope: "all"},
			{Resource: "agent.definition", Action: "update", Scope: "all"},
			{Resource: "agent.definition", Action: "delete", Scope: "all"},
			{Resource: "agent.tool", Action: "call", Target: "get_my_profile", Scope: "all"},
			{Resource: "me", Action: "read", Scope: "self"},
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-admin",
		TenantID:               "tenant-1",
		DisplayName:            "Agent Admin",
		Email:                  "admin@example.com",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-agent-admin"},
		CreatedAt:              now,
	}); err != nil {
		t.Fatal(err)
	}
}

type agentAdminFakeRuntime struct {
	run func(context.Context, service.AgentChatRuntimeRequest, service.AgentChatEmitFunc) error
}

func (f agentAdminFakeRuntime) RunAgentChat(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
	return f.run(ctx, req, emit)
}
