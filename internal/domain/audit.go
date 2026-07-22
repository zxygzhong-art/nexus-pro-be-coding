package domain

import "time"

// AuditLog 定義稽覈 log 的資料結構。
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

// OutboxAggregateAuthz 標記授權相關的 outbox 事件,dispatcher 依此路由。
const OutboxAggregateAuthz = "authz"

// Outbox 狀態與預設重試上限是 producer、dispatcher 與管理 API 的共用契約。
const (
	OutboxStatusPending      = "pending"
	OutboxStatusProcessing   = "processing"
	OutboxStatusSucceeded    = "succeeded"
	OutboxStatusFailed       = "failed"
	OutboxStatusParked       = "parked"
	OutboxStatusDeadLettered = "dead_lettered"

	DefaultOutboxMaxAttempts = 5
)

// OutboxEvent 定義 outbox 事件的資料結構。
type OutboxEvent struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id"`
	EventType      string         `json:"event_type"`
	AggregateType  string         `json:"aggregate_type,omitempty"`
	AggregateID    string         `json:"aggregate_id,omitempty"`
	Payload        map[string]any `json:"payload,omitempty"`
	PayloadVersion int            `json:"payload_version"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	Status         string         `json:"status"`
	RetryCount     int            `json:"retry_count"`
	AttemptCount   int            `json:"attempt_count"`
	MaxAttempts    *int           `json:"max_attempts,omitempty"`
	LastError      string         `json:"last_error,omitempty"`
	NextAttemptAt  time.Time      `json:"next_attempt_at"`
	ClaimOwner     string         `json:"claim_owner,omitempty"`
	ClaimToken     string         `json:"-"`
	ClaimExpiresAt *time.Time     `json:"claim_expires_at,omitempty"`
	LastAttemptAt  *time.Time     `json:"last_attempt_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	ProcessedAt    *time.Time     `json:"processed_at,omitempty"`
	DeadLetteredAt *time.Time     `json:"dead_lettered_at,omitempty"`
}
