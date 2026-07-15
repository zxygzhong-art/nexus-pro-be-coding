package v1_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"nexus-pro-be/internal/domain"
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
