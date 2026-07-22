package postgres_test

import (
	"strings"
	"testing"

	pgplatform "nexus-pro-api/internal/platform/postgres"
)

func TestRuntimeRoleSecurityAcceptsLeastPrivilegeRole(t *testing.T) {
	role := pgplatform.RuntimeRoleSecurity{Name: "nexus_app"}

	if err := role.Validate(); err != nil {
		t.Fatalf("expected least-privilege runtime role to validate, got %v", err)
	}
}

func TestRuntimeRoleSecurityRejectsEveryRLSBypassPath(t *testing.T) {
	role := pgplatform.RuntimeRoleSecurity{
		Name:                "migration_owner",
		Superuser:           true,
		BypassRLS:           true,
		CanCreateInPublic:   true,
		OwnsBusinessTables:  true,
		CanAccessGooseTable: true,
	}

	err := role.Validate()
	if err == nil {
		t.Fatal("expected privileged runtime role to be rejected")
	}
	for _, want := range []string{"superuser", "BYPASSRLS", "CREATE on schema public", "owns business tables", "goose_db_version"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected validation error to contain %q, got %v", want, err)
		}
	}
}

func TestRuntimeRoleSecurityRejectsMissingEffectiveRole(t *testing.T) {
	err := (pgplatform.RuntimeRoleSecurity{}).Validate()
	if err == nil || !strings.Contains(err.Error(), "effective role name is empty") {
		t.Fatalf("expected empty role name to fail closed, got %v", err)
	}
}
