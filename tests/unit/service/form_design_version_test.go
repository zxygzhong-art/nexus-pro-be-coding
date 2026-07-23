package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
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

// TestWorkspaceFormEditKeepsPublishedVersionUntilRepublish 驗證儲存草稿不會偷偷切換線上表單版本。
func TestWorkspaceFormEditKeepsPublishedVersionUntilRepublish(t *testing.T) {
	store := memory.NewStore()
	seedFormDesignTenant(t, store, "tenant-1")
	currentNow := time.Date(2026, 7, 13, 6, 0, 0, 0, time.UTC)
	svc := service.New(store, service.Options{Now: func() time.Time { return currentNow }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}
	enabled := true
	baseFields := []domain.PlatformFormBuilderField{
		{ID: "subject", Type: "text", Label: "原始主旨", Placeholder: "請輸入", Required: true},
	}
	stages := []domain.PlatformFormBuilderStage{
		{ID: "stage-1", Type: "approver", Label: "主管審批", Detail: "主管審批", Config: map[string]any{"role": "manager"}},
	}
	created, err := svc.Workspace().CreateWorkspaceFormDesign(ctx, domain.SaveWorkspaceFormDesignInput{
		ID: "versioned-form", Name: "版本表單", Category: "自訂", Enabled: &enabled, Fields: baseFields, Stages: stages,
	})
	if err != nil {
		t.Fatal(err)
	}
	createdForm := findFormDesignVersion(t, created.Forms, "versioned-form")
	if createdForm.CurrentVersion != 1 || createdForm.PublishedVersion != 1 {
		t.Fatalf("expected v1 to be published on create, got %+v", createdForm)
	}

	currentNow = currentNow.Add(time.Hour)
	nextFields := []domain.PlatformFormBuilderField{
		{ID: "subject", Type: "text", Label: "新版主旨", Placeholder: "請輸入新版主旨", Required: true},
	}
	updated, err := svc.Workspace().UpdateWorkspaceFormDesign(ctx, "versioned-form", domain.UpdateWorkspaceFormDesignInput{
		Fields: &nextFields,
	})
	if err != nil {
		t.Fatal(err)
	}
	updatedForm := findFormDesignVersion(t, updated.Forms, "versioned-form")
	if updatedForm.CurrentVersion != 2 || updatedForm.PublishedVersion != 1 {
		t.Fatalf("expected saved draft v2 with published v1, got %+v", updatedForm)
	}
	runtimeBeforePublish, err := svc.Workflow().GetRuntimeFormTemplate(ctx, "versioned-form", "")
	if err != nil {
		t.Fatal(err)
	}
	if runtimeBeforePublish.Version != 1 || runtimeBeforePublish.Fields[0].Label != "原始主旨" {
		t.Fatalf("runtime changed before republish: %+v", runtimeBeforePublish)
	}

	published, err := svc.Workspace().PublishWorkspaceFormDesign(ctx, "versioned-form", domain.PublishWorkspaceFormDesignInput{Version: 2})
	if err != nil {
		t.Fatal(err)
	}
	publishedForm := findFormDesignVersion(t, published.Forms, "versioned-form")
	if publishedForm.CurrentVersion != 2 || publishedForm.PublishedVersion != 2 {
		t.Fatalf("expected published v2, got %+v", publishedForm)
	}
	runtimeAfterPublish, err := svc.Workflow().GetRuntimeFormTemplate(ctx, "versioned-form", "")
	if err != nil {
		t.Fatal(err)
	}
	if runtimeAfterPublish.Version != 2 || runtimeAfterPublish.Fields[0].Label != "新版主旨" {
		t.Fatalf("runtime did not switch to v2: %+v", runtimeAfterPublish)
	}
}

func findFormDesignVersion(t *testing.T, forms []domain.PlatformFormDesignForm, id string) domain.PlatformFormDesignForm {
	t.Helper()
	for _, form := range forms {
		if form.ID == id {
			return form
		}
	}
	t.Fatalf("form %q not found", id)
	return domain.PlatformFormDesignForm{}
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
			{Resource: "workflow.form_template", Action: "update", Scope: "all"},
			{Resource: "workflow.form_instance", Action: "read", Scope: "all"},
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
