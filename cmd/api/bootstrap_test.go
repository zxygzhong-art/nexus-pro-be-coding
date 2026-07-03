package main

import (
	"strings"
	"testing"

	"nexus-pro-be/internal/config"
	"nexus-pro-be/internal/startup"
)

func TestConfiguredOIDCProvidersHonorsEnabledFlags(t *testing.T) {
	cfg := config.Config{
		HTTPAddr:                  ":8080",
		AuthSessionSigningKey:     "session-secret",
		GoogleOIDCEnabled:         false,
		GoogleOIDCClientID:        "google-client",
		GoogleOIDCClientSecret:    "google-secret",
		MicrosoftOIDCEnabled:      true,
		MicrosoftOIDCClientID:     "microsoft-client",
		MicrosoftOIDCClientSecret: "microsoft-secret",
	}

	providers, deps := configuredOIDCProviders(cfg, nil)
	if _, ok := providers["google"]; ok {
		t.Fatal("expected disabled Google OIDC provider to be skipped")
	}
	if _, ok := providers["microsoft"]; !ok {
		t.Fatal("expected enabled Microsoft OIDC provider to be configured")
	}
	if dep := oidcDependency(deps, "Google OIDC"); dep == nil || dep.Status != "skipped" {
		t.Fatalf("expected skipped Google dependency, got %+v", dep)
	}
	if dep := oidcDependency(deps, "Microsoft OIDC"); dep == nil || dep.Status != "configured" || !strings.Contains(dep.Detail, "redirect=http://localhost:8080/v1/auth/oidc/microsoft/callback") {
		t.Fatalf("expected configured Microsoft dependency with derived redirect, got %+v", dep)
	}
}

func TestConfiguredOIDCProvidersRequiresSessionSigningKeyWhenEnabled(t *testing.T) {
	cfg := config.Config{
		GoogleOIDCEnabled:      true,
		GoogleOIDCClientID:     "google-client",
		GoogleOIDCClientSecret: "google-secret",
	}

	providers, deps := configuredOIDCProviders(cfg, nil)
	if len(providers) != 0 {
		t.Fatalf("expected no OIDC providers without session signing key, got %d", len(providers))
	}
	if dep := oidcDependency(deps, "Google OIDC"); dep == nil || dep.Status != "incomplete" || !strings.Contains(dep.Detail, "AUTH_SESSION_SIGNING_KEY") {
		t.Fatalf("expected incomplete Google dependency with missing session key, got %+v", dep)
	}
}

func TestOIDCRedirectURLDerivesLocalCallback(t *testing.T) {
	tests := map[string]string{
		"":             "http://localhost:8080/v1/auth/oidc/google/callback",
		":8080":        "http://localhost:8080/v1/auth/oidc/google/callback",
		"0.0.0.0:8080": "http://localhost:8080/v1/auth/oidc/google/callback",
		"[::]:8080":    "http://localhost:8080/v1/auth/oidc/google/callback",
	}
	for input, want := range tests {
		if got := oidcRedirectURL(input, "google"); got != want {
			t.Fatalf("oidcRedirectURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func oidcDependency(deps []startup.Dependency, name string) *startup.Dependency {
	for i := range deps {
		if deps[i].Name == name {
			return &deps[i]
		}
	}
	return nil
}
