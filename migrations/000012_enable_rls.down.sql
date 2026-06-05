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
        EXECUTE format('DROP POLICY IF EXISTS %1$s_tenant_isolation ON %1$I', t);
        EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY', t);
        EXECUTE format('REVOKE ALL ON %I FROM app_user', t);
    END LOOP;
END
$$;

REVOKE ALL ON iam_tenants, iam_applications FROM app_user;
