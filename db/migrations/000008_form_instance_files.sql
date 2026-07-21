-- +goose Up
-- Form instance attachments reuse file_assets and bind them to a form field.

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

ALTER TABLE form_instance_files ENABLE ROW LEVEL SECURITY;
ALTER TABLE form_instance_files FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_form_instance_files ON form_instance_files
    USING (tenant_id = current_setting('app.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation_form_instance_files ON form_instance_files;
DROP TABLE IF EXISTS form_instance_files;
