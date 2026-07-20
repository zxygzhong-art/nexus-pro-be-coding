package v1_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	v1api "nexus-pro-api/internal/api/v1"
	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

// TestUnlinkedExternalIdentityEnvelopeIncludesStableReasonCode protects the frontend auth-boundary contract.
func TestUnlinkedExternalIdentityEnvelopeIncludesStableReasonCode(t *testing.T) {
	store := memory.NewStore()
	populateDemoFixture(store)
	handler := v1api.New(service.New(store), nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{
			Provider: "keycloak",
			Subject:  "unlinked-subject",
			TenantID: "demo",
		}, ok: true},
	}).Routes()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for an unlinked identity, got %d: %s", rec.Code, rec.Body.String())
	}
	apiErr := decodeError(t, rec.Body.Bytes())
	if apiErr.Code != domain.ErrorCodeUnauthorized || apiErr.ReasonCode != "identity_not_linked" {
		t.Fatalf("expected stable identity_not_linked envelope, got %+v", apiErr)
	}
}
