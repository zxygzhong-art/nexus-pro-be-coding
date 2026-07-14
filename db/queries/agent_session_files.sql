-- name: UpsertFileAsset :one
INSERT INTO file_assets (
    id, tenant_id, created_by_account_id, original_filename,
    object_provider, object_bucket, object_key, content_type,
    size_bytes, sha256, scan_status, parse_status, retention_class,
    expires_at, created_at, updated_at, deleted_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(created_by_account_id), sqlc.arg(original_filename),
    sqlc.arg(object_provider), sqlc.arg(object_bucket), sqlc.arg(object_key), sqlc.arg(content_type),
    sqlc.arg(size_bytes), sqlc.arg(sha256), sqlc.arg(scan_status), sqlc.arg(parse_status), sqlc.arg(retention_class),
    sqlc.arg(expires_at), sqlc.arg(created_at), sqlc.arg(updated_at), sqlc.arg(deleted_at)
)
ON CONFLICT (id) DO UPDATE SET
    original_filename = EXCLUDED.original_filename,
    object_provider = EXCLUDED.object_provider,
    object_bucket = EXCLUDED.object_bucket,
    object_key = EXCLUDED.object_key,
    content_type = EXCLUDED.content_type,
    size_bytes = EXCLUDED.size_bytes,
    sha256 = EXCLUDED.sha256,
    scan_status = EXCLUDED.scan_status,
    parse_status = EXCLUDED.parse_status,
    retention_class = EXCLUDED.retention_class,
    expires_at = EXCLUDED.expires_at,
    updated_at = EXCLUDED.updated_at,
    deleted_at = EXCLUDED.deleted_at
RETURNING *;

-- name: InsertFileChunk :one
INSERT INTO file_chunks (
    id, tenant_id, file_id, ordinal, content, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(file_id),
    sqlc.arg(ordinal), sqlc.arg(content), sqlc.arg(created_at)
)
RETURNING *;

-- name: ListFileChunks :many
SELECT * FROM file_chunks
WHERE tenant_id = sqlc.arg(tenant_id)
  AND file_id = sqlc.arg(file_id)
ORDER BY ordinal ASC;

-- name: InsertAgentSessionFile :one
INSERT INTO agent_session_files (
    tenant_id, session_id, file_id, context_version, state, created_at, updated_at
) VALUES (
    sqlc.arg(tenant_id), sqlc.arg(session_id), sqlc.arg(file_id),
    sqlc.arg(context_version), sqlc.arg(state), sqlc.arg(created_at), sqlc.arg(updated_at)
)
RETURNING *;

-- name: GetCurrentAgentSessionFile :one
SELECT
    assets.id, assets.tenant_id, session_files.session_id, session_files.context_version,
    assets.created_by_account_id, assets.original_filename,
    assets.object_provider, assets.object_bucket, assets.object_key,
    assets.content_type, assets.size_bytes, assets.sha256,
    assets.scan_status, assets.parse_status, assets.retention_class,
    session_files.state, assets.expires_at, assets.created_at, assets.updated_at
FROM agent_session_files session_files
JOIN agent_sessions sessions
  ON sessions.tenant_id = session_files.tenant_id
 AND sessions.id = session_files.session_id
JOIN file_assets assets
  ON assets.tenant_id = session_files.tenant_id
 AND assets.id = session_files.file_id
WHERE session_files.tenant_id = sqlc.arg(tenant_id)
  AND session_files.session_id = sqlc.arg(session_id)
  AND session_files.file_id = sqlc.arg(file_id)
  AND session_files.context_version = sessions.context_version
  AND assets.deleted_at IS NULL;

-- name: ListCurrentAgentSessionFiles :many
SELECT
    assets.id, assets.tenant_id, session_files.session_id, session_files.context_version,
    assets.created_by_account_id, assets.original_filename,
    assets.object_provider, assets.object_bucket, assets.object_key,
    assets.content_type, assets.size_bytes, assets.sha256,
    assets.scan_status, assets.parse_status, assets.retention_class,
    session_files.state, assets.expires_at, assets.created_at, assets.updated_at
FROM agent_session_files session_files
JOIN agent_sessions sessions
  ON sessions.tenant_id = session_files.tenant_id
 AND sessions.id = session_files.session_id
JOIN file_assets assets
  ON assets.tenant_id = session_files.tenant_id
 AND assets.id = session_files.file_id
WHERE session_files.tenant_id = sqlc.arg(tenant_id)
  AND session_files.session_id = sqlc.arg(session_id)
  AND session_files.context_version = sessions.context_version
  AND assets.deleted_at IS NULL
ORDER BY session_files.created_at ASC, assets.id ASC;

-- name: MarkAgentSessionFileAttached :one
UPDATE agent_session_files session_files
SET state = 'attached', updated_at = sqlc.arg(updated_at)
FROM agent_sessions sessions
WHERE session_files.tenant_id = sqlc.arg(tenant_id)
  AND session_files.session_id = sqlc.arg(session_id)
  AND session_files.file_id = sqlc.arg(file_id)
  AND sessions.tenant_id = session_files.tenant_id
  AND sessions.id = session_files.session_id
  AND session_files.context_version = sessions.context_version
RETURNING session_files.*;

-- name: InsertAgentMessageAttachment :one
INSERT INTO agent_message_attachments (
    tenant_id, message_id, file_id, ordinal, created_at
) VALUES (
    sqlc.arg(tenant_id), sqlc.arg(message_id), sqlc.arg(file_id),
    sqlc.arg(ordinal), sqlc.arg(created_at)
)
RETURNING *;

-- name: ListCurrentAgentMessageAttachments :many
SELECT
    attachments.message_id, attachments.ordinal,
    assets.id, assets.tenant_id, session_files.session_id, session_files.context_version,
    assets.created_by_account_id, assets.original_filename,
    assets.object_provider, assets.object_bucket, assets.object_key,
    assets.content_type, assets.size_bytes, assets.sha256,
    assets.scan_status, assets.parse_status, assets.retention_class,
    session_files.state, assets.expires_at, assets.created_at, assets.updated_at
FROM agent_message_attachments attachments
JOIN agent_session_messages messages
  ON messages.tenant_id = attachments.tenant_id
 AND messages.id = attachments.message_id
JOIN agent_sessions sessions
  ON sessions.tenant_id = messages.tenant_id
 AND sessions.id = messages.session_id
JOIN agent_session_files session_files
  ON session_files.tenant_id = attachments.tenant_id
 AND session_files.session_id = messages.session_id
 AND session_files.file_id = attachments.file_id
JOIN file_assets assets
  ON assets.tenant_id = attachments.tenant_id
 AND assets.id = attachments.file_id
WHERE messages.tenant_id = sqlc.arg(tenant_id)
  AND messages.session_id = sqlc.arg(session_id)
  AND messages.context_version = sessions.context_version
  AND session_files.context_version = sessions.context_version
  AND assets.deleted_at IS NULL
ORDER BY messages.created_at ASC, messages.id ASC, attachments.ordinal ASC, assets.id ASC;

-- name: DeleteCurrentDraftAgentSessionFile :one
DELETE FROM agent_session_files session_files
USING agent_sessions sessions
WHERE session_files.tenant_id = sqlc.arg(tenant_id)
  AND session_files.session_id = sqlc.arg(session_id)
  AND session_files.file_id = sqlc.arg(file_id)
  AND session_files.state = 'draft'
  AND sessions.tenant_id = session_files.tenant_id
  AND sessions.id = session_files.session_id
  AND session_files.context_version = sessions.context_version
RETURNING session_files.file_id;

-- name: DeleteFileAsset :exec
DELETE FROM file_assets
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(file_id);
