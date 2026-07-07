package domain

import "time"

// Reference 定義 reference 的資料結構。
type Reference struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	Source  string `json:"source,omitempty"`
}

// AgentRunStatus 表示 agent 執行狀態。
type AgentRunStatus string

// 下列常數定義此模組使用的固定值。
const (
	AgentRunStatusQueued    AgentRunStatus = "queued"
	AgentRunStatusRunning   AgentRunStatus = "running"
	AgentRunStatusCompleted AgentRunStatus = "completed"
	AgentRunStatusFailed    AgentRunStatus = "failed"
)

// AgentRun 定義 agent 執行的資料結構。
type AgentRun struct {
	ID            string        `json:"id"`
	TenantID      string        `json:"tenant_id"`
	AccountID     string        `json:"account_id"`
	Mode          string        `json:"mode"`
	Prompt        string        `json:"prompt"`
	Answer        string        `json:"answer,omitempty"`
	Status        string        `json:"status"`
	References    []Reference   `json:"references,omitempty"`
	ToolDecisions []CheckResult `json:"tool_decisions,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// CreateAgentRunInput 定義 agent 執行輸入的資料結構。
type CreateAgentRunInput struct {
	Mode   string `json:"mode,omitempty"`
	Prompt string `json:"prompt"`
}
