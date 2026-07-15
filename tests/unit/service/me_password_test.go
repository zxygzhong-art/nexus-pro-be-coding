package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

type recordingPasswordChanger struct {
	input domain.IdentityPasswordChangeInput
	err   error
}

func (c *recordingPasswordChanger) ChangePassword(_ context.Context, input domain.IdentityPasswordChangeInput) error {
	c.input = input
	return c.err
}

// TestMeChangePasswordUsesOnlyBoundIdentity verifies callers cannot choose another Keycloak subject.
func TestMeChangePasswordUsesOnlyBoundIdentity(t *testing.T) {
	store, ctx := seedMePasswordAccount(t)
	changer := &recordingPasswordChanger{}
	svc := service.New(store, service.Options{IdentityPasswordChanger: changer})
	err := svc.Me().ChangePassword(ctx, domain.ChangePasswordInput{
		CurrentPassword: "old-password", NewPassword: "new-password", Confirmation: "new-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	if changer.input.Subject != "kc-subject-1" || changer.input.TenantID != "tenant-1" || changer.input.AccountID != "acct-1" {
		t.Fatalf("unexpected bound identity input: %+v", changer.input)
	}
}

// TestMeChangePasswordMapsCredentialErrors keeps invalid credentials actionable without exposing provider details.
func TestMeChangePasswordMapsCredentialErrors(t *testing.T) {
	store, ctx := seedMePasswordAccount(t)
	changer := &recordingPasswordChanger{err: domain.ErrIdentityCurrentPasswordInvalid}
	err := service.New(store, service.Options{IdentityPasswordChanger: changer}).Me().ChangePassword(ctx, domain.ChangePasswordInput{
		CurrentPassword: "wrong", NewPassword: "new-password", Confirmation: "new-password",
	})
	var appErr *domain.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected app error, got %v", err)
	}
	if appErr.Status != 400 || appErr.NumericCode() != domain.ErrorCodeCurrentPasswordInvalid || appErr.ReasonCode != "current_password_invalid" {
		t.Fatalf("unexpected current-password error: %+v", appErr)
	}
}

func seedMePasswordAccount(t *testing.T) (*memory.Store, domain.RequestContext) {
	t.Helper()
	now := time.Date(2026, 7, 15, 2, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	ctx := context.Background()
	if err := store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-1", Name: "Tenant", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(ctx, domain.PermissionSet{
		ID: "ps-me", TenantID: "tenant-1", Name: "Self service",
		Permissions: []domain.Permission{{Resource: "me", Action: "update", Scope: "self"}}, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(ctx, domain.Account{
		ID: "acct-1", TenantID: "tenant-1", EmployeeID: "emp-1", DisplayName: "Employee", Status: "active", DirectPermissionSetIDs: []string{"ps-me"}, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUserIdentity(ctx, domain.UserIdentity{
		ID: "uid-1", TenantID: "tenant-1", AccountID: "acct-1", Provider: domain.IdentityProviderKeycloak, Subject: "kc-subject-1", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	return store, domain.RequestContext{Context: ctx, TenantID: "tenant-1", AccountID: "acct-1"}
}
