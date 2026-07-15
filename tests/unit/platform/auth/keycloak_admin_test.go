package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"nexus-pro-be/internal/domain"
	platformauth "nexus-pro-be/internal/platform/auth"
)

// TestKeycloakAdminClientEnsureUserCreatesAndInvites 驗證 Keycloak 管理員 client ensure 使用者 creates and invites。
func TestKeycloakAdminClientEnsureUserCreatesAndInvites(t *testing.T) {
	created := false
	inviteSent := false
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/nexus/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected token method %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_id") != "admin-client" || r.Form.Get("client_secret") != "admin-secret" {
			t.Fatalf("unexpected token form: %+v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "admin-token", "expires_in": 60})
	})
	mux.HandleFunc("/admin/realms/nexus/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer admin-token" {
			t.Fatalf("unexpected auth header %q", r.Header.Get("Authorization"))
		}
		switch r.Method {
		case http.MethodGet:
			if r.URL.Query().Get("email") != "import.login@example.com" || r.URL.Query().Get("exact") != "true" {
				t.Fatalf("unexpected user search query: %s", r.URL.RawQuery)
			}
			if created {
				_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "kc-user-1", "username": "import.login@example.com", "email": "import.login@example.com"}})
				return
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case http.MethodPost:
			var payload struct {
				Username        string              `json:"username"`
				Email           string              `json:"email"`
				Enabled         bool                `json:"enabled"`
				RequiredActions []string            `json:"requiredActions"`
				Attributes      map[string][]string `json:"attributes"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.Username != "import.login@example.com" || payload.Email != "import.login@example.com" || !payload.Enabled {
				t.Fatalf("unexpected create payload: %+v", payload)
			}
			if len(payload.RequiredActions) != 1 || payload.RequiredActions[0] != "UPDATE_PASSWORD" {
				t.Fatalf("expected update password required action, got %+v", payload.RequiredActions)
			}
			if got := payload.Attributes["account_id"]; len(got) != 1 || got[0] != "acct-1" {
				t.Fatalf("expected account_id attribute, got %+v", payload.Attributes)
			}
			created = true
			w.Header().Set("Location", "http://example.test/admin/realms/nexus/users/kc-user-1")
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected users method %s", r.Method)
		}
	})
	mux.HandleFunc("/admin/realms/nexus/users/kc-user-1/execute-actions-email", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected invite method %s", r.Method)
		}
		if r.URL.Query().Get("client_id") != "nexus-api" || r.URL.Query().Get("redirect_uri") != "https://app.example/login" {
			t.Fatalf("unexpected invite query: %s", r.URL.RawQuery)
		}
		var actions []string
		if err := json.NewDecoder(r.Body).Decode(&actions); err != nil {
			t.Fatal(err)
		}
		if len(actions) != 1 || actions[0] != "UPDATE_PASSWORD" {
			t.Fatalf("unexpected invite actions: %+v", actions)
		}
		inviteSent = true
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := platformauth.NewKeycloakAdminClient(platformauth.KeycloakAdminConfig{
		IssuerURL:         server.URL + "/realms/nexus",
		ClientID:          "admin-client",
		ClientSecret:      "admin-secret",
		SendInviteEmail:   true,
		InviteClientID:    "nexus-api",
		InviteRedirectURL: "https://app.example/login",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := client.EnsureUser(context.Background(), domain.IdentityProvisioningInput{
		TenantID:    "tenant-1",
		AccountID:   "acct-1",
		EmployeeID:  "emp-1",
		EmployeeNo:  "E2101",
		Email:       "Import.Login@Example.com",
		DisplayName: "Import Login",
		Enabled:     true,
		SendInvite:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if identity.Provider != domain.IdentityProviderKeycloak || identity.Subject != "kc-user-1" || identity.Email != "import.login@example.com" {
		t.Fatalf("unexpected provisioned identity: %+v", identity)
	}
	if !created || !inviteSent {
		t.Fatalf("expected keycloak user creation and invite, created=%v invite_sent=%v", created, inviteSent)
	}
}

// TestKeycloakAdminClientRejectsCrossTenantExistingEmail 驗證共享 realm 不會覆寫其他 tenant 的 ownership attributes。
func TestKeycloakAdminClientRejectsCrossTenantExistingEmail(t *testing.T) {
	updated := false
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/nexus/protocol/openid-connect/token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "admin-token", "expires_in": 60})
	})
	mux.HandleFunc("/admin/realms/nexus/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected users method %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"id": "kc-other", "username": "shared@example.com", "email": "shared@example.com",
			"attributes": map[string][]string{"tenant_id": {"tenant-other"}, "account_id": {"acct-other"}},
		}})
	})
	mux.HandleFunc("/admin/realms/nexus/users/kc-other", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			updated = true
		}
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := platformauth.NewKeycloakAdminClient(platformauth.KeycloakAdminConfig{
		IssuerURL: server.URL + "/realms/nexus", ClientID: "admin-client", ClientSecret: "admin-secret",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.EnsureUser(context.Background(), domain.IdentityProvisioningInput{
		TenantID: "tenant-1", AccountID: "acct-1", Email: "shared@example.com", Enabled: true,
	})
	if err == nil {
		t.Fatal("expected cross-tenant email ownership conflict")
	}
	if updated {
		t.Fatal("expected ownership conflict before any Keycloak PUT")
	}
}

// TestKeycloakAdminClientChangePasswordVerifiesAndClosesSession verifies the credential without leaving a login session.
func TestKeycloakAdminClientChangePasswordVerifiesAndClosesSession(t *testing.T) {
	passwordReset := false
	verificationLoggedOut := false
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/nexus/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		switch r.Form.Get("grant_type") {
		case "client_credentials":
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "admin-token", "expires_in": 60})
		case "password":
			if r.Form.Get("client_id") != "login-client" || r.Form.Get("username") != "employee@example.com" || r.Form.Get("password") != "old-password" {
				t.Fatalf("unexpected password verification form: %+v", r.Form)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "verification-token", "refresh_token": "verification-refresh"})
		default:
			t.Fatalf("unexpected grant type %q", r.Form.Get("grant_type"))
		}
	})
	mux.HandleFunc("/realms/nexus/protocol/openid-connect/logout", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("client_id") != "login-client" || r.Form.Get("refresh_token") != "verification-refresh" {
			t.Fatalf("unexpected verification logout form: %+v", r.Form)
		}
		verificationLoggedOut = true
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/admin/realms/nexus/users/kc-user-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer admin-token" {
			t.Fatalf("unexpected user lookup: method=%s auth=%q", r.Method, r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "kc-user-1", "username": "employee@example.com", "email": "employee@example.com",
			"attributes": map[string][]string{"tenant_id": {"tenant-1"}, "account_id": {"acct-1"}},
		})
	})
	mux.HandleFunc("/admin/realms/nexus/users/kc-user-1/reset-password", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.Header.Get("Authorization") != "Bearer admin-token" {
			t.Fatalf("unexpected reset request: method=%s auth=%q", r.Method, r.Header.Get("Authorization"))
		}
		var payload struct {
			Type      string `json:"type"`
			Value     string `json:"value"`
			Temporary bool   `json:"temporary"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Type != "password" || payload.Value != "new-password" || payload.Temporary {
			t.Fatalf("unexpected reset payload: %+v", payload)
		}
		passwordReset = true
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := platformauth.NewKeycloakAdminClient(platformauth.KeycloakAdminConfig{
		IssuerURL: server.URL + "/realms/nexus", ClientID: "admin-client", ClientSecret: "admin-secret", LoginClientID: "login-client",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	err = client.ChangePassword(context.Background(), domain.IdentityPasswordChangeInput{
		TenantID: "tenant-1", AccountID: "acct-1", Subject: "kc-user-1", CurrentPassword: "old-password", NewPassword: "new-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !verificationLoggedOut || !passwordReset {
		t.Fatalf("expected verification logout and password reset, logout=%v reset=%v", verificationLoggedOut, passwordReset)
	}
}

// TestKeycloakAdminClientChangePasswordRejectsInvalidCurrentPassword prevents reset after failed re-authentication.
func TestKeycloakAdminClientChangePasswordRejectsInvalidCurrentPassword(t *testing.T) {
	resetCalled := false
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/nexus/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") == "client_credentials" {
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "admin-token", "expires_in": 60})
			return
		}
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
	})
	mux.HandleFunc("/admin/realms/nexus/users/kc-user-1", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "kc-user-1", "username": "employee@example.com"})
	})
	mux.HandleFunc("/admin/realms/nexus/users/kc-user-1/reset-password", func(w http.ResponseWriter, _ *http.Request) {
		resetCalled = true
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := platformauth.NewKeycloakAdminClient(platformauth.KeycloakAdminConfig{
		IssuerURL: server.URL + "/realms/nexus", ClientID: "admin-client", ClientSecret: "admin-secret", LoginClientID: "login-client",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	err = client.ChangePassword(context.Background(), domain.IdentityPasswordChangeInput{
		TenantID: "tenant-1", AccountID: "acct-1", Subject: "kc-user-1", CurrentPassword: "wrong", NewPassword: "new-password",
	})
	if !errors.Is(err, domain.ErrIdentityCurrentPasswordInvalid) {
		t.Fatalf("expected invalid current password, got %v", err)
	}
	if resetCalled {
		t.Fatal("password reset must not run after failed current-password verification")
	}
}
