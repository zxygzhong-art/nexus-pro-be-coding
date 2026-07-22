package agent

import (
	"context"
	"regexp"
	"testing"
)

func TestADKToolsUseDynamicExternalContract(t *testing.T) {
	name := externalToolRuntimeName("exttool-ticket-lookup")
	tools, err := adkTools(map[string]AgentTool{
		name: func(context.Context, map[string]any) (map[string]any, error) { return nil, nil },
	}, map[string]AgentToolSpec{
		name: {
			Description: "Read one support ticket without mutation.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"ticket_id"},
				"properties": map[string]any{
					"ticket_id": map[string]any{"type": "string", "description": "Stable ticket identifier."},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].Name() != name || tools[0].Description() != "Read one support ticket without mutation." {
		t.Fatalf("dynamic tool contract was not forwarded to ADK: %+v", tools)
	}

	schema, err := dynamicAgentToolInputSchema(map[string]any{
		"type":     "object",
		"required": []any{"ticket_id"},
		"properties": map[string]any{
			"ticket_id": map[string]any{"type": "string", "description": "Stable ticket identifier."},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "ticket_id" || schema.Properties["ticket_id"].Description != "Stable ticket identifier." {
		t.Fatalf("dynamic JSON schema lost constraints: %+v", schema)
	}
	if _, err := dynamicAgentToolInputSchema(map[string]any{"type": "array"}); err == nil {
		t.Fatal("non-object tool input schema should be rejected")
	}
}

func TestExternalToolRuntimeNamesAreCollisionResistantAndADKSafe(t *testing.T) {
	validName := regexp.MustCompile(`^[a-z_][a-z0-9_]{0,63}$`)
	first := externalToolRuntimeName("capability-1")
	second := externalToolRuntimeName("capability-2")
	if first == second || !validName.MatchString(first) || !validName.MatchString(second) {
		t.Fatalf("runtime names must be unique and ADK-safe: %q %q", first, second)
	}
}
