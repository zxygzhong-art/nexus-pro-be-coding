package domain

import "time"

// FormTemplate 定義表單範本的資料結構。
type FormTemplate struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	Key         string         `json:"key"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"schema,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// FormInstance 定義表單實例的資料結構。
type FormInstance struct {
	ID                 string         `json:"id"`
	TenantID           string         `json:"tenant_id"`
	TemplateID         string         `json:"template_id"`
	ApplicantAccountID string         `json:"applicant_account_id"`
	Status             string         `json:"status"`
	Payload            map[string]any `json:"payload,omitempty"`
	SubmittedAt        time.Time      `json:"submitted_at"`
	ApprovedBy         string         `json:"approved_by,omitempty"`
	UpdatedAt          time.Time      `json:"updated_at"`
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
	TemplateKey string         `json:"template_key,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
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
	TemplateKey string         `json:"template_key"`
	Payload     map[string]any `json:"payload,omitempty"`
}

// ApproveFormInput 定義表單輸入的資料結構。
type ApproveFormInput struct{}

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
	ID         string                  `json:"id"`
	Status     string                  `json:"status"`
	StatusText string                  `json:"status_text"`
	Title      string                  `json:"title"`
	Who        string                  `json:"who,omitempty"`
	Desc       string                  `json:"desc"`
	Time       string                  `json:"time"`
	ReviewLog  []WorkflowReviewLogItem `json:"review_log,omitempty"`
	Instance   FormInstance            `json:"instance"`
}

// WorkflowReviewQueueResponse 定義流程審核佇列回應的資料結構。
type WorkflowReviewQueueResponse struct {
	PendingReview   []WorkflowReviewItem `json:"pending_review"`
	AlreadyReviewed []WorkflowReviewItem `json:"already_reviewed"`
	Notified        []WorkflowReviewItem `json:"notified"`
}
