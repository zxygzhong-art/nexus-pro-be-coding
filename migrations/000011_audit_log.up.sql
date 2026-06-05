-- Append-only audit trail for permission decisions and high-risk actions.

CREATE TABLE iam_audit_logs (
    id                      text PRIMARY KEY,
    tenant_id               text NOT NULL REFERENCES iam_tenants(id),
    application_code        text,
    actor_account_id        text,
    action                  text,
    resource_type           text,
    resource_id             text,
    authz_decision          text, -- allow | deny | approval_required
    matched_permissions     jsonb NOT NULL DEFAULT '[]',
    matched_sources         jsonb NOT NULL DEFAULT '[]',
    assumed_role_session_id text,
    permission_boundary     text,
    data_scope              text,
    field_policies          jsonb NOT NULL DEFAULT '{}',
    request_id              text,
    trace_id                text,
    metadata                jsonb NOT NULL DEFAULT '{}',
    created_at              timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_iam_audit_logs_recent ON iam_audit_logs (tenant_id, created_at DESC);
CREATE INDEX ix_iam_audit_logs_actor ON iam_audit_logs (tenant_id, actor_account_id, created_at DESC);
CREATE INDEX ix_iam_audit_logs_resource ON iam_audit_logs (tenant_id, resource_type, resource_id);
