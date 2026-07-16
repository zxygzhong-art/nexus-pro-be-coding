package postgres_integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1api "nexus-pro-be/internal/api/v1"
	"nexus-pro-be/internal/domain"
	postgresrepo "nexus-pro-be/internal/repository/postgres"
	"nexus-pro-be/internal/service"
)

// TestWorkflowHTTPPostgresAcceptance 驗證 workflow HTTP 路由在 Postgres 上可 submit → review → approve。
func TestWorkflowHTTPPostgresAcceptance(t *testing.T) {
	pool := openIntegrationPool(t)
	defer pool.Close()
	requireWorkflowRuntimeSchema(t, pool)
	store := postgresrepo.NewStore(pool)
	ctx := context.Background()
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	suffix := strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_") + "_" + now.Format("150405000000")
	tenantID := "tenant_" + suffix
	applicantID := "acct_" + suffix + "_applicant"
	approverID := "acct_" + suffix + "_approver"
	applicantEmpID := "emp_" + suffix + "_applicant"

	if err := store.UpsertTenant(ctx, domain.Tenant{ID: tenantID, Name: tenantID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	for _, item := range []domain.PermissionSet{
		{
			ID: "ps_" + suffix + "_applicant", TenantID: tenantID, Name: "Applicant",
			Permissions: []domain.Permission{
				{Resource: "workflow.form_instance", Action: "submit", Scope: "self"},
				{Resource: "workflow.form_instance", Action: "read", Scope: "self"},
				{Resource: "me", Action: "read", Scope: "self", MenuKey: "workbench"},
			},
			CreatedAt: now,
		},
		{
			ID: "ps_" + suffix + "_approver", TenantID: tenantID, Name: "Approver",
			Permissions: []domain.Permission{
				{Resource: "workflow.form_instance", Action: "read", Scope: "all"},
				{Resource: "workflow.form_instance", Action: "approve", Scope: "all"},
			},
			CreatedAt: now,
		},
	} {
		if err := store.UpsertPermissionSet(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpsertAccount(ctx, domain.Account{
		ID: applicantID, TenantID: tenantID, DisplayName: "Applicant", EmployeeID: applicantEmpID, Status: "active",
		DirectPermissionSetIDs: []string{"ps_" + suffix + "_applicant"}, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEmployee(ctx, domain.Employee{
		ID: applicantEmpID, TenantID: tenantID, Name: "Applicant", AccountID: applicantID,
		Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(ctx, domain.Account{
		ID: approverID, TenantID: tenantID, DisplayName: "Approver", Status: "active",
		DirectPermissionSetIDs: []string{"ps_" + suffix + "_approver"}, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	upsertIntegrationIdentity(t, store, tenantID, applicantID, now)
	upsertIntegrationIdentity(t, store, tenantID, approverID, now)
	if err := store.UpsertFormTemplate(ctx, domain.FormTemplate{
		ID: "ft_" + suffix, TenantID: tenantID, Key: "general", Name: "通用簽呈",
		Schema: postgresWorkflowTemplateSchema(approverID), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	svc := newIntegrationServiceWithFormApprovalWorkflows(
		store,
		service.Options{Now: func() time.Time { return now.Add(time.Hour) }},
	)
	handler := v1api.New(
		svc,
		nil,
		v1api.Options{TokenResolver: integrationTokenResolver{}},
	).Routes()

	submitRec := httptest.NewRecorder()
	submitReq := httptest.NewRequest(http.MethodPost, "/v1/workflows/forms/general/submit", strings.NewReader(`{"payload":{"desc":"http integration"}}`))
	submitReq.Header.Set("Content-Type", "application/json")
	addIntegrationHeaders(submitReq, tenantID, applicantID, "req-"+suffix+"-submit")
	handler.ServeHTTP(submitRec, submitReq)
	if submitRec.Code != http.StatusCreated {
		t.Fatalf("submit expected 201, got %d: %s", submitRec.Code, submitRec.Body.String())
	}
	submitted := decodeHTTPData[domain.FormInstance](t, submitRec.Body.Bytes())
	if submitted.Status != domain.WorkflowFormStatusInReview || submitted.CurrentRunID == "" {
		t.Fatalf("expected in_review with run, got %+v", submitted)
	}

	stateRec := httptest.NewRecorder()
	stateReq := httptest.NewRequest(http.MethodGet, "/v1/workflows/forms/"+submitted.ID+"/workflow", nil)
	addIntegrationHeaders(stateReq, tenantID, approverID, "req-"+suffix+"-state")
	handler.ServeHTTP(stateRec, stateReq)
	if stateRec.Code != http.StatusOK {
		t.Fatalf("workflow state expected 200, got %d: %s", stateRec.Code, stateRec.Body.String())
	}
	state := decodeHTTPData[domain.WorkflowFormStateResponse](t, stateRec.Body.Bytes())
	if !state.CanAct || state.RunStatus != domain.WorkflowRunStatusRunning {
		t.Fatalf("expected approver can act, got %+v", state)
	}

	reviewsRec := httptest.NewRecorder()
	reviewsReq := httptest.NewRequest(http.MethodGet, "/v1/workflows/reviews", nil)
	addIntegrationHeaders(reviewsReq, tenantID, approverID, "req-"+suffix+"-reviews")
	handler.ServeHTTP(reviewsRec, reviewsReq)
	if reviewsRec.Code != http.StatusOK {
		t.Fatalf("reviews expected 200, got %d: %s", reviewsRec.Code, reviewsRec.Body.String())
	}
	reviews := decodeHTTPData[domain.WorkflowReviewQueueResponse](t, reviewsRec.Body.Bytes())
	if len(reviews.PendingReview) == 0 {
		t.Fatalf("expected pending review items, got %+v", reviews)
	}

	approveRec := httptest.NewRecorder()
	approveReq := httptest.NewRequest(http.MethodPost, "/v1/workflows/forms/"+submitted.ID+"/approve", strings.NewReader(`{"reason":"http ok"}`))
	approveReq.Header.Set("Content-Type", "application/json")
	addIntegrationHeaders(approveReq, tenantID, approverID, "req-"+suffix+"-approve")
	handler.ServeHTTP(approveRec, approveReq)
	if approveRec.Code != http.StatusOK {
		t.Fatalf("approve expected 200, got %d: %s", approveRec.Code, approveRec.Body.String())
	}
	approved := decodeHTTPData[domain.FormInstance](t, approveRec.Body.Bytes())
	if approved.Status != "approved" {
		t.Fatalf("expected approved, got %+v", approved)
	}

	notifRec := httptest.NewRecorder()
	notifReq := httptest.NewRequest(http.MethodGet, "/v1/notifications", nil)
	addIntegrationHeaders(notifReq, tenantID, applicantID, "req-"+suffix+"-notif")
	handler.ServeHTTP(notifRec, notifReq)
	if notifRec.Code != http.StatusOK {
		t.Fatalf("notifications expected 200, got %d: %s", notifRec.Code, notifRec.Body.String())
	}
}

func decodeHTTPData[T any](t *testing.T, body []byte) T {
	t.Helper()
	var envelope struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode response: %v body=%s", err, string(body))
	}
	return envelope.Data
}
