-- Legacy AgentSession compatibility over the v2 conversation/segment model.

-- name: UpsertAgentSession :one
WITH upserted_conversation AS (
    INSERT INTO conversations (
        id, tenant_id, owner_account_id, agent_id, current_segment_id,
        next_message_sequence, title, status, last_message_at,
        created_at, updated_at, archived_at
    ) VALUES (
        sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(account_id),
        NULLIF(sqlc.arg(agent_id)::text, ''), NULL, 1,
        sqlc.arg(title), sqlc.arg(status), sqlc.arg(last_message_at),
        sqlc.arg(created_at), sqlc.arg(updated_at),
        CASE WHEN sqlc.arg(status)::text = 'archived' THEN sqlc.arg(updated_at)::timestamptz ELSE NULL END
    )
    ON CONFLICT (id) DO UPDATE SET
        owner_account_id = EXCLUDED.owner_account_id,
        agent_id = EXCLUDED.agent_id,
        title = EXCLUDED.title,
        status = EXCLUDED.status,
        last_message_at = EXCLUDED.last_message_at,
        updated_at = EXCLUDED.updated_at,
        archived_at = EXCLUDED.archived_at
    WHERE conversations.tenant_id = EXCLUDED.tenant_id
    RETURNING *
), inserted_segment AS (
    INSERT INTO conversation_segments (
        id, tenant_id, conversation_id, ordinal, start_reason, created_at
    )
    SELECT
        COALESCE(
            NULLIF(sqlc.arg(segment_id)::text, ''),
            sqlc.arg(id)::text || ':segment:' || GREATEST(sqlc.arg(context_version)::bigint, 1)::text
        ),
        tenant_id,
        id,
        GREATEST(sqlc.arg(context_version)::bigint, 1)::integer,
        CASE WHEN GREATEST(sqlc.arg(context_version)::bigint, 1) = 1 THEN 'initial' ELSE 'context_reset' END,
        sqlc.arg(updated_at)
    FROM upserted_conversation
    ON CONFLICT (tenant_id, conversation_id, ordinal) DO NOTHING
    RETURNING *
), target_segment AS (
    SELECT id, tenant_id, conversation_id, ordinal
    FROM inserted_segment
    UNION ALL
    SELECT segments.id, segments.tenant_id, segments.conversation_id, segments.ordinal
    FROM conversation_segments segments
    JOIN upserted_conversation conversations
      ON conversations.tenant_id = segments.tenant_id
     AND conversations.id = segments.conversation_id
    WHERE segments.ordinal = GREATEST(sqlc.arg(context_version)::bigint, 1)::integer
    LIMIT 1
), updated_conversation AS (
    UPDATE conversations
    SET current_segment_id = target_segment.id
    FROM target_segment
    WHERE conversations.tenant_id = target_segment.tenant_id
      AND conversations.id = target_segment.conversation_id
    RETURNING conversations.*
)
SELECT
    conversations.id,
    conversations.tenant_id,
    conversations.owner_account_id AS account_id,
    conversations.agent_id,
    target_segment.id AS segment_id,
    conversations.title,
    conversations.status,
    target_segment.ordinal::bigint AS context_version,
    conversations.last_message_at,
    conversations.created_at,
    conversations.updated_at
FROM updated_conversation conversations
JOIN target_segment
  ON target_segment.tenant_id = conversations.tenant_id
 AND target_segment.conversation_id = conversations.id;

-- name: UpsertAgentChatExecution :execrows
INSERT INTO conversation_executions (
    id, tenant_id, account_id, conversation_id, segment_id, input_message_id,
    agent_id, agent_revision_id, model_connection_id,
    mode, trigger_type, status, queued_at, started_at, completed_at,
    error_code, error_category, safe_error_message,
    llm_call_count, input_tokens, cached_tokens, output_tokens, total_tokens,
    usage_complete, created_at, updated_at
)
SELECT
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(account_id),
    messages.conversation_id, messages.segment_id, messages.id,
    NULLIF(sqlc.arg(agent_id)::text, ''),
    NULLIF(sqlc.arg(agent_revision_id)::text, ''),
    NULLIF(sqlc.arg(model_connection_id)::text, ''),
    sqlc.arg(mode), 'chat', sqlc.arg(status), sqlc.arg(created_at),
    CASE
        WHEN sqlc.arg(status)::text = 'queued' THEN NULL
        ELSE sqlc.arg(updated_at)::timestamptz
    END,
    CASE
        WHEN sqlc.arg(status)::text IN ('completed', 'failed', 'cancelled')
            THEN sqlc.arg(updated_at)::timestamptz
        ELSE NULL
    END,
    sqlc.arg(error_code), sqlc.arg(error_category),
    CASE WHEN sqlc.arg(status)::text = 'failed' THEN sqlc.arg(safe_error_message)::text ELSE '' END,
    sqlc.arg(llm_call_count), sqlc.arg(input_tokens), sqlc.arg(cached_tokens),
    sqlc.arg(output_tokens), sqlc.arg(total_tokens), sqlc.arg(usage_complete),
    sqlc.arg(created_at), sqlc.arg(updated_at)
FROM conversation_messages messages
JOIN conversations
  ON conversations.tenant_id = messages.tenant_id
 AND conversations.id = messages.conversation_id
 AND conversations.current_segment_id = messages.segment_id
WHERE messages.tenant_id = sqlc.arg(tenant_id)
  AND messages.id = sqlc.arg(input_message_id)
  AND messages.conversation_id = sqlc.arg(session_id)
  AND messages.segment_id = sqlc.arg(segment_id)
  AND messages.role = 'user'
  AND conversations.owner_account_id = sqlc.arg(account_id)
ON CONFLICT (id) DO UPDATE SET
    status = EXCLUDED.status,
    started_at = CASE
        WHEN EXCLUDED.status = 'queued' THEN conversation_executions.started_at
        ELSE COALESCE(conversation_executions.started_at, EXCLUDED.started_at)
    END,
    completed_at = EXCLUDED.completed_at,
    error_code = EXCLUDED.error_code,
    error_category = EXCLUDED.error_category,
    safe_error_message = EXCLUDED.safe_error_message,
    llm_call_count = EXCLUDED.llm_call_count,
    input_tokens = EXCLUDED.input_tokens,
    cached_tokens = EXCLUDED.cached_tokens,
    output_tokens = EXCLUDED.output_tokens,
    total_tokens = EXCLUDED.total_tokens,
    usage_complete = EXCLUDED.usage_complete,
    updated_at = EXCLUDED.updated_at
WHERE conversation_executions.tenant_id = EXCLUDED.tenant_id;

-- name: GetAgentSession :one
SELECT
    conversations.id,
    conversations.tenant_id,
    conversations.owner_account_id AS account_id,
    conversations.agent_id,
    segments.id AS segment_id,
    conversations.title,
    conversations.status,
    segments.ordinal::bigint AS context_version,
    conversations.last_message_at,
    conversations.created_at,
    conversations.updated_at
FROM conversations
JOIN conversation_segments segments
  ON segments.tenant_id = conversations.tenant_id
 AND segments.conversation_id = conversations.id
 AND segments.id = conversations.current_segment_id
WHERE conversations.tenant_id = sqlc.arg(tenant_id)
  AND conversations.id = sqlc.arg(id);

-- name: GetAgentSessionForUpdate :one
SELECT
    conversations.id,
    conversations.tenant_id,
    conversations.owner_account_id AS account_id,
    conversations.agent_id,
    segments.id AS segment_id,
    conversations.title,
    conversations.status,
    segments.ordinal::bigint AS context_version,
    conversations.last_message_at,
    conversations.created_at,
    conversations.updated_at
FROM conversations
JOIN conversation_segments segments
  ON segments.tenant_id = conversations.tenant_id
 AND segments.conversation_id = conversations.id
 AND segments.id = conversations.current_segment_id
WHERE conversations.tenant_id = sqlc.arg(tenant_id)
  AND conversations.id = sqlc.arg(id)
FOR UPDATE OF conversations;

-- name: ListAgentSessionsByAccount :many
SELECT
    conversations.id,
    conversations.tenant_id,
    conversations.owner_account_id AS account_id,
    conversations.agent_id,
    segments.id AS segment_id,
    conversations.title,
    conversations.status,
    segments.ordinal::bigint AS context_version,
    conversations.last_message_at,
    conversations.created_at,
    conversations.updated_at
FROM conversations
JOIN conversation_segments segments
  ON segments.tenant_id = conversations.tenant_id
 AND segments.conversation_id = conversations.id
 AND segments.id = conversations.current_segment_id
WHERE conversations.tenant_id = sqlc.arg(tenant_id)
  AND conversations.owner_account_id = sqlc.arg(account_id)
  AND (sqlc.arg(status)::text = '' OR conversations.status = sqlc.arg(status))
  AND (sqlc.arg(agent_id)::text = '' OR conversations.agent_id = sqlc.arg(agent_id))
  AND (
    NOT sqlc.arg(has_cursor)::boolean
    OR conversations.created_at < sqlc.arg(cursor_created_at)::timestamptz
    OR (conversations.created_at = sqlc.arg(cursor_created_at)::timestamptz AND conversations.id < sqlc.arg(cursor_id))
  )
ORDER BY conversations.created_at DESC, conversations.id DESC
LIMIT sqlc.arg(limit_count)::int;

-- name: ListAgentUsageByAccount :many
WITH conversation_usage AS (
    SELECT
        owner_account_id AS account_id,
        count(*)::bigint AS session_count,
        max(COALESCE(last_message_at, updated_at))::timestamptz AS last_session_at
    FROM conversations
    WHERE conversations.tenant_id = sqlc.arg(tenant_id)
    GROUP BY conversations.owner_account_id
), message_usage AS (
    SELECT
        conversations.owner_account_id AS account_id,
        count(conversation_messages.id)::bigint AS message_count,
        max(conversation_messages.created_at)::timestamptz AS last_message_at
    FROM conversations
    JOIN conversation_messages
      ON conversation_messages.tenant_id = conversations.tenant_id
     AND conversation_messages.conversation_id = conversations.id
    WHERE conversations.tenant_id = sqlc.arg(tenant_id)
    GROUP BY conversations.owner_account_id
), execution_usage AS (
    SELECT
        account_id,
        sum(llm_call_count)::bigint AS llm_call_count,
        sum(input_tokens)::bigint AS input_tokens,
        sum(cached_tokens)::bigint AS cached_tokens,
        sum(output_tokens)::bigint AS output_tokens,
        sum(total_tokens)::bigint AS total_tokens,
        max(updated_at)::timestamptz AS last_execution_at
    FROM conversation_executions
    WHERE tenant_id = sqlc.arg(tenant_id)
    GROUP BY account_id
), account_usage AS (
    SELECT
        accounts.id AS account_id,
        accounts.display_name,
        accounts.email,
        accounts.status,
        COALESCE(conversation_usage.session_count, 0)::bigint AS session_count,
        COALESCE(message_usage.message_count, 0)::bigint AS message_count,
        COALESCE(execution_usage.llm_call_count, 0)::bigint AS llm_call_count,
        COALESCE(execution_usage.input_tokens, 0)::bigint AS input_tokens,
        COALESCE(execution_usage.cached_tokens, 0)::bigint AS cached_tokens,
        COALESCE(execution_usage.output_tokens, 0)::bigint AS output_tokens,
        COALESCE(execution_usage.total_tokens, 0)::bigint AS total_tokens,
        GREATEST(COALESCE(execution_usage.total_tokens, 0) - COALESCE(execution_usage.cached_tokens, 0), 0)::bigint AS actual_tokens,
        GREATEST(conversation_usage.last_session_at, message_usage.last_message_at, execution_usage.last_execution_at)::timestamptz AS last_active_at
    FROM accounts
    LEFT JOIN conversation_usage ON conversation_usage.account_id = accounts.id
    LEFT JOIN message_usage ON message_usage.account_id = accounts.id
    LEFT JOIN execution_usage ON execution_usage.account_id = accounts.id
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
WITH conversation_usage AS (
    SELECT owner_account_id AS account_id, count(*)::bigint AS session_count,
           max(COALESCE(last_message_at, updated_at))::timestamptz AS last_session_at
    FROM conversations
    WHERE tenant_id = sqlc.arg(tenant_id)
    GROUP BY owner_account_id
), message_usage AS (
    SELECT conversations.owner_account_id AS account_id, count(conversation_messages.id)::bigint AS message_count,
           max(conversation_messages.created_at)::timestamptz AS last_message_at
    FROM conversations
    JOIN conversation_messages ON conversation_messages.tenant_id = conversations.tenant_id AND conversation_messages.conversation_id = conversations.id
    WHERE conversations.tenant_id = sqlc.arg(tenant_id)
    GROUP BY conversations.owner_account_id
), execution_usage AS (
    SELECT account_id, sum(llm_call_count)::bigint AS llm_call_count,
           sum(input_tokens)::bigint AS input_tokens, sum(cached_tokens)::bigint AS cached_tokens,
           sum(output_tokens)::bigint AS output_tokens, sum(total_tokens)::bigint AS total_tokens,
           max(updated_at)::timestamptz AS last_execution_at
    FROM conversation_executions
    WHERE tenant_id = sqlc.arg(tenant_id)
    GROUP BY account_id
)
SELECT
    accounts.id AS account_id, accounts.display_name, accounts.email, accounts.status,
    COALESCE(conversation_usage.session_count, 0)::bigint AS session_count,
    COALESCE(message_usage.message_count, 0)::bigint AS message_count,
    COALESCE(execution_usage.llm_call_count, 0)::bigint AS llm_call_count,
    COALESCE(execution_usage.input_tokens, 0)::bigint AS input_tokens,
    COALESCE(execution_usage.cached_tokens, 0)::bigint AS cached_tokens,
    COALESCE(execution_usage.output_tokens, 0)::bigint AS output_tokens,
    COALESCE(execution_usage.total_tokens, 0)::bigint AS total_tokens,
    GREATEST(COALESCE(execution_usage.total_tokens, 0) - COALESCE(execution_usage.cached_tokens, 0), 0)::bigint AS actual_tokens,
    GREATEST(conversation_usage.last_session_at, message_usage.last_message_at, execution_usage.last_execution_at)::timestamptz AS last_active_at
FROM accounts
LEFT JOIN conversation_usage ON conversation_usage.account_id = accounts.id
LEFT JOIN message_usage ON message_usage.account_id = accounts.id
LEFT JOIN execution_usage ON execution_usage.account_id = accounts.id
WHERE accounts.tenant_id = sqlc.arg(tenant_id)
  AND accounts.id = sqlc.arg(account_id);

-- name: GetAgentUsageSummary :one
WITH conversation_usage AS (
    SELECT owner_account_id AS account_id, count(*)::bigint AS session_count
    FROM conversations WHERE conversations.tenant_id = sqlc.arg(tenant_id) GROUP BY conversations.owner_account_id
), message_usage AS (
    SELECT conversations.owner_account_id AS account_id, count(conversation_messages.id)::bigint AS message_count
    FROM conversations
    JOIN conversation_messages ON conversation_messages.tenant_id = conversations.tenant_id AND conversation_messages.conversation_id = conversations.id
    WHERE conversations.tenant_id = sqlc.arg(tenant_id)
    GROUP BY conversations.owner_account_id
), execution_usage AS (
    SELECT account_id, sum(llm_call_count)::bigint AS llm_call_count,
           sum(input_tokens)::bigint AS input_tokens, sum(cached_tokens)::bigint AS cached_tokens,
           sum(output_tokens)::bigint AS output_tokens, sum(total_tokens)::bigint AS total_tokens
    FROM conversation_executions WHERE tenant_id = sqlc.arg(tenant_id) GROUP BY account_id
), account_usage AS (
    SELECT
        COALESCE(conversation_usage.session_count, 0)::bigint AS session_count,
        COALESCE(message_usage.message_count, 0)::bigint AS message_count,
        COALESCE(execution_usage.llm_call_count, 0)::bigint AS llm_call_count,
        COALESCE(execution_usage.input_tokens, 0)::bigint AS input_tokens,
        COALESCE(execution_usage.cached_tokens, 0)::bigint AS cached_tokens,
        COALESCE(execution_usage.output_tokens, 0)::bigint AS output_tokens,
        COALESCE(execution_usage.total_tokens, 0)::bigint AS total_tokens,
        GREATEST(COALESCE(execution_usage.total_tokens, 0) - COALESCE(execution_usage.cached_tokens, 0), 0)::bigint AS actual_tokens
    FROM accounts
    LEFT JOIN conversation_usage ON conversation_usage.account_id = accounts.id
    LEFT JOIN message_usage ON message_usage.account_id = accounts.id
    LEFT JOIN execution_usage ON execution_usage.account_id = accounts.id
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
WITH account_conversations AS (
    SELECT id
    FROM conversations
    WHERE tenant_id = sqlc.arg(tenant_id) AND owner_account_id = sqlc.arg(account_id)
), message_usage AS (
    SELECT conversation_messages.conversation_id, count(*)::bigint AS message_count,
           max(conversation_messages.created_at)::timestamptz AS last_message_at
    FROM conversation_messages
    JOIN account_conversations ON account_conversations.id = conversation_messages.conversation_id
    WHERE conversation_messages.tenant_id = sqlc.arg(tenant_id)
    GROUP BY conversation_messages.conversation_id
), execution_usage AS (
    SELECT conversation_executions.conversation_id, sum(llm_call_count)::bigint AS llm_call_count,
           sum(input_tokens)::bigint AS input_tokens, sum(cached_tokens)::bigint AS cached_tokens,
           sum(output_tokens)::bigint AS output_tokens, sum(total_tokens)::bigint AS total_tokens,
           max(updated_at)::timestamptz AS last_execution_at
    FROM conversation_executions
    JOIN account_conversations ON account_conversations.id = conversation_executions.conversation_id
    WHERE conversation_executions.tenant_id = sqlc.arg(tenant_id)
    GROUP BY conversation_executions.conversation_id
)
SELECT
    conversations.id AS session_id,
    conversations.owner_account_id AS account_id,
    conversations.title,
    conversations.status,
    COALESCE(message_usage.message_count, 0)::bigint AS message_count,
    COALESCE(execution_usage.llm_call_count, 0)::bigint AS llm_call_count,
    COALESCE(execution_usage.input_tokens, 0)::bigint AS input_tokens,
    COALESCE(execution_usage.cached_tokens, 0)::bigint AS cached_tokens,
    COALESCE(execution_usage.output_tokens, 0)::bigint AS output_tokens,
    COALESCE(execution_usage.total_tokens, 0)::bigint AS total_tokens,
    GREATEST(COALESCE(execution_usage.total_tokens, 0) - COALESCE(execution_usage.cached_tokens, 0), 0)::bigint AS actual_tokens,
    GREATEST(COALESCE(conversations.last_message_at, conversations.updated_at), message_usage.last_message_at, execution_usage.last_execution_at)::timestamptz AS last_active_at
FROM conversations
LEFT JOIN message_usage ON message_usage.conversation_id = conversations.id
LEFT JOIN execution_usage ON execution_usage.conversation_id = conversations.id
WHERE conversations.tenant_id = sqlc.arg(tenant_id)
  AND conversations.owner_account_id = sqlc.arg(account_id)
ORDER BY last_active_at DESC, conversations.id DESC
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;

-- name: CountAgentUsageSessionsByAccount :one
SELECT count(*)::bigint
FROM conversations
WHERE tenant_id = sqlc.arg(tenant_id)
  AND owner_account_id = sqlc.arg(account_id);

-- name: DeleteAgentSession :one
WITH archived AS (
    UPDATE conversations
    SET status = 'archived',
        archived_at = COALESCE(archived_at, now()),
        updated_at = GREATEST(updated_at, now())
    WHERE conversations.tenant_id = sqlc.arg(tenant_id)
      AND conversations.id = sqlc.arg(id)
    RETURNING *
)
SELECT
    archived.id,
    archived.tenant_id,
    archived.owner_account_id AS account_id,
    archived.agent_id,
    segments.id AS segment_id,
    archived.title,
    archived.status,
    segments.ordinal::bigint AS context_version,
    archived.last_message_at,
    archived.created_at,
    archived.updated_at
FROM archived
JOIN conversation_segments segments
  ON segments.tenant_id = archived.tenant_id
 AND segments.conversation_id = archived.id
 AND segments.id = archived.current_segment_id;

-- name: InsertAgentSessionMessage :one
WITH allocated AS (
    UPDATE conversations
    SET next_message_sequence = next_message_sequence + 1,
        last_message_at = GREATEST(COALESCE(last_message_at, sqlc.arg(created_at)::timestamptz), sqlc.arg(created_at)::timestamptz),
        updated_at = GREATEST(updated_at, sqlc.arg(created_at)::timestamptz)
    WHERE conversations.tenant_id = sqlc.arg(tenant_id)
      AND conversations.id = sqlc.arg(session_id)
    RETURNING *, next_message_sequence - 1 AS allocated_sequence
), inserted AS (
    INSERT INTO conversation_messages (
        id, tenant_id, conversation_id, segment_id, sequence_no, role,
        content, content_json, execution_id, execution_step_id, created_at
    )
    SELECT
        sqlc.arg(id), allocated.tenant_id, allocated.id, allocated.current_segment_id,
        allocated.allocated_sequence, sqlc.arg(role), sqlc.arg(content),
        sqlc.arg(metadata)::jsonb,
        conversation_executions.id, NULL, sqlc.arg(created_at)
    FROM allocated
    LEFT JOIN conversation_executions
      ON conversation_executions.tenant_id = allocated.tenant_id
     AND conversation_executions.conversation_id = allocated.id
     AND conversation_executions.segment_id = allocated.current_segment_id
     AND conversation_executions.id = NULLIF(sqlc.arg(run_id)::text, '')
    RETURNING *
)
SELECT
    inserted.id,
    inserted.tenant_id,
    inserted.conversation_id AS session_id,
    inserted.segment_id,
    inserted.sequence_no,
    inserted.role,
    inserted.content,
    inserted.execution_id AS run_id,
    segments.ordinal::bigint AS context_version,
    COALESCE(inserted.content_json, '{}'::jsonb) AS metadata,
    inserted.created_at
FROM inserted
JOIN conversation_segments segments
  ON segments.tenant_id = inserted.tenant_id
 AND segments.conversation_id = inserted.conversation_id
 AND segments.id = inserted.segment_id;

-- name: ListAgentSessionMessages :many
SELECT
    conversation_messages.id,
    conversation_messages.tenant_id,
    conversation_messages.conversation_id AS session_id,
    conversation_messages.segment_id,
    conversation_messages.sequence_no,
    conversation_messages.role,
    conversation_messages.content,
    conversation_messages.execution_id AS run_id,
    segments.ordinal::bigint AS context_version,
    COALESCE(conversation_messages.content_json, '{}'::jsonb) AS metadata,
    conversation_messages.created_at
FROM conversation_messages
JOIN conversations
  ON conversations.tenant_id = conversation_messages.tenant_id
 AND conversations.id = conversation_messages.conversation_id
JOIN conversation_segments segments
  ON segments.tenant_id = conversation_messages.tenant_id
 AND segments.conversation_id = conversation_messages.conversation_id
 AND segments.id = conversation_messages.segment_id
WHERE conversation_messages.tenant_id = sqlc.arg(tenant_id)
  AND conversation_messages.conversation_id = sqlc.arg(session_id)
  AND conversation_messages.segment_id = conversations.current_segment_id
  AND (
    NOT sqlc.arg(has_cursor)::boolean
    OR conversation_messages.created_at > sqlc.arg(cursor_created_at)::timestamptz
    OR (conversation_messages.created_at = sqlc.arg(cursor_created_at)::timestamptz AND conversation_messages.id > sqlc.arg(cursor_id))
  )
ORDER BY conversation_messages.created_at ASC, conversation_messages.id ASC
LIMIT sqlc.arg(limit_count)::int;

-- name: ListRecentAgentSessionMessages :many
SELECT * FROM (
    SELECT
        conversation_messages.id,
        conversation_messages.tenant_id,
        conversation_messages.conversation_id AS session_id,
        conversation_messages.segment_id,
        conversation_messages.sequence_no,
        conversation_messages.role,
        conversation_messages.content,
        conversation_messages.execution_id AS run_id,
        segments.ordinal::bigint AS context_version,
        COALESCE(conversation_messages.content_json, '{}'::jsonb) AS metadata,
        conversation_messages.created_at
    FROM conversation_messages
    JOIN conversations
      ON conversations.tenant_id = conversation_messages.tenant_id
     AND conversations.id = conversation_messages.conversation_id
    JOIN conversation_segments segments
      ON segments.tenant_id = conversation_messages.tenant_id
     AND segments.conversation_id = conversation_messages.conversation_id
     AND segments.id = conversation_messages.segment_id
    WHERE conversation_messages.tenant_id = sqlc.arg(tenant_id)
      AND conversation_messages.conversation_id = sqlc.arg(session_id)
      AND conversation_messages.segment_id = conversations.current_segment_id
    ORDER BY conversation_messages.created_at DESC, conversation_messages.id DESC
    LIMIT sqlc.arg(limit_count)::int
) recent
ORDER BY created_at ASC, id ASC;

-- name: CountActiveAgentRunsBySession :one
SELECT count(*)
FROM conversation_executions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND conversation_id = sqlc.arg(session_id)
  AND status IN ('queued', 'running');

-- name: UpsertAgentMemory :one
WITH scope_values AS (
    SELECT
        CASE
            WHEN sqlc.arg(session_id)::text <> '' THEN 'conversation'
            WHEN sqlc.arg(agent_id)::text <> '' THEN 'agent'
            ELSE 'global'
        END::text AS scope_type,
        CASE WHEN sqlc.arg(session_id)::text = '' THEN NULLIF(sqlc.arg(agent_id)::text, '') ELSE NULL END AS agent_id,
        CASE WHEN sqlc.arg(session_id)::text <> '' THEN conversations.id ELSE NULL END AS conversation_id,
        CASE WHEN sqlc.arg(session_id)::text <> '' THEN conversations.current_segment_id ELSE NULL END AS segment_id
    FROM (SELECT 1) seed
    LEFT JOIN conversations
      ON conversations.tenant_id = sqlc.arg(tenant_id)
     AND conversations.id = NULLIF(sqlc.arg(session_id)::text, '')
    WHERE sqlc.arg(session_id)::text = '' OR conversations.id IS NOT NULL
), updated_by_id AS (
    UPDATE agent_memories
    SET account_id = sqlc.arg(account_id),
        scope_type = scope_values.scope_type,
        agent_id = scope_values.agent_id,
        conversation_id = scope_values.conversation_id,
        segment_id = scope_values.segment_id,
        key = sqlc.arg(key),
        content = sqlc.arg(content),
        source_type = CASE WHEN sqlc.arg(source)::text = 'manual' THEN 'manual' ELSE 'extracted' END,
        source_message_id = NULLIF(sqlc.arg(source_message_id)::text, ''),
        confidence = sqlc.arg(confidence),
        importance = LEAST(GREATEST(sqlc.arg(importance)::int, 1), 5),
        status = COALESCE(NULLIF(sqlc.arg(status)::text, ''), 'active'),
        expires_at = sqlc.arg(expires_at),
        updated_at = sqlc.arg(updated_at)
    FROM scope_values
    WHERE agent_memories.tenant_id = sqlc.arg(tenant_id)
      AND agent_memories.id = sqlc.arg(id)
    RETURNING agent_memories.*
), inserted_or_merged AS (
    INSERT INTO agent_memories (
        id, tenant_id, account_id, scope_type, agent_id, conversation_id, segment_id,
        key, content, source_type, source_message_id, confidence, importance,
        status, expires_at, created_at, updated_at
    )
    SELECT
        sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(account_id), scope_values.scope_type,
        scope_values.agent_id, scope_values.conversation_id, scope_values.segment_id,
        sqlc.arg(key), sqlc.arg(content),
        CASE WHEN sqlc.arg(source)::text = 'manual' THEN 'manual' ELSE 'extracted' END,
        NULLIF(sqlc.arg(source_message_id)::text, ''), sqlc.arg(confidence),
        LEAST(GREATEST(sqlc.arg(importance)::int, 1), 5),
        COALESCE(NULLIF(sqlc.arg(status)::text, ''), 'active'),
        sqlc.arg(expires_at), sqlc.arg(created_at), sqlc.arg(updated_at)
    FROM scope_values
    WHERE NOT EXISTS (SELECT 1 FROM updated_by_id)
    ON CONFLICT (tenant_id, account_id, scope_type, agent_id, conversation_id, segment_id, key)
        WHERE status = 'active'
    DO UPDATE SET
        content = EXCLUDED.content,
        source_type = EXCLUDED.source_type,
        source_message_id = EXCLUDED.source_message_id,
        confidence = EXCLUDED.confidence,
        importance = EXCLUDED.importance,
        status = EXCLUDED.status,
        expires_at = EXCLUDED.expires_at,
        updated_at = EXCLUDED.updated_at
    RETURNING *
), upserted AS (
    SELECT * FROM updated_by_id
    UNION ALL
    SELECT * FROM inserted_or_merged
)
SELECT
    id, tenant_id, account_id, agent_id, conversation_id AS session_id, segment_id,
    scope_type AS scope, source_message_id, confidence, status,
    key, content,
    CASE WHEN source_type = 'manual' THEN 'manual' ELSE 'auto' END::text AS source,
    importance, expires_at, created_at, updated_at
FROM upserted;

-- name: GetAgentMemory :one
SELECT
    id, tenant_id, account_id, agent_id, conversation_id AS session_id, segment_id,
    scope_type AS scope, source_message_id, confidence, status,
    key, content,
    CASE WHEN source_type = 'manual' THEN 'manual' ELSE 'auto' END::text AS source,
    importance, expires_at, created_at, updated_at
FROM agent_memories
WHERE agent_memories.tenant_id = sqlc.arg(tenant_id)
  AND agent_memories.id = sqlc.arg(id);

-- name: ListAgentMemoriesByAccount :many
SELECT
    id, tenant_id, account_id, agent_id, conversation_id AS session_id, segment_id,
    scope_type AS scope, source_message_id, confidence, status,
    key, content,
    CASE WHEN source_type = 'manual' THEN 'manual' ELSE 'auto' END::text AS source,
    importance, expires_at, created_at, updated_at
FROM agent_memories
WHERE agent_memories.tenant_id = sqlc.arg(tenant_id)
  AND agent_memories.account_id = sqlc.arg(account_id)
  AND agent_memories.status = 'active'
  AND (
      agent_memories.scope_type = 'global'
      OR (
          agent_memories.scope_type = 'agent'
          AND sqlc.arg(agent_id)::text <> ''
          AND agent_memories.agent_id = sqlc.arg(agent_id)
      )
      OR (
          agent_memories.scope_type = 'conversation'
          AND sqlc.arg(session_id)::text <> ''
          AND agent_memories.conversation_id = sqlc.arg(session_id)
          AND agent_memories.segment_id = (
              SELECT conversations.current_segment_id
              FROM conversations
              WHERE conversations.tenant_id = sqlc.arg(tenant_id)
                AND conversations.id = sqlc.arg(session_id)
          )
      )
  )
  AND (agent_memories.expires_at IS NULL OR agent_memories.expires_at > now())
ORDER BY agent_memories.importance DESC, agent_memories.updated_at DESC, agent_memories.id DESC
LIMIT sqlc.arg(limit_count)::int;

-- name: DeleteAgentMemory :one
WITH superseded AS (
    UPDATE agent_memories
    SET status = 'superseded', updated_at = GREATEST(updated_at, now())
    WHERE tenant_id = sqlc.arg(tenant_id)
      AND id = sqlc.arg(id)
    RETURNING *
)
SELECT
    id, tenant_id, account_id, agent_id, conversation_id AS session_id, segment_id,
    scope_type AS scope, source_message_id, confidence, status,
    key, content,
    CASE WHEN source_type = 'manual' THEN 'manual' ELSE 'auto' END::text AS source,
    importance, expires_at, created_at, updated_at
FROM superseded;

-- name: UpsertExecutionStepV2 :one
INSERT INTO conversation_execution_steps (
    id, tenant_id, execution_id, parent_step_id, sequence_no, step_type,
    name, model_connection_id, external_tool_id, status,
    input_summary, output_summary, input_tokens, cached_tokens, output_tokens,
    started_at, completed_at, error_code, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(execution_id), NULLIF(sqlc.arg(parent_step_id)::text, ''),
    sqlc.arg(sequence_no), sqlc.arg(step_type), sqlc.arg(name),
    NULLIF(sqlc.arg(model_connection_id)::text, ''), NULLIF(sqlc.arg(external_tool_id)::text, ''),
    sqlc.arg(status), sqlc.arg(input_summary)::jsonb, sqlc.arg(output_summary)::jsonb,
    sqlc.arg(input_tokens), sqlc.arg(cached_tokens), sqlc.arg(output_tokens),
    sqlc.arg(started_at), sqlc.arg(completed_at), sqlc.arg(error_code), sqlc.arg(created_at)
)
ON CONFLICT (id) DO UPDATE SET
    status = EXCLUDED.status,
    input_summary = EXCLUDED.input_summary,
    output_summary = EXCLUDED.output_summary,
    input_tokens = EXCLUDED.input_tokens,
    cached_tokens = EXCLUDED.cached_tokens,
    output_tokens = EXCLUDED.output_tokens,
    started_at = EXCLUDED.started_at,
    completed_at = EXCLUDED.completed_at,
    error_code = EXCLUDED.error_code
WHERE conversation_execution_steps.tenant_id = EXCLUDED.tenant_id
  AND conversation_execution_steps.execution_id = EXCLUDED.execution_id
RETURNING *;

-- name: AppendExecutionStepV2 :one
WITH locked_execution AS (
    SELECT id, tenant_id
    FROM conversation_executions
    WHERE conversation_executions.tenant_id = sqlc.arg(tenant_id)
      AND conversation_executions.id = sqlc.arg(execution_id)
    FOR UPDATE
), next_sequence AS (
    SELECT locked_execution.id AS execution_id,
           locked_execution.tenant_id,
           COALESCE(max(steps.sequence_no), 0)::integer + 1 AS sequence_no
    FROM locked_execution
    LEFT JOIN conversation_execution_steps steps
      ON steps.tenant_id = locked_execution.tenant_id
     AND steps.execution_id = locked_execution.id
    GROUP BY locked_execution.id, locked_execution.tenant_id
)
INSERT INTO conversation_execution_steps (
    id, tenant_id, execution_id, parent_step_id, sequence_no, step_type,
    name, model_connection_id, external_tool_id, status,
    input_summary, output_summary, input_tokens, cached_tokens, output_tokens,
    started_at, completed_at, error_code, created_at
)
SELECT
    sqlc.arg(id), next_sequence.tenant_id, next_sequence.execution_id,
    NULLIF(sqlc.arg(parent_step_id)::text, ''), next_sequence.sequence_no,
    sqlc.arg(step_type), sqlc.arg(name),
    NULLIF(sqlc.arg(model_connection_id)::text, ''),
    NULLIF(sqlc.arg(external_tool_id)::text, ''),
    'running', sqlc.arg(input_summary)::jsonb, '{}'::jsonb,
    0, 0, 0, sqlc.arg(started_at), NULL, '', sqlc.arg(created_at)
FROM next_sequence
RETURNING *;

-- name: GetExecutionStepV2 :one
SELECT *
FROM conversation_execution_steps
WHERE tenant_id = sqlc.arg(tenant_id) AND id = sqlc.arg(id);

-- name: ListExecutionStepsV2 :many
SELECT *
FROM conversation_execution_steps
WHERE tenant_id = sqlc.arg(tenant_id) AND execution_id = sqlc.arg(execution_id)
ORDER BY sequence_no, id;

-- name: UpsertAgentConfirmationV2 :one
WITH inserted AS (
    INSERT INTO agent_confirmations (
        id, tenant_id, account_id, conversation_id, segment_id,
        execution_id, source_message_id, kind, title, action,
        public_payload, action_payload, result_payload, status,
        last_error, expires_at, consumed_at, created_at, updated_at
    ) VALUES (
        sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(account_id),
        sqlc.arg(conversation_id), sqlc.arg(segment_id),
        NULLIF(sqlc.arg(execution_id)::text, ''), NULLIF(sqlc.arg(source_message_id)::text, ''),
        sqlc.arg(kind), sqlc.arg(title), sqlc.arg(action),
        sqlc.arg(public_payload)::jsonb, sqlc.arg(action_payload)::jsonb,
        sqlc.arg(result_payload)::jsonb, sqlc.arg(status), sqlc.arg(last_error),
        sqlc.arg(expires_at), sqlc.arg(consumed_at), sqlc.arg(created_at), sqlc.arg(updated_at)
    )
    ON CONFLICT (id) DO NOTHING
    RETURNING *
)
SELECT * FROM inserted
UNION ALL
SELECT confirmations.*
FROM agent_confirmations confirmations
WHERE confirmations.tenant_id = sqlc.arg(tenant_id)
  AND confirmations.id = sqlc.arg(id)
  AND NOT EXISTS (SELECT 1 FROM inserted)
LIMIT 1;

-- name: ListPendingAgentConfirmationsV2 :many
SELECT confirmations.*
FROM agent_confirmations confirmations
JOIN conversations
  ON conversations.tenant_id = confirmations.tenant_id
 AND conversations.id = confirmations.conversation_id
 AND conversations.current_segment_id = confirmations.segment_id
WHERE confirmations.tenant_id = sqlc.arg(tenant_id)
  AND confirmations.account_id = sqlc.arg(account_id)
  AND confirmations.conversation_id = sqlc.arg(conversation_id)
  AND confirmations.segment_id = sqlc.arg(segment_id)
  AND confirmations.status = 'pending'
  AND confirmations.expires_at > sqlc.arg(now_at)
ORDER BY confirmations.created_at, confirmations.id;

-- name: ClaimAgentConfirmationV2 :one
WITH expired AS (
    UPDATE agent_confirmations confirmations
    SET status = 'expired', consumed_at = sqlc.arg(now_at), updated_at = sqlc.arg(now_at)
    FROM conversations
    WHERE confirmations.tenant_id = sqlc.arg(tenant_id)
      AND confirmations.account_id = sqlc.arg(account_id)
      AND confirmations.id = sqlc.arg(id)
      AND confirmations.status = 'pending'
      AND confirmations.expires_at <= sqlc.arg(now_at)
      AND conversations.tenant_id = confirmations.tenant_id
      AND conversations.id = confirmations.conversation_id
      AND conversations.current_segment_id = confirmations.segment_id
    RETURNING confirmations.*
), claimed AS (
    UPDATE agent_confirmations confirmations
    SET status = 'executing', updated_at = sqlc.arg(now_at)
    FROM conversations
    WHERE confirmations.tenant_id = sqlc.arg(tenant_id)
      AND confirmations.account_id = sqlc.arg(account_id)
      AND confirmations.id = sqlc.arg(id)
      AND confirmations.status = 'pending'
      AND confirmations.expires_at > sqlc.arg(now_at)
      AND conversations.tenant_id = confirmations.tenant_id
      AND conversations.id = confirmations.conversation_id
      AND conversations.current_segment_id = confirmations.segment_id
    RETURNING confirmations.*
)
SELECT * FROM expired
UNION ALL
SELECT * FROM claimed
LIMIT 1;

-- name: UpdateAgentConfirmationV2 :one
UPDATE agent_confirmations
SET result_payload = sqlc.arg(result_payload)::jsonb,
    status = sqlc.arg(status),
    last_error = sqlc.arg(last_error),
    consumed_at = sqlc.arg(consumed_at),
    updated_at = sqlc.arg(updated_at)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND account_id = sqlc.arg(account_id)
  AND id = sqlc.arg(id)
  AND (
    (agent_confirmations.status = 'executing' AND sqlc.arg(status)::text IN ('pending', 'completed', 'failed', 'cancelled', 'expired'))
    OR (agent_confirmations.status = 'pending' AND sqlc.arg(status)::text IN ('cancelled', 'expired'))
    OR agent_confirmations.status = sqlc.arg(status)
  )
RETURNING *;
