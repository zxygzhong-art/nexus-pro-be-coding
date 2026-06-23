package auth_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
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

func oidcProviderTestServer(t *testing.T, key *rsa.PrivateKey, claims map[string]any) (*httptest.Server, string) {
	t.Helper()
	var issuer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			discoveryIssuer := issuer
			if _, ok := claims["tid"]; ok {
				discoveryIssuer = issuer + "/{tenantid}/v2.0"
				claims["iss"] = issuer + "/" + claims["tid"].(string) + "/v2.0"
			} else {
				claims["iss"] = issuer
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
			if r.Form.Get("code") != "ok" || r.Form.Get("client_id") != "nexus-client" || r.Form.Get("client_secret") != "secret" {
				t.Fatalf("unexpected token exchange form: %s", r.Form.Encode())
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id_token": signedRS256JWT(t, "kid-1", key, claims)})
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

	authURL, err := provider.AuthorizationURL("signed-state")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(authURL, "state=signed-state") || !strings.Contains(authURL, "client_id=nexus-client") {
		t.Fatalf("unexpected authorization URL: %s", authURL)
	}
}
