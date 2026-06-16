-- +goose Up

ALTER TABLE employees ADD COLUMN manager_employee_id text;
CREATE INDEX employees_tenant_manager_employee_idx ON employees (tenant_id, manager_employee_id) WHERE manager_employee_id IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS employees_tenant_manager_employee_idx;
ALTER TABLE employees DROP COLUMN IF EXISTS manager_employee_id;
