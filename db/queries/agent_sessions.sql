-- name: UpsertAgentSession :one
INSERT INTO agent_sessions (
    id, tenant_id, account_id, agent_id, title, status,
    last_message_at, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(account_id), sqlc.arg(agent_id),
    sqlc.arg(title), sqlc.arg(status), sqlc.arg(last_message_at),
    sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    account_id = EXCLUDED.account_id,
    agent_id = EXCLUDED.agent_id,
    title = EXCLUDED.title,
    status = EXCLUDED.status,
    last_message_at = EXCLUDED.last_message_at,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetAgentSession :one
SELECT * FROM agent_sessions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- name: ListAgentSessionsByAccount :many
SELECT * FROM agent_sessions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND account_id = sqlc.arg(account_id)
  AND (sqlc.arg(status)::text = '' OR status = sqlc.arg(status))
  AND (sqlc.arg(agent_id)::text = '' OR agent_id = sqlc.arg(agent_id))
ORDER BY COALESCE(last_message_at, updated_at) DESC, id DESC;

-- name: DeleteAgentSession :one
DELETE FROM agent_sessions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
RETURNING *;

-- name: InsertAgentSessionMessage :one
INSERT INTO agent_session_messages (
    id, tenant_id, session_id, role, content, run_id, metadata, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(session_id), sqlc.arg(role),
    sqlc.arg(content), sqlc.arg(run_id), sqlc.arg(metadata)::jsonb, sqlc.arg(created_at)
)
RETURNING *;

-- name: ListAgentSessionMessages :many
SELECT * FROM agent_session_messages
WHERE tenant_id = sqlc.arg(tenant_id)
  AND session_id = sqlc.arg(session_id)
ORDER BY created_at ASC, id ASC;

-- name: ListRecentAgentSessionMessages :many
SELECT * FROM (
    SELECT * FROM agent_session_messages
    WHERE tenant_id = sqlc.arg(tenant_id)
      AND session_id = sqlc.arg(session_id)
    ORDER BY created_at DESC, id DESC
    LIMIT sqlc.arg(limit_count)::int
) recent
ORDER BY created_at ASC, id ASC;

-- name: UpsertAgentMemory :one
INSERT INTO agent_memories (
    id, tenant_id, account_id, agent_id, session_id, key, content,
    source, importance, expires_at, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(account_id), sqlc.arg(agent_id),
    sqlc.arg(session_id), sqlc.arg(key), sqlc.arg(content), sqlc.arg(source),
    sqlc.arg(importance), sqlc.arg(expires_at), sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    account_id = EXCLUDED.account_id,
    agent_id = EXCLUDED.agent_id,
    session_id = EXCLUDED.session_id,
    key = EXCLUDED.key,
    content = EXCLUDED.content,
    source = EXCLUDED.source,
    importance = EXCLUDED.importance,
    expires_at = EXCLUDED.expires_at,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetAgentMemory :one
SELECT * FROM agent_memories
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- name: ListAgentMemoriesByAccount :many
SELECT * FROM agent_memories
WHERE tenant_id = sqlc.arg(tenant_id)
  AND account_id = sqlc.arg(account_id)
  AND (sqlc.arg(agent_id)::text = '' OR agent_id = sqlc.arg(agent_id) OR agent_id IS NULL OR agent_id = '')
  AND (sqlc.arg(session_id)::text = '' OR session_id = sqlc.arg(session_id))
  AND (expires_at IS NULL OR expires_at > now())
ORDER BY importance DESC, updated_at DESC, id DESC
LIMIT sqlc.arg(limit_count)::int;

-- name: DeleteAgentMemory :one
DELETE FROM agent_memories
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
RETURNING *;
