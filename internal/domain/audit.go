package domain

import "time"

// AuditLog records an auditable action performed inside a tenant.
type AuditLog struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id"`
	ActorAccountID string         `json:"actor_account_id"`
	Action         string         `json:"action"`
	Resource       string         `json:"resource"`
	Target         string         `json:"target,omitempty"`
	Result         string         `json:"result,omitempty"`
	TraceID        string         `json:"trace_id,omitempty"`
	Severity       string         `json:"severity"`
	Details        map[string]any `json:"details,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

// OutboxEvent records tenant-scoped business events waiting for external delivery.
type OutboxEvent struct {
	ID            string         `json:"id"`
	TenantID      string         `json:"tenant_id"`
	EventType     string         `json:"event_type"`
	AggregateType string         `json:"aggregate_type,omitempty"`
	AggregateID   string         `json:"aggregate_id,omitempty"`
	Payload       map[string]any `json:"payload,omitempty"`
	Status        string         `json:"status"`
	RetryCount    int            `json:"retry_count"`
	LastError     string         `json:"last_error,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	ProcessedAt   *time.Time     `json:"processed_at,omitempty"`
}
