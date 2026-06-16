-- +goose Up

DO $$
DECLARE
    table_name text;
    table_names text[] := ARRAY[
        'accounts',
        'user_groups',
        'permission_sets',
        'assumable_roles',
        'user_identities',
        'authz_applications',
        'authz_permissions',
        'authz_permission_set_permissions',
        'authz_group_memberships',
        'authz_data_scopes',
        'authz_policy_conditions',
        'authz_field_policies',
        'authz_permission_set_assignments',
        'authz_assumable_role_sessions',
        'authz_relationship_tuples',
        'authz_permission_versions',
        'authz_outbox_events',
        'org_units',
        'employees',
        'employee_number_sequences',
        'employee_import_sessions',
        'leave_balances',
        'form_templates',
        'form_instances',
        'leave_requests',
        'knowledge_articles',
        'agent_runs',
        'audit_logs'
    ];
BEGIN
    FOREACH table_name IN ARRAY table_names LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', table_name);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', table_name);
        EXECUTE format('DROP POLICY IF EXISTS %I ON %I', 'tenant_isolation_' || table_name, table_name);
        EXECUTE format(
            'CREATE POLICY %I ON %I USING (tenant_id = current_setting(''app.tenant_id'', true)) WITH CHECK (tenant_id = current_setting(''app.tenant_id'', true))',
            'tenant_isolation_' || table_name,
            table_name
        );
    END LOOP;
END $$;

-- +goose Down

DO $$
DECLARE
    table_name text;
    table_names text[] := ARRAY[
        'accounts',
        'user_groups',
        'permission_sets',
        'assumable_roles',
        'user_identities',
        'authz_applications',
        'authz_permissions',
        'authz_permission_set_permissions',
        'authz_group_memberships',
        'authz_data_scopes',
        'authz_policy_conditions',
        'authz_field_policies',
        'authz_permission_set_assignments',
        'authz_assumable_role_sessions',
        'authz_relationship_tuples',
        'authz_permission_versions',
        'authz_outbox_events',
        'org_units',
        'employees',
        'employee_number_sequences',
        'employee_import_sessions',
        'leave_balances',
        'form_templates',
        'form_instances',
        'leave_requests',
        'knowledge_articles',
        'agent_runs',
        'audit_logs'
    ];
BEGIN
    FOREACH table_name IN ARRAY table_names LOOP
        EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', table_name);
        EXECUTE format('DROP POLICY IF EXISTS %I ON %I', 'tenant_isolation_' || table_name, table_name);
        EXECUTE format(
            'CREATE POLICY %I ON %I USING (tenant_id = current_setting(''app.tenant_id'', true))',
            'tenant_isolation_' || table_name,
            table_name
        );
    END LOOP;
END $$;
