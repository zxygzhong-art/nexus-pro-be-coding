package jobs_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/jobs"
	"nexus-pro-api/internal/repository/memory"
)

// TestOutboxDispatcherConsumesTypedRelationshipPayload 驗證生產端 typed payload 經 wire map 消費的完整 round-trip。
func TestOutboxDispatcherConsumesTypedRelationshipPayload(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	payload, err := (domain.OpenFGARelationshipPayload{
		Operation:   string(domain.AuthzRelationshipTupleWrite),
		ObjectType:  "hr.employee",
		ObjectID:    "emp-1",
		Relation:    "owner",
		SubjectType: "account",
		SubjectID:   "acct-1",
	}).Map()
	if err != nil {
		t.Fatal(err)
	}
	_ = store.AppendOutboxEvent(ctx, domain.OutboxEvent{
		ID:            "outbox-typed-1",
		TenantID:      "tenant-1",
		EventType:     string(domain.EventOpenFGARelationshipWrite),
		AggregateType: domain.OutboxAggregateAuthz,
		AggregateID:   "emp-1",
		Payload:       payload,
		Status:        "pending",
		CreatedAt:     now,
	})
	writer := &recordingTupleWriter{}
	dispatcher := jobs.NewOutboxDispatcher(store, writer, nil)

	if _, err := dispatcher.ProcessTenant(ctx, "tenant-1", jobs.OutboxDispatchOptions{BatchSize: 10}); err != nil {
		t.Fatal(err)
	}
	if len(writer.changes) != 1 {
		t.Fatalf("expected one OpenFGA write, got %+v", writer.changes)
	}
	tuple := writer.changes[0].Tuple
	if tuple.ObjectType != "hr.employee" || tuple.ObjectID != "emp-1" || tuple.Relation != "owner" || tuple.SubjectType != "account" || tuple.SubjectID != "acct-1" {
		t.Fatalf("unexpected tuple change: %+v", writer.changes[0])
	}
	events, err := store.ListOutboxEvents(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Status != "succeeded" {
		t.Fatalf("expected succeeded event, got %+v", events[0])
	}
}

// TestOutboxDispatcherFailsMalformedRelationshipPayload 驗證 payload 鍵值型別錯誤時事件標記失敗且帶明確錯誤。
func TestOutboxDispatcherFailsMalformedRelationshipPayload(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.AppendOutboxEvent(ctx, domain.OutboxEvent{
		ID:            "outbox-bad-1",
		TenantID:      "tenant-1",
		EventType:     string(domain.EventOpenFGARelationshipWrite),
		AggregateType: domain.OutboxAggregateAuthz,
		Payload: map[string]any{
			"object_type":  "hr.employee",
			"object_id":    42,
			"relation":     "owner",
			"subject_type": "account",
			"subject_id":   "acct-1",
		},
		Status:    "pending",
		CreatedAt: now,
	})
	writer := &recordingTupleWriter{}
	dispatcher := jobs.NewOutboxDispatcher(store, writer, nil)

	if _, err := dispatcher.ProcessTenant(ctx, "tenant-1", jobs.OutboxDispatchOptions{BatchSize: 10}); err != nil {
		t.Fatal(err)
	}
	if len(writer.changes) != 0 {
		t.Fatalf("malformed payload must not reach the writer, got %+v", writer.changes)
	}
	events, err := store.ListOutboxEvents(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Status != "failed" || !strings.Contains(events[0].LastError, "decode openfga relationship payload") {
		t.Fatalf("expected failed event with explicit decode error, got %+v", events[0])
	}
}

// TestLiteLLMModelSyncerReadsModelIDFromPayload 驗證缺少 aggregate id 時從 typed payload 取 model_id。
func TestLiteLLMModelSyncerReadsModelIDFromPayload(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	if err := store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-1", Name: "Tenant", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	payload, err := (domain.AgentModelSyncPayload{ModelID: "amodel-1"}).Map()
	if err != nil {
		t.Fatal(err)
	}
	admin := &fakeLiteLLMModelAdmin{}
	syncer := jobs.NewLiteLLMModelSyncer(store, admin, nil)
	event := domain.OutboxEvent{
		ID:            "outbox-model-1",
		TenantID:      "tenant-1",
		EventType:     string(domain.EventAgentModelDelete),
		AggregateType: domain.OutboxAggregateAgentModel,
		Payload:       payload,
		Status:        "pending",
		CreatedAt:     now,
	}
	if err := syncer.HandleAgentModelSyncEvent(ctx, event); err != nil {
		t.Fatal(err)
	}
	if len(admin.deleted) != 1 || admin.deleted[0] != "amodel-1" {
		t.Fatalf("expected delete for amodel-1, got %+v", admin.deleted)
	}
}

// TestLiteLLMModelSyncerRejectsMalformedModelID 驗證 model_id 型別錯誤回傳明確錯誤而不是靜默空串。
func TestLiteLLMModelSyncerRejectsMalformedModelID(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	admin := &fakeLiteLLMModelAdmin{}
	syncer := jobs.NewLiteLLMModelSyncer(store, admin, nil)
	event := domain.OutboxEvent{
		ID:        "outbox-model-bad",
		TenantID:  "tenant-1",
		EventType: string(domain.EventAgentModelDelete),
		Payload:   map[string]any{"model_id": 123},
		Status:    "pending",
		CreatedAt: time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC),
	}
	err := syncer.HandleAgentModelSyncEvent(ctx, event)
	if err == nil || !strings.Contains(err.Error(), "decode agent model sync payload") {
		t.Fatalf("expected explicit decode error, got %v", err)
	}
	if len(admin.deleted) != 0 {
		t.Fatalf("malformed payload must not reach LiteLLM, got %+v", admin.deleted)
	}
}
