package domain

import "testing"

func TestAgentToolCatalogHasCompletePresentationMetadata(t *testing.T) {
	catalog := AgentToolCatalog()
	if len(catalog) != 24 {
		t.Fatalf("expected 24 agent tools, got %d", len(catalog))
	}

	seen := make(map[string]struct{}, len(catalog))
	for _, tool := range catalog {
		if _, exists := seen[tool.Value]; exists {
			t.Errorf("duplicate agent tool id %q", tool.Value)
		}
		seen[tool.Value] = struct{}{}
		if tool.Label == "" || tool.Description == "" || tool.DescriptionZhTW == "" || tool.Category == "" {
			t.Errorf("agent tool %q has incomplete presentation metadata: %+v", tool.Value, tool)
		}
		if tool.RequiredPermission != "agent.tool.call:"+tool.Value {
			t.Errorf("agent tool %q has mismatched permission %q", tool.Value, tool.RequiredPermission)
		}
	}
}
