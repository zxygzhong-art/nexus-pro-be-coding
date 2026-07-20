package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
	agentservice "nexus-pro-api/internal/service/agent"
)

// failingObjectStore simulates an object store outage with an error whose text
// leaks internal topology; it must never reach the API response message.
type failingObjectStore struct{ err error }

func (s *failingObjectStore) PutObject(context.Context, string, string, []byte) error { return s.err }
func (s *failingObjectStore) GetObject(context.Context, string) ([]byte, error)       { return nil, s.err }

var errObjectStoreOutage = errors.New("put https://sftpgo.internal:28080/tenants: 401 unauthorized")

func assertObjectStoreError(t *testing.T, err error) {
	t.Helper()
	appErr, ok := domain.AsAppError(err)
	if !ok {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Status != 502 || appErr.Code != "object_store_error" {
		t.Fatalf("expected 502 object_store_error, got status=%d code=%q message=%q", appErr.Status, appErr.Code, appErr.Message)
	}
	if strings.Contains(appErr.Message, "sftpgo.internal") || strings.Contains(appErr.Message, "401") {
		t.Fatalf("object store error message must not leak internals, got %q", appErr.Message)
	}
}

// TestUploadSessionFileObjectStoreFailure 驗證對話檔案儲存失敗以 502 固定文案返回。
func TestUploadSessionFileObjectStoreFailure(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
	})
	svc := service.New(store, service.Options{
		Now:         func() time.Time { return now },
		ObjectStore: &failingObjectStore{err: errObjectStoreOutage},
	})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	agentSvc := agentservice.New(svc)
	session, err := agentSvc.CreateSession(ctx, domain.CreateAgentSessionInput{Title: "Files"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = agentSvc.UploadSessionFile(ctx, session.ID, domain.UploadAgentSessionFileInput{
		Filename: "notes.txt", ContentType: "text/plain", Content: []byte("hello"),
	})
	assertObjectStoreError(t, err)
}

// TestUpdateEmployeeAvatarObjectStoreFailure 驗證頭像儲存失敗以 502 固定文案返回。
func TestUpdateEmployeeAvatarObjectStoreFailure(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "update", Scope: "all"},
	}, service.Options{ObjectStore: &failingObjectStore{err: errObjectStoreOutage}})
	now := time.Now().UTC()
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-avatar", TenantID: "tenant-1", EmployeeNo: "E4001", Name: "Avatar Person",
		CompanyEmail: "avatar.person@example.com", Status: "active", EmploymentStatus: "active",
		CreatedAt: now, UpdatedAt: now,
	})

	_, err := svc.HR().UpdateEmployeeAvatar(ctx, "emp-avatar", domain.EmployeeAvatarInput{
		Filename: "photo.png", ContentType: "image/png", Content: testPNGBytes(),
	})
	assertObjectStoreError(t, err)
}

// TestPreviewEmployeeImportObjectStoreFailure 驗證匯入檔案儲存失敗以 502 固定文案返回。
func TestPreviewEmployeeImportObjectStoreFailure(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-1", TenantID: "tenant-1", Name: "HQ", Path: []string{"ou-1"}, CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-hr", TenantID: "tenant-1", Name: "HR", CreatedAt: now,
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "import", Scope: "all"},
			{Resource: "hr.employee", Action: "read", Scope: "all"},
		},
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-hr"}, CreatedAt: now})
	svc := service.New(store, service.Options{ObjectStore: &failingObjectStore{err: errObjectStoreOutage}})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	_, err := svc.HR().PreviewEmployeeImport(ctx, domain.EmployeeImportPreviewInput{
		Filename: "employees.csv",
		Content:  "員工編號,姓名,Email,部門,職位,類別,電話,狀態,到職日期,主管員工ID\nE2001,Partial Wu,partial@example.com,ou-1,HRBP,全職,0911000222,在職,2026-06-01,\n",
	})
	assertObjectStoreError(t, err)
}
