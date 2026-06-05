-- Structured data scopes and reusable JSON policy conditions. The engine emits
-- structured conditions; business code never receives raw SQL strings.

CREATE TABLE iam_data_scopes (
    id         text PRIMARY KEY,
    tenant_id  text NOT NULL REFERENCES iam_tenants(id),
    name       text,
    scope_type text NOT NULL CHECK (scope_type IN (
        'own','direct_reports','department','department_subtree',
        'assigned_org_units','custom_condition','tenant','system')),
    conditions jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_iam_data_scopes_tenant_type ON iam_data_scopes (tenant_id, scope_type);

CREATE TABLE iam_policy_conditions (
    id          text PRIMARY KEY,
    tenant_id   text NOT NULL REFERENCES iam_tenants(id),
    name        text,
    expression  jsonb NOT NULL DEFAULT '{}', -- JSON condition tree, never SQL
    description text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
