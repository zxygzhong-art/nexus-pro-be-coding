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
		"CREATE TABLE credential_secrets (",
		"ciphertext text NOT NULL",
		"CONSTRAINT model_connections_secret_fk FOREIGN KEY (tenant_id, credential_secret_id) REFERENCES credential_secrets (tenant_id, id) ON DELETE RESTRICT",
		"CONSTRAINT authz_group_memberships_no_overlap EXCLUDE USING gist",
		"CREATE TRIGGER authz_group_memberships_projection_trigger",
		"CREATE TABLE attendance_policy_versions",
		"CREATE TABLE leave_balances (",
		"remaining_minutes integer NOT NULL",
		"CONSTRAINT leave_balances_local_anchor_zero_check CHECK",
		"CREATE INDEX leave_balances_fefo_idx",
		"CREATE UNIQUE INDEX leave_balances_local_anchor_idx",
		"CREATE TABLE leave_balance_entries (",
		"amount_minutes integer NOT NULL CHECK (amount_minutes <> 0)",
		"CREATE TABLE attendance_day_projections (",
		"CONSTRAINT leave_balances_leave_type_fk FOREIGN KEY (tenant_id, leave_type_id)",
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
		"CREATE TABLE leave_balance_ledger (",
		"remaining_hours numeric(12,2) NOT NULL",
		"granted_hours numeric(12,2) NOT NULL",
		"used_hours numeric(12,2) NOT NULL",
		"requested_hours numeric(12,2) NOT NULL",
		"policy_version integer NOT NULL DEFAULT 0,\n    prorate_ratio double precision,\n    updated_at timestamptz NOT NULL,\n    CONSTRAINT leave_balances_tenant_id_id_idx",
		"CREATE INDEX leave_balances_tenant_id_idx",
		"CREATE POLICY system_task_positions",
		"CREATE TABLE employment_contracts (",
		"CREATE TABLE tenant_leave_type_settings",
		"CREATE TABLE leave_type_definitions",
		"CREATE TABLE leave_type_external_mappings",
		"CREATE TABLE leave_type_sync_issues",
		"CREATE FUNCTION sync_leave_type_catalog_from_policy",
		"CREATE TRIGGER attendance_policies_leave_type_catalog_trigger",
		"CREATE TABLE attendance_policies (",
		"CREATE FUNCTION snapshot_attendance_policy_version",
		"policy_id text NOT NULL",
		"leave_types jsonb NOT NULL",
		"CREATE TABLE attendance_shifts",
		"CREATE TABLE attendance_shift_assignments",
		"CONSTRAINT leave_balances_period_no_overlap EXCLUDE USING gist",
		"CREATE UNIQUE INDEX leave_balances_snapshot_period_idx",
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
