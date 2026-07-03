package db_test

import (
	"os"
	"strings"
	"testing"
)

func TestAIAgentArchitectureTablesStayInSchema(t *testing.T) {
	raw, err := os.ReadFile("../../../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	requiredTables := []string{
		"companies",
		"users",
		"roles",
		"workspaces",
		"workspace_users",
		"agents",
		"knowledges",
		"agent_knowledges",
		"knowledge_user_permissions",
		"company_storage_configs",
		"files",
		"file_process_tasks",
		"agent_platform_files",
		"pricing_plans",
		"company_plans",
		"licenses",
	}
	for _, table := range requiredTables {
		if !strings.Contains(schema, "CREATE TABLE "+table+" (") {
			t.Fatalf("expected AI Agent architecture table %q in db/schema.sql", table)
		}
	}
	requiredEdges := []string{
		"company_id integer NOT NULL REFERENCES companies(id) ON DELETE CASCADE",
		"workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE",
		"agent_id uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE",
		"knowledge_id uuid NOT NULL REFERENCES knowledges(id) ON DELETE CASCADE",
		"file_id uuid NOT NULL REFERENCES files(id) ON DELETE CASCADE",
		"pricing_plan_id uuid NOT NULL REFERENCES pricing_plans(id)",
		"CREATE POLICY company_isolation_agents",
		"CREATE POLICY company_isolation_files",
	}
	for _, edge := range requiredEdges {
		if !strings.Contains(schema, edge) {
			t.Fatalf("expected AI Agent architecture schema edge/policy %q", edge)
		}
	}
}

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
		"-- name: UpsertAccount :one\nINSERT INTO accounts",
		"ON CONFLICT (id) DO UPDATE SET\n    tenant_id = EXCLUDED.tenant_id",
		"-- name: UpsertEmployee :one\nINSERT INTO employees",
		"-- name: UpsertAuthzPermissionSetAssignment :one\nINSERT INTO authz_permission_set_assignments",
		"ON CONFLICT (id) DO UPDATE SET\n    tenant_id = EXCLUDED.tenant_id",
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
