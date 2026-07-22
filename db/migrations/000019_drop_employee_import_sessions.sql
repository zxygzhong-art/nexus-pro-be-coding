-- +goose Up
-- Retire the interactive employee batch-import workflow and its persisted staging data.

DROP TABLE IF EXISTS employee_import_sessions;

-- +goose Down
CREATE TABLE employee_import_sessions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    filename text NOT NULL,
    object_provider text NOT NULL DEFAULT '',
    object_bucket text NOT NULL DEFAULT '',
    object_key text NOT NULL DEFAULT '',
    content_type text NOT NULL DEFAULT '',
    size_bytes bigint NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    sha256 text NOT NULL DEFAULT '',
    status text NOT NULL,
    rows jsonb NOT NULL DEFAULT '[]'::jsonb,
    summary jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by_account_id text NOT NULL DEFAULT '',
    confirmed_by_account_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    confirmed_at timestamptz
);

CREATE INDEX employee_import_sessions_tenant_id_idx
    ON employee_import_sessions (tenant_id, created_at DESC);

ALTER TABLE employee_import_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE employee_import_sessions FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_employee_import_sessions ON employee_import_sessions
    USING (tenant_id = current_setting('app.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
