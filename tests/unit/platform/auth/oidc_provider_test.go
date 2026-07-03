package auth_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platformauth "nexus-pro-be/internal/platform/auth"
)

func TestOIDCProviderResolvesAuthorizationCodeIDToken(t *testing.T) {
	key := mustRSAKey(t)
	server, issuer := oidcProviderTestServer(t, key, oidcProviderTestClaims("", "nexus-client"))
	defer server.Close()

	provider := platformauth.NewOIDCProvider(platformauth.OIDCProviderConfig{
		Code:         "google",
		IssuerURL:    issuer,
		ClientID:     "nexus-client",
		ClientSecret: "secret",
		RedirectURL:  "https://app.example/auth/callback",
	}, server.Client())

	principal, err := provider.ResolveCallback(context.Background(), "ok")
	if err != nil {
		t.Fatal(err)
	}
	if principal.Provider != "google" || principal.Subject != "external-subject" || principal.Email != "user@example.com" {
		t.Fatalf("unexpected principal: %+v", principal)
	}
}

func TestOIDCProviderAcceptsMicrosoftTenantIssuerTemplate(t *testing.T) {
	key := mustRSAKey(t)
	claims := oidcProviderTestClaims("", "nexus-client")
	claims["tid"] = "tenant-123"
	server, issuer := oidcProviderTestServer(t, key, claims)
	defer server.Close()

	provider := platformauth.NewOIDCProvider(platformauth.OIDCProviderConfig{
		Code:         "microsoft",
		IssuerURL:    issuer,
		ClientID:     "nexus-client",
		ClientSecret: "secret",
		RedirectURL:  "https://app.example/auth/callback",
	}, server.Client())

	principal, err := provider.ResolveCallback(context.Background(), "ok")
	if err != nil {
		t.Fatal(err)
	}
	if principal.Provider != "microsoft" || principal.Subject != "external-subject" {
		t.Fatalf("unexpected principal: %+v", principal)
	}
}

func TestOIDCProviderRejectsMicrosoftTenantIssuerMismatch(t *testing.T) {
	key := mustRSAKey(t)
	claims := oidcProviderTestClaims("https://evil.example/tenant-123/v2.0", "nexus-client")
	claims["tid"] = "tenant-123"
	server, issuer := oidcProviderTestServer(t, key, claims)
	defer server.Close()

	provider := platformauth.NewOIDCProvider(platformauth.OIDCProviderConfig{
		Code:         "microsoft",
		IssuerURL:    issuer,
		ClientID:     "nexus-client",
		ClientSecret: "secret",
		RedirectURL:  "https://app.example/auth/callback",
	}, server.Client())

	if _, err := provider.ResolveCallback(context.Background(), "ok"); err == nil {
		t.Fatal("expected issuer mismatch to fail")
	}
}

func oidcProviderTestServer(t *testing.T, key *rsa.PrivateKey, claims map[string]any) (*httptest.Server, string) {
	t.Helper()
	var issuer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			discoveryIssuer := issuer
			if _, ok := claims["tid"]; ok {
				discoveryIssuer = issuer + "/{tenantid}/v2.0"
				if claim, _ := claims["iss"].(string); strings.TrimSpace(claim) == "" {
					claims["iss"] = issuer + "/" + claims["tid"].(string) + "/v2.0"
				}
			} else {
				if claim, _ := claims["iss"].(string); strings.TrimSpace(claim) == "" {
					claims["iss"] = issuer
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer":                 discoveryIssuer,
				"authorization_endpoint": issuer + "/authorize",
				"token_endpoint":         issuer + "/token",
				"jwks_uri":               issuer + "/certs",
			})
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			clientID, clientSecret, _ := r.BasicAuth()
			if clientID == "" {
				clientID = r.Form.Get("client_id")
			}
			if clientSecret == "" {
				clientSecret = r.Form.Get("client_secret")
			}
			if r.Form.Get("code") != "ok" || clientID != "nexus-client" || clientSecret != "secret" {
				t.Fatalf("unexpected token exchange form: %s basic=%s/%s", r.Form.Encode(), clientID, clientSecret)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "provider-access-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
				"id_token":     signedRS256JWT(t, "kid-1", key, claims),
			})
		case "/certs":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{jwkFromKey("kid-1", &key.PublicKey)}})
		default:
			http.NotFound(w, r)
		}
	}))
	issuer = server.URL
	return server, issuer
}

func oidcProviderTestClaims(issuer, audience string) map[string]any {
	return map[string]any{
		"iss":   issuer,
		"aud":   audience,
		"sub":   "external-subject",
		"email": "user@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}
}

func mustRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func signedRS256JWT(t *testing.T, kid string, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	headerBytes, err := json.Marshal(map[string]string{"alg": "RS256", "kid": kid, "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	signingInput := base64.RawURLEncoding.EncodeToString(headerBytes) + "." + base64.RawURLEncoding.EncodeToString(payloadBytes)
	sum := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func jwkFromKey(kid string, key *rsa.PublicKey) map[string]string {
	return map[string]string{
		"kid": kid,
		"kty": "RSA",
		"alg": "RS256",
		"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}
}

func TestOIDCProviderAuthorizationURLIncludesState(t *testing.T) {
	key := mustRSAKey(t)
	server, issuer := oidcProviderTestServer(t, key, oidcProviderTestClaims("", "nexus-client"))
	defer server.Close()
	provider := platformauth.NewOIDCProvider(platformauth.OIDCProviderConfig{
		Code:         "google",
		IssuerURL:    issuer,
		ClientID:     "nexus-client",
		ClientSecret: "secret",
		RedirectURL:  "https://app.example/auth/callback",
	}, server.Client())

	authURL, err := provider.AuthorizationURL(context.Background(), "signed-state")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(authURL, "state=signed-state") || !strings.Contains(authURL, "client_id=nexus-client") {
		t.Fatalf("unexpected authorization URL: %s", authURL)
	}
}

func TestOIDCProviderAuthorizationURLUsesRequestContext(t *testing.T) {
	client := &http.Client{Transport: oidcContextTransport{t: t}}
	provider := platformauth.NewOIDCProvider(platformauth.OIDCProviderConfig{
		Code:         "google",
		IssuerURL:    "https://issuer.example",
		ClientID:     "nexus-client",
		ClientSecret: "secret",
		RedirectURL:  "https://app.example/auth/callback",
	}, client)
	ctx := context.WithValue(context.Background(), oidcContextMarkerKey{}, "present")

	authURL, err := provider.AuthorizationURL(ctx, "signed-state")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(authURL, "state=signed-state") {
		t.Fatalf("unexpected authorization URL: %s", authURL)
	}
}

type oidcContextMarkerKey struct{}

type oidcContextTransport struct {
	t *testing.T
}

func (t oidcContextTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.t.Helper()
	if got := req.Context().Value(oidcContextMarkerKey{}); got != "present" {
		t.t.Fatalf("expected authorization URL metadata request to use caller context, got %v", got)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"issuer":"https://issuer.example",
			"authorization_endpoint":"https://issuer.example/auth",
			"token_endpoint":"https://issuer.example/token",
			"jwks_uri":"https://issuer.example/jwks"
		}`)),
		Request: req,
	}, nil
}
