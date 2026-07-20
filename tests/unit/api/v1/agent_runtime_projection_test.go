package v1_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1api "nexus-pro-api/internal/api/v1"
	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

// TestAgentCatalogAndRuntimePermissionProjection keeps catalog state non-runnable while runtime endpoints remain forbidden.
func TestAgentCatalogAndRuntimePermissionProjection(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	if err := store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-me-read", TenantID: "tenant-1", Name: "Me Read", CreatedAt: now,
		Permissions: []domain.Permission{{Resource: "me", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "workbench"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID: "acct-no-run", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-me-read"}, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUserIdentity(context.Background(), domain.UserIdentity{
		ID: "uid-no-run", TenantID: "tenant-1", AccountID: "acct-no-run", Provider: "keycloak", Subject: "acct-no-run", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgentDefinition(context.Background(), domain.AgentDefinition{
		ID: "agent-published", TenantID: "tenant-1", Name: "Published Agent", Category: domain.AgentCategoryWorkflow,
		Status: domain.AgentDefinitionStatusPublished, Visibility: domain.AgentVisibilityAll, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	handler := v1api.New(service.New(store), nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{
			Provider: "keycloak", Subject: "acct-no-run", TenantID: "tenant-1", AccountID: "acct-no-run",
		}, ok: true},
	}).Routes()

	catalog := httptest.NewRecorder()
	handler.ServeHTTP(catalog, httptest.NewRequest(http.MethodGet, "/v1/platform/assistants", nil))
	if catalog.Code != http.StatusOK || !strings.Contains(catalog.Body.String(), `"runnable":false`) {
		t.Fatalf("expected a non-runnable catalog item, got %d: %s", catalog.Code, catalog.Body.String())
	}

	for _, request := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/v1/agents/sessions"},
		{method: http.MethodGet, path: "/v1/agents/memories"},
		{method: http.MethodPost, path: "/v1/agents/chat", body: `{"message":"hello"}`},
	} {
		t.Run(request.method+" "+request.path, func(t *testing.T) {
			req := httptest.NewRequest(request.method, request.path, strings.NewReader(request.body))
			if request.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}
