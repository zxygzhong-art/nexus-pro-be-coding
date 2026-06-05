-- AssumableRole: a high-security identity that trusted principals can temporarily
-- assume. Boundaries are created first because assumable roles reference them.

CREATE TABLE iam_permission_boundaries (
    id                  text PRIMARY KEY, -- e.g. boundary.platform-support-readonly
    tenant_id           text NOT NULL REFERENCES iam_tenants(id),
    name                text,
    allowed_permissions jsonb NOT NULL DEFAULT '[]', -- whitelist of permission ids / patterns
    scope_type          text,
    scope_conditions    jsonb NOT NULL DEFAULT '{}',
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE iam_assumable_roles (
    id                     text PRIMARY KEY, -- e.g. assumable.platform-support-readonly
    tenant_id              text NOT NULL REFERENCES iam_tenants(id),
    name                   text,
    description            text,
    permission_boundary_id text REFERENCES iam_permission_boundaries(id),
    max_session_minutes    int NOT NULL DEFAULT 60,
    requires_approval      boolean NOT NULL DEFAULT false,
    audit_level            text NOT NULL DEFAULT 'full',
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    deleted_at             timestamptz
);

CREATE TABLE iam_trust_policies (
    id                text PRIMARY KEY,
    tenant_id         text NOT NULL REFERENCES iam_tenants(id),
    assumable_role_id text NOT NULL REFERENCES iam_assumable_roles(id),
    policy            jsonb NOT NULL DEFAULT '{}', -- who can assume + mfa/source_ip/time_window/approval
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_iam_trust_policies_role ON iam_trust_policies (tenant_id, assumable_role_id);

CREATE TABLE iam_session_policies (
    id                text PRIMARY KEY,
    tenant_id         text NOT NULL REFERENCES iam_tenants(id),
    assumable_role_id text REFERENCES iam_assumable_roles(id),
    policy            jsonb NOT NULL DEFAULT '{}', -- e.g. {"deny": ["hr.employee.export"]}
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
