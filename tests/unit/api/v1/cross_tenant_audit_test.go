package v1_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	v1api "nexus-pro-be/internal/api/v1"
	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func TestCrossTenantDeniedWritesCriticalAudit(t *testing.T) {
	store := memory.NewStore()
	populateDemoFixture(store)
	handler := v1api.New(service.New(store), nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{Provider: "keycloak", Subject: "acct-admin", TenantID: "demo", AccountID: "acct-admin"}, ok: true},
	}).Routes()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/me?tenant_id=other-tenant", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	errPayload := decodeError(t, rec.Body.Bytes())
	if errPayload.Code != domain.ErrorCodeCrossTenantDenied || errPayload.ReasonCode != "cross_tenant_denied" {
		t.Fatalf("expected cross tenant error code, got %+v", errPayload)
	}
	logs, err := store.ListAuditLogs(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	log, ok := findAPIAuditLog(logs, "security.cross_tenant.denied")
	if !ok {
		t.Fatalf("expected cross tenant audit, got %+v", logs)
	}
	if log.Severity != string(domain.SeverityCritical) || log.Details["target_tenant_id"] != "other-tenant" || log.Details["application_code"] != "platform" {
		t.Fatalf("expected critical cross tenant details, got %+v", log)
	}
}
