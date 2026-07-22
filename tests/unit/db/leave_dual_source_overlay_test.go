package db_test

import (
	"os"
	"strings"
	"testing"
)

func TestLeaveDualSourceOverlaySchema(t *testing.T) {
	schema := readLeaveSchemaFile(t, "../../../db/schema.sql")
	for _, fragment := range []string{
		"CREATE TABLE leave_type_external_refs",
		"CREATE TABLE leave_cases",
		"CREATE TABLE external_leave_records",
		"CREATE TABLE leave_case_sources",
		"CREATE TABLE leave_balance_entries",
		"reconciliation_status text NOT NULL DEFAULT 'not_required'",
		"external_leave_code text NOT NULL DEFAULT ''",
		"external_reconcile",
		"tenant_isolation_leave_balance_entries",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("schema is missing dual-source leave fragment %q", fragment)
		}
	}
}

func TestLeaveDualSourceOverlayMigrationIsReversible(t *testing.T) {
	migration := readLeaveSchemaFile(t, "../../../db/migrations/000017_leave_dual_source_overlay.sql")
	for _, fragment := range []string{
		"-- +goose Up",
		"-- +goose Down",
		"-- +goose StatementBegin",
		"DROP TABLE IF EXISTS leave_balance_entries",
		"DROP TABLE IF EXISTS external_leave_records",
		"CREATE OR REPLACE FUNCTION append_leave_balance_ledger()",
	} {
		if !strings.Contains(migration, fragment) {
			t.Fatalf("migration is missing fragment %q", fragment)
		}
	}
}

func readLeaveSchemaFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
