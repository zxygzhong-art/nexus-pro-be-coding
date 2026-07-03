package service

import (
	"context"
	"strings"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

// IdentityService resolves external authenticated principals to local accounts.
type IdentityService struct {
	*Service
	store identityStore
}

// Identity returns the external identity mapping facade.
func (c *Service) Identity() IdentityService {
	return IdentityService{Service: c, store: c.store}
}

// ResolveAuthenticatedPrincipal maps provider/subject identity to a local tenant/account.
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

// ResolveBoundAuthenticatedPrincipal requires a pre-existing provider/subject binding.
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
