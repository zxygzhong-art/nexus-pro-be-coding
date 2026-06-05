-- Bump iam_tenants.permission_version whenever permission configuration changes,
-- so cached permission snapshots (keyed by version) invalidate naturally (§15).

CREATE OR REPLACE FUNCTION iam_bump_permission_version() RETURNS trigger AS $$
DECLARE
    tid text;
BEGIN
    tid := COALESCE(NEW.tenant_id, OLD.tenant_id);
    IF tid IS NOT NULL THEN
        UPDATE iam_tenants SET permission_version = permission_version + 1, updated_at = now()
        WHERE id = tid;
    END IF;
    RETURN NULL; -- AFTER trigger, return value ignored
END;
$$ LANGUAGE plpgsql;

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
        EXECUTE format($q$
            CREATE TRIGGER %1$s_bump_version
            AFTER INSERT OR UPDATE OR DELETE ON %1$I
            FOR EACH ROW EXECUTE FUNCTION iam_bump_permission_version()
        $q$, t);
    END LOOP;
END
$$;
