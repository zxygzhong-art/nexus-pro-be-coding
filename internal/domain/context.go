package domain

import "context"

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
