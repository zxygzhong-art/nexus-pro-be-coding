package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

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

	mu       sync.Mutex
	metadata oidcMetadata
	fetched  time.Time
	keys     map[string]*rsa.PublicKey
}

type oidcMetadata struct {
	Issuer   string
	AuthURL  string
	TokenURL string
	JWKSURI  string
}

const oidcMetadataTTL = 10 * time.Minute

// NewOIDCProvider creates a provider for Google, Microsoft, or any OIDC-compatible IdP.
func NewOIDCProvider(cfg OIDCProviderConfig, client *http.Client) *OIDCProvider {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "email", "profile"}
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
func (p *OIDCProvider) AuthorizationURL(state string) (string, error) {
	meta, err := p.metadataFor(context.Background())
	if err != nil {
		return "", err
	}
	values := url.Values{}
	values.Set("response_type", "code")
	values.Set("client_id", p.clientID)
	values.Set("redirect_uri", p.redirectURL)
	values.Set("scope", strings.Join(p.scopes, " "))
	values.Set("state", state)
	return meta.AuthURL + "?" + values.Encode(), nil
}

// ResolveCallback exchanges an authorization code and validates the returned ID token.
func (p *OIDCProvider) ResolveCallback(ctx context.Context, code string) (domain.AuthenticatedPrincipal, error) {
	meta, err := p.metadataFor(ctx)
	if err != nil {
		return domain.AuthenticatedPrincipal{}, err
	}
	idToken, err := p.exchangeCode(ctx, meta.TokenURL, code)
	if err != nil {
		return domain.AuthenticatedPrincipal{}, err
	}
	claims, err := p.verifyIDToken(ctx, meta, idToken)
	if err != nil {
		return domain.AuthenticatedPrincipal{}, domain.Unauthorized("invalid OIDC id token")
	}
	return tokenPrincipalFromClaims(p.code, claims), nil
}

func (p *OIDCProvider) metadataFor(ctx context.Context) (oidcMetadata, error) {
	p.mu.Lock()
	if p.metadata.Issuer != "" && time.Since(p.fetched) < oidcMetadataTTL {
		meta := p.metadata
		p.mu.Unlock()
		return meta, nil
	}
	p.mu.Unlock()

	if p.issuerURL == "" || p.clientID == "" || p.clientSecret == "" || p.redirectURL == "" {
		return oidcMetadata{}, errors.New("OIDC provider is not fully configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.issuerURL+"/.well-known/openid-configuration", nil)
	if err != nil {
		return oidcMetadata{}, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return oidcMetadata{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return oidcMetadata{}, errors.New("OIDC discovery failed")
	}
	var body struct {
		Issuer   string `json:"issuer"`
		AuthURL  string `json:"authorization_endpoint"`
		TokenURL string `json:"token_endpoint"`
		JWKSURI  string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return oidcMetadata{}, err
	}
	if body.Issuer == "" || body.AuthURL == "" || body.TokenURL == "" || body.JWKSURI == "" {
		return oidcMetadata{}, errors.New("invalid OIDC discovery document")
	}
	meta := oidcMetadata{Issuer: strings.TrimRight(body.Issuer, "/"), AuthURL: body.AuthURL, TokenURL: body.TokenURL, JWKSURI: body.JWKSURI}
	p.mu.Lock()
	p.metadata = meta
	p.fetched = time.Now()
	p.mu.Unlock()
	return meta, nil
}

func (p *OIDCProvider) exchangeCode(ctx context.Context, tokenURL, code string) (string, error) {
	values := url.Values{}
	values.Set("grant_type", "authorization_code")
	values.Set("code", strings.TrimSpace(code))
	values.Set("redirect_uri", p.redirectURL)
	values.Set("client_id", p.clientID)
	values.Set("client_secret", p.clientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", domain.Unauthorized("OIDC code exchange failed")
	}
	var body struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return "", err
	}
	if strings.TrimSpace(body.IDToken) == "" {
		return "", errors.New("OIDC token response missing id_token")
	}
	return body.IDToken, nil
}

func (p *OIDCProvider) verifyIDToken(ctx context.Context, meta oidcMetadata, token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("jwt must have three parts")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, err
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, err
	}
	if header.Alg != "RS256" || header.Kid == "" {
		return nil, errors.New("unsupported jwt header")
	}
	keys, err := p.keysFor(ctx, meta.JWKSURI)
	if err != nil {
		return nil, err
	}
	key := keys[header.Kid]
	if key == nil {
		return nil, errors.New("jwk not found")
	}
	signingInput := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, err
	}
	if err := verifyRS256Signature(key, signingInput, signature); err != nil {
		return nil, err
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var claims map[string]any
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, err
	}
	if err := p.validateClaims(meta, claims); err != nil {
		return nil, err
	}
	return claims, nil
}

func (p *OIDCProvider) keysFor(ctx context.Context, uri string) (map[string]*rsa.PublicKey, error) {
	p.mu.Lock()
	if len(p.keys) > 0 && time.Since(p.fetched) < oidcMetadataTTL {
		keys := p.keys
		p.mu.Unlock()
		return keys, nil
	}
	p.mu.Unlock()

	keys, err := p.fetchJWKS(ctx, uri)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.keys = keys
	p.mu.Unlock()
	return keys, nil
}

func (p *OIDCProvider) fetchJWKS(ctx context.Context, uri string) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("jwks fetch failed")
	}
	var body struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	keys := map[string]*rsa.PublicKey{}
	for _, raw := range body.Keys {
		key, err := raw.rsaPublicKey()
		if err == nil && raw.Kid != "" {
			keys[raw.Kid] = key
		}
	}
	if len(keys) == 0 {
		return nil, errors.New("no rsa keys in jwks")
	}
	return keys, nil
}

func (p *OIDCProvider) validateClaims(meta oidcMetadata, claims map[string]any) error {
	if !oidcIssuerMatches(meta.Issuer, claimString(claims, "iss"), claims) {
		return errors.New("issuer mismatch")
	}
	if !audienceContains(claims["aud"], p.clientID) {
		return errors.New("audience mismatch")
	}
	now := time.Now().Unix()
	if exp := claimUnix(claims["exp"]); exp == 0 || exp <= now {
		return errors.New("token expired")
	}
	if nbf := claimUnix(claims["nbf"]); nbf > now {
		return errors.New("token not yet valid")
	}
	if claimString(claims, "sub") == "" {
		return errors.New("missing subject claim")
	}
	return nil
}

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

func verifyRS256Signature(key crypto.PublicKey, signingInput string, signature []byte) error {
	rsaKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return errors.New("not rsa")
	}
	sum := sha256.Sum256([]byte(signingInput))
	return rsa.VerifyPKCS1v15(rsaKey, crypto.SHA256, sum[:], signature)
}
