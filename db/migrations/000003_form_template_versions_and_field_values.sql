-- +goose Up

ALTER TABLE form_templates
    ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'draft',
    ADD COLUMN IF NOT EXISTS current_version integer NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS updated_at timestamptz,
    ADD COLUMN IF NOT EXISTS deleted_at timestamptz;

UPDATE form_templates
SET status = CASE
        WHEN COALESCE((schema -> 'workspace_design' ->> 'deleted')::boolean, false) THEN 'archived'
        WHEN COALESCE((schema -> 'workspace_design' ->> 'enabled')::boolean, true) THEN 'published'
        ELSE 'draft'
    END,
    current_version = GREATEST(current_version, 1),
    updated_at = COALESCE(updated_at, created_at),
    deleted_at = CASE
        WHEN COALESCE((schema -> 'workspace_design' ->> 'deleted')::boolean, false) THEN COALESCE(updated_at, created_at)
        ELSE deleted_at
    END;

ALTER TABLE form_templates
    ALTER COLUMN updated_at SET NOT NULL,
    ADD CONSTRAINT form_templates_status_check CHECK (status IN ('draft', 'published', 'archived')),
    ADD CONSTRAINT form_templates_current_version_check CHECK (current_version > 0);

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
    CONSTRAINT form_template_versions_template_fk FOREIGN KEY (tenant_id, template_id) REFERENCES form_templates (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT form_template_versions_tenant_template_version_idx UNIQUE (tenant_id, template_id, version)
);

CREATE INDEX form_template_versions_tenant_template_idx ON form_template_versions (tenant_id, template_id, version DESC);

INSERT INTO form_template_versions (
    id, tenant_id, template_id, version, schema, status, created_at, published_at
)
SELECT
    'ftv-' || md5(tenant_id || ':' || id || ':1'),
    tenant_id,
    id,
    1,
    schema,
    status,
    created_at,
    CASE WHEN status = 'published' THEN updated_at ELSE NULL END
FROM form_templates;

ALTER TABLE form_instances ADD COLUMN template_version_id text;

UPDATE form_instances fi
SET template_version_id = version.id
FROM form_template_versions version
WHERE version.tenant_id = fi.tenant_id
  AND version.template_id = fi.template_id
  AND version.version = 1;

ALTER TABLE form_instances
    ALTER COLUMN template_version_id SET NOT NULL,
    ADD CONSTRAINT form_instances_template_version_fk FOREIGN KEY (tenant_id, template_version_id) REFERENCES form_template_versions (tenant_id, id);

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
    CONSTRAINT form_instance_field_values_instance_fk FOREIGN KEY (tenant_id, form_instance_id) REFERENCES form_instances (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT form_instance_field_values_template_fk FOREIGN KEY (tenant_id, template_id) REFERENCES form_templates (tenant_id, id),
    CONSTRAINT form_instance_field_values_template_version_fk FOREIGN KEY (tenant_id, template_version_id) REFERENCES form_template_versions (tenant_id, id)
);

CREATE INDEX form_instance_field_values_text_idx ON form_instance_field_values (tenant_id, template_id, field_id, value_text);
CREATE INDEX form_instance_field_values_number_idx ON form_instance_field_values (tenant_id, template_id, field_id, value_number);
CREATE INDEX form_instance_field_values_boolean_idx ON form_instance_field_values (tenant_id, template_id, field_id, value_boolean);
CREATE INDEX form_instance_field_values_date_idx ON form_instance_field_values (tenant_id, template_id, field_id, value_date);
CREATE INDEX form_instance_field_values_timestamp_idx ON form_instance_field_values (tenant_id, template_id, field_id, value_timestamp);
CREATE INDEX form_instance_field_values_created_idx ON form_instance_field_values (tenant_id, template_id, field_id, created_at DESC);

ALTER TABLE form_template_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE form_template_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE form_instance_field_values ENABLE ROW LEVEL SECURITY;
ALTER TABLE form_instance_field_values FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_form_template_versions ON form_template_versions
    USING (tenant_id = current_setting('app.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

CREATE POLICY tenant_isolation_form_instance_field_values ON form_instance_field_values
    USING (tenant_id = current_setting('app.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- +goose Down

DROP TABLE IF EXISTS form_instance_field_values;
DROP INDEX IF EXISTS form_instances_tenant_template_status_submitted_idx;
ALTER TABLE form_instances DROP CONSTRAINT IF EXISTS form_instances_template_version_fk;
ALTER TABLE form_instances DROP COLUMN IF EXISTS template_version_id;
DROP TABLE IF EXISTS form_template_versions;
ALTER TABLE form_templates
    DROP CONSTRAINT IF EXISTS form_templates_current_version_check,
    DROP CONSTRAINT IF EXISTS form_templates_status_check,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS current_version,
    DROP COLUMN IF EXISTS status;
