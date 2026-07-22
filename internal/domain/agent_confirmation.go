package domain

import "time"

// AgentConfirmationItem 描述需要使用者逐筆確認的單據摘要。
type AgentConfirmationItem struct {
	ID       string             `json:"id"`
	Title    string             `json:"title"`
	Subtitle string             `json:"subtitle,omitempty"`
	Status   string             `json:"status,omitempty"`
	Rows     []AgentAnalysisRow `json:"rows,omitempty"`
}

// AgentConfirmation 描述 Agent 已準備完成、但尚未執行的高影響操作。
type AgentConfirmation struct {
	ID          string                  `json:"id"`
	Kind        string                  `json:"kind"`
	Title       string                  `json:"title"`
	Description string                  `json:"description,omitempty"`
	Action      string                  `json:"action"`
	ActionLabel string                  `json:"action_label"`
	Rows        []AgentAnalysisRow      `json:"rows,omitempty"`
	Items       []AgentConfirmationItem `json:"items,omitempty"`
	ExpiresAt   time.Time               `json:"expires_at"`
}

// ExecuteAgentConfirmationInput 定義確認操作輸入。
type ExecuteAgentConfirmationInput struct{}

// CreateExternalToolConfirmationInput binds a mutating external-tool preview
// to the exact discovered capability contract that the model observed.
type CreateExternalToolConfirmationInput struct {
	ConnectionID   string         `json:"connection_id"`
	CapabilityID   string         `json:"capability_id"`
	SchemaChecksum string         `json:"schema_checksum"`
	Arguments      map[string]any `json:"arguments"`
}

// AgentConfirmationExecution 回傳一次性確認操作的執行結果。
type AgentConfirmationExecution struct {
	ConfirmationID string                   `json:"confirmation_id"`
	Kind           string                   `json:"kind"`
	Status         string                   `json:"status"`
	FormInstance   *FormInstance            `json:"form_instance,omitempty"`
	BulkReview     *BulkReviewFormsResponse `json:"bulk_review,omitempty"`
	Data           map[string]any           `json:"data,omitempty"`
}
