package service

import (
	"encoding/json"
	"strings"
)

// agentArtifactMetadataKey 是 agent session message metadata 中承載 artifact 的穩定鍵。
// 值為 JSON 字串（JSON-in-JSON），讓動態表單欄位 ID 不被外層編碼改寫；
// 前端依此鍵與格式讀取，鍵名與編碼方式皆屬線上契約，不可變更。
const agentArtifactMetadataKey = "agent_artifact_json"

// encodeAgentArtifactMetadata 將 payload 序列化為不透明 JSON 字串，封入 message metadata。
func encodeAgentArtifactMetadata(payload map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return map[string]any{agentArtifactMetadataKey: string(raw)}, nil
}

// decodeAgentArtifactMetadata 從 message metadata 取回並解析 artifact payload；
// 鍵缺失、非字串或 JSON 解析失敗皆回傳 false。
func decodeAgentArtifactMetadata(metadata map[string]any) (map[string]any, bool) {
	raw, ok := metadata[agentArtifactMetadataKey].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, false
	}
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, false
	}
	return payload, true
}
