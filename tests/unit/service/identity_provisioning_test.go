package service_test

import (
	"context"
	"errors"
	"testing"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// TestEmployeeCreateSucceedsAndKeepsOutboxPendingWhenKeycloakFails 驗證員工 create succeeds and keeps outbox pending when Keycloak fails。
func TestEmployeeCreateSucceedsAndKeepsOutboxPendingWhenKeycloakFails(t *testing.T) {
	provisioner := &recordingIdentityProvisioner{err: errors.New("keycloak unavailable")}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "create", Scope: "all"},
	}, service.Options{IdentityProvisioner: provisioner})

	input := validEmployeeInput("E1950", "Outbox Pending", "outbox.pending@example.com")
	input.AccountPolicy = "create_active"
	created, err := svc.HR().CreateEmployee(ctx, input)
	if err != nil {
		t.Fatalf("expected employee creation to succeed despite keycloak outage, got %v", err)
	}
	if created.AccountID == "" {
		t.Fatalf("expected account link, got %+v", created)
	}
	if len(provisioner.inputs) != 1 {
		t.Fatalf("expected one fast-path provisioning attempt, got %+v", provisioner.inputs)
	}
	identities, err := store.ListUserIdentities(context.Background(), "tenant-1", created.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(identities) != 0 {
		t.Fatalf("expected no identity binding while keycloak is down, got %+v", identities)
	}
	pending, err := store.ListPendingIdentityProvisioningOutboxEvents(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].AccountID != created.AccountID || pending[0].RetryCount != 1 || pending[0].LastError == "" {
		t.Fatalf("expected one pending outbox event with recorded failure, got %+v", pending)
	}
	if pending[0].Email != "outbox.pending@example.com" || !pending[0].Enabled || pending[0].SendInvite {
		t.Fatalf("expected outbox event to snapshot provisioning input, got %+v", pending[0])
	}
}

// TestEmployeeCreateFastPathWritesIdentityAndDrainsOutbox 驗證員工 create fast path writes 身分 and drains outbox。
func TestEmployeeCreateFastPathWritesIdentityAndDrainsOutbox(t *testing.T) {
	provisioner := &recordingIdentityProvisioner{}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "create", Scope: "all"},
	}, service.Options{IdentityProvisioner: provisioner})

	input := validEmployeeInput("E1951", "Fast Path", "fast.path@example.com")
	input.AccountPolicy = "create_pending_invite"
	created, err := svc.HR().CreateEmployee(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(provisioner.inputs) != 1 || !provisioner.inputs[0].SendInvite {
		t.Fatalf("expected one invited provisioning call, got %+v", provisioner.inputs)
	}
	identities, err := store.ListUserIdentities(context.Background(), "tenant-1", created.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(identities) != 1 || identities[0].Subject != "kc-"+created.AccountID {
		t.Fatalf("expected keycloak identity binding from fast path, got %+v", identities)
	}
	pending, err := store.ListPendingIdentityProvisioningOutboxEvents(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected outbox to be drained after fast path success, got %+v", pending)
	}
}
