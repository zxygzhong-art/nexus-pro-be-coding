package db_test

import (
	"os"
	"strings"
	"testing"
)

// TestAgentModelPersistenceSeparatesUpstreamFromAlias keeps the provider model
// name in model_connections while deriving the stable LiteLLM route from the ID.
func TestAgentModelPersistenceSeparatesUpstreamFromAlias(t *testing.T) {
	raw, err := os.ReadFile("../../../db/queries/agent_admin.sql")
	if err != nil {
		t.Fatal(err)
	}
	queries := string(raw)

	if strings.Contains(queries, "sqlc.arg(litellm_model)") {
		t.Fatal("model_connections.upstream_model must not be populated from the LiteLLM alias")
	}
	if !strings.Contains(queries, "sqlc.arg(model_name),\n        sqlc.arg(api_base_url)") {
		t.Fatal("model_connections.upstream_model must be populated from model_name")
	}
	if strings.Contains(queries, "upstream_model AS litellm_model") {
		t.Fatal("LiteLLM aliases must not expose the upstream provider model")
	}
	if got := strings.Count(queries, "'nexus-agent-model-' ||"); got != 6 {
		t.Fatalf("expected all six Agent model projections to derive the stable alias, got %d", got)
	}
}
