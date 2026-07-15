-- name: UpsertAgentSession :one
INSERT INTO agent_sessions (
    id, tenant_id, account_id, agent_id, title, status,
    context_version, last_message_at, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(account_id), sqlc.arg(agent_id),
    sqlc.arg(title), sqlc.arg(status), sqlc.arg(context_version), sqlc.arg(last_message_at),
    sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    account_id = EXCLUDED.account_id,
    agent_id = EXCLUDED.agent_id,
    title = EXCLUDED.title,
    status = EXCLUDED.status,
    context_version = EXCLUDED.context_version,
    last_message_at = EXCLUDED.last_message_at,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetAgentSession :one
SELECT * FROM agent_sessions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- name: GetAgentSessionForUpdate :one
SELECT * FROM agent_sessions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
FOR UPDATE;

-- name: ListAgentSessionsByAccount :many
SELECT * FROM agent_sessions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND account_id = sqlc.arg(account_id)
  AND (sqlc.arg(status)::text = '' OR status = sqlc.arg(status))
  AND (sqlc.arg(agent_id)::text = '' OR agent_id = sqlc.arg(agent_id))
ORDER BY COALESCE(last_message_at, updated_at) DESC, id DESC;

-- name: ListAgentUsageByAccount :many
WITH session_usage AS (
    SELECT
        account_id,
        count(*)::bigint AS session_count,
        max(COALESCE(last_message_at, updated_at))::timestamptz AS last_session_at
    FROM agent_sessions sessions
    WHERE sessions.tenant_id = sqlc.arg(tenant_id)
    GROUP BY sessions.account_id
), message_usage AS (
    SELECT
        sessions.account_id,
        count(messages.id)::bigint AS message_count,
        max(messages.created_at)::timestamptz AS last_message_at
    FROM agent_sessions sessions
    JOIN agent_session_messages messages
      ON messages.tenant_id = sessions.tenant_id
     AND messages.session_id = sessions.id
    WHERE sessions.tenant_id = sqlc.arg(tenant_id)
    GROUP BY sessions.account_id
), run_usage AS (
    SELECT
        account_id,
        sum(llm_call_count)::bigint AS llm_call_count,
        sum(input_tokens)::bigint AS input_tokens,
        sum(cached_tokens)::bigint AS cached_tokens,
        sum(output_tokens)::bigint AS output_tokens,
        sum(total_tokens)::bigint AS total_tokens
    FROM agent_runs
    WHERE tenant_id = sqlc.arg(tenant_id)
    GROUP BY account_id
), account_usage AS (
SELECT
    accounts.id AS account_id,
    accounts.display_name,
    accounts.email,
    accounts.status,
    COALESCE(session_usage.session_count, 0)::bigint AS session_count,
    COALESCE(message_usage.message_count, 0)::bigint AS message_count,
    COALESCE(run_usage.llm_call_count, 0)::bigint AS llm_call_count,
    COALESCE(run_usage.input_tokens, 0)::bigint AS input_tokens,
    COALESCE(run_usage.cached_tokens, 0)::bigint AS cached_tokens,
    COALESCE(run_usage.output_tokens, 0)::bigint AS output_tokens,
    COALESCE(run_usage.total_tokens, 0)::bigint AS total_tokens,
    GREATEST(COALESCE(run_usage.total_tokens, 0) - COALESCE(run_usage.cached_tokens, 0), 0)::bigint AS actual_tokens,
    GREATEST(session_usage.last_session_at, message_usage.last_message_at)::timestamptz AS last_active_at
FROM accounts
LEFT JOIN session_usage ON session_usage.account_id = accounts.id
LEFT JOIN message_usage ON message_usage.account_id = accounts.id
LEFT JOIN run_usage ON run_usage.account_id = accounts.id
WHERE accounts.tenant_id = sqlc.arg(tenant_id)
)
SELECT *
FROM account_usage
WHERE (sqlc.arg(search_query)::text = '' OR lower(display_name || ' ' || email || ' ' || account_id) LIKE '%' || lower(sqlc.arg(search_query)) || '%')
  AND (sqlc.arg(account_status)::text = '' OR status = sqlc.arg(account_status))
ORDER BY
    CASE WHEN sqlc.arg(sort_order)::text = 'session_count_asc' THEN session_count END ASC,
    CASE WHEN sqlc.arg(sort_order)::text IN ('usage_desc', 'session_count_desc') THEN session_count END DESC,
    CASE WHEN sqlc.arg(sort_order)::text = 'message_count_asc' THEN message_count END ASC,
    CASE WHEN sqlc.arg(sort_order)::text IN ('usage_desc', 'message_count_desc') THEN message_count END DESC,
    CASE WHEN sqlc.arg(sort_order)::text = 'total_tokens_asc' THEN total_tokens END ASC,
    CASE WHEN sqlc.arg(sort_order)::text = 'total_tokens_desc' THEN total_tokens END DESC,
    CASE WHEN sqlc.arg(sort_order)::text = 'cached_tokens_asc' THEN cached_tokens END ASC,
    CASE WHEN sqlc.arg(sort_order)::text = 'cached_tokens_desc' THEN cached_tokens END DESC,
    CASE WHEN sqlc.arg(sort_order)::text = 'actual_tokens_asc' THEN actual_tokens END ASC,
    CASE WHEN sqlc.arg(sort_order)::text = 'actual_tokens_desc' THEN actual_tokens END DESC,
    CASE WHEN sqlc.arg(sort_order)::text = 'last_active_at_asc' THEN last_active_at END ASC NULLS LAST,
    CASE WHEN sqlc.arg(sort_order)::text = 'last_active_at_desc' THEN last_active_at END DESC NULLS LAST,
    lower(display_name), account_id
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;

-- name: CountAgentUsageByAccount :one
SELECT count(*)::bigint
FROM accounts
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(search_query)::text = '' OR lower(display_name || ' ' || email || ' ' || id) LIKE '%' || lower(sqlc.arg(search_query)) || '%')
  AND (sqlc.arg(account_status)::text = '' OR status = sqlc.arg(account_status));

-- name: GetAgentUsageByAccount :one
WITH session_usage AS (
    SELECT
        account_id,
        count(*)::bigint AS session_count,
        max(COALESCE(last_message_at, updated_at))::timestamptz AS last_session_at
    FROM agent_sessions sessions
    WHERE sessions.tenant_id = sqlc.arg(tenant_id)
    GROUP BY sessions.account_id
), message_usage AS (
    SELECT
        sessions.account_id,
        count(messages.id)::bigint AS message_count,
        max(messages.created_at)::timestamptz AS last_message_at
    FROM agent_sessions sessions
    JOIN agent_session_messages messages
      ON messages.tenant_id = sessions.tenant_id
     AND messages.session_id = sessions.id
    WHERE sessions.tenant_id = sqlc.arg(tenant_id)
    GROUP BY sessions.account_id
), run_usage AS (
    SELECT
        account_id,
        sum(llm_call_count)::bigint AS llm_call_count,
        sum(input_tokens)::bigint AS input_tokens,
        sum(cached_tokens)::bigint AS cached_tokens,
        sum(output_tokens)::bigint AS output_tokens,
        sum(total_tokens)::bigint AS total_tokens
    FROM agent_runs
    WHERE tenant_id = sqlc.arg(tenant_id)
    GROUP BY account_id
)
SELECT
    accounts.id AS account_id,
    accounts.display_name,
    accounts.email,
    accounts.status,
    COALESCE(session_usage.session_count, 0)::bigint AS session_count,
    COALESCE(message_usage.message_count, 0)::bigint AS message_count,
    COALESCE(run_usage.llm_call_count, 0)::bigint AS llm_call_count,
    COALESCE(run_usage.input_tokens, 0)::bigint AS input_tokens,
    COALESCE(run_usage.cached_tokens, 0)::bigint AS cached_tokens,
    COALESCE(run_usage.output_tokens, 0)::bigint AS output_tokens,
    COALESCE(run_usage.total_tokens, 0)::bigint AS total_tokens,
    GREATEST(COALESCE(run_usage.total_tokens, 0) - COALESCE(run_usage.cached_tokens, 0), 0)::bigint AS actual_tokens,
    GREATEST(session_usage.last_session_at, message_usage.last_message_at)::timestamptz AS last_active_at
FROM accounts
LEFT JOIN session_usage ON session_usage.account_id = accounts.id
LEFT JOIN message_usage ON message_usage.account_id = accounts.id
LEFT JOIN run_usage ON run_usage.account_id = accounts.id
WHERE accounts.tenant_id = sqlc.arg(tenant_id)
  AND accounts.id = sqlc.arg(account_id);

-- name: GetAgentUsageSummary :one
WITH session_usage AS (
    SELECT account_id, count(*)::bigint AS session_count
    FROM agent_sessions sessions
    WHERE sessions.tenant_id = sqlc.arg(tenant_id)
    GROUP BY sessions.account_id
), message_usage AS (
    SELECT sessions.account_id, count(messages.id)::bigint AS message_count
    FROM agent_sessions sessions
    JOIN agent_session_messages messages
      ON messages.tenant_id = sessions.tenant_id
     AND messages.session_id = sessions.id
    WHERE sessions.tenant_id = sqlc.arg(tenant_id)
    GROUP BY sessions.account_id
), run_usage AS (
    SELECT
        account_id,
        sum(llm_call_count)::bigint AS llm_call_count,
        sum(input_tokens)::bigint AS input_tokens,
        sum(cached_tokens)::bigint AS cached_tokens,
        sum(output_tokens)::bigint AS output_tokens,
        sum(total_tokens)::bigint AS total_tokens
    FROM agent_runs
    WHERE tenant_id = sqlc.arg(tenant_id)
    GROUP BY account_id
), account_usage AS (
    SELECT
        COALESCE(session_usage.session_count, 0)::bigint AS session_count,
        COALESCE(message_usage.message_count, 0)::bigint AS message_count,
        COALESCE(run_usage.llm_call_count, 0)::bigint AS llm_call_count,
        COALESCE(run_usage.input_tokens, 0)::bigint AS input_tokens,
        COALESCE(run_usage.cached_tokens, 0)::bigint AS cached_tokens,
        COALESCE(run_usage.output_tokens, 0)::bigint AS output_tokens,
        COALESCE(run_usage.total_tokens, 0)::bigint AS total_tokens,
        GREATEST(COALESCE(run_usage.total_tokens, 0) - COALESCE(run_usage.cached_tokens, 0), 0)::bigint AS actual_tokens
    FROM accounts
    LEFT JOIN session_usage ON session_usage.account_id = accounts.id
    LEFT JOIN message_usage ON message_usage.account_id = accounts.id
    LEFT JOIN run_usage ON run_usage.account_id = accounts.id
    WHERE accounts.tenant_id = sqlc.arg(tenant_id)
)
SELECT
    count(*)::bigint AS user_count,
    count(*) FILTER (WHERE session_count > 0 OR message_count > 0)::bigint AS users_with_usage,
    COALESCE(sum(session_count), 0)::bigint AS session_count,
    COALESCE(sum(message_count), 0)::bigint AS message_count,
    COALESCE(sum(llm_call_count), 0)::bigint AS llm_call_count,
    COALESCE(sum(input_tokens), 0)::bigint AS input_tokens,
    COALESCE(sum(cached_tokens), 0)::bigint AS cached_tokens,
    COALESCE(sum(output_tokens), 0)::bigint AS output_tokens,
    COALESCE(sum(total_tokens), 0)::bigint AS total_tokens,
    COALESCE(sum(actual_tokens), 0)::bigint AS actual_tokens
FROM account_usage;

-- name: ListAgentUsageBySession :many
WITH account_sessions AS (
	SELECT id
	FROM agent_sessions
	WHERE tenant_id = sqlc.arg(tenant_id)
	  AND account_id = sqlc.arg(account_id)
), message_usage AS (
    SELECT
		messages.session_id,
        count(*)::bigint AS message_count,
        max(messages.created_at)::timestamptz AS last_message_at
    FROM agent_session_messages messages
	JOIN account_sessions ON account_sessions.id = messages.session_id
    WHERE messages.tenant_id = sqlc.arg(tenant_id)
    GROUP BY messages.session_id
), run_usage AS (
    SELECT
		runs.session_id,
        sum(llm_call_count)::bigint AS llm_call_count,
        sum(input_tokens)::bigint AS input_tokens,
        sum(cached_tokens)::bigint AS cached_tokens,
        sum(output_tokens)::bigint AS output_tokens,
        sum(total_tokens)::bigint AS total_tokens,
        max(updated_at)::timestamptz AS last_run_at
	FROM agent_runs runs
	JOIN account_sessions ON account_sessions.id = runs.session_id
	WHERE runs.tenant_id = sqlc.arg(tenant_id)
	GROUP BY runs.session_id
)
SELECT
    sessions.id AS session_id,
    sessions.account_id,
    sessions.title,
    sessions.status,
    COALESCE(message_usage.message_count, 0)::bigint AS message_count,
    COALESCE(run_usage.llm_call_count, 0)::bigint AS llm_call_count,
    COALESCE(run_usage.input_tokens, 0)::bigint AS input_tokens,
    COALESCE(run_usage.cached_tokens, 0)::bigint AS cached_tokens,
    COALESCE(run_usage.output_tokens, 0)::bigint AS output_tokens,
    COALESCE(run_usage.total_tokens, 0)::bigint AS total_tokens,
    GREATEST(COALESCE(run_usage.total_tokens, 0) - COALESCE(run_usage.cached_tokens, 0), 0)::bigint AS actual_tokens,
    GREATEST(COALESCE(sessions.last_message_at, sessions.updated_at), message_usage.last_message_at, run_usage.last_run_at)::timestamptz AS last_active_at
FROM agent_sessions sessions
LEFT JOIN message_usage ON message_usage.session_id = sessions.id
LEFT JOIN run_usage ON run_usage.session_id = sessions.id
WHERE sessions.tenant_id = sqlc.arg(tenant_id)
  AND sessions.account_id = sqlc.arg(account_id)
ORDER BY last_active_at DESC, sessions.id DESC
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;

-- name: CountAgentUsageSessionsByAccount :one
SELECT count(*)::bigint
FROM agent_sessions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND account_id = sqlc.arg(account_id);

-- name: DeleteAgentSession :one
DELETE FROM agent_sessions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
RETURNING *;

-- name: InsertAgentSessionMessage :one
INSERT INTO agent_session_messages (
    id, tenant_id, session_id, role, content, run_id, context_version, metadata, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(session_id), sqlc.arg(role),
    sqlc.arg(content), sqlc.arg(run_id), sqlc.arg(context_version), sqlc.arg(metadata)::jsonb, sqlc.arg(created_at)
)
RETURNING *;

-- name: ListAgentSessionMessages :many
SELECT messages.*
FROM agent_session_messages messages
JOIN agent_sessions sessions
  ON sessions.tenant_id = messages.tenant_id
 AND sessions.id = messages.session_id
WHERE messages.tenant_id = sqlc.arg(tenant_id)
  AND messages.session_id = sqlc.arg(session_id)
  AND messages.context_version = sessions.context_version
ORDER BY messages.created_at ASC, messages.id ASC;

-- name: ListRecentAgentSessionMessages :many
SELECT * FROM (
    SELECT messages.*
    FROM agent_session_messages messages
    JOIN agent_sessions sessions
      ON sessions.tenant_id = messages.tenant_id
     AND sessions.id = messages.session_id
    WHERE messages.tenant_id = sqlc.arg(tenant_id)
      AND messages.session_id = sqlc.arg(session_id)
      AND messages.context_version = sessions.context_version
    ORDER BY messages.created_at DESC, messages.id DESC
    LIMIT sqlc.arg(limit_count)::int
) recent
ORDER BY created_at ASC, id ASC;

-- name: CountActiveAgentRunsBySession :one
SELECT count(*) FROM agent_runs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND session_id = sqlc.arg(session_id)
  AND status IN ('queued', 'running');

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
