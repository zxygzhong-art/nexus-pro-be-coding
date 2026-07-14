package auth_test

import (
	"context"
	"encoding/json"
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
