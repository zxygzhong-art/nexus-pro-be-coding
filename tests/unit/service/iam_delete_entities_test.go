package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

func TestDeletePermissionSetHappyPathAndConflictWhenReferenced(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-admin", TenantID: "tenant-1", Name: "IAM Admin",
		Permissions: []domain.Permission{{Resource: "iam.permission_set", Action: "delete", Scope: "all"}},
		CreatedAt:   now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-free", TenantID: "tenant-1", Name: "Free Set", CreatedAt: now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-used", TenantID: "tenant-1", Name: "Used Set", CreatedAt: now,
	})
	_ = store.UpsertUserGroup(context.Background(), domain.UserGroup{
		ID: "ug-1", TenantID: "tenant-1", Name: "Ops", PermissionSetIDs: []string{"ps-used"}, CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-admin"}, CreatedAt: now,
	})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	if _, err := svc.IAM().DeletePermissionSet(ctx, "ps-used"); err == nil {
		t.Fatal("expected conflict when permission set referenced by user group")
	} else if appErr, ok := err.(*domain.AppError); !ok || appErr.Status != 409 {
		t.Fatalf("expected 409 conflict, got %v", err)
	}

	deleted, err := svc.IAM().DeletePermissionSet(ctx, "ps-free")
	if err != nil {
		t.Fatal(err)
	}
	if deleted.ID != "ps-free" {
		t.Fatalf("expected deleted permission set, got %+v", deleted)
	}
	if _, ok, err := store.GetPermissionSet(context.Background(), "tenant-1", "ps-free"); err != nil || ok {
		t.Fatalf("expected permission set removed, ok=%v err=%v", ok, err)
	}
}

func TestDeletePermissionSetConflictOnActiveAssignment(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 5, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-admin", TenantID: "tenant-1", Name: "IAM Admin",
		Permissions: []domain.Permission{{Resource: "iam.permission_set", Action: "delete", Scope: "all"}},
		CreatedAt:   now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-bound", TenantID: "tenant-1", Name: "Bound Set", CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-admin"}, CreatedAt: now,
	})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID: "psa-1", TenantID: "tenant-1", PrincipalType: "account", PrincipalID: "acct-1",
		PermissionSetID: "ps-bound", Effect: "allow", CreatedAt: now,
	})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	if _, err := svc.IAM().DeletePermissionSet(ctx, "ps-bound"); err == nil {
		t.Fatal("expected conflict when permission set has active assignments")
	}
}

func TestDeleteUserGroupHappyPathAndConflictWhenMembers(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 10, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-admin", TenantID: "tenant-1", Name: "IAM Admin",
		Permissions: []domain.Permission{{Resource: "iam.user_group", Action: "delete", Scope: "all"}},
		CreatedAt:   now,
	})
	_ = store.UpsertUserGroup(context.Background(), domain.UserGroup{
		ID: "ug-empty", TenantID: "tenant-1", Name: "Empty", CreatedAt: now,
	})
	_ = store.UpsertUserGroup(context.Background(), domain.UserGroup{
		ID: "ug-members", TenantID: "tenant-1", Name: "With Members", MemberAccountIDs: []string{"acct-2"}, CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-admin"}, CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID: "acct-2", TenantID: "tenant-1", Status: "active", UserGroupIDs: []string{"ug-members"}, CreatedAt: now,
	})
	seedActiveGroupMembership(t, store, "tenant-1", "ug-members", "acct-2", now)
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	if _, err := svc.IAM().DeleteUserGroup(ctx, "ug-members"); err == nil {
		t.Fatal("expected conflict when user group has members")
	}

	deleted, err := svc.IAM().DeleteUserGroup(ctx, "ug-empty")
	if err != nil {
		t.Fatal(err)
	}
	if deleted.ID != "ug-empty" {
		t.Fatalf("expected deleted user group, got %+v", deleted)
	}
	if _, ok, err := store.GetUserGroup(context.Background(), "tenant-1", "ug-empty"); err != nil || ok {
		t.Fatalf("expected user group removed, ok=%v err=%v", ok, err)
	}
}

func TestDeleteUserGroupConflictWhenTrustPolicyReferences(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 15, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-admin", TenantID: "tenant-1", Name: "IAM Admin",
		Permissions: []domain.Permission{{Resource: "iam.user_group", Action: "delete", Scope: "all"}},
		CreatedAt:   now,
	})
	_ = store.UpsertUserGroup(context.Background(), domain.UserGroup{
		ID: "ug-trusted", TenantID: "tenant-1", Name: "Trusted", CreatedAt: now,
	})
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID: "ar-1", TenantID: "tenant-1", Name: "Breakglass", Trusted: true,
		TrustPolicy:            map[string]any{"user_group_ids": []any{"ug-trusted"}},
		PermissionBoundary:     map[string]any{"mode": "deny_all"},
		SessionDurationSeconds: 3600,
		CreatedAt:              now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-admin"}, CreatedAt: now,
	})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	if _, err := svc.IAM().DeleteUserGroup(ctx, "ug-trusted"); err == nil {
		t.Fatal("expected conflict when user group referenced by trust policy")
	}
}

func TestDeleteAssumableRoleHappyPathAndConflictWhenActiveSession(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 20, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-admin", TenantID: "tenant-1", Name: "IAM Admin",
		Permissions: []domain.Permission{{Resource: "iam.assumable_role", Action: "delete", Scope: "all"}},
		CreatedAt:   now,
	})
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID: "ar-free", TenantID: "tenant-1", Name: "Free Role", Trusted: true,
		TrustPolicy: map[string]any{"accounts": []any{"acct-1"}}, PermissionBoundary: map[string]any{"mode": "deny_all"},
		SessionDurationSeconds: 3600, CreatedAt: now,
	})
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID: "ar-active", TenantID: "tenant-1", Name: "Active Role", Trusted: true,
		TrustPolicy: map[string]any{"accounts": []any{"acct-1"}}, PermissionBoundary: map[string]any{"mode": "deny_all"},
		SessionDurationSeconds: 3600, CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID: "acct-1", TenantID: "tenant-1", Status: "active",
		DirectPermissionSetIDs: []string{"ps-admin"}, ActiveAssumableRoleID: "ar-free", CreatedAt: now,
	})
	_ = store.UpsertAssumableRoleSession(context.Background(), domain.AssumableRoleSession{
		ID: "sess-1", TenantID: "tenant-1", AccountID: "acct-1", AssumableRoleID: "ar-active",
		ExpiresAt: time.Now().UTC().Add(time.Hour), CreatedAt: now,
	})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	if _, err := svc.IAM().DeleteAssumableRole(ctx, "ar-active"); err == nil {
		t.Fatal("expected conflict when assumable role has active sessions")
	}

	deleted, err := svc.IAM().DeleteAssumableRole(ctx, "ar-free")
	if err != nil {
		t.Fatal(err)
	}
	if deleted.ID != "ar-free" {
		t.Fatalf("expected deleted assumable role, got %+v", deleted)
	}
	if _, ok, err := store.GetAssumableRole(context.Background(), "tenant-1", "ar-free"); err != nil || ok {
		t.Fatalf("expected assumable role removed, ok=%v err=%v", ok, err)
	}
	account, ok, err := store.GetAccount(context.Background(), "tenant-1", "acct-1")
	if err != nil || !ok {
		t.Fatalf("expected account, ok=%v err=%v", ok, err)
	}
	if account.ActiveAssumableRoleID != "" {
		t.Fatalf("expected active_assumable_role_id cleared, got %q", account.ActiveAssumableRoleID)
	}
}
