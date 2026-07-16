package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func TestPermissionConditionsAreRejectedFromPermissionSetAuthoring(t *testing.T) {
	now := time.Date(2026, 7, 16, 3, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-admin", TenantID: "tenant-1", Name: "Admin",
		Permissions: []domain.Permission{
			{Resource: "iam.permission_set", Action: domain.ActionCreate, Scope: domain.ScopeAll},
			{Resource: "iam.permission_set", Action: domain.ActionUpdate, Scope: domain.ScopeAll},
		},
		CreatedAt: now,
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-target", TenantID: "tenant-1", Name: "Target",
		Permissions: []domain.Permission{{Resource: "hr.employee", Action: domain.ActionRead, Scope: domain.ScopeSelf}}, CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-admin"}, CreatedAt: now,
	})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	_, err := svc.IAM().CreatePermissionSet(ctx, domain.CreatePermissionSetInput{
		Name: "Misleading conditions",
		Permissions: []domain.Permission{{
			Resource: "hr.employee", Action: domain.ActionRead,
			Conditions: map[string]any{"employee_ids": []string{"emp-1"}},
		}},
	})
	assertPermissionConditionsReadOnlyError(t, err)

	_, err = svc.IAM().UpdatePermissionSet(ctx, "ps-target", domain.UpdatePermissionSetInput{
		Permissions: []domain.Permission{{
			Resource: "hr.employee", Action: domain.ActionRead,
			Conditions: map[string]any{},
		}},
	})
	assertPermissionConditionsReadOnlyError(t, err)

	target, ok, err := store.GetPermissionSet(context.Background(), "tenant-1", "ps-target")
	if err != nil || !ok || len(target.Permissions) != 1 || target.Permissions[0].Scope != domain.ScopeSelf {
		t.Fatalf("rejected update must leave the stored permission set unchanged, target=%+v ok=%t err=%v", target, ok, err)
	}
}

func TestPermissionConditionsAreRejectedFromPermissionPackages(t *testing.T) {
	content := service.DefaultHRPermissionPackageContent()
	content.Permissions = append([]domain.Permission(nil), content.Permissions...)
	content.Permissions[0].Conditions = map[string]any{"employee_ids": []string{"emp-1"}}
	if err := service.ValidatePermissionPackageContent(content); err == nil {
		t.Fatal("expected top-level package permission conditions to be rejected")
	}

	content = service.DefaultHRPermissionPackageContent()
	content.PermissionSetTemplates = append([]domain.PermissionSetTemplateContent(nil), content.PermissionSetTemplates...)
	content.PermissionSetTemplates[0].Permissions = append([]domain.Permission(nil), content.PermissionSetTemplates[0].Permissions...)
	content.PermissionSetTemplates[0].Permissions[0].Conditions = map[string]any{}
	if err := service.ValidatePermissionPackageContent(content); err == nil {
		t.Fatal("expected permission-set template conditions to be rejected")
	}
}

func assertPermissionConditionsReadOnlyError(t *testing.T, err error) {
	t.Helper()
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Status != 400 || appErr.Message != "permission conditions are read-only" {
		t.Fatalf("expected conditions read-only validation error, got %v", err)
	}
}
