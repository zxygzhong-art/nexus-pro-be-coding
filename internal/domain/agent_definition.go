package domain

import "time"

// AgentDefinitionStatus 表示 Agent 發布狀態。
type AgentDefinitionStatus string

const (
	AgentDefinitionStatusDraft     AgentDefinitionStatus = "draft"
	AgentDefinitionStatusPublished AgentDefinitionStatus = "published"
)

// AgentModelStatus 表示模型啟用狀態。
type AgentModelStatus string

const (
	AgentModelStatusActive   AgentModelStatus = "active"
	AgentModelStatusDisabled AgentModelStatus = "disabled"
)

// AgentVisibility 表示 Agent 可見範圍。
type AgentVisibility string

const (
	AgentVisibilityAll        AgentVisibility = "all"
	AgentVisibilityDepartment AgentVisibility = "department"
	AgentVisibilityRole       AgentVisibility = "role"
)

// AgentCategory 表示助理分類。
type AgentCategory string

const (
	AgentCategoryWorkflow  AgentCategory = "workflow"
	AgentCategoryDoc       AgentCategory = "doc"
	AgentCategoryAnalytics AgentCategory = "analytics"
	AgentCategoryIT        AgentCategory = "it"
)

// AgentModel 定義租戶模型設定。
type AgentModel struct {
	ID              string           `json:"id"`
	TenantID        string           `json:"tenant_id"`
	Name            string           `json:"name"`
	Provider        string           `json:"provider"`
	ModelName       string           `json:"model_name"`
	LiteLLMModel    string           `json:"litellm_model"`
	APIBaseURL      string           `json:"api_base_url,omitempty"`
	APIKey          string           `json:"-"`
	APIKeySet       bool             `json:"api_key_set"`
	APIKeyPreview   string           `json:"api_key_preview,omitempty"`
	RateLimitRPM    int              `json:"rate_limit_rpm"`
	Status          AgentModelStatus `json:"status"`
	TimeoutSeconds  int              `json:"timeout_seconds"`
	MonthlyQuota    int64            `json:"monthly_quota"`
	UsedQuota       int64            `json:"used_quota"`
	LastTestedAt    *time.Time       `json:"last_tested_at,omitempty"`
	LastTestStatus  string           `json:"last_test_status"`
	LastTestMessage string           `json:"last_test_message,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

// AgentDefinitionVersion 定義 Agent 版本快照。
type AgentDefinitionVersion struct {
	ID                 string    `json:"id"`
	TenantID           string    `json:"tenant_id"`
	AgentID            string    `json:"agent_id"`
	Version            int       `json:"version"`
	SystemPrompt       string    `json:"system_prompt"`
	Tools              []string  `json:"tools"`
	ModelID            string    `json:"model_id"`
	Note               string    `json:"note"`
	CreatedByAccountID string    `json:"created_by_account_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

// AgentUsageStats 定義用量統計。
type AgentUsageStats struct {
	TotalRuns    int64      `json:"total_runs"`
	SuccessRuns  int64      `json:"success_runs"`
	FailedRuns   int64      `json:"failed_runs"`
	AvgLatencyMs int        `json:"avg_latency_ms"`
	LastRunAt    *time.Time `json:"last_run_at,omitempty"`
	TopPrompts   []string   `json:"top_prompts,omitempty"`
}

// AgentDefinition 定義租戶 Agent。
type AgentDefinition struct {
	ID                 string                   `json:"id"`
	TenantID           string                   `json:"tenant_id"`
	Name               string                   `json:"name"`
	Description        string                   `json:"description"`
	Emoji              string                   `json:"emoji"`
	Category           AgentCategory            `json:"category"`
	ModelID            string                   `json:"model_id"`
	SystemPrompt       string                   `json:"system_prompt"`
	Tools              []string                 `json:"tools"`
	Status             AgentDefinitionStatus    `json:"status"`
	Visibility         AgentVisibility          `json:"visibility"`
	VisibilityTargets  []string                 `json:"visibility_targets"`
	TimeoutSeconds     int                      `json:"timeout_seconds"`
	Version            int                      `json:"version"`
	Versions           []AgentDefinitionVersion `json:"versions,omitempty"`
	Usage              AgentUsageStats          `json:"usage"`
	CreatedByAccountID string                   `json:"created_by_account_id,omitempty"`
	UpdatedByAccountID string                   `json:"updated_by_account_id,omitempty"`
	CreatedAt          time.Time                `json:"created_at"`
	UpdatedAt          time.Time                `json:"updated_at"`
}

// AgentAudit 定義 Agent/模型變更紀錄。
type AgentAudit struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	EntityType       string    `json:"entity_type"`
	EntityID         string    `json:"entity_id"`
	EntityName       string    `json:"entity_name"`
	Action           string    `json:"action"`
	ActorAccountID   string    `json:"actor_account_id,omitempty"`
	ActorDisplayName string    `json:"actor_display_name"`
	Detail           string    `json:"detail"`
	CreatedAt        time.Time `json:"created_at"`
}

// AgentToolMeta 定義工具說明。
type AgentToolMeta struct {
	Value              string `json:"value"`
	Label              string `json:"label"`
	Description        string `json:"description"`
	Readonly           bool   `json:"readonly"`
	RequiredPermission string `json:"required_permission"`
}

// CreateAgentModelInput 定義建立模型輸入。
type CreateAgentModelInput struct {
	Name           string `json:"name"`
	Provider       string `json:"provider"`
	ModelName      string `json:"model_name"`
	LiteLLMModel   string `json:"litellm_model"`
	APIBaseURL     string `json:"api_base_url"`
	APIKey         string `json:"api_key"`
	RateLimitRPM   int    `json:"rate_limit_rpm"`
	Status         string `json:"status"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	MonthlyQuota   int64  `json:"monthly_quota"`
}

// UpdateAgentModelInput 定義更新模型輸入。
type UpdateAgentModelInput struct {
	Name           *string `json:"name"`
	Provider       *string `json:"provider"`
	ModelName      *string `json:"model_name"`
	LiteLLMModel   *string `json:"litellm_model"`
	APIBaseURL     *string `json:"api_base_url"`
	APIKey         *string `json:"api_key"`
	RateLimitRPM   *int    `json:"rate_limit_rpm"`
	Status         *string `json:"status"`
	TimeoutSeconds *int    `json:"timeout_seconds"`
	MonthlyQuota   *int64  `json:"monthly_quota"`
}

// CreateAgentDefinitionInput 定義建立 Agent 輸入。
type CreateAgentDefinitionInput struct {
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Emoji             string   `json:"emoji"`
	Category          string   `json:"category"`
	ModelID           string   `json:"model_id"`
	SystemPrompt      string   `json:"system_prompt"`
	Tools             []string `json:"tools"`
	Visibility        string   `json:"visibility"`
	VisibilityTargets []string `json:"visibility_targets"`
	TimeoutSeconds    int      `json:"timeout_seconds"`
}

// UpdateAgentDefinitionInput 定義更新 Agent 輸入。
type UpdateAgentDefinitionInput struct {
	Name              *string  `json:"name"`
	Description       *string  `json:"description"`
	Emoji             *string  `json:"emoji"`
	Category          *string  `json:"category"`
	ModelID           *string  `json:"model_id"`
	SystemPrompt      *string  `json:"system_prompt"`
	Tools             []string `json:"tools"`
	Visibility        *string  `json:"visibility"`
	VisibilityTargets []string `json:"visibility_targets"`
	TimeoutSeconds    *int     `json:"timeout_seconds"`
	VersionNote       string   `json:"version_note"`
}

// RollbackAgentDefinitionInput 定義回滾輸入。
type RollbackAgentDefinitionInput struct {
	Version int `json:"version"`
}

// AgentTrialInput 定義試用對話輸入。
type AgentTrialInput struct {
	Message string `json:"message"`
}

// AgentTrialResult 定義試用對話結果。
type AgentTrialResult struct {
	Reply     string   `json:"reply"`
	LatencyMs int      `json:"latency_ms"`
	ToolsUsed []string `json:"tools_used"`
	ModelName string   `json:"model_name"`
}
