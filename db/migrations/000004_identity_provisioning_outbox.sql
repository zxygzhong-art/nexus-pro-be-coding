-- +goose Up

-- Keycloak Admin provisioning 會排入此佇列，而不是在 HR transaction 內直接執行；
-- 因此 rollback 不會留下孤立外部使用者，Keycloak outage 也不會阻塞員工建立、匯入或邀請。
CREATE TABLE identity_provisioning_outbox (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id text NOT NULL,
    employee_id text NOT NULL DEFAULT '',
    employee_no text NOT NULL DEFAULT '',
    email text NOT NULL,
    display_name text NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    send_invite boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'succeeded', 'failed')),
    retry_count integer NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE INDEX identity_provisioning_outbox_tenant_status_idx ON identity_provisioning_outbox (tenant_id, status, created_at);

ALTER TABLE identity_provisioning_outbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE identity_provisioning_outbox FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_identity_provisioning_outbox ON identity_provisioning_outbox USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- +goose Down

DROP TABLE IF EXISTS identity_provisioning_outbox;
