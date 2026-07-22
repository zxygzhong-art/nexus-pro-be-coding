-- +goose Up
-- Converge tenant leave-type enablement onto leave_types.status and drop the
-- sparse override table. Policy sync must not overwrite HR-owned status.

UPDATE leave_types leave_type
SET
    status = CASE WHEN setting.enabled THEN 'active' ELSE 'inactive' END,
    updated_at = GREATEST(leave_type.updated_at, setting.updated_at)
FROM tenant_leave_type_settings setting
WHERE leave_type.tenant_id = setting.tenant_id
  AND leave_type.code = setting.leave_type_code;

INSERT INTO leave_types (
    id, tenant_id, code, name, category, source_of_truth, status, created_at, updated_at
)
SELECT
    'lt_' || setting.leave_type_code,
    setting.tenant_id,
    setting.leave_type_code,
    coalesce(definition.name_zh, setting.leave_type_code),
    'company',
    'system_default',
    CASE WHEN setting.enabled THEN 'active' ELSE 'inactive' END,
    setting.updated_at,
    setting.updated_at
FROM tenant_leave_type_settings setting
LEFT JOIN leave_type_definitions definition
  ON definition.code = setting.leave_type_code
WHERE NOT EXISTS (
    SELECT 1
    FROM leave_types leave_type
    WHERE leave_type.tenant_id = setting.tenant_id
      AND leave_type.code = setting.leave_type_code
)
ON CONFLICT (tenant_id, code) DO UPDATE SET
    status = EXCLUDED.status,
    updated_at = EXCLUDED.updated_at;

DROP POLICY IF EXISTS tenant_isolation_tenant_leave_type_settings ON tenant_leave_type_settings;
DROP TABLE IF EXISTS tenant_leave_type_settings;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION sync_leave_type_catalog_from_policy()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    item jsonb;
    normalized_code text;
    leave_type_id text;
BEGIN
    FOR item IN SELECT value FROM jsonb_array_elements(NEW.leave_types)
    LOOP
        normalized_code := lower(trim(item->>'code'));
        IF normalized_code = '' THEN
            CONTINUE;
        END IF;
        leave_type_id := coalesce(nullif(trim(item->>'id'), ''), 'lt_' || normalized_code);
        INSERT INTO leave_types (
            id, tenant_id, code, name, category, source_of_truth, status, created_at, updated_at
        ) VALUES (
            leave_type_id,
            NEW.tenant_id,
            normalized_code,
            coalesce(nullif(trim(item->>'name'), ''), normalized_code),
            'company',
            CASE WHEN normalized_code = 'compensatory' THEN 'overtime' ELSE 'local_policy' END,
            CASE WHEN lower(coalesce(nullif(trim(item->>'active'), ''), 'true')) IN ('false', '0', 'no', 'off') THEN 'inactive' ELSE 'active' END,
            NEW.updated_at,
            NEW.updated_at
        )
        ON CONFLICT (tenant_id, id) DO UPDATE SET
            code = EXCLUDED.code,
            name = EXCLUDED.name,
            updated_at = EXCLUDED.updated_at;
        -- status is owned by the HR leave-type catalog toggle; do not overwrite it.
    END LOOP;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION sync_leave_type_catalog_from_policy()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    item jsonb;
    normalized_code text;
    leave_type_id text;
BEGIN
    FOR item IN SELECT value FROM jsonb_array_elements(NEW.leave_types)
    LOOP
        normalized_code := lower(trim(item->>'code'));
        IF normalized_code = '' THEN
            CONTINUE;
        END IF;
        leave_type_id := coalesce(nullif(trim(item->>'id'), ''), 'lt_' || normalized_code);
        INSERT INTO leave_types (
            id, tenant_id, code, name, category, source_of_truth, status, created_at, updated_at
        ) VALUES (
            leave_type_id,
            NEW.tenant_id,
            normalized_code,
            coalesce(nullif(trim(item->>'name'), ''), normalized_code),
            'company',
            CASE WHEN normalized_code = 'compensatory' THEN 'overtime' ELSE 'local_policy' END,
            CASE WHEN lower(coalesce(nullif(trim(item->>'active'), ''), 'true')) IN ('false', '0', 'no', 'off') THEN 'inactive' ELSE 'active' END,
            NEW.updated_at,
            NEW.updated_at
        )
        ON CONFLICT (tenant_id, id) DO UPDATE SET
            code = EXCLUDED.code,
            name = EXCLUDED.name,
            status = EXCLUDED.status,
            updated_at = EXCLUDED.updated_at;
    END LOOP;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TABLE tenant_leave_type_settings (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    leave_type_code text NOT NULL REFERENCES leave_type_definitions(code) ON DELETE CASCADE,
    enabled boolean NOT NULL DEFAULT true,
    updated_by_account_id text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, leave_type_code)
);

INSERT INTO tenant_leave_type_settings (
    tenant_id, leave_type_code, enabled, updated_by_account_id, updated_at
)
SELECT
    leave_type.tenant_id,
    leave_type.code,
    leave_type.status = 'active',
    '',
    leave_type.updated_at
FROM leave_types leave_type
JOIN leave_type_definitions definition
  ON definition.code = leave_type.code
ON CONFLICT (tenant_id, leave_type_code) DO UPDATE SET
    enabled = EXCLUDED.enabled,
    updated_at = EXCLUDED.updated_at;

ALTER TABLE tenant_leave_type_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_leave_type_settings FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_tenant_leave_type_settings ON tenant_leave_type_settings
USING (tenant_id = current_setting('app.tenant_id', true))
WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
