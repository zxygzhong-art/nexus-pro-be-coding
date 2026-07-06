-- +goose Up

CREATE TABLE notifications (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    tone text NOT NULL CHECK (tone IN ('success', 'info', 'warning')),
    category text NOT NULL DEFAULT 'system',
    title text NOT NULL,
    body text NOT NULL,
    status_text text NOT NULL,
    link_url text NOT NULL DEFAULT '',
    source_type text NOT NULL DEFAULT '',
    source_id text NOT NULL DEFAULT '',
    created_by_account_id text,
    created_at timestamptz NOT NULL,
    expires_at timestamptz,
    CONSTRAINT notifications_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT notifications_created_by_fk FOREIGN KEY (tenant_id, created_by_account_id) REFERENCES accounts (tenant_id, id)
);

CREATE INDEX notifications_tenant_created_at_idx ON notifications (tenant_id, created_at DESC, id DESC);
CREATE UNIQUE INDEX notifications_source_unique_idx ON notifications (tenant_id, source_type, source_id) WHERE source_type <> '' AND source_id <> '';

CREATE TABLE notification_recipients (
    notification_id text NOT NULL,
    tenant_id text NOT NULL,
    account_id text NOT NULL,
    read_at timestamptz,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (notification_id, account_id),
    CONSTRAINT notification_recipients_notification_fk FOREIGN KEY (tenant_id, notification_id) REFERENCES notifications (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT notification_recipients_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX notification_recipients_account_idx ON notification_recipients (tenant_id, account_id, read_at, created_at DESC);

ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE notifications FORCE ROW LEVEL SECURITY;
ALTER TABLE notification_recipients ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_recipients FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_notifications ON notifications USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_notification_recipients ON notification_recipients USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- +goose Down

DROP TABLE IF EXISTS notification_recipients;
DROP TABLE IF EXISTS notifications;
