-- User groups: the primary day-to-day authorization subject within a tenant.

CREATE TABLE iam_user_groups (
    id          text PRIMARY KEY,
    tenant_id   text NOT NULL REFERENCES iam_tenants(id),
    code        text,
    name        text NOT NULL,
    description text,
    source      text NOT NULL DEFAULT 'manual', -- manual | template | sync
    template_id text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);
CREATE UNIQUE INDEX uq_iam_user_groups_tenant_code ON iam_user_groups (tenant_id, code) WHERE code IS NOT NULL AND deleted_at IS NULL;

CREATE TABLE iam_group_memberships (
    id           text PRIMARY KEY,
    tenant_id    text NOT NULL REFERENCES iam_tenants(id),
    account_id   text NOT NULL REFERENCES iam_accounts(id),
    group_id     text NOT NULL REFERENCES iam_user_groups(id),
    valid_from   timestamptz,
    valid_until  timestamptz,
    source       text NOT NULL DEFAULT 'manual',
    approval_ref text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_iam_group_memberships ON iam_group_memberships (tenant_id, account_id, group_id);
CREATE INDEX ix_iam_group_memberships_group ON iam_group_memberships (tenant_id, group_id);
CREATE INDEX ix_iam_group_memberships_account ON iam_group_memberships (tenant_id, account_id);
