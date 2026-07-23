package db_test

import (
	"os"
	"strings"
	"testing"
)

func TestAttendancePolicyVersionsAreCanonicalImmutableTable(t *testing.T) {
	raw, err := os.ReadFile("../../../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, fragment := range []string{
		"CREATE TABLE attendance_policy_versions (",
		"CREATE POLICY tenant_isolation_attendance_policy_versions ON attendance_policy_versions",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("expected attendance policy schema fragment %q", fragment)
		}
	}
	if strings.Contains(schema, "CREATE TABLE attendance_policies (") {
		t.Fatal("final schema must not retain mutable attendance_policies table")
	}
}
