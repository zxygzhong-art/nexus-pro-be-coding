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
