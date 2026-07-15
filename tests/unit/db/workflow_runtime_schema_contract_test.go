package db_test

import (
	"os"
	"strings"
	"testing"
)

// TestWorkflowRuntimeSchemaAllowsNoCurrentStage keeps initialization and terminal workflow states representable.
func TestWorkflowRuntimeSchemaAllowsNoCurrentStage(t *testing.T) {
	raw, err := os.ReadFile("../../../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	start := strings.Index(schema, "CREATE TABLE workflow_runs (")
	if start < 0 {
		t.Fatal("workflow_runs table is missing")
	}
	end := strings.Index(schema[start:], "\n);")
	if end < 0 {
		t.Fatal("workflow_runs table definition is incomplete")
	}
	definition := schema[start : start+end]
	if !strings.Contains(definition, "current_stage_instance_id text,") {
		t.Fatal("current_stage_instance_id must be nullable")
	}
	if strings.Contains(definition, "current_stage_instance_id text NOT NULL") || strings.Contains(definition, "current_stage_instance_id text DEFAULT") {
		t.Fatal("current_stage_instance_id must use NULL instead of an empty-string sentinel")
	}
	for _, fragment := range []string{
		"ADD CONSTRAINT workflow_runs_current_stage_fk",
		"FOREIGN KEY (tenant_id, id, current_stage_instance_id)",
		"REFERENCES workflow_stage_instances (tenant_id, run_id, id)",
		"DEFERRABLE INITIALLY DEFERRED",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("expected workflow current-stage constraint fragment %q", fragment)
		}
	}
}

// TestWorkflowRuntimeInitAllowsNoCurrentStage keeps the squashed migration aligned with the runtime schema.
func TestWorkflowRuntimeInitAllowsNoCurrentStage(t *testing.T) {
	raw, err := os.ReadFile("../../../db/migrations/000001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration := string(raw)
	for _, fragment := range []string{
		"current_stage_instance_id text,",
		"CONSTRAINT workflow_stage_instances_run_identity_idx UNIQUE (tenant_id, run_id, id)",
		"ADD CONSTRAINT workflow_runs_current_stage_fk",
		"FOREIGN KEY (tenant_id, id, current_stage_instance_id)",
		"REFERENCES workflow_stage_instances (tenant_id, run_id, id)",
		"DEFERRABLE INITIALLY DEFERRED",
	} {
		if !strings.Contains(migration, fragment) {
			t.Fatalf("expected workflow current-stage init fragment %q", fragment)
		}
	}
}
