package db_test

import (
	"os"
	"strings"
	"testing"
)

func TestPermissionPackageMigrationDefinesTemplatesImportsAndRLS(t *testing.T) {
	raw, err := os.ReadFile("../../../db/migrations/000005_permission_packages.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration := string(raw)
	required := []string{
		"ADD COLUMN source_template_key text NOT NULL DEFAULT ''",
		"ADD COLUMN source_package_version text NOT NULL DEFAULT ''",
		"CREATE TABLE permission_packages",
		"CONSTRAINT permission_packages_application_version_idx UNIQUE (application_code, version)",
		"CREATE TABLE permission_set_templates",
		"CREATE TABLE user_group_templates",
		"CREATE TABLE assumable_role_templates",
		"CREATE TABLE permission_package_imports",
		"CONSTRAINT permission_package_imports_unique_idx UNIQUE (tenant_id, package_id, version)",
		"ALTER TABLE permission_package_imports ENABLE ROW LEVEL SECURITY",
		"CREATE POLICY tenant_isolation_permission_package_imports",
	}
	for _, item := range required {
		if !strings.Contains(migration, item) {
			t.Fatalf("expected permission package migration fragment %q", item)
		}
	}
}
