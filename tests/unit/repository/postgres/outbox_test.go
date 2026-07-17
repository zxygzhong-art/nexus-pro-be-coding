package postgres_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	postgresrepo "nexus-pro-be/internal/repository/postgres"
	"nexus-pro-be/internal/utils/tenantctx"
)

// TestOutboxEventRepositoryQueries 驗證 outbox 事件主鍵查詢、SQL 分頁與定期清理刪除。
func TestOutboxEventRepositoryQueries(t *testing.T) {
	pool := openPostgresIntegrationPool(t)
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	var migrated bool
	if err := pool.QueryRow(ctx, "select to_regclass('public.outbox_events') is not null").Scan(&migrated); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	if !migrated {
		t.Skip("postgres schema is not migrated; skipping outbox repository test")
	}
	store := postgresrepo.NewStore(pool)
	ctx = context.Background()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	tenantID := "tenant-outbox-" + suffix
	otherTenantID := "tenant-outbox-other-" + suffix
	base := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	for _, id := range []string{tenantID, otherTenantID} {
		if err := store.UpsertTenant(ctx, domain.Tenant{ID: id, Name: "Outbox Tenant " + id, CreatedAt: base}); err != nil {
			t.Fatal(err)
		}
	}
	events := []domain.OutboxEvent{
		{ID: "obe_old_ok_" + suffix, TenantID: tenantID, EventType: "iam.relationship.write", Status: "succeeded", CreatedAt: base.Add(-8 * 24 * time.Hour)},
		{ID: "obe_recent_ok_" + suffix, TenantID: tenantID, EventType: "iam.relationship.write", Status: "succeeded", CreatedAt: base.Add(-time.Hour)},
		{ID: "obe_failed_" + suffix, TenantID: tenantID, EventType: "iam.relationship.write", Status: "failed", RetryCount: 3, LastError: "openfga unavailable", CreatedAt: base.Add(-9 * 24 * time.Hour)},
		{ID: "obe_pending_" + suffix, TenantID: tenantID, EventType: "tenant.provisioned", Status: "pending", CreatedAt: base},
		{ID: "obe_other_ok_" + suffix, TenantID: otherTenantID, EventType: "iam.relationship.write", Status: "succeeded", CreatedAt: base.Add(-30 * 24 * time.Hour)},
	}
	for _, event := range events {
		// AppendOutboxEvent 沿用呼叫端的租戶 scope;直接寫入時需帶上對應租戶。
		if err := store.AppendOutboxEvent(tenantctx.WithTenantID(ctx, event.TenantID), event); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("get by primary key", func(t *testing.T) {
		got, ok, err := store.GetOutboxEventByID(ctx, tenantID, "obe_failed_"+suffix)
		if err != nil || !ok {
			t.Fatalf("expected event, ok=%v err=%v", ok, err)
		}
		if got.Status != "failed" || got.RetryCount != 3 || got.LastError != "openfga unavailable" {
			t.Fatalf("unexpected event: %+v", got)
		}
		if _, ok, err := store.GetOutboxEventByID(ctx, tenantID, "obe_missing_"+suffix); err != nil || ok {
			t.Fatalf("expected missing event, ok=%v err=%v", ok, err)
		}
		if _, ok, err := store.GetOutboxEventByID(ctx, otherTenantID, "obe_failed_"+suffix); err != nil || ok {
			t.Fatalf("expected cross-tenant lookup to miss, ok=%v err=%v", ok, err)
		}
	})

	t.Run("sql pagination and filters", func(t *testing.T) {
		first, total, err := store.ListOutboxEventPage(ctx, tenantID, domain.OutboxEventQuery{}, domain.PageRequest{Page: 1, PageSize: 2})
		if err != nil {
			t.Fatal(err)
		}
		if total != 4 || len(first) != 2 {
			t.Fatalf("expected total 4 with 2 items, total=%d items=%+v", total, first)
		}
		if first[0].ID != "obe_pending_"+suffix || first[1].ID != "obe_recent_ok_"+suffix {
			t.Fatalf("expected default created_at_desc order, got %+v", first)
		}
		second, total, err := store.ListOutboxEventPage(ctx, tenantID, domain.OutboxEventQuery{}, domain.PageRequest{Page: 2, PageSize: 2})
		if err != nil || total != 4 || len(second) != 2 {
			t.Fatalf("expected second page, total=%d items=%+v err=%v", total, second, err)
		}
		// created_at_desc 順序: pending(base), recent_ok(-1h), old_ok(-8d), failed(-9d)
		if second[0].ID != "obe_old_ok_"+suffix || second[1].ID != "obe_failed_"+suffix {
			t.Fatalf("unexpected second page: %+v", second)
		}
		asc, _, err := store.ListOutboxEventPage(ctx, tenantID, domain.OutboxEventQuery{}, domain.PageRequest{Page: 1, PageSize: 1, Sort: "created_at_asc"})
		if err != nil || len(asc) != 1 || asc[0].ID != "obe_failed_"+suffix {
			t.Fatalf("expected oldest event first with created_at_asc, items=%+v err=%v", asc, err)
		}
		filtered, total, err := store.ListOutboxEventPage(ctx, tenantID, domain.OutboxEventQuery{Status: "failed", LastError: "UNAVAILABLE"}, domain.PageRequest{Page: 1, PageSize: 10})
		if err != nil || total != 1 || len(filtered) != 1 || filtered[0].ID != "obe_failed_"+suffix {
			t.Fatalf("expected filtered failed event, total=%d items=%+v err=%v", total, filtered, err)
		}
		retryCount := 3
		byRetry, total, err := store.ListOutboxEventPage(ctx, tenantID, domain.OutboxEventQuery{RetryCount: &retryCount}, domain.PageRequest{Page: 1, PageSize: 10})
		if err != nil || total != 1 || len(byRetry) != 1 {
			t.Fatalf("expected retry_count filter to match one event, total=%d items=%+v err=%v", total, byRetry, err)
		}
		hasError := false
		noError, total, err := store.ListOutboxEventPage(ctx, tenantID, domain.OutboxEventQuery{HasError: &hasError}, domain.PageRequest{Page: 1, PageSize: 10})
		if err != nil || total != 3 || len(noError) != 3 {
			t.Fatalf("expected has_error=false to match three events, total=%d items=%+v err=%v", total, noError, err)
		}
		byType, total, err := store.ListOutboxEventPage(ctx, tenantID, domain.OutboxEventQuery{EventType: "tenant.provisioned"}, domain.PageRequest{Page: 1, PageSize: 10})
		if err != nil || total != 1 || len(byType) != 1 || byType[0].ID != "obe_pending_"+suffix {
			t.Fatalf("expected event_type filter to match one event, total=%d items=%+v err=%v", total, byType, err)
		}
	})

	t.Run("delete succeeded before cutoff", func(t *testing.T) {
		deleted, err := store.DeleteSucceededOutboxEventsBefore(ctx, tenantID, base.Add(-7*24*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if deleted != 1 {
			t.Fatalf("expected one expired succeeded event deleted, got %d", deleted)
		}
		remaining, err := store.ListOutboxEvents(ctx, tenantID)
		if err != nil {
			t.Fatal(err)
		}
		if len(remaining) != 3 {
			t.Fatalf("expected three remaining events, got %+v", remaining)
		}
		for _, event := range remaining {
			if event.ID == "obe_old_ok_"+suffix {
				t.Fatalf("expected expired succeeded event removed, got %+v", event)
			}
		}
		deleted, err = store.DeleteSucceededOutboxEventsBefore(ctx, tenantID, base.Add(-7*24*time.Hour))
		if err != nil || deleted != 0 {
			t.Fatalf("expected idempotent cleanup, deleted=%d err=%v", deleted, err)
		}
		deleted, err = store.DeleteSucceededOutboxEventsBefore(ctx, otherTenantID, base.Add(-7*24*time.Hour))
		if err != nil || deleted != 1 {
			t.Fatalf("expected other tenant cleanup to delete its own event, deleted=%d err=%v", deleted, err)
		}
	})
}
