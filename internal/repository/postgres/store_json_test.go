package postgres

import (
	"encoding/json"
	"testing"

	"nexus-pro-api/internal/domain"
)

func TestCollectionJSONEncodesNilSliceAsArray(t *testing.T) {
	if got := string(collectionJSON([]string(nil))); got != "[]" {
		t.Fatalf("collectionJSON(nil) = %s, want []", got)
	}
}

func TestAgentTeamMembersJSONEncodesNilCollectionsAsArrays(t *testing.T) {
	encoded := agentTeamMembersJSON([]domain.AgentTeamMember{{
		ID: "member-1", Name: "Member", Role: "Role", ModelID: "model-1",
	}})
	var decoded []map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 1 {
		t.Fatalf("decoded members = %d, want 1", len(decoded))
	}
	for _, field := range []string{"tools", "external_tool_ids", "knowledge_base_ids"} {
		value, ok := decoded[0][field].([]any)
		if !ok || len(value) != 0 {
			t.Fatalf("%s = %#v, want empty array", field, decoded[0][field])
		}
	}
}
