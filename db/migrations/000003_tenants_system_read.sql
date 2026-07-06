-- +goose Up

-- 跨 tenant 背景工作（例如 OpenFGA outbox processor）需要列舉所有 tenant，
-- 但 tenant_isolation_tenants 只會暴露符合 app.tenant_id 的列。這個唯讀 policy
-- 允許透過 set_config('app.system_task', 'on', true) opt in 的連線在沒有 BYPASSRLS
-- 的情況下列出所有 tenant。應用程式會透過 tenantctx.WithSystemTask 注入此設定
-- （見 internal/repository/postgres/tenant_dbtx.go）；行為由 tenantctx 單元測試與
-- tests/integration/postgres 內的 non-BYPASSRLS ListTenants 整合測試覆蓋。
CREATE POLICY system_read_tenants ON tenants FOR SELECT USING (current_setting('app.system_task', true) = 'on');

-- +goose Down

DROP POLICY IF EXISTS system_read_tenants ON tenants;
