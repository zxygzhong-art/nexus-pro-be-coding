package db_test

import (
	"os"
	"strings"
	"testing"
)

// TestSchemaDesignHardeningKeepsDatabaseLevelInvariants verifies the reviewed failure modes stay closed.
func TestSchemaDesignHardeningKeepsDatabaseLevelInvariants(t *testing.T) {
	raw, err := os.ReadFile("../../../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	required := []string{
		"api_key_ciphertext text NOT NULL DEFAULT ''",
		"CONSTRAINT authz_group_memberships_no_overlap EXCLUDE USING gist",
		"CREATE TRIGGER authz_group_memberships_projection_trigger",
		"CREATE TABLE attendance_policy_versions",
		"CREATE TABLE leave_balance_ledger",
		"remaining_hours numeric(12,2) NOT NULL",
		"CONSTRAINT leave_balances_employee_type_period_idx UNIQUE NULLS NOT DISTINCT",
		"CONSTRAINT leave_balances_period_no_overlap EXCLUDE USING gist",
		"CONSTRAINT attendance_shift_assignments_active_no_overlap EXCLUDE USING gist",
		"status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'succeeded', 'failed'))",
		"FOREIGN KEY (tenant_id, id, current_stage_instance_id)",
	}
	for _, fragment := range required {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("expected schema hardening fragment %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"api_key text NOT NULL DEFAULT ''",
		"CREATE POLICY system_task_positions",
		"CREATE POLICY system_task_employment_contracts",
	} {
		if strings.Contains(schema, forbidden) {
			t.Fatalf("forbidden schema fragment remains: %q", forbidden)
		}
	}
}

// TestIdentityProvisioningClaimUsesLeaseAndSkipLocked verifies workers cannot consume one event concurrently.
func TestIdentityProvisioningClaimUsesLeaseAndSkipLocked(t *testing.T) {
	raw, err := os.ReadFile("../../../db/queries/identity_provisioning.sql")
	if err != nil {
		t.Fatal(err)
	}
	query := string(raw)
	for _, fragment := range []string{"FOR UPDATE SKIP LOCKED", "status = 'processing'", "claim_expires_at", "next_attempt_at"} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("expected identity outbox claim fragment %q", fragment)
		}
	}
}
