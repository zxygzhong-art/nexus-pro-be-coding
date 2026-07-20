package service_test

import (
	"encoding/json"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
	agentservice "nexus-pro-api/internal/service/agent"
)

// TestAgentDefinitionResponsesUseNonNullCollections covers every public read and mutation response shape.
func TestAgentDefinitionResponsesUseNonNullCollections(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentAdminAccount(t, store, now)
	svc := service.New(store, service.Options{
		Now:              func() time.Time { return now },
		CredentialCipher: newTestCredentialCipher(t),
	})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}

	model, err := agentservice.New(svc).CreateModel(ctx, domain.CreateAgentModelInput{
		Name: "Contract Model", ModelName: "gpt-4.1", APIKey: "sk-test",
	})
	if err != nil {
		t.Fatal(err)
	}

	created, err := agentservice.New(svc).CreateDefinition(ctx, domain.CreateAgentDefinitionInput{
		Name:    "Contract Agent",
		ModelID: model.ID,
		SubAgents: []domain.AgentTeamMember{{
			Name: "Contract Worker", Role: "Verify response collections",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertAgentDefinitionResponseArrays(t, "create", created)

	description := "Updated contract fixture"
	updated, err := agentservice.New(svc).UpdateDefinition(ctx, created.ID, domain.UpdateAgentDefinitionInput{Description: &description})
	if err != nil {
		t.Fatal(err)
	}
	assertAgentDefinitionResponseArrays(t, "update", updated)

	published, err := agentservice.New(svc).PublishDefinition(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertAgentDefinitionResponseArrays(t, "publish", published)

	unpublished, err := agentservice.New(svc).UnpublishDefinition(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertAgentDefinitionResponseArrays(t, "unpublish", unpublished)

	rolledBack, err := agentservice.New(svc).RollbackDefinition(ctx, created.ID, domain.RollbackAgentDefinitionInput{Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	assertAgentDefinitionResponseArrays(t, "rollback", rolledBack)

	got, err := agentservice.New(svc).GetDefinition(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertAgentDefinitionResponseArrays(t, "get", got)

	listed, err := agentservice.New(svc).ListDefinitions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected one Agent in list, got %d", len(listed))
	}
	assertAgentDefinitionResponseArrays(t, "list", listed[0])
}

// assertAgentDefinitionResponseArrays verifies wire-required collections and nested version members serialize as arrays.
func assertAgentDefinitionResponseArrays(t *testing.T, operation string, agent domain.AgentDefinition) {
	t.Helper()
	encoded, err := json.Marshal(agent)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	assertJSONArrays(t, operation, payload, []string{
		"sub_agents",
		"suggested_questions",
		"suggested_question_translations",
		"tools",
		"knowledge_base_ids",
		"visibility_targets",
	})
	assertAgentTeamMemberResponseArrays(t, operation+".sub_agents", payload["sub_agents"])
	if versions, exists := payload["versions"]; exists {
		versionItems, ok := versions.([]any)
		if !ok {
			t.Fatalf("%s versions must serialize as an array: %s", operation, encoded)
		}
		for index, item := range versionItems {
			version, ok := item.(map[string]any)
			if !ok {
				t.Fatalf("%s.versions[%d] must be an object: %s", operation, index, encoded)
			}
			path := operation + ".versions"
			assertJSONArrays(t, path, version, []string{
				"sub_agents",
				"suggested_questions",
				"suggested_question_translations",
				"tools",
				"knowledge_base_ids",
			})
			assertAgentTeamMemberResponseArrays(t, path+".sub_agents", version["sub_agents"])
		}
	}
}

// assertAgentTeamMemberResponseArrays verifies nested Agent tool and knowledge collections never serialize as null.
func assertAgentTeamMemberResponseArrays(t *testing.T, path string, value any) {
	t.Helper()
	members, ok := value.([]any)
	if !ok {
		t.Fatalf("%s must be an array, got %#v", path, value)
	}
	for index, item := range members {
		member, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("%s[%d] must be an object, got %#v", path, index, item)
		}
		assertJSONArrays(t, path, member, []string{"tools", "knowledge_base_ids"})
	}
}

// assertJSONArrays checks that required JSON fields are present and array-valued.
func assertJSONArrays(t *testing.T, path string, payload map[string]any, fields []string) {
	t.Helper()
	for _, field := range fields {
		if _, ok := payload[field].([]any); !ok {
			t.Fatalf("%s.%s must serialize as an array, got %#v", path, field, payload[field])
		}
	}
}
