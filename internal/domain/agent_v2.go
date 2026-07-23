package domain

import (
	"strings"
	"time"
)

// ModelConnectionStatus describes whether a model connection accepts new traffic.
type ModelConnectionStatus string

const (
	ModelConnectionStatusActive   ModelConnectionStatus = "active"
	ModelConnectionStatusDisabled ModelConnectionStatus = "disabled"
	ModelConnectionStatusArchived ModelConnectionStatus = "archived"
)

// Valid reports whether the model connection status is supported by storage.
func (s ModelConnectionStatus) Valid() bool {
	switch s {
	case ModelConnectionStatusActive, ModelConnectionStatusDisabled, ModelConnectionStatusArchived:
		return true
	default:
		return false
	}
}

// ModelConnectionSyncStatus describes desired-to-observed runtime reconciliation.
type ModelConnectionSyncStatus string

const (
	ModelConnectionSyncStatusPending ModelConnectionSyncStatus = "pending"
	ModelConnectionSyncStatusSynced  ModelConnectionSyncStatus = "synced"
	ModelConnectionSyncStatusFailed  ModelConnectionSyncStatus = "failed"
)

// Valid reports whether the model sync status is supported by storage.
func (s ModelConnectionSyncStatus) Valid() bool {
	return s == ModelConnectionSyncStatusPending || s == ModelConnectionSyncStatusSynced || s == ModelConnectionSyncStatusFailed
}

// ConnectionTestStatus is the latest explicit connection test result.
type ConnectionTestStatus string

const (
	ConnectionTestStatusUntested ConnectionTestStatus = "untested"
	ConnectionTestStatusOK       ConnectionTestStatus = "ok"
	ConnectionTestStatusFailed   ConnectionTestStatus = "failed"
)

// Valid reports whether the connection test status is supported by storage.
func (s ConnectionTestStatus) Valid() bool {
	return s == ConnectionTestStatusUntested || s == ConnectionTestStatusOK || s == ConnectionTestStatusFailed
}

// ModelConnection is the desired model gateway configuration.
type ModelConnection struct {
	ID                 string                `json:"id"`
	TenantID           string                `json:"tenant_id"`
	Name               string                `json:"name"`
	Provider           string                `json:"provider"`
	UpstreamModel      string                `json:"upstream_model"`
	APIBaseURL        string                `json:"api_base_url,omitempty"`
	APIKeyCiphertext  string                `json:"-"`
	APIKeyPreview     string                `json:"api_key_preview,omitempty"`
	RateLimitRPM      int                   `json:"rate_limit_rpm"`
	TimeoutMS          int                   `json:"timeout_ms"`
	Status             ModelConnectionStatus `json:"status"`
	CreatedByAccountID string                `json:"created_by_account_id,omitempty"`
	UpdatedByAccountID string                `json:"updated_by_account_id,omitempty"`
	CreatedAt          time.Time             `json:"created_at"`
	UpdatedAt          time.Time             `json:"updated_at"`
	ArchivedAt         *time.Time            `json:"archived_at,omitempty"`
}

// TimeoutDuration exposes timeout_ms without adding a second persisted timeout field.
func (c ModelConnection) TimeoutDuration() time.Duration {
	return time.Duration(c.TimeoutMS) * time.Millisecond
}

// TimeoutSeconds returns the rounded-up compatibility timeout in seconds.
func (c ModelConnection) TimeoutSeconds() int {
	if c.TimeoutMS <= 0 {
		return 0
	}
	return (c.TimeoutMS + 999) / 1000
}

// Validate checks desired model connection invariants without network I/O.
func (c ModelConnection) Validate() error {
	fields := make([]FieldError, 0, 10)
	agentV2Required(&fields, "id", c.ID)
	agentV2Required(&fields, "tenant_id", c.TenantID)
	agentV2Required(&fields, "name", c.Name)
	agentV2Required(&fields, "provider", c.Provider)
	agentV2Required(&fields, "upstream_model", c.UpstreamModel)
	if c.RateLimitRPM < 0 {
		fields = append(fields, agentV2Invalid("rate_limit_rpm", "rate_limit_rpm cannot be negative"))
	}
	if c.TimeoutMS <= 0 {
		fields = append(fields, agentV2Invalid("timeout_ms", "timeout_ms must be greater than zero"))
	}
	if !c.Status.Valid() {
		fields = append(fields, agentV2Invalid("status", "status must be active, disabled, or archived"))
	} else if c.Status == ModelConnectionStatusArchived && c.ArchivedAt == nil {
		fields = append(fields, agentV2RequiredError("archived_at"))
	} else if c.Status != ModelConnectionStatusArchived && c.ArchivedAt != nil {
		fields = append(fields, agentV2Invalid("archived_at", "archived_at is only valid for an archived connection"))
	}
	return agentV2ValidationResult("model connection is invalid", fields)
}

// ModelConnectionState is mutable observed gateway state for one desired connection.
type ModelConnectionState struct {
	TenantID             string                    `json:"tenant_id"`
	ModelConnectionID    string                    `json:"model_connection_id"`
	SyncStatus           ModelConnectionSyncStatus `json:"sync_status"`
	SyncedConfigChecksum string                    `json:"synced_config_checksum,omitempty"`
	LastSyncedAt         *time.Time                `json:"last_synced_at,omitempty"`
	LastSyncError        string                    `json:"last_sync_error,omitempty"`
	LastTestedAt         *time.Time                `json:"last_tested_at,omitempty"`
	LastTestStatus       ConnectionTestStatus      `json:"last_test_status"`
	LastTestMessage      string                    `json:"last_test_message,omitempty"`
	UpdatedAt            time.Time                 `json:"updated_at"`
}

// Validate checks observed model state independently from desired configuration.
func (s ModelConnectionState) Validate() error {
	fields := make([]FieldError, 0, 4)
	agentV2Required(&fields, "tenant_id", s.TenantID)
	agentV2Required(&fields, "model_connection_id", s.ModelConnectionID)
	if !s.SyncStatus.Valid() {
		fields = append(fields, agentV2Invalid("sync_status", "sync_status must be pending, synced, or failed"))
	}
	if !s.LastTestStatus.Valid() {
		fields = append(fields, agentV2Invalid("last_test_status", "last_test_status must be untested, ok, or failed"))
	}
	return agentV2ValidationResult("model connection state is invalid", fields)
}

// ExternalToolConnectionKind identifies the external tool protocol family.
type ExternalToolConnectionKind string

const (
	ExternalToolConnectionKindMCP  ExternalToolConnectionKind = "mcp"
	ExternalToolConnectionKindHTTP ExternalToolConnectionKind = "http"
)

// Valid reports whether the external tool kind is supported by storage.
func (k ExternalToolConnectionKind) Valid() bool {
	return k == ExternalToolConnectionKindMCP || k == ExternalToolConnectionKindHTTP
}

// ExternalToolTransport identifies how an external tool endpoint is invoked.
type ExternalToolTransport string

const (
	ExternalToolTransportSSE            ExternalToolTransport = "sse"
	ExternalToolTransportStreamableHTTP ExternalToolTransport = "streamable_http"
	ExternalToolTransportHTTP           ExternalToolTransport = "http"
)

// Valid reports whether the transport is supported by storage.
func (t ExternalToolTransport) Valid() bool {
	return t == ExternalToolTransportSSE || t == ExternalToolTransportStreamableHTTP || t == ExternalToolTransportHTTP
}

// ExternalToolAuthType identifies the credential shape used by an external tool connection.
type ExternalToolAuthType string

const (
	ExternalToolAuthTypeNone   ExternalToolAuthType = "none"
	ExternalToolAuthTypeBearer ExternalToolAuthType = "bearer"
	ExternalToolAuthTypeAPIKey ExternalToolAuthType = "api_key"
	ExternalToolAuthTypeBasic  ExternalToolAuthType = "basic"
)

// Valid reports whether the auth type is supported by storage.
func (a ExternalToolAuthType) Valid() bool {
	switch a {
	case ExternalToolAuthTypeNone, ExternalToolAuthTypeBearer, ExternalToolAuthTypeAPIKey, ExternalToolAuthTypeBasic:
		return true
	default:
		return false
	}
}

// ExternalToolConnectionStatus describes whether a connection accepts new invocations.
type ExternalToolConnectionStatus string

const (
	ExternalToolConnectionStatusActive   ExternalToolConnectionStatus = "active"
	ExternalToolConnectionStatusDisabled ExternalToolConnectionStatus = "disabled"
	ExternalToolConnectionStatusArchived ExternalToolConnectionStatus = "archived"
)

// Valid reports whether the external tool status is supported by storage.
func (s ExternalToolConnectionStatus) Valid() bool {
	return s == ExternalToolConnectionStatusActive || s == ExternalToolConnectionStatusDisabled || s == ExternalToolConnectionStatusArchived
}

// ExternalToolConnection is tenant-owned remote tool-server configuration and its latest test result.
type ExternalToolConnection struct {
	ID                 string                       `json:"id"`
	TenantID           string                       `json:"tenant_id"`
	Name               string                       `json:"name"`
	Description        string                       `json:"description"`
	Kind               ExternalToolConnectionKind   `json:"kind"`
	Transport          ExternalToolTransport        `json:"transport"`
	EndpointURL        string                       `json:"endpoint_url"`
	AuthType           ExternalToolAuthType         `json:"auth_type"`
	AuthHeaderName       string                       `json:"auth_header_name,omitempty"`
	AuthUsername         string                       `json:"auth_username,omitempty"`
	AuthSecretCiphertext string                       `json:"-"`
	TimeoutMS            int                          `json:"timeout_ms"`
	Status             ExternalToolConnectionStatus `json:"status"`
	LastTestedAt       *time.Time                   `json:"last_tested_at,omitempty"`
	LastTestStatus     ConnectionTestStatus         `json:"last_test_status"`
	LastTestMessage    string                       `json:"last_test_message,omitempty"`
	CreatedByAccountID string                       `json:"created_by_account_id,omitempty"`
	UpdatedByAccountID string                       `json:"updated_by_account_id,omitempty"`
	CreatedAt          time.Time                    `json:"created_at"`
	UpdatedAt          time.Time                    `json:"updated_at"`
	ArchivedAt         *time.Time                   `json:"archived_at,omitempty"`
}

// Validate checks external connection metadata without calling the endpoint.
func (c ExternalToolConnection) Validate() error {
	fields := make([]FieldError, 0, 12)
	agentV2Required(&fields, "id", c.ID)
	agentV2Required(&fields, "tenant_id", c.TenantID)
	agentV2Required(&fields, "name", c.Name)
	agentV2Required(&fields, "endpoint_url", c.EndpointURL)
	if !c.Kind.Valid() {
		fields = append(fields, agentV2Invalid("kind", "kind must be mcp or http"))
	}
	if !c.Transport.Valid() {
		fields = append(fields, agentV2Invalid("transport", "transport must be sse, streamable_http, or http"))
	} else if (c.Kind == ExternalToolConnectionKindMCP && c.Transport == ExternalToolTransportHTTP) ||
		(c.Kind == ExternalToolConnectionKindHTTP && c.Transport != ExternalToolTransportHTTP) {
		fields = append(fields, agentV2Invalid("transport", "transport is not valid for the selected connection kind"))
	}
	if !c.AuthType.Valid() {
		fields = append(fields, agentV2Invalid("auth_type", "auth_type must be none, bearer, api_key, or basic"))
	} else if c.AuthType != ExternalToolAuthTypeNone && strings.TrimSpace(c.AuthSecretCiphertext) == "" {
		fields = append(fields, agentV2RequiredError("auth_secret_ciphertext"))
	}
	if c.TimeoutMS < 1_000 || c.TimeoutMS > 120_000 {
		fields = append(fields, agentV2Invalid("timeout_ms", "timeout_ms must be between 1000 and 120000"))
	}
	if c.AuthType == ExternalToolAuthTypeAPIKey && strings.TrimSpace(c.AuthHeaderName) == "" {
		fields = append(fields, agentV2RequiredError("auth_header_name"))
	}
	if !c.Status.Valid() {
		fields = append(fields, agentV2Invalid("status", "status must be active, disabled, or archived"))
	} else if c.Status == ExternalToolConnectionStatusArchived && c.ArchivedAt == nil {
		fields = append(fields, agentV2RequiredError("archived_at"))
	} else if c.Status != ExternalToolConnectionStatusArchived && c.ArchivedAt != nil {
		fields = append(fields, agentV2Invalid("archived_at", "archived_at is only valid for an archived connection"))
	}
	if !c.LastTestStatus.Valid() {
		fields = append(fields, agentV2Invalid("last_test_status", "last_test_status must be untested, ok, or failed"))
	}
	return agentV2ValidationResult("external tool connection is invalid", fields)
}

// ExternalToolCapability is one discovered operation persisted in external_tools.
type ExternalToolCapability struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id"`
	ConnectionID   string         `json:"connection_id"`
	ToolName       string         `json:"tool_name"`
	Description    string         `json:"description"`
	HTTPMethod     string         `json:"http_method,omitempty"`
	HTTPPath       string         `json:"http_path,omitempty"`
	InputSchema    map[string]any `json:"input_schema"`
	OutputSchema   map[string]any `json:"output_schema"`
	Readonly       bool           `json:"readonly"`
	Enabled        bool           `json:"enabled"`
	SchemaChecksum string         `json:"schema_checksum,omitempty"`
	DiscoveredAt   time.Time      `json:"discovered_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	ArchivedAt     *time.Time     `json:"archived_at,omitempty"`
}

// Validate checks persisted external capability identity and HTTP method constraints.
func (c ExternalToolCapability) Validate() error {
	fields := make([]FieldError, 0, 5)
	agentV2Required(&fields, "id", c.ID)
	agentV2Required(&fields, "tenant_id", c.TenantID)
	agentV2Required(&fields, "connection_id", c.ConnectionID)
	agentV2Required(&fields, "tool_name", c.ToolName)
	if !externalToolHTTPMethodValid(c.HTTPMethod) {
		fields = append(fields, agentV2Invalid("http_method", "http_method must be empty, GET, POST, PUT, PATCH, or DELETE"))
	}
	return agentV2ValidationResult("external tool capability is invalid", fields)
}

// AgentLifecycleStatus describes the lifecycle of a stable Agent identity.
type AgentLifecycleStatus string

const (
	AgentLifecycleStatusActive   AgentLifecycleStatus = "active"
	AgentLifecycleStatusArchived AgentLifecycleStatus = "archived"
)

// Valid reports whether the Agent lifecycle status is supported by storage.
func (s AgentLifecycleStatus) Valid() bool {
	return s == AgentLifecycleStatusActive || s == AgentLifecycleStatusArchived
}

// Agent is stable identity plus draft and published revision pointers.
type Agent struct {
	ID                  string               `json:"id"`
	TenantID            string               `json:"tenant_id"`
	ParentAgentID       string               `json:"parent_agent_id,omitempty"`
	LifecycleStatus     AgentLifecycleStatus `json:"lifecycle_status"`
	DraftRevisionID     string               `json:"draft_revision_id,omitempty"`
	PublishedRevisionID string               `json:"published_revision_id,omitempty"`
	NextRevisionNo      int                  `json:"next_revision_no"`
	CreatedByAccountID  string               `json:"created_by_account_id,omitempty"`
	CreatedAt           time.Time            `json:"created_at"`
	UpdatedAt           time.Time            `json:"updated_at"`
	ArchivedAt          *time.Time           `json:"archived_at,omitempty"`
}

// Validate checks stable Agent identity and archive consistency.
func (a Agent) Validate() error {
	fields := make([]FieldError, 0, 6)
	agentV2Required(&fields, "id", a.ID)
	agentV2Required(&fields, "tenant_id", a.TenantID)
	if a.ParentAgentID == a.ID {
		fields = append(fields, agentV2Invalid("parent_agent_id", "an agent cannot be its own parent"))
	}
	if a.NextRevisionNo <= 0 {
		fields = append(fields, agentV2Invalid("next_revision_no", "next_revision_no must be greater than zero"))
	}
	if !a.LifecycleStatus.Valid() {
		fields = append(fields, agentV2Invalid("lifecycle_status", "lifecycle_status must be active or archived"))
	} else if a.LifecycleStatus == AgentLifecycleStatusArchived && a.ArchivedAt == nil {
		fields = append(fields, agentV2RequiredError("archived_at"))
	} else if a.LifecycleStatus != AgentLifecycleStatusArchived && a.ArchivedAt != nil {
		fields = append(fields, agentV2Invalid("archived_at", "archived_at is only valid for an archived agent"))
	}
	return agentV2ValidationResult("agent identity is invalid", fields)
}

// AgentRevision is the immutable display, access, and runtime row for one Agent version.
type AgentRevision struct {
	ID                            string                            `json:"id"`
	TenantID                      string                            `json:"tenant_id"`
	AgentID                       string                            `json:"agent_id"`
	RevisionNo                    int                               `json:"revision_no"`
	Ordinal                       *int                              `json:"ordinal,omitempty"`
	Name                          string                            `json:"name"`
	Description                   string                            `json:"description"`
	Icon                          string                            `json:"icon"`
	Category                      AgentCategory                     `json:"category"`
	Visibility                    AgentVisibility                   `json:"visibility"`
	VisibilityTargets             []string                          `json:"visibility_targets"`
	MainAgentRole                 string                            `json:"main_agent_role"`
	SystemPrompt                  string                            `json:"system_prompt"`
	WelcomeMessage                string                            `json:"welcome_message"`
	SuggestedQuestions            []string                          `json:"suggested_questions"`
	SuggestedQuestionTranslations []LocalizedAgentSuggestedQuestion `json:"suggested_question_translations"`
	ModelConnectionID             string                            `json:"model_connection_id"`
	ModelConfigChecksum           string                            `json:"model_config_checksum,omitempty"`
	TimeoutMS                     int                               `json:"timeout_ms"`
	ConfigSchemaVersion           int                               `json:"config_schema_version"`
	Checksum                      string                            `json:"checksum"`
	RevisionNote                  string                            `json:"revision_note"`
	CreatedByAccountID            string                            `json:"created_by_account_id,omitempty"`
	CreatedAt                     time.Time                         `json:"created_at"`
}

// TimeoutDuration exposes timeout_ms without duplicating persisted timeout state.
func (r AgentRevision) TimeoutDuration() time.Duration {
	return time.Duration(r.TimeoutMS) * time.Millisecond
}

// TimeoutSeconds returns the rounded-up compatibility timeout in seconds.
func (r AgentRevision) TimeoutSeconds() int {
	if r.TimeoutMS <= 0 {
		return 0
	}
	return (r.TimeoutMS + 999) / 1000
}

// Validate checks the persisted Agent revision row.
func (r AgentRevision) Validate() error {
	fields := make([]FieldError, 0, 14)
	agentV2Required(&fields, "id", r.ID)
	agentV2Required(&fields, "tenant_id", r.TenantID)
	agentV2Required(&fields, "agent_id", r.AgentID)
	agentV2Required(&fields, "name", r.Name)
	agentV2Required(&fields, "model_connection_id", r.ModelConnectionID)
	agentV2Required(&fields, "checksum", r.Checksum)
	if r.RevisionNo <= 0 {
		fields = append(fields, agentV2Invalid("revision_no", "revision_no must be greater than zero"))
	}
	if r.Ordinal != nil && *r.Ordinal < 0 {
		fields = append(fields, agentV2Invalid("ordinal", "ordinal must not be negative"))
	}
	if !agentV2CategoryValid(r.Category) {
		fields = append(fields, agentV2Invalid("category", "category must be workflow, doc, analytics, or it"))
	}
	if !agentV2VisibilityValid(r.Visibility) {
		fields = append(fields, agentV2Invalid("visibility", "visibility must be all, department, or role"))
	} else if r.Visibility != AgentVisibilityAll && len(r.VisibilityTargets) == 0 {
		fields = append(fields, agentV2RequiredError("visibility_targets"))
	}
	if r.TimeoutMS <= 0 {
		fields = append(fields, agentV2Invalid("timeout_ms", "timeout_ms must be greater than zero"))
	}
	if r.ConfigSchemaVersion <= 0 {
		fields = append(fields, agentV2Invalid("config_schema_version", "config_schema_version must be greater than zero"))
	}
	if len(r.SuggestedQuestions) > MaxAgentSuggestedQuestions {
		fields = append(fields, agentV2Invalid("suggested_questions", "too many suggested questions"))
	}
	for _, question := range r.SuggestedQuestions {
		if len([]rune(strings.TrimSpace(question))) > MaxAgentSuggestedQuestionCharacters {
			fields = append(fields, agentV2Invalid("suggested_questions", "a suggested question exceeds the maximum length"))
			break
		}
	}
	return agentV2ValidationResult("agent revision is invalid", fields)
}

// AgentRevisionBuiltinTool is a root built-in tool binding.
type AgentRevisionBuiltinTool struct {
	TenantID   string         `json:"tenant_id"`
	RevisionID string         `json:"revision_id"`
	ToolKey    string         `json:"tool_key"`
	Ordinal    int            `json:"ordinal"`
	Config     map[string]any `json:"config"`
}

// AgentRevisionExternalTool is a root external tool binding with a publish-time schema checksum.
type AgentRevisionExternalTool struct {
	TenantID           string         `json:"tenant_id"`
	RevisionID         string         `json:"revision_id"`
	ExternalToolID     string         `json:"external_tool_id"`
	ToolSchemaChecksum string         `json:"tool_schema_checksum,omitempty"`
	Ordinal            int            `json:"ordinal"`
	Config             map[string]any `json:"config"`
}

// AgentRevisionKnowledgeBase is a root knowledge-base binding.
type AgentRevisionKnowledgeBase struct {
	TenantID        string `json:"tenant_id"`
	RevisionID      string `json:"revision_id"`
	KnowledgeBaseID string `json:"knowledge_base_id"`
	Ordinal         int    `json:"ordinal"`
}

// AgentRevisionSnapshot assembles one revision row and its normalized binding rows for services.
type AgentRevisionSnapshot struct {
	Revision       AgentRevision                `json:"revision"`
	BuiltinTools   []AgentRevisionBuiltinTool   `json:"builtin_tools"`
	ExternalTools  []AgentRevisionExternalTool  `json:"external_tools"`
	KnowledgeBases []AgentRevisionKnowledgeBase `json:"knowledge_bases"`
}

// Validate checks the revision row and normalized binding ownership and ordinals.
func (s AgentRevisionSnapshot) Validate() error {
	if err := s.Revision.Validate(); err != nil {
		return err
	}
	fields := make([]FieldError, 0, 3)
	for _, binding := range s.BuiltinTools {
		if !agentV2BindingOwnerValid(s.Revision, binding.TenantID, binding.RevisionID) || strings.TrimSpace(binding.ToolKey) == "" || binding.Ordinal < 0 {
			fields = append(fields, agentV2Invalid("builtin_tools", "one or more built-in tool bindings are invalid"))
			break
		}
	}
	for _, binding := range s.ExternalTools {
		if !agentV2BindingOwnerValid(s.Revision, binding.TenantID, binding.RevisionID) || strings.TrimSpace(binding.ExternalToolID) == "" || binding.Ordinal < 0 {
			fields = append(fields, agentV2Invalid("external_tools", "one or more external tool bindings are invalid"))
			break
		}
	}
	for _, binding := range s.KnowledgeBases {
		if !agentV2BindingOwnerValid(s.Revision, binding.TenantID, binding.RevisionID) || strings.TrimSpace(binding.KnowledgeBaseID) == "" || binding.Ordinal < 0 {
			fields = append(fields, agentV2Invalid("knowledge_bases", "one or more knowledge-base bindings are invalid"))
			break
		}
	}
	return agentV2ValidationResult("agent revision snapshot is invalid", fields)
}

// ConversationStatus describes whether a conversation accepts new messages.
type ConversationStatus string

const (
	ConversationStatusActive   ConversationStatus = "active"
	ConversationStatusArchived ConversationStatus = "archived"
)

// Valid reports whether the conversation status is supported by storage.
func (s ConversationStatus) Valid() bool {
	return s == ConversationStatusActive || s == ConversationStatusArchived
}

// Conversation is the stable, user-owned container for context segments.
type Conversation struct {
	ID                  string             `json:"id"`
	TenantID            string             `json:"tenant_id"`
	OwnerAccountID      string             `json:"owner_account_id"`
	AgentID             string             `json:"agent_id,omitempty"`
	CurrentSegmentID    string             `json:"current_segment_id,omitempty"`
	NextMessageSequence int64              `json:"next_message_sequence"`
	Title               string             `json:"title"`
	Status              ConversationStatus `json:"status"`
	LastMessageAt       *time.Time         `json:"last_message_at,omitempty"`
	CreatedAt           time.Time          `json:"created_at"`
	UpdatedAt           time.Time          `json:"updated_at"`
	ArchivedAt          *time.Time         `json:"archived_at,omitempty"`
}

// Validate checks conversation ownership, sequence allocation, and archive consistency.
func (c Conversation) Validate() error {
	fields := make([]FieldError, 0, 7)
	agentV2Required(&fields, "id", c.ID)
	agentV2Required(&fields, "tenant_id", c.TenantID)
	agentV2Required(&fields, "owner_account_id", c.OwnerAccountID)
	if c.NextMessageSequence <= 0 {
		fields = append(fields, agentV2Invalid("next_message_sequence", "next_message_sequence must be greater than zero"))
	}
	if !c.Status.Valid() {
		fields = append(fields, agentV2Invalid("status", "status must be active or archived"))
	} else if c.Status == ConversationStatusArchived && c.ArchivedAt == nil {
		fields = append(fields, agentV2RequiredError("archived_at"))
	} else if c.Status != ConversationStatusArchived && c.ArchivedAt != nil {
		fields = append(fields, agentV2Invalid("archived_at", "archived_at is only valid for an archived conversation"))
	}
	return agentV2ValidationResult("conversation is invalid", fields)
}

// ConversationSegmentReason records why a context segment began.
type ConversationSegmentReason string

const (
	ConversationSegmentReasonInitial      ConversationSegmentReason = "initial"
	ConversationSegmentReasonContextReset ConversationSegmentReason = "context_reset"
)

// Valid reports whether the segment reason is supported by storage.
func (r ConversationSegmentReason) Valid() bool {
	return r == ConversationSegmentReasonInitial || r == ConversationSegmentReasonContextReset
}

// ConversationSegment is one immutable context boundary within a conversation.
type ConversationSegment struct {
	ID             string                    `json:"id"`
	TenantID       string                    `json:"tenant_id"`
	ConversationID string                    `json:"conversation_id"`
	Ordinal        int                       `json:"ordinal"`
	StartReason    ConversationSegmentReason `json:"start_reason"`
	CreatedAt      time.Time                 `json:"created_at"`
}

// MessageRole identifies the producer of a persisted message.
type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleSystem    MessageRole = "system"
	MessageRoleTool      MessageRole = "tool"
)

// Valid reports whether the message role is supported by storage.
func (r MessageRole) Valid() bool {
	return r == MessageRoleUser || r == MessageRoleAssistant || r == MessageRoleSystem || r == MessageRoleTool
}

// ConversationMessage is the sole source of user prompt and assistant answer content.
type ConversationMessage struct {
	ID              string         `json:"id"`
	TenantID        string         `json:"tenant_id"`
	ConversationID  string         `json:"conversation_id"`
	SegmentID       string         `json:"segment_id"`
	SequenceNo      int64          `json:"sequence_no"`
	Role            MessageRole    `json:"role"`
	Content         string         `json:"content"`
	ContentJSON     map[string]any `json:"content_json,omitempty"`
	ExecutionID     string         `json:"execution_id,omitempty"`
	ExecutionStepID string         `json:"execution_step_id,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
}

// ExecutionTriggerType identifies the entrypoint that created an execution.
type ExecutionTriggerType string

const (
	ExecutionTriggerTypeChat   ExecutionTriggerType = "chat"
	ExecutionTriggerTypeAPI    ExecutionTriggerType = "api"
	ExecutionTriggerTypeTrial  ExecutionTriggerType = "trial"
	ExecutionTriggerTypeSystem ExecutionTriggerType = "system"
)

// Valid reports whether the execution trigger is supported by storage.
func (t ExecutionTriggerType) Valid() bool {
	return t == ExecutionTriggerTypeChat || t == ExecutionTriggerTypeAPI || t == ExecutionTriggerTypeTrial || t == ExecutionTriggerTypeSystem
}

// ExecutionStatus describes the persisted orchestration lifecycle.
type ExecutionStatus string

const (
	ExecutionStatusQueued    ExecutionStatus = "queued"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusCompleted ExecutionStatus = "completed"
	ExecutionStatusFailed    ExecutionStatus = "failed"
	ExecutionStatusCancelled ExecutionStatus = "cancelled"
)

// Valid reports whether the execution status is supported by storage.
func (s ExecutionStatus) Valid() bool {
	return s == ExecutionStatusQueued || s == ExecutionStatusRunning || s == ExecutionStatusCompleted || s == ExecutionStatusFailed || s == ExecutionStatusCancelled
}

// Terminal reports whether an execution can no longer transition.
func (s ExecutionStatus) Terminal() bool {
	return s == ExecutionStatusCompleted || s == ExecutionStatusFailed || s == ExecutionStatusCancelled
}

// CanTransitionTo enforces monotonic execution state changes.
func (s ExecutionStatus) CanTransitionTo(next ExecutionStatus) bool {
	if s == next {
		return true
	}
	switch s {
	case ExecutionStatusQueued:
		return next == ExecutionStatusRunning || next == ExecutionStatusFailed || next == ExecutionStatusCancelled
	case ExecutionStatusRunning:
		return next.Terminal()
	default:
		return false
	}
}

// Execution stores invocation identity, state, timing, errors, and usage; message content is not duplicated here.
type Execution struct {
	ID                string               `json:"id"`
	TenantID          string               `json:"tenant_id"`
	AccountID         string               `json:"account_id"`
	ConversationID    string               `json:"conversation_id"`
	SegmentID         string               `json:"segment_id"`
	InputMessageID    string               `json:"input_message_id"`
	AgentID           string               `json:"agent_id,omitempty"`
	AgentRevisionID   string               `json:"agent_revision_id,omitempty"`
	ModelConnectionID string               `json:"model_connection_id,omitempty"`
	Mode              string               `json:"mode"`
	TriggerType       ExecutionTriggerType `json:"trigger_type"`
	Status            ExecutionStatus      `json:"status"`
	QueuedAt          time.Time            `json:"queued_at"`
	StartedAt         *time.Time           `json:"started_at,omitempty"`
	CompletedAt       *time.Time           `json:"completed_at,omitempty"`
	ErrorCode         string               `json:"error_code,omitempty"`
	ErrorCategory     string               `json:"error_category,omitempty"`
	SafeErrorMessage  string               `json:"safe_error_message,omitempty"`
	LLMCallCount      int64                `json:"llm_call_count"`
	InputTokens       int64                `json:"input_tokens"`
	CachedTokens      int64                `json:"cached_tokens"`
	OutputTokens      int64                `json:"output_tokens"`
	TotalTokens       int64                `json:"total_tokens"`
	UsageComplete     bool                 `json:"usage_complete"`
	CreatedAt         time.Time            `json:"created_at"`
	UpdatedAt         time.Time            `json:"updated_at"`
}

// Validate checks the same reference, usage, and timestamp invariants enforced by executions storage.
func (e Execution) Validate() error {
	fields := make([]FieldError, 0, 14)
	agentV2Required(&fields, "id", e.ID)
	agentV2Required(&fields, "tenant_id", e.TenantID)
	agentV2Required(&fields, "account_id", e.AccountID)
	agentV2Required(&fields, "conversation_id", e.ConversationID)
	agentV2Required(&fields, "segment_id", e.SegmentID)
	agentV2Required(&fields, "input_message_id", e.InputMessageID)
	if !e.TriggerType.Valid() {
		fields = append(fields, agentV2Invalid("trigger_type", "trigger_type must be chat, api, trial, or system"))
	}
	if !e.Status.Valid() {
		fields = append(fields, agentV2Invalid("status", "execution status is invalid"))
	}
	agentBindingCount := 0
	for _, value := range []string{e.AgentID, e.AgentRevisionID, e.ModelConnectionID} {
		if strings.TrimSpace(value) != "" {
			agentBindingCount++
		}
	}
	if agentBindingCount != 0 && agentBindingCount != 3 {
		fields = append(fields, agentV2Invalid("agent_id", "agent_id, agent_revision_id, and model_connection_id must be set together"))
	}
	if e.LLMCallCount < 0 || e.InputTokens < 0 || e.CachedTokens < 0 || e.OutputTokens < 0 || e.TotalTokens < 0 {
		fields = append(fields, agentV2Invalid("usage", "execution usage cannot be negative"))
	} else if e.CachedTokens > e.InputTokens {
		fields = append(fields, agentV2Invalid("cached_tokens", "cached_tokens cannot exceed input_tokens"))
	}
	switch e.Status {
	case ExecutionStatusQueued:
		if e.StartedAt != nil || e.CompletedAt != nil {
			fields = append(fields, agentV2Invalid("timestamps", "queued execution cannot have started_at or completed_at"))
		}
	case ExecutionStatusRunning:
		if e.StartedAt == nil || e.CompletedAt != nil {
			fields = append(fields, agentV2Invalid("timestamps", "running execution requires started_at and no completed_at"))
		}
	case ExecutionStatusCompleted, ExecutionStatusFailed, ExecutionStatusCancelled:
		if e.CompletedAt == nil {
			fields = append(fields, agentV2RequiredError("completed_at"))
		}
	}
	return agentV2ValidationResult("execution is invalid", fields)
}

// ExecutionStepType identifies one observable orchestration operation.
type ExecutionStepType string

const (
	ExecutionStepTypeLLM       ExecutionStepType = "llm"
	ExecutionStepTypeTool      ExecutionStepType = "tool"
	ExecutionStepTypeSubAgent  ExecutionStepType = "sub_agent"
	ExecutionStepTypeRetrieval ExecutionStepType = "retrieval"
)

// Valid reports whether the execution step type is supported by storage.
func (t ExecutionStepType) Valid() bool {
	return t == ExecutionStepTypeLLM || t == ExecutionStepTypeTool || t == ExecutionStepTypeSubAgent || t == ExecutionStepTypeRetrieval
}

// ExecutionStepStatus describes one persisted step lifecycle.
type ExecutionStepStatus string

const (
	ExecutionStepStatusQueued    ExecutionStepStatus = "queued"
	ExecutionStepStatusRunning   ExecutionStepStatus = "running"
	ExecutionStepStatusCompleted ExecutionStepStatus = "completed"
	ExecutionStepStatusFailed    ExecutionStepStatus = "failed"
	ExecutionStepStatusCancelled ExecutionStepStatus = "cancelled"
)

// Valid reports whether the execution step status is supported by storage.
func (s ExecutionStepStatus) Valid() bool {
	return s == ExecutionStepStatusQueued || s == ExecutionStepStatusRunning || s == ExecutionStepStatusCompleted || s == ExecutionStepStatusFailed || s == ExecutionStepStatusCancelled
}

// Terminal reports whether an execution step has finished.
func (s ExecutionStepStatus) Terminal() bool {
	return s == ExecutionStepStatusCompleted || s == ExecutionStepStatusFailed || s == ExecutionStepStatusCancelled
}

// ExecutionStep records one normalized operation within an execution.
type ExecutionStep struct {
	ID                string              `json:"id"`
	TenantID          string              `json:"tenant_id"`
	ExecutionID       string              `json:"execution_id"`
	ParentStepID      string              `json:"parent_step_id,omitempty"`
	SequenceNo        int                 `json:"sequence_no"`
	StepType          ExecutionStepType   `json:"step_type"`
	Name              string              `json:"name"`
	ModelConnectionID string              `json:"model_connection_id,omitempty"`
	ExternalToolID    string              `json:"external_tool_id,omitempty"`
	Status            ExecutionStepStatus `json:"status"`
	InputSummary      map[string]any      `json:"input_summary"`
	OutputSummary     map[string]any      `json:"output_summary"`
	InputTokens       int64               `json:"input_tokens"`
	CachedTokens      int64               `json:"cached_tokens"`
	OutputTokens      int64               `json:"output_tokens"`
	StartedAt         *time.Time          `json:"started_at,omitempty"`
	CompletedAt       *time.Time          `json:"completed_at,omitempty"`
	ErrorCode         string              `json:"error_code,omitempty"`
	CreatedAt         time.Time           `json:"created_at"`
}

// TotalTokens returns step token usage without adding a non-schema persisted field.
func (s ExecutionStep) TotalTokens() int64 {
	return s.InputTokens + s.OutputTokens
}

// ConversationFileState describes whether an uploaded asset is still a draft or attached to context.
type ConversationFileState string

const (
	ConversationFileStateDraft    ConversationFileState = "draft"
	ConversationFileStateAttached ConversationFileState = "attached"
)

// Valid reports whether the conversation file state is supported by storage.
func (s ConversationFileState) Valid() bool {
	return s == ConversationFileStateDraft || s == ConversationFileStateAttached
}

// ConversationFile binds an existing file asset to one conversation segment.
// When State is attached, MessageID and Ordinal record the single owning message.
type ConversationFile struct {
	ID             string                `json:"id"`
	TenantID       string                `json:"tenant_id"`
	ConversationID string                `json:"conversation_id"`
	SegmentID      string                `json:"segment_id"`
	FileAssetID    string                `json:"file_asset_id"`
	MessageID      string                `json:"message_id,omitempty"`
	Ordinal        *int                  `json:"ordinal,omitempty"`
	State          ConversationFileState `json:"state"`
	CreatedAt      time.Time             `json:"created_at"`
	UpdatedAt      time.Time             `json:"updated_at"`
}

// MemoryScope identifies the retrieval boundary of a memory.
type MemoryScope string

const (
	MemoryScopeGlobal       MemoryScope = "global"
	MemoryScopeAgent        MemoryScope = "agent"
	MemoryScopeConversation MemoryScope = "conversation"
)

// Valid reports whether the memory scope is supported by storage.
func (s MemoryScope) Valid() bool {
	return s == MemoryScopeGlobal || s == MemoryScopeAgent || s == MemoryScopeConversation
}

// MemorySource identifies whether a memory was manually entered or extracted from a message.
type MemorySource string

const (
	MemorySourceManual    MemorySource = "manual"
	MemorySourceExtracted MemorySource = "extracted"
)

// Valid reports whether the memory source is supported by storage.
func (s MemorySource) Valid() bool {
	return s == MemorySourceManual || s == MemorySourceExtracted
}

// MemoryStatus describes whether a memory participates in retrieval.
type MemoryStatus string

const (
	MemoryStatusActive     MemoryStatus = "active"
	MemoryStatusSuperseded MemoryStatus = "superseded"
)

// Valid reports whether the memory status is supported by storage.
func (s MemoryStatus) Valid() bool {
	return s == MemoryStatusActive || s == MemoryStatusSuperseded
}

// Memory is a scoped, attributable user memory.
type Memory struct {
	ID              string       `json:"id"`
	TenantID        string       `json:"tenant_id"`
	AccountID       string       `json:"account_id"`
	ScopeType       MemoryScope  `json:"scope_type"`
	AgentID         string       `json:"agent_id,omitempty"`
	ConversationID  string       `json:"conversation_id,omitempty"`
	SegmentID       string       `json:"segment_id,omitempty"`
	Key             string       `json:"key"`
	Content         string       `json:"content"`
	SourceType      MemorySource `json:"source_type"`
	SourceMessageID string       `json:"source_message_id,omitempty"`
	Confidence      float64      `json:"confidence"`
	Importance      int          `json:"importance"`
	Status          MemoryStatus `json:"status"`
	ExpiresAt       *time.Time   `json:"expires_at,omitempty"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

// Validate checks memory scope and ranking constraints.
func (m Memory) Validate() error {
	fields := make([]FieldError, 0, 10)
	agentV2Required(&fields, "id", m.ID)
	agentV2Required(&fields, "tenant_id", m.TenantID)
	agentV2Required(&fields, "account_id", m.AccountID)
	agentV2Required(&fields, "key", m.Key)
	if !m.ScopeType.Valid() {
		fields = append(fields, agentV2Invalid("scope_type", "scope_type must be global, agent, or conversation"))
	} else {
		switch m.ScopeType {
		case MemoryScopeGlobal:
			if m.AgentID != "" || m.ConversationID != "" || m.SegmentID != "" {
				fields = append(fields, agentV2Invalid("scope_type", "global memory cannot reference an agent, conversation, or segment"))
			}
		case MemoryScopeAgent:
			if strings.TrimSpace(m.AgentID) == "" {
				fields = append(fields, agentV2RequiredError("agent_id"))
			}
			if m.ConversationID != "" || m.SegmentID != "" {
				fields = append(fields, agentV2Invalid("scope_type", "agent memory cannot reference a conversation or segment"))
			}
		case MemoryScopeConversation:
			if strings.TrimSpace(m.ConversationID) == "" || strings.TrimSpace(m.SegmentID) == "" {
				fields = append(fields, agentV2Invalid("scope_type", "conversation memory requires conversation_id and segment_id"))
			}
			if m.AgentID != "" {
				fields = append(fields, agentV2Invalid("agent_id", "conversation memory cannot reference agent_id"))
			}
		}
	}
	if !m.SourceType.Valid() {
		fields = append(fields, agentV2Invalid("source_type", "source_type must be manual or extracted"))
	}
	if !m.Status.Valid() {
		fields = append(fields, agentV2Invalid("status", "status must be active or superseded"))
	}
	if m.Confidence < 0 || m.Confidence > 1 {
		fields = append(fields, agentV2Invalid("confidence", "confidence must be between 0 and 1"))
	}
	if m.Importance < 1 || m.Importance > 5 {
		fields = append(fields, agentV2Invalid("importance", "importance must be between 1 and 5"))
	}
	return agentV2ValidationResult("memory is invalid", fields)
}

// ActiveAt reports whether a memory is eligible for retrieval at the supplied time.
func (m Memory) ActiveAt(now time.Time) bool {
	return m.Status == MemoryStatusActive && (m.ExpiresAt == nil || m.ExpiresAt.After(now))
}

// AgentConfirmationStatus describes durable one-time confirmation processing.
type AgentConfirmationStatus string

const (
	AgentConfirmationStatusPending   AgentConfirmationStatus = "pending"
	AgentConfirmationStatusExecuting AgentConfirmationStatus = "executing"
	AgentConfirmationStatusCompleted AgentConfirmationStatus = "completed"
	AgentConfirmationStatusFailed    AgentConfirmationStatus = "failed"
	AgentConfirmationStatusCancelled AgentConfirmationStatus = "cancelled"
	AgentConfirmationStatusExpired   AgentConfirmationStatus = "expired"
)

// Valid reports whether the confirmation status is supported by storage.
func (s AgentConfirmationStatus) Valid() bool {
	return s == AgentConfirmationStatusPending || s == AgentConfirmationStatusExecuting || s == AgentConfirmationStatusCompleted || s == AgentConfirmationStatusFailed || s == AgentConfirmationStatusCancelled || s == AgentConfirmationStatusExpired
}

// Terminal reports whether a confirmation can no longer execute.
func (s AgentConfirmationStatus) Terminal() bool {
	return s == AgentConfirmationStatusCompleted || s == AgentConfirmationStatusFailed || s == AgentConfirmationStatusCancelled || s == AgentConfirmationStatusExpired
}

// CanTransitionTo enforces single-consumer confirmation processing.
func (s AgentConfirmationStatus) CanTransitionTo(next AgentConfirmationStatus) bool {
	if s == next {
		return true
	}
	switch s {
	case AgentConfirmationStatusPending:
		return next == AgentConfirmationStatusExecuting || next == AgentConfirmationStatusCancelled || next == AgentConfirmationStatusExpired
	case AgentConfirmationStatusExecuting:
		return next == AgentConfirmationStatusPending || next == AgentConfirmationStatusCompleted || next == AgentConfirmationStatusFailed || next == AgentConfirmationStatusCancelled || next == AgentConfirmationStatusExpired
	default:
		return false
	}
}

// AgentConfirmationRecord persists confirmation display data separately from protected action input.
type AgentConfirmationRecord struct {
	ID              string                  `json:"id"`
	TenantID        string                  `json:"tenant_id"`
	AccountID       string                  `json:"account_id"`
	ConversationID  string                  `json:"conversation_id"`
	SegmentID       string                  `json:"segment_id"`
	ExecutionID     string                  `json:"execution_id,omitempty"`
	SourceMessageID string                  `json:"source_message_id,omitempty"`
	Kind            string                  `json:"kind"`
	Title           string                  `json:"title"`
	Action          string                  `json:"action"`
	PublicPayload   map[string]any          `json:"public_payload"`
	ActionPayload   map[string]any          `json:"-"`
	ResultPayload   map[string]any          `json:"result_payload"`
	Status          AgentConfirmationStatus `json:"status"`
	LastError       string                  `json:"last_error,omitempty"`
	ExpiresAt       time.Time               `json:"expires_at"`
	ConsumedAt      *time.Time              `json:"consumed_at,omitempty"`
	CreatedAt       time.Time               `json:"created_at"`
	UpdatedAt       time.Time               `json:"updated_at"`
}

// ClaimableAt reports whether a pending confirmation may be atomically moved to executing.
func (c AgentConfirmationRecord) ClaimableAt(now time.Time) bool {
	return c.Status == AgentConfirmationStatusPending && now.Before(c.ExpiresAt)
}

// Validate checks confirmation ownership, context binding, state, and expiry.
func (c AgentConfirmationRecord) Validate() error {
	fields := make([]FieldError, 0, 11)
	agentV2Required(&fields, "id", c.ID)
	agentV2Required(&fields, "tenant_id", c.TenantID)
	agentV2Required(&fields, "account_id", c.AccountID)
	agentV2Required(&fields, "conversation_id", c.ConversationID)
	agentV2Required(&fields, "segment_id", c.SegmentID)
	agentV2Required(&fields, "kind", c.Kind)
	agentV2Required(&fields, "title", c.Title)
	agentV2Required(&fields, "action", c.Action)
	if !c.Status.Valid() {
		fields = append(fields, agentV2Invalid("status", "confirmation status is invalid"))
	}
	if c.ExpiresAt.IsZero() {
		fields = append(fields, agentV2RequiredError("expires_at"))
	}
	return agentV2ValidationResult("agent confirmation is invalid", fields)
}

func externalToolHTTPMethodValid(method string) bool {
	switch strings.TrimSpace(method) {
	case "", "GET", "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}

func agentV2BindingOwnerValid(revision AgentRevision, tenantID, revisionID string) bool {
	return tenantID == revision.TenantID && revisionID == revision.ID
}

func agentV2Required(fields *[]FieldError, name, value string) {
	if strings.TrimSpace(value) == "" {
		*fields = append(*fields, agentV2RequiredError(name))
	}
}

func agentV2RequiredError(field string) FieldError {
	return FieldError{Field: field, Code: "required", Message: field + " is required"}
}

func agentV2Invalid(field, message string) FieldError {
	return FieldError{Field: field, Code: "invalid", Message: message}
}

func agentV2ValidationResult(message string, fields []FieldError) error {
	if len(fields) == 0 {
		return nil
	}
	return ValidationFailed(message, fields)
}

func agentV2CategoryValid(category AgentCategory) bool {
	return category == AgentCategoryWorkflow || category == AgentCategoryDoc || category == AgentCategoryAnalytics || category == AgentCategoryIT
}

func agentV2VisibilityValid(visibility AgentVisibility) bool {
	return visibility == AgentVisibilityAll || visibility == AgentVisibilityDepartment || visibility == AgentVisibilityRole
}
