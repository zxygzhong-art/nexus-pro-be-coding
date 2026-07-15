package v1_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1api "nexus-pro-be/internal/api/v1"
	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

// TestWorkflowAssignedSelfReaderCanApproveOverHTTP keeps route authz from hiding active assignee access.
func TestWorkflowAssignedSelfReaderCanApproveOverHTTP(t *testing.T) {
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	populateDemoFixture(store)
	if err := store.UpsertPermissionSet(t.Context(), domain.PermissionSet{
		ID: "ps-http-assignee", TenantID: "demo", Name: "HTTP assigned reviewer",
		Permissions: []domain.Permission{{Resource: "workflow.form_instance", Action: "read", Scope: "self"}},
		CreatedAt:   now,
	}); err != nil {
		t.Fatal(err)
	}
	reviewer, ok, err := store.GetAccount(t.Context(), "demo", "acct-workflow-approver")
	if err != nil || !ok {
		t.Fatalf("reviewer lookup failed ok=%v err=%v", ok, err)
	}
	reviewer.UserGroupIDs = nil
	reviewer.DirectPermissionSetIDs = []string{"ps-http-assignee"}
	if err := store.UpsertAccount(t.Context(), reviewer); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEmployee(t.Context(), domain.Employee{
		ID: "emp-http-bystander", TenantID: "demo", Name: "HTTP Bystander", AccountID: "acct-http-bystander",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(t.Context(), domain.Account{
		ID: "acct-http-bystander", TenantID: "demo", DisplayName: "HTTP Bystander", EmployeeID: "emp-http-bystander",
		Status: "active", DirectPermissionSetIDs: []string{"ps-http-assignee"}, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUserIdentity(t.Context(), domain.UserIdentity{
		ID: "uid-http-bystander", TenantID: "demo", AccountID: "acct-http-bystander",
		Provider: "keycloak", Subject: "acct-http-bystander", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFormTemplate(t.Context(), domain.FormTemplate{
		ID: "ft-http-assignee", TenantID: "demo", Key: "http-assignee", Name: "HTTP assignee",
		Schema: map[string]any{"workspace_design": map[string]any{
			"enabled": true,
			"stages": []any{map[string]any{
				"id": "stage-review", "type": "approver", "label": "Review",
				"config": map[string]any{"account_ids": []any{"acct-workflow-approver"}},
			}},
		}},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	fake := &apiFakeFormApprovalWorkflowClient{started: map[string]domain.FormApprovalWorkflowStart{}}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, FormApprovalWorkflows: fake})
	fake.service = svc
	instance, err := svc.Workflow().SubmitForm(domain.RequestContext{TenantID: "demo", AccountID: "acct-admin"}, domain.SubmitFormInput{
		TemplateKey: "http-assignee",
		Payload:     map[string]any{"subject": "assigned HTTP review"},
	})
	if err != nil {
		t.Fatal(err)
	}

	request := func(accountID string) *httptest.ResponseRecorder {
		t.Helper()
		handler := v1api.New(svc, nil, v1api.Options{TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{
			Provider: "keycloak", Subject: accountID, TenantID: "demo", AccountID: accountID,
		}, ok: true}}).Routes()
		req := httptest.NewRequest(http.MethodPost, "/v1/workflows/forms/"+instance.ID+"/approve", strings.NewReader(`{"reason":"assigned"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	if rec := request("acct-http-bystander"); rec.Code != http.StatusForbidden {
		t.Fatalf("expected unassigned self reader to receive 403, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := request("acct-workflow-approver"); rec.Code != http.StatusOK {
		t.Fatalf("expected assigned self reader to approve, got %d: %s", rec.Code, rec.Body.String())
	}
}
