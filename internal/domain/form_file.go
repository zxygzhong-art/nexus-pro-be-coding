package domain

import "time"

// FormInstanceFile is one attachment staged against a form instance field.
type FormInstanceFile struct {
	ID                 string     `json:"id"`
	TenantID           string     `json:"tenant_id"`
	FormInstanceID     string     `json:"form_instance_id"`
	FieldID            string     `json:"field_id"`
	CreatedByAccountID string     `json:"created_by_account_id"`
	OriginalFilename   string     `json:"original_filename"`
	ObjectProvider     string     `json:"-"`
	ObjectBucket       string     `json:"-"`
	ObjectKey          string     `json:"-"`
	ContentType        string     `json:"content_type"`
	SizeBytes          int64      `json:"size_bytes"`
	SHA256             string     `json:"sha256"`
	ScanStatus         string     `json:"scan_status"`
	ParseStatus        string     `json:"parse_status"`
	RetentionClass     string     `json:"retention_class"`
	State              string     `json:"state"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// UploadFormInstanceFileInput carries validated multipart bytes into the service.
type UploadFormInstanceFileInput struct {
	FieldID     string
	Filename    string
	ContentType string
	Content     []byte
}

// FormInstanceFileDownload carries authorized object bytes back to the HTTP layer.
type FormInstanceFileDownload struct {
	File    FormInstanceFile
	Content []byte
}

// FormInstanceFileListResponse lists attachments for one form instance.
type FormInstanceFileListResponse struct {
	Items []FormInstanceFile `json:"items"`
	Total int                `json:"total"`
}

// FormAttachmentRef is the JSON value stored inside a form payload file field.
type FormAttachmentRef struct {
	ID               string `json:"id"`
	OriginalFilename string `json:"original_filename"`
	ContentType      string `json:"content_type"`
	SizeBytes        int64  `json:"size_bytes"`
}
