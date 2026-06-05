-- Permission points and reusable permission sets.

CREATE TABLE iam_permissions (
    id               text PRIMARY KEY, -- e.g. hr.employee.read
    tenant_id        text NOT NULL REFERENCES iam_tenants(id),
    application_code text NOT NULL,
    resource_type    text NOT NULL,
    action           text NOT NULL,
    default_scope    text,
    risk_level       text NOT NULL DEFAULT 'normal', -- normal | high | critical
    high_risk        boolean NOT NULL DEFAULT false,
    description      text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_iam_permissions_lookup ON iam_permissions (tenant_id, application_code, resource_type, action);

CREATE TABLE iam_permission_sets (
    id          text PRIMARY KEY,
    tenant_id   text NOT NULL REFERENCES iam_tenants(id),
    name        text NOT NULL,
    description text,
    source      text NOT NULL DEFAULT 'manual',
    version     int NOT NULL DEFAULT 1,
    copied_from text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

-- Join: permission_set -> permission.
CREATE TABLE iam_permission_set_permissions (
    tenant_id         text NOT NULL REFERENCES iam_tenants(id),
    permission_set_id text NOT NULL REFERENCES iam_permission_sets(id),
    permission_id     text NOT NULL REFERENCES iam_permissions(id),
    PRIMARY KEY (permission_set_id, permission_id)
);
CREATE INDEX ix_iam_psp_tenant ON iam_permission_set_permissions (tenant_id);
