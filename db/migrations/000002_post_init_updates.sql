-- +goose Up
-- 歷史版本是不可變審計快照，其 model_id 僅作歷史記錄，不應以參照完整性鎖死模型刪除。
ALTER TABLE agent_definition_versions DROP CONSTRAINT agent_definition_versions_model_fk;

DROP TRIGGER IF EXISTS org_units_manager_position_check ON org_units;
DROP TRIGGER IF EXISTS positions_reference_check ON positions;
DROP FUNCTION IF EXISTS validate_org_unit_manager_position();
DROP FUNCTION IF EXISTS validate_position_references();
DROP INDEX IF EXISTS org_units_tenant_manager_position_idx;
DROP INDEX IF EXISTS positions_tenant_org_unit_idx;
ALTER TABLE org_units DROP COLUMN IF EXISTS manager_position_id;
ALTER TABLE positions DROP COLUMN IF EXISTS org_unit_id;

ALTER TABLE org_units
ADD COLUMN IF NOT EXISTS show_in_org_chart boolean NOT NULL DEFAULT true;

ALTER TABLE positions DROP CONSTRAINT IF EXISTS positions_tenant_code_idx;

CREATE UNIQUE INDEX IF NOT EXISTS positions_tenant_code_ci_idx
    ON positions (tenant_id, lower(code));

CREATE INDEX IF NOT EXISTS positions_tenant_name_ci_idx
    ON positions (tenant_id, lower(name));

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

-- +goose Down

DROP TABLE IF EXISTS ehrms_leave_types;

DROP INDEX IF EXISTS positions_tenant_name_ci_idx;
DROP INDEX IF EXISTS positions_tenant_code_ci_idx;

ALTER TABLE positions
    ADD CONSTRAINT positions_tenant_code_idx UNIQUE (tenant_id, code);

ALTER TABLE org_units
DROP COLUMN IF EXISTS show_in_org_chart;

ALTER TABLE positions ADD COLUMN org_unit_id text NOT NULL DEFAULT '';
ALTER TABLE org_units ADD COLUMN manager_position_id text NOT NULL DEFAULT '';

CREATE INDEX positions_tenant_org_unit_idx
    ON positions (tenant_id, org_unit_id)
    WHERE org_unit_id <> '';

CREATE INDEX org_units_tenant_manager_position_idx
    ON org_units (tenant_id, manager_position_id)
    WHERE manager_position_id <> '';

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

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_org_unit_manager_position()
RETURNS trigger AS $$
BEGIN
    IF NEW.manager_position_id <> '' AND NOT EXISTS (
        SELECT 1
        FROM positions
        WHERE tenant_id = NEW.tenant_id
          AND id = NEW.manager_position_id
          AND (org_unit_id = '' OR org_unit_id = NEW.id)
    ) THEN
        RAISE EXCEPTION 'org unit manager_position_id % does not exist in tenant % for org unit %',
            NEW.manager_position_id, NEW.tenant_id, NEW.id
            USING ERRCODE = 'foreign_key_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER org_units_manager_position_check
BEFORE INSERT OR UPDATE OF tenant_id, id, manager_position_id ON org_units
FOR EACH ROW EXECUTE FUNCTION validate_org_unit_manager_position();

-- 回填前僅當所有歷史版本引用的模型仍存在時才能重建約束。
ALTER TABLE agent_definition_versions ADD CONSTRAINT agent_definition_versions_model_fk FOREIGN KEY (tenant_id, model_id) REFERENCES agent_models (tenant_id, id);
