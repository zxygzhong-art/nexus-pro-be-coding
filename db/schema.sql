
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE tenants (
    id text PRIMARY KEY,
    name text NOT NULL,
    created_at timestamptz NOT NULL
);

CREATE TABLE accounts (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    display_name text NOT NULL,
    email text NOT NULL DEFAULT '',
    employee_id text NOT NULL DEFAULT '',
    status text NOT NULL,
    user_group_ids text[] NOT NULL DEFAULT '{}',
    direct_permission_set_ids text[] NOT NULL DEFAULT '{}',
    active_assumable_role_id text NOT NULL DEFAULT '',
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL,
    CONSTRAINT accounts_tenant_id_id_idx UNIQUE (tenant_id, id)
);

CREATE INDEX accounts_tenant_id_idx ON accounts (tenant_id);
CREATE UNIQUE INDEX accounts_tenant_email_idx ON accounts (tenant_id, lower(email)) WHERE email <> '';
CREATE INDEX accounts_keyword_trgm_idx ON accounts USING gin (
    lower(
        coalesce(display_name, '') || ' ' ||
        coalesce(email, '') || ' ' ||
        coalesce(employee_id, '')
    ) gin_trgm_ops
);

CREATE TABLE user_groups (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    member_account_ids text[] NOT NULL DEFAULT '{}',
    permission_set_ids text[] NOT NULL DEFAULT '{}',
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL,
    CONSTRAINT user_groups_tenant_id_id_idx UNIQUE (tenant_id, id)
);

CREATE INDEX user_groups_tenant_id_idx ON user_groups (tenant_id);

CREATE TABLE permission_sets (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    permissions jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL,
    CONSTRAINT permission_sets_tenant_id_id_idx UNIQUE (tenant_id, id)
);

CREATE INDEX permission_sets_tenant_id_idx ON permission_sets (tenant_id);

CREATE TABLE permissions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    application text NOT NULL,
    resource text NOT NULL,
    action text NOT NULL,
    permission_type text NOT NULL CHECK (permission_type IN ('menu', 'api', 'button', 'field', 'scope')),
    menu_key text NOT NULL DEFAULT '',
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    high_risk boolean NOT NULL DEFAULT false,
    severity text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    CONSTRAINT permissions_tenant_id_id_idx UNIQUE (tenant_id, id)
);

CREATE UNIQUE INDEX permissions_tenant_catalog_unique_idx ON permissions (
    tenant_id, application, resource, action, permission_type
);
CREATE INDEX permissions_tenant_id_idx ON permissions (tenant_id);
CREATE INDEX permissions_tenant_menu_key_idx ON permissions (tenant_id, menu_key) WHERE menu_key <> '';

CREATE TABLE menu_items (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    key text NOT NULL,
    label text NOT NULL,
    path text NOT NULL DEFAULT '',
    icon text NOT NULL DEFAULT '',
    parent_key text NOT NULL DEFAULT '',
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL,
    CONSTRAINT menu_items_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT menu_items_tenant_key_idx UNIQUE (tenant_id, key)
);

CREATE INDEX menu_items_tenant_parent_idx ON menu_items (tenant_id, parent_key, sort_order);

CREATE TABLE permission_set_items (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    permission_set_id text NOT NULL,
    permission_id text NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT permission_set_items_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT permission_set_items_unique_idx UNIQUE (tenant_id, permission_set_id, permission_id),
    CONSTRAINT permission_set_items_set_fk FOREIGN KEY (tenant_id, permission_set_id) REFERENCES permission_sets (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT permission_set_items_permission_fk FOREIGN KEY (tenant_id, permission_id) REFERENCES permissions (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX permission_set_items_tenant_set_idx ON permission_set_items (tenant_id, permission_set_id);
CREATE INDEX permission_set_items_tenant_permission_idx ON permission_set_items (tenant_id, permission_id);

CREATE TABLE assumable_roles (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    permission_set_ids text[] NOT NULL DEFAULT '{}',
    trusted boolean NOT NULL DEFAULT false,
    trust_policy jsonb NOT NULL DEFAULT '{}'::jsonb,
    permission_boundary jsonb NOT NULL DEFAULT '{}'::jsonb,
    session_duration_seconds integer NOT NULL DEFAULT 28800 CHECK (session_duration_seconds > 0),
    created_at timestamptz NOT NULL,
    CONSTRAINT assumable_roles_tenant_id_id_idx UNIQUE (tenant_id, id)
);

CREATE INDEX assumable_roles_tenant_id_idx ON assumable_roles (tenant_id);

CREATE TABLE user_identities (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id text NOT NULL,
    provider text NOT NULL,
    subject text NOT NULL,
    email text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL
);

CREATE INDEX user_identities_tenant_account_idx ON user_identities (tenant_id, account_id);
CREATE UNIQUE INDEX user_identities_provider_subject_idx ON user_identities (tenant_id, provider, subject);

CREATE TABLE authz_data_scopes (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code text NOT NULL,
    name text NOT NULL,
    scope_type text NOT NULL,
    params jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL
);

CREATE UNIQUE INDEX authz_data_scopes_tenant_code_idx ON authz_data_scopes (tenant_id, code);

CREATE TABLE authz_field_policies (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    application_code text NOT NULL,
    resource_type text NOT NULL,
    field_name text NOT NULL,
    effect text NOT NULL CHECK (effect IN ('allow', 'deny', 'mask', 'readonly', 'hide')),
    mask_strategy text NOT NULL DEFAULT '',
    permission_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL
);

CREATE INDEX authz_field_policies_resource_idx ON authz_field_policies (
    tenant_id, application_code, resource_type
);

CREATE TABLE authz_permission_set_assignments (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    principal_type text NOT NULL CHECK (principal_type IN ('account', 'user_group', 'assumable_role', 'service', 'agent')),
    principal_id text NOT NULL,
    permission_set_id text NOT NULL,
    effect text NOT NULL DEFAULT 'allow' CHECK (effect IN ('allow', 'deny')),
    data_scope_id text NOT NULL DEFAULT '',
    condition_id text NOT NULL DEFAULT '',
    starts_at timestamptz,
    expires_at timestamptz,
    created_at timestamptz NOT NULL,
    CONSTRAINT authz_permission_set_assignments_set_fk FOREIGN KEY (tenant_id, permission_set_id) REFERENCES permission_sets (tenant_id, id)
);

CREATE INDEX authz_permission_set_assignments_principal_idx ON authz_permission_set_assignments (
    tenant_id, principal_type, principal_id
);
CREATE INDEX authz_permission_set_assignments_set_idx ON authz_permission_set_assignments (tenant_id, permission_set_id);

CREATE OR REPLACE FUNCTION validate_authz_assignment_references()
RETURNS trigger AS $$
BEGIN
    IF NEW.data_scope_id <> '' AND NOT EXISTS (
        SELECT 1 FROM authz_data_scopes
        WHERE tenant_id = NEW.tenant_id AND id = NEW.data_scope_id
    ) THEN
        RAISE EXCEPTION 'authz data_scope_id % does not exist for tenant %', NEW.data_scope_id, NEW.tenant_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER authz_permission_set_assignments_reference_check
BEFORE INSERT OR UPDATE ON authz_permission_set_assignments
FOR EACH ROW EXECUTE FUNCTION validate_authz_assignment_references();

CREATE TABLE authz_assumable_role_sessions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id text NOT NULL,
    assumable_role_id text NOT NULL,
    session_policy jsonb NOT NULL DEFAULT '{}'::jsonb,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL,
    CONSTRAINT authz_assumable_role_sessions_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT authz_assumable_role_sessions_role_fk FOREIGN KEY (tenant_id, assumable_role_id) REFERENCES assumable_roles (tenant_id, id)
);

CREATE INDEX authz_assumable_role_sessions_account_idx ON authz_assumable_role_sessions (tenant_id, account_id, created_at DESC);

CREATE TABLE authz_relationship_tuples (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    object_type text NOT NULL,
    object_id text NOT NULL,
    relation text NOT NULL,
    subject_type text NOT NULL,
    subject_id text NOT NULL,
    created_at timestamptz NOT NULL
);

CREATE INDEX authz_relationship_tuples_object_idx ON authz_relationship_tuples (
    tenant_id, object_type, object_id, relation
);
CREATE UNIQUE INDEX authz_relationship_tuples_unique_idx ON authz_relationship_tuples (
    tenant_id, object_type, object_id, relation, subject_type, subject_id
);

CREATE TABLE authz_permission_versions (
    tenant_id text PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    version bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL
);

CREATE TABLE identity_provisioning_outbox (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id text NOT NULL,
    employee_id text NOT NULL DEFAULT '',
    employee_no text NOT NULL DEFAULT '',
    email text NOT NULL,
    display_name text NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    send_invite boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'succeeded', 'failed')),
    retry_count integer NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE INDEX identity_provisioning_outbox_tenant_status_idx ON identity_provisioning_outbox (tenant_id, status, created_at);

CREATE TABLE org_units (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code text NOT NULL DEFAULT '',
    name text NOT NULL,
    parent_id text NOT NULL DEFAULT '',
    path text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL
);

CREATE INDEX org_units_tenant_id_idx ON org_units (tenant_id);
CREATE INDEX org_units_path_idx ON org_units USING gin (path);

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

CREATE TRIGGER positions_reference_check
BEFORE INSERT OR UPDATE OF tenant_id, org_unit_id ON positions
FOR EACH ROW EXECUTE FUNCTION validate_position_references();

CREATE TABLE employees (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_no text NOT NULL DEFAULT '',
    name text NOT NULL,
    company_email text NOT NULL DEFAULT '',
    personal_email text NOT NULL DEFAULT '',
    phone text NOT NULL DEFAULT '',
    org_unit_id text NOT NULL DEFAULT '',
    account_id text NOT NULL DEFAULT '',
    manager_employee_id text,
    position_id text NOT NULL DEFAULT '',
    position text NOT NULL DEFAULT '',
    category text NOT NULL DEFAULT '',
    status text NOT NULL,
    employment_status text NOT NULL DEFAULT '',
    hire_date timestamptz,
    resign_date timestamptz,
    basic_info jsonb NOT NULL DEFAULT '{}'::jsonb,
    employment_info jsonb NOT NULL DEFAULT '{}'::jsonb,
    education_military_info jsonb NOT NULL DEFAULT '{}'::jsonb,
    contact_info jsonb NOT NULL DEFAULT '{}'::jsonb,
    insurance_info jsonb NOT NULL DEFAULT '{}'::jsonb,
    internal_experiences jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT employees_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT employees_manager_employee_fk FOREIGN KEY (tenant_id, manager_employee_id) REFERENCES employees (tenant_id, id)
);

CREATE OR REPLACE FUNCTION validate_employee_references()
RETURNS trigger AS $$
BEGIN
    IF NEW.account_id <> '' AND NOT EXISTS (
        SELECT 1 FROM accounts WHERE tenant_id = NEW.tenant_id AND id = NEW.account_id
    ) THEN
        RAISE EXCEPTION 'employee account_id % does not exist in tenant %', NEW.account_id, NEW.tenant_id
            USING ERRCODE = 'foreign_key_violation';
    END IF;
    IF NEW.org_unit_id <> '' AND NOT EXISTS (
        SELECT 1 FROM org_units WHERE tenant_id = NEW.tenant_id AND id = NEW.org_unit_id
    ) THEN
        RAISE EXCEPTION 'employee org_unit_id % does not exist in tenant %', NEW.org_unit_id, NEW.tenant_id
            USING ERRCODE = 'foreign_key_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER employees_reference_check
BEFORE INSERT OR UPDATE OF tenant_id, account_id, org_unit_id ON employees
FOR EACH ROW EXECUTE FUNCTION validate_employee_references();

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

CREATE TRIGGER employees_position_reference_check
BEFORE INSERT OR UPDATE OF tenant_id, position_id ON employees
FOR EACH ROW EXECUTE FUNCTION validate_employee_position_reference();

CREATE INDEX employees_tenant_id_idx ON employees (tenant_id);
CREATE INDEX employees_tenant_status_idx ON employees (tenant_id, employment_status, status);
CREATE INDEX employees_tenant_category_idx ON employees (tenant_id, category);
CREATE INDEX employees_tenant_org_unit_idx ON employees (tenant_id, org_unit_id);
CREATE INDEX employees_tenant_manager_employee_idx ON employees (tenant_id, manager_employee_id) WHERE manager_employee_id IS NOT NULL;
CREATE INDEX employees_tenant_position_idx ON employees (tenant_id, position_id) WHERE position_id <> '';
CREATE INDEX employees_tenant_hire_date_idx ON employees (tenant_id, hire_date);
CREATE INDEX employees_keyword_trgm_idx ON employees USING gin (
    lower(
        coalesce(employee_no, '') || ' ' ||
        coalesce(name, '') || ' ' ||
        coalesce(company_email, '') || ' ' ||
        coalesce(personal_email, '') || ' ' ||
        coalesce(phone, '')
    ) gin_trgm_ops
);
CREATE UNIQUE INDEX employees_tenant_employee_no_idx ON employees (tenant_id, employee_no) WHERE employee_no <> '';
CREATE UNIQUE INDEX employees_tenant_account_id_idx ON employees (tenant_id, account_id) WHERE account_id <> '';
CREATE UNIQUE INDEX employees_tenant_company_email_idx ON employees (tenant_id, lower(company_email)) WHERE company_email <> '';
CREATE UNIQUE INDEX employees_tenant_personal_email_idx ON employees (tenant_id, lower(personal_email)) WHERE personal_email <> '';
CREATE UNIQUE INDEX employees_tenant_national_id_idx ON employees (tenant_id, lower(basic_info->>'national_id')) WHERE coalesce(basic_info->>'national_id', '') <> '';
CREATE UNIQUE INDEX employees_tenant_passport_no_idx ON employees (tenant_id, lower(basic_info->>'passport_no')) WHERE coalesce(basic_info->>'passport_no', '') <> '';
CREATE UNIQUE INDEX employees_tenant_arc_no_idx ON employees (tenant_id, lower(basic_info->>'arc_no')) WHERE coalesce(basic_info->>'arc_no', '') <> '';
CREATE UNIQUE INDEX employees_tenant_tax_id_idx ON employees (tenant_id, lower(basic_info->>'tax_id')) WHERE coalesce(basic_info->>'tax_id', '') <> '';
CREATE UNIQUE INDEX employees_tenant_work_permit_no_idx ON employees (tenant_id, lower(basic_info->>'work_permit_no')) WHERE coalesce(basic_info->>'work_permit_no', '') <> '';

CREATE TABLE employee_number_sequences (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    prefix text NOT NULL,
    next_value integer NOT NULL DEFAULT 1 CHECK (next_value > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, prefix)
);

CREATE TABLE employee_import_sessions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    filename text NOT NULL,
    object_provider text NOT NULL DEFAULT '',
    object_bucket text NOT NULL DEFAULT '',
    object_key text NOT NULL DEFAULT '',
    content_type text NOT NULL DEFAULT '',
    size_bytes bigint NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    sha256 text NOT NULL DEFAULT '',
    status text NOT NULL,
    rows jsonb NOT NULL DEFAULT '[]'::jsonb,
    summary jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by_account_id text NOT NULL DEFAULT '',
    confirmed_by_account_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    confirmed_at timestamptz
);

CREATE INDEX employee_import_sessions_tenant_id_idx ON employee_import_sessions (tenant_id, created_at DESC);

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

CREATE TABLE attendance_policies (
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

CREATE TABLE leave_balances (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    leave_type text NOT NULL,
    remaining_hours double precision NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT leave_balances_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id)
);

CREATE INDEX leave_balances_tenant_id_idx ON leave_balances (tenant_id);
CREATE UNIQUE INDEX leave_balances_tenant_employee_type_idx ON leave_balances (tenant_id, employee_id, leave_type);

CREATE TABLE form_templates (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    key text NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    schema jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL,
    CONSTRAINT form_templates_tenant_id_id_idx UNIQUE (tenant_id, id)
);

CREATE INDEX form_templates_tenant_id_idx ON form_templates (tenant_id);
CREATE UNIQUE INDEX form_templates_tenant_key_idx ON form_templates (tenant_id, key);

CREATE TABLE form_instances (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    template_id text NOT NULL,
    applicant_account_id text NOT NULL,
    status text NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    submitted_at timestamptz NOT NULL,
    approved_by text NOT NULL DEFAULT '',
    current_run_id text NOT NULL DEFAULT '',
    version bigint NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL,
    CONSTRAINT form_instances_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT form_instances_template_fk FOREIGN KEY (tenant_id, template_id) REFERENCES form_templates (tenant_id, id),
    CONSTRAINT form_instances_applicant_account_fk FOREIGN KEY (tenant_id, applicant_account_id) REFERENCES accounts (tenant_id, id)
);

CREATE INDEX form_instances_tenant_id_idx ON form_instances (tenant_id);
CREATE INDEX form_instances_template_id_idx ON form_instances (template_id);
CREATE INDEX form_instances_tenant_applicant_status_idx ON form_instances (tenant_id, applicant_account_id, status, submitted_at DESC);

CREATE TABLE workflow_runs (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    form_instance_id text NOT NULL,
    template_id text NOT NULL,
    version integer NOT NULL,
    status text NOT NULL,
    current_stage_instance_id text NOT NULL DEFAULT '',
    stage_definitions_json text NOT NULL DEFAULT '[]',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT workflow_runs_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT workflow_runs_form_fk FOREIGN KEY (tenant_id, form_instance_id) REFERENCES form_instances (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT workflow_runs_template_fk FOREIGN KEY (tenant_id, template_id) REFERENCES form_templates (tenant_id, id)
);

CREATE INDEX workflow_runs_tenant_form_version_idx ON workflow_runs (tenant_id, form_instance_id, version);

CREATE TABLE workflow_stage_instances (
    id text PRIMARY KEY,
    tenant_id text NOT NULL,
    run_id text NOT NULL,
    stage_id text NOT NULL,
    stage_type text NOT NULL,
    label text NOT NULL,
    status text NOT NULL,
    sequence integer NOT NULL,
    result jsonb NOT NULL DEFAULT '{}'::jsonb,
    started_at timestamptz,
    completed_at timestamptz,
    CONSTRAINT workflow_stage_instances_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT workflow_stage_instances_run_fk FOREIGN KEY (tenant_id, run_id) REFERENCES workflow_runs (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX workflow_stage_instances_run_sequence_idx ON workflow_stage_instances (tenant_id, run_id, sequence);

CREATE TABLE workflow_stage_assignees (
    tenant_id text NOT NULL,
    stage_instance_id text NOT NULL,
    account_id text NOT NULL,
    status text NOT NULL,
    PRIMARY KEY (tenant_id, stage_instance_id, account_id),
    CONSTRAINT workflow_stage_assignees_stage_fk FOREIGN KEY (tenant_id, stage_instance_id) REFERENCES workflow_stage_instances (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT workflow_stage_assignees_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id)
);

CREATE INDEX workflow_stage_assignees_pending_idx ON workflow_stage_assignees (tenant_id, account_id, status);

CREATE TABLE workflow_actions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL,
    run_id text NOT NULL,
    stage_instance_id text NOT NULL,
    account_id text NOT NULL,
    action text NOT NULL,
    comment text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    CONSTRAINT workflow_actions_run_fk FOREIGN KEY (tenant_id, run_id) REFERENCES workflow_runs (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX workflow_actions_run_created_idx ON workflow_actions (tenant_id, run_id, created_at);

CREATE TABLE leave_requests (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    leave_type text NOT NULL,
    start_at timestamptz NOT NULL,
    end_at timestamptz NOT NULL,
    hours double precision NOT NULL,
    reason text NOT NULL DEFAULT '',
    status text NOT NULL,
    form_instance_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    CONSTRAINT leave_requests_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id)
);

CREATE INDEX leave_requests_tenant_id_idx ON leave_requests (tenant_id);
CREATE INDEX leave_requests_employee_id_idx ON leave_requests (employee_id);
CREATE INDEX leave_requests_tenant_form_instance_idx ON leave_requests (tenant_id, form_instance_id);
CREATE INDEX leave_requests_tenant_employee_status_dates_idx ON leave_requests (tenant_id, employee_id, status, start_at, end_at);

CREATE TABLE attendance_worksites (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    address text NOT NULL DEFAULT '',
    latitude double precision NOT NULL CHECK (latitude >= -90 AND latitude <= 90),
    longitude double precision NOT NULL CHECK (longitude >= -180 AND longitude <= 180),
    radius_meters integer NOT NULL CHECK (radius_meters > 0),
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT attendance_worksites_tenant_id_id_idx UNIQUE (tenant_id, id)
);

CREATE INDEX attendance_worksites_tenant_status_idx ON attendance_worksites (tenant_id, status);

CREATE TABLE attendance_shifts (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    clock_in_start text NOT NULL,
    clock_in_end text NOT NULL,
    clock_out_start text NOT NULL,
    clock_out_end text NOT NULL,
    late_grace_minutes integer NOT NULL DEFAULT 0 CHECK (late_grace_minutes >= 0),
    early_leave_grace_minutes integer NOT NULL DEFAULT 0 CHECK (early_leave_grace_minutes >= 0),
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT attendance_shifts_tenant_id_id_idx UNIQUE (tenant_id, id)
);

CREATE INDEX attendance_shifts_tenant_status_idx ON attendance_shifts (tenant_id, status);

CREATE TABLE attendance_shift_assignments (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    shift_id text NOT NULL,
    worksite_id text NOT NULL,
    effective_from timestamptz NOT NULL,
    effective_to timestamptz,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT attendance_shift_assignments_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT attendance_shift_assignments_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT attendance_shift_assignments_shift_fk FOREIGN KEY (tenant_id, shift_id) REFERENCES attendance_shifts (tenant_id, id),
    CONSTRAINT attendance_shift_assignments_worksite_fk FOREIGN KEY (tenant_id, worksite_id) REFERENCES attendance_worksites (tenant_id, id)
);

CREATE INDEX attendance_shift_assignments_tenant_employee_idx ON attendance_shift_assignments (tenant_id, employee_id, effective_from DESC);
CREATE INDEX attendance_shift_assignments_shift_idx ON attendance_shift_assignments (tenant_id, shift_id);
CREATE INDEX attendance_shift_assignments_worksite_idx ON attendance_shift_assignments (tenant_id, worksite_id);

CREATE TABLE attendance_clock_records (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    shift_assignment_id text NOT NULL,
    shift_id text NOT NULL,
    worksite_id text NOT NULL,
    work_date text NOT NULL,
    direction text NOT NULL,
    clocked_at timestamptz NOT NULL,
    latitude double precision NOT NULL CHECK (latitude >= -90 AND latitude <= 90),
    longitude double precision NOT NULL CHECK (longitude >= -180 AND longitude <= 180),
    accuracy_meters double precision NOT NULL DEFAULT 0 CHECK (accuracy_meters >= 0),
    distance_meters double precision NOT NULL DEFAULT 0 CHECK (distance_meters >= 0),
    record_status text NOT NULL,
    rejection_reason text NOT NULL DEFAULT '',
    source text NOT NULL,
    device_id text NOT NULL DEFAULT '',
    device_info jsonb NOT NULL DEFAULT '{}'::jsonb,
    correction_request_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    CONSTRAINT attendance_clock_records_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT attendance_clock_records_shift_assignment_fk FOREIGN KEY (tenant_id, shift_assignment_id) REFERENCES attendance_shift_assignments (tenant_id, id),
    CONSTRAINT attendance_clock_records_shift_fk FOREIGN KEY (tenant_id, shift_id) REFERENCES attendance_shifts (tenant_id, id),
    CONSTRAINT attendance_clock_records_worksite_fk FOREIGN KEY (tenant_id, worksite_id) REFERENCES attendance_worksites (tenant_id, id)
);

CREATE INDEX attendance_clock_records_tenant_employee_date_idx ON attendance_clock_records (tenant_id, employee_id, work_date DESC);
CREATE INDEX attendance_clock_records_tenant_status_idx ON attendance_clock_records (tenant_id, record_status, clocked_at DESC);
CREATE UNIQUE INDEX attendance_clock_records_one_accepted_idx ON attendance_clock_records (tenant_id, employee_id, work_date, direction) WHERE record_status = 'accepted';

CREATE TABLE attendance_correction_requests (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    direction text NOT NULL,
    requested_clocked_at timestamptz NOT NULL,
    work_date text NOT NULL,
    reason text NOT NULL DEFAULT '',
    status text NOT NULL,
    form_instance_id text NOT NULL DEFAULT '',
    clock_record_id text NOT NULL DEFAULT '',
    reviewed_by_account_id text NOT NULL DEFAULT '',
    review_reason text NOT NULL DEFAULT '',
    reviewed_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT attendance_correction_requests_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id)
);

CREATE INDEX attendance_correction_requests_tenant_employee_date_idx ON attendance_correction_requests (tenant_id, employee_id, work_date DESC);
CREATE INDEX attendance_correction_requests_tenant_status_idx ON attendance_correction_requests (tenant_id, status, created_at DESC);

CREATE TABLE overtime_requests (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    work_date text NOT NULL,
    start_at timestamptz NOT NULL,
    end_at timestamptz NOT NULL,
    hours double precision NOT NULL CHECK (hours > 0),
    overtime_type text NOT NULL DEFAULT 'weekday',
    compensation_type text NOT NULL DEFAULT 'leave',
    reason text NOT NULL DEFAULT '',
    status text NOT NULL,
    form_instance_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT overtime_requests_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id)
);

CREATE INDEX overtime_requests_tenant_id_idx ON overtime_requests (tenant_id);
CREATE INDEX overtime_requests_tenant_employee_date_idx ON overtime_requests (tenant_id, employee_id, work_date DESC);
CREATE INDEX overtime_requests_tenant_form_instance_idx ON overtime_requests (tenant_id, form_instance_id);
CREATE INDEX overtime_requests_tenant_status_dates_idx ON overtime_requests (tenant_id, status, start_at, end_at);

CREATE TABLE platform_task_items (
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

CREATE INDEX platform_task_items_tenant_account_date_idx ON platform_task_items (tenant_id, account_id, work_date DESC, created_at ASC);

CREATE TABLE platform_task_todos (
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

CREATE INDEX platform_task_todos_tenant_account_status_idx ON platform_task_todos (tenant_id, account_id, status, created_at ASC);

CREATE TABLE agent_runs (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id text NOT NULL,
    mode text NOT NULL,
    prompt text NOT NULL,
    answer text NOT NULL DEFAULT '',
    status text NOT NULL,
    reference_items jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT agent_runs_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id)
);

CREATE INDEX agent_runs_tenant_id_idx ON agent_runs (tenant_id);
CREATE INDEX agent_runs_account_id_idx ON agent_runs (account_id);
CREATE INDEX agent_runs_tenant_account_created_at_idx ON agent_runs (tenant_id, account_id, created_at DESC);

CREATE TABLE notifications (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    tone text NOT NULL CHECK (tone IN ('success', 'info', 'warning')),
    category text NOT NULL DEFAULT 'system',
    title text NOT NULL,
    body text NOT NULL,
    status_text text NOT NULL,
    link_url text NOT NULL DEFAULT '',
    source_type text NOT NULL DEFAULT '',
    source_id text NOT NULL DEFAULT '',
    created_by_account_id text,
    created_at timestamptz NOT NULL,
    expires_at timestamptz,
    CONSTRAINT notifications_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT notifications_created_by_fk FOREIGN KEY (tenant_id, created_by_account_id) REFERENCES accounts (tenant_id, id)
);

CREATE INDEX notifications_tenant_created_at_idx ON notifications (tenant_id, created_at DESC, id DESC);
CREATE UNIQUE INDEX notifications_source_unique_idx ON notifications (tenant_id, source_type, source_id) WHERE source_type <> '' AND source_id <> '';

CREATE TABLE notification_recipients (
    notification_id text NOT NULL,
    tenant_id text NOT NULL,
    account_id text NOT NULL,
    read_at timestamptz,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (notification_id, account_id),
    CONSTRAINT notification_recipients_notification_fk FOREIGN KEY (tenant_id, notification_id) REFERENCES notifications (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT notification_recipients_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX notification_recipients_account_idx ON notification_recipients (tenant_id, account_id, read_at, created_at DESC);

CREATE TABLE outbox_events (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    event_type text NOT NULL,
    aggregate_type text NOT NULL DEFAULT '',
    aggregate_id text NOT NULL DEFAULT '',
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'pending',
    retry_count integer NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    processed_at timestamptz
);

CREATE INDEX outbox_events_tenant_status_idx ON outbox_events (tenant_id, status, created_at);

CREATE TABLE audit_logs (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    actor_account_id text NOT NULL,
    action text NOT NULL,
    resource text NOT NULL,
    target text NOT NULL DEFAULT '',
    result text NOT NULL DEFAULT '',
    trace_id text NOT NULL DEFAULT '',
    severity text NOT NULL,
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL
);

CREATE INDEX audit_logs_tenant_id_created_at_idx ON audit_logs (tenant_id, created_at DESC);
CREATE INDEX audit_logs_actor_account_id_idx ON audit_logs (actor_account_id);

ALTER TABLE accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE accounts FORCE ROW LEVEL SECURITY;
ALTER TABLE user_groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_groups FORCE ROW LEVEL SECURITY;
ALTER TABLE permission_sets ENABLE ROW LEVEL SECURITY;
ALTER TABLE permission_sets FORCE ROW LEVEL SECURITY;
ALTER TABLE permissions ENABLE ROW LEVEL SECURITY;
ALTER TABLE permissions FORCE ROW LEVEL SECURITY;
ALTER TABLE menu_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE menu_items FORCE ROW LEVEL SECURITY;
ALTER TABLE permission_set_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE permission_set_items FORCE ROW LEVEL SECURITY;
ALTER TABLE assumable_roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE assumable_roles FORCE ROW LEVEL SECURITY;
ALTER TABLE user_identities ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_identities FORCE ROW LEVEL SECURITY;
ALTER TABLE authz_data_scopes ENABLE ROW LEVEL SECURITY;
ALTER TABLE authz_data_scopes FORCE ROW LEVEL SECURITY;
ALTER TABLE authz_field_policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE authz_field_policies FORCE ROW LEVEL SECURITY;
ALTER TABLE authz_permission_set_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE authz_permission_set_assignments FORCE ROW LEVEL SECURITY;
ALTER TABLE authz_assumable_role_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE authz_assumable_role_sessions FORCE ROW LEVEL SECURITY;
ALTER TABLE authz_relationship_tuples ENABLE ROW LEVEL SECURITY;
ALTER TABLE authz_relationship_tuples FORCE ROW LEVEL SECURITY;
ALTER TABLE authz_permission_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE authz_permission_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE identity_provisioning_outbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE identity_provisioning_outbox FORCE ROW LEVEL SECURITY;
ALTER TABLE org_units ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_units FORCE ROW LEVEL SECURITY;
ALTER TABLE positions ENABLE ROW LEVEL SECURITY;
ALTER TABLE positions FORCE ROW LEVEL SECURITY;
ALTER TABLE employees ENABLE ROW LEVEL SECURITY;
ALTER TABLE employees FORCE ROW LEVEL SECURITY;
ALTER TABLE employee_number_sequences ENABLE ROW LEVEL SECURITY;
ALTER TABLE employee_number_sequences FORCE ROW LEVEL SECURITY;
ALTER TABLE employee_import_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE employee_import_sessions FORCE ROW LEVEL SECURITY;
ALTER TABLE employment_contracts ENABLE ROW LEVEL SECURITY;
ALTER TABLE employment_contracts FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_policies FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_balances ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_balances FORCE ROW LEVEL SECURITY;
ALTER TABLE form_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE form_templates FORCE ROW LEVEL SECURITY;
ALTER TABLE form_instances ENABLE ROW LEVEL SECURITY;
ALTER TABLE form_instances FORCE ROW LEVEL SECURITY;
ALTER TABLE workflow_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_runs FORCE ROW LEVEL SECURITY;
ALTER TABLE workflow_stage_instances ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_stage_instances FORCE ROW LEVEL SECURITY;
ALTER TABLE workflow_stage_assignees ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_stage_assignees FORCE ROW LEVEL SECURITY;
ALTER TABLE workflow_actions ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_actions FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_requests FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_worksites ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_worksites FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_shifts ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_shifts FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_shift_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_shift_assignments FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_clock_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_clock_records FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_correction_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_correction_requests FORCE ROW LEVEL SECURITY;
ALTER TABLE overtime_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE overtime_requests FORCE ROW LEVEL SECURITY;
ALTER TABLE platform_task_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform_task_items FORCE ROW LEVEL SECURITY;
ALTER TABLE platform_task_todos ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform_task_todos FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_runs FORCE ROW LEVEL SECURITY;
ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE notifications FORCE ROW LEVEL SECURITY;
ALTER TABLE notification_recipients ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_recipients FORCE ROW LEVEL SECURITY;
ALTER TABLE outbox_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox_events FORCE ROW LEVEL SECURITY;
ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_logs FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_accounts ON accounts USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_user_groups ON user_groups USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_permission_sets ON permission_sets USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_permissions ON permissions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_menu_items ON menu_items USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_permission_set_items ON permission_set_items USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_assumable_roles ON assumable_roles USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_user_identities ON user_identities USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_authz_data_scopes ON authz_data_scopes USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_authz_field_policies ON authz_field_policies USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_authz_permission_set_assignments ON authz_permission_set_assignments USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_authz_assumable_role_sessions ON authz_assumable_role_sessions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_authz_relationship_tuples ON authz_relationship_tuples USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_authz_permission_versions ON authz_permission_versions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_identity_provisioning_outbox ON identity_provisioning_outbox USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_org_units ON org_units USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_positions ON positions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY system_task_positions ON positions USING (current_setting('app.system_task', true) = 'on') WITH CHECK (current_setting('app.system_task', true) = 'on');
CREATE POLICY tenant_isolation_employees ON employees USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_employee_number_sequences ON employee_number_sequences USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_employee_import_sessions ON employee_import_sessions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_employment_contracts ON employment_contracts USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY system_task_employment_contracts ON employment_contracts USING (current_setting('app.system_task', true) = 'on') WITH CHECK (current_setting('app.system_task', true) = 'on');
CREATE POLICY tenant_isolation_attendance_policies ON attendance_policies USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_balances ON leave_balances USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_form_templates ON form_templates USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_form_instances ON form_instances USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_workflow_runs ON workflow_runs USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_workflow_stage_instances ON workflow_stage_instances USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_workflow_stage_assignees ON workflow_stage_assignees USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_workflow_actions ON workflow_actions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_requests ON leave_requests USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_worksites ON attendance_worksites USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_shifts ON attendance_shifts USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_shift_assignments ON attendance_shift_assignments USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_clock_records ON attendance_clock_records USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_correction_requests ON attendance_correction_requests USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_overtime_requests ON overtime_requests USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_platform_task_items ON platform_task_items USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_platform_task_todos ON platform_task_todos USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_runs ON agent_runs USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_notifications ON notifications USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_notification_recipients ON notification_recipients USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_outbox_events ON outbox_events USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_audit_logs ON audit_logs USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
