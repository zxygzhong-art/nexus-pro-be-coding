\set ON_ERROR_STOP on
-- Read the secret from the environment so it is not exposed in the psql
-- command line or checked into a migration. The shell wrapper rejects empty
-- passwords before invoking this file.
\set runtime_password `printenv RUNTIME_DB_PASSWORD`

SELECT current_user <> :'runtime_role' AS admin_is_distinct \gset
\if :admin_is_distinct
\else
  \echo 'MIGRATION_DATABASE_URL must not authenticate as the runtime role'
  \quit 1
\endif

SELECT EXISTS (
  SELECT 1 FROM pg_roles WHERE rolname = :'migration_owner'
) AS migration_owner_exists \gset
\if :migration_owner_exists
\else
  \echo 'MIGRATION_DB_OWNER does not identify an existing PostgreSQL role'
  \quit 1
\endif

BEGIN;

SELECT format(
  'CREATE ROLE %I LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS',
  :'runtime_role'
)
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'runtime_role')
\gexec

SELECT format(
  'ALTER ROLE %I WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD %L',
  :'runtime_role',
  :'runtime_password'
)
\gexec

SELECT format('GRANT CONNECT ON DATABASE %I TO %I', current_database(), :'runtime_role')
\gexec

-- PostgreSQL installations upgraded from older releases may still grant
-- CREATE on public to PUBLIC. Remove that inherited path before granting the
-- runtime role schema usage only.
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
SELECT format('REVOKE ALL PRIVILEGES ON SCHEMA public FROM %I', :'runtime_role')
\gexec
SELECT format('GRANT USAGE ON SCHEMA public TO %I', :'runtime_role')
\gexec

SELECT format('REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM %I', :'runtime_role')
\gexec
SELECT format(
  'GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE %I.%I TO %I',
  n.nspname,
  c.relname,
  :'runtime_role'
)
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public'
  AND c.relkind IN ('r', 'p')
  AND c.relname <> 'goose_db_version'
\gexec

SELECT format('REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM %I', :'runtime_role')
\gexec
SELECT format('GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO %I', :'runtime_role')
\gexec

SELECT format(
  'ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %I',
  :'migration_owner',
  :'runtime_role'
)
\gexec
SELECT format(
  'ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO %I',
  :'migration_owner',
  :'runtime_role'
)
\gexec

COMMIT;

WITH runtime_role AS (
  SELECT oid, rolname, rolsuper, rolbypassrls
  FROM pg_roles
  WHERE rolname = :'runtime_role'
), role_check AS (
  SELECT
    NOT r.rolsuper
      AND NOT r.rolbypassrls
      AND NOT has_schema_privilege(r.rolname, 'public', 'CREATE')
      AND NOT EXISTS (
        SELECT 1
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public'
          AND c.relkind IN ('r', 'p')
          AND c.relname <> 'goose_db_version'
          AND c.relowner = r.oid
      )
      AND NOT EXISTS (
        SELECT 1
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public'
          AND c.relkind IN ('r', 'p')
          AND c.relname <> 'goose_db_version'
          AND NOT (
            has_table_privilege(r.rolname, c.oid, 'SELECT')
            AND has_table_privilege(r.rolname, c.oid, 'INSERT')
            AND has_table_privilege(r.rolname, c.oid, 'UPDATE')
            AND has_table_privilege(r.rolname, c.oid, 'DELETE')
          )
      )
      AND NOT EXISTS (
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
      ) AS safe
  FROM runtime_role r
)
SELECT COALESCE(bool_and(safe), false) AS runtime_role_is_safe
FROM role_check
\gset

\if :runtime_role_is_safe
  \echo 'runtime database role provisioned and verified'
\else
  \echo 'runtime database role verification failed'
  SELECT
    r.rolname,
    r.rolsuper,
    r.rolbypassrls,
    has_schema_privilege(r.rolname, 'public', 'CREATE') AS can_create_in_public,
    EXISTS (
      SELECT 1
      FROM pg_class c
      JOIN pg_namespace n ON n.oid = c.relnamespace
      WHERE n.nspname = 'public'
        AND c.relkind IN ('r', 'p')
        AND c.relname <> 'goose_db_version'
        AND c.relowner = r.oid
    ) AS owns_business_tables,
    EXISTS (
      SELECT 1
      FROM pg_class c
      JOIN pg_namespace n ON n.oid = c.relnamespace
      WHERE n.nspname = 'public'
        AND c.relkind IN ('r', 'p')
        AND c.relname <> 'goose_db_version'
        AND NOT (
          has_table_privilege(r.rolname, c.oid, 'SELECT')
          AND has_table_privilege(r.rolname, c.oid, 'INSERT')
          AND has_table_privilege(r.rolname, c.oid, 'UPDATE')
          AND has_table_privilege(r.rolname, c.oid, 'DELETE')
        )
    ) AS missing_business_table_dml,
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
    ) AS can_access_goose_table
  FROM pg_roles r
  WHERE r.rolname = :'runtime_role';
  \quit 1
\endif
