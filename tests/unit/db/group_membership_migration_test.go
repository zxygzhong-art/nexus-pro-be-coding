package db_test

import (
	"os"
	"strings"
	"testing"
)

func TestGroupMembershipMigrationExpandsUserGroupMemberProjection(t *testing.T) {
	raw, err := os.ReadFile("../../../db/migrations/000001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration := string(raw)
	required := []string{
		"CREATE TABLE authz_group_memberships",
		"CONSTRAINT authz_group_memberships_no_overlap EXCLUDE USING gist",
		"CREATE TRIGGER authz_group_memberships_projection_trigger",
		"source text NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'import', 'template', 'approval', 'migration'))",
		"ALTER TABLE authz_group_memberships ENABLE ROW LEVEL SECURITY",
		"CREATE POLICY tenant_isolation_authz_group_memberships",
	}
	for _, item := range required {
		if !strings.Contains(migration, item) {
			t.Fatalf("expected group membership migration fragment %q", item)
		}
	}
}
