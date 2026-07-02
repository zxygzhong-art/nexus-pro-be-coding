package service

import (
	"context"
	"net/url"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
)

// AuthnService owns public authentication flows that end in a local account context.
type AuthnService struct {
	*Service
}

// Authn returns the authentication facade.
func (c *Service) Authn() AuthnService {
	return AuthnService{Service: c}
}

// OIDCAuthorizationURL builds a provider authorization URL with signed callback state.
func (c AuthnService) OIDCAuthorizationURL(ctx context.Context, provider string, input OIDCAuthorizationInput) (OIDCAuthorizationResponse, error) {
	provider = strings.TrimSpace(provider)
	oidcProvider, ok := c.oidcProviders[provider]
	if !ok || oidcProvider == nil {
		return OIDCAuthorizationResponse{}, NotFound("oidc provider", provider)
	}
	if c.authStateCodec == nil {
		return OIDCAuthorizationResponse{}, domain.E(503, "service_unavailable", "OIDC state signing is not configured")
	}
	tenantID := strings.TrimSpace(input.TenantID)
	if tenantID == "" {
		return OIDCAuthorizationResponse{}, domain.Unauthorized("tenant id is required for external login")
	}
	returnURL, err := normalizeOIDCReturnURL(input.ReturnURL)
	if err != nil {
		return OIDCAuthorizationResponse{}, err
	}
	state, err := c.authStateCodec.EncodeOIDCState(provider, tenantID, returnURL)
	if err != nil {
		return OIDCAuthorizationResponse{}, err
	}
	authURL, err := oidcProvider.AuthorizationURL(ctx, state)
	if err != nil {
		return OIDCAuthorizationResponse{}, err
	}
	return OIDCAuthorizationResponse{Provider: provider, AuthorizationURL: authURL, State: state}, nil
}

// CompleteOIDCCallback exchanges an authorization code and issues a first-party bearer token.
func (c AuthnService) CompleteOIDCCallback(ctx context.Context, provider, code, state string) (AuthLoginResponse, error) {
	provider = strings.TrimSpace(provider)
	oidcProvider, ok := c.oidcProviders[provider]
	if !ok || oidcProvider == nil {
		return AuthLoginResponse{}, NotFound("oidc provider", provider)
	}
	if c.authStateCodec == nil {
		return AuthLoginResponse{}, domain.E(503, "service_unavailable", "OIDC state signing is not configured")
	}
	if c.authTokenIssuer == nil {
		return AuthLoginResponse{}, domain.E(503, "service_unavailable", "auth token issuer is not configured")
	}
	if strings.TrimSpace(code) == "" {
		return AuthLoginResponse{}, BadRequest("authorization code is required")
	}
	decodedState, err := c.authStateCodec.DecodeOIDCState(state)
	if err != nil {
		return AuthLoginResponse{}, err
	}
	if decodedState.Provider != provider {
		return AuthLoginResponse{}, domain.Unauthorized("OIDC state provider mismatch")
	}
	returnURL, err := normalizeOIDCReturnURL(decodedState.ReturnURL)
	if err != nil {
		return AuthLoginResponse{}, err
	}
	principal, err := oidcProvider.ResolveCallback(ctx, strings.TrimSpace(code))
	if err != nil {
		return AuthLoginResponse{}, err
	}
	if principal.TenantID != "" && principal.TenantID != decodedState.TenantID {
		return AuthLoginResponse{}, domain.Unauthorized("OIDC token tenant mismatch")
	}
	principal.TenantID = decodedState.TenantID
	if principal.TenantHint == "" {
		principal.TenantHint = decodedState.TenantID
	}
	resolution, err := c.Identity().ResolveBoundAuthenticatedPrincipal(ctx, principal)
	if err != nil {
		return AuthLoginResponse{}, err
	}
	if _, _, err := c.resolveAccount(RequestContext{Context: ctx, TenantID: resolution.TenantID, AccountID: resolution.AccountID}); err != nil {
		return AuthLoginResponse{}, err
	}
	token, expiresAt, err := c.authTokenIssuer.IssueToken(resolution, principal)
	if err != nil {
		return AuthLoginResponse{}, err
	}
	expiresIn := int64(time.Until(expiresAt).Seconds())
	if expiresIn < 0 {
		expiresIn = 0
	}
	return AuthLoginResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt.UTC().Format(time.RFC3339),
		ExpiresIn:   expiresIn,
		TenantID:    resolution.TenantID,
		AccountID:   resolution.AccountID,
		Provider:    principal.Provider,
		Subject:     principal.Subject,
		Email:       principal.Email,
		ReturnURL:   returnURL,
	}, nil
}

func normalizeOIDCReturnURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if strings.ContainsAny(raw, "\r\n\t\\") || strings.HasPrefix(raw, "//") {
		return "", BadRequest("return_url must be a same-origin path")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", BadRequest("return_url must be a same-origin path")
	}
	if parsed.IsAbs() || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") {
		return "", BadRequest("return_url must be a same-origin path")
	}
	return parsed.String(), nil
}
