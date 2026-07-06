package domain

import "time"

// KnowledgeArticle 定義知識文章的資料結構。
type KnowledgeArticle struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

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

// Company 定義公司的資料結構。
type Company struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CompanyUser 定義公司使用者的資料結構。
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

// Role 定義角色的資料結構。
type Role struct {
	ID          string       `json:"id"`
	CompanyID   int          `json:"company_id"`
	Code        string       `json:"code"`
	Name        string       `json:"name"`
	Permissions []Permission `json:"permissions,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// Workspace 定義工作區的資料結構。
type Workspace struct {
	ID        string    `json:"id"`
	CompanyID int       `json:"company_id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WorkspaceUser 定義工作區使用者的資料結構。
type WorkspaceUser struct {
	ID          string    `json:"id"`
	CompanyID   int       `json:"company_id"`
	WorkspaceID string    `json:"workspace_id"`
	UserID      string    `json:"user_id"`
	RoleID      string    `json:"role_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// Agent 定義 agent 的資料結構。
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

// Knowledge 定義知識的資料結構。
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

// AgentKnowledge 定義 agent 知識的資料結構。
type AgentKnowledge struct {
	ID          string    `json:"id"`
	CompanyID   int       `json:"company_id"`
	AgentID     string    `json:"agent_id"`
	KnowledgeID string    `json:"knowledge_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// KnowledgeUserPermission 定義知識使用者權限的資料結構。
type KnowledgeUserPermission struct {
	ID          string    `json:"id"`
	CompanyID   int       `json:"company_id"`
	KnowledgeID string    `json:"knowledge_id"`
	UserID      string    `json:"user_id"`
	Permission  string    `json:"permission"`
	CreatedAt   time.Time `json:"created_at"`
}

// CompanyStorageConfig 定義公司 storage 組態的資料結構。
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

// KnowledgeFile 定義知識檔案的資料結構。
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

// FileProcessTask 定義檔案 process 任務的資料結構。
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

// AgentPlatformFile 定義 agent 平台檔案的資料結構。
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

// PricingPlan 定義 pricing plan 的資料結構。
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

// CompanyPlan 定義公司 plan 的資料結構。
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

// License 定義 license 的資料結構。
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
