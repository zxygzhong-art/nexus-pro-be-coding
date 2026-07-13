package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

// TestWorkspaceFormDesignUsesGloballyUniqueTemplateIDs 驗證不同租戶使用相同表單 key 時不再碰撞主鍵。
func TestWorkspaceFormDesignUsesGloballyUniqueTemplateIDs(t *testing.T) {
	store := memory.NewStore()
	svc := service.New(store)
	for _, tenantID := range []string{"tenant-a", "tenant-b"} {
		seedFormDesignTenant(t, store, tenantID)
		ctx := domain.RequestContext{TenantID: tenantID, AccountID: "acct-admin"}
		if _, err := svc.Workspace().CreateWorkspaceFormDesign(ctx, domain.SaveWorkspaceFormDesignInput{
			ID: "shared-form", Name: "共用名稱", Category: "自訂",
			Stages: []domain.PlatformFormBuilderStage{{
				ID: "stage-1", Type: "approver", Label: "主管審批", Config: map[string]any{"role": "manager"},
			}},
		}); err != nil {
			t.Fatalf("create form for %s: %v", tenantID, err)
		}
	}

	first, ok, err := store.GetFormTemplateByKey(context.Background(), "tenant-a", "shared-form")
	if err != nil || !ok {
		t.Fatalf("tenant-a template missing: ok=%v err=%v", ok, err)
	}
	second, ok, err := store.GetFormTemplateByKey(context.Background(), "tenant-b", "shared-form")
	if err != nil || !ok {
		t.Fatalf("tenant-b template missing: ok=%v err=%v", ok, err)
	}
	if first.ID == second.ID {
		t.Fatalf("expected globally unique template IDs, both tenants got %q", first.ID)
	}
}

// seedFormDesignTenant 建立表單設計測試所需的最小租戶與權限資料。
func seedFormDesignTenant(t *testing.T, store *memory.Store, tenantID string) {
	t.Helper()
	now := time.Date(2026, 7, 13, 6, 0, 0, 0, time.UTC)
	ctx := context.Background()
	if err := store.UpsertTenant(ctx, domain.Tenant{ID: tenantID, Name: tenantID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(ctx, domain.PermissionSet{
		ID: "ps-form-admin", TenantID: tenantID, Name: "Form Admin", CreatedAt: now,
		Permissions: []domain.Permission{
			{Resource: "workflow.form_template", Action: "create", Scope: "all"},
			{Resource: "workflow.form_template", Action: "read", Scope: "all"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(ctx, domain.Account{
		ID: "acct-admin", TenantID: tenantID, Status: "active", DirectPermissionSetIDs: []string{"ps-form-admin"}, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
}
