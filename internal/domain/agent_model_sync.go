package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const (
	// OutboxAggregateAgentModel 標記需同步到 LiteLLM 的模型事件。
	OutboxAggregateAgentModel = "agent_model"
	// EventAgentModelUpsert 建立或更新 LiteLLM 模型路由。
	EventAgentModelUpsert EventType = "agent.model.upsert"
	// EventAgentModelDelete 刪除 LiteLLM 模型路由。
	EventAgentModelDelete EventType = "agent.model.delete"
)

// AgentModelLiteLLMAlias 以不可變的本地模型 ID 產生穩定 LiteLLM 路由名。
func AgentModelLiteLLMAlias(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	return "nexus-agent-model-" + id
}

// AgentModelSyncConfigHash 計算會影響 LiteLLM 路由的設定摘要。
func AgentModelSyncConfigHash(model AgentModel) string {
	payload, _ := json.Marshal(struct {
		Provider       string           `json:"provider"`
		ModelName      string           `json:"model_name"`
		LiteLLMModel   string           `json:"litellm_model"`
		APIBaseURL     string           `json:"api_base_url"`
		APIKey         string           `json:"api_key"`
		RateLimitRPM   int              `json:"rate_limit_rpm"`
		TimeoutSeconds int              `json:"timeout_seconds"`
		Status         AgentModelStatus `json:"status"`
	}{
		Provider:       model.Provider,
		ModelName:      model.ModelName,
		LiteLLMModel:   model.LiteLLMModel,
		APIBaseURL:     model.APIBaseURL,
		APIKey:         model.APIKey,
		RateLimitRPM:   model.RateLimitRPM,
		TimeoutSeconds: model.TimeoutSeconds,
		Status:         model.Status,
	})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
