package db_test

import (
	"os"
	"strings"
	"testing"
)

func TestLeaveStorageUsesThreeAnnualTables(t *testing.T) {
	schema := readLeaveSchemaFile(t, "../../../db/schema.sql")
	normalized := normalizeSQL(schema)

	balances := normalizedTableDefinition(t, schema, "leave_balances")
	requireSQLFragments(t, balances, []string{
		"entitlement_year integer NOT NULL",
		"granted_minutes integer NOT NULL DEFAULT 0",
		"used_minutes integer NOT NULL DEFAULT 0",
		"remaining_minutes integer NOT NULL DEFAULT 0",
		"source text NOT NULL CHECK (source IN ('nexus', 'ehrms'))",
		"CONSTRAINT leave_balances_employee_type_year_idx UNIQUE (tenant_id, employee_id, leave_type_id, entitlement_year)",
	})
	forbidSQLFragments(t, balances, []string{
		"period_start", "period_end", "carry_in_minutes", "carry_expire",
		"external_leave_code", "external_category_code", "raw_payload",
	})

	records := normalizedTableDefinition(t, schema, "leave_records")
	requireSQLFragments(t, records, []string{
		"balance_id text NOT NULL",
		"entitlement_year integer NOT NULL",
		"source text NOT NULL CHECK (source IN ('nexus', 'ehrms'))",
		"event_date timestamptz NOT NULL",
		"matched_record_id text",
		"CONSTRAINT leave_records_year_check CHECK",
		"REFERENCES leave_balances (tenant_id, id, employee_id, leave_type_id, entitlement_year)",
	})
	forbidSQLFragments(t, records, []string{
		"external_ref", "leave_name", "gross_minutes", "deduct_minutes",
		"source_label", "raw_payload", "payload_hash", "first_seen_at",
	})

	entries := normalizedTableDefinition(t, schema, "leave_balance_entries")
	requireSQLFragments(t, entries, []string{
		"leave_record_id text",
		"entitlement_year integer NOT NULL",
		"amount_minutes integer NOT NULL CHECK (amount_minutes <> 0)",
		"CONSTRAINT leave_balance_entries_record_fk FOREIGN KEY",
		"CONSTRAINT leave_balance_entries_idempotency_idx UNIQUE (tenant_id, idempotency_key)",
	})
	forbidSQLFragments(t, entries, []string{
		"allocation_id", "leave_request_id", "leave_case_id", "overtime_request_id", "metadata",
	})

	forbidSQLFragments(t, normalized, []string{
		"CREATE TABLE leave_request_allocations (",
		"CREATE TABLE leave_cases (",
		"CREATE TABLE leave_external_records (",
		"CREATE TABLE leave_case_sources (",
		"CREATE TABLE leave_balance_ledger (",
	})
}

func TestLeaveQueriesUseDirectRecordAndAnnualBalance(t *testing.T) {
	queries := readLeaveSchemaFile(t, "../../../db/queries/core.sql")
	appendEntry := normalizedNamedQuery(t, queries, "AppendLeaveBalanceEntry")
	requireSQLFragments(t, appendEntry, []string{
		"LEFT JOIN leave_records record",
		"record.balance_id = balance.id",
		"ON CONFLICT (tenant_id, idempotency_key) DO NOTHING",
	})
	overlay := normalizedNamedQuery(t, queries, "GetLeaveBalanceForOverlay")
	requireSQLFragments(t, overlay, []string{
		"balance.entitlement_year = EXTRACT( YEAR FROM sqlc.arg(as_of)::timestamptz AT TIME ZONE 'Asia/Shanghai' )::integer",
	})
	upsert := normalizedNamedQuery(t, queries, "UpsertLeaveRecord")
	requireSQLFragments(t, upsert, []string{
		"INSERT INTO leave_records",
		"matched_record_id",
		"reconciliation_status",
	})
	forbidSQLFragments(t, normalizeSQL(queries), []string{
		"leave_request_allocations", "leave_cases", "leave_external_records", "leave_case_sources",
	})
}

func TestLeaveTablesKeepTenantRLSAndIndexes(t *testing.T) {
	schema := normalizeSQL(readLeaveSchemaFile(t, "../../../db/schema.sql"))
	for _, table := range []string{"leave_balances", "leave_records", "leave_balance_entries"} {
		requireSQLFragments(t, schema, []string{
			"ALTER TABLE " + table + " ENABLE ROW LEVEL SECURITY",
			"ALTER TABLE " + table + " FORCE ROW LEVEL SECURITY",
			"CREATE POLICY tenant_isolation_" + table + " ON " + table,
		})
	}
	requireSQLFragments(t, schema, []string{
		"CREATE INDEX leave_balances_employee_year_idx",
		"CREATE INDEX leave_records_employee_interval_idx",
		"CREATE UNIQUE INDEX leave_records_ehrms_match_idx",
		"CREATE INDEX leave_balance_entries_record_idx",
	})
}

func TestLeaveStorageIsSquashedIntoPostInitMigration(t *testing.T) {
	migration := normalizeSQL(readLeaveSchemaFile(t, "../../../db/migrations/000002_post_init_updates.sql"))
	requireSQLFragments(t, migration, []string{
		"CREATE TABLE leave_balances (",
		"CREATE TABLE leave_records (",
		"CREATE TABLE leave_balance_entries (",
	})
	forbidSQLFragments(t, migration, []string{
		"CREATE TABLE leave_request_allocations (",
		"CREATE TABLE leave_cases (",
		"CREATE TABLE leave_external_records (",
		"CREATE TABLE leave_case_sources (",
		"CREATE TABLE leave_balance_ledger (",
	})
}

func readLeaveSchemaFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func normalizedTableDefinition(t *testing.T, sql, table string) string {
	t.Helper()
	marker := "CREATE TABLE " + table + " ("
	start := strings.Index(sql, marker)
	if start < 0 {
		t.Fatalf("table %s is missing", table)
	}
	end := strings.Index(sql[start:], "\n);")
	if end < 0 {
		t.Fatalf("table %s definition is incomplete", table)
	}
	return normalizeSQL(sql[start : start+end+3])
}

func normalizedNamedQuery(t *testing.T, sql, name string) string {
	t.Helper()
	marker := "-- name: " + name + " "
	start := strings.Index(sql, marker)
	if start < 0 {
		t.Fatalf("query %s is missing", name)
	}
	rest := sql[start+len(marker):]
	end := strings.Index(rest, "\n-- name:")
	if end < 0 {
		end = len(rest)
	}
	return normalizeSQL(rest[:end])
}

func normalizeSQL(sql string) string {
	return strings.Join(strings.Fields(sql), " ")
}

func canonicalSQL(sql string) string {
	return strings.NewReplacer("( ", "(", " )", ")", ", ", ",").Replace(normalizeSQL(sql))
}

func requireSQLFragments(t *testing.T, sql string, fragments []string) {
	t.Helper()
	for _, fragment := range fragments {
		normalized := normalizeSQL(fragment)
		if !strings.Contains(sql, normalized) {
			t.Errorf("SQL contract is missing %q", normalized)
		}
	}
}

func forbidSQLFragments(t *testing.T, sql string, fragments []string) {
	t.Helper()
	for _, fragment := range fragments {
		normalized := normalizeSQL(fragment)
		if strings.Contains(sql, normalized) {
			t.Errorf("SQL contract still contains forbidden fragment %q", normalized)
		}
	}
}
