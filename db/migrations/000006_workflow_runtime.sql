-- +goose Up

ALTER TABLE form_instances
    ADD COLUMN IF NOT EXISTS current_run_id text NOT NULL DEFAULT '';

ALTER TABLE form_instances
    ADD CONSTRAINT form_instances_tenant_id_id_idx UNIQUE (tenant_id, id);

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

ALTER TABLE workflow_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_runs FORCE ROW LEVEL SECURITY;
ALTER TABLE workflow_stage_instances ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_stage_instances FORCE ROW LEVEL SECURITY;
ALTER TABLE workflow_stage_assignees ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_stage_assignees FORCE ROW LEVEL SECURITY;
ALTER TABLE workflow_actions ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_actions FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_workflow_runs ON workflow_runs USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_workflow_stage_instances ON workflow_stage_instances USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_workflow_stage_assignees ON workflow_stage_assignees USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_workflow_actions ON workflow_actions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- +goose Down

DROP TABLE IF EXISTS workflow_actions;
DROP TABLE IF EXISTS workflow_stage_assignees;
DROP TABLE IF EXISTS workflow_stage_instances;
DROP TABLE IF EXISTS workflow_runs;

ALTER TABLE form_instances DROP CONSTRAINT IF EXISTS form_instances_tenant_id_id_idx;
ALTER TABLE form_instances DROP COLUMN IF EXISTS current_run_id;
