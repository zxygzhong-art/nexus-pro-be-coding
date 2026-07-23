package domain

import "time"

// AgentSessionFile defines one file uploaded into a session context version.
type AgentSessionFile struct {
	ID                 string     `json:"id"`
	TenantID           string     `json:"tenant_id"`
	SessionID          string     `json:"session_id"`
	SegmentID          string     `json:"segment_id"`
	ConversationFileID string     `json:"conversation_file_id"`
	ContextVersion     int64      `json:"context_version"`
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
	MessageID          string     `json:"message_id,omitempty"`
	Ordinal            *int       `json:"ordinal,omitempty"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// AgentMessageAttachment is the API/view shape for a file attached to one message.
type AgentMessageAttachment struct {
	MessageID          string           `json:"message_id"`
	ConversationFileID string           `json:"conversation_file_id"`
	Ordinal            int              `json:"ordinal"`
	File               AgentSessionFile `json:"file"`
}

// UploadAgentSessionFileInput defines a validated multipart upload passed into the service.
type UploadAgentSessionFileInput struct {
	Filename    string
	ContentType string
	Content     []byte
}

// AgentSessionFileDownload carries authorized object bytes back to the HTTP layer.
type AgentSessionFileDownload struct {
	File    AgentSessionFile
	Content []byte
}
