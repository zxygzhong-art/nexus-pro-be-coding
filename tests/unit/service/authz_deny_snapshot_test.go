package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

// TestAuthzDenySnapshotUsesShortTTLAndExpires 驗證 deny 決策以短 TTL 快取並準時失效。
func TestAuthzDenySnapshotUsesShortTTLAndExpires(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	cache := &recordingAuthzSnapshot{values: map[string]domain.CheckResult{}, now: func() time.Time { return now }}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, AuthzSnapshot: cache})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	req := domain.CheckRequest{Resource: "hr.employee", Action: "read"}

	denied, err := svc.Authz().Check(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if denied.Allowed {
		t.Fatalf("expected missing permission deny, got %+v", denied)
	}
	if cache.sets != 1 || len(cache.ttls) != 1 || cache.ttls[0] != time.Minute {
		t.Fatalf("expected deny snapshot with 1m TTL, got sets=%d ttls=%+v", cache.sets, cache.ttls)
	}

	// A direct store write bypasses service-level invalidation, so the cached
	// deny must still be returned as deny (never mis-allowed) until it expires.
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-late", TenantID: "tenant-1", Name: "Late Grant", CreatedAt: now,
		Permissions: []domain.Permission{{Resource: "hr.employee", Action: "read", Scope: "all"}},
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-late"}, CreatedAt: now})
	cached, err := svc.Authz().Check(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if cached.Allowed {
		t.Fatalf("expected cached deny snapshot to stay denied, got %+v", cached)
	}
	if cache.sets != 1 || cache.gets != 2 {
		t.Fatalf("expected second check to hit the snapshot without re-caching, got sets=%d gets=%d", cache.sets, cache.gets)
	}

	now = now.Add(time.Minute + time.Second)
	allowed, err := svc.Authz().Check(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !allowed.Allowed {
		t.Fatalf("expected deny snapshot to expire and re-evaluate to allow, got %+v", allowed)
	}
}

// TestAuthzRelationshipFallbackDenyIsSnapshotted 驗證 OpenFGA fallback 的 deny 也進快照。
func TestAuthzRelationshipFallbackDenyIsSnapshotted(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	checker := &mappedRelationshipChecker{allowed: map[string]bool{}}
	cache := &recordingAuthzSnapshot{values: map[string]domain.CheckResult{}}
	svc := service.New(store, service.Options{Relationships: checker, AuthzSnapshot: cache})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	req := domain.CheckRequest{Resource: "hr.employee", ResourceID: "emp-1", Action: domain.ActionRead}

	denied, err := svc.Authz().Check(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if denied.Allowed || denied.Reason != "missing permission" {
		t.Fatalf("expected relationship fallback deny, got %+v", denied)
	}
	if len(checker.checks) != 1 {
		t.Fatalf("expected one relationship check, got %+v", checker.checks)
	}
	if cache.sets != 1 || len(cache.ttls) != 1 || cache.ttls[0] != time.Minute {
		t.Fatalf("expected fallback deny snapshot with 1m TTL, got sets=%d ttls=%+v", cache.sets, cache.ttls)
	}

	cached, err := svc.Authz().Check(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if cached.Allowed {
		t.Fatalf("expected cached fallback deny to stay denied, got %+v", cached)
	}
	if len(checker.checks) != 1 {
		t.Fatalf("expected cached deny to skip the OpenFGA fallback call, got %+v", checker.checks)
	}
}
