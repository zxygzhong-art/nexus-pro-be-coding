-- +goose Up

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE UNIQUE INDEX IF NOT EXISTS accounts_tenant_id_id_idx ON accounts (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS employees_tenant_id_id_idx ON employees (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS user_groups_tenant_id_id_idx ON user_groups (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS permission_sets_tenant_id_id_idx ON permission_sets (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS authz_permissions_tenant_id_id_idx ON authz_permissions (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS assumable_roles_tenant_id_id_idx ON assumable_roles (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS form_templates_tenant_id_id_idx ON form_templates (tenant_id, id);

CREATE INDEX IF NOT EXISTS employees_keyword_trgm_idx ON employees USING gin (
    lower(
        coalesce(employee_no, '') || ' ' ||
        coalesce(name, '') || ' ' ||
        coalesce(company_email, '') || ' ' ||
        coalesce(personal_email, '') || ' ' ||
        coalesce(phone, '')
    ) gin_trgm_ops
);

ALTER TABLE employees
    ADD CONSTRAINT employees_manager_employee_fk
    FOREIGN KEY (tenant_id, manager_employee_id)
    REFERENCES employees (tenant_id, id)
    NOT VALID;

ALTER TABLE leave_balances
    ADD CONSTRAINT leave_balances_employee_fk
    FOREIGN KEY (tenant_id, employee_id)
    REFERENCES employees (tenant_id, id)
    NOT VALID;

ALTER TABLE leave_requests
    ADD CONSTRAINT leave_requests_employee_fk
    FOREIGN KEY (tenant_id, employee_id)
    REFERENCES employees (tenant_id, id)
    NOT VALID;

ALTER TABLE form_instances
    ADD CONSTRAINT form_instances_template_fk
    FOREIGN KEY (tenant_id, template_id)
    REFERENCES form_templates (tenant_id, id)
    NOT VALID;

ALTER TABLE form_instances
    ADD CONSTRAINT form_instances_applicant_account_fk
    FOREIGN KEY (tenant_id, applicant_account_id)
    REFERENCES accounts (tenant_id, id)
    NOT VALID;

ALTER TABLE agent_runs
    ADD CONSTRAINT agent_runs_account_fk
    FOREIGN KEY (tenant_id, account_id)
    REFERENCES accounts (tenant_id, id)
    NOT VALID;

ALTER TABLE authz_permission_set_permissions
    ADD CONSTRAINT authz_permission_set_permissions_set_fk
    FOREIGN KEY (tenant_id, permission_set_id)
    REFERENCES permission_sets (tenant_id, id)
    NOT VALID;

ALTER TABLE authz_permission_set_permissions
    ADD CONSTRAINT authz_permission_set_permissions_permission_fk
    FOREIGN KEY (tenant_id, permission_id)
    REFERENCES authz_permissions (tenant_id, id)
    NOT VALID;

ALTER TABLE authz_group_memberships
    ADD CONSTRAINT authz_group_memberships_group_fk
    FOREIGN KEY (tenant_id, group_id)
    REFERENCES user_groups (tenant_id, id)
    NOT VALID;

ALTER TABLE authz_group_memberships
    ADD CONSTRAINT authz_group_memberships_account_fk
    FOREIGN KEY (tenant_id, account_id)
    REFERENCES accounts (tenant_id, id)
    NOT VALID;

ALTER TABLE authz_permission_set_assignments
    ADD CONSTRAINT authz_permission_set_assignments_set_fk
    FOREIGN KEY (tenant_id, permission_set_id)
    REFERENCES permission_sets (tenant_id, id)
    NOT VALID;

ALTER TABLE authz_assumable_role_sessions
    ADD CONSTRAINT authz_assumable_role_sessions_account_fk
    FOREIGN KEY (tenant_id, account_id)
    REFERENCES accounts (tenant_id, id)
    NOT VALID;

ALTER TABLE authz_assumable_role_sessions
    ADD CONSTRAINT authz_assumable_role_sessions_role_fk
    FOREIGN KEY (tenant_id, assumable_role_id)
    REFERENCES assumable_roles (tenant_id, id)
    NOT VALID;

-- +goose Down

ALTER TABLE authz_assumable_role_sessions DROP CONSTRAINT IF EXISTS authz_assumable_role_sessions_role_fk;
ALTER TABLE authz_assumable_role_sessions DROP CONSTRAINT IF EXISTS authz_assumable_role_sessions_account_fk;
ALTER TABLE authz_permission_set_assignments DROP CONSTRAINT IF EXISTS authz_permission_set_assignments_set_fk;
ALTER TABLE authz_group_memberships DROP CONSTRAINT IF EXISTS authz_group_memberships_account_fk;
ALTER TABLE authz_group_memberships DROP CONSTRAINT IF EXISTS authz_group_memberships_group_fk;
ALTER TABLE authz_permission_set_permissions DROP CONSTRAINT IF EXISTS authz_permission_set_permissions_permission_fk;
ALTER TABLE authz_permission_set_permissions DROP CONSTRAINT IF EXISTS authz_permission_set_permissions_set_fk;
ALTER TABLE agent_runs DROP CONSTRAINT IF EXISTS agent_runs_account_fk;
ALTER TABLE form_instances DROP CONSTRAINT IF EXISTS form_instances_applicant_account_fk;
ALTER TABLE form_instances DROP CONSTRAINT IF EXISTS form_instances_template_fk;
ALTER TABLE leave_requests DROP CONSTRAINT IF EXISTS leave_requests_employee_fk;
ALTER TABLE leave_balances DROP CONSTRAINT IF EXISTS leave_balances_employee_fk;
ALTER TABLE employees DROP CONSTRAINT IF EXISTS employees_manager_employee_fk;

DROP INDEX IF EXISTS employees_keyword_trgm_idx;
DROP INDEX IF EXISTS form_templates_tenant_id_id_idx;
DROP INDEX IF EXISTS assumable_roles_tenant_id_id_idx;
DROP INDEX IF EXISTS authz_permissions_tenant_id_id_idx;
DROP INDEX IF EXISTS permission_sets_tenant_id_id_idx;
DROP INDEX IF EXISTS user_groups_tenant_id_id_idx;
DROP INDEX IF EXISTS employees_tenant_id_id_idx;
DROP INDEX IF EXISTS accounts_tenant_id_id_idx;
