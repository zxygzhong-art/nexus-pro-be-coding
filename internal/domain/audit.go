package domain

import "time"

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
