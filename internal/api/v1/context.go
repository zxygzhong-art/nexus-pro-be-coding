package v1

import (
	"net/http"
	"strings"

	"nexus-pro-be/internal/domain"
)

func (a *API) requestContext(r *http.Request) (domain.RequestContext, error) {
	var tenantID, accountID string
	// Token-derived identity wins over configurable demo/header fallbacks.
	tokenCtx, ok, err := a.tokenResolver.Resolve(r)
	if err != nil {
		return domain.RequestContext{}, err
	}
	if ok {
		tenantID = tokenCtx.TenantID
		accountID = tokenCtx.AccountID
	}
	if a.allowHeaderContext {
		if tenantID == "" {
			tenantID = strings.TrimSpace(r.Header.Get("X-Tenant-ID"))
		}
		if accountID == "" {
			accountID = strings.TrimSpace(r.Header.Get("X-Account-ID"))
		}
	}
	if tenantID == "" {
		if !a.allowDemoContext {
			return domain.RequestContext{}, domain.Unauthorized("authenticated tenant context is required")
		}
		tenantID = "demo"
	}
	if accountID == "" {
		if !a.allowDemoContext {
			return domain.RequestContext{}, domain.Unauthorized("authenticated account context is required")
		}
		accountID = "acct-admin"
	}
	return domain.RequestContext{
		Context:              r.Context(),
		TenantID:             tenantID,
		AccountID:            accountID,
		AssumedRoleSessionID: strings.TrimSpace(r.Header.Get("X-Assumable-Role-Session-ID")),
		RequestID:            requestIDFrom(r),
		ApprovalConfirmed:    a.approvalConfirmed(r),
		ApprovalInstanceID:   strings.TrimSpace(r.Header.Get("X-Approval-Instance-ID")),
	}, nil
}
