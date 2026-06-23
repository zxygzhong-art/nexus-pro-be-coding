package domain

import "context"

// RequestContext carries trusted request identity and approval metadata through services.
type RequestContext struct {
	Context              context.Context
	TenantID             string
	AccountID            string
	AssumedRoleID        string
	AssumedRoleSessionID string
	RequestID            string
	ApprovalConfirmed    bool
	ApprovalInstanceID   string
}
