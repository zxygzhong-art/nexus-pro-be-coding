package jobs

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	platformsecret "nexus-pro-be/internal/platform/secret"
	"nexus-pro-be/internal/repository/memory"
)

type fakeLiteLLMModelAdmin struct {
	synced  []string
	apiKeys []string
	deleted []string
	remote  []string
	err     error
}

// ListManagedModelIDs 回傳測試用遠端 managed deployment。
func (f *fakeLiteLLMModelAdmin) ListManagedModelIDs(context.Context) ([]string, error) {
	return append([]string(nil), f.remote...), f.err
}

// SyncModel 記錄背景 upsert 呼叫。
func (f *fakeLiteLLMModelAdmin) SyncModel(_ context.Context, model domain.AgentModel) (string, error) {
	f.synced = append(f.synced, model.ID)
	f.apiKeys = append(f.apiKeys, model.APIKey)
	return "synced", f.err
}

// TestLiteLLMModelSyncerDecryptsCredentialOnlyAtDispatch verifies persisted ciphertext is opened just in time.
func TestLiteLLMModelSyncerDecryptsCredentialOnlyAtDispatch(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	now := time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC)
	cipher, err := platformsecret.NewAESGCMCipher(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := cipher.Encrypt([]byte("sk-upstream"), domain.AgentModelCredentialAAD("tenant-1", "amodel-encrypted"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-1", Name: "Tenant", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgentModel(ctx, domain.AgentModel{
		ID: "amodel-encrypted", TenantID: "tenant-1", Name: "Encrypted", Provider: "openai",
		ModelName: "gpt-4.1-mini", LiteLLMModel: domain.AgentModelLiteLLMAlias("amodel-encrypted"),
		APIKeyCiphertext: ciphertext, APIKeySet: true, Status: domain.AgentModelStatusActive,
		SyncStatus: domain.AgentModelSyncStatusPending, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	admin := &fakeLiteLLMModelAdmin{}
	if _, err := NewLiteLLMModelSyncer(store, admin, nil).WithCredentialCipher(cipher).ReconcileAll(ctx); err != nil {
		t.Fatal(err)
	}
	if len(admin.apiKeys) != 1 || admin.apiKeys[0] != "sk-upstream" {
		t.Fatalf("expected just-in-time plaintext credential, got %+v", admin.apiKeys)
	}
	stored, ok, err := store.GetAgentModel(ctx, "tenant-1", "amodel-encrypted")
	if err != nil || !ok || stored.APIKey != "" || stored.APIKeyCiphertext != ciphertext {
		t.Fatalf("persisted model credential changed or leaked: model=%+v ok=%v err=%v", stored, ok, err)
	}
}

// DeleteModel 記錄背景 delete 呼叫。
func (f *fakeLiteLLMModelAdmin) DeleteModel(_ context.Context, id string) (string, error) {
	f.deleted = append(f.deleted, id)
	return "deleted", f.err
}

// TestLiteLLMModelSyncerReconcileAllUpsertsActiveAndDeletesDisabled 驗證全量對帳與狀態寫回。
func TestLiteLLMModelSyncerReconcileAllUpsertsActiveAndDeletesDisabled(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	now := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	if err := store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-1", Name: "Tenant", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	models := []domain.AgentModel{
		{ID: "amodel-active", TenantID: "tenant-1", Name: "Active", Provider: "openai", ModelName: "gpt-4.1-mini", LiteLLMModel: domain.AgentModelLiteLLMAlias("amodel-active"), APIKey: "sk-active", Status: domain.AgentModelStatusActive, SyncStatus: domain.AgentModelSyncStatusPending, CreatedAt: now, UpdatedAt: now},
		{ID: "amodel-disabled", TenantID: "tenant-1", Name: "Disabled", Provider: "anthropic", ModelName: "claude-sonnet-4", LiteLLMModel: domain.AgentModelLiteLLMAlias("amodel-disabled"), APIKey: "sk-disabled", Status: domain.AgentModelStatusDisabled, SyncStatus: domain.AgentModelSyncStatusPending, CreatedAt: now, UpdatedAt: now},
	}
	for _, model := range models {
		if err := store.UpsertAgentModel(ctx, model); err != nil {
			t.Fatal(err)
		}
	}
	admin := &fakeLiteLLMModelAdmin{remote: []string{"amodel-active", "amodel-disabled", "amodel-orphan"}}
	syncer := NewLiteLLMModelSyncer(store, admin, nil)
	syncer.now = func() time.Time { return now.Add(time.Minute) }
	processed, err := syncer.ReconcileAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 3 || len(admin.synced) != 1 || admin.synced[0] != "amodel-active" || len(admin.deleted) != 2 || admin.deleted[0] != "amodel-disabled" || admin.deleted[1] != "amodel-orphan" {
		t.Fatalf("unexpected reconciliation calls: processed=%d synced=%v deleted=%v", processed, admin.synced, admin.deleted)
	}
	for _, model := range models {
		stored, ok, err := store.GetAgentModel(ctx, model.TenantID, model.ID)
		if err != nil || !ok {
			t.Fatalf("missing reconciled model %s: ok=%v err=%v", model.ID, ok, err)
		}
		if stored.SyncStatus != domain.AgentModelSyncStatusSynced || stored.LastSyncedAt == nil || stored.SyncedConfigHash == "" {
			t.Fatalf("unexpected sync result for %s: %+v", model.ID, stored)
		}
	}
}

// TestLiteLLMModelSyncerDoesNotOverwriteRouteWithoutCredential protects legacy databases during credential migration.
func TestLiteLLMModelSyncerDoesNotOverwriteRouteWithoutCredential(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	now := time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC)
	if err := store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-1", Name: "Tenant", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgentModel(ctx, domain.AgentModel{
		ID: "amodel-legacy", TenantID: "tenant-1", Name: "Legacy", Provider: "openai",
		ModelName: "gpt-4.1-mini", LiteLLMModel: domain.AgentModelLiteLLMAlias("amodel-legacy"),
		Status: domain.AgentModelStatusActive, SyncStatus: domain.AgentModelSyncStatusSynced, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	admin := &fakeLiteLLMModelAdmin{}
	syncer := NewLiteLLMModelSyncer(store, admin, nil)
	processed, err := syncer.ReconcileAll(ctx)
	if processed != 1 || err == nil {
		t.Fatalf("expected missing credential to fail closed, processed=%d err=%v", processed, err)
	}
	if len(admin.synced) != 0 {
		t.Fatalf("missing credential must not overwrite LiteLLM, synced=%v", admin.synced)
	}
}

// TestOutboxDispatcherKeepsModelEventPendingWithoutLiteLLMConfig 驗證缺少設定時事件不會耗盡重試。
func TestOutboxDispatcherKeepsModelEventPendingWithoutLiteLLMConfig(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	now := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	if err := store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-1", Name: "Tenant", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	event := domain.OutboxEvent{ID: "outbox-1", TenantID: "tenant-1", EventType: string(domain.EventAgentModelDelete), AggregateType: domain.OutboxAggregateAgentModel, AggregateID: "amodel-1", Status: "pending", CreatedAt: now}
	if err := store.AppendOutboxEvent(ctx, event); err != nil {
		t.Fatal(err)
	}
	dispatcher := NewOutboxDispatcher(store, nil, nil).WithAgentModelSyncHandler(NewLiteLLMModelSyncer(store, nil, nil))
	if _, err := dispatcher.ProcessTenant(ctx, "tenant-1", OutboxDispatchOptions{}); err != nil {
		t.Fatal(err)
	}
	events, err := store.ListOutboxEvents(ctx, "tenant-1")
	if err != nil || len(events) != 1 {
		t.Fatalf("missing outbox event: events=%v err=%v", events, err)
	}
	stored := events[0]
	if stored.Status != "pending" || stored.RetryCount != 0 {
		t.Fatalf("expected pending event without retry exhaustion, got %+v", stored)
	}
}
