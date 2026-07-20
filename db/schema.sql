
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS btree_gist;

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
    preferred_locale text NOT NULL DEFAULT 'zh-TW' CHECK (preferred_locale IN ('zh-TW', 'en-US')),
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
    source_template_key text NOT NULL DEFAULT '',
    source_package_version text NOT NULL DEFAULT '',
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL,
    CONSTRAINT user_groups_tenant_id_id_idx UNIQUE (tenant_id, id)
);

CREATE INDEX user_groups_tenant_id_idx ON user_groups (tenant_id);
CREATE INDEX user_groups_source_template_idx ON user_groups (tenant_id, source_template_key) WHERE source_template_key <> '';

CREATE TABLE authz_group_memberships (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_group_id text NOT NULL,
    account_id text NOT NULL,
    valid_from timestamptz NOT NULL,
    valid_until timestamptz,
    source text NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'import', 'template', 'approval', 'migration')),
    approval_instance_id text NOT NULL DEFAULT '',
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    CONSTRAINT authz_group_memberships_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT authz_group_memberships_interval_check CHECK (valid_until IS NULL OR valid_until >= valid_from),
    CONSTRAINT authz_group_memberships_no_overlap EXCLUDE USING gist (
        tenant_id WITH =,
        user_group_id WITH =,
        account_id WITH =,
        tstzrange(valid_from, valid_until, '[)') WITH &&
    ),
    CONSTRAINT authz_group_memberships_group_fk FOREIGN KEY (tenant_id, user_group_id) REFERENCES user_groups (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT authz_group_memberships_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX authz_group_memberships_group_idx ON authz_group_memberships (tenant_id, user_group_id, created_at);
CREATE INDEX authz_group_memberships_account_active_idx ON authz_group_memberships (tenant_id, account_id, valid_from, valid_until);

CREATE FUNCTION refresh_group_membership_projection(p_tenant_id text, p_user_group_id text, p_account_id text)
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
    UPDATE user_groups
    SET member_account_ids = COALESCE((
            SELECT array_agg(account_id ORDER BY account_id)
            FROM authz_group_memberships
            WHERE tenant_id = p_tenant_id
              AND user_group_id = p_user_group_id
              AND valid_from <= CURRENT_TIMESTAMP
              AND (valid_until IS NULL OR valid_until > CURRENT_TIMESTAMP)
        ), '{}'::text[]),
        version = version + 1
    WHERE tenant_id = p_tenant_id AND id = p_user_group_id;

    UPDATE accounts
    SET user_group_ids = COALESCE((
            SELECT array_agg(user_group_id ORDER BY user_group_id)
            FROM authz_group_memberships
            WHERE tenant_id = p_tenant_id
              AND account_id = p_account_id
              AND valid_from <= CURRENT_TIMESTAMP
              AND (valid_until IS NULL OR valid_until > CURRENT_TIMESTAMP)
        ), '{}'::text[]),
        version = version + 1
    WHERE tenant_id = p_tenant_id AND id = p_account_id;
END;
$$;

CREATE FUNCTION sync_group_membership_projections()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
	IF TG_OP = 'DELETE' OR (TG_OP = 'UPDATE' AND (OLD.tenant_id, OLD.user_group_id, OLD.account_id) IS DISTINCT FROM (NEW.tenant_id, NEW.user_group_id, NEW.account_id)) THEN
        PERFORM refresh_group_membership_projection(OLD.tenant_id, OLD.user_group_id, OLD.account_id);
    END IF;
    IF TG_OP IN ('INSERT', 'UPDATE') THEN
        PERFORM refresh_group_membership_projection(NEW.tenant_id, NEW.user_group_id, NEW.account_id);
    END IF;
	IF TG_OP = 'DELETE' THEN
		RETURN OLD;
	END IF;
	RETURN NEW;
END;
$$;

CREATE TRIGGER authz_group_memberships_projection_trigger
AFTER INSERT OR UPDATE OR DELETE ON authz_group_memberships
FOR EACH ROW EXECUTE FUNCTION sync_group_membership_projections();

CREATE TABLE permission_sets (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    permissions jsonb NOT NULL DEFAULT '[]'::jsonb,
    source_template_key text NOT NULL DEFAULT '',
    source_package_version text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    CONSTRAINT permission_sets_tenant_id_id_idx UNIQUE (tenant_id, id)
);

CREATE INDEX permission_sets_tenant_id_idx ON permission_sets (tenant_id);
CREATE INDEX permission_sets_source_template_idx ON permission_sets (tenant_id, source_template_key) WHERE source_template_key <> '';

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

COMMENT ON COLUMN permission_sets.permissions IS 'Authoring source for the permission set contract; service transactions rebuild permission_set_items after each write.';
COMMENT ON TABLE permission_set_items IS 'Normalized query projection derived from permission_sets.permissions; do not write independently from application flows.';

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
    source_template_key text NOT NULL DEFAULT '',
    source_package_version text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    CONSTRAINT assumable_roles_tenant_id_id_idx UNIQUE (tenant_id, id)
);

CREATE INDEX assumable_roles_tenant_id_idx ON assumable_roles (tenant_id);
CREATE INDEX assumable_roles_source_template_idx ON assumable_roles (tenant_id, source_template_key) WHERE source_template_key <> '';

-- Permission package registry and templates are platform-global immutable snapshots.
-- Tenant isolation is enforced on permission_package_imports and instantiated tenant artifacts.
CREATE TABLE permission_packages (
    id text PRIMARY KEY,
    application_code text NOT NULL,
    version text NOT NULL,
    status text NOT NULL CHECK (status IN ('draft', 'published', 'deprecated')),
    content jsonb NOT NULL,
    checksum text NOT NULL,
    created_at timestamptz NOT NULL,
    published_at timestamptz,
    CONSTRAINT permission_packages_application_version_idx UNIQUE (application_code, version)
);

CREATE INDEX permission_packages_application_idx ON permission_packages (application_code, status, version);

CREATE TABLE permission_set_templates (
    id text PRIMARY KEY,
    package_id text NOT NULL REFERENCES permission_packages(id) ON DELETE CASCADE,
    template_key text NOT NULL,
    name text NOT NULL,
    content jsonb NOT NULL DEFAULT '{}'::jsonb,
    version text NOT NULL,
    CONSTRAINT permission_set_templates_package_key_idx UNIQUE (package_id, template_key)
);

CREATE TABLE user_group_templates (
    id text PRIMARY KEY,
    package_id text NOT NULL REFERENCES permission_packages(id) ON DELETE CASCADE,
    template_key text NOT NULL,
    name text NOT NULL,
    content jsonb NOT NULL DEFAULT '{}'::jsonb,
    version text NOT NULL,
    CONSTRAINT user_group_templates_package_key_idx UNIQUE (package_id, template_key)
);

CREATE TABLE assumable_role_templates (
    id text PRIMARY KEY,
    package_id text NOT NULL REFERENCES permission_packages(id) ON DELETE CASCADE,
    template_key text NOT NULL,
    name text NOT NULL,
    content jsonb NOT NULL DEFAULT '{}'::jsonb,
    version text NOT NULL,
    CONSTRAINT assumable_role_templates_package_key_idx UNIQUE (package_id, template_key)
);

CREATE TABLE permission_package_imports (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    package_id text NOT NULL REFERENCES permission_packages(id) ON DELETE RESTRICT,
    version text NOT NULL,
    imported_at timestamptz NOT NULL,
    imported_by text NOT NULL DEFAULT '',
    artifact_id_map jsonb NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT permission_package_imports_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT permission_package_imports_unique_idx UNIQUE (tenant_id, package_id, version)
);

CREATE INDEX permission_package_imports_tenant_idx ON permission_package_imports (tenant_id, imported_at DESC);

CREATE TABLE user_identities (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id text NOT NULL,
    provider text NOT NULL,
    subject text NOT NULL,
    email text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    CONSTRAINT user_identities_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id) ON DELETE CASCADE
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
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'succeeded', 'failed')),
    retry_count integer NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
    last_error text NOT NULL DEFAULT '',
    next_attempt_at timestamptz NOT NULL,
    claim_expires_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE INDEX identity_provisioning_outbox_tenant_status_idx ON identity_provisioning_outbox (tenant_id, status, next_attempt_at, created_at);

CREATE TABLE org_units (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code text NOT NULL DEFAULT '',
    name text NOT NULL,
    name_en text NOT NULL DEFAULT '',
    parent_id text NOT NULL DEFAULT '',
    path text[] NOT NULL DEFAULT '{}',
    manager_position_id text NOT NULL DEFAULT '',
    source text NOT NULL DEFAULT '',
    closed boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE INDEX org_units_tenant_id_idx ON org_units (tenant_id);
CREATE INDEX org_units_path_idx ON org_units USING gin (path);
CREATE INDEX org_units_tenant_manager_position_idx
    ON org_units (tenant_id, manager_position_id)
    WHERE manager_position_id <> '';

CREATE TABLE positions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code text NOT NULL,
    name text NOT NULL,
    name_en text NOT NULL DEFAULT '',
    org_unit_id text NOT NULL DEFAULT '',
    level text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    description text NOT NULL DEFAULT '',
    source text NOT NULL DEFAULT '',
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

CREATE TRIGGER org_units_manager_position_check
BEFORE INSERT OR UPDATE OF tenant_id, id, manager_position_id ON org_units
FOR EACH ROW EXECUTE FUNCTION validate_org_unit_manager_position();

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
    show_in_org_chart boolean NOT NULL DEFAULT true,
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
    version integer NOT NULL DEFAULT 1,
    effective_from timestamptz,
    updated_by_account_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, id),
    CONSTRAINT attendance_policies_tenant_id_idx UNIQUE (tenant_id)
);

CREATE TABLE attendance_policy_versions (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    version integer NOT NULL CHECK (version > 0),
    policy_id text NOT NULL,
    work_time jsonb NOT NULL,
    leave_types jsonb NOT NULL,
    effective_from timestamptz,
    updated_by_account_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, version)
);

CREATE FUNCTION snapshot_attendance_policy_version()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO attendance_policy_versions (
        tenant_id, version, policy_id, work_time, leave_types,
        effective_from, updated_by_account_id, created_at
    ) VALUES (
        NEW.tenant_id, NEW.version, NEW.id, NEW.work_time, NEW.leave_types,
        NEW.effective_from, NEW.updated_by_account_id, NEW.updated_at
    )
    ON CONFLICT (tenant_id, version) DO NOTHING;
    RETURN NEW;
END;
$$;

CREATE TRIGGER attendance_policies_version_trigger
AFTER INSERT OR UPDATE ON attendance_policies
FOR EACH ROW EXECUTE FUNCTION snapshot_attendance_policy_version();

CREATE TABLE leave_types (
    id text NOT NULL,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code text NOT NULL,
    name text NOT NULL,
    category text NOT NULL DEFAULT 'company' CHECK (category IN ('statutory', 'company')),
    source_of_truth text NOT NULL DEFAULT 'local_policy' CHECK (source_of_truth IN ('local_policy', 'ehrms', 'overtime', 'manual')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, id),
    CONSTRAINT leave_types_tenant_code_idx UNIQUE (tenant_id, code)
);

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

CREATE FUNCTION sync_leave_type_catalog_from_policy()
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

CREATE TRIGGER attendance_policies_leave_type_catalog_trigger
AFTER INSERT OR UPDATE OF leave_types ON attendance_policies
FOR EACH ROW EXECUTE FUNCTION sync_leave_type_catalog_from_policy();

CREATE TABLE leave_balances (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    leave_type text NOT NULL,
    leave_type_id text NOT NULL DEFAULT '',
    remaining_hours numeric(12,2) NOT NULL,
    period_start date,
    period_end date,
    granted_hours numeric(12,2) NOT NULL DEFAULT 0,
    used_hours numeric(12,2) NOT NULL DEFAULT 0,
    source text NOT NULL DEFAULT 'legacy',
    policy_version integer NOT NULL DEFAULT 0,
    prorate_ratio double precision,
    updated_at timestamptz NOT NULL,
    CONSTRAINT leave_balances_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT leave_balances_employee_type_period_idx UNIQUE NULLS NOT DISTINCT (tenant_id, employee_id, leave_type, period_start, period_end),
    CONSTRAINT leave_balances_period_check CHECK (period_start IS NULL OR period_end IS NULL OR period_end >= period_start),
    CONSTRAINT leave_balances_nonnegative_check CHECK (remaining_hours >= 0 AND granted_hours >= 0 AND used_hours >= 0),
    CONSTRAINT leave_balances_period_no_overlap EXCLUDE USING gist (
        tenant_id WITH =,
        employee_id WITH =,
        leave_type WITH =,
        daterange(period_start, period_end, '[]') WITH &&
    ),
    CONSTRAINT leave_balances_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id)
);

CREATE INDEX leave_balances_tenant_id_idx ON leave_balances (tenant_id);

CREATE TABLE leave_balance_ledger (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id text NOT NULL,
    balance_id text NOT NULL,
    employee_id text NOT NULL,
    leave_type text NOT NULL,
    period_start date,
    period_end date,
    event_type text NOT NULL CHECK (event_type IN ('snapshot', 'reserve', 'release', 'adjustment')),
    delta_hours numeric(12,2) NOT NULL,
    remaining_hours numeric(12,2) NOT NULL,
    granted_hours numeric(12,2) NOT NULL,
    used_hours numeric(12,2) NOT NULL,
    source text NOT NULL,
    occurred_at timestamptz NOT NULL,
    CONSTRAINT leave_balance_ledger_balance_fk FOREIGN KEY (tenant_id, balance_id) REFERENCES leave_balances (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX leave_balance_ledger_employee_period_idx ON leave_balance_ledger (tenant_id, employee_id, leave_type, period_start, occurred_at);

CREATE FUNCTION append_leave_balance_ledger()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    ledger_event_type text;
BEGIN
    ledger_event_type := CASE
        WHEN TG_OP = 'INSERT' THEN 'snapshot'
        WHEN NEW.remaining_hours < OLD.remaining_hours THEN 'reserve'
        WHEN NEW.remaining_hours > OLD.remaining_hours AND NEW.used_hours < OLD.used_hours THEN 'release'
        ELSE 'adjustment'
    END;
    INSERT INTO leave_balance_ledger (
        tenant_id, balance_id, employee_id, leave_type, period_start, period_end,
        event_type, delta_hours, remaining_hours, granted_hours, used_hours, source, occurred_at
    ) VALUES (
        NEW.tenant_id, NEW.id, NEW.employee_id, NEW.leave_type, NEW.period_start, NEW.period_end,
        ledger_event_type, NEW.remaining_hours - CASE WHEN TG_OP = 'INSERT' THEN 0 ELSE OLD.remaining_hours END,
        NEW.remaining_hours, NEW.granted_hours, NEW.used_hours, NEW.source, NEW.updated_at
    );
    RETURN NEW;
END;
$$;

CREATE TRIGGER leave_balances_ledger_trigger
AFTER INSERT OR UPDATE ON leave_balances
FOR EACH ROW EXECUTE FUNCTION append_leave_balance_ledger();

CREATE TABLE form_definition_drafts (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    owner_account_id text NOT NULL,
    base_template_id text NOT NULL DEFAULT '',
    schema_version integer NOT NULL CHECK (schema_version > 0),
    authoring_schema jsonb NOT NULL DEFAULT '{}'::jsonb,
    compiled_schema jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL CHECK (status IN ('draft', 'review_pending', 'rejected', 'published')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    source text NOT NULL DEFAULT 'manual',
    agent_id text NOT NULL DEFAULT '',
    agent_run_id text NOT NULL DEFAULT '',
    agent_session_id text NOT NULL DEFAULT '',
    tool_call_id text NOT NULL DEFAULT '',
    validation_result jsonb NOT NULL DEFAULT '{}'::jsonb,
    submitted_at timestamptz,
    published_template_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT form_definition_drafts_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT form_definition_drafts_owner_fk FOREIGN KEY (tenant_id, owner_account_id) REFERENCES accounts (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX form_definition_drafts_tenant_status_updated_idx ON form_definition_drafts (tenant_id, status, updated_at DESC);
CREATE INDEX form_definition_drafts_tenant_owner_updated_idx ON form_definition_drafts (tenant_id, owner_account_id, updated_at DESC);
CREATE UNIQUE INDEX form_definition_drafts_agent_call_idx ON form_definition_drafts (tenant_id, agent_run_id, tool_call_id) WHERE agent_run_id <> '' AND tool_call_id <> '';

CREATE TABLE form_templates (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    key text NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    schema jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'archived')),
    current_version integer NOT NULL DEFAULT 1 CHECK (current_version > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    deleted_at timestamptz,
    CONSTRAINT form_templates_tenant_id_id_idx UNIQUE (tenant_id, id)
);

CREATE INDEX form_templates_tenant_id_idx ON form_templates (tenant_id);
CREATE UNIQUE INDEX form_templates_tenant_key_idx ON form_templates (tenant_id, key);

CREATE TABLE form_template_versions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    template_id text NOT NULL,
    version integer NOT NULL CHECK (version > 0),
    schema jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL CHECK (status IN ('draft', 'published', 'archived')),
    created_at timestamptz NOT NULL,
    published_at timestamptz,
    CONSTRAINT form_template_versions_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT form_template_versions_tenant_template_id_idx UNIQUE (tenant_id, template_id, id),
    CONSTRAINT form_template_versions_template_fk FOREIGN KEY (tenant_id, template_id) REFERENCES form_templates (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT form_template_versions_tenant_template_version_idx UNIQUE (tenant_id, template_id, version)
);

CREATE INDEX form_template_versions_tenant_template_idx ON form_template_versions (tenant_id, template_id, version DESC);

CREATE TABLE form_instances (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    template_id text NOT NULL,
    template_version_id text NOT NULL,
    applicant_account_id text NOT NULL,
    status text NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    submitted_at timestamptz NOT NULL,
    approved_by text NOT NULL DEFAULT '',
    current_run_id text NOT NULL DEFAULT '',
    version bigint NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL,
    CONSTRAINT form_instances_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT form_instances_identity_idx UNIQUE (tenant_id, id, template_id, template_version_id),
    CONSTRAINT form_instances_template_fk FOREIGN KEY (tenant_id, template_id) REFERENCES form_templates (tenant_id, id),
    CONSTRAINT form_instances_template_version_fk FOREIGN KEY (tenant_id, template_id, template_version_id) REFERENCES form_template_versions (tenant_id, template_id, id),
    CONSTRAINT form_instances_applicant_account_fk FOREIGN KEY (tenant_id, applicant_account_id) REFERENCES accounts (tenant_id, id)
);

CREATE INDEX form_instances_tenant_id_idx ON form_instances (tenant_id);
CREATE INDEX form_instances_template_id_idx ON form_instances (template_id);
CREATE INDEX form_instances_tenant_applicant_status_idx ON form_instances (tenant_id, applicant_account_id, status, submitted_at DESC);
CREATE INDEX form_instances_tenant_template_status_submitted_idx ON form_instances (tenant_id, template_id, status, submitted_at DESC);

CREATE TABLE form_instance_field_values (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    form_instance_id text NOT NULL,
    template_id text NOT NULL,
    template_version_id text NOT NULL,
    field_id text NOT NULL,
    value_type text NOT NULL CHECK (value_type IN ('text', 'number', 'boolean', 'date', 'timestamp', 'json')),
    value_text text,
    value_number numeric,
    value_boolean boolean,
    value_date date,
    value_timestamp timestamptz,
    value_json jsonb,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, form_instance_id, field_id),
    CONSTRAINT form_instance_field_values_instance_fk FOREIGN KEY (tenant_id, form_instance_id, template_id, template_version_id) REFERENCES form_instances (tenant_id, id, template_id, template_version_id) ON DELETE CASCADE,
    CONSTRAINT form_instance_field_values_one_value_check CHECK (
        num_nonnulls(value_text, value_number, value_boolean, value_date, value_timestamp, value_json) = 1
        AND CASE value_type
            WHEN 'text' THEN value_text IS NOT NULL
            WHEN 'number' THEN value_number IS NOT NULL
            WHEN 'boolean' THEN value_boolean IS NOT NULL
            WHEN 'date' THEN value_date IS NOT NULL
            WHEN 'timestamp' THEN value_timestamp IS NOT NULL
            WHEN 'json' THEN value_json IS NOT NULL
            ELSE false
        END
    )
);

CREATE INDEX form_instance_field_values_text_idx ON form_instance_field_values (tenant_id, template_id, field_id, value_text);
CREATE INDEX form_instance_field_values_number_idx ON form_instance_field_values (tenant_id, template_id, field_id, value_number);
CREATE INDEX form_instance_field_values_boolean_idx ON form_instance_field_values (tenant_id, template_id, field_id, value_boolean);
CREATE INDEX form_instance_field_values_date_idx ON form_instance_field_values (tenant_id, template_id, field_id, value_date);
CREATE INDEX form_instance_field_values_timestamp_idx ON form_instance_field_values (tenant_id, template_id, field_id, value_timestamp);
CREATE INDEX form_instance_field_values_created_idx ON form_instance_field_values (tenant_id, template_id, field_id, created_at DESC);

CREATE TABLE workflow_runs (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    form_instance_id text NOT NULL,
    template_id text NOT NULL,
    version integer NOT NULL,
    status text NOT NULL CHECK (status IN ('running', 'returned', 'completed', 'cancelled', 'start_failed')),
    current_stage_instance_id text,
    stage_definitions_json jsonb NOT NULL DEFAULT '[]'::jsonb,
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
    stage_type text NOT NULL CHECK (stage_type IN ('notify', 'condition', 'approver', 'parallel')),
    label text NOT NULL,
    status text NOT NULL CHECK (status IN ('pending', 'active', 'completed', 'skipped', 'rejected')),
    sequence integer NOT NULL,
    result jsonb NOT NULL DEFAULT '{}'::jsonb,
    started_at timestamptz,
    completed_at timestamptz,
    CONSTRAINT workflow_stage_instances_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT workflow_stage_instances_run_identity_idx UNIQUE (tenant_id, run_id, id),
    CONSTRAINT workflow_stage_instances_run_fk FOREIGN KEY (tenant_id, run_id) REFERENCES workflow_runs (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX workflow_stage_instances_run_sequence_idx ON workflow_stage_instances (tenant_id, run_id, sequence);

CREATE TABLE workflow_stage_assignees (
    tenant_id text NOT NULL,
    stage_instance_id text NOT NULL,
    account_id text NOT NULL,
    status text NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'returned')),
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
    action text NOT NULL CHECK (action IN ('submit', 'approve', 'reject', 'return', 'withdraw', 'notify', 'auto_condition')),
    comment text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    CONSTRAINT workflow_actions_run_fk FOREIGN KEY (tenant_id, run_id) REFERENCES workflow_runs (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT workflow_actions_stage_fk FOREIGN KEY (tenant_id, run_id, stage_instance_id) REFERENCES workflow_stage_instances (tenant_id, run_id, id) ON DELETE CASCADE
);

CREATE INDEX workflow_actions_run_created_idx ON workflow_actions (tenant_id, run_id, created_at);

ALTER TABLE workflow_runs
    ADD CONSTRAINT workflow_runs_current_stage_fk
    FOREIGN KEY (tenant_id, id, current_stage_instance_id)
    REFERENCES workflow_stage_instances (tenant_id, run_id, id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE leave_requests (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    leave_type text NOT NULL,
    leave_type_id text NOT NULL DEFAULT '',
    policy_version integer NOT NULL DEFAULT 0,
    rule_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    evaluation_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    start_at timestamptz NOT NULL,
    end_at timestamptz NOT NULL,
    hours double precision NOT NULL,
    reason text NOT NULL DEFAULT '',
    status text NOT NULL,
    form_instance_id text NOT NULL DEFAULT '',
    leave_balance_id text,
    created_at timestamptz NOT NULL,
    CONSTRAINT leave_requests_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT leave_requests_balance_fk FOREIGN KEY (tenant_id, leave_balance_id) REFERENCES leave_balances (tenant_id, id)
);

CREATE INDEX leave_requests_tenant_id_idx ON leave_requests (tenant_id);
CREATE INDEX leave_requests_employee_id_idx ON leave_requests (employee_id);
CREATE INDEX leave_requests_tenant_form_instance_idx ON leave_requests (tenant_id, form_instance_id);
CREATE INDEX leave_requests_tenant_employee_status_dates_idx ON leave_requests (tenant_id, employee_id, status, start_at, end_at);

CREATE TABLE leave_request_allocations (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    leave_request_id text NOT NULL REFERENCES leave_requests(id) ON DELETE CASCADE,
    leave_balance_id text NOT NULL REFERENCES leave_balances(id),
    reserved_hours numeric(12,2) NOT NULL CHECK (reserved_hours > 0),
    created_at timestamptz NOT NULL,
    CONSTRAINT leave_request_allocations_request_balance_idx UNIQUE (tenant_id, leave_request_id, leave_balance_id)
);

CREATE INDEX leave_request_allocations_tenant_request_idx ON leave_request_allocations (tenant_id, leave_request_id);

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
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT attendance_shift_assignments_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT attendance_shift_assignments_interval_check CHECK (effective_to IS NULL OR effective_to >= effective_from),
    CONSTRAINT attendance_shift_assignments_active_no_overlap EXCLUDE USING gist (
        tenant_id WITH =,
        employee_id WITH =,
        tstzrange(effective_from, effective_to, '[]') WITH &&
    ) WHERE (status = 'active'),
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
    shift_assignment_id text,
    shift_id text,
    worksite_id text,
    work_date text NOT NULL,
    direction text NOT NULL,
    client_event_id text NOT NULL DEFAULT '',
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
    voided boolean NOT NULL DEFAULT false,
    voided_at timestamptz,
    voided_by_account_id text NOT NULL DEFAULT '',
    void_reason text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    CONSTRAINT attendance_clock_records_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT attendance_clock_records_shift_assignment_fk FOREIGN KEY (tenant_id, shift_assignment_id) REFERENCES attendance_shift_assignments (tenant_id, id),
    CONSTRAINT attendance_clock_records_shift_fk FOREIGN KEY (tenant_id, shift_id) REFERENCES attendance_shifts (tenant_id, id),
    CONSTRAINT attendance_clock_records_worksite_fk FOREIGN KEY (tenant_id, worksite_id) REFERENCES attendance_worksites (tenant_id, id)
);

CREATE INDEX attendance_clock_records_tenant_employee_date_idx ON attendance_clock_records (tenant_id, employee_id, work_date DESC);
CREATE INDEX attendance_clock_records_tenant_status_idx ON attendance_clock_records (tenant_id, record_status, clocked_at DESC);
CREATE INDEX attendance_clock_records_effective_boundary_idx ON attendance_clock_records (tenant_id, employee_id, work_date, direction, clocked_at, created_at, id) WHERE record_status = 'accepted' AND voided = false;
CREATE INDEX attendance_clock_records_effective_latest_idx ON attendance_clock_records (tenant_id, employee_id, work_date, clocked_at, created_at, id) WHERE record_status = 'accepted' AND voided = false;
CREATE UNIQUE INDEX attendance_clock_records_client_event_idx ON attendance_clock_records (tenant_id, client_event_id) WHERE client_event_id <> '';

CREATE TABLE attendance_daily_summaries (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    work_date text NOT NULL,
    shift_start text NOT NULL DEFAULT '',
    shift_end text NOT NULL DEFAULT '',
    shift_hours double precision NOT NULL DEFAULT 0,
    daily_hours double precision NOT NULL DEFAULT 0,
    clock_hours double precision NOT NULL DEFAULT 0,
    clock_start text NOT NULL DEFAULT '',
    clock_end text NOT NULL DEFAULT '',
    attend_start text NOT NULL DEFAULT '',
    attend_end text NOT NULL DEFAULT '',
    attend_hours double precision NOT NULL DEFAULT 0,
    attend_counted boolean NOT NULL DEFAULT false,
    leave_type text NOT NULL DEFAULT '',
    leave_start text NOT NULL DEFAULT '',
    leave_end text NOT NULL DEFAULT '',
    leave_hours double precision NOT NULL DEFAULT 0,
    leave_counted boolean NOT NULL DEFAULT false,
    leave2_type text NOT NULL DEFAULT '',
    leave2_start text NOT NULL DEFAULT '',
    leave2_end text NOT NULL DEFAULT '',
    leave2_hours double precision NOT NULL DEFAULT 0,
    leave2_counted boolean NOT NULL DEFAULT false,
    overtime_start text NOT NULL DEFAULT '',
    overtime_end text NOT NULL DEFAULT '',
    overtime_hours double precision NOT NULL DEFAULT 0,
    overtime_counted boolean NOT NULL DEFAULT false,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    source text NOT NULL DEFAULT 'manual',
    external_ref text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT attendance_daily_summaries_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT attendance_daily_summaries_employee_date_idx UNIQUE (tenant_id, employee_id, work_date)
);

CREATE INDEX attendance_daily_summaries_tenant_employee_date_idx ON attendance_daily_summaries (tenant_id, employee_id, work_date DESC);
CREATE INDEX attendance_daily_summaries_tenant_source_date_idx ON attendance_daily_summaries (tenant_id, source, work_date DESC);
CREATE UNIQUE INDEX attendance_daily_summaries_external_ref_idx ON attendance_daily_summaries (tenant_id, external_ref) WHERE external_ref <> '';

CREATE TABLE attendance_correction_requests (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    direction text NOT NULL,
    requested_clocked_at timestamptz NOT NULL,
    work_date text NOT NULL,
    correction_type text NOT NULL DEFAULT 'add_record' CHECK (correction_type IN ('add_record', 'void_record', 'replace_record')),
    target_clock_record_id text NOT NULL DEFAULT '',
    replacement_clock_record_id text NOT NULL DEFAULT '',
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

CREATE TABLE agent_models (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    provider text NOT NULL DEFAULT 'openai',
    model_name text NOT NULL,
    litellm_model text NOT NULL,
    api_base_url text NOT NULL DEFAULT '',
    api_key_ciphertext text NOT NULL DEFAULT '',
    api_key_preview text NOT NULL DEFAULT '',
    rate_limit_rpm integer NOT NULL DEFAULT 0 CHECK (rate_limit_rpm >= 0),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    timeout_seconds integer NOT NULL DEFAULT 60 CHECK (timeout_seconds > 0),
    monthly_quota bigint NOT NULL DEFAULT 100000 CHECK (monthly_quota >= 0),
    used_quota bigint NOT NULL DEFAULT 0 CHECK (used_quota >= 0),
    last_tested_at timestamptz,
    last_test_status text NOT NULL DEFAULT 'untested' CHECK (last_test_status IN ('ok', 'failed', 'untested')),
    last_test_message text NOT NULL DEFAULT '',
    sync_status text NOT NULL DEFAULT 'pending' CHECK (sync_status IN ('pending', 'synced', 'failed')),
    last_synced_at timestamptz,
    last_sync_error text NOT NULL DEFAULT '',
    synced_config_hash text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT agent_models_tenant_id_id_idx UNIQUE (tenant_id, id)
);

CREATE INDEX agent_models_tenant_name_idx ON agent_models (tenant_id, name);

CREATE TABLE agent_external_tools (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    kind text NOT NULL CHECK (kind IN ('mcp', 'http')),
    transport text NOT NULL CHECK (transport IN ('sse', 'streamable_http', 'http')),
    endpoint_url text NOT NULL,
    auth_type text NOT NULL DEFAULT 'none' CHECK (auth_type IN ('none', 'bearer', 'api_key', 'basic')),
    auth_header_name text NOT NULL DEFAULT '',
    auth_username text NOT NULL DEFAULT '',
    auth_secret_ciphertext text NOT NULL DEFAULT '',
    created_by_account_id text,
    created_at timestamptz NOT NULL,
    CONSTRAINT agent_external_tools_transport_kind_check CHECK (
        (kind = 'mcp' AND transport IN ('sse', 'streamable_http')) OR
        (kind = 'http' AND transport = 'http')
    ),
    CONSTRAINT agent_external_tools_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT agent_external_tools_created_by_fk FOREIGN KEY (tenant_id, created_by_account_id) REFERENCES accounts (tenant_id, id)
);

CREATE INDEX agent_external_tools_tenant_created_idx ON agent_external_tools (tenant_id, created_at DESC, id);

CREATE TABLE knowledge_bases (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    chunk_mode text NOT NULL DEFAULT 'auto' CHECK (chunk_mode IN ('auto', 'paragraph', 'fixed')),
    chunk_size integer NOT NULL DEFAULT 1200 CHECK (chunk_size BETWEEN 200 AND 4000),
    chunk_overlap integer NOT NULL DEFAULT 200 CHECK (chunk_overlap >= 0 AND chunk_overlap < chunk_size),
    created_by_account_id text,
    updated_by_account_id text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT knowledge_bases_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT knowledge_bases_created_by_fk FOREIGN KEY (tenant_id, created_by_account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT knowledge_bases_updated_by_fk FOREIGN KEY (tenant_id, updated_by_account_id) REFERENCES accounts (tenant_id, id)
);

CREATE INDEX knowledge_bases_tenant_updated_idx ON knowledge_bases (tenant_id, updated_at DESC, id);

CREATE TABLE knowledge_documents (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    knowledge_base_id text NOT NULL,
    title text NOT NULL,
    content text NOT NULL,
    source_type text NOT NULL DEFAULT 'manual' CHECK (source_type IN ('manual', 'text', 'pdf')),
    original_filename text NOT NULL DEFAULT '',
    content_type text NOT NULL DEFAULT '',
    size_bytes bigint NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    sha256 text NOT NULL DEFAULT '',
    object_provider text NOT NULL DEFAULT '',
    object_bucket text NOT NULL DEFAULT '',
    object_key text NOT NULL DEFAULT '',
    parse_status text NOT NULL DEFAULT 'ready' CHECK (parse_status IN ('ready', 'failed')),
    parse_error text NOT NULL DEFAULT '',
    created_by_account_id text,
    updated_by_account_id text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT knowledge_documents_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT knowledge_documents_base_fk FOREIGN KEY (tenant_id, knowledge_base_id) REFERENCES knowledge_bases (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT knowledge_documents_created_by_fk FOREIGN KEY (tenant_id, created_by_account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT knowledge_documents_updated_by_fk FOREIGN KEY (tenant_id, updated_by_account_id) REFERENCES accounts (tenant_id, id)
);

CREATE INDEX knowledge_documents_base_updated_idx ON knowledge_documents (tenant_id, knowledge_base_id, updated_at DESC, id);

CREATE TABLE knowledge_document_chunks (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    knowledge_base_id text NOT NULL,
    document_id text NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    content text NOT NULL,
    embedding_model text NOT NULL,
    embedding_dimensions integer NOT NULL CHECK (embedding_dimensions > 0),
    embedding vector NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT knowledge_document_chunks_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT knowledge_document_chunks_document_ordinal_idx UNIQUE (tenant_id, document_id, ordinal),
    CONSTRAINT knowledge_document_chunks_base_fk FOREIGN KEY (tenant_id, knowledge_base_id) REFERENCES knowledge_bases (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT knowledge_document_chunks_document_fk FOREIGN KEY (tenant_id, document_id) REFERENCES knowledge_documents (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX knowledge_document_chunks_search_idx
    ON knowledge_document_chunks (tenant_id, knowledge_base_id, embedding_model, embedding_dimensions, document_id, ordinal);

CREATE TABLE agent_definitions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    emoji text NOT NULL DEFAULT 'AI',
    category text NOT NULL DEFAULT 'workflow' CHECK (category IN ('workflow', 'doc', 'analytics', 'it')),
    model_id text NOT NULL,
	main_agent_role text NOT NULL DEFAULT '',
	sub_agents jsonb NOT NULL DEFAULT '[]'::jsonb,
    system_prompt text NOT NULL DEFAULT '',
    welcome_message text NOT NULL DEFAULT '',
    suggested_questions jsonb NOT NULL DEFAULT '[]'::jsonb,
    suggested_question_translations jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(suggested_question_translations) = 'array'),
    tools jsonb NOT NULL DEFAULT '[]'::jsonb,
    knowledge_base_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
    visibility text NOT NULL DEFAULT 'all' CHECK (visibility IN ('all', 'department', 'role')),
    visibility_targets jsonb NOT NULL DEFAULT '[]'::jsonb,
    timeout_seconds integer NOT NULL DEFAULT 60 CHECK (timeout_seconds > 0),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
	published_version integer NOT NULL DEFAULT 0 CHECK (published_version >= 0),
    usage_total_runs bigint NOT NULL DEFAULT 0,
    usage_success_runs bigint NOT NULL DEFAULT 0,
    usage_failed_runs bigint NOT NULL DEFAULT 0,
    usage_avg_latency_ms integer NOT NULL DEFAULT 0,
    usage_last_run_at timestamptz,
    usage_top_prompts jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_by_account_id text,
    updated_by_account_id text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT agent_definitions_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT agent_definitions_model_fk FOREIGN KEY (tenant_id, model_id) REFERENCES agent_models (tenant_id, id),
    CONSTRAINT agent_definitions_created_by_fk FOREIGN KEY (tenant_id, created_by_account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT agent_definitions_updated_by_fk FOREIGN KEY (tenant_id, updated_by_account_id) REFERENCES accounts (tenant_id, id)
);

CREATE INDEX agent_definitions_tenant_status_idx ON agent_definitions (tenant_id, status, updated_at DESC);
CREATE INDEX agent_definitions_tenant_name_idx ON agent_definitions (tenant_id, name);

CREATE TABLE agent_definition_versions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    agent_id text NOT NULL,
    version integer NOT NULL CHECK (version > 0),
	main_agent_role text NOT NULL DEFAULT '',
	sub_agents jsonb NOT NULL DEFAULT '[]'::jsonb,
    system_prompt text NOT NULL DEFAULT '',
    welcome_message text NOT NULL DEFAULT '',
    suggested_questions jsonb NOT NULL DEFAULT '[]'::jsonb,
    suggested_question_translations jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(suggested_question_translations) = 'array'),
    tools jsonb NOT NULL DEFAULT '[]'::jsonb,
    knowledge_base_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
    model_id text NOT NULL,
    note text NOT NULL DEFAULT '',
    created_by_account_id text,
    created_at timestamptz NOT NULL,
    CONSTRAINT agent_definition_versions_unique UNIQUE (tenant_id, agent_id, version),
    CONSTRAINT agent_definition_versions_agent_fk FOREIGN KEY (tenant_id, agent_id) REFERENCES agent_definitions (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT agent_definition_versions_created_by_fk FOREIGN KEY (tenant_id, created_by_account_id) REFERENCES accounts (tenant_id, id)
);

CREATE INDEX agent_definition_versions_agent_idx ON agent_definition_versions (tenant_id, agent_id, version DESC);

CREATE TABLE agent_runs (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id text NOT NULL,
    agent_id text,
    session_id text,
    mode text NOT NULL,
    prompt text NOT NULL,
    answer text NOT NULL DEFAULT '',
    status text NOT NULL,
    reference_items jsonb NOT NULL DEFAULT '[]'::jsonb,
    llm_call_count bigint NOT NULL DEFAULT 0 CHECK (llm_call_count >= 0),
    input_tokens bigint NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    cached_tokens bigint NOT NULL DEFAULT 0 CHECK (cached_tokens >= 0),
    output_tokens bigint NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    total_tokens bigint NOT NULL DEFAULT 0 CHECK (total_tokens >= 0),
    usage_complete boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT agent_runs_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT agent_runs_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT agent_runs_agent_fk FOREIGN KEY (tenant_id, agent_id) REFERENCES agent_definitions (tenant_id, id) ON DELETE SET NULL (agent_id)
);

CREATE INDEX agent_runs_tenant_id_idx ON agent_runs (tenant_id);
CREATE INDEX agent_runs_account_id_idx ON agent_runs (account_id);
CREATE INDEX agent_runs_tenant_account_created_at_idx ON agent_runs (tenant_id, account_id, created_at DESC);
CREATE INDEX agent_runs_tenant_agent_id_idx ON agent_runs (tenant_id, agent_id, created_at DESC);
CREATE UNIQUE INDEX agent_runs_active_session_unique
    ON agent_runs (tenant_id, session_id)
    WHERE session_id IS NOT NULL AND status IN ('queued', 'running');

CREATE TABLE agent_sessions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id text NOT NULL,
    agent_id text,
    title text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    context_version bigint NOT NULL DEFAULT 1 CHECK (context_version > 0),
    last_message_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT agent_sessions_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT agent_sessions_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT agent_sessions_agent_fk FOREIGN KEY (tenant_id, agent_id) REFERENCES agent_definitions (tenant_id, id) ON DELETE SET NULL (agent_id)
);

ALTER TABLE agent_runs
    ADD CONSTRAINT agent_runs_session_fk FOREIGN KEY (tenant_id, session_id) REFERENCES agent_sessions (tenant_id, id) ON DELETE SET NULL (session_id);

CREATE INDEX agent_sessions_tenant_account_updated_idx
    ON agent_sessions (tenant_id, account_id, status, updated_at DESC, id DESC);
CREATE INDEX agent_sessions_tenant_agent_idx
    ON agent_sessions (tenant_id, agent_id, updated_at DESC);

CREATE TABLE agent_session_messages (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    session_id text NOT NULL,
    role text NOT NULL CHECK (role IN ('user', 'assistant', 'system', 'tool')),
    content text NOT NULL DEFAULT '',
    run_id text,
    context_version bigint NOT NULL DEFAULT 1 CHECK (context_version > 0),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL,
    CONSTRAINT agent_session_messages_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT agent_session_messages_session_fk
        FOREIGN KEY (tenant_id, session_id) REFERENCES agent_sessions (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT agent_session_messages_run_fk
        FOREIGN KEY (tenant_id, run_id) REFERENCES agent_runs (tenant_id, id) ON DELETE SET NULL (run_id)
);

CREATE INDEX agent_session_messages_session_created_idx
    ON agent_session_messages (tenant_id, session_id, created_at ASC, id ASC);

CREATE INDEX agent_session_messages_context_idx
    ON agent_session_messages (tenant_id, session_id, context_version, created_at ASC, id ASC);

CREATE TABLE file_assets (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    created_by_account_id text NOT NULL,
    original_filename text NOT NULL,
    object_provider text NOT NULL,
    object_bucket text NOT NULL DEFAULT '',
    object_key text NOT NULL,
    content_type text NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
    sha256 text NOT NULL,
    scan_status text NOT NULL DEFAULT 'not_configured' CHECK (scan_status IN ('not_configured', 'pending', 'clean', 'rejected')),
    parse_status text NOT NULL DEFAULT 'pending' CHECK (parse_status IN ('pending', 'ready', 'failed', 'unsupported')),
    retention_class text NOT NULL DEFAULT 'conversation' CHECK (retention_class IN ('conversation', 'knowledge', 'permanent')),
    expires_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    deleted_at timestamptz,
    CONSTRAINT file_assets_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT file_assets_creator_fk FOREIGN KEY (tenant_id, created_by_account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT file_assets_object_key_idx UNIQUE (tenant_id, object_key)
);

CREATE INDEX file_assets_tenant_created_idx
    ON file_assets (tenant_id, created_at DESC, id DESC);

CREATE TABLE file_chunks (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    file_id text NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    content text NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT file_chunks_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT file_chunks_file_ordinal_idx UNIQUE (tenant_id, file_id, ordinal),
    CONSTRAINT file_chunks_file_fk FOREIGN KEY (tenant_id, file_id) REFERENCES file_assets (tenant_id, id) ON DELETE CASCADE
);

CREATE TABLE agent_session_files (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    session_id text NOT NULL,
    file_id text NOT NULL,
    context_version bigint NOT NULL CHECK (context_version > 0),
    state text NOT NULL DEFAULT 'draft' CHECK (state IN ('draft', 'attached')),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, session_id, file_id),
    CONSTRAINT agent_session_files_session_fk FOREIGN KEY (tenant_id, session_id) REFERENCES agent_sessions (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT agent_session_files_file_fk FOREIGN KEY (tenant_id, file_id) REFERENCES file_assets (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX agent_session_files_context_idx
    ON agent_session_files (tenant_id, session_id, context_version, created_at ASC, file_id ASC);

CREATE TABLE agent_message_attachments (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    message_id text NOT NULL,
    file_id text NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, message_id, file_id),
    CONSTRAINT agent_message_attachments_message_fk FOREIGN KEY (tenant_id, message_id) REFERENCES agent_session_messages (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT agent_message_attachments_file_fk FOREIGN KEY (tenant_id, file_id) REFERENCES file_assets (tenant_id, id) ON DELETE RESTRICT
);

CREATE INDEX agent_message_attachments_message_idx
    ON agent_message_attachments (tenant_id, message_id, ordinal ASC, file_id ASC);

CREATE TABLE agent_memories (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id text NOT NULL,
    agent_id text,
    session_id text,
    key text NOT NULL DEFAULT '',
    content text NOT NULL,
    source text NOT NULL DEFAULT 'auto' CHECK (source IN ('auto', 'manual')),
    importance integer NOT NULL DEFAULT 1 CHECK (importance >= 1 AND importance <= 5),
    expires_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT agent_memories_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT agent_memories_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT agent_memories_agent_fk
        FOREIGN KEY (tenant_id, agent_id) REFERENCES agent_definitions (tenant_id, id) ON DELETE SET NULL (agent_id),
    CONSTRAINT agent_memories_session_fk
        FOREIGN KEY (tenant_id, session_id) REFERENCES agent_sessions (tenant_id, id) ON DELETE SET NULL (session_id)
);

CREATE INDEX agent_memories_tenant_account_idx
    ON agent_memories (tenant_id, account_id, updated_at DESC, id DESC);
CREATE INDEX agent_memories_tenant_agent_idx
    ON agent_memories (tenant_id, account_id, agent_id, importance DESC, updated_at DESC);

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
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'succeeded', 'failed', 'parked')),
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

ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenants FORCE ROW LEVEL SECURITY;
ALTER TABLE accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE accounts FORCE ROW LEVEL SECURITY;
ALTER TABLE user_groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_groups FORCE ROW LEVEL SECURITY;
ALTER TABLE authz_group_memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE authz_group_memberships FORCE ROW LEVEL SECURITY;
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
ALTER TABLE permission_package_imports ENABLE ROW LEVEL SECURITY;
ALTER TABLE permission_package_imports FORCE ROW LEVEL SECURITY;
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
ALTER TABLE attendance_policy_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_policy_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_types ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_types FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_type_external_mappings ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_type_external_mappings FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_type_sync_issues ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_type_sync_issues FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_balances ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_balances FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_balance_ledger ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_balance_ledger FORCE ROW LEVEL SECURITY;
ALTER TABLE form_definition_drafts ENABLE ROW LEVEL SECURITY;
ALTER TABLE form_definition_drafts FORCE ROW LEVEL SECURITY;
ALTER TABLE form_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE form_templates FORCE ROW LEVEL SECURITY;
ALTER TABLE form_template_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE form_template_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE form_instances ENABLE ROW LEVEL SECURITY;
ALTER TABLE form_instances FORCE ROW LEVEL SECURITY;
ALTER TABLE form_instance_field_values ENABLE ROW LEVEL SECURITY;
ALTER TABLE form_instance_field_values FORCE ROW LEVEL SECURITY;
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
ALTER TABLE leave_request_allocations ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_request_allocations FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_worksites ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_worksites FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_shifts ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_shifts FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_shift_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_shift_assignments FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_clock_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_clock_records FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_daily_summaries ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_daily_summaries FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_correction_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_correction_requests FORCE ROW LEVEL SECURITY;
ALTER TABLE overtime_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE overtime_requests FORCE ROW LEVEL SECURITY;
ALTER TABLE platform_task_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform_task_items FORCE ROW LEVEL SECURITY;
ALTER TABLE platform_task_todos ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform_task_todos FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_models ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_models FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_external_tools ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_external_tools FORCE ROW LEVEL SECURITY;
ALTER TABLE knowledge_bases ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_bases FORCE ROW LEVEL SECURITY;
ALTER TABLE knowledge_documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_documents FORCE ROW LEVEL SECURITY;
ALTER TABLE knowledge_document_chunks ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_document_chunks FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_definitions ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_definition_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_definition_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_runs FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_sessions FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_session_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_session_messages FORCE ROW LEVEL SECURITY;
ALTER TABLE file_assets ENABLE ROW LEVEL SECURITY;
ALTER TABLE file_assets FORCE ROW LEVEL SECURITY;
ALTER TABLE file_chunks ENABLE ROW LEVEL SECURITY;
ALTER TABLE file_chunks FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_session_files ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_session_files FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_message_attachments ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_message_attachments FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_memories ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_memories FORCE ROW LEVEL SECURITY;

ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE notifications FORCE ROW LEVEL SECURITY;
ALTER TABLE notification_recipients ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_recipients FORCE ROW LEVEL SECURITY;
ALTER TABLE outbox_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox_events FORCE ROW LEVEL SECURITY;
ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_logs FORCE ROW LEVEL SECURITY;

-- tenants table 沒有 tenant_id 欄位；每一列以自身 id 隔離。
CREATE POLICY tenant_isolation_tenants ON tenants USING (id = current_setting('app.tenant_id', true)) WITH CHECK (id = current_setting('app.tenant_id', true));

-- 跨 tenant 背景工作（例如 OpenFGA outbox processor）需要列舉所有 tenant，
-- 但 tenant_isolation_tenants 只會暴露符合 app.tenant_id 的列。這個唯讀 policy
-- 允許透過 set_config('app.system_task', 'on', true) opt in 的連線在沒有 BYPASSRLS
-- 的情況下列出所有 tenant。應用程式會透過 tenantctx.WithSystemTask 注入此設定
-- （見 internal/repository/postgres/tenant_dbtx.go）；行為由 tenantctx 單元測試與
-- tests/integration/postgres 內的 non-BYPASSRLS ListTenants 整合測試覆蓋。
CREATE POLICY system_read_tenants ON tenants FOR SELECT USING (current_setting('app.system_task', true) = 'on');

CREATE POLICY tenant_isolation_accounts ON accounts USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_user_groups ON user_groups USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_authz_group_memberships ON authz_group_memberships USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_permission_sets ON permission_sets USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_permissions ON permissions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_menu_items ON menu_items USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_permission_set_items ON permission_set_items USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_assumable_roles ON assumable_roles USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_permission_package_imports ON permission_package_imports USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
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
CREATE POLICY tenant_isolation_employees ON employees USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_employee_number_sequences ON employee_number_sequences USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_employee_import_sessions ON employee_import_sessions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_employment_contracts ON employment_contracts USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_policies ON attendance_policies USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_policy_versions ON attendance_policy_versions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_types ON leave_types USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_type_external_mappings ON leave_type_external_mappings USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_type_sync_issues ON leave_type_sync_issues USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_balances ON leave_balances USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_balance_ledger ON leave_balance_ledger USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_form_definition_drafts ON form_definition_drafts USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_form_templates ON form_templates USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_form_template_versions ON form_template_versions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_form_instances ON form_instances USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_form_instance_field_values ON form_instance_field_values USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_workflow_runs ON workflow_runs USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_workflow_stage_instances ON workflow_stage_instances USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_workflow_stage_assignees ON workflow_stage_assignees USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_workflow_actions ON workflow_actions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_requests ON leave_requests USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_request_allocations ON leave_request_allocations USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_worksites ON attendance_worksites USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_shifts ON attendance_shifts USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_shift_assignments ON attendance_shift_assignments USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_clock_records ON attendance_clock_records USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_daily_summaries ON attendance_daily_summaries USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_correction_requests ON attendance_correction_requests USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_overtime_requests ON overtime_requests USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_platform_task_items ON platform_task_items USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_platform_task_todos ON platform_task_todos USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_models ON agent_models USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_external_tools ON agent_external_tools USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_knowledge_bases ON knowledge_bases USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_knowledge_documents ON knowledge_documents USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_knowledge_document_chunks ON knowledge_document_chunks USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_definitions ON agent_definitions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_definition_versions ON agent_definition_versions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_runs ON agent_runs USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_sessions ON agent_sessions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_session_messages ON agent_session_messages USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_file_assets ON file_assets USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_file_chunks ON file_chunks USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_session_files ON agent_session_files USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_message_attachments ON agent_message_attachments USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_memories ON agent_memories USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

CREATE POLICY tenant_isolation_notifications ON notifications USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_notification_recipients ON notification_recipients USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_outbox_events ON outbox_events USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_audit_logs ON audit_logs USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
