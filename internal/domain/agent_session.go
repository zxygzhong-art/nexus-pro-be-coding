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
	ID            string             `json:"id"`
	TenantID      string             `json:"tenant_id"`
	AccountID     string             `json:"account_id"`
	AgentID       string             `json:"agent_id,omitempty"`
	Title         string             `json:"title"`
	Status        AgentSessionStatus `json:"status"`
	LastMessageAt *time.Time         `json:"last_message_at,omitempty"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
}

// AgentSessionMessage 定義會話訊息。
type AgentSessionMessage struct {
	ID        string           `json:"id"`
	TenantID  string           `json:"tenant_id"`
	SessionID string           `json:"session_id"`
	Role      AgentMessageRole `json:"role"`
	Content   string           `json:"content"`
	RunID     string           `json:"run_id,omitempty"`
	Metadata  map[string]any   `json:"metadata,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
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
	AgentID string `json:"agent_id"`
	Status  string `json:"status"`
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
