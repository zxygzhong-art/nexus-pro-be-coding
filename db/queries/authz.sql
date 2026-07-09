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

-- name: GetUserIdentity :one
SELECT * FROM user_identities
WHERE tenant_id = $1 AND provider = $2 AND subject = $3;

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

-- name: UpdateAuthzDataScope :one
UPDATE authz_data_scopes
SET code = $3,
    name = $4,
    scope_type = $5,
    params = $6::jsonb
WHERE tenant_id = $1 AND id = $2
RETURNING *;

-- name: DeleteAuthzDataScope :one
DELETE FROM authz_data_scopes
WHERE tenant_id = $1 AND id = $2
RETURNING *;

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

-- name: GetAuthzFieldPolicy :one
SELECT * FROM authz_field_policies
WHERE tenant_id = $1 AND id = $2;

-- name: ListAuthzFieldPolicies :many
SELECT * FROM authz_field_policies
WHERE tenant_id = $1
  AND ($2::text = '' OR application_code = $2)
  AND ($3::text = '' OR resource_type = $3)
ORDER BY field_name ASC;

-- name: DeleteAuthzFieldPolicy :one
DELETE FROM authz_field_policies
WHERE tenant_id = $1 AND id = $2
RETURNING *;

-- name: UpsertAuthzPermissionSetAssignment :one
INSERT INTO authz_permission_set_assignments (
    id, tenant_id, principal_type, principal_id, permission_set_id,
    effect, data_scope_id, condition_id, starts_at, expires_at, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    principal_type = EXCLUDED.principal_type,
    principal_id = EXCLUDED.principal_id,
    permission_set_id = EXCLUDED.permission_set_id,
    effect = EXCLUDED.effect,
    data_scope_id = EXCLUDED.data_scope_id,
    condition_id = EXCLUDED.condition_id,
    starts_at = EXCLUDED.starts_at,
    expires_at = EXCLUDED.expires_at,
    created_at = EXCLUDED.created_at
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

-- name: DeleteAuthzPermissionSetAssignment :one
DELETE FROM authz_permission_set_assignments
WHERE tenant_id = $1 AND id = $2
RETURNING *;

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

-- name: ListActiveAuthzAssumableRoleSessionsForRole :many
SELECT * FROM authz_assumable_role_sessions
WHERE tenant_id = $1
  AND assumable_role_id = $2
  AND revoked_at IS NULL
  AND expires_at > now()
ORDER BY created_at ASC;

-- name: DeleteAuthzAssumableRoleSessionsForRole :exec
DELETE FROM authz_assumable_role_sessions
WHERE tenant_id = $1
  AND assumable_role_id = $2;

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
