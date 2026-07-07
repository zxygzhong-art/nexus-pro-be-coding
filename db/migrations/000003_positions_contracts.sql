-- +goose Up

CREATE TABLE positions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code text NOT NULL,
    name text NOT NULL,
    org_unit_id text NOT NULL DEFAULT '',
    level text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    description text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT positions_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT positions_tenant_code_idx UNIQUE (tenant_id, code)
);

CREATE INDEX positions_tenant_status_idx ON positions (tenant_id, status, name);
CREATE INDEX positions_tenant_org_unit_idx ON positions (tenant_id, org_unit_id) WHERE org_unit_id <> '';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_position_references()
RETURNS trigger AS $$
BEGIN
    IF NEW.org_unit_id <> '' AND NOT EXISTS (
        SELECT 1 FROM org_units WHERE tenant_id = NEW.tenant_id AND id = NEW.org_unit_id
    ) THEN
        RAISE EXCEPTION 'position org_unit_id % does not exist in tenant %', NEW.org_unit_id, NEW.tenant_id
            USING ERRCODE = 'foreign_key_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER positions_reference_check
BEFORE INSERT OR UPDATE OF tenant_id, org_unit_id ON positions
FOR EACH ROW EXECUTE FUNCTION validate_position_references();

ALTER TABLE employees ADD COLUMN position_id text NOT NULL DEFAULT '';

WITH distinct_positions AS (
    SELECT
        tenant_id,
        btrim(position) AS name,
        min(created_at) AS created_at,
        max(updated_at) AS updated_at
    FROM employees
    WHERE btrim(position) <> ''
    GROUP BY tenant_id, btrim(position)
),
slugged AS (
    SELECT
        tenant_id,
        name,
        coalesce(nullif(btrim(regexp_replace(lower(name), '[^a-z0-9]+', '-', 'g'), '-'), ''), 'position') AS base_code,
        created_at,
        updated_at
    FROM distinct_positions
),
numbered AS (
    SELECT
        tenant_id,
        name,
        base_code,
        row_number() OVER (PARTITION BY tenant_id, base_code ORDER BY name) AS code_seq,
        created_at,
        updated_at
    FROM slugged
)
INSERT INTO positions (
    id, tenant_id, code, name, org_unit_id, level, status, description, created_at, updated_at
)
SELECT
    'pos_' || substr(md5(tenant_id || ':' || name), 1, 24),
    tenant_id,
    base_code || CASE WHEN code_seq = 1 THEN '' ELSE '-' || code_seq::text END,
    name,
    '',
    '',
    'active',
    '',
    created_at,
    updated_at
FROM numbered;

UPDATE employees e
SET position_id = p.id
FROM positions p
WHERE p.tenant_id = e.tenant_id
  AND p.name = btrim(e.position)
  AND btrim(e.position) <> '';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_employee_position_reference()
RETURNS trigger AS $$
BEGIN
    IF NEW.position_id <> '' AND NOT EXISTS (
        SELECT 1 FROM positions WHERE tenant_id = NEW.tenant_id AND id = NEW.position_id
    ) THEN
        RAISE EXCEPTION 'employee position_id % does not exist in tenant %', NEW.position_id, NEW.tenant_id
            USING ERRCODE = 'foreign_key_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER employees_position_reference_check
BEFORE INSERT OR UPDATE OF tenant_id, position_id ON employees
FOR EACH ROW EXECUTE FUNCTION validate_employee_position_reference();

CREATE INDEX employees_tenant_position_idx ON employees (tenant_id, position_id) WHERE position_id <> '';

CREATE TABLE employment_contracts (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    contract_type text NOT NULL CHECK (contract_type IN ('fulltime', 'parttime', 'contractor', 'intern')),
    contract_no text NOT NULL DEFAULT '',
    start_date timestamptz NOT NULL,
    end_date timestamptz,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'expired', 'terminated', 'renewed')),
    attachment_object_key text NOT NULL DEFAULT '',
    notes text NOT NULL DEFAULT '',
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT employment_contracts_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT employment_contracts_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id)
);

CREATE INDEX employment_contracts_tenant_employee_idx ON employment_contracts (tenant_id, employee_id, start_date DESC);
CREATE INDEX employment_contracts_tenant_status_idx ON employment_contracts (tenant_id, status, end_date);

ALTER TABLE positions ENABLE ROW LEVEL SECURITY;
ALTER TABLE positions FORCE ROW LEVEL SECURITY;
ALTER TABLE employment_contracts ENABLE ROW LEVEL SECURITY;
ALTER TABLE employment_contracts FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_positions ON positions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_employment_contracts ON employment_contracts USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY system_task_positions ON positions USING (current_setting('app.system_task', true) = 'on') WITH CHECK (current_setting('app.system_task', true) = 'on');
CREATE POLICY system_task_employment_contracts ON employment_contracts USING (current_setting('app.system_task', true) = 'on') WITH CHECK (current_setting('app.system_task', true) = 'on');

-- +goose Down

DROP POLICY IF EXISTS system_task_employment_contracts ON employment_contracts;
DROP POLICY IF EXISTS system_task_positions ON positions;
DROP POLICY IF EXISTS tenant_isolation_employment_contracts ON employment_contracts;
DROP POLICY IF EXISTS tenant_isolation_positions ON positions;

DROP TABLE IF EXISTS employment_contracts;

DROP INDEX IF EXISTS employees_tenant_position_idx;
DROP TRIGGER IF EXISTS employees_position_reference_check ON employees;
DROP FUNCTION IF EXISTS validate_employee_position_reference();
ALTER TABLE employees DROP COLUMN IF EXISTS position_id;

DROP TRIGGER IF EXISTS positions_reference_check ON positions;
DROP FUNCTION IF EXISTS validate_position_references();
DROP TABLE IF EXISTS positions;
