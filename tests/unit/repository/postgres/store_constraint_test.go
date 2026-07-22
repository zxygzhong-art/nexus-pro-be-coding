package postgres_test

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"nexus-pro-api/internal/repository/postgres"
)

// TestIsExclusionConstraint verifies overlap conflicts are recognized through wrapped driver errors.
func TestIsExclusionConstraint(t *testing.T) {
	err := fmt.Errorf("upsert leave balance: %w", &pgconn.PgError{
		Code:           "23P01",
		ConstraintName: "leave_balances_period_no_overlap",
	})

	if !postgres.IsExclusionConstraint(err, "leave_balances_period_no_overlap") {
		t.Fatal("expected exclusion constraint violation to match")
	}
	if postgres.IsExclusionConstraint(err, "another_constraint") {
		t.Fatal("unexpected match for another constraint")
	}
	if postgres.IsExclusionConstraint(&pgconn.PgError{Code: "23505", ConstraintName: "leave_balances_period_no_overlap"}, "leave_balances_period_no_overlap") {
		t.Fatal("unique violation must not match exclusion constraint")
	}
}
