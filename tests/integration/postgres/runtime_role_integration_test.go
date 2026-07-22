package postgres_integration_test

import (
	"context"
	"testing"
	"time"

	pgplatform "nexus-pro-api/internal/platform/postgres"
)

// TestPostgresRuntimeRoleBoundary proves that the migration connection is
// rejected by the runtime policy while the provisioned application role is
// accepted through the same database boundary.
func TestPostgresRuntimeRoleBoundary(t *testing.T) {
	migrationPool := openIntegrationPool(t)
	defer migrationPool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	migrationRole, err := pgplatform.InspectRuntimeRole(ctx, migrationPool)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if err := migrationRole.Validate(); err == nil {
		t.Fatalf("expected migration role %q to be rejected for API runtime use", migrationRole.Name)
	}

	runtimePool := openRLSIntegrationPool(t)
	defer runtimePool.Close()
	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	runtimeRole, err := pgplatform.InspectRuntimeRole(ctx, runtimePool)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if err := runtimeRole.Validate(); err != nil {
		t.Fatalf("expected provisioned runtime role to be accepted, got %v", err)
	}
}
