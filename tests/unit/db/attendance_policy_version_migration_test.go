package db_test

import (
	"os"
	"strings"
	"testing"
)

func TestAttendancePolicyVersionMigrationUsesOneImmutableTable(t *testing.T) {
	raw, err := os.ReadFile("../../../db/migrations/000016_merge_attendance_policy_versions.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration := string(raw)
	for _, fragment := range []string{
		"DROP TRIGGER IF EXISTS attendance_policies_version_trigger",
		"INSERT INTO attendance_policy_versions",
		"DROP TABLE attendance_policies",
	} {
		if !strings.Contains(migration, fragment) {
			t.Fatalf("expected attendance policy migration fragment %q", fragment)
		}
	}
}
