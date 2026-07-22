package v1

import (
	"net/http"
	"strings"

	"nexus-pro-api/internal/domain"
)

// requestContext 處理請求 context。
func (a *API) requestContext(r *http.Request) (domain.RequestContext, error) {
	var tenantID, accountID string
	var platformAdmin bool
	// 由 token 推導出的身分是 request context 唯一可信來源。
	tokenCtx, ok, err := a.tokenResolver.Resolve(r)
	if err != nil {
		return domain.RequestContext{}, err
	}
	if ok {
		platformAdmin = AuthenticatedPlatformAdmin(tokenCtx)
		if a.identity != nil {
			resolved, err := a.identity.ResolveAuthenticatedPrincipal(r.Context(), tokenCtx)
			if err != nil {
				return domain.RequestContext{}, err
			}
			tenantID = resolved.TenantID
			accountID = resolved.AccountID
		} else {
			tenantID = tokenCtx.TenantID
			accountID = tokenCtx.AccountID
		}
	}
	if tenantID == "" {
		return domain.RequestContext{}, domain.Unauthorized("authenticated tenant context is required")
	}
	if accountID == "" {
		return domain.RequestContext{}, domain.Unauthorized("authenticated account context is required")
	}
	requestID := requestIDFrom(r)
	traceID, spanID := traceContextIDs(r)
	if traceID == "" {
		traceID = requestID
	}
	return domain.RequestContext{
		Context:              r.Context(),
		TenantID:             tenantID,
		AccountID:            accountID,
		PlatformAdmin:        platformAdmin,
		AssumedRoleSessionID: strings.TrimSpace(r.Header.Get("X-Assumable-Role-Session-ID")),
		RequestID:            requestID,
		IdempotencyKey:       strings.TrimSpace(r.Header.Get("Idempotency-Key")),
		TraceID:              traceID,
		SpanID:               spanID,
	}, nil
}

// AuthenticatedPlatformAdmin reports whether the authenticated principal has platform-wide administration authority.
func AuthenticatedPlatformAdmin(principal domain.AuthenticatedPrincipal) bool {
	if value, ok := principal.Claims["platform_admin"].(bool); ok && value {
		return true
	}
	realmAccess, ok := principal.Claims["realm_access"].(map[string]any)
	if !ok {
		return false
	}
	roles, ok := realmAccess["roles"].([]any)
	if !ok {
		return false
	}
	for _, role := range roles {
		if value, ok := role.(string); ok && strings.TrimSpace(value) == "nexus-platform-admin" {
			return true
		}
	}
	return false
}
