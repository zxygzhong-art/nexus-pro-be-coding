-- File assets remain shared infrastructure; conversation bindings use the v2 segment model.

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
WHERE file_assets.tenant_id = EXCLUDED.tenant_id
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
INSERT INTO conversation_files (
    id, tenant_id, conversation_id, segment_id, file_asset_id,
    state, created_at, updated_at
)
SELECT
    COALESCE(
        NULLIF(sqlc.arg(conversation_file_id)::text, ''),
        conversations.id || ':segment:' || segments.id || ':file:' || sqlc.arg(file_id)::text
    ),
    conversations.tenant_id,
    conversations.id,
    segments.id,
    sqlc.arg(file_id),
    sqlc.arg(state),
    sqlc.arg(created_at),
    sqlc.arg(updated_at)
FROM conversations
JOIN conversation_segments segments
  ON segments.tenant_id = conversations.tenant_id
 AND segments.conversation_id = conversations.id
 AND segments.id = conversations.current_segment_id
WHERE conversations.tenant_id = sqlc.arg(tenant_id)
  AND conversations.id = sqlc.arg(session_id)
  AND segments.ordinal = GREATEST(sqlc.arg(context_version)::bigint, 1)::integer
ON CONFLICT (tenant_id, conversation_id, segment_id, file_asset_id) DO UPDATE SET
    state = EXCLUDED.state,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetCurrentAgentSessionFile :one
SELECT
    assets.id,
    assets.tenant_id,
    conversation_files.conversation_id AS session_id,
    conversation_files.segment_id,
    conversation_files.id AS conversation_file_id,
    segments.ordinal::bigint AS context_version,
    assets.created_by_account_id,
    assets.original_filename,
    assets.object_provider,
    assets.object_bucket,
    assets.object_key,
    assets.content_type,
    assets.size_bytes,
    assets.sha256,
    assets.scan_status,
    assets.parse_status,
    assets.retention_class,
    conversation_files.state,
    assets.expires_at,
    assets.created_at,
    assets.updated_at
FROM conversation_files
JOIN conversations
  ON conversations.tenant_id = conversation_files.tenant_id
 AND conversations.id = conversation_files.conversation_id
 AND conversations.current_segment_id = conversation_files.segment_id
JOIN conversation_segments segments
  ON segments.tenant_id = conversation_files.tenant_id
 AND segments.conversation_id = conversation_files.conversation_id
 AND segments.id = conversation_files.segment_id
JOIN file_assets assets
  ON assets.tenant_id = conversation_files.tenant_id
 AND assets.id = conversation_files.file_asset_id
WHERE conversation_files.tenant_id = sqlc.arg(tenant_id)
  AND conversation_files.conversation_id = sqlc.arg(session_id)
  AND conversation_files.file_asset_id = sqlc.arg(file_id)
  AND assets.deleted_at IS NULL;

-- name: ListCurrentAgentSessionFiles :many
SELECT
    assets.id,
    assets.tenant_id,
    conversation_files.conversation_id AS session_id,
    conversation_files.segment_id,
    conversation_files.id AS conversation_file_id,
    segments.ordinal::bigint AS context_version,
    assets.created_by_account_id,
    assets.original_filename,
    assets.object_provider,
    assets.object_bucket,
    assets.object_key,
    assets.content_type,
    assets.size_bytes,
    assets.sha256,
    assets.scan_status,
    assets.parse_status,
    assets.retention_class,
    conversation_files.state,
    assets.expires_at,
    assets.created_at,
    assets.updated_at
FROM conversation_files
JOIN conversations
  ON conversations.tenant_id = conversation_files.tenant_id
 AND conversations.id = conversation_files.conversation_id
 AND conversations.current_segment_id = conversation_files.segment_id
JOIN conversation_segments segments
  ON segments.tenant_id = conversation_files.tenant_id
 AND segments.conversation_id = conversation_files.conversation_id
 AND segments.id = conversation_files.segment_id
JOIN file_assets assets
  ON assets.tenant_id = conversation_files.tenant_id
 AND assets.id = conversation_files.file_asset_id
WHERE conversation_files.tenant_id = sqlc.arg(tenant_id)
  AND conversation_files.conversation_id = sqlc.arg(session_id)
  AND assets.deleted_at IS NULL
ORDER BY conversation_files.created_at ASC, assets.id ASC;

-- name: MarkAgentSessionFileAttached :one
UPDATE conversation_files
SET state = 'attached', updated_at = sqlc.arg(updated_at)
FROM conversations
WHERE conversation_files.tenant_id = sqlc.arg(tenant_id)
  AND conversation_files.conversation_id = sqlc.arg(session_id)
  AND conversation_files.file_asset_id = sqlc.arg(file_id)
  AND conversations.tenant_id = conversation_files.tenant_id
  AND conversations.id = conversation_files.conversation_id
  AND conversations.current_segment_id = conversation_files.segment_id
RETURNING conversation_files.*;

-- name: InsertAgentMessageAttachment :one
INSERT INTO message_attachments (
    tenant_id, conversation_id, segment_id, message_id,
    conversation_file_id, ordinal, created_at
)
SELECT
    messages.tenant_id,
    messages.conversation_id,
    messages.segment_id,
    messages.id,
    conversation_files.id,
    sqlc.arg(ordinal),
    sqlc.arg(created_at)
FROM messages
JOIN conversation_files
  ON conversation_files.tenant_id = messages.tenant_id
 AND conversation_files.conversation_id = messages.conversation_id
 AND conversation_files.segment_id = messages.segment_id
 AND conversation_files.file_asset_id = sqlc.arg(file_id)
WHERE messages.tenant_id = sqlc.arg(tenant_id)
  AND messages.id = sqlc.arg(message_id)
ON CONFLICT (tenant_id, message_id, conversation_file_id) DO UPDATE SET
    ordinal = EXCLUDED.ordinal
RETURNING *;

-- name: ListCurrentAgentMessageAttachments :many
SELECT
    attachments.message_id,
    attachments.conversation_file_id,
    attachments.ordinal,
    assets.id,
    assets.tenant_id,
    conversation_files.conversation_id AS session_id,
    conversation_files.segment_id,
    segments.ordinal::bigint AS context_version,
    assets.created_by_account_id,
    assets.original_filename,
    assets.object_provider,
    assets.object_bucket,
    assets.object_key,
    assets.content_type,
    assets.size_bytes,
    assets.sha256,
    assets.scan_status,
    assets.parse_status,
    assets.retention_class,
    conversation_files.state,
    assets.expires_at,
    assets.created_at,
    assets.updated_at
FROM message_attachments attachments
JOIN messages
  ON messages.tenant_id = attachments.tenant_id
 AND messages.conversation_id = attachments.conversation_id
 AND messages.segment_id = attachments.segment_id
 AND messages.id = attachments.message_id
JOIN conversations
  ON conversations.tenant_id = messages.tenant_id
 AND conversations.id = messages.conversation_id
 AND conversations.current_segment_id = messages.segment_id
JOIN conversation_files
  ON conversation_files.tenant_id = attachments.tenant_id
 AND conversation_files.conversation_id = attachments.conversation_id
 AND conversation_files.segment_id = attachments.segment_id
 AND conversation_files.id = attachments.conversation_file_id
JOIN conversation_segments segments
  ON segments.tenant_id = conversation_files.tenant_id
 AND segments.conversation_id = conversation_files.conversation_id
 AND segments.id = conversation_files.segment_id
JOIN file_assets assets
  ON assets.tenant_id = conversation_files.tenant_id
 AND assets.id = conversation_files.file_asset_id
WHERE messages.tenant_id = sqlc.arg(tenant_id)
  AND messages.conversation_id = sqlc.arg(session_id)
  AND assets.deleted_at IS NULL
ORDER BY messages.sequence_no ASC, attachments.ordinal ASC, assets.id ASC;

-- name: DeleteCurrentDraftAgentSessionFile :one
WITH target AS (
    SELECT assets.tenant_id, assets.id, assets.updated_at
    FROM conversation_files
    JOIN conversations
      ON conversations.tenant_id = conversation_files.tenant_id
     AND conversations.id = conversation_files.conversation_id
     AND conversations.current_segment_id = conversation_files.segment_id
    JOIN file_assets assets
      ON assets.tenant_id = conversation_files.tenant_id
     AND assets.id = conversation_files.file_asset_id
    WHERE conversation_files.tenant_id = sqlc.arg(tenant_id)
      AND conversation_files.conversation_id = sqlc.arg(session_id)
      AND conversation_files.file_asset_id = sqlc.arg(file_id)
      AND conversation_files.state = 'draft'
      AND assets.deleted_at IS NULL
), soft_deleted AS (
    UPDATE file_assets
    SET deleted_at = COALESCE(deleted_at, now()),
        updated_at = GREATEST(file_assets.updated_at, now())
    FROM target
    WHERE file_assets.tenant_id = target.tenant_id
      AND file_assets.id = target.id
    RETURNING file_assets.id
)
SELECT id AS file_id FROM soft_deleted;

-- name: DeleteFileAsset :exec
UPDATE file_assets
SET deleted_at = COALESCE(deleted_at, now()),
    updated_at = GREATEST(updated_at, now())
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(file_id);
