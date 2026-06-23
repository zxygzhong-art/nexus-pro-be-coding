package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
)

// TokenResolverChain tries resolvers in order until one accepts the token shape.
type TokenResolverChain struct {
	resolvers []TokenResolver
}

// NewTokenResolverChain builds a token resolver from multiple token families.
func NewTokenResolverChain(resolvers ...TokenResolver) TokenResolver {
	items := make([]TokenResolver, 0, len(resolvers))
	for _, resolver := range resolvers {
		if resolver != nil {
			items = append(items, resolver)
		}
	}
	return TokenResolverChain{resolvers: items}
}

// Resolve returns the first resolver that recognizes the request token.
func (r TokenResolverChain) Resolve(req *http.Request) (domain.AuthenticatedPrincipal, bool, error) {
	for _, resolver := range r.resolvers {
		principal, ok, err := resolver.Resolve(req)
		if ok || err != nil {
			return principal, ok, err
		}
	}
	return domain.AuthenticatedPrincipal{}, false, nil
}

// InternalTokenIssuer issues short-lived first-party HS256 bearer tokens after external login.
type InternalTokenIssuer struct {
	signingKey []byte
	issuer     string
	audience   string
	ttl        time.Duration
}

// NewInternalTokenIssuer creates a first-party token issuer and resolver pair.
func NewInternalTokenIssuer(signingKey, issuer, audience string, ttl time.Duration) *InternalTokenIssuer {
	if ttl <= 0 {
		ttl = 8 * time.Hour
	}
	return &InternalTokenIssuer{
		signingKey: []byte(strings.TrimSpace(signingKey)),
		issuer:     strings.TrimSpace(issuer),
		audience:   strings.TrimSpace(audience),
		ttl:        ttl,
	}
}

// IssueToken signs a bearer token for the resolved local account.
func (i *InternalTokenIssuer) IssueToken(resolution domain.IdentityResolution, principal domain.AuthenticatedPrincipal) (string, time.Time, error) {
	if i == nil || len(i.signingKey) == 0 {
		return "", time.Time{}, errors.New("internal token signing key is required")
	}
	now := time.Now().UTC()
	expiresAt := now.Add(i.ttl)
	header := map[string]any{"alg": "HS256", "typ": "JWT"}
	claims := map[string]any{
		"iss":            i.issuer,
		"aud":            i.audience,
		"iat":            now.Unix(),
		"nbf":            now.Unix(),
		"exp":            expiresAt.Unix(),
		"sub":            resolution.AccountID,
		"tenant_id":      resolution.TenantID,
		"account_id":     resolution.AccountID,
		"email":          principal.Email,
		"authn_provider": principal.Provider,
		"authn_subject":  principal.Subject,
	}
	token, err := signJWT(header, claims, i.signingKey)
	return token, expiresAt, err
}

// InternalTokenResolver validates first-party HS256 bearer tokens.
type InternalTokenResolver struct {
	signingKey []byte
	issuer     string
	audience   string
}

// NewInternalTokenResolver creates a resolver for first-party login tokens.
func NewInternalTokenResolver(signingKey, issuer, audience string) *InternalTokenResolver {
	return &InternalTokenResolver{signingKey: []byte(strings.TrimSpace(signingKey)), issuer: strings.TrimSpace(issuer), audience: strings.TrimSpace(audience)}
}

// Resolve validates HS256 internal bearer tokens and ignores other token families.
func (r *InternalTokenResolver) Resolve(req *http.Request) (domain.AuthenticatedPrincipal, bool, error) {
	token := bearerToken(req)
	if token == "" {
		return domain.AuthenticatedPrincipal{}, false, nil
	}
	claims, ok, err := r.verify(token)
	if err != nil || !ok {
		if !ok {
			return domain.AuthenticatedPrincipal{}, false, nil
		}
		return domain.AuthenticatedPrincipal{}, true, domain.Unauthorized("invalid bearer token")
	}
	return domain.AuthenticatedPrincipal{
		Provider:  "internal",
		Subject:   claimString(claims, "sub", "account_id"),
		Email:     claimString(claims, "email"),
		TenantID:  claimString(claims, "tenant_id", "tid"),
		AccountID: claimString(claims, "account_id", "acct", "sub"),
		Claims:    copyClaims(claims),
	}, true, nil
}

func (r *InternalTokenResolver) verify(token string) (map[string]any, bool, error) {
	if r == nil || len(r.signingKey) == 0 {
		return nil, false, nil
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, false, nil
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, true, err
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, true, err
	}
	if header.Alg != "HS256" {
		return nil, false, nil
	}
	signingInput := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, true, err
	}
	mac := hmac.New(sha256.New, r.signingKey)
	_, _ = mac.Write([]byte(signingInput))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return nil, true, errors.New("invalid internal token signature")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, true, err
	}
	var claims map[string]any
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, true, err
	}
	if err := r.validateClaims(claims); err != nil {
		return nil, true, err
	}
	return claims, true, nil
}

func (r *InternalTokenResolver) validateClaims(claims map[string]any) error {
	if r.issuer != "" && claimString(claims, "iss") != r.issuer {
		return errors.New("issuer mismatch")
	}
	if r.audience != "" && !audienceContains(claims["aud"], r.audience) {
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

func signJWT(header, claims map[string]any, key []byte) (string, error) {
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimBytes, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := base64.RawURLEncoding.EncodeToString(headerBytes) + "." + base64.RawURLEncoding.EncodeToString(claimBytes)
	sum := hmac.New(sha256.New, key)
	_, _ = sum.Write([]byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sum.Sum(nil)), nil
}
