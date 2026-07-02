package domain

import "time"

// FormTemplate defines a reusable workflow form schema.
type FormTemplate struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	Key         string         `json:"key"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"schema,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// FormInstance records one submitted workflow form and its approval state.
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

// FormInstanceQuery carries list filters for workflow form instances.
type FormInstanceQuery struct {
	Status             string `json:"status,omitempty"`
	TemplateID         string `json:"template_id,omitempty"`
	TemplateKey        string `json:"template_key,omitempty"`
	ApplicantAccountID string `json:"applicant_account_id,omitempty"`
	Mine               bool   `json:"mine,omitempty"`
}

// SaveFormDraftInput carries the template and payload for a resumable form draft.
type SaveFormDraftInput struct {
	TemplateKey string         `json:"template_key,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
}

// UpdateFormDraftInput carries an updated payload for an existing draft.
type UpdateFormDraftInput struct {
	TemplateKey string         `json:"template_key,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
}

// CreateFormTemplateInput carries the payload for creating a form template.
type CreateFormTemplateInput struct {
	Key         string         `json:"key"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"schema,omitempty"`
}

// SubmitFormInput carries the payload used to submit a workflow form.
type SubmitFormInput struct {
	TemplateKey string         `json:"template_key"`
	Payload     map[string]any `json:"payload,omitempty"`
}

// ApproveFormInput is reserved for future approval metadata.
type ApproveFormInput struct{}

// RejectFormInput carries reviewer notes when a workflow form is rejected.
type RejectFormInput struct {
	Reason string `json:"reason,omitempty"`
}

// ReturnFormInput carries reviewer notes when a workflow form is returned for revision.
type ReturnFormInput struct {
	Reason string `json:"reason,omitempty"`
}

// CancelFormInput carries applicant notes when a workflow form is cancelled.
type CancelFormInput struct {
	Reason string `json:"reason,omitempty"`
}

// ExportedFormFile is a downloadable representation of a form instance.
type ExportedFormFile struct {
	FileName    string
	ContentType string
	Body        []byte
}

// BulkReviewFormsInput carries one notification-page batch review operation.
type BulkReviewFormsInput struct {
	FormInstanceIDs []string `json:"form_instance_ids"`
	Action          string   `json:"action"`
	Reason          string   `json:"reason,omitempty"`
}

// BulkReviewFormResult reports the outcome for one reviewed form instance.
type BulkReviewFormResult struct {
	FormInstanceID string        `json:"form_instance_id"`
	Success        bool          `json:"success"`
	Action         string        `json:"action,omitempty"`
	Code           string        `json:"code,omitempty"`
	Message        string        `json:"message,omitempty"`
	Instance       *FormInstance `json:"instance,omitempty"`
}

// BulkReviewFormsResponse wraps per-form batch review outcomes.
type BulkReviewFormsResponse struct {
	Results []BulkReviewFormResult `json:"results"`
}

// WorkflowReviewLogItem describes one visible review timeline entry.
type WorkflowReviewLogItem struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Role    string `json:"role,omitempty"`
	Time    string `json:"time"`
	Comment string `json:"comment,omitempty"`
}

// WorkflowReviewItem is the backend projection consumed by review/notification UIs.
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

// WorkflowReviewQueueResponse groups review items into the UI's tab model.
type WorkflowReviewQueueResponse struct {
	PendingReview   []WorkflowReviewItem `json:"pending_review"`
	AlreadyReviewed []WorkflowReviewItem `json:"already_reviewed"`
	Notified        []WorkflowReviewItem `json:"notified"`
}
