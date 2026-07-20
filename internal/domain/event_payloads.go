package domain

import (
	"encoding/json"
	"fmt"
)

// OpenFGARelationshipPayload 定義 openfga.relationship.* outbox 事件的 wire payload。
// JSON 鍵必須與生產端寫入 outbox_events.payload 的鍵保持一致,以免破壞線上已有事件。
type OpenFGARelationshipPayload struct {
	Operation   string `json:"operation,omitempty"`
	ObjectType  string `json:"object_type"`
	ObjectID    string `json:"object_id"`
	Relation    string `json:"relation"`
	SubjectType string `json:"subject_type"`
	SubjectID   string `json:"subject_id"`
}

// AgentModelSyncPayload 定義 agent.model.* outbox 事件的 wire payload。
type AgentModelSyncPayload struct {
	ModelID string `json:"model_id"`
}

// Map 將 typed payload 經 JSON round-trip 轉回 wire map,wire 格式維持不變。
func (p OpenFGARelationshipPayload) Map() (map[string]any, error) {
	return encodeEventPayload(p)
}

// Map 將 typed payload 經 JSON round-trip 轉回 wire map,wire 格式維持不變。
func (p AgentModelSyncPayload) Map() (map[string]any, error) {
	return encodeEventPayload(p)
}

// DecodeOpenFGARelationshipPayload 將 wire map 解析為 typed payload。
// 鍵值型別不符時回傳明確錯誤,而不是靜默還原為空字串。
func DecodeOpenFGARelationshipPayload(payload map[string]any) (OpenFGARelationshipPayload, error) {
	var out OpenFGARelationshipPayload
	if err := decodeEventPayload(payload, &out); err != nil {
		return OpenFGARelationshipPayload{}, fmt.Errorf("decode openfga relationship payload: %w", err)
	}
	return out, nil
}

// DecodeAgentModelSyncPayload 將 wire map 解析為 typed payload。
// 鍵值型別不符時回傳明確錯誤,而不是靜默還原為空字串。
func DecodeAgentModelSyncPayload(payload map[string]any) (AgentModelSyncPayload, error) {
	var out AgentModelSyncPayload
	if err := decodeEventPayload(payload, &out); err != nil {
		return AgentModelSyncPayload{}, fmt.Errorf("decode agent model sync payload: %w", err)
	}
	return out, nil
}

// encodeEventPayload 經 JSON round-trip 將 typed payload 轉為 wire map。
func encodeEventPayload(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// decodeEventPayload 經 JSON round-trip 將 wire map 解析為 typed payload。
// nil payload 解析為零值,必填欄位檢查由消費端負責。
func decodeEventPayload(payload map[string]any, out any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}
