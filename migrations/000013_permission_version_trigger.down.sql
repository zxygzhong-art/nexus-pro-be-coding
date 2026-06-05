DO $$
DECLARE
    t text;
    versioned_tables text[] := ARRAY[
        'iam_user_groups','iam_group_memberships','iam_permission_sets',
        'iam_permission_set_permissions','iam_permission_set_assignments',
        'iam_permissions','iam_field_policies','iam_data_scopes','iam_menu_items'
    ];
BEGIN
    FOREACH t IN ARRAY versioned_tables LOOP
        EXECUTE format('DROP TRIGGER IF EXISTS %1$s_bump_version ON %1$I', t);
    END LOOP;
END
$$;

DROP FUNCTION IF EXISTS iam_bump_permission_version();
