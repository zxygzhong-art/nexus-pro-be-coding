-- +goose Up
-- Make immutable attendance policy versions the sole source of truth.

DROP TRIGGER IF EXISTS attendance_policies_version_trigger ON attendance_policies;
DROP FUNCTION IF EXISTS snapshot_attendance_policy_version();

ALTER TABLE attendance_policy_versions
    ADD COLUMN IF NOT EXISTS published_by_account_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS published_at timestamptz;

UPDATE attendance_policy_versions
SET
    effective_from = COALESCE(effective_from, created_at),
    published_by_account_id = updated_by_account_id,
    published_at = created_at;

INSERT INTO attendance_policy_versions (
    tenant_id,
    version,
    policy_id,
    work_time,
    leave_types,
    effective_from,
    updated_by_account_id,
    created_at,
    published_by_account_id,
    published_at
)
SELECT
    tenant_id,
    version,
    id,
    work_time,
    leave_types,
    COALESCE(effective_from, updated_at),
    updated_by_account_id,
    updated_at,
    updated_by_account_id,
    updated_at
FROM attendance_policies
ON CONFLICT (tenant_id, version) DO NOTHING;

INSERT INTO attendance_policy_versions (
    tenant_id,
    version,
    policy_id,
    work_time,
    leave_types,
    effective_from,
    updated_by_account_id,
    created_at,
    published_by_account_id,
    published_at
)
SELECT
    tenant.id,
    1,
    'current',
    '{
      "require_worksite": true,
      "clock_mode": "flexible",
      "flexible_clock_in_earliest": "00:00",
      "flexible_clock_out_latest": "23:30",
      "standard_start": "09:00",
      "standard_end": "17:00",
      "break_start": "12:00",
      "break_end": "13:00",
      "weekend": "週六、週日",
      "cycle_start": "1 日",
      "cycle_end": "本月 月底（最後一日）"
    }'::jsonb,
    '[]'::jsonb,
    tenant.created_at,
    '',
    tenant.created_at,
    '',
    tenant.created_at
FROM tenants tenant
WHERE NOT EXISTS (
    SELECT 1
    FROM attendance_policy_versions policy
    WHERE policy.tenant_id = tenant.id
      AND policy.version = 1
)
ON CONFLICT (tenant_id, version) DO NOTHING;

ALTER TABLE attendance_policy_versions
    ALTER COLUMN effective_from SET NOT NULL,
    ALTER COLUMN published_at SET NOT NULL,
    DROP COLUMN policy_id,
    DROP COLUMN leave_types,
    DROP COLUMN updated_by_account_id,
    DROP COLUMN created_at;

DROP POLICY IF EXISTS tenant_isolation_attendance_policies ON attendance_policies;
DROP TABLE attendance_policies;

-- +goose Down
CREATE TABLE attendance_policies (
    id text NOT NULL,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    work_time jsonb NOT NULL DEFAULT '{}'::jsonb,
    leave_types jsonb NOT NULL DEFAULT '[]'::jsonb,
    version integer NOT NULL DEFAULT 1,
    effective_from timestamptz,
    updated_by_account_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, id),
    CONSTRAINT attendance_policies_tenant_id_idx UNIQUE (tenant_id)
);

INSERT INTO attendance_policies (
    id, tenant_id, work_time, leave_types, version, effective_from,
    updated_by_account_id, created_at, updated_at
)
SELECT DISTINCT ON (tenant_id)
    'current', tenant_id, work_time, '[]'::jsonb, version, effective_from,
    published_by_account_id, published_at, published_at
FROM attendance_policy_versions
ORDER BY tenant_id, version DESC;

ALTER TABLE attendance_policy_versions
    ADD COLUMN policy_id text NOT NULL DEFAULT 'current',
    ADD COLUMN leave_types jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN updated_by_account_id text NOT NULL DEFAULT '',
    ADD COLUMN created_at timestamptz;

UPDATE attendance_policy_versions
SET
    updated_by_account_id = published_by_account_id,
    created_at = published_at;

ALTER TABLE attendance_policy_versions
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN effective_from DROP NOT NULL,
    DROP COLUMN published_by_account_id,
    DROP COLUMN published_at;

-- +goose StatementBegin
CREATE FUNCTION snapshot_attendance_policy_version()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO attendance_policy_versions (
        tenant_id, version, policy_id, work_time, leave_types,
        effective_from, updated_by_account_id, created_at
    ) VALUES (
        NEW.tenant_id, NEW.version, NEW.id, NEW.work_time, NEW.leave_types,
        NEW.effective_from, NEW.updated_by_account_id, NEW.updated_at
    )
    ON CONFLICT (tenant_id, version) DO NOTHING;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER attendance_policies_version_trigger
AFTER INSERT OR UPDATE ON attendance_policies
FOR EACH ROW EXECUTE FUNCTION snapshot_attendance_policy_version();

ALTER TABLE attendance_policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_policies FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_attendance_policies ON attendance_policies
USING (tenant_id = current_setting('app.tenant_id', true))
WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
