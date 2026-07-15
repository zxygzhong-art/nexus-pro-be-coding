package postgres_integration_test

import (
	"context"
	"testing"
	"time"
)

// TestPostgresWorkflowSchemaMatchesRuntimeContract verifies the migrated database accepts empty current-stage states safely.
func TestPostgresWorkflowSchemaMatchesRuntimeContract(t *testing.T) {
	pool := openIntegrationPool(t)
	defer pool.Close()
	requireWorkflowRuntimeSchema(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var nullable, hasDefault, hasDeferredForeignKey bool
	if err := pool.QueryRow(ctx, `
		SELECT NOT attribute.attnotnull,
		       attribute.atthasdef,
		       EXISTS (
		           SELECT 1
		           FROM pg_constraint constraint_definition
		           WHERE constraint_definition.conrelid = 'workflow_runs'::regclass
		             AND constraint_definition.conname = 'workflow_runs_current_stage_fk'
		             AND constraint_definition.contype = 'f'
		             AND constraint_definition.condeferrable
		             AND constraint_definition.condeferred
		       )
		FROM pg_attribute attribute
		WHERE attribute.attrelid = 'workflow_runs'::regclass
		  AND attribute.attname = 'current_stage_instance_id'
		  AND NOT attribute.attisdropped
	`).Scan(&nullable, &hasDefault, &hasDeferredForeignKey); err != nil {
		t.Fatal(err)
	}
	if !nullable || hasDefault || !hasDeferredForeignKey {
		t.Fatalf("workflow current-stage schema mismatch: nullable=%t has_default=%t deferred_fk=%t", nullable, hasDefault, hasDeferredForeignKey)
	}
}
