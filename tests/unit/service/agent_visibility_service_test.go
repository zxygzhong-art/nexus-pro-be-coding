package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

// TestPlatformAssistantsDoNotFallbackToStaticCatalog 驗證未發布 Agent 時不洩漏靜態假清單。
func TestPlatformAssistantsDoNotFallbackToStaticCatalog(t *testing.T) {
	now := time.Date(2026, 7, 13, 11, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{{Resource: "agent.run", Action: "create", Scope: "all"}})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, CredentialCipher: newTestCredentialCipher(t)})

	response, err := svc.Platform().ListAssistants(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		domain.PlatformAssistantsQuery{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if response.Total != 0 || len(response.Data) != 0 {
		t.Fatalf("expected an empty managed assistant list, got %+v", response.Data)
	}
}

// TestPlatformAssistantsUsePublishedConversationExperience keeps draft edits out of the runtime assistant page.
func TestPlatformAssistantsUsePublishedConversationExperience(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentAdminAccount(t, store, now)
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, CredentialCipher: newTestCredentialCipher(t)})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}
	model, err := svc.Agent().CreateModel(ctx, domain.CreateAgentModelInput{
		Name: "Model", ModelName: "gpt-4.1", LiteLLMModel: "openai/gpt-4.1", APIKey: "sk-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := svc.Agent().CreateDefinition(ctx, domain.CreateAgentDefinitionInput{
		Name: "Leave Agent", ModelID: model.ID, WelcomeMessage: "Published welcome", SuggestedQuestions: []string{"Published question"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Agent().PublishDefinition(ctx, agent.ID); err != nil {
		t.Fatal(err)
	}
	draftWelcome := "Draft welcome"
	if _, err = svc.Agent().UpdateDefinition(ctx, agent.ID, domain.UpdateAgentDefinitionInput{
		WelcomeMessage: &draftWelcome, SuggestedQuestions: []string{"Draft question"},
	}); err != nil {
		t.Fatal(err)
	}

	response, err := svc.Platform().ListAssistants(ctx, domain.PlatformAssistantsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Data) != 1 || response.Data[0].WelcomeMessage != "Published welcome" || len(response.Data[0].SuggestedQuestions) != 1 || response.Data[0].SuggestedQuestions[0] != "Published question" {
		t.Fatalf("expected deployed conversation experience, got %+v", response.Data)
	}

	if _, err = svc.Agent().PublishDefinition(ctx, agent.ID); err != nil {
		t.Fatal(err)
	}
	response, err = svc.Platform().ListAssistants(ctx, domain.PlatformAssistantsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if response.Data[0].WelcomeMessage != "Draft welcome" || response.Data[0].SuggestedQuestions[0] != "Draft question" {
		t.Fatalf("expected newly deployed conversation experience, got %+v", response.Data[0])
	}
}

// TestPlatformAssistantsLocalizePublishedSuggestions verifies account-locale selection and per-item fallback.
func TestPlatformAssistantsLocalizePublishedSuggestions(t *testing.T) {
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentAdminAccount(t, store, now)
	if _, ok, err := store.UpdateAccountPreferredLocale(
		context.Background(),
		"tenant-1",
		"acct-admin",
		domain.PreferredLocaleENUS,
	); err != nil || !ok {
		t.Fatalf("set preferred locale: ok=%v err=%v", ok, err)
	}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, CredentialCipher: newTestCredentialCipher(t)})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}
	model, err := svc.Agent().CreateModel(ctx, domain.CreateAgentModelInput{
		Name: "Model", ModelName: "gpt-4.1", LiteLLMModel: "openai/gpt-4.1", APIKey: "sk-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := svc.Agent().CreateDefinition(ctx, domain.CreateAgentDefinitionInput{
		Name:    "Leave Agent",
		ModelID: model.ID,
		SuggestedQuestionTranslations: []domain.LocalizedAgentSuggestedQuestion{
			{Translations: map[string]string{
				domain.PreferredLocaleZHTW: "幫我請特休",
				domain.PreferredLocaleENUS: "Request annual leave",
			}},
			{Translations: map[string]string{
				domain.PreferredLocaleZHTW: "查詢假期餘額",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(agent.SuggestedQuestions) != 2 || agent.SuggestedQuestions[0] != "幫我請特休" {
		t.Fatalf("expected zh-TW compatibility projection, got %+v", agent.SuggestedQuestions)
	}
	if _, err = svc.Agent().PublishDefinition(ctx, agent.ID); err != nil {
		t.Fatal(err)
	}

	response, err := svc.Platform().ListAssistants(ctx, domain.PlatformAssistantsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Data) != 1 || len(response.Data[0].SuggestedQuestions) != 2 {
		t.Fatalf("expected localized suggestions, got %+v", response.Data)
	}
	if response.Data[0].SuggestedQuestions[0] != "Request annual leave" || response.Data[0].SuggestedQuestions[1] != "查詢假期餘額" {
		t.Fatalf("expected en-US with zh-TW fallback, got %+v", response.Data[0].SuggestedQuestions)
	}
}

func TestPublishedAssistantsEnforceDepartmentVisibility(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{{Resource: "agent.run", Action: "create", Scope: "all"}})
	if err := store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-hr", TenantID: "tenant-1", Name: "HR", Path: []string{"ou-hr"}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-sales", TenantID: "tenant-1", Name: "Sales", Path: []string{"ou-sales"}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-1", TenantID: "tenant-1", AccountID: "acct-1", Name: "Employee One", OrgUnitID: "ou-hr", Status: "active", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	for _, agent := range []domain.AgentDefinition{
		{ID: "agent-visible", TenantID: "tenant-1", Name: "Visible", Category: domain.AgentCategoryWorkflow, Status: domain.AgentDefinitionStatusPublished, Visibility: domain.AgentVisibilityDepartment, VisibilityTargets: []string{"ou-hr"}, CreatedAt: now, UpdatedAt: now},
		{ID: "agent-hidden", TenantID: "tenant-1", Name: "Hidden", Category: domain.AgentCategoryWorkflow, Status: domain.AgentDefinitionStatusPublished, Visibility: domain.AgentVisibilityDepartment, VisibilityTargets: []string{"ou-sales"}, CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.UpsertAgentDefinition(context.Background(), agent); err != nil {
			t.Fatal(err)
		}
	}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: fakeAgentChatRuntime{run: func(context.Context, service.AgentChatRuntimeRequest, service.AgentChatEmitFunc) error {
		t.Fatal("hidden agent must be rejected before runtime")
		return nil
	}}})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	response, err := svc.Platform().ListAssistants(ctx, domain.PlatformAssistantsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Data) != 1 || response.Data[0].ID != "agent-visible" || !response.Data[0].Runnable {
		t.Fatalf("expected only the runnable department assistant, got %+v", response.Data)
	}
	if _, err := svc.Agent().Chat(ctx, domain.AgentChatInput{AgentID: "agent-hidden", Message: "hello"}, nil); err == nil {
		t.Fatal("expected direct chat with a hidden agent to fail")
	}
	if _, err := svc.Agent().CreateSession(ctx, domain.CreateAgentSessionInput{AgentID: "agent-hidden", Title: "hidden"}); err == nil {
		t.Fatal("expected creating a session for a hidden agent to fail")
	}
}

func TestAgentDefinitionScopedVisibilityRequiresExistingTargets(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentAdminAccount(t, store, now)
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, CredentialCipher: newTestCredentialCipher(t)})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}
	model, err := svc.Agent().CreateModel(ctx, domain.CreateAgentModelInput{Name: "Model", ModelName: "gpt-4.1", LiteLLMModel: "openai/gpt-4.1", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Agent().CreateDefinition(ctx, domain.CreateAgentDefinitionInput{Name: "No target", ModelID: model.ID, Visibility: string(domain.AgentVisibilityDepartment)}); err == nil {
		t.Fatal("expected scoped visibility without targets to fail")
	}
	if _, err := svc.Agent().CreateDefinition(ctx, domain.CreateAgentDefinitionInput{Name: "Missing target", ModelID: model.ID, Visibility: string(domain.AgentVisibilityDepartment), VisibilityTargets: []string{"ou-missing"}}); err == nil {
		t.Fatal("expected a nonexistent visibility target to fail")
	}
}

func TestPublishedAssistantsRequireActiveAssumableRoleForRoleVisibility(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{{Resource: "agent.run", Action: "create", Scope: "all"}})
	if err := store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID: "role-reviewer", TenantID: "tenant-1", Name: "Reviewer", Trusted: true, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAssumableRoleSession(context.Background(), domain.AssumableRoleSession{
		ID: "role-session", TenantID: "tenant-1", AccountID: "acct-1", AssumableRoleID: "role-reviewer",
		ExpiresAt: time.Now().UTC().Add(time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgentDefinition(context.Background(), domain.AgentDefinition{
		ID: "agent-role", TenantID: "tenant-1", Name: "Role Agent", Category: domain.AgentCategoryWorkflow,
		Status: domain.AgentDefinitionStatusPublished, Visibility: domain.AgentVisibilityRole,
		VisibilityTargets: []string{"role-reviewer"}, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, CredentialCipher: newTestCredentialCipher(t)})
	withoutRole, err := svc.Platform().ListAssistants(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, domain.PlatformAssistantsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	for _, assistant := range withoutRole.Data {
		if assistant.ID == "agent-role" {
			t.Fatal("role-scoped assistant leaked without an active role session")
		}
	}
	withRole, err := svc.Platform().ListAssistants(domain.RequestContext{
		TenantID: "tenant-1", AccountID: "acct-1", AssumedRoleSessionID: "role-session",
	}, domain.PlatformAssistantsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(withRole.Data) != 1 || withRole.Data[0].ID != "agent-role" || !withRole.Data[0].Runnable {
		t.Fatalf("expected active role session to expose its scoped assistant, got %+v", withRole.Data)
	}
}
