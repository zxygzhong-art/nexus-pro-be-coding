-- +goose Up
CREATE TABLE leave_type_definitions (
    code text PRIMARY KEY,
    name_zh text NOT NULL,
    name_en text NOT NULL,
    unit text NOT NULL DEFAULT 'hour' CHECK (unit IN ('hour', 'day')),
    paid_ratio numeric(5,4) NOT NULL DEFAULT 1 CHECK (paid_ratio >= 0 AND paid_ratio <= 1),
    requires_balance boolean NOT NULL DEFAULT false,
    display_order integer NOT NULL UNIQUE CHECK (display_order > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tenant_leave_type_settings (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    leave_type_code text NOT NULL REFERENCES leave_type_definitions(code) ON DELETE CASCADE,
    enabled boolean NOT NULL DEFAULT true,
    updated_by_account_id text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, leave_type_code)
);

INSERT INTO leave_type_definitions (
    code, name_zh, name_en, unit, paid_ratio, requires_balance, display_order
) VALUES
    ('sick_full', '全薪病假', 'Full Pay Sick Leave', 'hour', 1, true, 1),
    ('flexible', '彈性休假', 'Additional Leave', 'hour', 1, true, 2),
    ('personal', '事假', 'Personal Leave', 'hour', 0, false, 3),
    ('family_care', '家庭照顧假', 'Family Care Leave', 'hour', 0, false, 4),
    ('sick_half', '半薪病假', 'Half Pay Sick Leave', 'hour', 0.5, true, 5),
    ('menstrual', '生理假', 'Menstruation Leave', 'hour', 0.5, false, 6),
    ('marriage', '婚假', 'Marriage Leave', 'hour', 1, false, 7),
    ('maternity', '八週產假', '8-Week Maternity Leave', 'hour', 1, false, 8),
    ('paternity', '陪產假', 'Paternity Leave', 'hour', 1, false, 9),
    ('bereavement', '喪假', 'Bereavement Leave', 'hour', 1, false, 10),
    ('official', '公假', 'Official Leave', 'hour', 1, false, 11),
    ('prenatal', '產檢假', 'Prenatal Leave', 'hour', 1, false, 12),
    ('compensatory', '補休假', 'Compensatory Leave', 'hour', 1, true, 13),
    ('annual', '特休假', 'Annual Leave', 'hour', 1, true, 14),
    ('business_trip', '外勤', 'Business Trip', 'hour', 1, false, 15);

ALTER TABLE tenant_leave_type_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_leave_type_settings FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_tenant_leave_type_settings ON tenant_leave_type_settings
USING (tenant_id = current_setting('app.tenant_id', true))
WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- External attendance mappings still reference the legacy per-tenant identity
-- table. New mappings create those compatibility rows on demand.
ALTER TABLE leave_types DROP CONSTRAINT IF EXISTS leave_types_source_of_truth_check;
ALTER TABLE leave_types ADD CONSTRAINT leave_types_source_of_truth_check
CHECK (source_of_truth IN ('local_policy', 'system_default', 'ehrms', 'overtime', 'manual'));

-- The catalog is now system-owned. Remove the persisted EHRMS snapshot and
-- its tenant enablement projection, including all previously synchronized data.
DROP TABLE IF EXISTS ehrms_leave_type_enablements;
DROP TABLE IF EXISTS ehrms_leave_types;

-- +goose Down
UPDATE leave_types SET source_of_truth = 'local_policy' WHERE source_of_truth = 'system_default';

ALTER TABLE leave_types DROP CONSTRAINT IF EXISTS leave_types_source_of_truth_check;
ALTER TABLE leave_types ADD CONSTRAINT leave_types_source_of_truth_check
CHECK (source_of_truth IN ('local_policy', 'ehrms', 'overtime', 'manual'));

DROP POLICY IF EXISTS tenant_isolation_tenant_leave_type_settings ON tenant_leave_type_settings;
DROP TABLE IF EXISTS tenant_leave_type_settings;
DROP TABLE IF EXISTS leave_type_definitions;

CREATE TABLE ehrms_leave_types (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code text NOT NULL,
    position integer NOT NULL CHECK (position >= 0),
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    synced_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, position)
);

CREATE INDEX ehrms_leave_types_tenant_code_idx
ON ehrms_leave_types (tenant_id, code);

ALTER TABLE ehrms_leave_types ENABLE ROW LEVEL SECURITY;
ALTER TABLE ehrms_leave_types FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_ehrms_leave_types ON ehrms_leave_types
USING (tenant_id = current_setting('app.tenant_id', true))
WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

CREATE TABLE ehrms_leave_type_enablements (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code text NOT NULL,
    enabled boolean NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, code)
);

ALTER TABLE ehrms_leave_type_enablements ENABLE ROW LEVEL SECURITY;
ALTER TABLE ehrms_leave_type_enablements FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_ehrms_leave_type_enablements ON ehrms_leave_type_enablements
USING (tenant_id = current_setting('app.tenant_id', true))
WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
