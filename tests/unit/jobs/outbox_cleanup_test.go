package jobs_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/jobs"
	"nexus-pro-be/internal/repository/memory"
)

// TestOutboxCleanerDeletesOnlyExpiredSucceededEvents 驗證清理 job 只刪除過期的已成功事件。
func TestOutboxCleanerDeletesOnlyExpiredSucceededEvents(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-2", Name: "Tenant 2", CreatedAt: now})
	events := []domain.OutboxEvent{
		{ID: "outbox-expired-ok", TenantID: "tenant-1", EventType: "iam.relationship.write", Status: "succeeded", CreatedAt: now.Add(-8 * 24 * time.Hour)},
		{ID: "outbox-recent-ok", TenantID: "tenant-1", EventType: "iam.relationship.write", Status: "succeeded", CreatedAt: now.Add(-time.Hour)},
		{ID: "outbox-expired-failed", TenantID: "tenant-1", EventType: "iam.relationship.write", Status: "failed", CreatedAt: now.Add(-30 * 24 * time.Hour)},
		{ID: "outbox-expired-pending", TenantID: "tenant-1", EventType: "iam.relationship.write", Status: "pending", CreatedAt: now.Add(-30 * 24 * time.Hour)},
		{ID: "outbox-tenant2-expired-ok", TenantID: "tenant-2", EventType: "iam.relationship.write", Status: "succeeded", CreatedAt: now.Add(-9 * 24 * time.Hour)},
	}
	for _, event := range events {
		if err := store.AppendOutboxEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	cleaner := jobs.NewOutboxCleaner(store, nil)

	deleted, err := cleaner.CleanupAllTenants(ctx, jobs.OutboxCleanupOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}
	remaining, err := store.ListOutboxEvents(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 3 {
		t.Fatalf("expected recent/failed/pending events kept, got %+v", remaining)
	}
	for _, event := range remaining {
		if event.ID == "outbox-expired-ok" {
			t.Fatalf("expected expired succeeded event removed, got %+v", event)
		}
	}
	remaining, err = store.ListOutboxEvents(ctx, "tenant-2")
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected tenant-2 expired succeeded event removed, got %+v", remaining)
	}
}

// TestOutboxCleanerContinuesAfterTenantFailure 驗證單一租戶清理失敗不阻塞其他租戶。
func TestOutboxCleanerContinuesAfterTenantFailure(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	store := &failingOutboxDeleteStore{Store: memory.NewStore(), failTenant: "tenant-1"}
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-2", Name: "Tenant 2", CreatedAt: now})
	_ = store.AppendOutboxEvent(ctx, domain.OutboxEvent{ID: "outbox-1", TenantID: "tenant-1", EventType: "iam.relationship.write", Status: "succeeded", CreatedAt: now.Add(-8 * 24 * time.Hour)})
	_ = store.AppendOutboxEvent(ctx, domain.OutboxEvent{ID: "outbox-2", TenantID: "tenant-2", EventType: "iam.relationship.write", Status: "succeeded", CreatedAt: now.Add(-8 * 24 * time.Hour)})
	cleaner := jobs.NewOutboxCleaner(store, nil)

	deleted, err := cleaner.CleanupAllTenants(ctx, jobs.OutboxCleanupOptions{})
	if err == nil || !strings.Contains(err.Error(), "tenant-1") {
		t.Fatalf("expected aggregated tenant-1 error, got %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected tenant-2 cleanup to proceed despite tenant-1 failure, deleted = %d", deleted)
	}
	remaining, err := store.ListOutboxEvents(ctx, "tenant-2")
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected tenant-2 event cleaned, got %+v", remaining)
	}
}

type failingOutboxDeleteStore struct {
	*memory.Store
	failTenant string
}

// DeleteSucceededOutboxEventsBefore 驗證指定租戶的刪除失敗。
func (s *failingOutboxDeleteStore) DeleteSucceededOutboxEventsBefore(ctx context.Context, tenantID string, before time.Time) (int64, error) {
	if tenantID == s.failTenant {
		return 0, errors.New("database unavailable")
	}
	return s.Store.DeleteSucceededOutboxEventsBefore(ctx, tenantID, before)
}
