package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"nexus-pro-api/internal/domain"
)

// TokenResolver 定義 token resolver 的行為契約。
type TokenResolver interface {
	Resolve(r *http.Request) (domain.AuthenticatedPrincipal, bool, error)
}

// KeycloakTokenResolver 定義 Keycloak token resolver 的資料結構。
type KeycloakTokenResolver struct {
	issuerURL string
	clientID  string
	provider  string
	client    *http.Client

	mu             sync.Mutex
	jwksURI        string
	jwksKeys       map[string]*rsa.PublicKey
	fetched        time.Time
	lastForceFetch time.Time
	missingKids    map[string]time.Time
}

const (
	jwksCacheTTL            = 10 * time.Minute
	jwksMissingKidTTL       = 1 * time.Minute
	jwksForcedRefreshWindow = 30 * time.Second
)

// NewKeycloakTokenResolver 建立 Keycloak token resolver。
func NewKeycloakTokenResolver(issuerURL, clientID string, client *http.Client) *KeycloakTokenResolver {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &KeycloakTokenResolver{
		issuerURL:   strings.TrimRight(strings.TrimSpace(issuerURL), "/"),
		clientID:    strings.TrimSpace(clientID),
		provider:    "keycloak",
		client:      client,
		missingKids: map[string]time.Time{},
	}
}

// WithProvider 附加提供者。
func (r *KeycloakTokenResolver) WithProvider(provider string) *KeycloakTokenResolver {
	if strings.TrimSpace(provider) != "" {
		r.provider = strings.TrimSpace(provider)
	}
	return r
}

// Resolve 解析 token 並回傳可信身分。
func (r *KeycloakTokenResolver) Resolve(req *http.Request) (domain.AuthenticatedPrincipal, bool, error) {
	token := bearerToken(req)
	if token == "" {
		return domain.AuthenticatedPrincipal{}, false, nil
	}
	if !keycloakTokenShape(token) {
		return domain.AuthenticatedPrincipal{}, false, nil
	}
	claims, err := r.verify(req.Context(), token)
	if err != nil {
		return domain.AuthenticatedPrincipal{}, true, domain.Unauthorized("invalid bearer token")
	}
	return tokenPrincipalFromClaims(r.provider, claims), true, nil
}

// Ping 檢查外部服務連線狀態。
func (r *KeycloakTokenResolver) Ping(ctx context.Context) error {
	if r == nil || r.issuerURL == "" {
		return errors.New("keycloak token resolver not configured")
	}
	return r.refreshKeysIfNeeded(ctx)
}

// verify 處理 verify。
func (r *KeycloakTokenResolver) verify(ctx context.Context, token string) (map[string]any, error) {
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
	if err := r.refreshKeysIfNeeded(ctx); err != nil {
		return nil, err
	}

	r.mu.Lock()
	key := r.jwksKeys[header.Kid]
	missingUntil := r.missingKids[header.Kid]
	r.mu.Unlock()
	if key == nil && time.Now().Before(missingUntil) {
		return nil, errors.New("jwk not found")
	}
	// 缺失 kid 可能代表 Keycloak 已輪替 key，因此以短節流強制刷新。
	if key == nil && r.canForceRefresh() {
		if err := r.refreshKeys(ctx, true); err != nil {
			return nil, err
		}
		r.mu.Lock()
		key = r.jwksKeys[header.Kid]
		r.mu.Unlock()
	}
	if key == nil {
		r.rememberMissingKid(header.Kid)
		return nil, errors.New("jwk not found")
	}
	signingInput := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], signature); err != nil {
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
	if err := r.validateClaims(claims); err != nil {
		return nil, err
	}
	return claims, nil
}

// validateClaims 驗證 claims。
func (r *KeycloakTokenResolver) validateClaims(claims map[string]any) error {
	if claimString(claims, "iss") != r.issuerURL {
		return errors.New("issuer mismatch")
	}
	if r.clientID != "" && !audienceContains(claims["aud"], r.clientID) && claimString(claims, "azp") != r.clientID {
		return errors.New("audience mismatch")
	}
	now := time.Now().Unix()
	if exp := claimUnix(claims["exp"]); exp == 0 || exp <= now {
		return errors.New("token expired")
	}
	if nbf := claimUnix(claims["nbf"]); nbf > now {
		return errors.New("token not yet valid")
	}
	if claimString(claims, "sub", "account_id", "acct") == "" {
		return errors.New("missing subject or account claim")
	}
	return nil
}

// refreshKeysIfNeeded 處理 refresh keys if needed。
func (r *KeycloakTokenResolver) refreshKeysIfNeeded(ctx context.Context) error {
	return r.refreshKeys(ctx, false)
}

// refreshKeys 處理 refresh keys。
func (r *KeycloakTokenResolver) refreshKeys(ctx context.Context, force bool) error {
	r.mu.Lock()
	if !force && len(r.jwksKeys) > 0 && time.Since(r.fetched) < jwksCacheTTL {
		r.mu.Unlock()
		return nil
	}
	if force {
		r.lastForceFetch = time.Now()
	}
	r.mu.Unlock()

	jwksURI, err := r.discoveryJWKSURI(ctx)
	if err != nil {
		return err
	}
	keys, err := r.fetchJWKS(ctx, jwksURI)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.jwksURI = jwksURI
	r.jwksKeys = keys
	r.fetched = time.Now()
	r.missingKids = map[string]time.Time{}
	r.mu.Unlock()
	return nil
}

// canForceRefresh 處理 can force refresh。
func (r *KeycloakTokenResolver) canForceRefresh() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastForceFetch.IsZero() || time.Since(r.lastForceFetch) >= jwksForcedRefreshWindow
}

// rememberMissingKid 處理 remember missing kid。
func (r *KeycloakTokenResolver) rememberMissingKid(kid string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.missingKids == nil {
		r.missingKids = map[string]time.Time{}
	}
	r.missingKids[kid] = time.Now().Add(jwksMissingKidTTL)
}

// discoveryJWKSURI 處理 discovery jwksuri。
func (r *KeycloakTokenResolver) discoveryJWKSURI(ctx context.Context) (string, error) {
	if r.issuerURL == "" {
		return "", errors.New("issuer is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.issuerURL+"/.well-known/openid-configuration", nil)
	if err != nil {
		return "", err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errors.New("oidc discovery failed")
	}
	var body struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if strings.TrimRight(body.Issuer, "/") != r.issuerURL || body.JWKSURI == "" {
		return "", errors.New("invalid oidc discovery document")
	}
	return body.JWKSURI, nil
}

// fetchJWKS 處理 fetch JWKS。
func (r *KeycloakTokenResolver) fetchJWKS(ctx context.Context, uri string) (map[string]*rsa.PublicKey, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.client.Do(req)
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

type jwk struct {
	Kid string   `json:"kid"`
	Kty string   `json:"kty"`
	Alg string   `json:"alg"`
	N   string   `json:"n"`
	E   string   `json:"e"`
	X5C []string `json:"x5c"`
}

// rsaPublicKey 處理 rsa public key。
func (j jwk) rsaPublicKey() (*rsa.PublicKey, error) {
	if j.Kty != "" && j.Kty != "RSA" {
		return nil, errors.New("not rsa")
	}
	if len(j.X5C) > 0 {
		der, err := base64.StdEncoding.DecodeString(j.X5C[0])
		if err != nil {
			return nil, err
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, err
		}
		key, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("certificate key is not rsa")
		}
		return key, nil
	}
	n, err := base64.RawURLEncoding.DecodeString(j.N)
	if err != nil {
		return nil, err
	}
	e, err := base64.RawURLEncoding.DecodeString(j.E)
	if err != nil {
		return nil, err
	}
	exponent := int(new(big.Int).SetBytes(e).Int64())
	if exponent == 0 {
		return nil, errors.New("invalid exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exponent}, nil
}

// bearerToken 處理 bearer token。
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

// keycloakTokenShape 處理 Keycloak token shape。
func keycloakTokenShape(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return false
	}
	return header.Alg == "RS256" && header.Kid != ""
}

// tokenPrincipalFromClaims 處理 token principal 來源 claims。
func tokenPrincipalFromClaims(provider string, claims map[string]any) domain.AuthenticatedPrincipal {
	accountID := claimString(claims, "account_id", "acct")
	subject := claimString(claims, "sub")
	if subject == "" {
		subject = accountID
	}
	tenantID := claimString(claims, "tenant_id", "tid")
	tenantHint := claimString(claims, "tenant_hint")
	if tenantHint == "" {
		tenantHint = tenantID
	}
	return domain.AuthenticatedPrincipal{
		Provider:   strings.TrimSpace(provider),
		Subject:    subject,
		Email:      claimString(claims, "email"),
		Name:       claimString(claims, "name", "preferred_username"),
		TenantID:   tenantID,
		TenantHint: tenantHint,
		AccountID:  accountID,
		Claims:     copyClaims(claims),
	}
}

// copyClaims 複製 claims。
func copyClaims(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

// claimString 處理 claim 字串。
func claimString(claims map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := claims[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// audienceContains 處理 audience contains。
func audienceContains(value any, want string) bool {
	switch v := value.(type) {
	case string:
		return v == want
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

// claimUnix 處理 claim unix。
func claimUnix(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	default:
		return 0
	}
}
