package domain

import "time"

// FormTemplate 定義表單範本的資料結構。
type FormTemplate struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id"`
	Key            string         `json:"key"`
	Name           string         `json:"name"`
	Description    string         `json:"description,omitempty"`
	Schema         map[string]any `json:"schema,omitempty"`
	Status         string         `json:"status"`
	CurrentVersion int            `json:"current_version"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      *time.Time     `json:"deleted_at,omitempty"`
}

// FormTemplateVersion 保存不可變的表單資料契約快照。
type FormTemplateVersion struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	TemplateID  string         `json:"template_id"`
	Version     int            `json:"version"`
	Schema      map[string]any `json:"schema,omitempty"`
	Status      string         `json:"status"`
	CreatedAt   time.Time      `json:"created_at"`
	PublishedAt *time.Time     `json:"published_at,omitempty"`
}

// RuntimeFormTemplate is the immutable, published form contract used by form fillers.
type RuntimeFormTemplate struct {
	ID                string                     `json:"id"`
	TemplateVersionID string                     `json:"template_version_id"`
	Key               string                     `json:"key"`
	Name              string                     `json:"name"`
	Description       string                     `json:"description,omitempty"`
	Version           int                        `json:"version"`
	FormKind          string                     `json:"form_kind"`
	Icon              string                     `json:"icon"`
	Fields            []PlatformFormBuilderField `json:"fields"`
	Stages            []PlatformFormBuilderStage `json:"stages"`
}

// FormInstance 定義表單實例的資料結構。
type FormInstance struct {
	ID                 string         `json:"id"`
	TenantID           string         `json:"tenant_id"`
	TemplateID         string         `json:"template_id"`
	TemplateVersionID  string         `json:"template_version_id"`
	ApplicantAccountID string         `json:"applicant_account_id"`
	Status             string         `json:"status"`
	Payload            map[string]any `json:"payload,omitempty"`
	SubmittedAt        time.Time      `json:"submitted_at"`
	ApprovedBy         string         `json:"approved_by,omitempty"`
	CurrentRunID       string         `json:"current_run_id,omitempty"`
	Version            int64          `json:"version"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

// FormInstanceDetail adds immutable template identity needed to render a submitted form.
type FormInstanceDetail struct {
	FormInstance
	TemplateKey  string `json:"template_key,omitempty"`
	TemplateName string `json:"template_name,omitempty"`
}

// FormInstanceFieldValue 保存可統計欄位的類型化投影。
type FormInstanceFieldValue struct {
	TenantID          string    `json:"tenant_id"`
	FormInstanceID    string    `json:"form_instance_id"`
	TemplateID        string    `json:"template_id"`
	TemplateVersionID string    `json:"template_version_id"`
	FieldID           string    `json:"field_id"`
	ValueType         string    `json:"value_type"`
	ValueText         string    `json:"value_text,omitempty"`
	ValueNumber       string    `json:"value_number,omitempty"`
	ValueBoolean      *bool     `json:"value_boolean,omitempty"`
	ValueDate         string    `json:"value_date,omitempty"`
	ValueTimestamp    string    `json:"value_timestamp,omitempty"`
	ValueJSON         []byte    `json:"value_json,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

// FormInstanceQuery 定義表單實例查詢的資料結構。
type FormInstanceQuery struct {
	Status             string `json:"status,omitempty"`
	TemplateID         string `json:"template_id,omitempty"`
	TemplateKey        string `json:"template_key,omitempty"`
	ApplicantAccountID string `json:"applicant_account_id,omitempty"`
	Mine               bool   `json:"mine,omitempty"`
}

// SaveFormDraftInput 定義表單草稿輸入的資料結構。
type SaveFormDraftInput struct {
	TemplateKey       string         `json:"template_key,omitempty"`
	TemplateVersionID string         `json:"template_version_id,omitempty"`
	Payload           map[string]any `json:"payload,omitempty"`
}

// UpdateFormDraftInput 定義表單草稿輸入的資料結構。
type UpdateFormDraftInput struct {
	TemplateKey string         `json:"template_key,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
}

// CreateFormTemplateInput 定義表單範本輸入的資料結構。
type CreateFormTemplateInput struct {
	Key         string         `json:"key"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"schema,omitempty"`
}

// SubmitFormInput 定義表單輸入的資料結構。
type SubmitFormInput struct {
	TemplateKey       string         `json:"template_key"`
	TemplateVersionID string         `json:"template_version_id,omitempty"`
	Payload           map[string]any `json:"payload,omitempty"`
}

// FormDataSourceField 描述可在表單設計器選用的資料欄位。
type FormDataSourceField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Type  string `json:"type"`
}

// FormDataSource 描述一個受租戶與權限約束的表單資料源。
type FormDataSource struct {
	ID      string                   `json:"id"`
	Label   string                   `json:"label"`
	Kind    string                   `json:"kind"`
	Fields  []FormDataSourceField    `json:"fields"`
	Records []map[string]interface{} `json:"records"`
}

// FormDataSourceCatalogResponse 回傳設計與執行階段共用的資料源目錄。
type FormDataSourceCatalogResponse struct {
	DataSources []FormDataSource `json:"data_sources"`
}

// ApproveFormInput 定義表單輸入的資料結構。
type ApproveFormInput struct {
	Reason string `json:"reason,omitempty"`
}

// RejectFormInput 定義表單輸入的資料結構。
type RejectFormInput struct {
	Reason string `json:"reason,omitempty"`
}

// ReturnFormInput 定義表單輸入的資料結構。
type ReturnFormInput struct {
	Reason string `json:"reason,omitempty"`
}

// CancelFormInput 定義表單輸入的資料結構。
type CancelFormInput struct {
	Reason string `json:"reason,omitempty"`
}

// ExportedFormFile 定義 exported 表單檔案的資料結構。
type ExportedFormFile struct {
	FileName    string
	ContentType string
	Body        []byte
}

// BulkReviewFormsInput 定義批次審核表單輸入的資料結構。
type BulkReviewFormsInput struct {
	FormInstanceIDs []string `json:"form_instance_ids"`
	Action          string   `json:"action"`
	Reason          string   `json:"reason,omitempty"`
}

// BulkReviewFormResult 定義批次審核表單結果的資料結構。
type BulkReviewFormResult struct {
	FormInstanceID string        `json:"form_instance_id"`
	Success        bool          `json:"success"`
	Action         string        `json:"action,omitempty"`
	Code           string        `json:"code,omitempty"`
	Message        string        `json:"message,omitempty"`
	Instance       *FormInstance `json:"instance,omitempty"`
}

// BulkReviewFormsResponse 定義批次審核表單回應的資料結構。
type BulkReviewFormsResponse struct {
	Results []BulkReviewFormResult `json:"results"`
}

// WorkflowReviewLogItem 定義流程審核 log 項目的資料結構。
type WorkflowReviewLogItem struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Role    string `json:"role,omitempty"`
	Time    string `json:"time"`
	Comment string `json:"comment,omitempty"`
}

// WorkflowReviewItem 定義流程審核項目的資料結構。
type WorkflowReviewItem struct {
	ID          string                     `json:"id"`
	TemplateKey string                     `json:"template_key"`
	FormKind    string                     `json:"form_kind"`
	Fields      []PlatformFormBuilderField `json:"fields"`
	Status      string                     `json:"status"`
	StatusText  string                     `json:"status_text"`
	Title       string                     `json:"title"`
	Who         string                     `json:"who,omitempty"`
	Desc        string                     `json:"desc"`
	Time        string                     `json:"time"`
	ReviewLog   []WorkflowReviewLogItem    `json:"review_log,omitempty"`
	Instance    FormInstance               `json:"instance"`
}

// WorkflowReviewQueueResponse 定義流程審核佇列回應的資料結構。
type WorkflowReviewQueueResponse struct {
	PendingReview   []WorkflowReviewItem `json:"pending_review"`
	AlreadyReviewed []WorkflowReviewItem `json:"already_reviewed"`
	Notified        []WorkflowReviewItem `json:"notified"`
}

// Workflow run / stage runtime statuses.
const (
	WorkflowRunStatusRunning   = "running"
	WorkflowRunStatusReturned  = "returned"
	WorkflowRunStatusCompleted = "completed"
	WorkflowRunStatusCancelled = "cancelled"

	WorkflowStageStatusPending   = "pending"
	WorkflowStageStatusActive    = "active"
	WorkflowStageStatusCompleted = "completed"
	WorkflowStageStatusSkipped   = "skipped"
	WorkflowStageStatusRejected  = "rejected"

	WorkflowAssigneeStatusPending  = "pending"
	WorkflowAssigneeStatusApproved = "approved"
	WorkflowAssigneeStatusRejected = "rejected"
	WorkflowAssigneeStatusReturned = "returned"

	WorkflowFormStatusInReview            = "in_review"
	WorkflowFormStatusReturned            = "returned"
	WorkflowFormStatusWorkflowStartFailed = "workflow_start_failed"

	WorkflowRunStatusStartFailed = "start_failed"
)

// WorkflowStageConfig 定義流程節點可執行設定。
type WorkflowStageConfig struct {
	Role                    string   `json:"role,omitempty"`
	RelativeLevel           int      `json:"relative_level,omitempty"`
	Mode                    string   `json:"mode,omitempty"`
	Field                   string   `json:"field,omitempty"`
	Operator                string   `json:"operator,omitempty"`
	Value                   string   `json:"value,omitempty"`
	RemindAfterHours        int      `json:"remind_after_hours,omitempty"`
	Levels                  []int    `json:"levels,omitempty"`
	TrueNextStageID         string   `json:"true_next_stage_id,omitempty"`
	FalseNextStageID        string   `json:"false_next_stage_id,omitempty"`
	AccountIDs              []string `json:"account_ids,omitempty"`
	UserGroupIDs            []string `json:"user_group_ids,omitempty"`
	ExcludeApplicant        bool     `json:"exclude_applicant,omitempty"`
	RequireDistinctApprover bool     `json:"require_distinct_approver,omitempty"`
}

// WorkflowStageDefinition 定義從 template 解析出的流程節點。
type WorkflowStageDefinition struct {
	ID     string              `json:"id"`
	Type   string              `json:"type"`
	Label  string              `json:"label"`
	Detail string              `json:"detail"`
	Config WorkflowStageConfig `json:"config"`
}

// WorkflowRun 定義單據流程運行實例。
type WorkflowRun struct {
	ID                     string    `json:"id"`
	TenantID               string    `json:"tenant_id"`
	FormInstanceID         string    `json:"form_instance_id"`
	TemplateID             string    `json:"template_id"`
	Version                int       `json:"version"`
	Status                 string    `json:"status"`
	CurrentStageInstanceID string    `json:"current_stage_instance_id,omitempty"`
	StageDefinitionsJSON   string    `json:"-"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// WorkflowStageInstance 定義流程節點運行實例。
type WorkflowStageInstance struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	RunID       string         `json:"run_id"`
	StageID     string         `json:"stage_id"`
	StageType   string         `json:"stage_type"`
	Label       string         `json:"label"`
	Status      string         `json:"status"`
	Sequence    int            `json:"sequence"`
	Result      map[string]any `json:"result,omitempty"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
}

// WorkflowStageAssignee 定義節點待辦人。
type WorkflowStageAssignee struct {
	TenantID        string `json:"tenant_id"`
	StageInstanceID string `json:"stage_instance_id"`
	AccountID       string `json:"account_id"`
	Status          string `json:"status"`
}

// WorkflowAction 定義流程審批動作歷史。
type WorkflowAction struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	RunID           string    `json:"run_id"`
	StageInstanceID string    `json:"stage_instance_id"`
	AccountID       string    `json:"account_id"`
	Action          string    `json:"action"`
	Comment         string    `json:"comment,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// WorkflowFormStep 定義前端流程進度條節點。
type WorkflowFormStep struct {
	StageID   string                     `json:"stage_id"`
	Label     string                     `json:"label"`
	Detail    string                     `json:"detail,omitempty"`
	State     string                     `json:"state"`
	Assignees []WorkflowFormStepAssignee `json:"assignees,omitempty"`
}

// WorkflowFormStepAssignee 定義流程進度條上的待辦人。
type WorkflowFormStepAssignee struct {
	AccountID string `json:"account_id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
}

// WorkflowFormStateResponse 定義單據流程運行狀態回應。
type WorkflowFormStateResponse struct {
	FormInstanceID    string                  `json:"form_instance_id"`
	FormStatus        string                  `json:"form_status"`
	RunID             string                  `json:"run_id,omitempty"`
	RunStatus         string                  `json:"run_status,omitempty"`
	CurrentStageID    string                  `json:"current_stage_id,omitempty"`
	CurrentStageLabel string                  `json:"current_stage_label,omitempty"`
	CanAct            bool                    `json:"can_act"`
	AllowedActions    []string                `json:"allowed_actions,omitempty"`
	Steps             []WorkflowFormStep      `json:"steps"`
	Actions           []WorkflowReviewLogItem `json:"actions,omitempty"`
}
