-- Polymorphic assignment of a permission set to a user / group / assumable role.
-- subject_id is intentionally NOT a FK (it targets three different tables).

CREATE TABLE iam_permission_set_assignments (
    id                text PRIMARY KEY,
    tenant_id         text NOT NULL REFERENCES iam_tenants(id),
    permission_set_id text NOT NULL REFERENCES iam_permission_sets(id),
    subject_type      text NOT NULL CHECK (subject_type IN ('user','group','assumable_role')),
    subject_id        text NOT NULL,
    effect            text NOT NULL DEFAULT 'allow' CHECK (effect IN ('allow','deny')),
    data_scope_id     text REFERENCES iam_data_scopes(id),
    condition         jsonb NOT NULL DEFAULT '{}',
    valid_from        timestamptz,
    valid_until       timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    deleted_at        timestamptz
);
CREATE INDEX ix_iam_psa_subject ON iam_permission_set_assignments (tenant_id, subject_type, subject_id);
CREATE INDEX ix_iam_psa_set ON iam_permission_set_assignments (tenant_id, permission_set_id);
