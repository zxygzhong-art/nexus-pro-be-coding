-- Extensions and the application DB role used for RLS-enforced access.
-- Migrations run as a superuser/owner (MIGRATE_DSN); the app connects as app_user
-- (DB_DSN), which is NOT a superuser and has no BYPASSRLS, so RLS is enforced.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Application role. Password matches .env.example for local development.
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_user') THEN
    CREATE ROLE app_user LOGIN PASSWORD 'app_pass' NOSUPERUSER NOBYPASSRLS;
  END IF;
END
$$;

GRANT USAGE ON SCHEMA public TO app_user;

-- Tenant isolation relies on the custom GUC app.current_tenant, set per request
-- via `SET LOCAL app.current_tenant = '<tenant_id>'`. Namespaced GUCs need no
-- pre-declaration; current_setting('app.current_tenant', true) yields NULL when unset.
