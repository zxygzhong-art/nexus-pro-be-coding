-- Enable Row Level Security on every tenant-scoped table and grant the app role
-- DML access. RLS is FORCED so even the table owner is subject to isolation;
-- app_user (non-superuser, NOBYPASSRLS) only sees rows for app.current_tenant.

DO $$
DECLARE
    t text;
    tenant_tables text[] := ARRAY[
        'iam_accounts','iam_user_identities','iam_user_groups','iam_group_memberships',
        'iam_data_scopes','iam_policy_conditions','iam_permissions','iam_permission_sets',
        'iam_permission_set_permissions','iam_permission_set_assignments',
        'iam_permission_boundaries','iam_assumable_roles','iam_trust_policies',
        'iam_session_policies','iam_assumable_role_sessions','iam_menu_items',
        'iam_button_actions','iam_field_policies','iam_audit_logs'
    ];
BEGIN
    FOREACH t IN ARRAY tenant_tables LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format($p$
            CREATE POLICY %1$s_tenant_isolation ON %1$I
            USING (tenant_id = current_setting('app.current_tenant', true))
            WITH CHECK (tenant_id = current_setting('app.current_tenant', true))
        $p$, t);
        EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON %I TO app_user', t);
    END LOOP;
END
$$;

-- Global registries: readable by the app, not row-restricted.
GRANT SELECT ON iam_tenants, iam_applications TO app_user;
-- Allow bumping permission_version from the app layer.
GRANT UPDATE ON iam_tenants TO app_user;
