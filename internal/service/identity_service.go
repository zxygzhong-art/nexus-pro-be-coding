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
	if provider != "" && subject != "" {
		identity, ok, err := c.store.GetUserIdentity(ctx, tenantID, provider, subject)
		if err != nil {
			return IdentityResolution{}, err
		}
		if ok {
			return IdentityResolution{TenantID: identity.TenantID, AccountID: identity.AccountID, Identity: &identity}, nil
		}
		if !allowPrincipalAccountClaimFallback(provider) {
			return IdentityResolution{}, domain.Unauthorized("external identity is not linked to a local account")
		}
	}

	accountID := strings.TrimSpace(principal.AccountID)
	if accountID != "" {
		return IdentityResolution{TenantID: tenantID, AccountID: accountID}, nil
	}
	if provider == "" || subject == "" {
		return IdentityResolution{}, domain.Unauthorized("authenticated account context is required")
	}
	return IdentityResolution{}, domain.Unauthorized("external identity is not linked to a local account")
}

// allowPrincipalAccountClaimFallback limits account-claim trust to first-party or local-dev token families.
func allowPrincipalAccountClaimFallback(provider string) bool {
	switch strings.TrimSpace(provider) {
	case "", "internal", "unsigned_jwt":
		return true
	default:
		return false
	}
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
