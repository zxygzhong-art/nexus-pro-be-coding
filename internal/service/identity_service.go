package service

import (
	"context"
	"strings"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

type ssoAccountCandidate struct {
	Tenant  domain.Tenant
	Account domain.Account
}

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
	provider := strings.TrimSpace(principal.Provider)
	subject := strings.TrimSpace(principal.Subject)
	if provider == "" || subject == "" {
		return IdentityResolution{}, domain.Unauthorized("external identity provider and subject are required")
	}
	tenantID := strings.TrimSpace(utils.FirstNonEmpty(principal.TenantID, principal.TenantHint))
	if tenantID == "" {
		return c.resolveIdentityBySubject(ctx, provider, subject)
	}
	identity, ok, err := c.store.GetUserIdentity(ctx, tenantID, provider, subject)
	if err != nil {
		return IdentityResolution{}, err
	}
	if ok {
		return IdentityResolution{TenantID: identity.TenantID, AccountID: identity.AccountID, Identity: &identity}, nil
	}
	return IdentityResolution{}, domain.UnauthorizedReason("identity_not_linked", "external identity is not linked to a local account")
}

// VerifyGoogleSSOLogin 驗證 Google SSO principal 並建立本機身分綁定。
func (c IdentityService) VerifyGoogleSSOLogin(ctx context.Context, principal AuthenticatedPrincipal) (domain.SSOLoginVerification, error) {
	provider := strings.TrimSpace(principal.Provider)
	if provider == "" {
		provider = domain.IdentityProviderKeycloak
	}
	subject := strings.TrimSpace(principal.Subject)
	if subject == "" {
		return domain.SSOLoginVerification{}, domain.UnauthorizedReason("google_login_failed", "Google login failed")
	}
	email := strings.ToLower(strings.TrimSpace(principal.Email))
	if email == "" {
		return domain.SSOLoginVerification{}, domain.UnauthorizedReason("google_login_failed", "Google login failed")
	}
	if !identityClaimBool(principal.Claims, "email_verified") {
		return domain.SSOLoginVerification{}, domain.UnauthorizedReason("sso_email_unverified", "Google account email is not verified")
	}

	candidate, err := c.resolveSSOAccountByEmail(ctx, strings.TrimSpace(utils.FirstNonEmpty(principal.TenantID, principal.TenantHint)), email)
	if err != nil {
		return domain.SSOLoginVerification{}, err
	}
	if candidate.Account.Status != string(domain.AccountStatusActive) {
		return domain.SSOLoginVerification{}, domain.UnauthorizedReason("account_inactive", "account is not active")
	}
	if strings.TrimSpace(candidate.Tenant.ID) == "" {
		return domain.SSOLoginVerification{}, domain.UnauthorizedReason("company_inactive", "company is not active")
	}
	if err := c.bindSSOIdentity(ctx, candidate.Account, provider, subject, email); err != nil {
		return domain.SSOLoginVerification{}, err
	}
	return domain.SSOLoginVerification{
		Provider:  provider,
		TenantID:  candidate.Account.TenantID,
		AccountID: candidate.Account.ID,
		Email:     email,
	}, nil
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
		return IdentityResolution{}, domain.UnauthorizedReason("identity_not_linked", "external identity is not linked to a local account")
	}
	return IdentityResolution{TenantID: identity.TenantID, AccountID: identity.AccountID, Identity: &identity}, nil
}

// resolveIdentityBySubject 透過外部 subject 反查唯一的本機身分綁定。
func (c IdentityService) resolveIdentityBySubject(ctx context.Context, provider, subject string) (IdentityResolution, error) {
	tenants, err := c.store.ListTenants(ctx)
	if err != nil {
		return IdentityResolution{}, err
	}
	var found *domain.UserIdentity
	for _, tenant := range tenants {
		identity, ok, err := c.store.GetUserIdentity(ctx, tenant.ID, provider, subject)
		if err != nil {
			return IdentityResolution{}, err
		}
		if !ok {
			continue
		}
		if found != nil {
			return IdentityResolution{}, domain.UnauthorizedReason("sso_identity_conflict", "external identity is linked to multiple local accounts")
		}
		next := identity
		found = &next
	}
	if found == nil {
		return IdentityResolution{}, domain.UnauthorizedReason("identity_not_linked", "external identity is not linked to a local account")
	}
	return IdentityResolution{TenantID: found.TenantID, AccountID: found.AccountID, Identity: found}, nil
}

// resolveSSOAccountByEmail 尋找符合 Google email 的唯一 active-eligible Nexus 帳號。
func (c IdentityService) resolveSSOAccountByEmail(ctx context.Context, tenantHint, email string) (ssoAccountCandidate, error) {
	if tenantHint != "" {
		tenant, ok, err := c.store.GetTenant(ctx, tenantHint)
		if err != nil {
			return ssoAccountCandidate{}, err
		}
		if !ok {
			return ssoAccountCandidate{}, domain.UnauthorizedReason("company_inactive", "company is not active")
		}
		account, ok, err := c.findAccountByEmailInTenant(ctx, tenant.ID, email)
		if err != nil {
			return ssoAccountCandidate{}, err
		}
		if !ok {
			return ssoAccountCandidate{}, domain.UnauthorizedReason("sso_email_not_authorized", "Google account is not authorized to use Nexus")
		}
		return ssoAccountCandidate{Tenant: tenant, Account: account}, nil
	}

	tenants, err := c.store.ListTenants(ctx)
	if err != nil {
		return ssoAccountCandidate{}, err
	}
	var found *ssoAccountCandidate
	for _, tenant := range tenants {
		account, ok, err := c.findAccountByEmailInTenant(ctx, tenant.ID, email)
		if err != nil {
			return ssoAccountCandidate{}, err
		}
		if !ok {
			continue
		}
		if found != nil {
			return ssoAccountCandidate{}, domain.UnauthorizedReason("sso_email_ambiguous", "Google account matches multiple Nexus accounts")
		}
		next := ssoAccountCandidate{Tenant: tenant, Account: account}
		found = &next
	}
	if found == nil {
		return ssoAccountCandidate{}, domain.UnauthorizedReason("sso_email_not_authorized", "Google account is not authorized to use Nexus")
	}
	return *found, nil
}

// findAccountByEmailInTenant 從單一租戶中以 email 比對帳號。
func (c IdentityService) findAccountByEmailInTenant(ctx context.Context, tenantID, email string) (domain.Account, bool, error) {
	accounts, err := c.store.ListAccounts(ctx, tenantID)
	if err != nil {
		return domain.Account{}, false, err
	}
	for _, account := range accounts {
		if strings.EqualFold(strings.TrimSpace(account.Email), email) {
			return account, true, nil
		}
	}
	return domain.Account{}, false, nil
}

// bindSSOIdentity 將通過 email 校驗的 Google/Keycloak subject 綁到本機帳號。
func (c IdentityService) bindSSOIdentity(ctx context.Context, account domain.Account, provider, subject, email string) error {
	existing, ok, err := c.store.GetUserIdentity(ctx, account.TenantID, provider, subject)
	if err != nil {
		return err
	}
	if ok && existing.AccountID != account.ID {
		return domain.UnauthorizedReason("sso_identity_conflict", "external identity is already linked to another local account")
	}
	identityID := utils.NewID("uid")
	createdAt := c.Now()
	if ok {
		identityID = existing.ID
		createdAt = existing.CreatedAt
	}
	return c.store.UpsertUserIdentity(ctx, domain.UserIdentity{
		ID:        identityID,
		TenantID:  account.TenantID,
		AccountID: account.ID,
		Provider:  provider,
		Subject:   subject,
		Email:     email,
		CreatedAt: createdAt,
	})
}

// identityClaimBool 讀取 OIDC claims 中的布林值。
func identityClaimBool(claims map[string]any, keys ...string) bool {
	for _, key := range keys {
		switch value := claims[key].(type) {
		case bool:
			return value
		case string:
			return strings.EqualFold(strings.TrimSpace(value), "true")
		}
	}
	return false
}
