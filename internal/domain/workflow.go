package domain

import "time"

type FormTemplate struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	Key         string         `json:"key"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"schema,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

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
