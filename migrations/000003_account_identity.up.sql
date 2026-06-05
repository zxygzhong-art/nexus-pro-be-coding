-- Platform accounts and their external identity mappings (Keycloak/wecom/...).

CREATE TABLE iam_accounts (
    id           text PRIMARY KEY,
    tenant_id    text NOT NULL REFERENCES iam_tenants(id),
    email        text NOT NULL,
    display_name text,
    status       text NOT NULL DEFAULT 'active',
    account_type text NOT NULL DEFAULT 'human', -- human | service | external
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz
);
CREATE UNIQUE INDEX uq_iam_accounts_tenant_email ON iam_accounts (tenant_id, email) WHERE deleted_at IS NULL;
CREATE INDEX ix_iam_accounts_tenant_status ON iam_accounts (tenant_id, status);

CREATE TABLE iam_user_identities (
    id         text PRIMARY KEY,
    tenant_id  text NOT NULL REFERENCES iam_tenants(id),
    account_id text NOT NULL REFERENCES iam_accounts(id),
    provider   text NOT NULL, -- keycloak | wecom | dingtalk | azuread
    subject    text NOT NULL,
    raw_claims jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_iam_user_identities_provider_subject ON iam_user_identities (provider, subject);
CREATE INDEX ix_iam_user_identities_tenant_account ON iam_user_identities (tenant_id, account_id);
