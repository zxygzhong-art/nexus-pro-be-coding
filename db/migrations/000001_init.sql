-- +goose Up

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
    created_at timestamptz NOT NULL
);

CREATE INDEX accounts_tenant_id_idx ON accounts (tenant_id);
CREATE UNIQUE INDEX accounts_tenant_email_idx ON accounts (tenant_id, email) WHERE email <> '';

CREATE TABLE user_groups (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    member_account_ids text[] NOT NULL DEFAULT '{}',
    permission_set_ids text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL
);

CREATE INDEX user_groups_tenant_id_idx ON user_groups (tenant_id);

CREATE TABLE permission_sets (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    permissions jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL
);

CREATE INDEX permission_sets_tenant_id_idx ON permission_sets (tenant_id);

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
    created_at timestamptz NOT NULL
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

CREATE TABLE authz_applications (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code text NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL
);

CREATE UNIQUE INDEX authz_applications_tenant_code_idx ON authz_applications (tenant_id, code);

CREATE TABLE authz_permissions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    application_code text NOT NULL,
    resource_type text NOT NULL,
    action text NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    risk_level text NOT NULL DEFAULT 'normal' CHECK (risk_level IN ('normal', 'high', 'critical')),
    created_at timestamptz NOT NULL
);

CREATE UNIQUE INDEX authz_permissions_tenant_key_idx ON authz_permissions (
    tenant_id, application_code, resource_type, action
);

CREATE TABLE authz_permission_set_permissions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    permission_set_id text NOT NULL,
    permission_id text NOT NULL,
    effect text NOT NULL DEFAULT 'allow' CHECK (effect IN ('allow', 'deny')),
    created_at timestamptz NOT NULL
);

CREATE INDEX authz_permission_set_permissions_set_idx ON authz_permission_set_permissions (tenant_id, permission_set_id);
CREATE UNIQUE INDEX authz_permission_set_permissions_unique_idx ON authz_permission_set_permissions (
    tenant_id, permission_set_id, permission_id
);

CREATE TABLE authz_group_memberships (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    group_id text NOT NULL,
    account_id text NOT NULL,
    source text NOT NULL DEFAULT 'manual',
    expires_at timestamptz,
    created_at timestamptz NOT NULL
);

CREATE INDEX authz_group_memberships_account_idx ON authz_group_memberships (tenant_id, account_id);
CREATE UNIQUE INDEX authz_group_memberships_unique_idx ON authz_group_memberships (tenant_id, group_id, account_id);

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

CREATE TABLE authz_policy_conditions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    expression jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL
);

CREATE INDEX authz_policy_conditions_tenant_id_idx ON authz_policy_conditions (tenant_id);

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
    created_at timestamptz NOT NULL
);

CREATE INDEX authz_permission_set_assignments_principal_idx ON authz_permission_set_assignments (
    tenant_id, principal_type, principal_id
);
CREATE INDEX authz_permission_set_assignments_set_idx ON authz_permission_set_assignments (tenant_id, permission_set_id);

CREATE TABLE authz_assumable_role_sessions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id text NOT NULL,
    assumable_role_id text NOT NULL,
    session_policy jsonb NOT NULL DEFAULT '{}'::jsonb,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL
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

CREATE TABLE authz_outbox_events (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    event_type text NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'succeeded', 'failed')),
    retry_count integer NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    processed_at timestamptz
);

CREATE INDEX authz_outbox_events_tenant_status_idx ON authz_outbox_events (tenant_id, status, created_at);

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
    updated_at timestamptz NOT NULL
);

CREATE INDEX employees_tenant_id_idx ON employees (tenant_id);
CREATE INDEX employees_tenant_status_idx ON employees (tenant_id, employment_status, status);
CREATE INDEX employees_tenant_category_idx ON employees (tenant_id, category);
CREATE INDEX employees_tenant_org_unit_idx ON employees (tenant_id, org_unit_id);
CREATE INDEX employees_tenant_hire_date_idx ON employees (tenant_id, hire_date);
CREATE UNIQUE INDEX employees_tenant_employee_no_idx ON employees (tenant_id, employee_no) WHERE employee_no <> '';
CREATE UNIQUE INDEX employees_tenant_account_id_idx ON employees (tenant_id, account_id) WHERE account_id <> '';
CREATE UNIQUE INDEX employees_tenant_company_email_idx ON employees (tenant_id, company_email) WHERE company_email <> '';

CREATE TABLE employee_import_sessions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    filename text NOT NULL,
    object_key text NOT NULL DEFAULT '',
    status text NOT NULL,
    rows jsonb NOT NULL DEFAULT '[]'::jsonb,
    summary jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    confirmed_at timestamptz
);

CREATE INDEX employee_import_sessions_tenant_id_idx ON employee_import_sessions (tenant_id, created_at DESC);

CREATE TABLE leave_balances (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    leave_type text NOT NULL,
    remaining_hours double precision NOT NULL,
    updated_at timestamptz NOT NULL
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
    created_at timestamptz NOT NULL
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
    updated_at timestamptz NOT NULL
);

CREATE INDEX form_instances_tenant_id_idx ON form_instances (tenant_id);
CREATE INDEX form_instances_template_id_idx ON form_instances (template_id);

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
    created_at timestamptz NOT NULL
);

CREATE INDEX leave_requests_tenant_id_idx ON leave_requests (tenant_id);
CREATE INDEX leave_requests_employee_id_idx ON leave_requests (employee_id);

CREATE TABLE knowledge_articles (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    title text NOT NULL,
    content text NOT NULL,
    tags text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL
);

CREATE INDEX knowledge_articles_tenant_id_idx ON knowledge_articles (tenant_id);

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
    updated_at timestamptz NOT NULL
);

CREATE INDEX agent_runs_tenant_id_idx ON agent_runs (tenant_id);
CREATE INDEX agent_runs_account_id_idx ON agent_runs (account_id);

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
ALTER TABLE user_groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE permission_sets ENABLE ROW LEVEL SECURITY;
ALTER TABLE assumable_roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_identities ENABLE ROW LEVEL SECURITY;
ALTER TABLE authz_applications ENABLE ROW LEVEL SECURITY;
ALTER TABLE authz_permissions ENABLE ROW LEVEL SECURITY;
ALTER TABLE authz_permission_set_permissions ENABLE ROW LEVEL SECURITY;
ALTER TABLE authz_group_memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE authz_data_scopes ENABLE ROW LEVEL SECURITY;
ALTER TABLE authz_policy_conditions ENABLE ROW LEVEL SECURITY;
ALTER TABLE authz_field_policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE authz_permission_set_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE authz_assumable_role_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE authz_relationship_tuples ENABLE ROW LEVEL SECURITY;
ALTER TABLE authz_permission_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE authz_outbox_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_units ENABLE ROW LEVEL SECURITY;
ALTER TABLE employees ENABLE ROW LEVEL SECURITY;
ALTER TABLE employee_import_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_balances ENABLE ROW LEVEL SECURITY;
ALTER TABLE form_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE form_instances ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_articles ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_accounts ON accounts USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_user_groups ON user_groups USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_permission_sets ON permission_sets USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_assumable_roles ON assumable_roles USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_user_identities ON user_identities USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_authz_applications ON authz_applications USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_authz_permissions ON authz_permissions USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_authz_permission_set_permissions ON authz_permission_set_permissions USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_authz_group_memberships ON authz_group_memberships USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_authz_data_scopes ON authz_data_scopes USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_authz_policy_conditions ON authz_policy_conditions USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_authz_field_policies ON authz_field_policies USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_authz_permission_set_assignments ON authz_permission_set_assignments USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_authz_assumable_role_sessions ON authz_assumable_role_sessions USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_authz_relationship_tuples ON authz_relationship_tuples USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_authz_permission_versions ON authz_permission_versions USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_authz_outbox_events ON authz_outbox_events USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_org_units ON org_units USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_employees ON employees USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_employee_import_sessions ON employee_import_sessions USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_balances ON leave_balances USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_form_templates ON form_templates USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_form_instances ON form_instances USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_requests ON leave_requests USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_knowledge_articles ON knowledge_articles USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_runs ON agent_runs USING (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_audit_logs ON audit_logs USING (tenant_id = current_setting('app.tenant_id', true));

-- +goose Down

DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS agent_runs;
DROP TABLE IF EXISTS knowledge_articles;
DROP TABLE IF EXISTS leave_requests;
DROP TABLE IF EXISTS form_instances;
DROP TABLE IF EXISTS form_templates;
DROP TABLE IF EXISTS leave_balances;
DROP TABLE IF EXISTS employees;
DROP TABLE IF EXISTS employee_import_sessions;
DROP TABLE IF EXISTS org_units;
DROP TABLE IF EXISTS authz_relationship_tuples;
DROP TABLE IF EXISTS authz_assumable_role_sessions;
DROP TABLE IF EXISTS authz_permission_set_assignments;
DROP TABLE IF EXISTS authz_field_policies;
DROP TABLE IF EXISTS authz_policy_conditions;
DROP TABLE IF EXISTS authz_data_scopes;
DROP TABLE IF EXISTS authz_group_memberships;
DROP TABLE IF EXISTS authz_permission_set_permissions;
DROP TABLE IF EXISTS authz_permissions;
DROP TABLE IF EXISTS authz_applications;
DROP TABLE IF EXISTS user_identities;
DROP TABLE IF EXISTS assumable_roles;
DROP TABLE IF EXISTS permission_sets;
DROP TABLE IF EXISTS user_groups;
DROP TABLE IF EXISTS accounts;
DROP TABLE IF EXISTS tenants;
