package postgres_integration_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1api "nexus-pro-api/internal/api/v1"
	"nexus-pro-api/internal/domain"
	postgresrepo "nexus-pro-api/internal/repository/postgres"
	"nexus-pro-api/internal/service"
)

// TestFormInstanceFieldValueBooleanProjection 驗證 boolean 投影欄位在 Postgres 上可寫入並讀回。
// 2026-07-17 缺陷:InsertFormInstanceFieldValue 的 boolean CASE 分支缺少 ::boolean cast,
// Postgres PREPARE 報 42804,導致含 analytics.reportable 布林欄位的表單提交全部 500。
func TestFormInstanceFieldValueBooleanProjection(t *testing.T) {
	pool := openIntegrationPool(t)
	defer pool.Close()
	requireMigratedSchema(t, pool)
	store := postgresrepo.NewStore(pool)
	now := time.Now().UTC()
	suffix := strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_") + "_" + now.Format("150405000000")
	tenantID := "tenant_" + suffix
	ctx := tenantScopedContext(tenantID)

	if err := store.UpsertTenant(ctx, domain.Tenant{ID: tenantID, Name: tenantID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	templateID := "ft_" + suffix
	if err := store.UpsertFormTemplate(ctx, domain.FormTemplate{
		ID: templateID, TenantID: tenantID, Key: "overtime-approval", Name: "加班申請",
		Schema: map[string]any{"type": "object"}, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	applicantID := "acct_" + suffix
	if err := store.UpsertAccount(ctx, domain.Account{
		ID: applicantID, TenantID: tenantID, DisplayName: "Applicant", Status: "active", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	instanceID := "fi_" + suffix
	if err := store.UpsertFormInstance(ctx, domain.FormInstance{
		ID: instanceID, TenantID: tenantID, TemplateID: templateID, ApplicantAccountID: applicantID,
		Status: "submitted", SubmittedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	instance, ok, err := store.GetFormInstance(ctx, tenantID, instanceID)
	if err != nil || !ok {
		t.Fatalf("expected stored form instance, ok=%v err=%v", ok, err)
	}

	boolTrue := true
	values := []domain.FormInstanceFieldValue{
		{
			TenantID: tenantID, FormInstanceID: instanceID, TemplateID: templateID,
			TemplateVersionID: instance.TemplateVersionID,
			FieldID:           "overtime_confirmed", ValueType: "boolean", ValueBoolean: &boolTrue, CreatedAt: now,
		},
		{
			TenantID: tenantID, FormInstanceID: instanceID, TemplateID: templateID,
			TemplateVersionID: instance.TemplateVersionID,
			FieldID:           "reason", ValueType: "text", ValueText: "projection check", CreatedAt: now,
		},
	}
	if err := store.ReplaceFormInstanceFieldValues(ctx, tenantID, instanceID, values); err != nil {
		t.Fatalf("boolean projection insert must not fail with 42804: %v", err)
	}
	stored, err := store.ListFormInstanceFieldValues(ctx, tenantID, instanceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 2 {
		t.Fatalf("expected 2 projected values, got %+v", stored)
	}
	var booleanValue *domain.FormInstanceFieldValue
	for i := range stored {
		if stored[i].FieldID == "overtime_confirmed" {
			booleanValue = &stored[i]
		}
	}
	if booleanValue == nil || booleanValue.ValueBoolean == nil || !*booleanValue.ValueBoolean {
		t.Fatalf("expected boolean projection round-trip true, got %+v", stored)
	}
}

// TestAttendanceCorrectionHTTPValidation 驗證補卡 reason 長度校驗(P1)與非 pending 審核回 409(P2)。
func TestAttendanceCorrectionHTTPValidation(t *testing.T) {
	pool := openIntegrationPool(t)
	defer pool.Close()
	requireMigratedSchema(t, pool)
	store := postgresrepo.NewStore(pool)
	now := time.Now().UTC()
	suffix := strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_") + "_" + now.Format("150405000000")
	tenantID := "tenant_" + suffix
	applicantID := "acct_" + suffix + "_applicant"
	approverID := "acct_" + suffix + "_approver"
	employeeID := "emp_" + suffix
	ctx := tenantScopedContext(tenantID)

	if err := store.UpsertTenant(ctx, domain.Tenant{ID: tenantID, Name: tenantID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	for _, item := range []domain.PermissionSet{
		{
			ID: "ps_" + suffix + "_applicant", TenantID: tenantID, Name: "Applicant",
			Permissions: []domain.Permission{
				{Resource: "attendance.correction", Action: "create", Scope: "self"},
				{Resource: "attendance.correction", Action: "read", Scope: "self"},
			},
			CreatedAt: now,
		},
		{
			ID: "ps_" + suffix + "_approver", TenantID: tenantID, Name: "Approver",
			Permissions: []domain.Permission{
				{Resource: "attendance.correction", Action: "read", Scope: "all"},
				{Resource: "attendance.correction", Action: "approve", Scope: "all"},
				{Resource: "attendance.correction", Action: "update", Scope: "all"},
			},
			CreatedAt: now,
		},
	} {
		if err := store.UpsertPermissionSet(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpsertAccount(ctx, domain.Account{
		ID: applicantID, TenantID: tenantID, DisplayName: "Applicant", EmployeeID: employeeID, Status: "active",
		DirectPermissionSetIDs: []string{"ps_" + suffix + "_applicant"}, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEmployee(ctx, domain.Employee{
		ID: employeeID, TenantID: tenantID, Name: "Applicant", AccountID: applicantID,
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
	// approve add_record 會補一筆 manual 打卡,需要一個 active 打卡點。
	if err := store.UpsertAttendanceWorksite(ctx, domain.AttendanceWorksite{
		ID: "ws_" + suffix, TenantID: tenantID, Name: "HQ",
		Latitude: 25.033, Longitude: 121.565, RadiusMeters: 300,
		Status: "active", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	svc := newIntegrationServiceWithFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return now }})
	handler := v1api.New(svc, nil, v1api.Options{TokenResolver: integrationTokenResolver{}}).Routes()

	doRequest := func(accountID, requestID, method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		addIntegrationHeaders(req, tenantID, accountID, requestID)
		handler.ServeHTTP(rec, req)
		return rec
	}

	createBody := func(reason string) string {
		return `{"correction_type":"add_record","direction":"clock_in","requested_clocked_at":"2026-07-16T09:00:00Z","reason":` +
			`"` + reason + `"}`
	}

	// P1: create reason 少於 4 字元一律 400。
	for _, reason := range []string{"x", "abc"} {
		rec := doRequest(applicantID, "req-"+suffix+"-create-short-"+reason, http.MethodPost, "/v1/attendance/corrections", createBody(reason))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("create with %d-char reason expected 400, got %d: %s", len(reason), rec.Code, rec.Body.String())
		}
	}
	// P1: create reason 超過 200 字元 400。
	longReason := strings.Repeat("原", 201)
	rec := doRequest(applicantID, "req-"+suffix+"-create-long", http.MethodPost, "/v1/attendance/corrections", createBody(longReason))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create with 201-char reason expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	// P1: 4 字元(下限)可建立。
	rec = doRequest(applicantID, "req-"+suffix+"-create-ok", http.MethodPost, "/v1/attendance/corrections", createBody("忘記打卡"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create with 4-char reason expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	created := decodeHTTPData[domain.AttendanceCorrectionRequest](t, rec.Body.Bytes())
	if created.Status != "pending" {
		t.Fatalf("expected pending correction, got %+v", created)
	}

	// P1: approve reason 提供時少於 2 字元 400。
	rec = doRequest(approverID, "req-"+suffix+"-approve-short", http.MethodPost, "/v1/attendance/corrections/"+created.ID+"/approve", `{"reason":"y"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("approve with 1-char reason expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	// P2: 不存在 id 仍為 404。
	rec = doRequest(approverID, "req-"+suffix+"-approve-missing", http.MethodPost, "/v1/attendance/corrections/acorr_missing_"+suffix+"/approve", `{"reason":"ok"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("approve missing correction expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
	// 正常 approve → 200 approved。
	rec = doRequest(approverID, "req-"+suffix+"-approve-ok", http.MethodPost, "/v1/attendance/corrections/"+created.ID+"/approve", `{"reason":"ok"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	approved := decodeHTTPData[domain.AttendanceCorrectionRequest](t, rec.Body.Bytes())
	if approved.Status != "approved" {
		t.Fatalf("expected approved, got %+v", approved)
	}
	// P2: 對非 pending 記錄再次 approve → 409。
	rec = doRequest(approverID, "req-"+suffix+"-approve-again", http.MethodPost, "/v1/attendance/corrections/"+created.ID+"/approve", `{"reason":"ok"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("re-approve expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	// P2: 對非 pending 記錄 reject → 409。
	rec = doRequest(approverID, "req-"+suffix+"-reject-after-approve", http.MethodPost, "/v1/attendance/corrections/"+created.ID+"/reject", `{"reason":"no"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("reject after approve expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	// P2: reject 不存在 id 仍為 404。
	rec = doRequest(approverID, "req-"+suffix+"-reject-missing", http.MethodPost, "/v1/attendance/corrections/acorr_missing_"+suffix+"/reject", `{"reason":"no"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("reject missing correction expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}
