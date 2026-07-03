package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"nexus-pro-be/internal/domain"
)

// OIDCProviderConfig configures one external authorization-code provider.
type OIDCProviderConfig struct {
	Code         string
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
}

// OIDCProvider exchanges authorization codes and validates provider ID tokens.
type OIDCProvider struct {
	code         string
	issuerURL    string
	clientID     string
	clientSecret string
	redirectURL  string
	scopes       []string
	client       *http.Client

	mu              sync.Mutex
	provider        *oidc.Provider
	discoveryIssuer string
}

// NewOIDCProvider creates a provider for Google, Microsoft, or any OIDC-compatible IdP.
func NewOIDCProvider(cfg OIDCProviderConfig, client *http.Client) *OIDCProvider {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "email", "profile"}
	}
	return &OIDCProvider{
		code:         strings.TrimSpace(cfg.Code),
		issuerURL:    strings.TrimRight(strings.TrimSpace(cfg.IssuerURL), "/"),
		clientID:     strings.TrimSpace(cfg.ClientID),
		clientSecret: cfg.ClientSecret,
		redirectURL:  strings.TrimSpace(cfg.RedirectURL),
		scopes:       scopes,
		client:       client,
	}
}

// AuthorizationURL returns the provider redirect URL.
func (p *OIDCProvider) AuthorizationURL(ctx context.Context, state string) (string, error) {
	provider, err := p.providerFor(ctx)
	if err != nil {
		return "", err
	}
	return p.oauth2Config(provider).AuthCodeURL(state), nil
}

// ResolveCallback exchanges an authorization code and validates the returned ID token.
func (p *OIDCProvider) ResolveCallback(ctx context.Context, code string) (domain.AuthenticatedPrincipal, error) {
	provider, err := p.providerFor(ctx)
	if err != nil {
		return domain.AuthenticatedPrincipal{}, err
	}
	ctx = p.clientContext(ctx)
	token, err := p.oauth2Config(provider).Exchange(ctx, strings.TrimSpace(code))
	if err != nil {
		return domain.AuthenticatedPrincipal{}, domain.Unauthorized("OIDC code exchange failed")
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || strings.TrimSpace(rawIDToken) == "" {
		return domain.AuthenticatedPrincipal{}, errors.New("OIDC token response missing id_token")
	}

	var claims map[string]any
	verifier := provider.VerifierContext(ctx, &oidc.Config{
		ClientID:        p.clientID,
		SkipIssuerCheck: p.usesTemplateIssuer(),
	})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return domain.AuthenticatedPrincipal{}, domain.Unauthorized("invalid OIDC id token")
	}
	if err := idToken.Claims(&claims); err != nil {
		return domain.AuthenticatedPrincipal{}, err
	}
	if p.usesTemplateIssuer() && !oidcIssuerMatches(p.cachedDiscoveryIssuer(), claimString(claims, "iss"), claims) {
		return domain.AuthenticatedPrincipal{}, domain.Unauthorized("invalid OIDC id token")
	}
	if claimString(claims, "sub") == "" {
		return domain.AuthenticatedPrincipal{}, domain.Unauthorized("invalid OIDC id token")
	}
	return tokenPrincipalFromClaims(p.code, claims), nil
}

// providerFor discovers and caches the upstream OIDC provider metadata.
func (p *OIDCProvider) providerFor(ctx context.Context) (*oidc.Provider, error) {
	p.mu.Lock()
	if p.provider != nil {
		provider := p.provider
		p.mu.Unlock()
		return provider, nil
	}
	p.mu.Unlock()

	if p.issuerURL == "" || p.clientID == "" || p.clientSecret == "" || p.redirectURL == "" {
		return nil, errors.New("OIDC provider is not fully configured")
	}
	ctx = p.clientContext(ctx)
	if p.usesTemplateIssuer() {
		ctx = oidc.InsecureIssuerURLContext(ctx, p.issuerURL)
	}
	provider, err := oidc.NewProvider(ctx, p.issuerURL)
	if err != nil {
		return nil, err
	}
	discoveryIssuer := p.issuerURL
	var discovery struct {
		Issuer string `json:"issuer"`
	}
	if err := provider.Claims(&discovery); err == nil && strings.TrimSpace(discovery.Issuer) != "" {
		discoveryIssuer = strings.TrimRight(strings.TrimSpace(discovery.Issuer), "/")
	}

	p.mu.Lock()
	if p.provider == nil {
		p.provider = provider
		p.discoveryIssuer = discoveryIssuer
	}
	provider = p.provider
	p.mu.Unlock()
	return provider, nil
}

// oauth2Config builds the authorization-code client from discovered endpoints.
func (p *OIDCProvider) oauth2Config(provider *oidc.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     p.clientID,
		ClientSecret: p.clientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  p.redirectURL,
		Scopes:       p.scopes,
	}
}

// clientContext makes go-oidc and x/oauth2 share the configured HTTP client.
func (p *OIDCProvider) clientContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return oidc.ClientContext(ctx, p.client)
}

// cachedDiscoveryIssuer returns the provider-reported issuer used for manual checks.
func (p *OIDCProvider) cachedDiscoveryIssuer() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if strings.TrimSpace(p.discoveryIssuer) != "" {
		return p.discoveryIssuer
	}
	return p.issuerURL
}

// usesTemplateIssuer identifies providers whose discovery issuer contains tenant placeholders.
func (p *OIDCProvider) usesTemplateIssuer() bool {
	return strings.EqualFold(p.code, "microsoft")
}

// oidcIssuerMatches validates provider issuer templates after go-oidc verifies the token.
func oidcIssuerMatches(want, got string, claims map[string]any) bool {
	want = strings.TrimRight(strings.TrimSpace(want), "/")
	got = strings.TrimRight(strings.TrimSpace(got), "/")
	if want == got {
		return true
	}
	if strings.Contains(want, "{tenantid}") {
		tid := claimString(claims, "tid")
		return tid != "" && strings.ReplaceAll(want, "{tenantid}", tid) == got
	}
	return false
}
