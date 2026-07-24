
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
    source text NOT NULL DEFAULT '',
    closed boolean NOT NULL DEFAULT false,
    show_in_org_chart boolean NOT NULL DEFAULT true,
    manager_position_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE INDEX org_units_tenant_id_idx ON org_units (tenant_id);
CREATE INDEX org_units_path_idx ON org_units USING gin (path);
CREATE INDEX org_units_tenant_manager_position_idx ON org_units (tenant_id, manager_position_id) WHERE manager_position_id <> '';

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

CREATE TRIGGER org_units_manager_position_check
BEFORE INSERT OR UPDATE OF tenant_id, manager_position_id ON org_units
FOR EACH ROW EXECUTE FUNCTION validate_org_unit_manager_position();

CREATE TABLE positions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code text NOT NULL,
    name text NOT NULL,
    name_en text NOT NULL DEFAULT '',
    level text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    description text NOT NULL DEFAULT '',
    source text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT positions_tenant_id_id_idx UNIQUE (tenant_id, id)
);

CREATE UNIQUE INDEX positions_tenant_code_ci_idx ON positions (tenant_id, lower(code));
CREATE INDEX positions_tenant_name_ci_idx ON positions (tenant_id, lower(name));
CREATE INDEX positions_tenant_status_idx ON positions (tenant_id, status, name);
CREATE TABLE employees (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_no text NOT NULL DEFAULT '',
    external_source text NOT NULL DEFAULT '',
    external_employee_id text NOT NULL DEFAULT '',
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
    source_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    source_updated_at timestamptz,
    last_synced_at timestamptz,
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
CREATE UNIQUE INDEX employees_tenant_external_identity_idx ON employees (tenant_id, external_source, external_employee_id) WHERE external_source <> '' AND external_employee_id <> '';
CREATE UNIQUE INDEX employees_tenant_account_id_idx ON employees (tenant_id, account_id) WHERE account_id <> '';
CREATE UNIQUE INDEX employees_tenant_company_email_idx ON employees (tenant_id, lower(company_email)) WHERE company_email <> '';
CREATE UNIQUE INDEX employees_tenant_personal_email_idx ON employees (tenant_id, lower(personal_email)) WHERE personal_email <> '';
CREATE UNIQUE INDEX employees_tenant_national_id_idx ON employees (tenant_id, lower(basic_info->>'national_id')) WHERE coalesce(basic_info->>'national_id', '') <> '';
CREATE UNIQUE INDEX employees_tenant_passport_no_idx ON employees (tenant_id, lower(basic_info->>'passport_no')) WHERE coalesce(basic_info->>'passport_no', '') <> '';
CREATE UNIQUE INDEX employees_tenant_arc_no_idx ON employees (tenant_id, lower(basic_info->>'arc_no')) WHERE coalesce(basic_info->>'arc_no', '') <> '';
CREATE UNIQUE INDEX employees_tenant_tax_id_idx ON employees (tenant_id, lower(basic_info->>'tax_id')) WHERE coalesce(basic_info->>'tax_id', '') <> '';
CREATE UNIQUE INDEX employees_tenant_work_permit_no_idx ON employees (tenant_id, lower(basic_info->>'work_permit_no')) WHERE coalesce(basic_info->>'work_permit_no', '') <> '';

CREATE TABLE attendance_policy_versions (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    version integer NOT NULL CHECK (version > 0),
    work_time jsonb NOT NULL DEFAULT '{}'::jsonb,
    effective_from timestamptz NOT NULL,
    published_by_account_id text NOT NULL DEFAULT '',
    published_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, version)
);

CREATE TABLE leave_types (
    id text NOT NULL,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code text NOT NULL,
    kind text NOT NULL DEFAULT 'item' CHECK (kind IN ('category', 'item', 'special_group')),
    parent_id text,
    parent_code text,
    name text NOT NULL,
    name_zh text NOT NULL,
    name_en text NOT NULL DEFAULT '',
    category text NOT NULL DEFAULT 'company' CHECK (category IN ('statutory', 'company')),
    source_of_truth text NOT NULL DEFAULT 'ehrms' CHECK (source_of_truth IN ('local_policy', 'system_default', 'ehrms', 'overtime', 'manual')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    requires_balance boolean NOT NULL DEFAULT false,
    max_balance_minutes integer NOT NULL DEFAULT 0 CHECK (max_balance_minutes >= 0),
    unit text NOT NULL DEFAULT '',
    display_order integer NOT NULL DEFAULT 0,
    raw_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    last_synced_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, id),
    CONSTRAINT leave_types_tenant_kind_code_idx UNIQUE (tenant_id, kind, code),
    CONSTRAINT leave_types_id_format_check CHECK (
        id = code OR (kind <> 'item' AND id = kind || ':' || code)
    ),
    CONSTRAINT leave_types_parent_shape_check CHECK (
        (kind IN ('category', 'special_group') AND parent_id IS NULL AND parent_code IS NULL)
        OR (kind = 'item' AND (parent_id IS NULL) = (parent_code IS NULL) AND parent_id IS DISTINCT FROM id)
    ),
    CONSTRAINT leave_types_parent_fk FOREIGN KEY (tenant_id, parent_id)
        REFERENCES leave_types (tenant_id, id) ON UPDATE CASCADE ON DELETE RESTRICT
);

CREATE INDEX leave_types_tenant_code_idx ON leave_types (tenant_id, code);

COMMENT ON TABLE leave_types IS 'EHRMS /leave-types 假別樹；id 預設使用 code';
COMMENT ON COLUMN leave_types.id IS '穩定主鍵；預設等於 code，category 與 item 同 code 時 category 使用 category:<code>';
COMMENT ON COLUMN leave_types.tenant_id IS '租戶 ID';
COMMENT ON COLUMN leave_types.code IS '假別代碼（与上游 /leave-types code 一致）';
COMMENT ON COLUMN leave_types.kind IS '節點類型：category=上級分類 / item=可申請假別 / special_group=特殊群組';
COMMENT ON COLUMN leave_types.parent_id IS '解析後的上級節點 ID；自關聯 leave_types.id';
COMMENT ON COLUMN leave_types.parent_code IS '上級假別代碼；category 為 NULL，item 可指向 category';
COMMENT ON COLUMN leave_types.name IS '显示名称';
COMMENT ON COLUMN leave_types.name_zh IS '中文名称';
COMMENT ON COLUMN leave_types.name_en IS '英文名称';
COMMENT ON COLUMN leave_types.category IS '分类：statutory=法定 / company=公司';
COMMENT ON COLUMN leave_types.source_of_truth IS '数据来源：ehrms / local_policy / manual 等';
COMMENT ON COLUMN leave_types.status IS '启用状态：active / inactive';
COMMENT ON COLUMN leave_types.requires_balance IS '是否需要余额（由 max_value>0 推导）';
COMMENT ON COLUMN leave_types.max_balance_minutes IS '额度上限（分钟）；由上游 max_value 按 unit 换算';
COMMENT ON COLUMN leave_types.unit IS '上游额度单位（days/hours 等）';
COMMENT ON COLUMN leave_types.display_order IS '展示排序';
COMMENT ON COLUMN leave_types.raw_payload IS '上游 /leave-types 原始字段快照';
COMMENT ON COLUMN leave_types.last_synced_at IS '最近一次从 EHRMS 同步时间';
COMMENT ON COLUMN leave_types.created_at IS '创建时间';
COMMENT ON COLUMN leave_types.updated_at IS '更新时间';

CREATE TABLE leave_balances (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    leave_type_id text NOT NULL,
    entitlement_year integer NOT NULL CHECK (entitlement_year >= 2000),
    granted_minutes integer NOT NULL DEFAULT 0,
    used_minutes integer NOT NULL DEFAULT 0,
    remaining_minutes integer NOT NULL DEFAULT 0,
    source text NOT NULL CHECK (source IN ('nexus', 'ehrms')),
    source_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    source_updated_at timestamptz,
    last_synced_at timestamptz,
    updated_at timestamptz NOT NULL,
    CONSTRAINT leave_balances_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT leave_balances_nonnegative_check CHECK (granted_minutes >= 0 AND used_minutes >= 0),
    CONSTRAINT leave_balances_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT leave_balances_leave_type_fk FOREIGN KEY (tenant_id, leave_type_id) REFERENCES leave_types (tenant_id, id),
    CONSTRAINT leave_balances_tenant_identity_idx UNIQUE (tenant_id, id, employee_id, leave_type_id, entitlement_year),
    CONSTRAINT leave_balances_employee_type_year_idx UNIQUE (tenant_id, employee_id, leave_type_id, entitlement_year)
);

CREATE INDEX leave_balances_employee_year_idx
ON leave_balances (tenant_id, employee_id, entitlement_year DESC, leave_type_id);

COMMENT ON TABLE leave_balances IS '员工按年度、假别汇总的假期余额；同一员工、假别、年度仅一条';
COMMENT ON COLUMN leave_balances.id IS '余额主键';
COMMENT ON COLUMN leave_balances.tenant_id IS '租户 ID';
COMMENT ON COLUMN leave_balances.employee_id IS '员工 ID';
COMMENT ON COLUMN leave_balances.leave_type_id IS '假别代码；Nexus 与 eHRMS 使用同一套代码';
COMMENT ON COLUMN leave_balances.entitlement_year IS '额度所属年度；不结转';
COMMENT ON COLUMN leave_balances.granted_minutes IS '年度授予分钟数';
COMMENT ON COLUMN leave_balances.used_minutes IS '上游已使用分钟数';
COMMENT ON COLUMN leave_balances.remaining_minutes IS '余额快照剩余分钟数';
COMMENT ON COLUMN leave_balances.source IS '余额来源：nexus / ehrms';
COMMENT ON COLUMN leave_balances.source_payload IS '最近一次上游额度原始字段快照';
COMMENT ON COLUMN leave_balances.source_updated_at IS '上游额度记录更新时间';
COMMENT ON COLUMN leave_balances.last_synced_at IS '最近同步时间；Nexus 本地余额可为空';
COMMENT ON COLUMN leave_balances.updated_at IS '更新时间';

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
    published_version integer NOT NULL DEFAULT 0 CHECK (published_version >= 0 AND published_version <= current_version),
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
    temporal_start_status text NOT NULL DEFAULT 'started' CHECK (temporal_start_status IN ('pending_start', 'starting', 'started', 'abandoned')),
    temporal_workflow_id text NOT NULL DEFAULT '',
    temporal_run_id text NOT NULL DEFAULT '',
    temporal_start_event_id text NOT NULL DEFAULT '',
    temporal_started_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT workflow_runs_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT workflow_runs_form_fk FOREIGN KEY (tenant_id, form_instance_id) REFERENCES form_instances (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT workflow_runs_template_fk FOREIGN KEY (tenant_id, template_id) REFERENCES form_templates (tenant_id, id)
);

CREATE INDEX workflow_runs_tenant_form_version_idx ON workflow_runs (tenant_id, form_instance_id, version);
CREATE INDEX workflow_runs_temporal_start_claimable_idx ON workflow_runs (tenant_id, updated_at, id) WHERE temporal_start_status IN ('pending_start', 'starting');
CREATE UNIQUE INDEX workflow_runs_temporal_start_event_uidx ON workflow_runs (tenant_id, temporal_start_event_id) WHERE temporal_start_event_id <> '';

CREATE TABLE workflow_stage_instances (
    id text PRIMARY KEY,
    tenant_id text NOT NULL,
    run_id text NOT NULL,
    stage_id text NOT NULL,
    stage_type text NOT NULL CHECK (stage_type IN ('notify', 'condition', 'approver')),
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
    idempotency_key text NOT NULL DEFAULT '',
    command_fingerprint text NOT NULL DEFAULT '',
    request_id text NOT NULL DEFAULT '',
    trace_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    CONSTRAINT workflow_actions_run_fk FOREIGN KEY (tenant_id, run_id) REFERENCES workflow_runs (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT workflow_actions_stage_fk FOREIGN KEY (tenant_id, run_id, stage_instance_id) REFERENCES workflow_stage_instances (tenant_id, run_id, id) ON DELETE CASCADE
);

CREATE INDEX workflow_actions_run_created_idx ON workflow_actions (tenant_id, run_id, created_at);
CREATE UNIQUE INDEX workflow_actions_run_idempotency_uidx ON workflow_actions (tenant_id, run_id, idempotency_key) WHERE idempotency_key <> '';

ALTER TABLE workflow_runs
    ADD CONSTRAINT workflow_runs_current_stage_fk
    FOREIGN KEY (tenant_id, id, current_stage_instance_id)
    REFERENCES workflow_stage_instances (tenant_id, run_id, id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE form_business_records (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    form_instance_id text NOT NULL,
    business_type text NOT NULL CHECK (btrim(business_type) <> ''),
    schema_version integer NOT NULL DEFAULT 1 CHECK (schema_version > 0),
    subject_employee_id text,
    effective_from timestamptz,
    effective_to timestamptz,
    business_date date,
    data jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(data) = 'object'),
    effect_status text NOT NULL DEFAULT 'not_applied' CHECK (effect_status IN ('not_applied', 'applying', 'applied', 'failed', 'compensated')),
    result jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(result) = 'object'),
    last_error jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(last_error) = 'object'),
    handler_key text NOT NULL CHECK (btrim(handler_key) <> ''),
    handler_version integer NOT NULL DEFAULT 1 CHECK (handler_version > 0),
    applied_at timestamptz,
    lock_version integer NOT NULL DEFAULT 0 CHECK (lock_version >= 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT form_business_records_interval_check CHECK (
        effective_from IS NULL OR effective_to IS NULL OR effective_to > effective_from
    ),
    CONSTRAINT form_business_records_effect_check CHECK (
        (effect_status = 'applied' AND applied_at IS NOT NULL)
        OR (effect_status <> 'applied')
    ),
    CONSTRAINT form_business_records_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT form_business_records_subject_identity_idx UNIQUE (tenant_id, id, subject_employee_id),
    CONSTRAINT form_business_records_form_type_idx UNIQUE (tenant_id, form_instance_id, business_type),
    CONSTRAINT form_business_records_form_fk FOREIGN KEY (tenant_id, form_instance_id) REFERENCES form_instances (tenant_id, id),
    CONSTRAINT form_business_records_employee_fk FOREIGN KEY (tenant_id, subject_employee_id) REFERENCES employees (tenant_id, id)
);

CREATE INDEX form_business_records_tenant_type_subject_date_idx
ON form_business_records (tenant_id, business_type, subject_employee_id, business_date DESC);
CREATE INDEX form_business_records_tenant_effect_idx
ON form_business_records (tenant_id, effect_status, updated_at);
CREATE INDEX form_business_records_data_gin_idx ON form_business_records USING gin (data);

CREATE TABLE leave_records (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    leave_type_id text NOT NULL,
    balance_id text,
    entitlement_year integer NOT NULL CHECK (entitlement_year >= 2000),
    source text NOT NULL CHECK (source IN ('nexus', 'ehrms')),
    external_ref text NOT NULL DEFAULT '',
    event_date timestamptz NOT NULL,
    start_at timestamptz NOT NULL,
    end_at timestamptz NOT NULL,
    net_minutes integer NOT NULL CHECK (net_minutes > 0),
    remark text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('pending', 'active', 'cancelled', 'corrected')),
    matched_record_id text,
    reconciliation_status text NOT NULL DEFAULT 'unmatched' CHECK (
        reconciliation_status IN ('unmatched', 'matched', 'mismatch', 'ambiguous', 'not_required')
    ),
    balance_match_status text NOT NULL DEFAULT 'matched' CHECK (
        balance_match_status IN ('matched', 'unmatched')
    ),
    balance_match_reason text NOT NULL DEFAULT '',
    source_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    source_updated_at timestamptz,
    last_seen_at timestamptz,
    deleted_at timestamptz,
    updated_at timestamptz NOT NULL,
    CONSTRAINT leave_records_interval_check CHECK (end_at > start_at),
    CONSTRAINT leave_records_year_check CHECK (
        EXTRACT(YEAR FROM start_at AT TIME ZONE 'Asia/Shanghai')::integer = entitlement_year
    ),
    CONSTRAINT leave_records_match_shape_check CHECK (
        (source = 'nexus' AND matched_record_id IS NULL)
        OR source = 'ehrms'
    ),
    CONSTRAINT leave_records_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT leave_records_type_fk FOREIGN KEY (tenant_id, leave_type_id) REFERENCES leave_types (tenant_id, id),
    CONSTRAINT leave_records_balance_fk FOREIGN KEY (
        tenant_id, balance_id, employee_id, leave_type_id, entitlement_year
    ) REFERENCES leave_balances (tenant_id, id, employee_id, leave_type_id, entitlement_year),
    CONSTRAINT leave_records_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT leave_records_matched_fk FOREIGN KEY (tenant_id, matched_record_id)
        REFERENCES leave_records (tenant_id, id) DEFERRABLE INITIALLY DEFERRED
);

CREATE INDEX leave_records_employee_interval_idx
ON leave_records (tenant_id, employee_id, start_at, end_at);
CREATE INDEX leave_records_balance_idx ON leave_records (tenant_id, balance_id, event_date DESC);
CREATE UNIQUE INDEX leave_records_ehrms_match_idx
ON leave_records (tenant_id, matched_record_id)
WHERE source = 'ehrms' AND matched_record_id IS NOT NULL;
CREATE UNIQUE INDEX leave_records_tenant_source_external_ref_idx
ON leave_records (tenant_id, source, external_ref)
WHERE external_ref <> '';

CREATE OR REPLACE FUNCTION enforce_leave_record_match_source()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.matched_record_id IS NOT NULL AND NOT EXISTS (
        SELECT 1
        FROM leave_records nexus
        WHERE nexus.tenant_id = NEW.tenant_id
          AND nexus.id = NEW.matched_record_id
          AND nexus.source = 'nexus'
    ) THEN
        RAISE EXCEPTION 'matched_record_id must reference a Nexus leave record'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE CONSTRAINT TRIGGER leave_records_match_source_check
AFTER INSERT OR UPDATE OF tenant_id, source, matched_record_id ON leave_records
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION enforce_leave_record_match_source();

COMMENT ON TABLE leave_records IS '统一休假记录；Nexus 申请和 eHRMS 同步记录共用此表';
COMMENT ON COLUMN leave_records.id IS '休假记录主键；Nexus 记录与 leave request 使用同一 ID';
COMMENT ON COLUMN leave_records.tenant_id IS '租户 ID';
COMMENT ON COLUMN leave_records.employee_id IS '员工 ID';
COMMENT ON COLUMN leave_records.leave_type_id IS '假别代码；Nexus 与 eHRMS 使用同一套代码';
COMMENT ON COLUMN leave_records.balance_id IS '匹配到的年度余额 ID；EHRMS 明细允许暂时为空';
COMMENT ON COLUMN leave_records.entitlement_year IS '额度所属年度；未匹配余额时取开始日期年度';
COMMENT ON COLUMN leave_records.source IS '记录来源：nexus / ehrms';
COMMENT ON COLUMN leave_records.external_ref IS '上游请假明细稳定标识';
COMMENT ON COLUMN leave_records.event_date IS '来源记录创建时间';
COMMENT ON COLUMN leave_records.start_at IS '请假开始时间';
COMMENT ON COLUMN leave_records.end_at IS '请假结束时间';
COMMENT ON COLUMN leave_records.net_minutes IS '实际请假分钟数';
COMMENT ON COLUMN leave_records.remark IS '请假说明';
COMMENT ON COLUMN leave_records.status IS '记录状态：pending / active / cancelled / corrected';
COMMENT ON COLUMN leave_records.matched_record_id IS '一对一匹配的 Nexus 记录 ID；仅 eHRMS 记录填写';
COMMENT ON COLUMN leave_records.reconciliation_status IS '双来源核对状态';
COMMENT ON COLUMN leave_records.balance_match_status IS '年度余额关联状态：matched / unmatched';
COMMENT ON COLUMN leave_records.balance_match_reason IS '未匹配年度余额的机器可读原因';
COMMENT ON COLUMN leave_records.source_payload IS '最近一次上游请假明细原始字段快照';
COMMENT ON COLUMN leave_records.source_updated_at IS '上游请假明细记录更新时间';
COMMENT ON COLUMN leave_records.last_seen_at IS 'eHRMS 最近同步看到该记录的时间';
COMMENT ON COLUMN leave_records.deleted_at IS 'eHRMS 记录被上游移除的时间';
COMMENT ON COLUMN leave_records.updated_at IS '更新时间';

CREATE TABLE leave_balance_entries (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    balance_id text NOT NULL,
    leave_record_id text,
    employee_id text NOT NULL,
    leave_type_id text NOT NULL,
    entitlement_year integer NOT NULL,
    entry_type text NOT NULL CHECK (entry_type IN (
        'reserve', 'release', 'local_consume', 'local_refund',
        'external_reconcile', 'external_reversal', 'overtime_credit', 'manual_adjust'
    )),
    amount_minutes integer NOT NULL CHECK (amount_minutes <> 0),
    idempotency_key text NOT NULL CHECK (btrim(idempotency_key) <> ''),
    occurred_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT leave_balance_entries_sign_check CHECK (
        (entry_type IN ('reserve', 'local_consume', 'external_reversal') AND amount_minutes < 0)
        OR (entry_type IN ('release', 'local_refund', 'external_reconcile', 'overtime_credit') AND amount_minutes > 0)
        OR (entry_type = 'manual_adjust' AND amount_minutes <> 0)
    ),
    CONSTRAINT leave_balance_entries_balance_fk FOREIGN KEY (
        tenant_id, balance_id, employee_id, leave_type_id, entitlement_year
    ) REFERENCES leave_balances (tenant_id, id, employee_id, leave_type_id, entitlement_year),
    CONSTRAINT leave_balance_entries_record_fk FOREIGN KEY (
        tenant_id, leave_record_id
    ) REFERENCES leave_records (tenant_id, id),
    CONSTRAINT leave_balance_entries_idempotency_idx UNIQUE (tenant_id, idempotency_key)
);

CREATE INDEX leave_balance_entries_balance_idx ON leave_balance_entries (tenant_id, balance_id, occurred_at, id);
CREATE INDEX leave_balance_entries_record_idx ON leave_balance_entries (tenant_id, leave_record_id, occurred_at, id)
WHERE leave_record_id IS NOT NULL;

COMMENT ON TABLE leave_balance_entries IS '余额变动流水；金额为分钟，追加写入';
COMMENT ON COLUMN leave_balance_entries.id IS '余额流水主键';
COMMENT ON COLUMN leave_balance_entries.tenant_id IS '租户 ID';
COMMENT ON COLUMN leave_balance_entries.balance_id IS '被调整的年度余额 ID';
COMMENT ON COLUMN leave_balance_entries.leave_record_id IS '关联休假记录；加班入账和人工调整可为空';
COMMENT ON COLUMN leave_balance_entries.employee_id IS '员工 ID';
COMMENT ON COLUMN leave_balance_entries.leave_type_id IS '假别代码';
COMMENT ON COLUMN leave_balance_entries.entitlement_year IS '余额所属年度';
COMMENT ON COLUMN leave_balance_entries.entry_type IS '变动类型';
COMMENT ON COLUMN leave_balance_entries.amount_minutes IS '变动分钟数；负数扣减，正数返还或增加';
COMMENT ON COLUMN leave_balance_entries.idempotency_key IS '业务幂等键';
COMMENT ON COLUMN leave_balance_entries.occurred_at IS '业务发生时间';
COMMENT ON COLUMN leave_balance_entries.created_at IS '流水创建时间';

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

CREATE TABLE attendance_clock_records (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    worksite_id text,
    work_date date NOT NULL,
    direction text NOT NULL CHECK (direction IN ('clock_in', 'clock_out')),
    client_event_id text NOT NULL DEFAULT '',
    clocked_at timestamptz NOT NULL,
    latitude double precision CHECK (latitude >= -90 AND latitude <= 90),
    longitude double precision CHECK (longitude >= -180 AND longitude <= 180),
    accuracy_meters double precision CHECK (accuracy_meters >= 0),
    distance_meters double precision CHECK (distance_meters >= 0),
    record_status text NOT NULL CHECK (record_status IN ('accepted', 'abnormal', 'rejected')),
    rejection_reason text NOT NULL DEFAULT '',
    source text NOT NULL CHECK (source IN ('geofence', 'manual_correction')),
    device_id text NOT NULL DEFAULT '',
    device_info jsonb NOT NULL DEFAULT '{}'::jsonb,
    correction_request_id text,
    voided boolean NOT NULL DEFAULT false,
    voided_at timestamptz,
    voided_by_account_id text,
    void_reason text,
    created_at timestamptz NOT NULL,
    CONSTRAINT attendance_clock_records_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT attendance_clock_records_employee_identity_idx UNIQUE (tenant_id, id, employee_id),
    CONSTRAINT attendance_clock_records_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT attendance_clock_records_worksite_fk FOREIGN KEY (tenant_id, worksite_id) REFERENCES attendance_worksites (tenant_id, id),
    CONSTRAINT attendance_clock_records_gps_pair_check CHECK ((latitude IS NULL) = (longitude IS NULL)),
    CONSTRAINT attendance_clock_records_source_shape_check CHECK (
        (source = 'geofence' AND latitude IS NOT NULL AND longitude IS NOT NULL
            AND correction_request_id IS NULL)
        OR (source = 'manual_correction' AND correction_request_id IS NOT NULL)
    ),
    CONSTRAINT attendance_clock_records_void_shape_check CHECK (
        (voided = false AND voided_at IS NULL AND voided_by_account_id IS NULL AND void_reason IS NULL)
        OR (voided = true AND voided_at IS NOT NULL AND voided_by_account_id IS NOT NULL AND btrim(void_reason) <> '')
    )
);

CREATE INDEX attendance_clock_records_tenant_employee_date_idx ON attendance_clock_records (tenant_id, employee_id, work_date DESC);
CREATE INDEX attendance_clock_records_tenant_status_idx ON attendance_clock_records (tenant_id, record_status, clocked_at DESC);
CREATE INDEX attendance_clock_records_effective_boundary_idx ON attendance_clock_records (tenant_id, employee_id, work_date, direction, clocked_at, created_at, id) WHERE record_status = 'accepted' AND voided = false;
CREATE INDEX attendance_clock_records_effective_latest_idx ON attendance_clock_records (tenant_id, employee_id, work_date, clocked_at, created_at, id) WHERE record_status = 'accepted' AND voided = false;
CREATE UNIQUE INDEX attendance_clock_records_client_event_idx ON attendance_clock_records (tenant_id, client_event_id) WHERE client_event_id <> '';

CREATE TABLE attendance_daily_records (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    work_date date NOT NULL,
    source text NOT NULL CHECK (source IN ('local', 'ehrms')),
    scheduled_start_at timestamptz,
    scheduled_end_at timestamptz,
    scheduled_minutes integer NOT NULL DEFAULT 0 CHECK (scheduled_minutes >= 0),
    required_minutes integer NOT NULL DEFAULT 0 CHECK (required_minutes >= 0),
    worked_minutes integer NOT NULL DEFAULT 0 CHECK (worked_minutes >= 0),
    credited_leave_minutes integer NOT NULL DEFAULT 0 CHECK (credited_leave_minutes >= 0),
    overtime_minutes integer NOT NULL DEFAULT 0 CHECK (overtime_minutes >= 0),
    clock_in_at timestamptz,
    clock_out_at timestamptz,
    clock_in_record_id text,
    clock_out_record_id text,
    punch_count integer NOT NULL DEFAULT 0 CHECK (punch_count >= 0),
    day_status text NOT NULL DEFAULT '',
    anomaly_reasons text[] NOT NULL DEFAULT '{}'::text[],
    input_fingerprint text NOT NULL CHECK (btrim(input_fingerprint) <> ''),
    external_ref text NOT NULL DEFAULT '',
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, employee_id, work_date, source),
    CONSTRAINT attendance_daily_records_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT attendance_daily_records_clock_in_fk FOREIGN KEY (tenant_id, clock_in_record_id, employee_id) REFERENCES attendance_clock_records (tenant_id, id, employee_id),
    CONSTRAINT attendance_daily_records_clock_out_fk FOREIGN KEY (tenant_id, clock_out_record_id, employee_id) REFERENCES attendance_clock_records (tenant_id, id, employee_id),
    CONSTRAINT attendance_daily_records_schedule_check CHECK (
        scheduled_start_at IS NULL OR scheduled_end_at IS NULL OR scheduled_end_at > scheduled_start_at
    ),
    CONSTRAINT attendance_daily_records_clock_check CHECK (
        clock_in_at IS NULL OR clock_out_at IS NULL OR clock_out_at >= clock_in_at
    )
);

CREATE INDEX attendance_daily_records_tenant_source_date_idx
ON attendance_daily_records (tenant_id, source, work_date DESC, employee_id);

CREATE UNIQUE INDEX attendance_daily_records_external_ref_idx
ON attendance_daily_records (tenant_id, external_ref)
WHERE source = 'ehrms' AND external_ref <> '';

CREATE TABLE attendance_daily_leave_segments (
    tenant_id text NOT NULL,
    employee_id text NOT NULL,
    work_date date NOT NULL,
    daily_source text NOT NULL DEFAULT 'ehrms' CHECK (daily_source = 'ehrms'),
    segment_no smallint NOT NULL CHECK (segment_no > 0),
    leave_type_id text,
    source_leave_type text NOT NULL DEFAULT '',
    start_at timestamptz,
    end_at timestamptz,
    minutes integer NOT NULL DEFAULT 0 CHECK (minutes >= 0),
    counted boolean NOT NULL DEFAULT false,
    time_inferred boolean NOT NULL DEFAULT false,
    leave_record_id text,
    link_status text NOT NULL DEFAULT 'unmatched' CHECK (
        link_status IN ('unmatched', 'matched', 'mismatch', 'ambiguous')
    ),
    match_basis text NOT NULL DEFAULT '',
    candidate_record_ids text[] NOT NULL DEFAULT '{}'::text[],
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, employee_id, work_date, daily_source, segment_no),
    CONSTRAINT attendance_daily_leave_segments_daily_record_fk FOREIGN KEY (
        tenant_id, employee_id, work_date, daily_source
    ) REFERENCES attendance_daily_records (tenant_id, employee_id, work_date, source) ON DELETE CASCADE,
    CONSTRAINT attendance_daily_leave_segments_type_fk FOREIGN KEY (
        tenant_id, leave_type_id
    ) REFERENCES leave_types (tenant_id, id),
    CONSTRAINT attendance_daily_leave_segments_record_fk FOREIGN KEY (
        tenant_id, leave_record_id
    ) REFERENCES leave_records (tenant_id, id),
    CONSTRAINT attendance_daily_leave_segments_interval_check CHECK (
        start_at IS NULL OR end_at IS NULL OR end_at > start_at
    ),
    CONSTRAINT attendance_daily_leave_segments_match_check CHECK (
        (link_status = 'matched' AND leave_record_id IS NOT NULL)
        OR (link_status <> 'matched' AND leave_record_id IS NULL)
    )
);

CREATE INDEX attendance_daily_leave_segments_record_idx
ON attendance_daily_leave_segments (tenant_id, leave_record_id)
WHERE leave_record_id IS NOT NULL;

CREATE INDEX attendance_daily_leave_segments_unmatched_idx
ON attendance_daily_leave_segments (tenant_id, link_status, work_date, employee_id)
WHERE link_status <> 'matched';

CREATE TABLE attendance_daily_reconciliations (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    work_date date NOT NULL,
    local_fingerprint text NOT NULL DEFAULT '',
    ehrms_fingerprint text NOT NULL DEFAULT '',
    status text NOT NULL CHECK (status IN ('matched', 'mismatch', 'local_only', 'ehrms_only')),
    differences jsonb NOT NULL DEFAULT '{}'::jsonb,
    resolution_status text NOT NULL DEFAULT 'unresolved' CHECK (
        resolution_status IN ('unresolved', 'accepted_local', 'accepted_ehrms', 'ignored')
    ),
    resolved_by_account_id text,
    resolved_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, employee_id, work_date),
    CONSTRAINT attendance_daily_reconciliations_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT attendance_daily_reconciliations_resolver_fk FOREIGN KEY (tenant_id, resolved_by_account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT attendance_daily_reconciliations_resolution_check CHECK (
        (resolution_status = 'unresolved' AND resolved_by_account_id IS NULL AND resolved_at IS NULL)
        OR (resolution_status <> 'unresolved' AND resolved_by_account_id IS NOT NULL AND resolved_at IS NOT NULL)
    )
);

CREATE INDEX attendance_daily_reconciliations_status_idx
ON attendance_daily_reconciliations (tenant_id, status, work_date DESC, employee_id);

CREATE TABLE attendance_day_projections (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    work_date date NOT NULL,
    policy_version integer NOT NULL CHECK (policy_version > 0),
    scheduled_start_at timestamptz,
    scheduled_end_at timestamptz,
    clock_in_record_id text,
    clock_out_record_id text,
    last_punch_record_id text,
    punch_count integer NOT NULL DEFAULT 0 CHECK (punch_count >= 0),
    required_minutes integer NOT NULL DEFAULT 0 CHECK (required_minutes >= 0),
    worked_minutes integer NOT NULL DEFAULT 0 CHECK (worked_minutes >= 0),
    approved_leave_minutes integer NOT NULL DEFAULT 0 CHECK (approved_leave_minutes >= 0),
    pending_leave_minutes integer NOT NULL DEFAULT 0 CHECK (pending_leave_minutes >= 0),
    overtime_minutes integer NOT NULL DEFAULT 0 CHECK (overtime_minutes >= 0),
    day_status text NOT NULL CHECK (day_status IN (
        'not_started', 'working', 'complete', 'pending_leave', 'abnormal'
    )),
    anomaly_reasons text[] NOT NULL DEFAULT '{}'::text[],
    input_fingerprint text NOT NULL CHECK (btrim(input_fingerprint) <> ''),
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    computed_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, employee_id, work_date),
    CONSTRAINT attendance_day_projections_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT attendance_day_projections_policy_fk FOREIGN KEY (tenant_id, policy_version) REFERENCES attendance_policy_versions (tenant_id, version),
    CONSTRAINT attendance_day_projections_clock_in_fk FOREIGN KEY (tenant_id, clock_in_record_id, employee_id) REFERENCES attendance_clock_records (tenant_id, id, employee_id),
    CONSTRAINT attendance_day_projections_clock_out_fk FOREIGN KEY (tenant_id, clock_out_record_id, employee_id) REFERENCES attendance_clock_records (tenant_id, id, employee_id),
    CONSTRAINT attendance_day_projections_last_punch_fk FOREIGN KEY (tenant_id, last_punch_record_id, employee_id) REFERENCES attendance_clock_records (tenant_id, id, employee_id),
    CONSTRAINT attendance_day_projections_schedule_check CHECK (
        scheduled_start_at IS NULL OR scheduled_end_at IS NULL OR scheduled_end_at > scheduled_start_at
    )
);

CREATE INDEX attendance_day_projections_tenant_date_status_idx
ON attendance_day_projections (tenant_id, work_date, day_status, employee_id);

ALTER TABLE attendance_clock_records
    ADD CONSTRAINT attendance_clock_records_correction_fk
    FOREIGN KEY (tenant_id, correction_request_id, employee_id)
    REFERENCES form_business_records (tenant_id, id, subject_employee_id);

ALTER TABLE attendance_clock_records
    ADD CONSTRAINT attendance_clock_records_voided_by_fk
    FOREIGN KEY (tenant_id, voided_by_account_id)
    REFERENCES accounts (tenant_id, id);

ALTER TABLE leave_balance_entries
    ADD CONSTRAINT leave_balance_entries_overtime_request_fk
    FOREIGN KEY (tenant_id, overtime_request_id, employee_id)
    REFERENCES form_business_records (tenant_id, id, subject_employee_id);

CREATE INDEX leave_balance_entries_overtime_request_idx ON leave_balance_entries (tenant_id, overtime_request_id) WHERE overtime_request_id IS NOT NULL;

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


CREATE TABLE model_connections (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    provider text NOT NULL DEFAULT 'openai',
    upstream_model text NOT NULL,
    api_base_url text NOT NULL DEFAULT '',
    api_key_ciphertext text NOT NULL DEFAULT '',
    api_key_preview text NOT NULL DEFAULT '',
    rate_limit_rpm integer NOT NULL DEFAULT 0 CHECK (rate_limit_rpm >= 0),
    timeout_ms integer NOT NULL DEFAULT 60000 CHECK (timeout_ms > 0),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'archived')),
    created_by_account_id text,
    updated_by_account_id text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    archived_at timestamptz,
    CONSTRAINT model_connections_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT model_connections_created_by_fk FOREIGN KEY (tenant_id, created_by_account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT model_connections_updated_by_fk FOREIGN KEY (tenant_id, updated_by_account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT model_connections_archived_at_check CHECK (
        (status <> 'archived' AND archived_at IS NULL) OR
        (status = 'archived' AND archived_at IS NOT NULL)
    )
);

CREATE INDEX model_connections_tenant_status_idx ON model_connections (tenant_id, status, updated_at DESC, id);
CREATE INDEX model_connections_tenant_name_idx ON model_connections (tenant_id, name);

CREATE TABLE model_connection_state (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    model_connection_id text NOT NULL,
    sync_status text NOT NULL DEFAULT 'pending' CHECK (sync_status IN ('pending', 'synced', 'failed')),
    synced_config_checksum text NOT NULL DEFAULT '',
    last_synced_at timestamptz,
    last_sync_error text NOT NULL DEFAULT '',
    last_tested_at timestamptz,
    last_test_status text NOT NULL DEFAULT 'untested' CHECK (last_test_status IN ('ok', 'failed', 'untested')),
    last_test_message text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, model_connection_id),
    CONSTRAINT model_connection_state_connection_fk FOREIGN KEY (tenant_id, model_connection_id) REFERENCES model_connections (tenant_id, id) ON DELETE CASCADE
);

CREATE TABLE external_tool_connections (
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
    timeout_ms integer NOT NULL DEFAULT 30000 CHECK (timeout_ms BETWEEN 1000 AND 120000),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'archived')),
    last_tested_at timestamptz,
    last_test_status text NOT NULL DEFAULT 'untested' CHECK (last_test_status IN ('ok', 'failed', 'untested')),
    last_test_message text NOT NULL DEFAULT '',
    created_by_account_id text,
    updated_by_account_id text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    archived_at timestamptz,
    CONSTRAINT external_tool_connections_transport_kind_check CHECK (
        (kind = 'mcp' AND transport IN ('sse', 'streamable_http')) OR
        (kind = 'http' AND transport = 'http')
    ),
    CONSTRAINT external_tool_connections_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT external_tool_connections_created_by_fk FOREIGN KEY (tenant_id, created_by_account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT external_tool_connections_updated_by_fk FOREIGN KEY (tenant_id, updated_by_account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT external_tool_connections_archived_at_check CHECK (
        (status <> 'archived' AND archived_at IS NULL) OR
        (status = 'archived' AND archived_at IS NOT NULL)
    )
);

CREATE INDEX external_tool_connections_tenant_status_idx ON external_tool_connections (tenant_id, status, updated_at DESC, id);

CREATE TABLE external_tools (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    connection_id text NOT NULL,
    tool_name text NOT NULL,
    description text NOT NULL DEFAULT '',
    http_method text NOT NULL DEFAULT '' CHECK (http_method IN ('', 'GET', 'POST', 'PUT', 'PATCH', 'DELETE')),
    http_path text NOT NULL DEFAULT '',
    input_schema jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(input_schema) = 'object'),
    output_schema jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(output_schema) = 'object'),
    readonly boolean NOT NULL DEFAULT false,
    enabled boolean NOT NULL DEFAULT true,
    schema_checksum text NOT NULL DEFAULT '',
    discovered_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    archived_at timestamptz,
    CONSTRAINT external_tools_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT external_tools_connection_name_idx UNIQUE (tenant_id, connection_id, tool_name),
    CONSTRAINT external_tools_connection_fk FOREIGN KEY (tenant_id, connection_id) REFERENCES external_tool_connections (tenant_id, id) ON DELETE RESTRICT
);

CREATE INDEX external_tools_tenant_connection_idx ON external_tools (tenant_id, connection_id, enabled, tool_name) WHERE archived_at IS NULL;

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
CREATE INDEX knowledge_bases_tenant_created_idx ON knowledge_bases (tenant_id, created_at DESC, id);

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
CREATE INDEX knowledge_documents_base_created_idx ON knowledge_documents (tenant_id, knowledge_base_id, created_at DESC, id);

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

CREATE TABLE agents (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    parent_agent_id text,
    lifecycle_status text NOT NULL DEFAULT 'active' CHECK (lifecycle_status IN ('active', 'archived')),
    draft_revision_id text,
    published_revision_id text,
    next_revision_no integer NOT NULL DEFAULT 1 CHECK (next_revision_no > 0),
    created_by_account_id text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    archived_at timestamptz,
    CONSTRAINT agents_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT agents_parent_fk FOREIGN KEY (tenant_id, parent_agent_id) REFERENCES agents (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT agents_created_by_fk FOREIGN KEY (tenant_id, created_by_account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT agents_parent_not_self_check CHECK (parent_agent_id IS NULL OR parent_agent_id <> id),
    CONSTRAINT agents_archived_at_check CHECK (
        (lifecycle_status <> 'archived' AND archived_at IS NULL) OR
        (lifecycle_status = 'archived' AND archived_at IS NOT NULL)
    )
);

CREATE INDEX agents_tenant_status_idx ON agents (tenant_id, lifecycle_status, updated_at DESC, id);
CREATE INDEX agents_tenant_parent_idx ON agents (tenant_id, parent_agent_id, id) WHERE parent_agent_id IS NOT NULL;

CREATE TABLE agent_revisions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    agent_id text NOT NULL,
    revision_no integer NOT NULL CHECK (revision_no > 0),
    ordinal integer CHECK (ordinal >= 0),
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    icon text NOT NULL DEFAULT 'AI',
    category text NOT NULL DEFAULT 'workflow' CHECK (category IN ('workflow', 'doc', 'analytics', 'it')),
    visibility text NOT NULL DEFAULT 'all' CHECK (visibility IN ('all', 'department', 'role')),
    visibility_targets jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(visibility_targets) = 'array'),
    main_agent_role text NOT NULL DEFAULT '',
    system_prompt text NOT NULL DEFAULT '',
    welcome_message text NOT NULL DEFAULT '',
    suggested_questions jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(suggested_questions) = 'array'),
    suggested_question_translations jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(suggested_question_translations) = 'array'),
    model_connection_id text NOT NULL,
    model_config_checksum text NOT NULL DEFAULT '',
    timeout_ms integer NOT NULL DEFAULT 60000 CHECK (timeout_ms > 0),
    config_schema_version integer NOT NULL DEFAULT 1 CHECK (config_schema_version > 0),
    checksum text NOT NULL,
    revision_note text NOT NULL DEFAULT '',
    created_by_account_id text,
    created_at timestamptz NOT NULL,
    CONSTRAINT agent_revisions_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT agent_revisions_agent_revision_no_idx UNIQUE (tenant_id, agent_id, revision_no),
    CONSTRAINT agent_revisions_agent_id_id_idx UNIQUE (tenant_id, agent_id, id),
    CONSTRAINT agent_revisions_execution_binding_idx UNIQUE (tenant_id, agent_id, id, model_connection_id),
    CONSTRAINT agent_revisions_agent_fk FOREIGN KEY (tenant_id, agent_id) REFERENCES agents (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT agent_revisions_model_fk FOREIGN KEY (tenant_id, model_connection_id) REFERENCES model_connections (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT agent_revisions_created_by_fk FOREIGN KEY (tenant_id, created_by_account_id) REFERENCES accounts (tenant_id, id)
);

ALTER TABLE agents
    ADD CONSTRAINT agents_draft_revision_fk
    FOREIGN KEY (tenant_id, id, draft_revision_id)
    REFERENCES agent_revisions (tenant_id, agent_id, id) ON DELETE RESTRICT;

ALTER TABLE agents
    ADD CONSTRAINT agents_published_revision_fk
    FOREIGN KEY (tenant_id, id, published_revision_id)
    REFERENCES agent_revisions (tenant_id, agent_id, id) ON DELETE RESTRICT;

CREATE INDEX agent_revisions_agent_idx ON agent_revisions (tenant_id, agent_id, revision_no DESC);
CREATE INDEX agent_revisions_model_idx ON agent_revisions (tenant_id, model_connection_id, created_at DESC);

CREATE TABLE agent_revision_builtin_tools (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    revision_id text NOT NULL,
    tool_key text NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    config jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(config) = 'object'),
    PRIMARY KEY (tenant_id, revision_id, tool_key),
    CONSTRAINT agent_revision_builtin_tools_ordinal_idx UNIQUE (tenant_id, revision_id, ordinal),
    CONSTRAINT agent_revision_builtin_tools_revision_fk FOREIGN KEY (tenant_id, revision_id) REFERENCES agent_revisions (tenant_id, id) ON DELETE CASCADE
);

CREATE TABLE agent_revision_external_tools (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    revision_id text NOT NULL,
    external_tool_id text NOT NULL,
    tool_schema_checksum text NOT NULL DEFAULT '',
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    config jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(config) = 'object'),
    PRIMARY KEY (tenant_id, revision_id, external_tool_id),
    CONSTRAINT agent_revision_external_tools_ordinal_idx UNIQUE (tenant_id, revision_id, ordinal),
    CONSTRAINT agent_revision_external_tools_revision_fk FOREIGN KEY (tenant_id, revision_id) REFERENCES agent_revisions (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT agent_revision_external_tools_tool_fk FOREIGN KEY (tenant_id, external_tool_id) REFERENCES external_tools (tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE agent_revision_knowledge_bases (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    revision_id text NOT NULL,
    knowledge_base_id text NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    PRIMARY KEY (tenant_id, revision_id, knowledge_base_id),
    CONSTRAINT agent_revision_knowledge_bases_ordinal_idx UNIQUE (tenant_id, revision_id, ordinal),
    CONSTRAINT agent_revision_knowledge_bases_revision_fk FOREIGN KEY (tenant_id, revision_id) REFERENCES agent_revisions (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT agent_revision_knowledge_bases_base_fk FOREIGN KEY (tenant_id, knowledge_base_id) REFERENCES knowledge_bases (tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE conversations (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    owner_account_id text NOT NULL,
    agent_id text,
    current_segment_id text,
    next_message_sequence bigint NOT NULL DEFAULT 1 CHECK (next_message_sequence > 0),
    title text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    last_message_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    archived_at timestamptz,
    CONSTRAINT conversations_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT conversations_agent_id_idx UNIQUE (tenant_id, id, agent_id),
    CONSTRAINT conversations_owner_fk FOREIGN KEY (tenant_id, owner_account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT conversations_agent_fk FOREIGN KEY (tenant_id, agent_id) REFERENCES agents (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT conversations_archived_at_check CHECK (
        (status <> 'archived' AND archived_at IS NULL) OR
        (status = 'archived' AND archived_at IS NOT NULL)
    )
);

CREATE TABLE conversation_segments (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    conversation_id text NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal > 0),
    start_reason text NOT NULL DEFAULT 'initial' CHECK (start_reason IN ('initial', 'context_reset')),
    created_at timestamptz NOT NULL,
    CONSTRAINT conversation_segments_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT conversation_segments_conversation_ordinal_idx UNIQUE (tenant_id, conversation_id, ordinal),
    CONSTRAINT conversation_segments_conversation_id_idx UNIQUE (tenant_id, conversation_id, id),
    CONSTRAINT conversation_segments_conversation_fk FOREIGN KEY (tenant_id, conversation_id) REFERENCES conversations (tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE conversations
    ADD CONSTRAINT conversations_current_segment_fk
    FOREIGN KEY (tenant_id, id, current_segment_id)
    REFERENCES conversation_segments (tenant_id, conversation_id, id) ON DELETE RESTRICT;

CREATE INDEX conversations_tenant_owner_status_idx ON conversations (tenant_id, owner_account_id, status, updated_at DESC, id DESC);
CREATE INDEX conversations_tenant_agent_idx ON conversations (tenant_id, agent_id, updated_at DESC, id DESC);

CREATE TABLE conversation_messages (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    conversation_id text NOT NULL,
    segment_id text NOT NULL,
    sequence_no bigint NOT NULL CHECK (sequence_no > 0),
    role text NOT NULL CHECK (role IN ('user', 'assistant', 'system', 'tool')),
    content text NOT NULL DEFAULT '',
    content_json jsonb,
    execution_id text,
    execution_step_id text,
    created_at timestamptz NOT NULL,
    CONSTRAINT conversation_messages_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT conversation_messages_conversation_segment_id_idx UNIQUE (tenant_id, conversation_id, segment_id, id),
    CONSTRAINT conversation_messages_conversation_sequence_idx UNIQUE (tenant_id, conversation_id, sequence_no),
    CONSTRAINT conversation_messages_segment_fk FOREIGN KEY (tenant_id, conversation_id, segment_id) REFERENCES conversation_segments (tenant_id, conversation_id, id) ON DELETE CASCADE,
    CONSTRAINT conversation_messages_execution_step_requires_execution CHECK (execution_step_id IS NULL OR execution_id IS NOT NULL)
);

CREATE INDEX conversation_messages_conversation_segment_sequence_idx ON conversation_messages (tenant_id, conversation_id, segment_id, sequence_no ASC);

CREATE TABLE conversation_executions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id text NOT NULL,
    conversation_id text NOT NULL,
    segment_id text NOT NULL,
    input_message_id text NOT NULL,
    agent_id text,
    agent_revision_id text,
    model_connection_id text,
    mode text NOT NULL DEFAULT '',
    trigger_type text NOT NULL DEFAULT 'chat' CHECK (trigger_type IN ('chat', 'api', 'trial', 'system')),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled')),
    queued_at timestamptz NOT NULL,
    started_at timestamptz,
    completed_at timestamptz,
    error_code text NOT NULL DEFAULT '',
    error_category text NOT NULL DEFAULT '',
    safe_error_message text NOT NULL DEFAULT '',
    llm_call_count bigint NOT NULL DEFAULT 0 CHECK (llm_call_count >= 0),
    input_tokens bigint NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    cached_tokens bigint NOT NULL DEFAULT 0 CHECK (cached_tokens >= 0),
    output_tokens bigint NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    total_tokens bigint NOT NULL DEFAULT 0 CHECK (total_tokens >= 0),
    usage_complete boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT conversation_executions_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT conversation_executions_conversation_segment_id_idx UNIQUE (tenant_id, conversation_id, segment_id, id),
    CONSTRAINT conversation_executions_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT conversation_executions_conversation_agent_fk FOREIGN KEY (tenant_id, conversation_id, agent_id) REFERENCES conversations (tenant_id, id, agent_id) ON DELETE RESTRICT,
    CONSTRAINT conversation_executions_segment_fk FOREIGN KEY (tenant_id, conversation_id, segment_id) REFERENCES conversation_segments (tenant_id, conversation_id, id) ON DELETE RESTRICT,
    CONSTRAINT conversation_executions_input_message_fk FOREIGN KEY (tenant_id, conversation_id, segment_id, input_message_id) REFERENCES conversation_messages (tenant_id, conversation_id, segment_id, id) ON DELETE RESTRICT,
    CONSTRAINT conversation_executions_agent_revision_fk FOREIGN KEY (tenant_id, agent_id, agent_revision_id, model_connection_id) REFERENCES agent_revisions (tenant_id, agent_id, id, model_connection_id) ON DELETE RESTRICT,
    CONSTRAINT conversation_executions_model_fk FOREIGN KEY (tenant_id, model_connection_id) REFERENCES model_connections (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT conversation_executions_agent_binding_check CHECK (
        (agent_id IS NULL AND agent_revision_id IS NULL AND model_connection_id IS NULL) OR
        (agent_id IS NOT NULL AND agent_revision_id IS NOT NULL AND model_connection_id IS NOT NULL)
    ),
    CONSTRAINT conversation_executions_cached_tokens_lte_input_check CHECK (cached_tokens <= input_tokens),
    CONSTRAINT conversation_executions_timestamps_check CHECK (
        (status = 'queued' AND started_at IS NULL AND completed_at IS NULL) OR
        (status = 'running' AND started_at IS NOT NULL AND completed_at IS NULL) OR
        (status IN ('completed', 'failed', 'cancelled') AND completed_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX conversation_executions_active_conversation_unique
    ON conversation_executions (tenant_id, conversation_id)
    WHERE status IN ('queued', 'running');
CREATE INDEX conversation_executions_tenant_account_created_idx ON conversation_executions (tenant_id, account_id, created_at DESC, id DESC);
CREATE INDEX conversation_executions_tenant_revision_created_idx ON conversation_executions (tenant_id, agent_revision_id, created_at DESC, id DESC);

CREATE TABLE conversation_execution_steps (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    execution_id text NOT NULL,
    parent_step_id text,
    sequence_no integer NOT NULL CHECK (sequence_no > 0),
    step_type text NOT NULL CHECK (step_type IN ('llm', 'tool', 'sub_agent', 'retrieval')),
    name text NOT NULL DEFAULT '',
    model_connection_id text,
    external_tool_id text,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled')),
    input_summary jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(input_summary) = 'object'),
    output_summary jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(output_summary) = 'object'),
    input_tokens bigint NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    cached_tokens bigint NOT NULL DEFAULT 0 CHECK (cached_tokens >= 0),
    output_tokens bigint NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    started_at timestamptz,
    completed_at timestamptz,
    error_code text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    CONSTRAINT conversation_execution_steps_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT conversation_execution_steps_execution_id_idx UNIQUE (tenant_id, execution_id, id),
    CONSTRAINT conversation_execution_steps_execution_sequence_idx UNIQUE (tenant_id, execution_id, sequence_no),
    CONSTRAINT conversation_execution_steps_execution_fk FOREIGN KEY (tenant_id, execution_id) REFERENCES conversation_executions (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT conversation_execution_steps_parent_fk FOREIGN KEY (tenant_id, execution_id, parent_step_id) REFERENCES conversation_execution_steps (tenant_id, execution_id, id) ON DELETE RESTRICT,
    CONSTRAINT conversation_execution_steps_model_fk FOREIGN KEY (tenant_id, model_connection_id) REFERENCES model_connections (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT conversation_execution_steps_external_tool_fk FOREIGN KEY (tenant_id, external_tool_id) REFERENCES external_tools (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT conversation_execution_steps_cached_tokens_lte_input_check CHECK (cached_tokens <= input_tokens)
);

ALTER TABLE conversation_messages
    ADD CONSTRAINT conversation_messages_execution_fk
    FOREIGN KEY (tenant_id, conversation_id, segment_id, execution_id)
    REFERENCES conversation_executions (tenant_id, conversation_id, segment_id, id) ON DELETE RESTRICT;

ALTER TABLE conversation_messages
    ADD CONSTRAINT conversation_messages_execution_step_fk
    FOREIGN KEY (tenant_id, execution_id, execution_step_id)
    REFERENCES conversation_execution_steps (tenant_id, execution_id, id) ON DELETE RESTRICT;

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

CREATE TABLE conversation_files (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    conversation_id text NOT NULL,
    segment_id text NOT NULL,
    file_asset_id text NOT NULL,
    message_id text,
    ordinal integer,
    state text NOT NULL DEFAULT 'draft' CHECK (state IN ('draft', 'attached')),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT conversation_files_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT conversation_files_conversation_segment_id_idx UNIQUE (tenant_id, conversation_id, segment_id, id),
    CONSTRAINT conversation_files_asset_idx UNIQUE (tenant_id, conversation_id, segment_id, file_asset_id),
    CONSTRAINT conversation_files_segment_fk FOREIGN KEY (tenant_id, conversation_id, segment_id) REFERENCES conversation_segments (tenant_id, conversation_id, id) ON DELETE CASCADE,
    CONSTRAINT conversation_files_asset_fk FOREIGN KEY (tenant_id, file_asset_id) REFERENCES file_assets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT conversation_files_message_fk FOREIGN KEY (tenant_id, conversation_id, segment_id, message_id) REFERENCES conversation_messages (tenant_id, conversation_id, segment_id, id) ON DELETE RESTRICT,
    CONSTRAINT conversation_files_ordinal_check CHECK (ordinal IS NULL OR ordinal >= 0),
    CONSTRAINT conversation_files_attachment_state_check CHECK (
        (state = 'draft' AND message_id IS NULL AND ordinal IS NULL) OR
        (state = 'attached' AND message_id IS NOT NULL AND ordinal IS NOT NULL)
    )
);

CREATE INDEX conversation_files_segment_idx
    ON conversation_files (tenant_id, conversation_id, segment_id, state, created_at ASC, id ASC);
CREATE UNIQUE INDEX conversation_files_message_ordinal_idx
    ON conversation_files (tenant_id, message_id, ordinal)
    WHERE message_id IS NOT NULL;
CREATE INDEX conversation_files_message_idx
    ON conversation_files (tenant_id, conversation_id, segment_id, message_id, ordinal ASC)
    WHERE message_id IS NOT NULL;

CREATE TABLE form_instance_files (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    form_instance_id text NOT NULL,
    file_id text NOT NULL,
    field_id text NOT NULL,
    state text NOT NULL DEFAULT 'draft' CHECK (state IN ('draft', 'attached')),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, form_instance_id, file_id),
    CONSTRAINT form_instance_files_instance_fk
        FOREIGN KEY (tenant_id, form_instance_id)
        REFERENCES form_instances (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT form_instance_files_file_fk
        FOREIGN KEY (tenant_id, file_id)
        REFERENCES file_assets (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX form_instance_files_field_idx
    ON form_instance_files (tenant_id, form_instance_id, field_id, created_at ASC, file_id ASC);

CREATE TABLE agent_memories (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id text NOT NULL,
    scope_type text NOT NULL CHECK (scope_type IN ('global', 'agent', 'conversation')),
    agent_id text,
    conversation_id text,
    segment_id text,
    key text NOT NULL,
    content text NOT NULL,
    source_type text NOT NULL DEFAULT 'extracted' CHECK (source_type IN ('manual', 'extracted')),
    source_message_id text,
    confidence numeric(5,4) NOT NULL DEFAULT 1 CHECK (confidence >= 0 AND confidence <= 1),
    importance integer NOT NULL DEFAULT 1 CHECK (importance >= 1 AND importance <= 5),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'superseded')),
    expires_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT agent_memories_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT agent_memories_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT agent_memories_agent_fk FOREIGN KEY (tenant_id, agent_id) REFERENCES agents (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT agent_memories_conversation_segment_fk FOREIGN KEY (tenant_id, conversation_id, segment_id) REFERENCES conversation_segments (tenant_id, conversation_id, id) ON DELETE RESTRICT,
    CONSTRAINT agent_memories_source_message_fk FOREIGN KEY (tenant_id, source_message_id) REFERENCES conversation_messages (tenant_id, id) ON DELETE SET NULL (source_message_id),
    CONSTRAINT agent_memories_scope_check CHECK (
        (scope_type = 'global' AND agent_id IS NULL AND conversation_id IS NULL AND segment_id IS NULL) OR
        (scope_type = 'agent' AND agent_id IS NOT NULL AND conversation_id IS NULL AND segment_id IS NULL) OR
        (scope_type = 'conversation' AND agent_id IS NULL AND conversation_id IS NOT NULL AND segment_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX agent_memories_active_scope_key_idx
    ON agent_memories (tenant_id, account_id, scope_type, agent_id, conversation_id, segment_id, key) NULLS NOT DISTINCT
    WHERE status = 'active';
CREATE INDEX agent_memories_tenant_account_idx
    ON agent_memories (tenant_id, account_id, scope_type, importance DESC, updated_at DESC, id DESC)
    WHERE status = 'active';

CREATE TABLE agent_confirmations (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id text NOT NULL,
    conversation_id text NOT NULL,
    segment_id text NOT NULL,
    execution_id text,
    source_message_id text,
    kind text NOT NULL,
    title text NOT NULL,
    action text NOT NULL,
    public_payload jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(public_payload) = 'object'),
    action_payload jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(action_payload) = 'object'),
    result_payload jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(result_payload) = 'object'),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'executing', 'completed', 'failed', 'cancelled', 'expired')),
    last_error text NOT NULL DEFAULT '',
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT agent_confirmations_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT agent_confirmations_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT agent_confirmations_segment_fk FOREIGN KEY (tenant_id, conversation_id, segment_id) REFERENCES conversation_segments (tenant_id, conversation_id, id) ON DELETE RESTRICT,
    CONSTRAINT agent_confirmations_execution_fk FOREIGN KEY (tenant_id, conversation_id, segment_id, execution_id) REFERENCES conversation_executions (tenant_id, conversation_id, segment_id, id) ON DELETE RESTRICT,
    CONSTRAINT agent_confirmations_source_message_fk FOREIGN KEY (tenant_id, conversation_id, segment_id, source_message_id) REFERENCES conversation_messages (tenant_id, conversation_id, segment_id, id) ON DELETE RESTRICT
);

CREATE INDEX agent_confirmations_pending_idx
    ON agent_confirmations (tenant_id, account_id, conversation_id, segment_id, expires_at ASC, id)
    WHERE status = 'pending';

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
    payload_version integer NOT NULL DEFAULT 1 CHECK (payload_version > 0),
    idempotency_key text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'succeeded', 'failed', 'parked', 'dead_lettered')),
    retry_count integer NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    max_attempts integer NOT NULL DEFAULT 5 CHECK (max_attempts >= 0),
    last_error text NOT NULL DEFAULT '',
    next_attempt_at timestamptz NOT NULL DEFAULT NOW(),
    claim_owner text NOT NULL DEFAULT '',
    claim_token text NOT NULL DEFAULT '',
    claim_expires_at timestamptz,
    last_attempt_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    processed_at timestamptz,
    dead_lettered_at timestamptz
);

CREATE INDEX outbox_events_tenant_status_idx ON outbox_events (tenant_id, status, created_at);
CREATE INDEX outbox_events_dispatch_due_idx ON outbox_events (tenant_id, next_attempt_at, created_at, id) WHERE status IN ('pending', 'failed');
CREATE INDEX outbox_events_expired_claim_idx ON outbox_events (tenant_id, claim_expires_at, created_at, id) WHERE status = 'processing';
CREATE UNIQUE INDEX outbox_events_idempotency_idx ON outbox_events (tenant_id, event_type, idempotency_key) WHERE idempotency_key <> '';

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
ALTER TABLE attendance_policy_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_policy_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_types ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_types FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_balances ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_balances FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_balance_entries ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_balance_entries FORCE ROW LEVEL SECURITY;
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
ALTER TABLE form_business_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE form_business_records FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_records FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_worksites ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_worksites FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_clock_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_clock_records FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_daily_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_daily_records FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_daily_leave_segments ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_daily_leave_segments FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_daily_reconciliations ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_daily_reconciliations FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_day_projections ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_day_projections FORCE ROW LEVEL SECURITY;
ALTER TABLE platform_task_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform_task_items FORCE ROW LEVEL SECURITY;
ALTER TABLE platform_task_todos ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform_task_todos FORCE ROW LEVEL SECURITY;
ALTER TABLE model_connections ENABLE ROW LEVEL SECURITY;
ALTER TABLE model_connections FORCE ROW LEVEL SECURITY;
ALTER TABLE model_connection_state ENABLE ROW LEVEL SECURITY;
ALTER TABLE model_connection_state FORCE ROW LEVEL SECURITY;
ALTER TABLE external_tool_connections ENABLE ROW LEVEL SECURITY;
ALTER TABLE external_tool_connections FORCE ROW LEVEL SECURITY;
ALTER TABLE external_tools ENABLE ROW LEVEL SECURITY;
ALTER TABLE external_tools FORCE ROW LEVEL SECURITY;
ALTER TABLE knowledge_bases ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_bases FORCE ROW LEVEL SECURITY;
ALTER TABLE knowledge_documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_documents FORCE ROW LEVEL SECURITY;
ALTER TABLE knowledge_document_chunks ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_document_chunks FORCE ROW LEVEL SECURITY;
ALTER TABLE agents ENABLE ROW LEVEL SECURITY;
ALTER TABLE agents FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_revisions ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_revisions FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_revision_builtin_tools ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_revision_builtin_tools FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_revision_external_tools ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_revision_external_tools FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_revision_knowledge_bases ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_revision_knowledge_bases FORCE ROW LEVEL SECURITY;
ALTER TABLE conversations ENABLE ROW LEVEL SECURITY;
ALTER TABLE conversations FORCE ROW LEVEL SECURITY;
ALTER TABLE conversation_segments ENABLE ROW LEVEL SECURITY;
ALTER TABLE conversation_segments FORCE ROW LEVEL SECURITY;
ALTER TABLE conversation_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE conversation_messages FORCE ROW LEVEL SECURITY;
ALTER TABLE conversation_executions ENABLE ROW LEVEL SECURITY;
ALTER TABLE conversation_executions FORCE ROW LEVEL SECURITY;
ALTER TABLE conversation_execution_steps ENABLE ROW LEVEL SECURITY;
ALTER TABLE conversation_execution_steps FORCE ROW LEVEL SECURITY;
ALTER TABLE file_assets ENABLE ROW LEVEL SECURITY;
ALTER TABLE file_assets FORCE ROW LEVEL SECURITY;
ALTER TABLE file_chunks ENABLE ROW LEVEL SECURITY;
ALTER TABLE file_chunks FORCE ROW LEVEL SECURITY;
ALTER TABLE conversation_files ENABLE ROW LEVEL SECURITY;
ALTER TABLE conversation_files FORCE ROW LEVEL SECURITY;
ALTER TABLE form_instance_files ENABLE ROW LEVEL SECURITY;
ALTER TABLE form_instance_files FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_memories ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_memories FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_confirmations ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_confirmations FORCE ROW LEVEL SECURITY;

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
CREATE POLICY tenant_isolation_attendance_policy_versions ON attendance_policy_versions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_types ON leave_types USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_balances ON leave_balances USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_balance_entries ON leave_balance_entries USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_form_definition_drafts ON form_definition_drafts USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_form_templates ON form_templates USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_form_template_versions ON form_template_versions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_form_instances ON form_instances USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_form_instance_field_values ON form_instance_field_values USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_workflow_runs ON workflow_runs USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_workflow_stage_instances ON workflow_stage_instances USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_workflow_stage_assignees ON workflow_stage_assignees USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_workflow_actions ON workflow_actions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_form_business_records ON form_business_records USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_records ON leave_records USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_worksites ON attendance_worksites USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_clock_records ON attendance_clock_records USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_daily_records ON attendance_daily_records USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_daily_leave_segments ON attendance_daily_leave_segments USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_daily_reconciliations ON attendance_daily_reconciliations USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_day_projections ON attendance_day_projections USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_platform_task_items ON platform_task_items USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_platform_task_todos ON platform_task_todos USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_model_connections ON model_connections USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_model_connection_state ON model_connection_state USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_external_tool_connections ON external_tool_connections USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_external_tools ON external_tools USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_knowledge_bases ON knowledge_bases USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_knowledge_documents ON knowledge_documents USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_knowledge_document_chunks ON knowledge_document_chunks USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agents ON agents USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_revisions ON agent_revisions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_revision_builtin_tools ON agent_revision_builtin_tools USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_revision_external_tools ON agent_revision_external_tools USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_revision_knowledge_bases ON agent_revision_knowledge_bases USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_conversations ON conversations USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_conversation_segments ON conversation_segments USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_conversation_messages ON conversation_messages USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_conversation_executions ON conversation_executions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_conversation_execution_steps ON conversation_execution_steps USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_file_assets ON file_assets USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_file_chunks ON file_chunks USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_conversation_files ON conversation_files USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_form_instance_files ON form_instance_files USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_memories ON agent_memories USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_confirmations ON agent_confirmations USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

CREATE POLICY tenant_isolation_notifications ON notifications USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_notification_recipients ON notification_recipients USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_outbox_events ON outbox_events USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_audit_logs ON audit_logs USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
