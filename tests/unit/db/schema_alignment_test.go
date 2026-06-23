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
	}
	for _, item := range required {
		if !strings.Contains(schema, item) {
			t.Fatalf("expected employee integrity schema fragment %q", item)
		}
	}
}
