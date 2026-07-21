-- +goose Up
ALTER TABLE org_units
    ADD COLUMN IF NOT EXISTS manager_position_id text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS org_units_tenant_manager_position_idx
    ON org_units (tenant_id, manager_position_id)
    WHERE manager_position_id <> '';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_org_unit_manager_position()
RETURNS trigger AS $$
BEGIN
    IF NEW.manager_position_id <> '' AND NOT EXISTS (
        SELECT 1
        FROM positions
        WHERE tenant_id = NEW.tenant_id
          AND id = NEW.manager_position_id
    ) THEN
        RAISE EXCEPTION 'org unit manager_position_id % does not exist in tenant %',
            NEW.manager_position_id, NEW.tenant_id
            USING ERRCODE = 'foreign_key_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS org_units_manager_position_check ON org_units;
CREATE TRIGGER org_units_manager_position_check
BEFORE INSERT OR UPDATE OF tenant_id, manager_position_id ON org_units
FOR EACH ROW EXECUTE FUNCTION validate_org_unit_manager_position();

-- +goose Down
DROP TRIGGER IF EXISTS org_units_manager_position_check ON org_units;
DROP FUNCTION IF EXISTS validate_org_unit_manager_position();
DROP INDEX IF EXISTS org_units_tenant_manager_position_idx;
ALTER TABLE org_units DROP COLUMN IF EXISTS manager_position_id;
