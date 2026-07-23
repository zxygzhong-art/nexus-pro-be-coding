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
	queriesRaw, err := os.ReadFile("../../../db/queries/core.sql")
	if err != nil {
		t.Fatal(err)
	}

	schema := string(schemaRaw)
	for _, fragment := range []string{
		"('pending_start', 'starting', 'started', 'abandoned')",
		"workflow_runs_temporal_start_claimable_idx",
		"WHERE temporal_start_status IN ('pending_start', 'starting')",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("schema missing Temporal claim fragment %q", fragment)
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
