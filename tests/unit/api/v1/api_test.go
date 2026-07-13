package v1_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	v1api "nexus-pro-be/internal/api/v1"
	"nexus-pro-be/internal/domain"
	platformauth "nexus-pro-be/internal/platform/auth"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

// newTestAPI 驗證 test API。
func newTestAPI(authenticated bool) http.Handler {
	store := memory.NewStore()
	populateDemoFixture(store)
	options := v1api.Options{}
	if authenticated {
		options.TokenResolver = staticTokenResolver{ctx: v1api.TokenContext{Provider: "keycloak", Subject: "acct-admin", TenantID: "demo"}, ok: true}
	}
	return v1api.New(service.New(store), nil, options).Routes()
}

func newTestAPIWithFormApprovalWorkflows(authenticated bool) http.Handler {
	store := memory.NewStore()
	populateDemoFixture(store)
	fake := &apiFakeFormApprovalWorkflowClient{started: map[string]domain.FormApprovalWorkflowStart{}}
	svc := service.New(store, service.Options{FormApprovalWorkflows: fake})
	fake.service = svc
	options := v1api.Options{}
	if authenticated {
		options.TokenResolver = staticTokenResolver{ctx: v1api.TokenContext{Provider: "keycloak", Subject: "acct-admin", TenantID: "demo"}, ok: true}
	}
	return v1api.New(svc, nil, options).Routes()
}

type apiFakeFormApprovalWorkflowClient struct {
	service *service.Service
	started map[string]domain.FormApprovalWorkflowStart
}

func (c *apiFakeFormApprovalWorkflowClient) StartFormApprovalWorkflow(_ context.Context, start domain.FormApprovalWorkflowStart) error {
	c.started[domain.FormApprovalWorkflowID(start.TenantID, start.FormInstanceID)] = start
	return nil
}

func (c *apiFakeFormApprovalWorkflowClient) SignalFormApprovalWorkflow(ctx context.Context, signal domain.FormApprovalWorkflowSignal) error {
	workflowID := domain.FormApprovalWorkflowID(signal.TenantID, signal.FormInstanceID)
	if _, ok := c.started[workflowID]; !ok {
		projection, err := c.service.Workflow().LoadTemporalFormApprovalProjection(domain.RequestContext{
			Context:  ctx,
			TenantID: signal.TenantID,
		}, signal.FormInstanceID)
		if err != nil {
			return err
		}
		if projection.RunID == "" {
			return domain.ErrFormApprovalWorkflowNotFound
		}
	}
	_, err := c.service.Workflow().ApplyTemporalFormApprovalSignal(domain.RequestContext{
		Context:   ctx,
		TenantID:  signal.TenantID,
		AccountID: signal.AccountID,
		RequestID: signal.RequestID,
		TraceID:   signal.TraceID,
	}, signal)
	return err
}

// newTestAPIForAccountNow builds an authenticated API with deterministic time for endpoint tests.
func newTestAPIForAccountNow(accountID string, now time.Time, mutateStore func(*memory.Store)) http.Handler {
	store := memory.NewStore()
	populateDemoFixture(store)
	if mutateStore != nil {
		mutateStore(store)
	}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	return v1api.New(svc, nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{
			Provider:  "keycloak",
			Subject:   accountID,
			TenantID:  "demo",
			AccountID: accountID,
		}, ok: true},
	}).Routes()
}

// decodeData 驗證 decode 資料。
func decodeData[T any](t *testing.T, body []byte) T {
	t.Helper()
	var payload struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Data
}

type apiErrorPayload struct {
	Code        domain.ErrorCode     `json:"code"`
	ReasonCode  string               `json:"reason_code"`
	FieldErrors []domain.FieldError  `json:"field_errors"`
	RowErrors   []apiRowErrorPayload `json:"row_errors"`
	TraceID     string               `json:"trace_id"`
}

type apiRowErrorPayload struct {
	RowNumber   int                 `json:"row_number"`
	FieldErrors []domain.FieldError `json:"field_errors"`
}

// decodeError 驗證 decode 錯誤。
func decodeError(t *testing.T, body []byte) apiErrorPayload {
	t.Helper()
	var payload struct {
		Error apiErrorPayload `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Error
}

// validEmployeeCreateJSON 驗證有效員工 create JSON。
func validEmployeeCreateJSON(name, email string) string {
	nationalID := "ID-" + strings.NewReplacer("@", "-", ".", "-", "+", "-").Replace(email)
	return `{"name":"` + name + `","company_email":"` + email + `","org_unit_id":"ou-hq","position":"Engineer","category":"full-time","employment_status":"待加入","hire_date":"2026-06-01","basic_info":{"nationality_type":"local","national_id":"` + nationalID + `"},"employment_info":{"org_unit_id":"ou-hq","position":"Engineer","category":"full_time"},"education_military_info":{"highest_education":"master","school":"NTU"},"contact_info":{"mobile_phone":"0911222333","address":"Taipei","emergency_contact_relation":"spouse","emergency_contact_name":"Emergency Contact","emergency_contact_phone":"0922333444"},"insurance_info":{"labor_insurance_date":"2026-06-01","labor_insurance_level":"L1","labor_insurance_salary":"45800","health_insurance_date":"2026-06-01","health_insurance_level":"H1","health_insurance_amount":"826"}}`
}

// avatarMultipartBody 驗證 avatar multipart body。
func avatarMultipartBody(t *testing.T) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="avatar.png"`)
	header.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(testPNGBytes()); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body, writer.FormDataContentType()
}

// testPNGBytes 驗證 png bytes。
func testPNGBytes() []byte {
	return []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0}
}

// TestProductionContextRequiresAuthenticatedContext 驗證 production context requires authenticated context。
func TestProductionContextRequiresAuthenticatedContext(t *testing.T) {
	handler := newTestAPI(false)
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without authenticated context, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestDefaultAPIRequiresAuthenticatedContext 驗證預設 API requires authenticated context。
func TestDefaultAPIRequiresAuthenticatedContext(t *testing.T) {
	store := memory.NewStore()
	populateDemoFixture(store)
	handler := v1api.New(service.New(store), nil).Routes()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected default API to require auth context, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestPlatformHomeEndpointRequiresMeReadPermission 驗證平台首頁 endpoint requires me.read 權限。
func TestPlatformHomeEndpointRequiresMeReadPermission(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 30, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	handler := newTestAPIForAccountNow("acct-no-platform-home", now, func(store *memory.Store) {
		ctx := context.Background()
		_ = store.UpsertAccount(ctx, domain.Account{
			ID:          "acct-no-platform-home",
			TenantID:    "demo",
			DisplayName: "No Platform Home",
			Email:       "no-platform-home@demo.local",
			EmployeeID:  "emp-employee",
			Status:      "active",
			CreatedAt:   now,
		})
		_ = store.UpsertUserIdentity(ctx, domain.UserIdentity{
			ID:        "uid-no-platform-home",
			TenantID:  "demo",
			AccountID: "acct-no-platform-home",
			Provider:  "keycloak",
			Subject:   "acct-no-platform-home",
			Email:     "no-platform-home@demo.local",
			CreatedAt: now,
		})
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/platform/home", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without me.read, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestPlatformHomeEndpointReturnsHomeProjection 驗證平台首頁 endpoint returns projection。
func TestPlatformHomeEndpointReturnsHomeProjection(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 30, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	handler := newTestAPIForAccountNow("acct-employee", now, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/platform/home", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for platform home, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeData[domain.PlatformHomeResponse](t, rec.Body.Bytes())
	if len(body.Assistants) == 0 || len(body.FormColumns) == 0 {
		t.Fatalf("expected assistants and form columns in home projection, got %+v", body)
	}
	if body.ClockSummary.CheckedOutAt != nil {
		t.Fatalf("unexpected clock summary: %+v", body.ClockSummary)
	}
}

// TestAttendanceClockRecordCreateRequiresCreatePermission 驗證打卡建立 endpoint requires attendance.clock.create。
func TestAttendanceClockRecordCreateRequiresCreatePermission(t *testing.T) {
	now := time.Date(2026, 7, 2, 8, 30, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	handler := newTestAPIForAccountNow("acct-hr-readonly", now, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/attendance/clock-records", strings.NewReader(`{"direction":"clock_in","latitude":25.033964,"longitude":121.564468,"accuracy_meters":10}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without attendance.clock.create, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestAttendanceClockRecordCreateRejectsInvalidBody 驗證打卡建立 endpoint rejects invalid body。
func TestAttendanceClockRecordCreateRejectsInvalidBody(t *testing.T) {
	now := time.Date(2026, 7, 2, 8, 30, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	handler := newTestAPIForAccountNow("acct-employee", now, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/attendance/clock-records", strings.NewReader(`{"direction":`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON body, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestAttendanceClockRecordDuplicatePunchReturnsConflict 驗證重複打卡不會新增打卡紀錄。
func TestAttendanceClockRecordDuplicatePunchReturnsConflict(t *testing.T) {
	now := time.Date(2026, 7, 2, 8, 30, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	handler := newTestAPIForAccountNow("acct-employee", now, nil)
	body := `{"direction":"clock_in","latitude":25.033964,"longitude":121.564468,"accuracy_meters":10,"location_source":"browser_geolocation"}`

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/attendance/clock-records", strings.NewReader(body))
	firstReq.Header.Set("Content-Type", "application/json")
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for first clock in, got %d: %s", firstRec.Code, firstRec.Body.String())
	}
	first := decodeData[domain.AttendanceClockRecord](t, firstRec.Body.Bytes())
	if first.RecordStatus != "accepted" || first.RejectionReason != "" {
		t.Fatalf("expected first clock in to be accepted, got %+v", first)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/attendance/clock-records", strings.NewReader(body))
	secondReq.Header.Set("Content-Type", "application/json")
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate clock in, got %d: %s", secondRec.Code, secondRec.Body.String())
	}
}

// TestProductionContextRejectsHeaderOnlyContext 驗證 production context rejects header only context。
func TestProductionContextRejectsHeaderOnlyContext(t *testing.T) {
	handler := newTestAPI(false)
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("X-Tenant-ID", "demo")
	req.Header.Set("X-Account-ID", "acct-admin")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for header-only production context, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestProductionContextAcceptsBearerClaims 驗證 production context accepts bearer claims。
func TestProductionContextAcceptsBearerClaims(t *testing.T) {
	store := memory.NewStore()
	populateDemoFixture(store)
	handler := v1api.New(service.New(store), nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{Provider: "keycloak", Subject: "acct-admin", TenantID: "demo", AccountID: "acct-other"}, ok: true},
	}).Routes()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with token-derived context, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestTokenContextTakesPrecedenceOverSpoofedHeaders 驗證 token context takes precedence over spoofed headers。
func TestTokenContextTakesPrecedenceOverSpoofedHeaders(t *testing.T) {
	store := memory.NewStore()
	populateDemoFixture(store)
	handler := v1api.New(service.New(store), nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{Provider: "keycloak", Subject: "acct-admin", TenantID: "demo", AccountID: "acct-admin"}, ok: true},
	}).Routes()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("X-Tenant-ID", "other-tenant")
	req.Header.Set("X-Account-ID", "acct-other")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 using token-derived context despite spoofed headers, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestIdentityMappingOverridesLegacyAccountClaim 驗證身分 mapping overrides legacy 帳號 claim。
func TestIdentityMappingOverridesLegacyAccountClaim(t *testing.T) {
	store := memory.NewStore()
	populateDemoFixture(store)
	now := time.Now().UTC()
	if err := store.UpsertUserIdentity(context.Background(), domain.UserIdentity{
		ID:        "uid-google-employee",
		TenantID:  "demo",
		AccountID: "acct-employee",
		Provider:  "keycloak",
		Subject:   "google-oauth2|123",
		Email:     "employee@demo.local",
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	handler := v1api.New(service.New(store), nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{
			Provider:  "keycloak",
			Subject:   "google-oauth2|123",
			TenantID:  "demo",
			AccountID: "acct-admin",
		}, ok: true},
	}).Routes()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with mapped identity, got %d: %s", rec.Code, rec.Body.String())
	}
	me := decodeData[domain.MeResponse](t, rec.Body.Bytes())
	if me.Account.ID != "acct-employee" {
		t.Fatalf("expected mapped account to win over token account claim, got %+v", me.Account)
	}
}

// TestUnlinkedExternalIdentityIsRejected 驗證 unlinked 外部身分 is rejected。
func TestUnlinkedExternalIdentityIsRejected(t *testing.T) {
	store := memory.NewStore()
	populateDemoFixture(store)
	handler := v1api.New(service.New(store), nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{
			Provider: "keycloak",
			Subject:  "unknown-subject",
			TenantID: "demo",
		}, ok: true},
	}).Routes()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unlinked external identity, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestUnlinkedExternalIdentityWithAccountClaimIsRejected 驗證 unlinked 外部身分 with 帳號 claim is rejected。
func TestUnlinkedExternalIdentityWithAccountClaimIsRejected(t *testing.T) {
	store := memory.NewStore()
	populateDemoFixture(store)
	handler := v1api.New(service.New(store), nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{
			Provider:  "keycloak",
			Subject:   "unknown-subject",
			TenantID:  "demo",
			AccountID: "acct-admin",
		}, ok: true},
	}).Routes()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unlinked external identity with account claim, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestBoundExternalIdentityWithoutTenantClaimIsAccepted 驗證缺少租戶 claim 的已綁定外部身分可解析。
func TestBoundExternalIdentityWithoutTenantClaimIsAccepted(t *testing.T) {
	store := memory.NewStore()
	populateDemoFixture(store)
	now := time.Now().UTC()
	if err := store.UpsertUserIdentity(context.Background(), domain.UserIdentity{
		ID:        "uid-google-no-tenant",
		TenantID:  "demo",
		AccountID: "acct-employee",
		Provider:  "keycloak",
		Subject:   "google-no-tenant",
		Email:     "employee@demo.local",
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	handler := v1api.New(service.New(store), nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{
			Provider: "keycloak",
			Subject:  "google-no-tenant",
			Email:    "employee@demo.local",
		}, ok: true},
	}).Routes()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for bound identity without tenant claim, got %d: %s", rec.Code, rec.Body.String())
	}
	me := decodeData[domain.MeResponse](t, rec.Body.Bytes())
	if me.Account.ID != "acct-employee" {
		t.Fatalf("expected bound account, got %+v", me.Account)
	}
}

// TestGoogleSSOVerifyBindsExistingActiveEmail 驗證 Google SSO email 成功時建立外部身分綁定。
func TestGoogleSSOVerifyBindsExistingActiveEmail(t *testing.T) {
	store := memory.NewStore()
	populateDemoFixture(store)
	handler := v1api.New(service.New(store), nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{
			Provider: "keycloak",
			Subject:  "google-subject-123",
			Email:    "employee@demo.local",
			Claims:   map[string]any{"email_verified": true},
		}, ok: true},
	}).Routes()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/sso/google/verify", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for authorized Google email, got %d: %s", rec.Code, rec.Body.String())
	}
	result := decodeData[domain.SSOLoginVerification](t, rec.Body.Bytes())
	if result.TenantID != "demo" || result.AccountID != "acct-employee" || result.Email != "employee@demo.local" {
		t.Fatalf("unexpected SSO verification result: %+v", result)
	}
	identity, ok, err := store.GetUserIdentity(context.Background(), "demo", "keycloak", "google-subject-123")
	if err != nil || !ok || identity.AccountID != "acct-employee" {
		t.Fatalf("expected Google subject to bind to employee account, identity=%+v ok=%v err=%v", identity, ok, err)
	}
}

// TestGoogleSSOVerifyRejectsUnverifiedEmail 驗證 Google email 未驗證時拒絕登入。
func TestGoogleSSOVerifyRejectsUnverifiedEmail(t *testing.T) {
	store := memory.NewStore()
	populateDemoFixture(store)
	handler := v1api.New(service.New(store), nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{
			Provider: "keycloak",
			Subject:  "google-unverified",
			Email:    "employee@demo.local",
			Claims:   map[string]any{"email_verified": false},
		}, ok: true},
	}).Routes()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/sso/google/verify", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unverified Google email, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "sso_email_unverified") {
		t.Fatalf("expected sso_email_unverified reason, got %s", rec.Body.String())
	}
}

// TestDisabledAccountIsRejectedAfterIdentityResolution 驗證 disabled 帳號 is rejected after 身分 resolution。
func TestDisabledAccountIsRejectedAfterIdentityResolution(t *testing.T) {
	store := memory.NewStore()
	populateDemoFixture(store)
	account, ok, err := store.GetAccount(context.Background(), "demo", "acct-admin")
	if err != nil || !ok {
		t.Fatalf("expected fixture account, ok=%v err=%v", ok, err)
	}
	account.Status = string(domain.AccountStatusDisabled)
	if err := store.UpsertAccount(context.Background(), account); err != nil {
		t.Fatal(err)
	}
	handler := v1api.New(service.New(store), nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{Provider: "keycloak", Subject: "acct-admin", TenantID: "demo", AccountID: "acct-admin"}, ok: true},
	}).Routes()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for disabled account, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestProductionContextRejectsUnsignedBearerFallback 驗證 production context rejects unsigned bearer fallback。
func TestProductionContextRejectsUnsignedBearerFallback(t *testing.T) {
	handler := newTestAPI(false)
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+testJWT(map[string]any{"tenant_id": "demo", "account_id": "acct-admin"}))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without configured production token resolver, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestKeycloakTokenResolverRefreshesJWKSWhenKidRotates 驗證 Keycloak token resolver refreshes JWKS when kid rotates。
func TestKeycloakTokenResolverRefreshesJWKSWhenKidRotates(t *testing.T) {
	oldKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	newKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	var issuer string
	var mu sync.RWMutex
	keys := map[string]*rsa.PublicKey{"old": &oldKey.PublicKey}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer":   issuer,
				"jwks_uri": issuer + "/certs",
			})
		case "/certs":
			mu.RLock()
			body := map[string]any{"keys": jwksFromKeys(keys)}
			mu.RUnlock()
			_ = json.NewEncoder(w).Encode(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	issuer = server.URL

	resolver := platformauth.NewKeycloakTokenResolver(issuer, "nexus-api", server.Client())
	oldReq := httptest.NewRequest(http.MethodGet, "/", nil)
	oldReq.Header.Set("Authorization", "Bearer "+signedRS256JWT(t, "old", oldKey, keycloakClaims(issuer)))
	if tokenCtx, ok, err := resolver.Resolve(oldReq); err != nil || !ok || tokenCtx.TenantID != "demo" || tokenCtx.AccountID != "acct-admin" {
		t.Fatalf("expected old key token to resolve, ctx=%+v ok=%v err=%v", tokenCtx, ok, err)
	}

	mu.Lock()
	keys = map[string]*rsa.PublicKey{"new": &newKey.PublicKey}
	mu.Unlock()
	newReq := httptest.NewRequest(http.MethodGet, "/", nil)
	newReq.Header.Set("Authorization", "Bearer "+signedRS256JWT(t, "new", newKey, keycloakClaims(issuer)))
	if tokenCtx, ok, err := resolver.Resolve(newReq); err != nil || !ok || tokenCtx.TenantID != "demo" || tokenCtx.AccountID != "acct-admin" {
		t.Fatalf("expected rotated key token to resolve, ctx=%+v ok=%v err=%v", tokenCtx, ok, err)
	}
}

// TestKeycloakTokenResolverCachesUnknownKidMisses 驗證 Keycloak token resolver caches unknown kid misses。
func TestKeycloakTokenResolverCachesUnknownKidMisses(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var issuer string
	var certFetches int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer":   issuer,
				"jwks_uri": issuer + "/certs",
			})
		case "/certs":
			certFetches++
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": jwksFromKeys(map[string]*rsa.PublicKey{"good": &key.PublicKey})})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	issuer = server.URL

	resolver := platformauth.NewKeycloakTokenResolver(issuer, "nexus-api", server.Client())
	goodReq := httptest.NewRequest(http.MethodGet, "/", nil)
	goodReq.Header.Set("Authorization", "Bearer "+signedRS256JWT(t, "good", key, keycloakClaims(issuer)))
	if _, ok, err := resolver.Resolve(goodReq); err != nil || !ok {
		t.Fatalf("expected good token to resolve, ok=%v err=%v", ok, err)
	}
	badToken := signedRS256JWT(t, "missing", key, keycloakClaims(issuer))
	for i := 0; i < 2; i++ {
		badReq := httptest.NewRequest(http.MethodGet, "/", nil)
		badReq.Header.Set("Authorization", "Bearer "+badToken)
		if _, _, err := resolver.Resolve(badReq); err == nil {
			t.Fatal("expected missing kid token to fail")
		}
	}
	if certFetches != 2 {
		t.Fatalf("expected one initial fetch and one forced miss refresh, got %d", certFetches)
	}
}

// TestKeycloakTokenResolverAllowsTenantlessSSOToken 驗證 SSO token 可先不帶 tenant claim。
func TestKeycloakTokenResolverAllowsTenantlessSSOToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var issuer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer":   issuer,
				"jwks_uri": issuer + "/certs",
			})
		case "/certs":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": jwksFromKeys(map[string]*rsa.PublicKey{"sso": &key.PublicKey})})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	issuer = server.URL

	claims := keycloakClaims(issuer)
	delete(claims, "aud")
	delete(claims, "tenant_id")
	delete(claims, "account_id")
	claims["azp"] = "nexus-api"
	claims["sub"] = "google-subject-123"
	claims["email"] = "employee@demo.local"
	claims["email_verified"] = true
	resolver := platformauth.NewKeycloakTokenResolver(issuer, "nexus-api", server.Client())
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/sso/google/verify", nil)
	req.Header.Set("Authorization", "Bearer "+signedRS256JWT(t, "sso", key, claims))

	tokenCtx, ok, err := resolver.Resolve(req)
	if err != nil || !ok {
		t.Fatalf("expected tenantless SSO token to resolve, ok=%v err=%v", ok, err)
	}
	if tokenCtx.TenantID != "" || tokenCtx.AccountID != "" || tokenCtx.Subject != "google-subject-123" || tokenCtx.Email != "employee@demo.local" {
		t.Fatalf("unexpected tenantless token context: %+v", tokenCtx)
	}
}

// TestKeycloakTokenResolverUsesRequestContextForJWKSRequests 驗證 Keycloak token resolver uses 請求 context for JWKS 請求。
func TestKeycloakTokenResolverUsesRequestContextForJWKSRequests(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer := "https://issuer.example"
	transport := &keycloakContextTransport{
		t:      t,
		issuer: issuer,
		keys:   map[string]*rsa.PublicKey{"ctx": &key.PublicKey},
	}
	resolver := platformauth.NewKeycloakTokenResolver(issuer, "nexus-api", &http.Client{Transport: transport})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), keycloakContextMarkerKey{}, "present"))
	req.Header.Set("Authorization", "Bearer "+signedRS256JWT(t, "ctx", key, keycloakClaims(issuer)))

	if tokenCtx, ok, err := resolver.Resolve(req); err != nil || !ok || tokenCtx.TenantID != "demo" || tokenCtx.AccountID != "acct-admin" {
		t.Fatalf("expected context-bound token to resolve, ctx=%+v ok=%v err=%v", tokenCtx, ok, err)
	}
	if transport.calls != 2 {
		t.Fatalf("expected discovery and JWKS requests, got %d calls", transport.calls)
	}
}

// TestKeycloakTokenResolverPingChecksDiscoveryAndJWKS 驗證 Keycloak token resolver ping checks discovery and JWKS。
func TestKeycloakTokenResolverPingChecksDiscoveryAndJWKS(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var issuer string
	var certFetches int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer":   issuer,
				"jwks_uri": issuer + "/certs",
			})
		case "/certs":
			certFetches++
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": jwksFromKeys(map[string]*rsa.PublicKey{"ready": &key.PublicKey})})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	issuer = server.URL

	resolver := platformauth.NewKeycloakTokenResolver(issuer, "nexus-api", server.Client())
	if err := resolver.Ping(context.Background()); err != nil {
		t.Fatalf("expected keycloak ping to verify discovery and JWKS, got %v", err)
	}
	if certFetches != 1 {
		t.Fatalf("expected one JWKS fetch, got %d", certFetches)
	}
}

// TestKeycloakTokenResolverPingFailsWhenJWKSUnavailable 驗證 Keycloak token resolver ping fails when JWKS unavailable。
func TestKeycloakTokenResolverPingFailsWhenJWKSUnavailable(t *testing.T) {
	var issuer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer":   issuer,
				"jwks_uri": issuer + "/certs",
			})
		case "/certs":
			http.Error(w, "jwks unavailable", http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	issuer = server.URL

	resolver := platformauth.NewKeycloakTokenResolver(issuer, "nexus-api", server.Client())
	if err := resolver.Ping(context.Background()); err == nil {
		t.Fatal("expected keycloak ping to fail when JWKS is unavailable")
	}
}

// TestDemoContextAllowsLocalRequests 驗證 demo context allows 本機請求。
func TestDemoContextAllowsLocalRequests(t *testing.T) {
	handler := newTestAPI(true)
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with demo context, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestReadinessEndpointReportsDependencyFailures 驗證就緒檢查 endpoint reports 依賴 failures。
func TestReadinessEndpointReportsDependencyFailures(t *testing.T) {
	handler := v1api.New(nil, nil, v1api.Options{
		ReadinessChecks: map[string]v1api.ReadinessCheck{
			"postgres": func(_ context.Context) error { return nil },
			"redis":    func(_ context.Context) error { return errors.New("redis unavailable") },
		},
	}).Routes()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for failed readiness check, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != "degraded" || payload.Checks["postgres"] != "ok" || payload.Checks["redis"] != "error" {
		t.Fatalf("unexpected readiness payload: %+v", payload)
	}
}

// TestRecoveryReturnsJSONInternalError 驗證 recovery returns JSON 內部錯誤。
func TestRecoveryReturnsJSONInternalError(t *testing.T) {
	handler := v1api.New(nil, nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{TenantID: "demo", AccountID: "acct-admin"}, ok: true},
	}).Routes()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("X-Request-ID", "panic-trace")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for recovered panic, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("expected JSON content type, got %q", rec.Header().Get("Content-Type"))
	}
	var payload struct {
		Error struct {
			Code    domain.ErrorCode `json:"code"`
			Message string           `json:"message"`
			TraceID string           `json:"trace_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error.Code != domain.ErrorCodeInternal || payload.Error.TraceID != "panic-trace" {
		t.Fatalf("unexpected recovered panic payload: %+v", payload)
	}
}

// TestSwaggerUIDisplaysOpenAPISpec 驗證 swagger ui displays OpenAPI spec。
func TestSwaggerUIDisplaysOpenAPISpec(t *testing.T) {
	handler := newTestAPI(false)

	uiReq := httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil)
	uiRec := httptest.NewRecorder()
	handler.ServeHTTP(uiRec, uiReq)
	if uiRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for swagger ui, got %d: %s", uiRec.Code, uiRec.Body.String())
	}
	if !strings.Contains(uiRec.Body.String(), `swagger-ui`) {
		t.Fatalf("expected packaged swagger ui markup, got: %s", uiRec.Body.String())
	}
	if !strings.Contains(uiRec.Body.String(), `swagger-initializer.js`) {
		t.Fatalf("expected packaged swagger initializer, got: %s", uiRec.Body.String())
	}
	if strings.Contains(uiRec.Body.String(), "unpkg.com") {
		t.Fatalf("expected swagger page to avoid unpkg assets, got: %s", uiRec.Body.String())
	}

	initReq := httptest.NewRequest(http.MethodGet, "/swagger/swagger-initializer.js", nil)
	initRec := httptest.NewRecorder()
	handler.ServeHTTP(initRec, initReq)
	if initRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for swagger initializer, got %d: %s", initRec.Code, initRec.Body.String())
	}
	if !strings.Contains(initRec.Body.String(), `url: "/openapi.yaml"`) {
		t.Fatalf("expected swagger initializer to load embedded OpenAPI spec, got: %s", initRec.Body.String())
	}

	cssReq := httptest.NewRequest(http.MethodGet, "/swagger/swagger-ui.css", nil)
	cssRec := httptest.NewRecorder()
	handler.ServeHTTP(cssRec, cssReq)
	if cssRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for packaged swagger ui css, got %d: %s", cssRec.Code, cssRec.Body.String())
	}

	specReq := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	specRec := httptest.NewRecorder()
	handler.ServeHTTP(specRec, specReq)
	if specRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for openapi spec, got %d: %s", specRec.Code, specRec.Body.String())
	}
	if !strings.Contains(specRec.Body.String(), "openapi: 3.0.3") {
		t.Fatalf("expected openapi yaml response, got: %s", specRec.Body.String())
	}
}

// TestAuthzCheckReturnsTargetSchema 驗證授權 check returns target schema。
func TestAuthzCheckReturnsTargetSchema(t *testing.T) {
	handler := newTestAPI(true)
	req := httptest.NewRequest(http.MethodPost, "/v1/authz/check", strings.NewReader(`{"application_code":"hr","resource_type":"employee","action":"export","resource_id":"emp-employee"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for authz check, got %d: %s", rec.Code, rec.Body.String())
	}
	result := decodeData[domain.CheckResult](t, rec.Body.Bytes())
	if !result.Allowed || result.ApplicationCode != "hr" || result.ResourceType != "employee" || result.ResourceID != "emp-employee" {
		t.Fatalf("unexpected authz result: %+v", result)
	}
	if result.RiskLevel != string(domain.RiskCritical) {
		t.Fatalf("expected export check to retain critical audit risk, got %+v", result)
	}
	if len(result.MatchedBy) == 0 || len(result.PermissionSetIDs) == 0 {
		t.Fatalf("expected matched sources and permission sets, got %+v", result)
	}
}

// TestCreatePermissionSetAssignmentEndpointWritesAssignment 驗證權限集合指派 endpoint writes 指派。
func TestCreatePermissionSetAssignmentEndpointWritesAssignment(t *testing.T) {
	handler := newTestAPI(true)
	req := httptest.NewRequest(http.MethodPost, "/v1/iam/permission-set-assignments", strings.NewReader(`{"principal_type":"account","principal_id":"acct-employee","permission_set_id":"ps-audit"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for assignment create, got %d: %s", rec.Code, rec.Body.String())
	}
	assignment := decodeData[domain.PermissionSetAssignment](t, rec.Body.Bytes())
	if assignment.PrincipalID != "acct-employee" || assignment.PermissionSetID != "ps-audit" || assignment.Effect != "allow" {
		t.Fatalf("unexpected assignment: %+v", assignment)
	}
}

// TestReadJSONRejectsMultipleValues 驗證 JSON rejects multiple values。
func TestReadJSONRejectsMultipleValues(t *testing.T) {
	handler := newTestAPI(true)
	req := httptest.NewRequest(http.MethodPost, "/v1/authz/check", strings.NewReader(`{"resource":"me","action":"read"} {}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for multiple JSON values, got %d: %s", rec.Code, rec.Body.String())
	}
	errPayload := decodeError(t, rec.Body.Bytes())
	if errPayload.Code != domain.ErrorCodeMultipleJSONValues {
		t.Fatalf("expected multiple JSON values code, got %+v", errPayload)
	}
}

// TestHighRiskRouteUsesGrantedPermissionWithoutApprovalHeader 驗證高風險路由只依既有權限決定是否放行。
func TestHighRiskRouteUsesGrantedPermissionWithoutApprovalHeader(t *testing.T) {
	handler := newTestAPI(true)
	req := httptest.NewRequest(http.MethodPost, "/v1/iam/user-groups", strings.NewReader(`{"name":"Finance Admin"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected granted high-risk route to succeed without approval header, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestLegacyApprovalHeaderDoesNotChangeAuthorization 驗證舊審批 header 不再參與授權決策。
func TestLegacyApprovalHeaderDoesNotChangeAuthorization(t *testing.T) {
	handler := newTestAPI(true)
	req := httptest.NewRequest(http.MethodPost, "/v1/iam/user-groups", strings.NewReader(`{"name":"Finance Admin"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Approval-Confirmed", "true")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected legacy approval header to be ignored, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestEHRMSEmployeeSyncRouteReachesServiceWithoutApprovalHeader 驗證有權限的同步請求直接進入業務服務。
func TestEHRMSEmployeeSyncRouteReachesServiceWithoutApprovalHeader(t *testing.T) {
	handler := newTestAPI(true)
	req := httptest.NewRequest(http.MethodPost, "/v1/hr/employees/ehrms/sync", strings.NewReader(`{"mode":"upsert"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected sync to reach service configuration check without approval header, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestAuditLogRouteAllowsGrantedRead 驗證稽核列表依 read 權限直接放行。
func TestAuditLogRouteAllowsGrantedRead(t *testing.T) {
	handler := newTestAPI(true)
	req := httptest.NewRequest(http.MethodGet, "/v1/audit-logs", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for granted audit log read, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestHRRouteForbiddenReasonCodes 驗證 HR 路由禁止 reason 碼。
func TestHRRouteForbiddenReasonCodes(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-limited", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	_ = store.UpsertUserIdentity(context.Background(), domain.UserIdentity{ID: "uid-limited", TenantID: "tenant-1", AccountID: "acct-limited", Provider: "keycloak", Subject: "acct-limited", CreatedAt: now})
	handler := v1api.New(service.New(store), nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{Provider: "keycloak", Subject: "acct-limited", TenantID: "tenant-1"}, ok: true},
	}).Routes()

	listReq := httptest.NewRequest(http.MethodGet, "/v1/hr/employees", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for missing HR read permission, got %d: %s", listRec.Code, listRec.Body.String())
	}
	listErr := decodeError(t, listRec.Body.Bytes())
	if listErr.Code != domain.ErrorCodeMenuDenied || listErr.ReasonCode != "menu_denied" {
		t.Fatalf("expected menu_denied reason code, got %+v", listErr)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/hr/employees", strings.NewReader(`{"name":"No Button","company_email":"no.button@example.com"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for missing HR create permission, got %d: %s", createRec.Code, createRec.Body.String())
	}
	createErr := decodeError(t, createRec.Body.Bytes())
	if createErr.Code != domain.ErrorCodeButtonDenied || createErr.ReasonCode != "button_denied" {
		t.Fatalf("expected button_denied reason code, got %+v", createErr)
	}
}

// TestAssumeRoleEndpointReturnsCreatedTypedResponse 驗證角色 endpoint returns created typed 回應。
func TestAssumeRoleEndpointReturnsCreatedTypedResponse(t *testing.T) {
	handler := newTestAPI(true)
	createReq := httptest.NewRequest(http.MethodPost, "/v1/iam/assumable-roles", strings.NewReader(`{"name":"Audit Assume","trusted":true,"trust_policy":{"accounts":["acct-admin"]},"permission_boundary":{"allow":["audit.log.read","iam.permission_set.read"]},"permission_set_ids":["ps-audit"],"session_duration_seconds":3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for role create, got %d: %s", createRec.Code, createRec.Body.String())
	}
	role := decodeData[domain.AssumableRole](t, createRec.Body.Bytes())

	assumeReq := httptest.NewRequest(http.MethodPost, "/v1/iam/assumable-roles/"+role.ID+"/assume", strings.NewReader(`{"reason":"test"}`))
	assumeReq.Header.Set("Content-Type", "application/json")
	assumeRec := httptest.NewRecorder()
	handler.ServeHTTP(assumeRec, assumeReq)
	if assumeRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for assume role, got %d: %s", assumeRec.Code, assumeRec.Body.String())
	}
	result := decodeData[domain.AssumeRoleResponse](t, assumeRec.Body.Bytes())
	if result.SessionID == "" || result.SessionToken != result.SessionID || result.AssumedRole.ID != role.ID {
		t.Fatalf("unexpected assume role response: %+v", result)
	}
	if regexp.MustCompile(`^sess-\d+-\d{6}$`).MatchString(result.SessionToken) {
		t.Fatalf("assume role session token should not use timestamp-counter format: %q", result.SessionToken)
	}
}

// TestBatchDeleteEmployeesReturnsMultiStatusOnRowFailure 驗證批次 delete 員工 returns multi 狀態 on 列 failure。
func TestBatchDeleteEmployeesReturnsMultiStatusOnRowFailure(t *testing.T) {
	handler := newTestAPI(true)
	req := httptest.NewRequest(http.MethodPost, "/v1/hr/employees/batch-delete", strings.NewReader(`{"employee_ids":["emp-employee","emp-missing"],"reason":"cleanup"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("expected 207 for partial batch delete, got %d: %s", rec.Code, rec.Body.String())
	}
	result := decodeData[domain.BatchEmployeeResponse](t, rec.Body.Bytes())
	if len(result.Results) != 2 || !result.Results[0].Success || result.Results[1].Success {
		t.Fatalf("unexpected batch delete result: %+v", result)
	}
}

// TestEmployeeListDetailAndCSVExportEndpoints 驗證員工列表 detail and CSV export endpoints。
func TestEmployeeListDetailAndCSVExportEndpoints(t *testing.T) {
	handler := newTestAPI(true)
	req := httptest.NewRequest(http.MethodGet, "/v1/hr/employees?page=1&page_size=2&sort=created_at_desc", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for employee list, got %d: %s", rec.Code, rec.Body.String())
	}
	page := decodeData[domain.PageResponse[domain.Employee]](t, rec.Body.Bytes())
	if page.Total != 10 || page.Page != 1 || page.PageSize != 2 || len(page.Items) != 2 {
		t.Fatalf("unexpected employee page: %+v", page)
	}

	badPageReq := httptest.NewRequest(http.MethodGet, "/v1/hr/employees?page=abc", nil)
	badPageRec := httptest.NewRecorder()
	handler.ServeHTTP(badPageRec, badPageReq)
	if badPageRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid page, got %d: %s", badPageRec.Code, badPageRec.Body.String())
	}
	badPageErr := decodeError(t, badPageRec.Body.Bytes())
	if badPageErr.Code != domain.ErrorCodeInvalidQueryInteger {
		t.Fatalf("expected invalid query integer code, got %+v", badPageErr)
	}

	badSizeReq := httptest.NewRequest(http.MethodGet, "/v1/hr/employees?page_size=101", nil)
	badSizeRec := httptest.NewRecorder()
	handler.ServeHTTP(badSizeRec, badSizeReq)
	if badSizeRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized page_size, got %d: %s", badSizeRec.Code, badSizeRec.Body.String())
	}
	badSizeErr := decodeError(t, badSizeRec.Body.Bytes())
	if badSizeErr.Code != domain.ErrorCodeQueryAboveMaximum {
		t.Fatalf("expected query above maximum code, got %+v", badSizeErr)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/v1/hr/employees/emp-admin", nil)
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for employee detail, got %d: %s", detailRec.Code, detailRec.Body.String())
	}
	detail := decodeData[domain.EmployeeDetail](t, detailRec.Body.Bytes())
	if detail.ID != "emp-admin" || detail.BasicInfo["national_id"] == "" || detail.Sections.BasicInfo.NationalID == "" {
		t.Fatalf("unexpected employee detail: %+v", detail)
	}

	exportReq := httptest.NewRequest(http.MethodGet, "/v1/hr/employees/export", nil)
	exportRec := httptest.NewRecorder()
	handler.ServeHTTP(exportRec, exportReq)
	if exportRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for employee CSV export, got %d: %s", exportRec.Code, exportRec.Body.String())
	}
	if !strings.Contains(exportRec.Header().Get("Content-Type"), "text/csv") || !strings.Contains(exportRec.Body.String(), "Demo Admin") {
		t.Fatalf("unexpected CSV export: content-type=%s body=%q", exportRec.Header().Get("Content-Type"), exportRec.Body.String())
	}
}

// TestEmployeeExportAuditUsesOpenTelemetryTraceID 驗證員工 export 稽核 uses open 遙測 trace ID。
func TestEmployeeExportAuditUsesOpenTelemetryTraceID(t *testing.T) {
	spanRecorder := installAPISpanRecorder(t)
	store := memory.NewStore()
	populateDemoFixture(store)
	handler := v1api.New(service.New(store), nil, v1api.Options{
		TokenResolver:        staticTokenResolver{ctx: v1api.TokenContext{Provider: "keycloak", Subject: "acct-admin", TenantID: "demo"}, ok: true},
		TelemetryServiceName: "nexus-pro-be-test",
	}).Routes()

	req := httptest.NewRequest(http.MethodGet, "/v1/hr/employees/export", nil)
	req.Header.Set("X-Request-ID", "req-export-trace")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for employee CSV export, got %d: %s", rec.Code, rec.Body.String())
	}
	logs, err := store.ListAuditLogs(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	exportLog, ok := findAPIAuditLog(logs, "hr.employee.export")
	if !ok {
		t.Fatalf("expected employee export audit log, got %+v", logs)
	}
	if exportLog.Result == "" {
		t.Fatalf("expected employee export audit result, got %+v", exportLog)
	}
	if exportLog.TraceID == "" || exportLog.TraceID == "req-export-trace" {
		t.Fatalf("expected audit trace_id from OpenTelemetry span, got %+v", exportLog)
	}
	if exportLog.Details["trace_id"] != exportLog.TraceID || exportLog.Details["request_id"] != "req-export-trace" {
		t.Fatalf("expected audit details to keep distinct trace_id and request_id, got %+v", exportLog.Details)
	}
	if !apiSpanEnded(spanRecorder, "GET /v1/hr/employees/export") {
		t.Fatalf("expected BFF span for employee export, got %v", apiSpanNames(spanRecorder))
	}
	if !apiSpanEnded(spanRecorder, "service.authz.authorize") {
		t.Fatalf("expected HR Core authz span, got %v", apiSpanNames(spanRecorder))
	}
}

// TestEmployeeImportPreviewConfirmAndValidationErrors 驗證員工 import preview confirm and 驗證錯誤。
func TestEmployeeImportPreviewConfirmAndValidationErrors(t *testing.T) {
	handler := newTestAPI(true)
	payload, _ := json.Marshal(map[string]string{
		"filename": "employees.csv",
		"content":  "員工編號,姓名,Email,部門,職位,類別,電話,狀態,到職日期,主管員工ID\nE9001,Nina Lin,nina@example.com,ou-hq,Recruiter,全職,0911888999,在職,2026-06-01,\n",
	})
	previewReq := httptest.NewRequest(http.MethodPost, "/v1/hr/employees/import/preview", strings.NewReader(string(payload)))
	previewReq.Header.Set("Content-Type", "application/json")
	previewRec := httptest.NewRecorder()

	handler.ServeHTTP(previewRec, previewReq)

	if previewRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for import preview, got %d: %s", previewRec.Code, previewRec.Body.String())
	}
	session := decodeData[domain.EmployeeImportSession](t, previewRec.Body.Bytes())
	if session.ID == "" || session.Summary["valid"].(float64) != 1 {
		t.Fatalf("unexpected preview session: %+v", session)
	}

	confirmReq := httptest.NewRequest(http.MethodPost, "/v1/hr/employees/import/"+session.ID+"/confirm", strings.NewReader(`{"mode":"create"}`))
	confirmReq.Header.Set("Content-Type", "application/json")
	confirmRec := httptest.NewRecorder()
	handler.ServeHTTP(confirmRec, confirmReq)
	if confirmRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for import confirm, got %d: %s", confirmRec.Code, confirmRec.Body.String())
	}

	duplicatePreviewReq := httptest.NewRequest(http.MethodPost, "/v1/hr/employees/import/preview", strings.NewReader(string(payload)))
	duplicatePreviewReq.Header.Set("Content-Type", "application/json")
	duplicatePreviewRec := httptest.NewRecorder()
	handler.ServeHTTP(duplicatePreviewRec, duplicatePreviewReq)
	if duplicatePreviewRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for duplicate import preview, got %d: %s", duplicatePreviewRec.Code, duplicatePreviewRec.Body.String())
	}
	duplicateSession := decodeData[domain.EmployeeImportSession](t, duplicatePreviewRec.Body.Bytes())
	duplicateConfirmReq := httptest.NewRequest(http.MethodPost, "/v1/hr/employees/import/"+duplicateSession.ID+"/confirm", strings.NewReader(`{"mode":"create"}`))
	duplicateConfirmReq.Header.Set("Content-Type", "application/json")
	duplicateConfirmRec := httptest.NewRecorder()
	handler.ServeHTTP(duplicateConfirmRec, duplicateConfirmReq)
	if duplicateConfirmRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for duplicate import confirm, got %d: %s", duplicateConfirmRec.Code, duplicateConfirmRec.Body.String())
	}
	duplicateErr := decodeError(t, duplicateConfirmRec.Body.Bytes())
	if duplicateErr.Code != domain.ErrorCodeFieldUnique || len(duplicateErr.RowErrors) == 0 || duplicateErr.RowErrors[0].RowNumber == 0 || len(duplicateErr.RowErrors[0].FieldErrors) == 0 {
		t.Fatalf("expected row_number and field_errors for import error, got %+v", duplicateErr)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/hr/employees", strings.NewReader(`{}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-Request-ID", "trace-test")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid employee create, got %d: %s", createRec.Code, createRec.Body.String())
	}
	var errPayload struct {
		Error struct {
			Code        domain.ErrorCode    `json:"code"`
			FieldErrors []domain.FieldError `json:"field_errors"`
			TraceID     string              `json:"trace_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &errPayload); err != nil {
		t.Fatal(err)
	}
	if errPayload.Error.Code != domain.ErrorCodeFieldRequired || len(errPayload.Error.FieldErrors) == 0 || errPayload.Error.TraceID != "trace-test" {
		t.Fatalf("expected validation error details, got %+v", errPayload)
	}
}

// TestEmployeeImportPreviewRejectsOversizedMultipartBody 驗證員工 import preview rejects oversized multipart body。
func TestEmployeeImportPreviewRejectsOversizedMultipartBody(t *testing.T) {
	handler := newTestAPI(true)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "employees.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(bytes.Repeat([]byte("x"), 17<<20)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/hr/employees/import/preview", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized multipart import, got %d: %s", rec.Code, rec.Body.String())
	}
	errPayload := decodeError(t, rec.Body.Bytes())
	if errPayload.Code != domain.ErrorCodeInvalidMultipartForm {
		t.Fatalf("expected invalid multipart form code for oversized multipart import, got %+v", errPayload)
	}
}

// TestEmployeeCreateStatusAndDeleteContract 驗證員工 create 狀態 and delete contract。
func TestEmployeeCreateStatusAndDeleteContract(t *testing.T) {
	handler := newTestAPI(true)
	createReq := httptest.NewRequest(http.MethodPost, "/v1/hr/employees", strings.NewReader(validEmployeeCreateJSON("Contract Person", "contract.person@example.com")))
	createReq.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, createReq)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for employee create, got %d: %s", rec.Code, rec.Body.String())
	}
	created := decodeData[domain.EmployeeDetail](t, rec.Body.Bytes())
	if created.EmployeeNo != "IKL001" || created.Category != "full_time" || created.EmploymentStatus != "onboarding" {
		t.Fatalf("expected generated number and normalized fields, got %+v", created)
	}
	if created.Sections.BasicInfo.NationalID == "" || created.Sections.EmploymentInfo.OrgUnitID != "ou-hq" {
		t.Fatalf("expected create response to include detail sections, got %+v", created.Sections)
	}

	statusReq := httptest.NewRequest(http.MethodPatch, "/v1/hr/employees/"+created.ID+"/status", strings.NewReader(`{"status":"試用中"}`))
	statusReq.Header.Set("Content-Type", "application/json")
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for status patch, got %d: %s", statusRec.Code, statusRec.Body.String())
	}
	updated := decodeData[domain.Employee](t, statusRec.Body.Bytes())
	if updated.EmploymentStatus != "probation" {
		t.Fatalf("expected normalized probation status, got %+v", updated)
	}

	resignReq := httptest.NewRequest(http.MethodPatch, "/v1/hr/employees/"+created.ID+"/status", strings.NewReader(`{"status":"resigned"}`))
	resignReq.Header.Set("Content-Type", "application/json")
	resignRec := httptest.NewRecorder()
	handler.ServeHTTP(resignRec, resignReq)
	if resignRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when direct status patch tries to resign, got %d: %s", resignRec.Code, resignRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/hr/employees/"+created.ID, nil)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for confirmed delete, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/hr/employees?keyword=contract.person@example.com", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for list after delete, got %d: %s", listRec.Code, listRec.Body.String())
	}
	page := decodeData[domain.PageResponse[domain.Employee]](t, listRec.Body.Bytes())
	if page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("expected soft-deleted employee to be hidden by default, got %+v", page)
	}
}

// TestEmployeePreviewAvatarTemplateAndWorkflowApproveRoutes 驗證員工 preview avatar 範本 and 流程核准路由。
func TestEmployeePreviewAvatarTemplateAndWorkflowApproveRoutes(t *testing.T) {
	handler := newTestAPIWithFormApprovalWorkflows(true)

	previewReq := httptest.NewRequest(http.MethodPost, "/v1/hr/employees/preview", strings.NewReader(`{}`))
	previewReq.Header.Set("Content-Type", "application/json")
	previewRec := httptest.NewRecorder()
	handler.ServeHTTP(previewRec, previewReq)
	if previewRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for invalid employee preview response, got %d: %s", previewRec.Code, previewRec.Body.String())
	}
	preview := decodeData[domain.EmployeePreviewResponse](t, previewRec.Body.Bytes())
	if preview.Valid || len(preview.FieldErrors) == 0 {
		t.Fatalf("expected preview validation details, got %+v", preview)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/hr/employees", strings.NewReader(validEmployeeCreateJSON("Route Person", "route.person@example.com")))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for employee create, got %d: %s", createRec.Code, createRec.Body.String())
	}
	created := decodeData[domain.EmployeeDetail](t, createRec.Body.Bytes())

	updatePreviewReq := httptest.NewRequest(http.MethodPost, "/v1/hr/employees/"+created.ID+"/preview", strings.NewReader(`{"name":"Updated Route Person"}`))
	updatePreviewReq.Header.Set("Content-Type", "application/json")
	updatePreviewRec := httptest.NewRecorder()
	handler.ServeHTTP(updatePreviewRec, updatePreviewReq)
	if updatePreviewRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for employee update preview, got %d: %s", updatePreviewRec.Code, updatePreviewRec.Body.String())
	}
	updatePreview := decodeData[domain.EmployeePreviewResponse](t, updatePreviewRec.Body.Bytes())
	if !updatePreview.Valid || updatePreview.Employee.Name != "Updated Route Person" || updatePreview.Diff["name"] == nil {
		t.Fatalf("expected update preview diff, got %+v", updatePreview)
	}

	nonBasicUpdateReq := httptest.NewRequest(http.MethodPatch, "/v1/hr/employees/"+created.ID, strings.NewReader(`{"position":"Lead Engineer"}`))
	nonBasicUpdateReq.Header.Set("Content-Type", "application/json")
	nonBasicUpdateRec := httptest.NewRecorder()
	handler.ServeHTTP(nonBasicUpdateRec, nonBasicUpdateReq)
	if nonBasicUpdateRec.Code != http.StatusBadRequest || !strings.Contains(nonBasicUpdateRec.Body.String(), "basic_info_only") {
		t.Fatalf("expected non-basic employee update rejection, got %d: %s", nonBasicUpdateRec.Code, nonBasicUpdateRec.Body.String())
	}

	templateReq := httptest.NewRequest(http.MethodGet, "/v1/hr/employees/import/template?format=csv", nil)
	templateRec := httptest.NewRecorder()
	handler.ServeHTTP(templateRec, templateReq)
	if templateRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for import template, got %d: %s", templateRec.Code, templateRec.Body.String())
	}
	if !strings.Contains(templateRec.Header().Get("Content-Type"), "text/csv") || !bytes.HasPrefix(templateRec.Body.Bytes(), []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("expected csv template response with BOM, headers=%v body=%q", templateRec.Header(), templateRec.Body.String())
	}
	badTemplateReq := httptest.NewRequest(http.MethodGet, "/v1/hr/employees/import/template?format=pdf", nil)
	badTemplateRec := httptest.NewRecorder()
	handler.ServeHTTP(badTemplateRec, badTemplateReq)
	if badTemplateRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad template format, got %d: %s", badTemplateRec.Code, badTemplateRec.Body.String())
	}

	avatarBody, avatarContentType := avatarMultipartBody(t)
	avatarReq := httptest.NewRequest(http.MethodPost, "/v1/hr/employees/"+created.ID+"/avatar", avatarBody)
	avatarReq.Header.Set("Content-Type", avatarContentType)
	avatarRec := httptest.NewRecorder()
	handler.ServeHTTP(avatarRec, avatarReq)
	if avatarRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for avatar upload, got %d: %s", avatarRec.Code, avatarRec.Body.String())
	}
	withAvatar := decodeData[domain.Employee](t, avatarRec.Body.Bytes())
	if got, ok := withAvatar.BasicInfo["avatar_object_key"].(string); !ok || !strings.HasPrefix(got, "employee-avatars/demo/"+created.ID+"/") {
		t.Fatalf("expected avatar object key in response, got %+v", withAvatar.BasicInfo)
	}
	deleteAvatarReq := httptest.NewRequest(http.MethodDelete, "/v1/hr/employees/"+created.ID+"/avatar", nil)
	deleteAvatarRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteAvatarRec, deleteAvatarReq)
	if deleteAvatarRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for avatar delete, got %d: %s", deleteAvatarRec.Code, deleteAvatarRec.Body.String())
	}
	withoutAvatar := decodeData[domain.Employee](t, deleteAvatarRec.Body.Bytes())
	if _, ok := withoutAvatar.BasicInfo["avatar_object_key"]; ok {
		t.Fatalf("expected avatar metadata removed, got %+v", withoutAvatar.BasicInfo)
	}

	templateReqBody := `{"key":"approval-evidence","name":"Approval Evidence","schema":{"type":"object","properties":{"subject":{"type":"string"}},"workspace_design":{"stages":[{"id":"stage-admin","type":"approver","label":"審核","detail":"由管理員審核","config":{"account_ids":["acct-admin"]}}]}}}`
	createTemplateReq := httptest.NewRequest(http.MethodPost, "/v1/forms/templates", strings.NewReader(templateReqBody))
	createTemplateReq.Header.Set("Content-Type", "application/json")
	createTemplateRec := httptest.NewRecorder()
	handler.ServeHTTP(createTemplateRec, createTemplateReq)
	if createTemplateRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for workflow template create, got %d: %s", createTemplateRec.Code, createTemplateRec.Body.String())
	}

	workflowApprovalPayload := `{"payload":{"application_code":"hr","resource_type":"employee","action":"export","filters":{"employment_status":"active"},"subject":"approval evidence test"}}`
	submitReq := httptest.NewRequest(http.MethodPost, "/v1/workflows/forms/approval-evidence/submit", strings.NewReader(workflowApprovalPayload))
	submitReq.Header.Set("Content-Type", "application/json")
	submitRec := httptest.NewRecorder()
	handler.ServeHTTP(submitRec, submitReq)
	if submitRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for workflow form submit, got %d: %s", submitRec.Code, submitRec.Body.String())
	}
	form := decodeData[domain.FormInstance](t, submitRec.Body.Bytes())
	draftReq := httptest.NewRequest(http.MethodPost, "/v1/workflows/forms/drafts", strings.NewReader(`{"template_key":"approval-evidence","payload":{"application_code":"hr","resource_type":"employee","action":"export","subject":"draft approval evidence test"}}`))
	draftReq.Header.Set("Content-Type", "application/json")
	draftRec := httptest.NewRecorder()
	handler.ServeHTTP(draftRec, draftReq)
	if draftRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for workflow draft create, got %d: %s", draftRec.Code, draftRec.Body.String())
	}
	draft := decodeData[domain.FormInstance](t, draftRec.Body.Bytes())
	submitDraftReq := httptest.NewRequest(http.MethodPost, "/v1/workflows/forms/"+draft.ID+"/submit", strings.NewReader(workflowApprovalPayload))
	submitDraftReq.Header.Set("Content-Type", "application/json")
	submitDraftRec := httptest.NewRecorder()
	handler.ServeHTTP(submitDraftRec, submitDraftReq)
	if submitDraftRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for workflow draft submit, got %d: %s", submitDraftRec.Code, submitDraftRec.Body.String())
	}
	submittedDraft := decodeData[domain.FormInstance](t, submitDraftRec.Body.Bytes())
	if submittedDraft.ID != draft.ID || submittedDraft.Status != "in_review" {
		t.Fatalf("expected draft id to be submitted in place, draft=%s got %+v", draft.ID, submittedDraft)
	}
	approveReq := httptest.NewRequest(http.MethodPost, "/v1/workflows/forms/"+form.ID+"/approve", strings.NewReader(`{"reason":"frontend approval"}`))
	approveReq.Header.Set("Content-Type", "application/json")
	approveRec := httptest.NewRecorder()
	handler.ServeHTTP(approveRec, approveReq)
	if approveRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for workflow approve, got %d: %s", approveRec.Code, approveRec.Body.String())
	}
	approved := decodeData[domain.FormInstance](t, approveRec.Body.Bytes())
	if approved.Status != "approved" || approved.ApprovedBy != "acct-admin" {
		t.Fatalf("expected approved form instance, got %+v", approved)
	}
	review, _ := approved.Payload["_review"].(map[string]any)
	if review["type"] != "approve" || review["comment"] != "frontend approval" {
		t.Fatalf("expected approve reason in workflow review payload, got %+v", approved.Payload)
	}

	forgedApproveReq := httptest.NewRequest(http.MethodPost, "/v1/workflows/forms/"+form.ID+"/approve", strings.NewReader(`{"approved_by":"acct-other"}`))
	forgedApproveReq.Header.Set("Content-Type", "application/json")
	forgedApproveRec := httptest.NewRecorder()
	handler.ServeHTTP(forgedApproveRec, forgedApproveReq)
	if forgedApproveRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for forged approved_by, got %d: %s", forgedApproveRec.Code, forgedApproveRec.Body.String())
	}

	exportReq := httptest.NewRequest(http.MethodGet, "/v1/hr/employees/export?employment_status=active", nil)
	exportRec := httptest.NewRecorder()
	handler.ServeHTTP(exportRec, exportReq)
	if exportRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for permission-authorized export, got %d: %s", exportRec.Code, exportRec.Body.String())
	}
}

// TestListResponsesUsePageEnvelope 驗證回應 use 分頁 envelope。
func TestListResponsesUsePageEnvelope(t *testing.T) {
	handler := newTestAPI(true)
	req := httptest.NewRequest(http.MethodGet, "/v1/iam/user-groups?page=1&page_size=1", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for user group page, got %d: %s", rec.Code, rec.Body.String())
	}
	page := decodeData[domain.PageResponse[domain.UserGroup]](t, rec.Body.Bytes())
	if page.Total == 0 || page.Page != 1 || page.PageSize != 1 || len(page.Items) != 1 {
		t.Fatalf("unexpected page envelope: %+v", page)
	}

	badReq := httptest.NewRequest(http.MethodGet, "/v1/iam/user-groups?page_size=101", nil)
	badRec := httptest.NewRecorder()
	handler.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized page_size, got %d: %s", badRec.Code, badRec.Body.String())
	}
}

// testJWT 驗證 JWT。
func testJWT(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payloadBytes, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	return header + "." + payload + "."
}

// keycloakClaims 驗證 Keycloak claims。
func keycloakClaims(issuer string) map[string]any {
	return map[string]any{
		"iss":        issuer,
		"aud":        "nexus-api",
		"exp":        time.Now().Add(time.Hour).Unix(),
		"sub":        "acct-admin",
		"tenant_id":  "demo",
		"account_id": "acct-admin",
	}
}

// jwksFromKeys 驗證 JWKS 來源 keys。
func jwksFromKeys(keys map[string]*rsa.PublicKey) []map[string]string {
	out := make([]map[string]string, 0, len(keys))
	for kid, key := range keys {
		out = append(out, map[string]string{
			"kid": kid,
			"kty": "RSA",
			"alg": "RS256",
			"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		})
	}
	return out
}

// signedRS256JWT 驗證 signed rs 256 JWT。
func signedRS256JWT(t *testing.T, kid string, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	header, err := json.Marshal(map[string]string{"alg": "RS256", "kid": kid, "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	sum := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

type staticTokenResolver struct {
	ctx v1api.TokenContext
	ok  bool
	err error
}

type keycloakContextMarkerKey struct{}

type keycloakContextTransport struct {
	t      *testing.T
	issuer string
	keys   map[string]*rsa.PublicKey
	calls  int
}

// RoundTrip 驗證 round trip。
func (t *keycloakContextTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.calls++
	if got := req.Context().Value(keycloakContextMarkerKey{}); got != "present" {
		t.t.Fatalf("expected request context marker to propagate, got %v", got)
	}

	var payload any
	switch req.URL.Path {
	case "/.well-known/openid-configuration":
		payload = map[string]string{
			"issuer":   t.issuer,
			"jwks_uri": t.issuer + "/certs",
		}
	case "/certs":
		payload = map[string]any{"keys": jwksFromKeys(t.keys)}
	default:
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Request: req}, nil
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(raw))),
		Request:    req,
	}, nil
}

// Resolve 驗證目標路徑。
func (r staticTokenResolver) Resolve(*http.Request) (v1api.TokenContext, bool, error) {
	return r.ctx, r.ok, r.err
}

// installAPISpanRecorder 驗證 install API span recorder。
func installAPISpanRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	return recorder
}

// apiSpanEnded 驗證 API span ended。
func apiSpanEnded(recorder *tracetest.SpanRecorder, name string) bool {
	for _, span := range recorder.Ended() {
		if span.Name() == name {
			return true
		}
	}
	return false
}

// apiSpanNames 驗證 API span names。
func apiSpanNames(recorder *tracetest.SpanRecorder) []string {
	names := make([]string, 0)
	for _, span := range recorder.Ended() {
		names = append(names, span.Name())
	}
	return names
}

// findAPIAuditLog 驗證 find API 稽核 log。
func findAPIAuditLog(logs []domain.AuditLog, action string) (domain.AuditLog, bool) {
	for _, log := range logs {
		if log.Action == action {
			return log, true
		}
	}
	return domain.AuditLog{}, false
}
