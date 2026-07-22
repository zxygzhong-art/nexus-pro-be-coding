package db_test

import (
	"os"
	"strings"
	"testing"
)

// TestEmployeeIntegrityConstraintsStayInSchema 驗證員工 integrity constraints stay in schema。
func TestEmployeeIntegrityConstraintsStayInSchema(t *testing.T) {
	raw, err := os.ReadFile("../../../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	required := []string{
		"CREATE UNIQUE INDEX employees_tenant_company_email_idx ON employees (tenant_id, lower(company_email)) WHERE company_email <> '';",
		"CREATE UNIQUE INDEX employees_tenant_personal_email_idx ON employees (tenant_id, lower(personal_email)) WHERE personal_email <> '';",
		"CREATE UNIQUE INDEX employees_tenant_national_id_idx ON employees (tenant_id, lower(basic_info->>'national_id')) WHERE coalesce(basic_info->>'national_id', '') <> '';",
		"CREATE UNIQUE INDEX employees_tenant_passport_no_idx ON employees (tenant_id, lower(basic_info->>'passport_no')) WHERE coalesce(basic_info->>'passport_no', '') <> '';",
		"CREATE OR REPLACE FUNCTION validate_employee_references()",
		"IF NEW.account_id <> '' AND NOT EXISTS",
		"IF NEW.org_unit_id <> '' AND NOT EXISTS",
		"CREATE TRIGGER employees_reference_check",
		"CREATE TABLE outbox_events (",
		"CREATE POLICY tenant_isolation_outbox_events",
		"CREATE OR REPLACE FUNCTION validate_authz_assignment_references()",
		"CREATE TRIGGER authz_permission_set_assignments_reference_check",
	}
	for _, item := range required {
		if !strings.Contains(schema, item) {
			t.Fatalf("expected employee integrity schema fragment %q", item)
		}
	}
}

// TestOutboxReliabilityContractStayInSchema keeps rolling-deploy-safe explicit
// projections and the lease/idempotency indexes aligned with the domain model.
func TestOutboxReliabilityContractStayInSchema(t *testing.T) {
	schemaRaw, err := os.ReadFile("../../../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	queriesRaw, err := os.ReadFile("../../../db/queries/core.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(schemaRaw)
	queries := string(queriesRaw)
	for _, fragment := range []string{
		"payload_version integer NOT NULL DEFAULT 1",
		"attempt_count integer NOT NULL DEFAULT 0",
		"max_attempts integer NOT NULL DEFAULT 5",
		"claim_token text NOT NULL DEFAULT ''",
		"dead_lettered_at timestamptz",
		"CREATE INDEX outbox_events_dispatch_due_idx",
		"CREATE INDEX outbox_events_expired_claim_idx",
		"CREATE UNIQUE INDEX outbox_events_idempotency_idx",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("expected outbox reliability schema fragment %q", fragment)
		}
	}
	for _, fragment := range []string{
		"-- name: ClaimOutboxEvents :many",
		"candidate.claim_expires_at <= sqlc.arg(claimed_at)",
		"claim_token = sqlc.arg(claim_token)::text || ':' || claimed.id",
		"-- name: FinalizeOutboxEvent :one",
		"AND claim_token = sqlc.arg(claim_token)",
		"-- name: RetryOutboxEvent :one",
		"AND status IN ('failed', 'parked', 'dead_lettered')",
	} {
		if !strings.Contains(queries, fragment) {
			t.Fatalf("expected outbox reliability query fragment %q", fragment)
		}
	}
	for _, unsafeProjection := range []string{
		"SELECT * FROM outbox_events",
		"RETURNING claimed.*",
		"RETURNING outbox_events.*",
	} {
		if strings.Contains(queries, unsafeProjection) {
			t.Fatalf("outbox queries must use explicit rolling-deploy-safe columns: %q", unsafeProjection)
		}
	}
}

// TestPositionLookupIndexesStayInSchema keeps the retained position directory efficient and case-insensitive.
func TestPositionLookupIndexesStayInSchema(t *testing.T) {
	raw, err := os.ReadFile("../../../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	required := []string{
		"CREATE TABLE positions (",
		"CREATE UNIQUE INDEX positions_tenant_code_ci_idx ON positions (tenant_id, lower(code));",
		"CREATE INDEX positions_tenant_name_ci_idx ON positions (tenant_id, lower(name));",
		"CREATE INDEX positions_tenant_status_idx ON positions (tenant_id, status, name);",
	}
	for _, item := range required {
		if !strings.Contains(schema, item) {
			t.Fatalf("expected position directory schema fragment %q", item)
		}
	}
}

// TestTenantResourceIDsStayGloballyUniqueContract 驗證租戶 resource IDs stay globally unique contract。
func TestTenantResourceIDsStayGloballyUniqueContract(t *testing.T) {
	schemaRaw, err := os.ReadFile("../../../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	coreRaw, err := os.ReadFile("../../../db/queries/core.sql")
	if err != nil {
		t.Fatal(err)
	}
	authzRaw, err := os.ReadFile("../../../db/queries/authz.sql")
	if err != nil {
		t.Fatal(err)
	}
	openAPIRaw, err := os.ReadFile("../../../docs/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(schemaRaw)
	core := string(coreRaw)
	authz := string(authzRaw)
	openAPI := string(openAPIRaw)

	requiredSchema := []string{
		"CREATE TABLE accounts (\n    id text PRIMARY KEY",
		"CONSTRAINT accounts_tenant_id_id_idx UNIQUE (tenant_id, id)",
		"CREATE TABLE employees (\n    id text PRIMARY KEY",
		"CONSTRAINT employees_tenant_id_id_idx UNIQUE (tenant_id, id)",
		"CREATE TABLE platform_task_items (\n    id text PRIMARY KEY",
		"CONSTRAINT platform_task_items_tenant_id_id_idx UNIQUE (tenant_id, id)",
		"CREATE INDEX accounts_keyword_trgm_idx ON accounts USING gin",
	}
	for _, item := range requiredSchema {
		if !strings.Contains(schema, item) {
			t.Fatalf("expected globally unique tenant-resource id schema fragment %q", item)
		}
	}
	requiredQueries := []string{
		"-- name: UpsertAccount :one",
		"INSERT INTO accounts (",
		"ON CONFLICT (id) DO UPDATE SET\n    tenant_id = EXCLUDED.tenant_id",
		"-- name: UpsertEmployee :one\nINSERT INTO employees",
		"-- name: UpsertAuthzPermissionSetAssignment :one\nINSERT INTO authz_permission_set_assignments",
		// 樂觀鎖:三張熱點表的 upsert 必須帶 expected_version 檢查。
		"WHERE sqlc.arg(expected_version)::bigint = 0 OR accounts.version = sqlc.arg(expected_version)::bigint",
		"WHERE sqlc.arg(expected_version)::bigint = 0 OR user_groups.version = sqlc.arg(expected_version)::bigint",
		"AND (sqlc.arg(expected_version)::bigint = 0 OR form_instances.version = sqlc.arg(expected_version)::bigint)",
	}
	for _, item := range requiredQueries {
		if !strings.Contains(core, item) && !strings.Contains(authz, item) {
			t.Fatalf("expected globally unique tenant-resource id query fragment %q", item)
		}
	}
	if !strings.Contains(openAPI, "Identifiers for persisted tenant resources are globally unique across tenants.") {
		t.Fatal("expected OpenAPI to document globally unique persisted resource identifiers")
	}
}

// TestRelationshipHardeningStaysInSchema verifies typed workflow data and tenant-safe foreign keys.
func TestRelationshipHardeningStaysInSchema(t *testing.T) {
	raw, err := os.ReadFile("../../../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	required := []string{
		"stage_definitions_json jsonb NOT NULL DEFAULT '[]'::jsonb",
		"CONSTRAINT user_identities_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id) ON DELETE CASCADE",
		"CONSTRAINT workflow_actions_stage_fk FOREIGN KEY (tenant_id, run_id, stage_instance_id) REFERENCES workflow_stage_instances (tenant_id, run_id, id) ON DELETE CASCADE",
		"CONSTRAINT form_instances_template_version_fk FOREIGN KEY (tenant_id, template_id, template_version_id)",
		"CONSTRAINT form_instance_field_values_one_value_check CHECK",
		"CONSTRAINT agent_runs_agent_fk FOREIGN KEY (tenant_id, agent_id) REFERENCES agent_definitions (tenant_id, id) ON DELETE SET NULL (agent_id)",
		"ADD CONSTRAINT agent_runs_session_fk FOREIGN KEY (tenant_id, session_id) REFERENCES agent_sessions (tenant_id, id) ON DELETE SET NULL (session_id)",
		"FOREIGN KEY (tenant_id, run_id) REFERENCES agent_runs (tenant_id, id) ON DELETE SET NULL (run_id)",
		"FOREIGN KEY (tenant_id, session_id) REFERENCES agent_sessions (tenant_id, id) ON DELETE SET NULL (session_id)",
	}
	for _, item := range required {
		if !strings.Contains(schema, item) {
			t.Fatalf("expected relationship-hardening schema fragment %q", item)
		}
	}
	if strings.Contains(schema, "owner_account_id text NOT NULL REFERENCES accounts(id)") {
		t.Fatal("form_definition_drafts must not keep the redundant global owner foreign key")
	}
	if strings.Contains(schema, "workflow_actions_account_fk") {
		t.Fatal("workflow action actors may use the system sentinel and must not require an account row")
	}
	for _, redundantIndex := range []string{
		"permission_set_items_tenant_set_idx",
		"authz_relationship_tuples_object_idx",
	} {
		if strings.Contains(schema, redundantIndex) {
			t.Fatalf("schema must not recreate redundant prefix index %q", redundantIndex)
		}
	}
}
