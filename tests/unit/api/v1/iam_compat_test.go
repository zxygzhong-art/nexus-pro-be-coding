package v1_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	v1api "nexus-pro-api/internal/api/v1"
	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

// TestIAMRolesCompatibilityEndpoint 驗證 /v1/iam/roles 只讀投影。
func TestIAMRolesCompatibilityEndpoint(t *testing.T) {
	now := time.Date(2026, 7, 7, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	populateDemoFixture(store)
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID:                     "ar-audit",
		TenantID:               "demo",
		Name:                   "Audit Assume",
		PermissionSetIDs:       []string{"ps-audit"},
		Trusted:                true,
		TrustPolicy:            map[string]any{"accounts": []any{"acct-admin"}},
		PermissionBoundary:     map[string]any{"allow": []any{"audit.log.read"}},
		SessionDurationSeconds: 3600,
		CreatedAt:              now,
	})
	handler := v1api.New(service.New(store), nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{Provider: "keycloak", Subject: "acct-admin", TenantID: "demo", AccountID: "acct-admin"}, ok: true},
	}).Routes()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/iam/roles", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	page := decodeData[domain.PageResponse[domain.IAMRoleProjection]](t, rec.Body.Bytes())
	if page.Total != 1 || page.Items[0].ID != "ar-audit" || len(page.Items[0].PermissionSets) != 1 || page.Items[0].PermissionSets[0].ID != "ps-audit" {
		t.Fatalf("unexpected role projection page: %+v", page)
	}
}

// TestIAMRoleBindingsCompatibilityEndpoint 驗證 /v1/iam/role-bindings 只讀投影。
func TestIAMRoleBindingsCompatibilityEndpoint(t *testing.T) {
	now := time.Date(2026, 7, 7, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	populateDemoFixture(store)
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "psa-audit",
		TenantID:        "demo",
		PrincipalType:   string(domain.PrincipalTypeAccount),
		PrincipalID:     "acct-employee",
		PermissionSetID: "ps-audit",
		Effect:          string(domain.EffectAllow),
		CreatedAt:       now,
	})
	handler := v1api.New(service.New(store), nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{Provider: "keycloak", Subject: "acct-admin", TenantID: "demo", AccountID: "acct-admin"}, ok: true},
	}).Routes()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/iam/role-bindings", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	page := decodeData[domain.PageResponse[domain.IAMRoleBindingProjection]](t, rec.Body.Bytes())
	if page.Total != 1 || page.Items[0].ID != "psa-audit" || page.Items[0].PermissionSet == nil || page.Items[0].PermissionSet.ID != "ps-audit" {
		t.Fatalf("unexpected role binding projection page: %+v", page)
	}
}
