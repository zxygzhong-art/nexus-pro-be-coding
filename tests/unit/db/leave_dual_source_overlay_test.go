package db_test

import (
	"os"
	"strings"
	"testing"
)

func TestLeaveTargetModelUsesMinuteSnapshotsAndAppendOnlyEntries(t *testing.T) {
	schema := readLeaveSchemaFile(t, "../../../db/schema.sql")
	queries := readLeaveSchemaFile(t, "../../../db/queries/core.sql")

	balances := normalizedTableDefinition(t, schema, "leave_balances")
	requireSQLFragments(t, balances, []string{
		"remaining_minutes integer NOT NULL",
		"granted_minutes integer NOT NULL DEFAULT 0",
		"used_minutes integer NOT NULL DEFAULT 0",
		"carry_in_minutes integer NOT NULL DEFAULT 0 CHECK (carry_in_minutes >= 0)",
		"source text NOT NULL CHECK (source IN ('ehrms', 'explicit_snapshot', 'manual_snapshot', 'local_anchor'))",
		"CONSTRAINT leave_balances_nonnegative_check CHECK (remaining_minutes >= 0 AND granted_minutes >= 0 AND used_minutes >= 0)",
		"CONSTRAINT leave_balances_local_anchor_zero_check CHECK",
		"period_start IS NULL AND period_end IS NULL",
	})
	forbidSQLFragments(t, balances, []string{
		"remaining_hours",
		"granted_hours",
		"used_hours",
		"carry_in_hours",
	})
	requireSQLFragments(t, normalizeSQL(schema), []string{
		"CREATE INDEX leave_balances_fefo_idx ON leave_balances ( tenant_id, employee_id, leave_type_id, ((source = 'local_anchor')), period_end ASC NULLS LAST, period_start ASC NULLS FIRST, id )",
		"CREATE UNIQUE INDEX leave_balances_local_anchor_idx ON leave_balances (tenant_id, employee_id, leave_type_id) WHERE source = 'local_anchor'",
	})
	forbidSQLFragments(t, normalizeSQL(schema), []string{
		"CONSTRAINT leave_balances_period_no_overlap EXCLUDE USING gist",
		"CREATE UNIQUE INDEX leave_balances_snapshot_period_idx",
	})
	if strings.Contains(schema, "CREATE TABLE leave_balance_ledger (") {
		t.Fatal("legacy mutable leave_balance_ledger must not return")
	}

	requests := normalizedTableDefinition(t, schema, "leave_requests")
	requireSQLFragments(t, requests, []string{
		"requested_minutes integer NOT NULL CHECK (requested_minutes > 0)",
		"CONSTRAINT leave_requests_tenant_identity_idx UNIQUE (tenant_id, id, employee_id, leave_type_id)",
	})
	forbidSQLFragments(t, requests, []string{"requested_hours", "leave_balance_id"})

	entries := normalizedTableDefinition(t, schema, "leave_balance_entries")
	requireSQLFragments(t, entries, []string{
		"amount_minutes integer NOT NULL CHECK (amount_minutes <> 0)",
		"CONSTRAINT leave_balance_entries_sign_check CHECK",
		"CONSTRAINT leave_balance_entries_reference_shape_check CHECK",
		"CONSTRAINT leave_balance_entries_reconciliation_case_check CHECK",
		"CONSTRAINT leave_balance_entries_allocation_fk FOREIGN KEY ( tenant_id, allocation_id, leave_request_id, balance_id, employee_id, leave_type_id ) REFERENCES leave_request_allocations ( tenant_id, id, leave_request_id, leave_balance_id, employee_id, leave_type_id )",
		"CONSTRAINT leave_balance_entries_idempotency_idx UNIQUE (tenant_id, idempotency_key)",
	})
	forbidSQLFragments(t, entries, []string{"amount_hours"})

	appendEntry := normalizedNamedQuery(t, queries, "AppendLeaveBalanceEntry")
	requireSQLFragments(t, appendEntry, []string{
		"INSERT INTO leave_balance_entries",
		"allocation.id",
		"ON CONFLICT (tenant_id, idempotency_key) DO NOTHING",
	})
	appendStandalone := normalizedNamedQuery(t, queries, "AppendStandaloneLeaveBalanceEntry")
	requireSQLFragments(t, appendStandalone, []string{
		"INSERT INTO leave_balance_entries",
		"ON CONFLICT (tenant_id, idempotency_key) DO NOTHING",
	})
	forbidSQLFragments(t, normalizeSQL(queries), []string{
		"UPDATE leave_balance_entries",
		"DELETE FROM leave_balance_entries",
		"UPDATE leave_balances SET remaining_minutes",
	})
	upsertBalance := normalizedNamedQuery(t, queries, "UpsertLeaveBalance")
	requireSQLFragments(t, upsertBalance, []string{
		"ON CONFLICT (id) DO UPDATE SET",
	})
	forbidSQLFragments(t, upsertBalance, []string{
		"ON CONFLICT (tenant_id, employee_id, leave_type_id, period_start, period_end)",
	})
}

func TestLeaveRequestAllocationsAreCycleScopedAndImmutable(t *testing.T) {
	schema := readLeaveSchemaFile(t, "../../../db/schema.sql")
	queries := readLeaveSchemaFile(t, "../../../db/queries/core.sql")

	allocations := normalizedTableDefinition(t, schema, "leave_request_allocations")
	requireSQLFragments(t, allocations, []string{
		"id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY",
		"cycle integer NOT NULL CHECK (cycle > 0)",
		"reserved_minutes integer NOT NULL CHECK (reserved_minutes > 0)",
		"CONSTRAINT leave_request_allocations_request_balance_cycle_idx UNIQUE (tenant_id, leave_request_id, leave_balance_id, cycle)",
		"FOREIGN KEY ( tenant_id, leave_request_id, employee_id, leave_type_id ) REFERENCES leave_requests (tenant_id, id, employee_id, leave_type_id)",
		"FOREIGN KEY ( tenant_id, leave_balance_id, employee_id, leave_type_id ) REFERENCES leave_balances (tenant_id, id, employee_id, leave_type_id)",
	})

	upsert := normalizedNamedQuery(t, queries, "UpsertLeaveRequestAllocation")
	requireSQLFragments(t, upsert, []string{
		"sqlc.arg(cycle), sqlc.arg(reserved_minutes)",
		"ON CONFLICT (tenant_id, leave_request_id, leave_balance_id, cycle) DO NOTHING",
	})
	forbidSQLFragments(t, normalizeSQL(queries), []string{
		"DELETE FROM leave_request_allocations",
		"UPDATE leave_request_allocations SET",
	})
	byCycle := normalizedNamedQuery(t, queries, "ListLeaveRequestAllocationsByRequestCycle")
	requireSQLFragments(t, byCycle, []string{
		"leave_request_id = sqlc.arg(leave_request_id)",
		"cycle = sqlc.arg(cycle)",
	})
	readBack := normalizedNamedQuery(t, queries, "GetLeaveRequestAllocationByCycleBalance")
	requireSQLFragments(t, readBack, []string{
		"leave_request_id = sqlc.arg(leave_request_id)",
		"leave_balance_id = sqlc.arg(leave_balance_id)",
		"cycle = sqlc.arg(cycle)",
	})
}

func TestLeaveCasesUseTypedSourcesAndConfirmedCanonicalFacts(t *testing.T) {
	schema := readLeaveSchemaFile(t, "../../../db/schema.sql")
	queries := readLeaveSchemaFile(t, "../../../db/queries/core.sql")

	cases := normalizedTableDefinition(t, schema, "leave_cases")
	requireSQLFragments(t, cases, []string{
		"net_minutes integer NOT NULL CHECK (net_minutes > 0)",
		"status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'cancelled', 'corrected'))",
		"origin text NOT NULL CHECK (origin IN ('nexus', 'ehrms', 'both'))",
	})
	sources := normalizedTableDefinition(t, schema, "leave_case_sources")
	requireSQLFragments(t, sources, []string{
		"leave_request_id text",
		"external_leave_record_id text",
		"CONSTRAINT leave_case_sources_source_xor_check CHECK (num_nonnulls(leave_request_id, external_leave_record_id) = 1)",
		"CONSTRAINT leave_case_sources_request_fk FOREIGN KEY (tenant_id, leave_request_id) REFERENCES leave_requests (tenant_id, id)",
		"CONSTRAINT leave_case_sources_external_fk FOREIGN KEY (tenant_id, external_leave_record_id) REFERENCES external_leave_records (tenant_id, id)",
	})
	forbidSQLFragments(t, sources, []string{"source_type", "source_id"})
	requireSQLFragments(t, normalizeSQL(schema), []string{
		"CREATE UNIQUE INDEX leave_case_sources_request_idx ON leave_case_sources (tenant_id, leave_request_id) WHERE leave_request_id IS NOT NULL",
		"CREATE UNIQUE INDEX leave_case_sources_external_idx ON leave_case_sources (tenant_id, external_leave_record_id) WHERE external_leave_record_id IS NOT NULL",
	})

	for _, queryName := range []string{
		"UpsertLeaveRequestCaseSource",
		"UpsertExternalLeaveCaseSource",
		"GetLeaveCaseByLeaveRequestID",
		"GetLeaveCaseByExternalLeaveRecordID",
	} {
		normalizedNamedQuery(t, queries, queryName)
	}
	if strings.Contains(queries, "-- name: GetLeaveCaseBySource ") {
		t.Fatal("generic source_type/source_id leave case lookup must not replace typed source queries")
	}
	confirmed := normalizedNamedQuery(t, queries, "ListConfirmedActiveLeaveCasesByQuery")
	requireSQLFragments(t, confirmed, []string{
		"leave_case.status = 'active'",
		"source.match_status = 'confirmed'",
	})
}

func TestAttendanceTargetReadModelUsesDatesAndPolicyBoundDayProjections(t *testing.T) {
	schema := readLeaveSchemaFile(t, "../../../db/schema.sql")
	queries := readLeaveSchemaFile(t, "../../../db/queries/core.sql")

	for _, table := range []string{
		"attendance_clock_records",
		"attendance_daily_summaries",
		"attendance_day_projections",
		"attendance_correction_requests",
	} {
		definition := normalizedTableDefinition(t, schema, table)
		requireSQLFragments(t, definition, []string{"work_date date NOT NULL"})
		forbidSQLFragments(t, definition, []string{"work_date text NOT NULL"})
	}

	projection := normalizedTableDefinition(t, schema, "attendance_day_projections")
	requireSQLFragments(t, projection, []string{
		"PRIMARY KEY (tenant_id, employee_id, work_date)",
		"policy_version integer NOT NULL CHECK (policy_version > 0)",
		"worked_minutes integer NOT NULL DEFAULT 0 CHECK (worked_minutes >= 0)",
		"approved_leave_minutes integer NOT NULL DEFAULT 0 CHECK (approved_leave_minutes >= 0)",
		"pending_leave_minutes integer NOT NULL DEFAULT 0 CHECK (pending_leave_minutes >= 0)",
		"required_minutes integer NOT NULL DEFAULT 0 CHECK (required_minutes >= 0)",
		"overtime_minutes integer NOT NULL DEFAULT 0 CHECK (overtime_minutes >= 0)",
		"input_fingerprint text NOT NULL CHECK (btrim(input_fingerprint) <> '')",
		"CONSTRAINT attendance_day_projections_policy_fk FOREIGN KEY (tenant_id, policy_version) REFERENCES attendance_policy_versions (tenant_id, version)",
		"CONSTRAINT attendance_day_projections_clock_in_fk FOREIGN KEY (tenant_id, clock_in_record_id, employee_id) REFERENCES attendance_clock_records (tenant_id, id, employee_id)",
	})
	requireSQLFragments(t, normalizeSQL(schema), []string{
		"CREATE INDEX attendance_day_projections_tenant_date_status_idx ON attendance_day_projections (tenant_id, work_date, day_status, employee_id)",
	})

	upsert := normalizedNamedQuery(t, queries, "UpsertAttendanceDayProjection")
	requireSQLFragments(t, upsert, []string{
		"sqlc.arg(work_date)::date",
		"ON CONFLICT (tenant_id, employee_id, work_date) DO UPDATE SET",
		"input_fingerprint = EXCLUDED.input_fingerprint",
	})
	for _, queryName := range []string{
		"GetAttendanceDayProjection",
		"GetAttendanceDayProjectionForUpdate",
		"ListAttendanceDayProjections",
	} {
		query := normalizedNamedQuery(t, queries, queryName)
		requireSQLFragments(t, query, []string{"work_date"})
	}
	policyAsOf := normalizedNamedQuery(t, queries, "GetAttendancePolicyAsOf")
	requireSQLFragments(t, policyAsOf, []string{
		"effective_from <= sqlc.arg(as_of)::timestamptz",
		"ORDER BY effective_from DESC, version DESC",
	})
}

func TestAttendanceLeaveTablesKeepIndexesAndTenantRLS(t *testing.T) {
	schema := normalizeSQL(readLeaveSchemaFile(t, "../../../db/schema.sql"))

	for _, table := range []string{
		"leave_type_external_refs",
		"leave_balances",
		"leave_requests",
		"leave_request_allocations",
		"leave_cases",
		"external_leave_records",
		"leave_case_sources",
		"leave_balance_entries",
		"attendance_clock_records",
		"attendance_daily_summaries",
		"attendance_day_projections",
		"attendance_correction_requests",
	} {
		requireSQLFragments(t, schema, []string{
			"ALTER TABLE " + table + " ENABLE ROW LEVEL SECURITY",
			"ALTER TABLE " + table + " FORCE ROW LEVEL SECURITY",
			"CREATE POLICY tenant_isolation_" + table + " ON " + table + " USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true))",
		})
	}
	requireSQLFragments(t, schema, []string{
		"CREATE INDEX leave_balances_fefo_idx ON leave_balances ( tenant_id, employee_id, leave_type_id, ((source = 'local_anchor')), period_end ASC NULLS LAST, period_start ASC NULLS FIRST, id )",
		"CREATE INDEX leave_balance_entries_balance_idx ON leave_balance_entries (tenant_id, balance_id, occurred_at, id)",
		"CREATE INDEX leave_request_allocations_tenant_balance_idx ON leave_request_allocations (tenant_id, leave_balance_id)",
		"CREATE INDEX leave_cases_employee_interval_idx ON leave_cases (tenant_id, employee_id, start_at, end_at)",
		"CREATE INDEX attendance_clock_records_effective_boundary_idx ON attendance_clock_records (tenant_id, employee_id, work_date, direction, clocked_at, created_at, id) WHERE record_status = 'accepted' AND voided = false",
		"CREATE INDEX attendance_day_projections_tenant_date_status_idx ON attendance_day_projections (tenant_id, work_date, day_status, employee_id)",
	})
}

func TestAttendanceLeaveCleanSlateMigrationMatchesTargetSchema(t *testing.T) {
	schema := readLeaveSchemaFile(t, "../../../db/schema.sql")
	migration := readLeaveSchemaFile(t, "../../../db/migrations/000021_attendance_leave_clean_slate.sql")

	downMarker := strings.Index(migration, "-- +goose Down")
	if downMarker < 0 {
		t.Fatal("clean-slate migration must declare a goose Down section")
	}
	up := migration[:downMarker]
	requireSQLFragments(t, normalizeSQL(up), []string{
		"-- +goose Up",
		"DROP TABLE IF EXISTS leave_balance_ledger",
		"DROP FUNCTION IF EXISTS append_leave_balance_ledger()",
	})
	if strings.Contains(up, "CREATE TABLE leave_balance_ledger (") {
		t.Fatal("clean-slate migration must drop, not recreate, the legacy ledger")
	}

	for _, table := range []string{
		"leave_type_external_refs",
		"leave_balances",
		"leave_requests",
		"leave_request_allocations",
		"leave_cases",
		"external_leave_records",
		"leave_case_sources",
		"leave_balance_entries",
		"attendance_clock_records",
		"attendance_daily_summaries",
		"attendance_day_projections",
		"attendance_correction_requests",
	} {
		want := normalizedTableDefinition(t, schema, table)
		got := normalizedTableDefinition(t, up, table)
		if table == "leave_balance_entries" {
			// db/schema.sql adds this FK after overtime_requests is declared; the
			// clean-slate migration creates entries later and can declare it inline.
			overtimeFK := "CONSTRAINT leave_balance_entries_overtime_request_fk FOREIGN KEY (tenant_id, overtime_request_id, employee_id) REFERENCES overtime_requests (tenant_id, id, employee_id)"
			requireSQLFragments(t, got, []string{overtimeFK})
			got = strings.Replace(got, ", "+overtimeFK, "", 1)
			requireSQLFragments(t, normalizeSQL(schema), []string{
				"ALTER TABLE leave_balance_entries ADD " + overtimeFK,
			})
		}
		if canonicalSQL(got) != canonicalSQL(want) {
			t.Fatalf("migration definition for %s diverges from db/schema.sql\nwant: %s\n got: %s", table, want, got)
		}
		requireSQLFragments(t, normalizeSQL(up), []string{
			"ALTER TABLE " + table + " ENABLE ROW LEVEL SECURITY",
			"ALTER TABLE " + table + " FORCE ROW LEVEL SECURITY",
			"CREATE POLICY tenant_isolation_" + table + " ON " + table,
		})
	}

	down := normalizeSQL(migration[downMarker:])
	requireSQLFragments(t, down, []string{
		"-- +goose Down",
		"RAISE EXCEPTION '000021_attendance_leave_clean_slate is irreversible because legacy attendance and leave data was discarded'",
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
	return strings.NewReplacer(
		"( ", "(",
		" )", ")",
		", ", ",",
	).Replace(normalizeSQL(sql))
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
