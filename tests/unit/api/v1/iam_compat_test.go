package v1_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRemovedIAMCompatibilityEndpointsReturnNotFound verifies the retired compatibility projections stay unavailable.
func TestRemovedIAMCompatibilityEndpointsReturnNotFound(t *testing.T) {
	handler := newTestAPI(true)
	for _, path := range []string{"/v1/iam/roles", "/v1/iam/role-bindings"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for removed compatibility endpoint %s, got %d: %s", path, rec.Code, rec.Body.String())
		}
	}
}
