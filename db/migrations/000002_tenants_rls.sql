-- +goose Up

ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenants FORCE ROW LEVEL SECURITY;

-- tenants table 沒有 tenant_id 欄位；每一列以自身 id 隔離。
CREATE POLICY tenant_isolation_tenants ON tenants USING (id = current_setting('app.tenant_id', true)) WITH CHECK (id = current_setting('app.tenant_id', true));

-- +goose Down

DROP POLICY IF EXISTS tenant_isolation_tenants ON tenants;
ALTER TABLE tenants NO FORCE ROW LEVEL SECURITY;
ALTER TABLE tenants DISABLE ROW LEVEL SECURITY;
