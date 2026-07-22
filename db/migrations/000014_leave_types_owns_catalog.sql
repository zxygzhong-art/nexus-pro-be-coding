-- +goose Up
-- Converge leave catalog onto leave_types only: expand columns from
-- leave_type_definitions, then drop definitions / external mappings / sync issues
-- and stop auto-sync from attendance policy JSON.

ALTER TABLE leave_types
    ADD COLUMN IF NOT EXISTS name_zh text,
    ADD COLUMN IF NOT EXISTS name_en text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS requires_balance boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS display_order integer NOT NULL DEFAULT 0;

UPDATE leave_types leave_type
SET
    name_zh = coalesce(nullif(trim(definition.name_zh), ''), leave_type.name, leave_type.code),
    name_en = coalesce(definition.name_en, ''),
    requires_balance = coalesce(definition.requires_balance, false),
    display_order = coalesce(definition.display_order, 0),
    name = coalesce(nullif(trim(definition.name_zh), ''), leave_type.name, leave_type.code)
FROM leave_type_definitions definition
WHERE definition.code = leave_type.code;

UPDATE leave_types
SET name_zh = coalesce(nullif(trim(name_zh), ''), nullif(trim(name), ''), code)
WHERE name_zh IS NULL OR trim(name_zh) = '';

ALTER TABLE leave_types
    ALTER COLUMN name_zh SET NOT NULL;

DROP TRIGGER IF EXISTS attendance_policies_leave_type_catalog_trigger ON attendance_policies;
DROP FUNCTION IF EXISTS sync_leave_type_catalog_from_policy();

DROP POLICY IF EXISTS tenant_isolation_leave_type_external_mappings ON leave_type_external_mappings;
DROP POLICY IF EXISTS tenant_isolation_leave_type_sync_issues ON leave_type_sync_issues;
DROP TABLE IF EXISTS leave_type_external_mappings;
DROP TABLE IF EXISTS leave_type_sync_issues;
DROP TABLE IF EXISTS leave_type_definitions;

-- +goose Down
CREATE TABLE leave_type_definitions (
    code text PRIMARY KEY,
    name_zh text NOT NULL,
    name_en text NOT NULL,
    requires_balance boolean NOT NULL DEFAULT false,
    display_order integer NOT NULL UNIQUE CHECK (display_order > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO leave_type_definitions (
    code, name_zh, name_en, requires_balance, display_order
)
SELECT
    code,
    name_zh,
    name_en,
    requires_balance,
    row_number() OVER (ORDER BY code)::integer
FROM (
    SELECT DISTINCT ON (code)
        code,
        name_zh,
        name_en,
        requires_balance
    FROM leave_types
    ORDER BY code, display_order ASC
) catalog
ON CONFLICT (code) DO NOTHING;

CREATE TABLE leave_type_external_mappings (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    source text NOT NULL,
    external_code text NOT NULL,
    leave_type_id text NOT NULL,
    effective_from date,
    effective_to date,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT leave_type_external_mappings_type_fk FOREIGN KEY (tenant_id, leave_type_id) REFERENCES leave_types (tenant_id, id),
    CONSTRAINT leave_type_external_mappings_period_check CHECK (effective_from IS NULL OR effective_to IS NULL OR effective_to >= effective_from)
);

CREATE UNIQUE INDEX leave_type_external_mappings_active_idx
ON leave_type_external_mappings (tenant_id, source, lower(external_code), effective_from) NULLS NOT DISTINCT;

CREATE TABLE leave_type_sync_issues (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    source text NOT NULL,
    external_code text NOT NULL,
    issue_code text NOT NULL,
    message text NOT NULL DEFAULT '',
    occurrences integer NOT NULL DEFAULT 1 CHECK (occurrences > 0),
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
    first_seen_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    resolved_at timestamptz,
    CONSTRAINT leave_type_sync_issues_source_code_idx UNIQUE (tenant_id, source, external_code, issue_code)
);

CREATE INDEX leave_type_sync_issues_open_idx
ON leave_type_sync_issues (tenant_id, status, last_seen_at DESC);

ALTER TABLE leave_type_external_mappings ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_type_external_mappings FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_type_sync_issues ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_type_sync_issues FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_leave_type_external_mappings ON leave_type_external_mappings
USING (tenant_id = current_setting('app.tenant_id', true))
WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

CREATE POLICY tenant_isolation_leave_type_sync_issues ON leave_type_sync_issues
USING (tenant_id = current_setting('app.tenant_id', true))
WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

ALTER TABLE leave_types
    DROP COLUMN IF EXISTS display_order,
    DROP COLUMN IF EXISTS requires_balance,
    DROP COLUMN IF EXISTS name_en,
    DROP COLUMN IF EXISTS name_zh;

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
    END LOOP;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER attendance_policies_leave_type_catalog_trigger
AFTER INSERT OR UPDATE OF leave_types ON attendance_policies
FOR EACH ROW EXECUTE FUNCTION sync_leave_type_catalog_from_policy();
