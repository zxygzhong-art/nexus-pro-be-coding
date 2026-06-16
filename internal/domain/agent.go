package domain

import "time"

type KnowledgeArticle struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Reference struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	Source  string `json:"source,omitempty"`
}

type AgentRunStatus string

const (
	AgentRunStatusQueued    AgentRunStatus = "queued"
	AgentRunStatusRunning   AgentRunStatus = "running"
	AgentRunStatusCompleted AgentRunStatus = "completed"
	AgentRunStatusFailed    AgentRunStatus = "failed"
)

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
