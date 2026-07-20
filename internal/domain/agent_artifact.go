package domain

import (
	"encoding/json"
	"strings"
)

// AgentArtifactMetadataKey is the stable agent session message metadata key.
// The value remains JSON-in-JSON so dynamic form field IDs are not rewritten.
const AgentArtifactMetadataKey = "agent_artifact_json"

// EncodeAgentArtifactMetadata serializes an artifact payload into opaque message metadata.
func EncodeAgentArtifactMetadata(payload map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return map[string]any{AgentArtifactMetadataKey: string(raw)}, nil
}

// DecodeAgentArtifactMetadata restores an artifact payload from message metadata.
func DecodeAgentArtifactMetadata(metadata map[string]any) (map[string]any, bool) {
	raw, ok := metadata[AgentArtifactMetadataKey].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, false
	}
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, false
	}
	return payload, true
}
