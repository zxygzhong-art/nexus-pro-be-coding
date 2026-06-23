package domain

import "time"

// KnowledgeArticle stores searchable knowledge content for agent answers.
type KnowledgeArticle struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Reference describes a source snippet cited by an agent answer.
type Reference struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	Source  string `json:"source,omitempty"`
}

// AgentRunStatus is the lifecycle state of an agent run.
type AgentRunStatus string

// Agent run status values returned by the API.
const (
	AgentRunStatusQueued    AgentRunStatus = "queued"
	AgentRunStatusRunning   AgentRunStatus = "running"
	AgentRunStatusCompleted AgentRunStatus = "completed"
	AgentRunStatusFailed    AgentRunStatus = "failed"
)

// AgentRun records one user prompt, answer, and tool authorization decisions.
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

// CreateAgentRunInput carries the payload for starting an agent run.
type CreateAgentRunInput struct {
	Mode   string `json:"mode,omitempty"`
	Prompt string `json:"prompt"`
}

// Company represents an organization in the agent-platform compatibility model.
type Company struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CompanyUser represents a user membership in the agent-platform compatibility model.
type CompanyUser struct {
	ID           string    `json:"id"`
	CompanyID    int       `json:"company_id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	IsSuperAdmin bool      `json:"is_super_admin"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Role represents a company role with permissions in the compatibility model.
type Role struct {
	ID          string       `json:"id"`
	CompanyID   int          `json:"company_id"`
	Code        string       `json:"code"`
	Name        string       `json:"name"`
	Permissions []Permission `json:"permissions,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// Workspace represents a company workspace in the compatibility model.
type Workspace struct {
	ID        string    `json:"id"`
	CompanyID int       `json:"company_id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WorkspaceUser represents a user's workspace role assignment.
type WorkspaceUser struct {
	ID          string    `json:"id"`
	CompanyID   int       `json:"company_id"`
	WorkspaceID string    `json:"workspace_id"`
	UserID      string    `json:"user_id"`
	RoleID      string    `json:"role_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// Agent represents an AI agent configuration.
type Agent struct {
	ID                 string         `json:"id"`
	CompanyID          int            `json:"company_id"`
	WorkspaceID        string         `json:"workspace_id,omitempty"`
	Name               string         `json:"name"`
	AgentImage         string         `json:"agent_image"`
	AgentType          string         `json:"agent_type"`
	Opening            map[string]any `json:"opening,omitempty"`
	SuggestedQuestions []string       `json:"suggested_questions,omitempty"`
	Status             string         `json:"status"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

// Knowledge represents a knowledge base attached to agents.
type Knowledge struct {
	ID                 string         `json:"id"`
	CompanyID          int            `json:"company_id"`
	Name               string         `json:"name"`
	Description        string         `json:"description,omitempty"`
	MilvusCollectionID string         `json:"milvus_collection_id,omitempty"`
	Config             map[string]any `json:"config,omitempty"`
	Status             string         `json:"status"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

// AgentKnowledge links an agent to a knowledge base.
type AgentKnowledge struct {
	ID          string    `json:"id"`
	CompanyID   int       `json:"company_id"`
	AgentID     string    `json:"agent_id"`
	KnowledgeID string    `json:"knowledge_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// KnowledgeUserPermission grants a user access to a knowledge base.
type KnowledgeUserPermission struct {
	ID          string    `json:"id"`
	CompanyID   int       `json:"company_id"`
	KnowledgeID string    `json:"knowledge_id"`
	UserID      string    `json:"user_id"`
	Permission  string    `json:"permission"`
	CreatedAt   time.Time `json:"created_at"`
}

// CompanyStorageConfig describes where a company stores uploaded files.
type CompanyStorageConfig struct {
	ID        string         `json:"id"`
	CompanyID int            `json:"company_id"`
	Provider  string         `json:"provider"`
	Bucket    string         `json:"bucket"`
	BasePath  string         `json:"base_path,omitempty"`
	Config    map[string]any `json:"config,omitempty"`
	IsDefault bool           `json:"is_default"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// KnowledgeFile records one uploaded file associated with knowledge ingestion.
type KnowledgeFile struct {
	ID              string         `json:"id"`
	CompanyID       int            `json:"company_id"`
	KnowledgeID     string         `json:"knowledge_id,omitempty"`
	StorageConfigID string         `json:"storage_config_id,omitempty"`
	FileName        string         `json:"file_name"`
	MimeType        string         `json:"mime_type,omitempty"`
	SizeBytes       int64          `json:"size_bytes"`
	StoragePath     string         `json:"storage_path"`
	Checksum        string         `json:"checksum,omitempty"`
	Status          string         `json:"status"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// FileProcessTask tracks asynchronous processing for an uploaded file.
type FileProcessTask struct {
	ID           string         `json:"id"`
	CompanyID    int            `json:"company_id"`
	FileID       string         `json:"file_id"`
	TaskType     string         `json:"task_type"`
	Status       string         `json:"status"`
	QueueName    string         `json:"queue_name,omitempty"`
	Payload      map[string]any `json:"payload,omitempty"`
	Result       map[string]any `json:"result,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// AgentPlatformFile maps a local file to an external agent platform file ID.
type AgentPlatformFile struct {
	ID             string         `json:"id"`
	CompanyID      int            `json:"company_id"`
	AgentID        string         `json:"agent_id,omitempty"`
	UserID         string         `json:"user_id,omitempty"`
	FileID         string         `json:"file_id"`
	Platform       string         `json:"platform"`
	PlatformFileID string         `json:"platform_file_id"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

// PricingPlan describes commercial limits and features for a company plan.
type PricingPlan struct {
	ID                string         `json:"id"`
	Code              string         `json:"code"`
	Name              string         `json:"name"`
	MessageLimit      int            `json:"message_limit"`
	UserLimit         int            `json:"user_limit"`
	StorageLimitBytes int64          `json:"storage_limit_bytes"`
	Features          map[string]any `json:"features,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
}

// CompanyPlan records the active pricing plan for a company.
type CompanyPlan struct {
	ID            string         `json:"id"`
	CompanyID     int            `json:"company_id"`
	PricingPlanID string         `json:"pricing_plan_id"`
	Status        string         `json:"status"`
	QuotaSnapshot map[string]any `json:"quota_snapshot,omitempty"`
	StartsAt      time.Time      `json:"starts_at"`
	EndsAt        *time.Time     `json:"ends_at,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// License records a company license and its quota snapshot.
type License struct {
	ID            string         `json:"id"`
	CompanyID     int            `json:"company_id"`
	LicenseKey    string         `json:"license_key"`
	Status        string         `json:"status"`
	QuotaSnapshot map[string]any `json:"quota_snapshot,omitempty"`
	IssuedAt      time.Time      `json:"issued_at"`
	ExpiresAt     *time.Time     `json:"expires_at,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
}
