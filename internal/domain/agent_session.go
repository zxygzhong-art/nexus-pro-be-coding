package domain

import "time"

// AgentSessionStatus 表示會話狀態。
type AgentSessionStatus string

const (
	AgentSessionStatusActive   AgentSessionStatus = "active"
	AgentSessionStatusArchived AgentSessionStatus = "archived"
)

// AgentMessageRole 表示會話訊息角色。
type AgentMessageRole string

const (
	AgentMessageRoleUser      AgentMessageRole = "user"
	AgentMessageRoleAssistant AgentMessageRole = "assistant"
	AgentMessageRoleSystem    AgentMessageRole = "system"
	AgentMessageRoleTool      AgentMessageRole = "tool"
)

// AgentMemorySource 表示記憶來源。
type AgentMemorySource string

const (
	AgentMemorySourceAuto   AgentMemorySource = "auto"
	AgentMemorySourceManual AgentMemorySource = "manual"
)

// AgentSession 定義 Agent 對話會話。
type AgentSession struct {
	ID             string             `json:"id"`
	TenantID       string             `json:"tenant_id"`
	AccountID      string             `json:"account_id"`
	AgentID        string             `json:"agent_id,omitempty"`
	Title          string             `json:"title"`
	Status         AgentSessionStatus `json:"status"`
	ContextVersion int64              `json:"context_version"`
	LastMessageAt  *time.Time         `json:"last_message_at,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

// AgentSessionMessage 定義會話訊息。
type AgentSessionMessage struct {
	ID             string             `json:"id"`
	TenantID       string             `json:"tenant_id"`
	SessionID      string             `json:"session_id"`
	Role           AgentMessageRole   `json:"role"`
	Content        string             `json:"content"`
	RunID          string             `json:"run_id,omitempty"`
	ContextVersion int64              `json:"context_version"`
	Metadata       map[string]any     `json:"metadata,omitempty"`
	Attachments    []AgentSessionFile `json:"attachments,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
}

// AgentAccountUsage reports retained Agent conversation usage for one tenant account.
type AgentAccountUsage struct {
	AccountID    string     `json:"account_id"`
	DisplayName  string     `json:"display_name"`
	Email        string     `json:"email,omitempty"`
	Status       string     `json:"status"`
	SessionCount int64      `json:"session_count"`
	MessageCount int64      `json:"message_count"`
	LLMCallCount int64      `json:"llm_call_count"`
	InputTokens  int64      `json:"input_tokens"`
	CachedTokens int64      `json:"cached_tokens"`
	OutputTokens int64      `json:"output_tokens"`
	TotalTokens  int64      `json:"total_tokens"`
	ActualTokens int64      `json:"actual_tokens"`
	LastActiveAt *time.Time `json:"last_active_at,omitempty"`
}

// AgentSessionUsage reports retained conversation and token usage for one session.
type AgentSessionUsage struct {
	SessionID    string             `json:"session_id"`
	AccountID    string             `json:"account_id"`
	Title        string             `json:"title"`
	Status       AgentSessionStatus `json:"status"`
	MessageCount int64              `json:"message_count"`
	LLMCallCount int64              `json:"llm_call_count"`
	InputTokens  int64              `json:"input_tokens"`
	CachedTokens int64              `json:"cached_tokens"`
	OutputTokens int64              `json:"output_tokens"`
	TotalTokens  int64              `json:"total_tokens"`
	ActualTokens int64              `json:"actual_tokens"`
	LastActiveAt *time.Time         `json:"last_active_at,omitempty"`
}

// AgentUsageSummary aggregates retained Agent conversation usage for a tenant.
type AgentUsageSummary struct {
	UserCount      int   `json:"user_count"`
	UsersWithUsage int   `json:"users_with_usage"`
	SessionCount   int64 `json:"session_count"`
	MessageCount   int64 `json:"message_count"`
	LLMCallCount   int64 `json:"llm_call_count"`
	InputTokens    int64 `json:"input_tokens"`
	CachedTokens   int64 `json:"cached_tokens"`
	OutputTokens   int64 `json:"output_tokens"`
	TotalTokens    int64 `json:"total_tokens"`
	ActualTokens   int64 `json:"actual_tokens"`
}

// AgentUsageResponse contains the per-account breakdown and tenant totals.
type AgentUsageResponse struct {
	Items    []AgentAccountUsage `json:"items"`
	Total    int                 `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
	Summary  AgentUsageSummary   `json:"summary"`
}

// AgentAccountUsageQuery filters the tenant account usage overview.
type AgentAccountUsageQuery struct {
	Query  string `json:"query,omitempty"`
	Status string `json:"status,omitempty"`
}

// AgentSessionUsagePage contains one account's paginated session usage.
type AgentSessionUsagePage struct {
	Account  AgentAccountUsage   `json:"account"`
	Items    []AgentSessionUsage `json:"items"`
	Total    int                 `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
}

// AgentMemory 定義簡單記憶條目。
type AgentMemory struct {
	ID         string            `json:"id"`
	TenantID   string            `json:"tenant_id"`
	AccountID  string            `json:"account_id"`
	AgentID    string            `json:"agent_id,omitempty"`
	SessionID  string            `json:"session_id,omitempty"`
	Key        string            `json:"key,omitempty"`
	Content    string            `json:"content"`
	Source     AgentMemorySource `json:"source"`
	Importance int               `json:"importance"`
	ExpiresAt  *time.Time        `json:"expires_at,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// CreateAgentSessionInput 定義建立會話輸入。
type CreateAgentSessionInput struct {
	AgentID string `json:"agent_id"`
	Title   string `json:"title"`
}

// UpdateAgentSessionInput 定義更新會話輸入。
type UpdateAgentSessionInput struct {
	Title  *string `json:"title"`
	Status *string `json:"status"`
}

// ListAgentSessionsQuery 定義會話列表查詢。
type ListAgentSessionsQuery struct {
	AgentID  string `json:"agent_id"`
	Status   string `json:"status"`
	Cursor   string `json:"cursor,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
}

// ListAgentSessionMessagesQuery 定義會話訊息列表查詢。
type ListAgentSessionMessagesQuery struct {
	Cursor   string `json:"cursor,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
}

// KeysetPage 定義 (created_at, id) keyset 分頁條件。
type KeysetPage struct {
	Limit           int
	HasCursor       bool
	CursorCreatedAt time.Time
	CursorID        string
}

// AgentSessionListPage 包裝一頁會話 keyset 結果。
type AgentSessionListPage struct {
	Items      []AgentSession `json:"items"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

// AgentSessionMessageListPage 包裝一頁會話訊息 keyset 結果。
type AgentSessionMessageListPage struct {
	Items      []AgentSessionMessage `json:"items"`
	NextCursor string                `json:"next_cursor,omitempty"`
}

// CreateAgentMemoryInput 定義建立記憶輸入。
type CreateAgentMemoryInput struct {
	AgentID    string `json:"agent_id"`
	SessionID  string `json:"session_id"`
	Key        string `json:"key"`
	Content    string `json:"content"`
	Importance int    `json:"importance"`
}

// UpdateAgentMemoryInput 定義更新記憶輸入。
type UpdateAgentMemoryInput struct {
	Key        *string `json:"key"`
	Content    *string `json:"content"`
	Importance *int    `json:"importance"`
	ExpiresAt  *string `json:"expires_at"`
}

// ListAgentMemoriesQuery 定義記憶列表查詢。
type ListAgentMemoriesQuery struct {
	AgentID   string `json:"agent_id"`
	SessionID string `json:"session_id"`
}
