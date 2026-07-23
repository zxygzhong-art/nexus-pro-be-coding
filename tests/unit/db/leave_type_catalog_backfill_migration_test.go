package db_test

import (
	"os"
	"strings"
	"testing"
)

func TestLeaveTypeCatalogSchemaModelsSourceHierarchyAndCodeIdentity(t *testing.T) {
	raw, err := os.ReadFile("../../../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, fragment := range []string{
		"CREATE TABLE leave_types (",
		"kind text NOT NULL",
		"parent_id text",
		"parent_code text",
		"CHECK (kind IN ('category', 'item', 'special_group'))",
		"leave_types_tenant_kind_code_idx UNIQUE (tenant_id, kind, code)",
		"FOREIGN KEY (tenant_id, parent_id)",
		"REFERENCES leave_types (tenant_id, id)",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("expected hierarchical leave_types schema fragment %q", fragment)
		}
	}
	for _, table := range []string{
		"leave_balances",
		"leave_request_allocations",
		"leave_cases",
		"leave_external_records",
		"leave_balance_entries",
	} {
		if !strings.Contains(schema, "CREATE TABLE "+table+" (") {
			t.Fatalf("expected leave catalog consumer table %s in schema", table)
		}
	}
	if strings.Contains(schema, "CREATE TABLE leave_requests (") {
		t.Fatal("final schema must not retain typed leave_requests table")
	}
}
