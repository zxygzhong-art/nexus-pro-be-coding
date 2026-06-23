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
