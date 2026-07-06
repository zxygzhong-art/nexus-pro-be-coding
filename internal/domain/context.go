package domain

import "context"

// RequestContext 定義請求 context 的資料結構。
type RequestContext struct {
	Context              context.Context
	TenantID             string
	AccountID            string
	AssumedRoleID        string
	AssumedRoleSessionID string
	RequestID            string
	TraceID              string
	SpanID               string
	ApprovalConfirmed    bool
	ApprovalInstanceID   string
}
