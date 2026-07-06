package jobs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/jobs"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

type flakyIdentityProvisioner struct {
	failuresLeft int
	inputs       []domain.IdentityProvisioningInput
}

// EnsureUser 驗證使用者。
func (p *flakyIdentityProvisioner) EnsureUser(_ context.Context, input domain.IdentityProvisioningInput) (domain.ProvisionedIdentity, error) {
	p.inputs = append(p.inputs, input)
	if p.failuresLeft > 0 {
		p.failuresLeft--
		return domain.ProvisionedIdentity{}, errors.New("keycloak unavailable")
	}
	return domain.ProvisionedIdentity{Provider: domain.IdentityProviderKeycloak, Subject: "kc-" + input.AccountID, Email: input.Email}, nil
}

// newIdentityProvisioningFixture 驗證身分開通 fixture。
func newIdentityProvisioningFixture(t *testing.T, provisioner service.IdentityProvisioner, now *time.Time) (*memory.Store, *jobs.IdentityProvisioningOutboxProcessor) {
	t.Helper()
	ctx := context.Background()
	store := memory.NewStore()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: *now})
	svc := service.New(store, service.Options{
		IdentityProvisioner: provisioner,
		Now:                 func() time.Time { return *now },
	})
	return store, jobs.NewIdentityProvisioningOutboxProcessor(store, svc, nil)
}

// pendingProvisioningEvent 驗證 pending 開通事件。
func pendingProvisioningEvent(id string, createdAt time.Time) domain.IdentityProvisioningOutboxEvent {
	return domain.IdentityProvisioningOutboxEvent{
		ID:        id,
		TenantID:  "tenant-1",
		AccountID: "acct-1",
		Email:     "user@example.com",
		Enabled:   true,
		Status:    domain.IdentityProvisioningStatusPending,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
}

// TestIdentityProvisioningOutboxProcessorRetriesWithBackoffUntilSuccess 驗證身分開通 outbox processor retries with backoff until success。
func TestIdentityProvisioningOutboxProcessorRetriesWithBackoffUntilSuccess(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC)
	provisioner := &flakyIdentityProvisioner{failuresLeft: 1}
	store, processor := newIdentityProvisioningFixture(t, provisioner, &now)
	_ = store.AppendIdentityProvisioningOutboxEvent(ctx, pendingProvisioningEvent("idp-1", now))

	processed, err := processor.ProcessAllTenants(ctx, jobs.IdentityProvisioningOutboxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	pending, err := store.ListPendingIdentityProvisioningOutboxEvents(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].RetryCount != 1 || pending[0].LastError == "" {
		t.Fatalf("expected pending event with one recorded retry, got %+v", pending)
	}

	// 在 backoff 視窗內會跳過事件，避免持續打擊 Keycloak。
	processed, err = processor.ProcessAllTenants(ctx, jobs.IdentityProvisioningOutboxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if processed != 0 {
		t.Fatalf("expected backoff to defer the retry, processed = %d", processed)
	}

	now = now.Add(time.Minute)
	processed, err = processor.ProcessAllTenants(ctx, jobs.IdentityProvisioningOutboxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("expected due retry to be processed, processed = %d", processed)
	}
	identities, err := store.ListUserIdentities(ctx, "tenant-1", "acct-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(identities) != 1 || identities[0].Subject != "kc-acct-1" {
		t.Fatalf("expected identity binding after retry, got %+v", identities)
	}
	pending, err = store.ListPendingIdentityProvisioningOutboxEvents(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected outbox drained after success, got %+v", pending)
	}
	if len(provisioner.inputs) != 2 {
		t.Fatalf("expected two provisioning attempts, got %d", len(provisioner.inputs))
	}
}

// TestIdentityProvisioningOutboxProcessorMarksExhaustedEventFailed 驗證身分開通 outbox processor marks exhausted 事件 failed。
func TestIdentityProvisioningOutboxProcessorMarksExhaustedEventFailed(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC)
	provisioner := &flakyIdentityProvisioner{failuresLeft: 10}
	store, processor := newIdentityProvisioningFixture(t, provisioner, &now)
	_ = store.AppendIdentityProvisioningOutboxEvent(ctx, pendingProvisioningEvent("idp-1", now))

	if _, err := processor.ProcessAllTenants(ctx, jobs.IdentityProvisioningOutboxOptions{MaxRetries: 1}); err != nil {
		t.Fatal(err)
	}
	pending, err := store.ListPendingIdentityProvisioningOutboxEvents(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected event to leave the pending queue after max retries, got %+v", pending)
	}
	// 已耗盡重試的事件是終態；下一輪不再處理。
	now = now.Add(time.Hour)
	processed, err := processor.ProcessAllTenants(ctx, jobs.IdentityProvisioningOutboxOptions{MaxRetries: 1})
	if err != nil {
		t.Fatal(err)
	}
	if processed != 0 || len(provisioner.inputs) != 1 {
		t.Fatalf("expected no further attempts for failed event, processed=%d attempts=%d", processed, len(provisioner.inputs))
	}
}

// TestIdentityProvisioningOutboxProcessorMarksIdentityConflictFailed 驗證身分開通 outbox processor marks 身分衝突 failed。
func TestIdentityProvisioningOutboxProcessorMarksIdentityConflictFailed(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC)
	provisioner := &flakyIdentityProvisioner{}
	store, processor := newIdentityProvisioningFixture(t, provisioner, &now)
	_ = store.UpsertUserIdentity(ctx, domain.UserIdentity{
		ID:        "uid-existing",
		TenantID:  "tenant-1",
		AccountID: "acct-other",
		Provider:  domain.IdentityProviderKeycloak,
		Subject:   "kc-acct-1",
		Email:     "other@example.com",
		CreatedAt: now,
	})
	_ = store.AppendIdentityProvisioningOutboxEvent(ctx, pendingProvisioningEvent("idp-1", now))

	processed, err := processor.ProcessAllTenants(ctx, jobs.IdentityProvisioningOutboxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	pending, err := store.ListPendingIdentityProvisioningOutboxEvents(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected conflict to mark the event failed, got %+v", pending)
	}
	existing, ok, err := store.GetUserIdentity(ctx, "tenant-1", domain.IdentityProviderKeycloak, "kc-acct-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || existing.AccountID != "acct-other" {
		t.Fatalf("expected conflicting identity to stay linked to original account, got %+v", existing)
	}
	// 衝突是終態；下一輪不得重試或重新連結。
	now = now.Add(time.Hour)
	processed, err = processor.ProcessAllTenants(ctx, jobs.IdentityProvisioningOutboxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if processed != 0 || len(provisioner.inputs) != 1 {
		t.Fatalf("expected no retry after conflict, processed=%d attempts=%d", processed, len(provisioner.inputs))
	}
}
