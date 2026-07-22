package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RuntimeRoleSecurity describes the PostgreSQL capabilities that can bypass
// or weaken the application's tenant RLS boundary.
type RuntimeRoleSecurity struct {
	Name                string
	Superuser           bool
	BypassRLS           bool
	CanCreateInPublic   bool
	OwnsBusinessTables  bool
	CanAccessGooseTable bool
}

// InspectRuntimeRole reads the effective role used by the API connection.
func InspectRuntimeRole(ctx context.Context, pool *pgxpool.Pool) (RuntimeRoleSecurity, error) {
	if pool == nil {
		return RuntimeRoleSecurity{}, fmt.Errorf("postgres pool is required")
	}

	const query = `
SELECT
    r.rolname,
    r.rolsuper,
    r.rolbypassrls,
    has_schema_privilege(r.rolname, 'public', 'CREATE'),
    EXISTS (
        SELECT 1
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public'
          AND c.relkind IN ('r', 'p')
          AND c.relname <> 'goose_db_version'
          AND c.relowner = r.oid
    ),
    EXISTS (
        SELECT 1
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public'
          AND c.relkind IN ('r', 'p')
          AND c.relname = 'goose_db_version'
          AND (
              has_table_privilege(r.rolname, c.oid, 'SELECT')
              OR has_table_privilege(r.rolname, c.oid, 'INSERT')
              OR has_table_privilege(r.rolname, c.oid, 'UPDATE')
              OR has_table_privilege(r.rolname, c.oid, 'DELETE')
              OR has_table_privilege(r.rolname, c.oid, 'TRUNCATE')
          )
    )
FROM pg_roles r
WHERE r.rolname = current_user`

	var result RuntimeRoleSecurity
	if err := pool.QueryRow(ctx, query).Scan(
		&result.Name,
		&result.Superuser,
		&result.BypassRLS,
		&result.CanCreateInPublic,
		&result.OwnsBusinessTables,
		&result.CanAccessGooseTable,
	); err != nil {
		return RuntimeRoleSecurity{}, fmt.Errorf("inspect postgres runtime role: %w", err)
	}
	return result, nil
}

// Validate rejects roles that can bypass RLS or mutate the application schema.
func (r RuntimeRoleSecurity) Validate() error {
	problems := make([]string, 0, 5)
	if strings.TrimSpace(r.Name) == "" {
		problems = append(problems, "effective role name is empty")
	}
	if r.Superuser {
		problems = append(problems, "role is a PostgreSQL superuser")
	}
	if r.BypassRLS {
		problems = append(problems, "role has BYPASSRLS")
	}
	if r.CanCreateInPublic {
		problems = append(problems, "role has CREATE on schema public")
	}
	if r.OwnsBusinessTables {
		problems = append(problems, "role owns business tables in schema public")
	}
	if r.CanAccessGooseTable {
		problems = append(problems, "role can access goose_db_version")
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("unsafe PostgreSQL runtime role %q: %s", r.Name, strings.Join(problems, "; "))
}
