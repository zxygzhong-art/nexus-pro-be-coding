package db_test

import (
	"os"
	"strings"
	"testing"
)

func TestGroupMembershipMigrationExpandsUserGroupMemberProjection(t *testing.T) {
	raw, err := os.ReadFile("../../../db/migrations/000004_group_memberships.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration := string(raw)
	required := []string{
		"CREATE TABLE authz_group_memberships",
		"CROSS JOIN LATERAL unnest(g.member_account_ids) AS member(account_id)",
		"'migration'",
		"ON CONFLICT (tenant_id, user_group_id, account_id) DO NOTHING",
		"CREATE POLICY tenant_isolation_authz_group_memberships",
	}
	for _, item := range required {
		if !strings.Contains(migration, item) {
			t.Fatalf("expected group membership migration fragment %q", item)
		}
	}
}
