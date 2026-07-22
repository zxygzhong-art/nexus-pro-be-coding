package db_test

import (
	"os"
	"strings"
	"testing"
)

func TestWorkflowTemporalDeliveryClaimContractStaysInSchema(t *testing.T) {
	schemaRaw, err := os.ReadFile("../../../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	migrationRaw, err := os.ReadFile("../../../db/migrations/000010_workflow_temporal_delivery.sql")
	if err != nil {
		t.Fatal(err)
	}
	queriesRaw, err := os.ReadFile("../../../db/queries/core.sql")
	if err != nil {
		t.Fatal(err)
	}

	for name, raw := range map[string]string{"schema": string(schemaRaw), "migration": string(migrationRaw)} {
		for _, fragment := range []string{
			"('pending_start', 'starting', 'started', 'abandoned')",
			"workflow_runs_temporal_start_claimable_idx",
			"WHERE temporal_start_status IN ('pending_start', 'starting')",
		} {
			if !strings.Contains(raw, fragment) {
				t.Fatalf("%s missing Temporal claim fragment %q", name, fragment)
			}
		}
	}
	migration := string(migrationRaw)
	for _, fragment := range []string{
		"cannot downgrade 000010 while Temporal starts are pending/starting or run-scoped executions are active",
		"event_type = 'workflow.form_approval.start_requested'",
		"status <> 'succeeded'",
	} {
		if !strings.Contains(migration, fragment) {
			t.Fatalf("migration missing safe downgrade guard %q", fragment)
		}
	}

	queries := string(queriesRaw)
	for _, fragment := range []string{
		"-- name: ListPendingWorkflowRuns :many",
		"temporal_start_status = 'starting'",
		"updated_at <= sqlc.arg(stale_before)::timestamptz",
		"-- name: ClaimWorkflowRunTemporalStart :one",
		"SET temporal_start_status = 'starting'",
		"-- name: ReleaseWorkflowRunTemporalStart :one",
		"-- name: MarkWorkflowRunTemporalStarted :one",
		"-- name: AbandonPendingWorkflowRunTemporalStart :one",
		"-- name: AbandonClaimedWorkflowRunTemporalStart :one",
		"AND updated_at = sqlc.arg(claimed_at)::timestamptz",
	} {
		if !strings.Contains(queries, fragment) {
			t.Fatalf("workflow queries missing Temporal fencing fragment %q", fragment)
		}
	}
}
