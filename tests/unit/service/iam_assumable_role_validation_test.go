package service_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

const trustedRolePrincipalError = "trusted assumable role trust_policy must include at least one non-empty principal in accounts, account_ids, user_groups, or user_group_ids"

func TestCreateAssumableRoleRejectsTrustedPolicyWithoutPrincipal(t *testing.T) {
	_, svc, ctx := newAssumableRoleValidationFixture(t)

	tests := []struct {
		name   string
		policy map[string]any
	}{
		{name: "metadata only", policy: map[string]any{"require_reason": true}},
		{name: "empty recognized lists", policy: map[string]any{"accounts": []string{}, "user_groups": []any{}}},
		{name: "blank recognized principals", policy: map[string]any{"account_ids": []any{""}, "user_group_ids": []string{"   "}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.IAM().CreateAssumableRole(ctx, domain.CreateAssumableRoleInput{
				Name:               "Invalid trusted role",
				Trusted:            true,
				TrustPolicy:        tt.policy,
				PermissionBoundary: map[string]any{"allow": []string{"audit.log.read"}},
			})
			assertTrustedRolePrincipalBadRequest(t, err)
		})
	}
}

func TestUpdateAssumableRoleRejectsEnablingTrustedWithoutPrincipal(t *testing.T) {
	store, svc, ctx := newAssumableRoleValidationFixture(t)
	role, err := svc.IAM().CreateAssumableRole(ctx, domain.CreateAssumableRoleInput{
		Name:               "Initially untrusted",
		Trusted:            false,
		TrustPolicy:        map[string]any{"require_reason": true},
		PermissionBoundary: map[string]any{"allow": []string{"audit.log.read"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	trusted := true
	_, err = svc.IAM().UpdateAssumableRole(ctx, role.ID, domain.UpdateAssumableRoleInput{Trusted: &trusted})
	assertTrustedRolePrincipalBadRequest(t, err)

	stored, ok, err := store.GetAssumableRole(context.Background(), ctx.TenantID, role.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected original role to remain stored")
	}
	if stored.Trusted {
		t.Fatal("rejected update must not persist trusted=true")
	}
}

func TestCreateAssumableRoleAcceptsTrustedAccountOrGroupPrincipal(t *testing.T) {
	_, svc, ctx := newAssumableRoleValidationFixture(t)

	tests := []struct {
		name   string
		policy map[string]any
	}{
		{name: "accounts", policy: map[string]any{"accounts": []string{"acct-target"}}},
		{name: "account_ids", policy: map[string]any{"account_ids": []any{"acct-target"}}},
		{name: "user_groups", policy: map[string]any{"user_groups": []string{"ug-target"}}},
		{name: "user_group_ids", policy: map[string]any{"user_group_ids": []any{"ug-target"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, err := svc.IAM().CreateAssumableRole(ctx, domain.CreateAssumableRoleInput{
				Name:               "Valid trusted role " + tt.name,
				Trusted:            true,
				TrustPolicy:        tt.policy,
				PermissionBoundary: map[string]any{"allow": []string{"audit.log.read"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !role.Trusted {
				t.Fatal("expected trusted role to be created")
			}
		})
	}
}

func newAssumableRoleValidationFixture(t *testing.T) (*memory.Store, *service.Service, domain.RequestContext) {
	t.Helper()
	now := time.Date(2026, 7, 15, 15, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	if err := store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-iam-admin",
		TenantID: "tenant-1",
		Name:     "IAM Admin",
		Permissions: []domain.Permission{
			{Resource: "iam.assumable_role", Action: "create", Scope: "all"},
			{Resource: "iam.assumable_role", Action: "update", Scope: "all"},
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-admin",
		TenantID:               "tenant-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-iam-admin"},
		CreatedAt:              now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID:        "acct-target",
		TenantID:  "tenant-1",
		Status:    "active",
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUserGroup(context.Background(), domain.UserGroup{
		ID:        "ug-target",
		TenantID:  "tenant-1",
		Name:      "Target group",
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	return store, service.New(store, service.Options{Now: func() time.Time { return now }}), domain.RequestContext{
		TenantID:  "tenant-1",
		AccountID: "acct-admin",
	}
}

func assertTrustedRolePrincipalBadRequest(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected trusted role trust policy validation to fail")
	}
	var appErr *domain.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Status != http.StatusBadRequest || appErr.Message != trustedRolePrincipalError {
		t.Fatalf("unexpected validation error: status=%d message=%q", appErr.Status, appErr.Message)
	}
}
