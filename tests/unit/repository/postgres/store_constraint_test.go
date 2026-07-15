package postgres_test

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"nexus-pro-be/internal/repository/postgres"
)

// TestIsExclusionConstraint verifies overlap conflicts are recognized through wrapped driver errors.
func TestIsExclusionConstraint(t *testing.T) {
	err := fmt.Errorf("upsert attendance assignment: %w", &pgconn.PgError{
		Code:           "23P01",
		ConstraintName: "attendance_shift_assignments_active_no_overlap",
	})

	if !postgres.IsExclusionConstraint(err, "attendance_shift_assignments_active_no_overlap") {
		t.Fatal("expected exclusion constraint violation to match")
	}
	if postgres.IsExclusionConstraint(err, "another_constraint") {
		t.Fatal("unexpected match for another constraint")
	}
	if postgres.IsExclusionConstraint(&pgconn.PgError{Code: "23505", ConstraintName: "attendance_shift_assignments_active_no_overlap"}, "attendance_shift_assignments_active_no_overlap") {
		t.Fatal("unique violation must not match exclusion constraint")
	}
}
