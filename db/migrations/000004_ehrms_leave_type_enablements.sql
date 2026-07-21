-- +goose Up
CREATE TABLE ehrms_leave_type_enablements (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code text NOT NULL,
    enabled boolean NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, code)
);

ALTER TABLE ehrms_leave_type_enablements ENABLE ROW LEVEL SECURITY;
ALTER TABLE ehrms_leave_type_enablements FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_ehrms_leave_type_enablements ON ehrms_leave_type_enablements
USING (tenant_id = current_setting('app.tenant_id', true))
WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- +goose Down

DROP POLICY IF EXISTS tenant_isolation_ehrms_leave_type_enablements ON ehrms_leave_type_enablements;
DROP TABLE IF EXISTS ehrms_leave_type_enablements;
