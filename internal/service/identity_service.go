package service

import (
	"context"
	"strings"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

// IdentityService 定義身分服務的資料結構。
type IdentityService struct {
	*Service
	store identityStore
}

// Identity 處理身分的服務流程。
func (c *Service) Identity() IdentityService {
	return IdentityService{Service: c, store: c.store}
}

// ResolveAuthenticatedPrincipal 解析 authenticated principal 的服務流程。
func (c IdentityService) ResolveAuthenticatedPrincipal(ctx context.Context, principal AuthenticatedPrincipal) (IdentityResolution, error) {
	tenantID := strings.TrimSpace(utils.FirstNonEmpty(principal.TenantID, principal.TenantHint))
	if tenantID == "" {
		return IdentityResolution{}, domain.Unauthorized("authenticated tenant context is required")
	}

	provider := strings.TrimSpace(principal.Provider)
	subject := strings.TrimSpace(principal.Subject)
	if provider == "" || subject == "" {
		return IdentityResolution{}, domain.Unauthorized("external identity provider and subject are required")
	}
	identity, ok, err := c.store.GetUserIdentity(ctx, tenantID, provider, subject)
	if err != nil {
		return IdentityResolution{}, err
	}
	if ok {
		return IdentityResolution{TenantID: identity.TenantID, AccountID: identity.AccountID, Identity: &identity}, nil
	}
	return IdentityResolution{}, domain.Unauthorized("external identity is not linked to a local account")
}

// ResolveBoundAuthenticatedPrincipal 解析 bound authenticated principal 的服務流程。
func (c IdentityService) ResolveBoundAuthenticatedPrincipal(ctx context.Context, principal AuthenticatedPrincipal) (IdentityResolution, error) {
	tenantID := strings.TrimSpace(utils.FirstNonEmpty(principal.TenantID, principal.TenantHint))
	if tenantID == "" {
		return IdentityResolution{}, domain.Unauthorized("authenticated tenant context is required")
	}
	provider := strings.TrimSpace(principal.Provider)
	subject := strings.TrimSpace(principal.Subject)
	if provider == "" || subject == "" {
		return IdentityResolution{}, domain.Unauthorized("external identity provider and subject are required")
	}
	identity, ok, err := c.store.GetUserIdentity(ctx, tenantID, provider, subject)
	if err != nil {
		return IdentityResolution{}, err
	}
	if !ok {
		return IdentityResolution{}, domain.Unauthorized("external identity is not linked to a local account")
	}
	return IdentityResolution{TenantID: identity.TenantID, AccountID: identity.AccountID, Identity: &identity}, nil
}
