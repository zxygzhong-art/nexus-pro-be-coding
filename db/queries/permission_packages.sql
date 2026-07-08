-- name: UpsertPermissionPackage :one
INSERT INTO permission_packages (
    id, application_code, version, status, content, checksum, created_at, published_at
) VALUES (
    sqlc.arg(id), sqlc.arg(application_code), sqlc.arg(version), sqlc.arg(status),
    sqlc.arg(content)::jsonb, sqlc.arg(checksum), sqlc.arg(created_at), sqlc.narg(published_at)
)
ON CONFLICT (id) DO UPDATE SET
    application_code = EXCLUDED.application_code,
    version = EXCLUDED.version,
    status = EXCLUDED.status,
    content = EXCLUDED.content,
    checksum = EXCLUDED.checksum,
    created_at = EXCLUDED.created_at,
    published_at = EXCLUDED.published_at
RETURNING *;

-- name: UpdatePermissionPackageStatus :one
UPDATE permission_packages
SET status = $2,
    published_at = $3
WHERE id = $1
RETURNING *;

-- name: GetPermissionPackage :one
SELECT * FROM permission_packages
WHERE id = $1;

-- name: GetPermissionPackageByApplicationVersion :one
SELECT * FROM permission_packages
WHERE application_code = $1 AND version = $2;

-- name: ListPermissionPackages :many
SELECT * FROM permission_packages
ORDER BY application_code ASC, version ASC;

-- name: UpsertPermissionSetTemplate :one
INSERT INTO permission_set_templates (
    id, package_id, template_key, name, content, version
) VALUES (
    sqlc.arg(id), sqlc.arg(package_id), sqlc.arg(template_key), sqlc.arg(name), sqlc.arg(content)::jsonb, sqlc.arg(version)
)
ON CONFLICT (package_id, template_key) DO UPDATE SET
    name = EXCLUDED.name,
    content = EXCLUDED.content,
    version = EXCLUDED.version
RETURNING *;

-- name: ListPermissionSetTemplates :many
SELECT * FROM permission_set_templates
WHERE package_id = $1
ORDER BY template_key ASC;

-- name: UpsertUserGroupTemplate :one
INSERT INTO user_group_templates (
    id, package_id, template_key, name, content, version
) VALUES (
    sqlc.arg(id), sqlc.arg(package_id), sqlc.arg(template_key), sqlc.arg(name), sqlc.arg(content)::jsonb, sqlc.arg(version)
)
ON CONFLICT (package_id, template_key) DO UPDATE SET
    name = EXCLUDED.name,
    content = EXCLUDED.content,
    version = EXCLUDED.version
RETURNING *;

-- name: ListUserGroupTemplates :many
SELECT * FROM user_group_templates
WHERE package_id = $1
ORDER BY template_key ASC;

-- name: UpsertAssumableRoleTemplate :one
INSERT INTO assumable_role_templates (
    id, package_id, template_key, name, content, version
) VALUES (
    sqlc.arg(id), sqlc.arg(package_id), sqlc.arg(template_key), sqlc.arg(name), sqlc.arg(content)::jsonb, sqlc.arg(version)
)
ON CONFLICT (package_id, template_key) DO UPDATE SET
    name = EXCLUDED.name,
    content = EXCLUDED.content,
    version = EXCLUDED.version
RETURNING *;

-- name: ListAssumableRoleTemplates :many
SELECT * FROM assumable_role_templates
WHERE package_id = $1
ORDER BY template_key ASC;

-- name: UpsertPermissionPackageImport :one
INSERT INTO permission_package_imports (
    id, tenant_id, package_id, version, imported_at, imported_by, artifact_id_map
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(package_id), sqlc.arg(version),
    sqlc.arg(imported_at), sqlc.arg(imported_by), sqlc.arg(artifact_id_map)::jsonb
)
ON CONFLICT (tenant_id, package_id, version) DO UPDATE SET
    artifact_id_map = permission_package_imports.artifact_id_map
RETURNING *;

-- name: GetPermissionPackageImport :one
SELECT * FROM permission_package_imports
WHERE tenant_id = $1 AND package_id = $2 AND version = $3;

-- name: ListPermissionPackageImports :many
SELECT * FROM permission_package_imports
WHERE tenant_id = $1
ORDER BY imported_at ASC;
