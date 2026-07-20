package v1_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1api "nexus-pro-api/internal/api/v1"
	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

func TestCurrentAssumableRoleSessionCanBeReturnedFromNarrowRole(t *testing.T) {
	store := memory.NewStore()
	populateDemoFixture(store)
	svc := service.New(store)
	handler := assumedRoleSessionHandler(svc, "acct-admin")
	roleID := createNarrowAssumableRole(t, handler)
	sessionID := assumeTestRole(t, handler, roleID)

	revokeReq := httptest.NewRequest(http.MethodDelete, "/v1/iam/assumable-role-sessions/current", nil)
	revokeReq.Header.Set("X-Assumable-Role-Session-ID", sessionID)
	revokeRec := httptest.NewRecorder()
	handler.ServeHTTP(revokeRec, revokeReq)
	if revokeRec.Code != http.StatusNoContent {
		t.Fatalf("expected a narrow temporary role to return successfully, got %d", revokeRec.Code)
	}
	stored, ok, err := store.GetAssumableRoleSession(context.Background(), "demo", sessionID)
	if err != nil || !ok || stored.RevokedAt == nil {
		t.Fatalf("expected a retained revoked session record, ok=%v err=%v", ok, err)
	}

	repeatReq := httptest.NewRequest(http.MethodDelete, "/v1/iam/assumable-role-sessions/current", nil)
	repeatReq.Header.Set("X-Assumable-Role-Session-ID", sessionID)
	repeatRec := httptest.NewRecorder()
	handler.ServeHTTP(repeatRec, repeatReq)
	if repeatRec.Code != http.StatusNotFound {
		t.Fatalf("expected a stable revoked-session response, got %d", repeatRec.Code)
	}
	if got := decodeError(t, repeatRec.Body.Bytes()).ReasonCode; got != "assumed_role_session_revoked" {
		t.Fatalf("unexpected repeated revoke reason: %q", got)
	}
	if bytes.Contains(repeatRec.Body.Bytes(), []byte(sessionID)) {
		t.Fatal("error responses must not reflect an assumed-role bearer")
	}
}

func TestCurrentAssumableRoleSessionCannotRevokeAnotherAccount(t *testing.T) {
	store := memory.NewStore()
	populateDemoFixture(store)
	svc := service.New(store)
	adminHandler := assumedRoleSessionHandler(svc, "acct-admin")
	roleID := createNarrowAssumableRole(t, adminHandler)
	sessionID := assumeTestRole(t, adminHandler, roleID)

	otherHandler := assumedRoleSessionHandler(svc, "acct-employee")
	req := httptest.NewRequest(http.MethodDelete, "/v1/iam/assumable-role-sessions/current", nil)
	req.Header.Set("X-Assumable-Role-Session-ID", sessionID)
	rec := httptest.NewRecorder()
	otherHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected foreign session revocation to be forbidden, got %d", rec.Code)
	}
	if got := decodeError(t, rec.Body.Bytes()).ReasonCode; got != "assumed_role_session_foreign" {
		t.Fatalf("unexpected foreign-session reason: %q", got)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(sessionID)) {
		t.Fatal("foreign-session errors must not reflect the bearer")
	}
	stored, ok, err := store.GetAssumableRoleSession(context.Background(), "demo", sessionID)
	if err != nil || !ok || stored.RevokedAt != nil {
		t.Fatalf("another account's session must remain active, ok=%v err=%v", ok, err)
	}
}

func assumedRoleSessionHandler(svc *service.Service, accountID string) http.Handler {
	return v1api.New(svc, nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{
			Provider:  "keycloak",
			Subject:   accountID,
			TenantID:  "demo",
			AccountID: accountID,
		}, ok: true},
	}).Routes()
}

func createNarrowAssumableRole(t *testing.T, handler http.Handler) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/iam/assumable-roles", strings.NewReader(`{"name":"Temporary audit","trusted":true,"trust_policy":{"accounts":["acct-admin"]},"permission_boundary":{"allow":["audit.log.read"]},"permission_set_ids":["ps-audit"],"session_duration_seconds":1800}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected role creation, got %d", rec.Code)
	}
	return decodeData[domain.AssumableRole](t, rec.Body.Bytes()).ID
}

func assumeTestRole(t *testing.T, handler http.Handler, roleID string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/iam/assumable-roles/"+roleID+"/assume", strings.NewReader(`{"reason":"endpoint lifecycle test"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected role assumption, got %d", rec.Code)
	}
	result := decodeData[domain.AssumeRoleResponse](t, rec.Body.Bytes())
	if result.SessionID == "" || result.SessionToken != result.SessionID {
		t.Fatal("success response must return the temporary bearer to its caller")
	}
	return result.SessionID
}
