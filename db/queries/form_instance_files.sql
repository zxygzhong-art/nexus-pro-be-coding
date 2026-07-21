-- name: InsertFormInstanceFile :one
INSERT INTO form_instance_files (
    tenant_id, form_instance_id, file_id, field_id, state, created_at, updated_at
) VALUES (
    sqlc.arg(tenant_id), sqlc.arg(form_instance_id), sqlc.arg(file_id),
    sqlc.arg(field_id), sqlc.arg(state), sqlc.arg(created_at), sqlc.arg(updated_at)
)
RETURNING *;

-- name: GetFormInstanceFile :one
SELECT
    assets.id, assets.tenant_id, form_files.form_instance_id, form_files.field_id,
    assets.created_by_account_id, assets.original_filename,
    assets.object_provider, assets.object_bucket, assets.object_key,
    assets.content_type, assets.size_bytes, assets.sha256,
    assets.scan_status, assets.parse_status, assets.retention_class,
    form_files.state, assets.expires_at, assets.created_at, assets.updated_at
FROM form_instance_files form_files
JOIN file_assets assets
  ON assets.tenant_id = form_files.tenant_id
 AND assets.id = form_files.file_id
WHERE form_files.tenant_id = sqlc.arg(tenant_id)
  AND form_files.form_instance_id = sqlc.arg(form_instance_id)
  AND form_files.file_id = sqlc.arg(file_id)
  AND assets.deleted_at IS NULL;

-- name: ListFormInstanceFiles :many
SELECT
    assets.id, assets.tenant_id, form_files.form_instance_id, form_files.field_id,
    assets.created_by_account_id, assets.original_filename,
    assets.object_provider, assets.object_bucket, assets.object_key,
    assets.content_type, assets.size_bytes, assets.sha256,
    assets.scan_status, assets.parse_status, assets.retention_class,
    form_files.state, assets.expires_at, assets.created_at, assets.updated_at
FROM form_instance_files form_files
JOIN file_assets assets
  ON assets.tenant_id = form_files.tenant_id
 AND assets.id = form_files.file_id
WHERE form_files.tenant_id = sqlc.arg(tenant_id)
  AND form_files.form_instance_id = sqlc.arg(form_instance_id)
  AND assets.deleted_at IS NULL
ORDER BY form_files.created_at ASC, assets.id ASC;

-- name: ListFormInstanceFilesByField :many
SELECT
    assets.id, assets.tenant_id, form_files.form_instance_id, form_files.field_id,
    assets.created_by_account_id, assets.original_filename,
    assets.object_provider, assets.object_bucket, assets.object_key,
    assets.content_type, assets.size_bytes, assets.sha256,
    assets.scan_status, assets.parse_status, assets.retention_class,
    form_files.state, assets.expires_at, assets.created_at, assets.updated_at
FROM form_instance_files form_files
JOIN file_assets assets
  ON assets.tenant_id = form_files.tenant_id
 AND assets.id = form_files.file_id
WHERE form_files.tenant_id = sqlc.arg(tenant_id)
  AND form_files.form_instance_id = sqlc.arg(form_instance_id)
  AND form_files.field_id = sqlc.arg(field_id)
  AND assets.deleted_at IS NULL
ORDER BY form_files.created_at ASC, assets.id ASC;

-- name: MarkFormInstanceFileAttached :one
UPDATE form_instance_files
SET state = 'attached', updated_at = sqlc.arg(updated_at)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND form_instance_id = sqlc.arg(form_instance_id)
  AND file_id = sqlc.arg(file_id)
RETURNING *;

-- name: MarkFormInstanceFilesAttached :exec
UPDATE form_instance_files
SET state = 'attached', updated_at = sqlc.arg(updated_at)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND form_instance_id = sqlc.arg(form_instance_id)
  AND state = 'draft';

-- name: DeleteDraftFormInstanceFile :one
DELETE FROM form_instance_files
WHERE tenant_id = sqlc.arg(tenant_id)
  AND form_instance_id = sqlc.arg(form_instance_id)
  AND file_id = sqlc.arg(file_id)
  AND state = 'draft'
RETURNING file_id;

-- name: CountFormInstanceFilesByField :one
SELECT COUNT(*)::bigint AS count
FROM form_instance_files
WHERE tenant_id = sqlc.arg(tenant_id)
  AND form_instance_id = sqlc.arg(form_instance_id)
  AND field_id = sqlc.arg(field_id);
