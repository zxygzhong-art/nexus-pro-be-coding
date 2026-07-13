package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func TestIAMPermissionSetUpdateAuditsAndReplacesItems(t *testing.T) {
	now := time.Date(2026, 7, 8, 14, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-admin",
		TenantID: "tenant-1",
		Name:     "IAM Admin",
		Permissions: []domain.Permission{
			{Resource: "iam.permission_set", Action: "update", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:          "ps-target",
		TenantID:    "tenant-1",
		Name:        "Old Name",
		Description: "old description",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "self"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-admin"}, CreatedAt: now})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	name := "People Admin"
	description := "updated description"
	updated, err := svc.IAM().UpdatePermissionSet(ctx, "ps-target", domain.UpdatePermissionSetInput{
		Name:        &name,
		Description: &description,
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
			{Resource: "hr.employee", Action: "update", Scope: "all"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != name || updated.Description != description || len(updated.Permissions) != 2 {
		t.Fatalf("expected permission set to be updated, got %+v", updated)
	}
	items, err := store.ListPermissionSetItemsForSet(context.Background(), "tenant-1", "ps-target")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected permission set items to be replaced, got %+v", items)
	}
	version, err := store.GetPermissionVersion(context.Background(), "tenant-1")
	if err != nil || version != 1 {
		t.Fatalf("expected permission version 1 after permission set update, version=%d err=%v", version, err)
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findAuditLog(logs, "iam.permission_set.update"); !ok {
		t.Fatalf("expected permission set update audit, got %+v", logs)
	}
}

func TestIAMPermissionSetAssignmentDeleteAuditsAndInvalidatesPermissionVersion(t *testing.T) {
	now := time.Date(2026, 7, 8, 14, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-admin",
		TenantID: "tenant-1",
		Name:     "IAM Admin",
		Permissions: []domain.Permission{
			{Resource: "iam.permission_set_assignment", Action: "delete", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{ID: "ps-target", TenantID: "tenant-1", Name: "People Reader", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-admin"}, CreatedAt: now})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID:              "psa-1",
		TenantID:        "tenant-1",
		PrincipalType:   string(domain.PrincipalTypeAccount),
		PrincipalID:     "acct-1",
		PermissionSetID: "ps-target",
		Effect:          "allow",
		CreatedAt:       now,
	})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	deleted, err := svc.IAM().DeletePermissionSetAssignment(ctx, "psa-1")
	if err != nil {
		t.Fatal(err)
	}
	if deleted.ID != "psa-1" {
		t.Fatalf("expected deleted assignment, got %+v", deleted)
	}
	assignments, err := store.ListPermissionSetAssignments(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 0 {
		t.Fatalf("expected assignment removed, got %+v", assignments)
	}
	version, err := store.GetPermissionVersion(context.Background(), "tenant-1")
	if err != nil || version != 1 {
		t.Fatalf("expected permission version 1 after assignment delete, version=%d err=%v", version, err)
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findAuditLog(logs, "iam.permission_assignment.delete"); !ok {
		t.Fatalf("expected permission assignment delete audit, got %+v", logs)
	}
}

func TestIAMPermissionSetAssignmentPageFiltersPrincipalWithoutExpiryFilter(t *testing.T) {
	now := time.Date(2026, 7, 8, 15, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-reader",
		TenantID: "tenant-1",
		Name:     "Assignment Reader",
		Permissions: []domain.Permission{
			{Resource: "iam.permission_set_assignment", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-reader"}, CreatedAt: now})
	expiredAt := now.Add(-time.Hour)
	futureStart := now.Add(time.Hour)
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{ID: "psa-expired", TenantID: "tenant-1", PrincipalType: "account", PrincipalID: "acct-1", PermissionSetID: "ps-reader", Effect: "allow", ExpiresAt: &expiredAt, CreatedAt: now})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{ID: "psa-future", TenantID: "tenant-1", PrincipalType: "account", PrincipalID: "acct-1", PermissionSetID: "ps-reader", Effect: "allow", StartsAt: &futureStart, CreatedAt: now.Add(time.Second)})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{ID: "psa-other", TenantID: "tenant-1", PrincipalType: "account", PrincipalID: "acct-2", PermissionSetID: "ps-reader", Effect: "allow", CreatedAt: now.Add(2 * time.Second)})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	page, err := svc.IAM().ListPermissionSetAssignmentPage(ctx, domain.PermissionSetAssignmentQuery{
		PrincipalType: "account",
		PrincipalID:   "acct-1",
	}, domain.PageRequest{Page: 1, PageSize: 10, Sort: "created_at_asc"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || page.Items[0].ID != "psa-expired" || page.Items[1].ID != "psa-future" {
		t.Fatalf("expected all principal assignments without expiry filter, got %+v", page)
	}
}

func TestIAMAccountPageFiltersKeyword(t *testing.T) {
	now := time.Date(2026, 7, 8, 15, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-reader",
		TenantID: "tenant-1",
		Name:     "Account Reader",
		Permissions: []domain.Permission{
			{Resource: "iam.account", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-admin", TenantID: "tenant-1", DisplayName: "IAM Admin", Email: "admin@example.com", Status: "active", DirectPermissionSetIDs: []string{"ps-reader"}, CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-operator", TenantID: "tenant-1", DisplayName: "Operator", Email: "ops@example.com", Status: "active", CreatedAt: now.Add(time.Second)})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}

	page, err := svc.IAM().ListIamAccountPage(ctx, "ops", domain.PageRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Items[0].ID != "acct-operator" || page.Items[0].DisplayName != "Operator" {
		t.Fatalf("expected keyword-filtered account projection, got %+v", page)
	}
}
