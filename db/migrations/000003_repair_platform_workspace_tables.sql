-- +goose Up

CREATE TABLE IF NOT EXISTS attendance_policies (
    id text NOT NULL,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    work_time jsonb NOT NULL DEFAULT '{}'::jsonb,
    leave_types jsonb NOT NULL DEFAULT '[]'::jsonb,
    updated_by_account_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, id),
    CONSTRAINT attendance_policies_tenant_id_idx UNIQUE (tenant_id)
);

CREATE TABLE IF NOT EXISTS platform_task_items (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id text NOT NULL,
    work_date text NOT NULL,
    title text NOT NULL,
    category text NOT NULL DEFAULT '',
    product text NOT NULL DEFAULT '',
    hours double precision NOT NULL CHECK (hours > 0),
    note text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT platform_task_items_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT platform_task_items_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id)
);

CREATE INDEX IF NOT EXISTS platform_task_items_tenant_account_date_idx ON platform_task_items (tenant_id, account_id, work_date DESC, created_at ASC);

CREATE TABLE IF NOT EXISTS platform_task_todos (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id text NOT NULL,
    text text NOT NULL,
    due_date text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'done')),
    converted_task_item_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT platform_task_todos_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT platform_task_todos_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id)
);

CREATE INDEX IF NOT EXISTS platform_task_todos_tenant_account_status_idx ON platform_task_todos (tenant_id, account_id, status, created_at ASC);

-- +goose StatementBegin
DO $$
DECLARE
    rel text;
BEGIN
    FOREACH rel IN ARRAY ARRAY[
        'attendance_policies',
        'attendance_worksites',
        'attendance_shifts',
        'attendance_shift_assignments',
        'attendance_clock_records',
        'attendance_correction_requests',
        'platform_task_items',
        'platform_task_todos'
    ]
    LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', rel);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', rel);
        IF NOT EXISTS (
            SELECT 1
            FROM pg_policies
            WHERE schemaname = 'public'
              AND tablename = rel
              AND policyname = 'tenant_isolation_' || rel
        ) THEN
            EXECUTE format(
                'CREATE POLICY %I ON %I USING (tenant_id = current_setting(''app.tenant_id'', true)) WITH CHECK (tenant_id = current_setting(''app.tenant_id'', true))',
                'tenant_isolation_' || rel,
                rel
            );
        END IF;
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down

-- Intentionally no-op. This migration repairs older local/demo databases without
-- dropping tenant data or disabling row-level security.
