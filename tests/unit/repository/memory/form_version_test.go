package memory_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
)

// TestFormTemplateVersionsAreImmutableAndInstancesBindVersion 驗證模板更新保留舊快照且實例綁定當前版本。
func TestFormTemplateVersionsAreImmutableAndInstancesBindVersion(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 13, 6, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	template := domain.FormTemplate{
		ID: "ft-1", TenantID: "tenant-1", Key: "expense", Name: "費用", Status: "published",
		CurrentVersion: 1, Schema: map[string]any{"title": "v1"}, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.UpsertFormTemplate(ctx, template); err != nil {
		t.Fatal(err)
	}
	v1, ok, err := store.GetFormTemplateVersionByNumber(ctx, "tenant-1", "ft-1", 1)
	if err != nil || !ok {
		t.Fatalf("expected v1 snapshot: ok=%v err=%v", ok, err)
	}

	template.CurrentVersion = 2
	template.Schema = map[string]any{"title": "v2"}
	template.UpdatedAt = now.Add(time.Hour)
	if err := store.UpsertFormTemplate(ctx, template); err != nil {
		t.Fatal(err)
	}
	v1After, _, _ := store.GetFormTemplateVersionByNumber(ctx, "tenant-1", "ft-1", 1)
	if v1After.Schema["title"] != "v1" || v1After.ID != v1.ID {
		t.Fatalf("v1 snapshot was mutated: before=%+v after=%+v", v1, v1After)
	}
	v2, ok, err := store.GetFormTemplateVersionByNumber(ctx, "tenant-1", "ft-1", 2)
	if err != nil || !ok || v2.Schema["title"] != "v2" {
		t.Fatalf("expected immutable v2 snapshot: %+v ok=%v err=%v", v2, ok, err)
	}

	instance := domain.FormInstance{
		ID: "fi-1", TenantID: "tenant-1", TemplateID: "ft-1", ApplicantAccountID: "acct-1",
		Status: "draft", SubmittedAt: now, UpdatedAt: now,
	}
	if err := store.UpsertFormInstance(ctx, instance); err != nil {
		t.Fatal(err)
	}
	stored, ok, err := store.GetFormInstance(ctx, "tenant-1", "fi-1")
	if err != nil || !ok || stored.TemplateVersionID != v2.ID {
		t.Fatalf("expected instance bound to v2: %+v ok=%v err=%v", stored, ok, err)
	}
}
