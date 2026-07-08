-- +goose Up

ALTER TABLE permission_sets
    ADD COLUMN source_template_key text NOT NULL DEFAULT '',
    ADD COLUMN source_package_version text NOT NULL DEFAULT '';

ALTER TABLE user_groups
    ADD COLUMN source_template_key text NOT NULL DEFAULT '',
    ADD COLUMN source_package_version text NOT NULL DEFAULT '';

ALTER TABLE assumable_roles
    ADD COLUMN source_template_key text NOT NULL DEFAULT '',
    ADD COLUMN source_package_version text NOT NULL DEFAULT '';

CREATE INDEX permission_sets_source_template_idx ON permission_sets (tenant_id, source_template_key) WHERE source_template_key <> '';
CREATE INDEX user_groups_source_template_idx ON user_groups (tenant_id, source_template_key) WHERE source_template_key <> '';
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

ALTER TABLE permission_package_imports ENABLE ROW LEVEL SECURITY;
ALTER TABLE permission_package_imports FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_permission_package_imports ON permission_package_imports USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- +goose Down

DROP TABLE IF EXISTS permission_package_imports;
DROP TABLE IF EXISTS assumable_role_templates;
DROP TABLE IF EXISTS user_group_templates;
DROP TABLE IF EXISTS permission_set_templates;
DROP TABLE IF EXISTS permission_packages;

DROP INDEX IF EXISTS assumable_roles_source_template_idx;
DROP INDEX IF EXISTS user_groups_source_template_idx;
DROP INDEX IF EXISTS permission_sets_source_template_idx;

ALTER TABLE assumable_roles
    DROP COLUMN IF EXISTS source_package_version,
    DROP COLUMN IF EXISTS source_template_key;

ALTER TABLE user_groups
    DROP COLUMN IF EXISTS source_package_version,
    DROP COLUMN IF EXISTS source_template_key;

ALTER TABLE permission_sets
    DROP COLUMN IF EXISTS source_package_version,
    DROP COLUMN IF EXISTS source_template_key;
