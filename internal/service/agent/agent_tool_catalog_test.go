package agent

import (
	"context"
	"testing"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

// TestAgentToolCatalogMatchesRuntimeRegistry prevents admin-only catalog entries
// from being presented as usable when the chat Runtime has no matching handler.
func TestAgentToolCatalogMatchesRuntimeRegistry(t *testing.T) {
	svc := New(service.New(memory.NewStore(), service.Options{}))
	runtimeTools := svc.agentTools(domain.RequestContext{}, func(context.Context, domain.AgentChatEvent) error {
		return nil
	}, nil)
	catalog := domain.AgentToolCatalog()

	if len(catalog) != 24 {
		t.Fatalf("expected 24 catalog tools, got %d", len(catalog))
	}
	if len(runtimeTools) != len(catalog) {
		t.Fatalf("catalog/runtime count mismatch: catalog=%d runtime=%d", len(catalog), len(runtimeTools))
	}

	for _, meta := range catalog {
		if meta.Category == "" || meta.DescriptionZhTW == "" {
			t.Errorf("tool %q is missing category or zh-TW description: %+v", meta.Value, meta)
		}
		if runtimeTools[meta.Value] == nil {
			t.Errorf("catalog tool %q has no Runtime handler", meta.Value)
		}
		if agentToolDescription(meta.Value) == "Nexus Pro agent tool: "+meta.Value {
			t.Errorf("catalog tool %q has no model-facing description", meta.Value)
		}
		if agentToolInputSchema(meta.Value) == nil {
			t.Errorf("catalog tool %q has no input schema", meta.Value)
		}
	}
}
