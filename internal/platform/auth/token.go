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

	"nexus-pro-be/internal/domain"
)

type TokenContext struct {
	TenantID  string
	AccountID string
}

type TokenResolver interface {
	Resolve(r *http.Request) (TokenContext, bool, error)
}

type UnsignedJWTResolver struct{}

func (UnsignedJWTResolver) Resolve(r *http.Request) (TokenContext, bool, error) {
	claims, ok, err := parseUnsignedClaims(bearerToken(r))
	if err != nil || !ok {
		return TokenContext{}, ok, err
	}
	return tokenContextFromClaims(claims), true, nil
}

type KeycloakTokenResolver struct {
	issuerURL string
	clientID  string
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

func NewKeycloakTokenResolver(issuerURL, clientID string, client *http.Client) *KeycloakTokenResolver {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &KeycloakTokenResolver{
		issuerURL:   strings.TrimRight(strings.TrimSpace(issuerURL), "/"),
		clientID:    strings.TrimSpace(clientID),
		client:      client,
		missingKids: map[string]time.Time{},
	}
}

func (r *KeycloakTokenResolver) Resolve(req *http.Request) (TokenContext, bool, error) {
	token := bearerToken(req)
	if token == "" {
		return TokenContext{}, false, nil
	}
	claims, err := r.verify(req.Context(), token)
	if err != nil {
		return TokenContext{}, true, domain.Unauthorized("invalid bearer token")
	}
	return tokenContextFromClaims(claims), true, nil
}

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

func (r *KeycloakTokenResolver) validateClaims(claims map[string]any) error {
	if claimString(claims, "iss") != r.issuerURL {
		return errors.New("issuer mismatch")
	}
	if r.clientID != "" && !audienceContains(claims["aud"], r.clientID) {
		return errors.New("audience mismatch")
	}
	now := time.Now().Unix()
	if exp := claimUnix(claims["exp"]); exp == 0 || exp <= now {
		return errors.New("token expired")
	}
	if nbf := claimUnix(claims["nbf"]); nbf > now {
		return errors.New("token not yet valid")
	}
	if claimString(claims, "tenant_id", "tid") == "" || claimString(claims, "account_id", "acct", "sub") == "" {
		return errors.New("missing tenant or account claim")
	}
	return nil
}

func (r *KeycloakTokenResolver) refreshKeysIfNeeded(ctx context.Context) error {
	return r.refreshKeys(ctx, false)
}

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

func (r *KeycloakTokenResolver) canForceRefresh() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastForceFetch.IsZero() || time.Since(r.lastForceFetch) >= jwksForcedRefreshWindow
}

func (r *KeycloakTokenResolver) rememberMissingKid(kid string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.missingKids == nil {
		r.missingKids = map[string]time.Time{}
	}
	r.missingKids[kid] = time.Now().Add(jwksMissingKidTTL)
}

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

func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func parseUnsignedClaims(token string) (map[string]any, bool, error) {
	if token == "" {
		return nil, false, nil
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, false, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, true, err
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, true, err
	}
	return claims, true, nil
}

func tokenContextFromClaims(claims map[string]any) TokenContext {
	return TokenContext{
		TenantID:  claimString(claims, "tenant_id", "tid"),
		AccountID: claimString(claims, "account_id", "acct", "sub"),
	}
}

func claimString(claims map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := claims[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

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
