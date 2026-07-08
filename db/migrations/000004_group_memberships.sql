-- +goose Up

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
    CONSTRAINT authz_group_memberships_unique_idx UNIQUE (tenant_id, user_group_id, account_id),
    CONSTRAINT authz_group_memberships_group_fk FOREIGN KEY (tenant_id, user_group_id) REFERENCES user_groups (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT authz_group_memberships_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX authz_group_memberships_group_idx ON authz_group_memberships (tenant_id, user_group_id, created_at);
CREATE INDEX authz_group_memberships_account_active_idx ON authz_group_memberships (tenant_id, account_id, valid_from, valid_until);

INSERT INTO authz_group_memberships (
    id, tenant_id, user_group_id, account_id, valid_from, valid_until,
    source, approval_instance_id, created_by, created_at
)
SELECT
    'ugm_' || substr(md5(g.tenant_id || ':' || g.id || ':' || member.account_id), 1, 24),
    g.tenant_id,
    g.id,
    member.account_id,
    g.created_at,
    NULL,
    'migration',
    '',
    '',
    g.created_at
FROM user_groups g
CROSS JOIN LATERAL unnest(g.member_account_ids) AS member(account_id)
WHERE member.account_id <> ''
ON CONFLICT (tenant_id, user_group_id, account_id) DO NOTHING;

ALTER TABLE authz_group_memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE authz_group_memberships FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_authz_group_memberships ON authz_group_memberships USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- +goose Down

DROP TABLE IF EXISTS authz_group_memberships;
