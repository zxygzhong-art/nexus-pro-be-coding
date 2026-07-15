package service_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	platformsecret "nexus-pro-be/internal/platform/secret"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func newTestCredentialCipher(t *testing.T) service.CredentialCipher {
	t.Helper()
	cipher, err := platformsecret.NewAESGCMCipher(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	if err != nil {
		t.Fatal(err)
	}
	return cipher
}

func TestAgentAdminCreatesPublishesTrialsAndRollsBackAgent(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentAdminAccount(t, store, now)
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, CredentialCipher: newTestCredentialCipher(t)})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}

	model, err := svc.Agent().CreateModel(ctx, domain.CreateAgentModelInput{
		Name:         "GPT 4.1",
		Provider:     "openai",
		ModelName:    "gpt-4.1",
		LiteLLMModel: "openai/gpt-4.1",
		APIKey:       "sk-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if model.Status != domain.AgentModelStatusActive || model.TimeoutSeconds <= 0 {
		t.Fatalf("unexpected model defaults: %+v", model)
	}

	agent, err := svc.Agent().CreateDefinition(ctx, domain.CreateAgentDefinitionInput{
		Name:               "HR Helper",
		Description:        "Answers HR questions",
		ModelID:            model.ID,
		SystemPrompt:       "You are an HR helper.",
		WelcomeMessage:     "Welcome v1",
		SuggestedQuestions: []string{"Question v1"},
		Tools:              []string{"get_my_profile"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if agent.Status != domain.AgentDefinitionStatusDraft || agent.Version != 1 {
		t.Fatalf("unexpected draft agent: %+v", agent)
	}

	updatedPrompt := "You are a careful HR helper."
	updatedWelcome := "Welcome v2"
	agent, err = svc.Agent().UpdateDefinition(ctx, agent.ID, domain.UpdateAgentDefinitionInput{
		SystemPrompt:       &updatedPrompt,
		WelcomeMessage:     &updatedWelcome,
		SuggestedQuestions: []string{"Question v2"},
		VersionNote:        "careful prompt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if agent.Status != domain.AgentDefinitionStatusDraft || agent.Version != 2 {
		t.Fatalf("expected draft version 2 after config update, got %+v", agent)
	}
	agent, err = svc.Agent().PublishDefinition(ctx, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Status != domain.AgentDefinitionStatusPublished || agent.Version != 2 {
		t.Fatalf("expected published agent without version bump, got %+v", agent)
	}
	listed, err := svc.Agent().ListDefinitions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || len(listed[0].Versions) != 2 {
		t.Fatalf("expected list response to include stored version history, got %+v", listed)
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
	if rolledBack.Version != 3 || rolledBack.SystemPrompt != "You are an HR helper." || rolledBack.WelcomeMessage != "Welcome v1" || len(rolledBack.SuggestedQuestions) != 1 || rolledBack.SuggestedQuestions[0] != "Question v1" {
		t.Fatalf("expected rollback to create version 3 from v1, got %+v", rolledBack)
	}
	unpublished, err := svc.Agent().UnpublishDefinition(ctx, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unpublished.Status != domain.AgentDefinitionStatusDraft || unpublished.Version != 3 {
		t.Fatalf("expected unpublish to keep version and return draft, got %+v", unpublished)
	}
	deleted, err := svc.Agent().DeleteDefinition(ctx, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.ID != agent.ID {
		t.Fatalf("expected unpublished agent to be deleted, got %+v", deleted)
	}
	if _, err := svc.Agent().DeleteModel(ctx, model.ID); err != nil {
		t.Fatalf("expected unused model to be deleted with its audit: %v", err)
	}
}

func TestAgentAdminTrialReportsOnlyActuallyUsedTools(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentAdminAccount(t, store, now)
	var expectedModelAlias string
	runtime := agentAdminFakeRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		if req.ModelName != expectedModelAlias || !strings.Contains(req.Message, "You are an HR helper.") {
			t.Fatalf("trial did not apply its configured model and system prompt: %+v", req)
		}
		tool, ok := req.Tools["get_my_profile"]
		if !ok {
			t.Fatalf("expected get_my_profile to be available in trial tools: %+v", req.Tools)
		}
		if _, ok := req.Tools["workspace_insights"]; ok {
			t.Fatalf("trial exposed a tool not configured on the agent: %+v", req.Tools)
		}
		if _, ok := req.Tools["create_form_draft"]; ok {
			t.Fatalf("trial exposed a write tool: %+v", req.Tools)
		}
		_ = tool
		if err := emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventToolResult, Name: "get_my_profile", Status: "ok"}); err != nil {
			return err
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "profile checked"})
	}}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime, CredentialCipher: newTestCredentialCipher(t)})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}

	model, err := svc.Agent().CreateModel(ctx, domain.CreateAgentModelInput{
		Name:         "GPT 4.1",
		ModelName:    "gpt-4.1",
		LiteLLMModel: "openai/gpt-4.1",
		APIKey:       "sk-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	expectedModelAlias = domain.AgentModelLiteLLMAlias(model.ID)
	agent, err := svc.Agent().CreateDefinition(ctx, domain.CreateAgentDefinitionInput{
		Name:         "HR Helper",
		ModelID:      model.ID,
		SystemPrompt: "You are an HR helper.",
		Tools:        []string{"get_my_profile", "my_leave_balances", "create_form_draft"},
	})
	if err != nil {
		t.Fatal(err)
	}
	agent, err = svc.Agent().PublishDefinition(ctx, agent.ID)
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

func TestAgentExternalToolRegistryLifecycle(t *testing.T) {
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentAdminAccount(t, store, now)
	credentialCipher, err := platformsecret.NewAESGCMCipher(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, CredentialCipher: credentialCipher})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}

	created, err := svc.Agent().CreateExternalTool(ctx, domain.CreateAgentExternalToolInput{
		Name:           "Support MCP",
		Description:    "Customer support knowledge service",
		Kind:           "mcp",
		Transport:      "sse",
		EndpointURL:    "https://tools.example.com/mcp#ignored",
		AuthType:       "api_key",
		AuthHeaderName: "X-Support-Key",
		AuthSecret:     "support-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.EndpointURL != "https://tools.example.com/mcp" || created.CreatedByAccountID != "acct-admin" {
		t.Fatalf("unexpected external tool: %+v", created)
	}
	if created.Transport != "sse" || created.AuthType != "api_key" || created.AuthHeaderName != "X-Support-Key" || !created.CredentialSet {
		t.Fatalf("expected protocol and credential metadata, got %+v", created)
	}
	if created.AuthSecretCiphertext == "" || strings.Contains(created.AuthSecretCiphertext, "support-secret") {
		t.Fatalf("expected encrypted credential, got %q", created.AuthSecretCiphertext)
	}
	plaintext, err := credentialCipher.Decrypt(created.AuthSecretCiphertext, []byte("tenant-1\x00"+created.ID))
	if err != nil || string(plaintext) != "support-secret" {
		t.Fatalf("expected decryptable tenant-bound credential, plaintext=%q err=%v", plaintext, err)
	}
	encoded, err := json.Marshal(created)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "support-secret") || strings.Contains(string(encoded), "auth_secret_ciphertext") {
		t.Fatalf("credential leaked in external tool JSON: %s", encoded)
	}

	items, err := svc.Agent().ListExternalTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("expected created external tool in list, got %+v", items)
	}

	deleted, err := svc.Agent().DeleteExternalTool(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.ID != created.ID {
		t.Fatalf("expected deleted external tool, got %+v", deleted)
	}
	items, err = svc.Agent().ListExternalTools(ctx)
	if err != nil || len(items) != 0 {
		t.Fatalf("expected empty external tool registry after deletion, items=%+v err=%v", items, err)
	}

	if _, err := svc.Agent().CreateExternalTool(ctx, domain.CreateAgentExternalToolInput{
		Name: "Unsafe", Kind: "http", EndpointURL: "https://user:secret@tools.example.com/api",
	}); err == nil {
		t.Fatal("expected embedded endpoint credentials to be rejected")
	}
	if _, err := service.New(store).Agent().CreateExternalTool(ctx, domain.CreateAgentExternalToolInput{
		Name: "Missing cipher", Kind: "http", EndpointURL: "https://tools.example.com/api", AuthType: "bearer", AuthSecret: "secret",
	}); err == nil {
		t.Fatal("expected authenticated tool creation to fail closed without a credential cipher")
	}
}

// TestAgentTeamPublishSnapshotStaysStable 驗證自動更新不會越過已發佈版本影響線上 Team。
func TestAgentTeamPublishSnapshotStaysStable(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentAdminAccount(t, store, now)
	var requests []service.AgentChatRuntimeRequest
	runtime := agentAdminFakeRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		requests = append(requests, req)
		if len(req.SubAgents) > 0 {
			if err := emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, AgentName: req.SubAgents[0].Name, Delta: "child details"}); err != nil {
				return err
			}
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, AgentName: req.AgentName, Delta: "root summary"})
	}}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime, CredentialCipher: newTestCredentialCipher(t)})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}

	rootModel, err := svc.Agent().CreateModel(ctx, domain.CreateAgentModelInput{Name: "Root", ModelName: "root-model", APIKey: "sk-root"})
	if err != nil {
		t.Fatal(err)
	}
	childModel, err := svc.Agent().CreateModel(ctx, domain.CreateAgentModelInput{Name: "Child", ModelName: "child-model", APIKey: "sk-child"})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := svc.Agent().CreateDefinition(ctx, domain.CreateAgentDefinitionInput{
		Name:          "HR Team",
		ModelID:       rootModel.ID,
		MainAgentRole: "route old configuration",
		SubAgents: []domain.AgentTeamMember{{
			ID: "policy", Name: "Policy Agent", Role: "old policy role", ModelID: childModel.ID, Tools: []string{"get_my_profile"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	agent, err = svc.Agent().PublishDefinition(ctx, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if agent.PublishedVersion != 1 {
		t.Fatalf("expected v1 deployment snapshot, got %+v", agent)
	}

	newRole := "route new configuration"
	agent, err = svc.Agent().UpdateDefinition(ctx, agent.ID, domain.UpdateAgentDefinitionInput{
		MainAgentRole: &newRole,
		SubAgents: []domain.AgentTeamMember{{
			ID: "policy", Name: "Policy Agent", Role: "new policy role", ModelID: rootModel.ID, Tools: []string{"get_my_profile"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if agent.Version != 2 || agent.PublishedVersion != 1 {
		t.Fatalf("expected draft v2 while deployment remains v1, got %+v", agent)
	}
	trial, err := svc.Agent().Trial(ctx, agent.ID, domain.AgentTrialInput{Message: "check policy"})
	if err != nil {
		t.Fatal(err)
	}
	if trial.Reply != "root summary" || len(requests) != 1 || requests[0].AgentRole != "route old configuration" {
		t.Fatalf("trial did not use published v1 root snapshot: trial=%+v requests=%+v", trial, requests)
	}
	if len(requests[0].SubAgents) != 1 || requests[0].SubAgents[0].Role != "old policy role" || requests[0].SubAgents[0].ModelName != domain.AgentModelLiteLLMAlias(childModel.ID) {
		t.Fatalf("trial did not use published v1 child snapshot: %+v", requests[0].SubAgents)
	}
	if strings.Join(trial.AgentsUsed, ",") != "Policy Agent,HR Team" {
		t.Fatalf("expected observable executing agents, got %+v", trial.AgentsUsed)
	}

	if _, err = svc.Agent().UnpublishDefinition(ctx, agent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Agent().PublishDefinition(ctx, agent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Agent().Trial(ctx, agent.ID, domain.AgentTrialInput{Message: "check policy again"}); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || requests[1].AgentRole != newRole || requests[1].SubAgents[0].Role != "new policy role" {
		t.Fatalf("republish did not activate v2 snapshot: %+v", requests)
	}
}

func TestAgentAdminTrialRecordsRuntimeFailure(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentAdminAccount(t, store, now)
	svc := service.New(store, service.Options{
		Now:              func() time.Time { return now },
		CredentialCipher: newTestCredentialCipher(t),
		AgentChatRuntime: agentAdminFakeRuntime{run: func(context.Context, service.AgentChatRuntimeRequest, service.AgentChatEmitFunc) error {
			return errors.New("runtime unavailable")
		}},
	})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}
	model, err := svc.Agent().CreateModel(ctx, domain.CreateAgentModelInput{Name: "Model", ModelName: "gpt-4.1", LiteLLMModel: "openai/gpt-4.1", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := svc.Agent().CreateDefinition(ctx, domain.CreateAgentDefinitionInput{Name: "Failure counter", ModelID: model.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Agent().PublishDefinition(ctx, agent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Agent().Trial(ctx, agent.ID, domain.AgentTrialInput{Message: "fail"}); err == nil {
		t.Fatal("expected runtime failure")
	}
	stored, ok, err := store.GetAgentDefinition(context.Background(), "tenant-1", agent.ID)
	if err != nil || !ok {
		t.Fatalf("expected stored agent, ok=%v err=%v", ok, err)
	}
	if stored.Usage.TotalRuns != 1 || stored.Usage.FailedRuns != 1 || stored.Usage.SuccessRuns != 0 {
		t.Fatalf("expected failed trial usage to be recorded, got %+v", stored.Usage)
	}
}

func TestAgentAdminBlocksDeletingModelInUse(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentAdminAccount(t, store, now)
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, CredentialCipher: newTestCredentialCipher(t)})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}

	model, err := svc.Agent().CreateModel(ctx, domain.CreateAgentModelInput{Name: "Default", ModelName: "gpt-4.1", LiteLLMModel: "openai/gpt-4.1", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Agent().CreateDefinition(ctx, domain.CreateAgentDefinitionInput{Name: "Agent", ModelID: model.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Agent().DeleteModel(ctx, model.ID); err == nil {
		t.Fatal("expected deleting a model used by an agent to be blocked")
	} else if appErr, ok := domain.AsAppError(err); !ok || appErr.NumericCode() != domain.ErrorCodeAgentModelInUse || appErr.ReasonCode != "agent_model_in_use" {
		t.Fatalf("expected agent_model_in_use contract, got %#v", err)
	}
}

func TestAgentAdminBlocksDeletingPublishedAgent(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentAdminAccount(t, store, now)
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, CredentialCipher: newTestCredentialCipher(t)})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}
	model, err := svc.Agent().CreateModel(ctx, domain.CreateAgentModelInput{Name: "Default", ModelName: "gpt-4.1", LiteLLMModel: "openai/gpt-4.1", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := svc.Agent().CreateDefinition(ctx, domain.CreateAgentDefinitionInput{Name: "Published", ModelID: model.ID})
	if err != nil {
		t.Fatal(err)
	}
	agent, err = svc.Agent().PublishDefinition(ctx, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Agent().DeleteDefinition(ctx, agent.ID); err == nil {
		t.Fatal("expected published agent deletion to require unpublish first")
	} else if appErr, ok := domain.AsAppError(err); !ok || appErr.NumericCode() != domain.ErrorCodeAgentDefinitionPublished || appErr.ReasonCode != "agent_definition_published" {
		t.Fatalf("expected agent_definition_published contract, got %#v", err)
	}
	if stored, ok, err := store.GetAgentDefinition(context.Background(), "tenant-1", agent.ID); err != nil || !ok || stored.Status != domain.AgentDefinitionStatusPublished {
		t.Fatalf("published agent changed after blocked deletion: ok=%v err=%v agent=%+v", ok, err, stored)
	}
}

func TestAgentAdminSyncAndTestModelUseLiteLLMAdmin(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentAdminAccount(t, store, now)
	liteLLM := &fakeLiteLLMAdmin{}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, LiteLLMAdmin: liteLLM, CredentialCipher: newTestCredentialCipher(t)})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}

	model, err := svc.Agent().CreateModel(ctx, domain.CreateAgentModelInput{
		Name:         "GPT 4.1",
		ModelName:    "gpt-4.1",
		LiteLLMModel: "openai/gpt-4.1",
		APIKey:       "sk-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if model.LiteLLMModel != domain.AgentModelLiteLLMAlias(model.ID) || model.SyncStatus != domain.AgentModelSyncStatusPending {
		t.Fatalf("expected stable alias and pending sync state, got %+v", model)
	}
	model, err = svc.Agent().SyncModel(ctx, model.ID)
	if err != nil {
		t.Fatal(err)
	}
	if liteLLM.synced != 1 || model.SyncStatus != domain.AgentModelSyncStatusSynced || model.LastSyncedAt == nil || model.LastTestStatus != "untested" {
		t.Fatalf("expected sync to update status and call client, model=%+v calls=%d", model, liteLLM.synced)
	}
	model, err = svc.Agent().TestModel(ctx, model.ID)
	if err != nil {
		t.Fatal(err)
	}
	if liteLLM.tested != 1 || model.LastTestStatus != "ok" || !strings.Contains(model.LastTestMessage, "responded") {
		t.Fatalf("expected test to update status and call client, model=%+v calls=%d", model, liteLLM.tested)
	}
}

func TestAgentAdminModelCredentialsNormalizeAndHideAPIKey(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentAdminAccount(t, store, now)
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, CredentialCipher: newTestCredentialCipher(t)})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}

	_, err := svc.Agent().CreateModel(ctx, domain.CreateAgentModelInput{
		Name:         "OpenRouter Gemma",
		Provider:     "custom",
		ModelName:    "openrouter/google/gemma-3-27b-it",
		LiteLLMModel: "openrouter/google/gemma-3-27b-it",
		APIKey:       "sk-openrouter-test",
	})
	if err == nil || !strings.Contains(err.Error(), "api_base_url") {
		t.Fatalf("expected custom provider to require api_base_url, got %v", err)
	}

	model, err := svc.Agent().CreateModel(ctx, domain.CreateAgentModelInput{
		Name:         "OpenRouter Gemma",
		Provider:     "custom",
		ModelName:    "openrouter/google/gemma-3-27b-it",
		LiteLLMModel: "openrouter/google/gemma-3-27b-it",
		APIBaseURL:   " https://openrouter.ai/api/v1/ ",
		APIKey:       " sk-openrouter-test ",
		RateLimitRPM: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if model.APIBaseURL != "https://openrouter.ai/api/v1" || model.APIKey != "sk-openrouter-test" || model.RateLimitRPM != 100 {
		t.Fatalf("expected normalized credential fields, got %+v", model)
	}
	if !model.APIKeySet || model.APIKeyPreview != "****test" {
		t.Fatalf("expected API key state and preview, got %+v", model)
	}
	stored, ok, err := store.GetAgentModel(context.Background(), "tenant-1", model.ID)
	if err != nil || !ok {
		t.Fatalf("expected encrypted model row, ok=%v err=%v", ok, err)
	}
	if stored.APIKey != "" || stored.APIKeyCiphertext == "" || strings.Contains(stored.APIKeyCiphertext, "sk-openrouter-test") {
		t.Fatalf("model repository leaked plaintext credential: %+v", stored)
	}
	encoded, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "sk-openrouter-test") {
		t.Fatalf("API key leaked in JSON response: %s", string(encoded))
	}

	nextName := "OpenRouter Gemma 27B"
	updated, err := svc.Agent().UpdateModel(ctx, model.ID, domain.UpdateAgentModelInput{Name: &nextName})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != nextName || updated.APIKey != "sk-openrouter-test" || !updated.APIKeySet {
		t.Fatalf("expected update without api_key to preserve secret, got %+v", updated)
	}

	blankKey := ""
	if _, err := svc.Agent().UpdateModel(ctx, model.ID, domain.UpdateAgentModelInput{APIKey: &blankKey}); err == nil {
		t.Fatal("expected blank api_key update to be rejected")
	}
	negativeRPM := -1
	if _, err := svc.Agent().UpdateModel(ctx, model.ID, domain.UpdateAgentModelInput{RateLimitRPM: &negativeRPM}); err == nil {
		t.Fatal("expected negative rate_limit_rpm update to be rejected")
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
			{Resource: "agent.tool", Action: "read", Scope: "all"},
			{Resource: "agent.tool", Action: "create", Scope: "all"},
			{Resource: "agent.tool", Action: "delete", Scope: "all"},
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

type fakeLiteLLMAdmin struct {
	synced  int
	deleted int
	tested  int
}

func (f *fakeLiteLLMAdmin) SyncModel(_ context.Context, model domain.AgentModel) (string, error) {
	f.synced++
	return "synced " + model.LiteLLMModel, nil
}

func (f *fakeLiteLLMAdmin) DeleteModel(_ context.Context, _ string) (string, error) {
	f.deleted++
	return "deleted", nil
}

// ListManagedModelIDs 回傳測試用 managed deployment。
func (f *fakeLiteLLMAdmin) ListManagedModelIDs(context.Context) ([]string, error) {
	return nil, nil
}

func (f *fakeLiteLLMAdmin) TestModel(_ context.Context, model domain.AgentModel) (string, error) {
	f.tested++
	return "responded " + model.LiteLLMModel, nil
}
