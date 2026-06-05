-- Live assume sessions. Append-only operationally (status transitions), no soft delete.

CREATE TABLE iam_assumable_role_sessions (
    id                     text PRIMARY KEY, -- e.g. ars_01HX...
    tenant_id              text NOT NULL REFERENCES iam_tenants(id),
    account_id             text NOT NULL REFERENCES iam_accounts(id),
    assumable_role_id      text NOT NULL REFERENCES iam_assumable_roles(id),
    permission_boundary_id text REFERENCES iam_permission_boundaries(id),
    session_policy         jsonb NOT NULL DEFAULT '{}',
    reason                 text,
    status                 text NOT NULL DEFAULT 'active' CHECK (status IN ('active','expired','revoked')),
    expires_at             timestamptz NOT NULL,
    created_at             timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_iam_ars_account ON iam_assumable_role_sessions (tenant_id, account_id, status);
CREATE INDEX ix_iam_ars_expiry ON iam_assumable_role_sessions (expires_at);
