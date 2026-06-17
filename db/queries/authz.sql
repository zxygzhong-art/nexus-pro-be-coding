-- name: UpsertUserIdentity :one
INSERT INTO user_identities (
    id, tenant_id, account_id, provider, subject, email, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (tenant_id, provider, subject) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    account_id = EXCLUDED.account_id,
    email = EXCLUDED.email
RETURNING *;

-- name: ListUserIdentities :many
SELECT * FROM user_identities
WHERE tenant_id = $1 AND account_id = $2
ORDER BY created_at ASC;

-- name: UpsertAuthzApplication :one
INSERT INTO authz_applications (
    id, tenant_id, code, name, description, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6
)
ON CONFLICT (tenant_id, code) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description
RETURNING *;

-- name: ListAuthzApplications :many
SELECT * FROM authz_applications
WHERE tenant_id = $1
ORDER BY code ASC;

-- name: UpsertAuthzPermission :one
INSERT INTO authz_permissions (
    id, tenant_id, application_code, resource_type, action,
    name, description, risk_level, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
ON CONFLICT (tenant_id, application_code, resource_type, action) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    risk_level = EXCLUDED.risk_level
RETURNING *;

-- name: GetAuthzPermissionByKey :one
SELECT * FROM authz_permissions
WHERE tenant_id = $1
  AND application_code = $2
  AND resource_type = $3
  AND action = $4;

-- name: ListAuthzPermissions :many
SELECT * FROM authz_permissions
WHERE tenant_id = $1
ORDER BY application_code ASC, resource_type ASC, action ASC;

-- name: UpsertAuthzPermissionSetPermission :one
INSERT INTO authz_permission_set_permissions (
    id, tenant_id, permission_set_id, permission_id, effect, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6
)
ON CONFLICT (tenant_id, permission_set_id, permission_id) DO UPDATE SET
    effect = EXCLUDED.effect
RETURNING *;

-- name: ListAuthzPermissionSetPermissions :many
SELECT * FROM authz_permission_set_permissions
WHERE tenant_id = $1 AND permission_set_id = $2
ORDER BY created_at ASC;

-- name: UpsertAuthzGroupMembership :one
INSERT INTO authz_group_memberships (
    id, tenant_id, group_id, account_id, source, expires_at, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (tenant_id, group_id, account_id) DO UPDATE SET
    source = EXCLUDED.source,
    expires_at = EXCLUDED.expires_at
RETURNING *;

-- name: ListAuthzGroupMembershipsByAccount :many
SELECT * FROM authz_group_memberships
WHERE tenant_id = $1 AND account_id = $2
  AND (expires_at IS NULL OR expires_at > now())
ORDER BY created_at ASC;

-- name: UpsertAuthzDataScope :one
INSERT INTO authz_data_scopes (
    id, tenant_id, code, name, scope_type, params, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6::jsonb, $7
)
ON CONFLICT (tenant_id, code) DO UPDATE SET
    name = EXCLUDED.name,
    scope_type = EXCLUDED.scope_type,
    params = EXCLUDED.params
RETURNING *;

-- name: ListAuthzDataScopes :many
SELECT * FROM authz_data_scopes
WHERE tenant_id = $1
ORDER BY code ASC;

-- name: GetAuthzDataScope :one
SELECT * FROM authz_data_scopes
WHERE tenant_id = $1 AND id = $2;

-- name: GetAuthzDataScopeByCode :one
SELECT * FROM authz_data_scopes
WHERE tenant_id = $1 AND code = $2;

-- name: UpsertAuthzPolicyCondition :one
INSERT INTO authz_policy_conditions (
    id, tenant_id, name, expression, created_at
) VALUES (
    $1, $2, $3, $4::jsonb, $5
)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    expression = EXCLUDED.expression
RETURNING *;

-- name: ListAuthzPolicyConditions :many
SELECT * FROM authz_policy_conditions
WHERE tenant_id = $1
ORDER BY created_at ASC;

-- name: UpsertAuthzFieldPolicy :one
INSERT INTO authz_field_policies (
    id, tenant_id, application_code, resource_type, field_name,
    effect, mask_strategy, permission_id, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
ON CONFLICT (id) DO UPDATE SET
    application_code = EXCLUDED.application_code,
    resource_type = EXCLUDED.resource_type,
    field_name = EXCLUDED.field_name,
    effect = EXCLUDED.effect,
    mask_strategy = EXCLUDED.mask_strategy,
    permission_id = EXCLUDED.permission_id
RETURNING *;

-- name: ListAuthzFieldPolicies :many
SELECT * FROM authz_field_policies
WHERE tenant_id = $1
  AND application_code = $2
  AND resource_type = $3
ORDER BY field_name ASC;

-- name: CreateAuthzPermissionSetAssignment :one
INSERT INTO authz_permission_set_assignments (
    id, tenant_id, principal_type, principal_id, permission_set_id,
    effect, data_scope_id, condition_id, starts_at, expires_at, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
RETURNING *;

-- name: ListAuthzPermissionSetAssignmentsForPrincipal :many
SELECT * FROM authz_permission_set_assignments
WHERE tenant_id = $1
  AND principal_type = $2
  AND principal_id = $3
  AND (starts_at IS NULL OR starts_at <= now())
  AND (expires_at IS NULL OR expires_at > now())
ORDER BY created_at ASC;

-- name: ListAuthzPermissionSetAssignments :many
SELECT * FROM authz_permission_set_assignments
WHERE tenant_id = $1
ORDER BY created_at ASC;

-- name: CreateAuthzAssumableRoleSession :one
INSERT INTO authz_assumable_role_sessions (
    id, tenant_id, account_id, assumable_role_id, session_policy,
    expires_at, revoked_at, created_at
) VALUES (
    $1, $2, $3, $4, $5::jsonb, $6, $7, $8
)
RETURNING *;

-- name: GetActiveAuthzAssumableRoleSession :one
SELECT * FROM authz_assumable_role_sessions
WHERE tenant_id = $1
  AND id = $2
  AND revoked_at IS NULL
  AND expires_at > now();

-- name: RevokeAuthzAssumableRoleSession :one
UPDATE authz_assumable_role_sessions
SET revoked_at = $3
WHERE tenant_id = $1 AND id = $2
RETURNING *;

-- name: UpsertAuthzRelationshipTuple :one
INSERT INTO authz_relationship_tuples (
    id, tenant_id, object_type, object_id, relation,
    subject_type, subject_id, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
ON CONFLICT (tenant_id, object_type, object_id, relation, subject_type, subject_id) DO UPDATE SET
    created_at = EXCLUDED.created_at
RETURNING *;

-- name: ListAuthzRelationshipTuplesForObject :many
SELECT * FROM authz_relationship_tuples
WHERE tenant_id = $1
  AND object_type = $2
  AND object_id = $3
ORDER BY relation ASC, subject_type ASC, subject_id ASC;

-- name: DeleteAuthzRelationshipTuple :exec
DELETE FROM authz_relationship_tuples
WHERE tenant_id = $1
  AND object_type = $2
  AND object_id = $3
  AND relation = $4
  AND subject_type = $5
  AND subject_id = $6;

-- name: GetAuthzPermissionVersion :one
SELECT * FROM authz_permission_versions
WHERE tenant_id = $1;

-- name: IncrementAuthzPermissionVersion :one
INSERT INTO authz_permission_versions (
    tenant_id, version, updated_at
) VALUES (
    $1, 1, $2
)
ON CONFLICT (tenant_id) DO UPDATE SET
    version = authz_permission_versions.version + 1,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: AppendAuthzOutboxEvent :one
INSERT INTO authz_outbox_events (
    id, tenant_id, event_type, payload, status, retry_count,
    last_error, created_at, processed_at
) VALUES (
    $1, $2, $3, $4::jsonb, $5, $6, $7, $8, $9
)
RETURNING *;

-- name: ListAuthzOutboxEvents :many
SELECT * FROM authz_outbox_events
WHERE tenant_id = $1
ORDER BY created_at ASC;

-- name: UpdateAuthzOutboxEvent :one
UPDATE authz_outbox_events
SET status = $3,
    retry_count = $4,
    last_error = $5,
    processed_at = $6
WHERE tenant_id = $1
  AND id = $2
RETURNING *;
