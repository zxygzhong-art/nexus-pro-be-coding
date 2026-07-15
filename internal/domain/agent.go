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
	AgentID       string        `json:"agent_id,omitempty"`
	SessionID     string        `json:"session_id,omitempty"`
	Mode          string        `json:"mode"`
	Prompt        string        `json:"prompt"`
	Answer        string        `json:"answer,omitempty"`
	Status        string        `json:"status"`
	References    []Reference   `json:"references,omitempty"`
	ToolDecisions []CheckResult `json:"tool_decisions,omitempty"`
	LLMCallCount  int64         `json:"-"`
	InputTokens   int64         `json:"-"`
	CachedTokens  int64         `json:"-"`
	OutputTokens  int64         `json:"-"`
	TotalTokens   int64         `json:"-"`
	UsageComplete bool          `json:"-"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// AgentTokenUsage records one model response usage report.
type AgentTokenUsage struct {
	InputTokens  int64
	CachedTokens int64
	OutputTokens int64
	TotalTokens  int64
}

// CreateAgentRunInput 定義 agent 執行輸入的資料結構。
type CreateAgentRunInput struct {
	Mode    string `json:"mode,omitempty"`
	Prompt  string `json:"prompt"`
	AgentID string `json:"agent_id,omitempty"`
}

// AgentChatInput 定義流式 agent chat 請求。
type AgentChatInput struct {
	SessionID     string   `json:"session_id,omitempty"`
	Message       string   `json:"message"`
	Mode          string   `json:"mode,omitempty"`
	AgentID       string   `json:"agent_id,omitempty"`
	AttachmentIDs []string `json:"attachment_ids,omitempty"`
}

// AgentChatEventName 表示 SSE agent chat 事件名稱。
type AgentChatEventName string

// 下列常數定義 agent chat SSE 事件。
const (
	AgentChatEventSession        AgentChatEventName = "session"
	AgentChatEventMessageDelta   AgentChatEventName = "message_delta"
	AgentChatEventToolCall       AgentChatEventName = "tool_call"
	AgentChatEventToolResult     AgentChatEventName = "tool_result"
	AgentChatEventAnalysisResult AgentChatEventName = "analysis_result"
	AgentChatEventConfirmation   AgentChatEventName = "confirmation_required"
	AgentChatEventDone           AgentChatEventName = "done"
	AgentChatEventError          AgentChatEventName = "error"
)

// AgentAnalysisRow 定義分析卡片 row。
type AgentAnalysisRow struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// AgentChatEvent 定義流式 agent chat 事件資料。
type AgentChatEvent struct {
	Event        AgentChatEventName `json:"-"`
	AgentName    string             `json:"agent_name,omitempty"`
	AgentBranch  string             `json:"agent_branch,omitempty"`
	SessionID    string             `json:"session_id,omitempty"`
	RunID        string             `json:"run_id,omitempty"`
	Delta        string             `json:"delta,omitempty"`
	Name         string             `json:"name,omitempty"`
	Status       string             `json:"status,omitempty"`
	Title        string             `json:"title,omitempty"`
	Rows         []AgentAnalysisRow `json:"rows,omitempty"`
	Message      string             `json:"message,omitempty"`
	Data         map[string]any     `json:"data,omitempty"`
	Confirmation *AgentConfirmation `json:"confirmation,omitempty"`
}
