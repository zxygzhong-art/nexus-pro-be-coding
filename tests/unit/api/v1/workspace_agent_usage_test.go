package v1_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
)

// TestWorkspaceAgentUsageBindsServerListControls verifies overview filters and pagination reach the service.
func TestWorkspaceAgentUsageBindsServerListControls(t *testing.T) {
	handler := newTestAPI(true)
	req := httptest.NewRequest(http.MethodGet, "/v1/workspace/agent-usage?query=Demo%20Admin&status=active&page=1&page_size=1&sort=total_tokens_desc", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	page := decodeData[domain.AgentUsageResponse](t, rec.Body.Bytes())
	if page.Page != 1 || page.PageSize != 1 || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("unexpected usage overview page: %+v", page)
	}
}

// TestWorkspaceAgentUsageDetailBindsAccountID verifies Gin path params reach the usage service.
func TestWorkspaceAgentUsageDetailBindsAccountID(t *testing.T) {
	handler := newTestAPI(true)
	req := httptest.NewRequest(http.MethodGet, "/v1/workspace/agent-usage/acct-admin/sessions?page=1&page_size=20", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	page := decodeData[domain.AgentSessionUsagePage](t, rec.Body.Bytes())
	if page.Account.AccountID != "acct-admin" {
		t.Fatalf("expected account acct-admin, got %q", page.Account.AccountID)
	}
	if page.Page != 1 || page.PageSize != 20 {
		t.Fatalf("expected page 1 with size 20, got page %d with size %d", page.Page, page.PageSize)
	}
}

// TestWorkspaceAgentUsageUsesDedicatedTenantWidePermission covers the real route and service authz boundary.
func TestWorkspaceAgentUsageUsesDedicatedTenantWidePermission(t *testing.T) {
	now := time.Date(2026, 7, 15, 2, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name       string
		permission domain.Permission
		status     int
		reasonCode string
	}{
		{
			name:       "definition_read_is_not_usage_read",
			permission: domain.Permission{Resource: "agent.definition", Action: domain.ActionRead, Scope: domain.ScopeAll},
			status:     http.StatusForbidden,
			reasonCode: "menu_denied",
		},
		{
			name:       "usage_read_requires_explicit_scope",
			permission: domain.Permission{Resource: "agent.usage", Action: domain.ActionRead},
			status:     http.StatusForbidden,
			reasonCode: "data_scope_denied",
		},
		{
			name:       "tenant_usage_read",
			permission: domain.Permission{Resource: "agent.usage", Action: domain.ActionRead, Scope: domain.ScopeTenant},
			status:     http.StatusOK,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := newTestAPIForAccountNow("acct-usage-boundary", now, func(store *memory.Store) {
				if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
					ID:          "ps-usage-boundary",
					TenantID:    "demo",
					Name:        "Usage Boundary",
					Permissions: []domain.Permission{test.permission},
					CreatedAt:   now,
				}); err != nil {
					t.Fatal(err)
				}
				if err := store.UpsertAccount(context.Background(), domain.Account{
					ID:                     "acct-usage-boundary",
					TenantID:               "demo",
					DisplayName:            "Usage Boundary",
					Email:                  "usage-boundary@example.com",
					Status:                 "active",
					DirectPermissionSetIDs: []string{"ps-usage-boundary"},
					CreatedAt:              now,
				}); err != nil {
					t.Fatal(err)
				}
				if err := store.UpsertUserIdentity(context.Background(), domain.UserIdentity{
					ID:        "uid-usage-boundary",
					TenantID:  "demo",
					AccountID: "acct-usage-boundary",
					Provider:  domain.IdentityProviderKeycloak,
					Subject:   "acct-usage-boundary",
					Email:     "usage-boundary@example.com",
					CreatedAt: now,
				}); err != nil {
					t.Fatal(err)
				}
			})

			for _, path := range []string{
				"/v1/workspace/agent-usage?page=1&page_size=20",
				"/v1/workspace/agent-usage/acct-admin/sessions?page=1&page_size=20",
			} {
				req := httptest.NewRequest(http.MethodGet, path, nil)
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				if rec.Code != test.status {
					t.Fatalf("expected %s status %d, got %d: %s", path, test.status, rec.Code, rec.Body.String())
				}
				if test.reasonCode != "" {
					apiErr := decodeError(t, rec.Body.Bytes())
					if apiErr.ReasonCode != test.reasonCode {
						t.Fatalf("expected %s reason %s, got %+v", path, test.reasonCode, apiErr)
					}
				}
			}
		})
	}
}
