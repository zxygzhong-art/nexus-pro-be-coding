-- Agent v2 administration persistence. Legacy query names are retained as
-- compatibility projections while all writes target the v2 aggregates.

-- name: UpsertAgentModel :one
WITH upserted_connection AS (
    INSERT INTO model_connections (
        id, tenant_id, name, provider, upstream_model, api_base_url,
        api_key_ciphertext, api_key_preview, rate_limit_rpm, timeout_ms, status,
        created_by_account_id, updated_by_account_id,
        created_at, updated_at, archived_at
    ) VALUES (
        sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(name), sqlc.arg(provider),
        sqlc.arg(model_name),
        sqlc.arg(api_base_url),
        sqlc.arg(api_key_ciphertext), sqlc.arg(api_key_preview),
        sqlc.arg(rate_limit_rpm), GREATEST(sqlc.arg(timeout_seconds)::int, 1) * 1000,
        sqlc.arg(status), NULL, NULL,
        sqlc.arg(created_at), sqlc.arg(updated_at), NULL
    )
    ON CONFLICT (id) DO UPDATE SET
        name = EXCLUDED.name,
        provider = EXCLUDED.provider,
        upstream_model = EXCLUDED.upstream_model,
        api_base_url = EXCLUDED.api_base_url,
        api_key_ciphertext = CASE
            WHEN EXCLUDED.api_key_ciphertext = '' THEN model_connections.api_key_ciphertext
            ELSE EXCLUDED.api_key_ciphertext
        END,
        api_key_preview = CASE
            WHEN EXCLUDED.api_key_ciphertext = '' THEN model_connections.api_key_preview
            ELSE EXCLUDED.api_key_preview
        END,
        rate_limit_rpm = EXCLUDED.rate_limit_rpm,
        timeout_ms = EXCLUDED.timeout_ms,
        status = EXCLUDED.status,
        updated_at = EXCLUDED.updated_at,
        archived_at = NULL
    WHERE model_connections.tenant_id = EXCLUDED.tenant_id
    RETURNING *
), upserted_state AS (
    INSERT INTO model_connection_state (
        tenant_id, model_connection_id, sync_status, synced_config_checksum,
        last_synced_at, last_sync_error, last_tested_at, last_test_status,
        last_test_message, updated_at
    )
    SELECT
        tenant_id, id, sqlc.arg(sync_status), sqlc.arg(synced_config_hash),
        sqlc.arg(last_synced_at), sqlc.arg(last_sync_error),
        sqlc.arg(last_tested_at), sqlc.arg(last_test_status),
        sqlc.arg(last_test_message), sqlc.arg(updated_at)
    FROM upserted_connection
    ON CONFLICT (tenant_id, model_connection_id) DO UPDATE SET
        sync_status = EXCLUDED.sync_status,
        synced_config_checksum = EXCLUDED.synced_config_checksum,
        last_synced_at = EXCLUDED.last_synced_at,
        last_sync_error = EXCLUDED.last_sync_error,
        last_tested_at = EXCLUDED.last_tested_at,
        last_test_status = EXCLUDED.last_test_status,
        last_test_message = EXCLUDED.last_test_message,
        updated_at = EXCLUDED.updated_at
    RETURNING *
)
SELECT
    connections.id, connections.tenant_id, connections.name, connections.provider,
    connections.upstream_model AS model_name,
    ('nexus-agent-model-' || connections.id)::text AS litellm_model,
    connections.api_base_url,
    connections.api_key_ciphertext,
    connections.api_key_preview,
    connections.rate_limit_rpm,
    CASE WHEN connections.status = 'archived' THEN 'disabled' ELSE connections.status END::text AS status,
    GREATEST(connections.timeout_ms / 1000, 1)::integer AS timeout_seconds,
    0::bigint AS monthly_quota,
    0::bigint AS used_quota,
    state.last_tested_at, state.last_test_status, state.last_test_message,
    state.sync_status, state.last_synced_at, state.last_sync_error,
    state.synced_config_checksum AS synced_config_hash,
    connections.created_at, connections.updated_at
FROM upserted_connection connections
JOIN upserted_state state
  ON state.tenant_id = connections.tenant_id
 AND state.model_connection_id = connections.id;

-- name: GetAgentModel :one
SELECT
    connections.id, connections.tenant_id, connections.name, connections.provider,
    connections.upstream_model AS model_name, ('nexus-agent-model-' || connections.id)::text AS litellm_model,
    connections.api_base_url,
    connections.api_key_ciphertext,
    connections.api_key_preview,
    connections.rate_limit_rpm,
    CASE WHEN connections.status = 'archived' THEN 'disabled' ELSE connections.status END::text AS status,
    GREATEST(connections.timeout_ms / 1000, 1)::integer AS timeout_seconds,
    0::bigint AS monthly_quota, 0::bigint AS used_quota,
    state.last_tested_at, state.last_test_status, state.last_test_message,
    state.sync_status, state.last_synced_at, state.last_sync_error,
    state.synced_config_checksum AS synced_config_hash,
    connections.created_at, connections.updated_at
FROM model_connections connections
JOIN model_connection_state state
  ON state.tenant_id = connections.tenant_id AND state.model_connection_id = connections.id
WHERE connections.tenant_id = sqlc.arg(tenant_id)
  AND connections.id = sqlc.arg(id);

-- name: ListAgentModels :many
SELECT
    connections.id, connections.tenant_id, connections.name, connections.provider,
    connections.upstream_model AS model_name, ('nexus-agent-model-' || connections.id)::text AS litellm_model,
    connections.api_base_url,
    connections.api_key_ciphertext,
    connections.api_key_preview,
    connections.rate_limit_rpm,
    CASE WHEN connections.status = 'archived' THEN 'disabled' ELSE connections.status END::text AS status,
    GREATEST(connections.timeout_ms / 1000, 1)::integer AS timeout_seconds,
    0::bigint AS monthly_quota, 0::bigint AS used_quota,
    state.last_tested_at, state.last_test_status, state.last_test_message,
    state.sync_status, state.last_synced_at, state.last_sync_error,
    state.synced_config_checksum AS synced_config_hash,
    connections.created_at, connections.updated_at
FROM model_connections connections
JOIN model_connection_state state
  ON state.tenant_id = connections.tenant_id AND state.model_connection_id = connections.id
WHERE connections.tenant_id = sqlc.arg(tenant_id)
  AND connections.status <> 'archived'
ORDER BY connections.updated_at DESC, connections.id ASC;

-- name: DeleteAgentModel :one
WITH archived AS (
    UPDATE model_connections
    SET status = 'archived', archived_at = COALESCE(archived_at, now()), updated_at = GREATEST(updated_at, now())
    WHERE model_connections.tenant_id = sqlc.arg(tenant_id) AND model_connections.id = sqlc.arg(id)
    RETURNING *
)
SELECT
    archived.id, archived.tenant_id, archived.name, archived.provider,
    archived.upstream_model AS model_name, ('nexus-agent-model-' || archived.id)::text AS litellm_model,
    archived.api_base_url,
    archived.api_key_ciphertext,
    archived.api_key_preview,
    archived.rate_limit_rpm, 'disabled'::text AS status,
    GREATEST(archived.timeout_ms / 1000, 1)::integer AS timeout_seconds,
    0::bigint AS monthly_quota, 0::bigint AS used_quota,
    state.last_tested_at, state.last_test_status, state.last_test_message,
    state.sync_status, state.last_synced_at, state.last_sync_error,
    state.synced_config_checksum AS synced_config_hash,
    archived.created_at, archived.updated_at
FROM archived
JOIN model_connection_state state
  ON state.tenant_id = archived.tenant_id AND state.model_connection_id = archived.id;

-- name: UpdateAgentModelTestResult :one
WITH updated_state AS (
    UPDATE model_connection_state
    SET last_tested_at = sqlc.arg(last_tested_at),
        last_test_status = sqlc.arg(last_test_status),
        last_test_message = sqlc.arg(last_test_message),
        updated_at = sqlc.arg(updated_at)
    WHERE model_connection_state.tenant_id = sqlc.arg(tenant_id) AND model_connection_state.model_connection_id = sqlc.arg(id)
    RETURNING *
), touched AS (
    UPDATE model_connections
    SET updated_at = sqlc.arg(updated_at)
    FROM updated_state
    WHERE model_connections.tenant_id = updated_state.tenant_id
      AND model_connections.id = updated_state.model_connection_id
    RETURNING model_connections.*
)
SELECT
    touched.id, touched.tenant_id, touched.name, touched.provider,
    touched.upstream_model AS model_name, ('nexus-agent-model-' || touched.id)::text AS litellm_model,
    touched.api_base_url,
    touched.api_key_ciphertext,
    touched.api_key_preview,
    touched.rate_limit_rpm,
    CASE WHEN touched.status = 'archived' THEN 'disabled' ELSE touched.status END::text AS status,
    GREATEST(touched.timeout_ms / 1000, 1)::integer AS timeout_seconds,
    0::bigint AS monthly_quota, 0::bigint AS used_quota,
    updated_state.last_tested_at, updated_state.last_test_status, updated_state.last_test_message,
    updated_state.sync_status, updated_state.last_synced_at, updated_state.last_sync_error,
    updated_state.synced_config_checksum AS synced_config_hash,
    touched.created_at, touched.updated_at
FROM touched
JOIN updated_state ON updated_state.tenant_id = touched.tenant_id AND updated_state.model_connection_id = touched.id;

-- name: UpdateAgentModelSyncResult :one
WITH updated_state AS (
    UPDATE model_connection_state
    SET sync_status = sqlc.arg(sync_status),
        last_synced_at = sqlc.arg(last_synced_at),
        last_sync_error = sqlc.arg(last_sync_error),
        synced_config_checksum = sqlc.arg(synced_config_hash),
        updated_at = sqlc.arg(updated_at)
    WHERE model_connection_state.tenant_id = sqlc.arg(tenant_id) AND model_connection_state.model_connection_id = sqlc.arg(id)
    RETURNING *
), touched AS (
    UPDATE model_connections
    SET updated_at = sqlc.arg(updated_at)
    FROM updated_state
    WHERE model_connections.tenant_id = updated_state.tenant_id
      AND model_connections.id = updated_state.model_connection_id
    RETURNING model_connections.*
)
SELECT
    touched.id, touched.tenant_id, touched.name, touched.provider,
    touched.upstream_model AS model_name, ('nexus-agent-model-' || touched.id)::text AS litellm_model,
    touched.api_base_url,
    touched.api_key_ciphertext,
    touched.api_key_preview,
    touched.rate_limit_rpm,
    CASE WHEN touched.status = 'archived' THEN 'disabled' ELSE touched.status END::text AS status,
    GREATEST(touched.timeout_ms / 1000, 1)::integer AS timeout_seconds,
    0::bigint AS monthly_quota, 0::bigint AS used_quota,
    updated_state.last_tested_at, updated_state.last_test_status, updated_state.last_test_message,
    updated_state.sync_status, updated_state.last_synced_at, updated_state.last_sync_error,
    updated_state.synced_config_checksum AS synced_config_hash,
    touched.created_at, touched.updated_at
FROM touched
JOIN updated_state ON updated_state.tenant_id = touched.tenant_id AND updated_state.model_connection_id = touched.id;

-- name: ListAgentDefinitionRefsByModel :many
WITH selected_revisions AS (
    SELECT tenant_id, id AS agent_id, draft_revision_id AS revision_id
    FROM agents
    UNION
    SELECT tenant_id, id AS agent_id, published_revision_id AS revision_id
    FROM agents
)
SELECT DISTINCT agents.id, revisions.name
FROM agents
JOIN selected_revisions
  ON selected_revisions.tenant_id = agents.tenant_id
 AND selected_revisions.agent_id = agents.id
JOIN agent_revisions revisions
  ON revisions.tenant_id = agents.tenant_id
 AND revisions.agent_id = agents.id
 AND revisions.id = selected_revisions.revision_id
LEFT JOIN agents children
  ON children.tenant_id = agents.tenant_id
 AND children.parent_agent_id = agents.id
LEFT JOIN agent_revisions child_revisions
  ON child_revisions.tenant_id = children.tenant_id
 AND child_revisions.agent_id = children.id
 AND child_revisions.revision_no = revisions.revision_no
WHERE agents.tenant_id = sqlc.arg(tenant_id)
  AND agents.parent_agent_id IS NULL
  AND agents.lifecycle_status = 'active'
  AND (revisions.model_connection_id = sqlc.arg(model_id) OR child_revisions.model_connection_id = sqlc.arg(model_id))
ORDER BY revisions.name;

-- name: InsertAgentExternalTool :one
INSERT INTO external_tool_connections (
    id, tenant_id, name, description, kind, transport, endpoint_url,
    auth_type, auth_header_name, auth_username, auth_secret_ciphertext,
    timeout_ms, status, created_by_account_id, updated_by_account_id,
    created_at, updated_at, archived_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(name), sqlc.arg(description),
    sqlc.arg(kind), sqlc.arg(transport), sqlc.arg(endpoint_url), sqlc.arg(auth_type),
    sqlc.arg(auth_header_name), sqlc.arg(auth_username),
    sqlc.arg(auth_secret_ciphertext),
    GREATEST(sqlc.arg(timeout_seconds)::int, 1) * 1000,
    'active', sqlc.arg(created_by_account_id), sqlc.arg(created_by_account_id),
    sqlc.arg(created_at), sqlc.arg(created_at), NULL
)
RETURNING
    id, tenant_id, name, description, kind, transport, endpoint_url,
    auth_type, auth_header_name, auth_username,
    auth_secret_ciphertext,
    GREATEST(timeout_ms / 1000, 1)::integer AS timeout_seconds,
    status, last_tested_at, last_test_status, last_test_message,
    created_by_account_id, created_at, updated_at, archived_at;

-- name: ListAgentExternalTools :many
SELECT
    connections.id, connections.tenant_id, connections.name, connections.description,
    connections.kind, connections.transport, connections.endpoint_url,
    connections.auth_type, connections.auth_header_name, connections.auth_username,
    connections.auth_secret_ciphertext,
    GREATEST(connections.timeout_ms / 1000, 1)::integer AS timeout_seconds,
    connections.status, connections.last_tested_at, connections.last_test_status, connections.last_test_message,
    connections.created_by_account_id, connections.created_at, connections.updated_at, connections.archived_at
FROM external_tool_connections connections
WHERE connections.tenant_id = sqlc.arg(tenant_id)
ORDER BY CASE connections.status WHEN 'active' THEN 0 WHEN 'disabled' THEN 1 ELSE 2 END,
         connections.updated_at DESC, connections.id ASC;

-- name: DeleteAgentExternalTool :one
WITH archived AS (
    UPDATE external_tool_connections
    SET status = 'archived', archived_at = COALESCE(archived_at, now()), updated_at = GREATEST(updated_at, now())
    WHERE external_tool_connections.tenant_id = sqlc.arg(tenant_id) AND external_tool_connections.id = sqlc.arg(id)
    RETURNING *
), archived_tools AS (
    UPDATE external_tools
    SET enabled = false,
        archived_at = COALESCE(external_tools.archived_at, archived.archived_at),
        updated_at = archived.updated_at
    FROM archived
    WHERE external_tools.tenant_id = archived.tenant_id
      AND external_tools.connection_id = archived.id
    RETURNING external_tools.id
)
SELECT
    archived.id, archived.tenant_id, archived.name, archived.description,
    archived.kind, archived.transport, archived.endpoint_url,
    archived.auth_type, archived.auth_header_name, archived.auth_username,
    archived.auth_secret_ciphertext,
    GREATEST(archived.timeout_ms / 1000, 1)::integer AS timeout_seconds,
    archived.status, archived.last_tested_at, archived.last_test_status, archived.last_test_message,
    archived.created_by_account_id, archived.created_at, archived.updated_at, archived.archived_at
FROM archived;

-- name: CountAgentDefinitionsByKnowledgeBase :one
SELECT count(DISTINCT COALESCE(agent.parent_agent_id, agent.id))::bigint
FROM agent_revision_knowledge_bases binding
JOIN agent_revisions revision
  ON revision.tenant_id = binding.tenant_id
 AND revision.id = binding.revision_id
JOIN agents agent
  ON agent.tenant_id = revision.tenant_id
 AND agent.id = revision.agent_id
WHERE binding.tenant_id = sqlc.arg(tenant_id)
  AND binding.knowledge_base_id = sqlc.arg(knowledge_base_id);

-- name: UpsertAgentDefinition :one
WITH upserted_agent AS (
    INSERT INTO agents (
        id, tenant_id, parent_agent_id, lifecycle_status,
        draft_revision_id, published_revision_id,
        next_revision_no, created_by_account_id, created_at, updated_at, archived_at
    ) VALUES (
        sqlc.arg(id), sqlc.arg(tenant_id), NULL, 'active', NULL, NULL,
        GREATEST(sqlc.arg(version)::int + 1, 2), sqlc.arg(created_by_account_id),
        sqlc.arg(created_at), sqlc.arg(updated_at), NULL
    )
    ON CONFLICT (id) DO UPDATE SET
        lifecycle_status = 'active',
        next_revision_no = GREATEST(agents.next_revision_no, EXCLUDED.next_revision_no),
        updated_at = EXCLUDED.updated_at,
        archived_at = NULL
    WHERE agents.tenant_id = EXCLUDED.tenant_id
      AND agents.parent_agent_id IS NULL
    RETURNING *
), inserted_revision AS (
    INSERT INTO agent_revisions (
        id, tenant_id, agent_id, revision_no, ordinal,
        name, description, icon, category,
        visibility, visibility_targets, main_agent_role, system_prompt, welcome_message,
        suggested_questions, suggested_question_translations,
        model_connection_id, model_config_checksum, timeout_ms,
        config_schema_version, checksum, revision_note,
        created_by_account_id, created_at
    )
    SELECT
        COALESCE(NULLIF(sqlc.arg(draft_revision_id)::text, ''), sqlc.arg(id)::text || ':revision:' || GREATEST(sqlc.arg(version)::int, 1)::text),
        sqlc.arg(tenant_id), sqlc.arg(id), GREATEST(sqlc.arg(version)::int, 1), NULL,
        sqlc.arg(name), sqlc.arg(description), sqlc.arg(emoji), sqlc.arg(category),
        sqlc.arg(visibility), sqlc.arg(visibility_targets)::jsonb,
        sqlc.arg(main_agent_role), sqlc.arg(system_prompt), sqlc.arg(welcome_message),
        sqlc.arg(suggested_questions)::jsonb, sqlc.arg(suggested_question_translations)::jsonb,
        sqlc.arg(model_id), COALESCE(model_state.synced_config_checksum, ''),
        GREATEST(sqlc.arg(timeout_seconds)::int, 1) * 1000,
        1,
        md5(concat_ws('|', sqlc.arg(model_id)::text, sqlc.arg(main_agent_role)::text,
            sqlc.arg(system_prompt)::text, sqlc.arg(tools)::text,
            sqlc.arg(external_tool_ids)::text, sqlc.arg(knowledge_base_ids)::text,
            sqlc.arg(sub_agents)::text)),
        '', sqlc.arg(created_by_account_id), sqlc.arg(updated_at)
    FROM upserted_agent
    LEFT JOIN model_connection_state model_state
      ON model_state.tenant_id = sqlc.arg(tenant_id)
     AND model_state.model_connection_id = sqlc.arg(model_id)
    ON CONFLICT (tenant_id, agent_id, revision_no) DO NOTHING
    RETURNING *
), target_revision AS (
    SELECT * FROM inserted_revision
    UNION ALL
    SELECT revisions.*
    FROM agent_revisions revisions
    WHERE revisions.tenant_id = sqlc.arg(tenant_id)
      AND revisions.agent_id = sqlc.arg(id)
      AND revisions.revision_no = GREATEST(sqlc.arg(version)::int, 1)
    LIMIT 1
), requested_members AS (
    SELECT
        target_revision.tenant_id,
        target_revision.agent_id AS parent_agent_id,
        target_revision.id AS parent_revision_id,
        target_revision.revision_no,
        target_revision.agent_id || ':member:' || (members.value->>'id') AS child_agent_id,
        target_revision.id || ':member:' || (members.value->>'id') AS child_revision_id,
        members.value->>'name' AS name,
        members.value->>'role' AS role,
        members.value->>'model_id' AS model_id,
        COALESCE(NULLIF(members.value->>'model_config_checksum', ''), member_model_state.synced_config_checksum, '') AS model_config_checksum,
        COALESCE(members.value->'tools', '[]'::jsonb) AS tools,
        COALESCE(members.value->'external_tool_ids', '[]'::jsonb) AS external_tool_ids,
        COALESCE(members.value->'knowledge_base_ids', '[]'::jsonb) AS knowledge_base_ids,
        members.ordinality::int - 1 AS ordinal
    FROM target_revision
    CROSS JOIN LATERAL jsonb_array_elements(sqlc.arg(sub_agents)::jsonb)
        WITH ORDINALITY AS members(value, ordinality)
    LEFT JOIN model_connection_state member_model_state
      ON member_model_state.tenant_id = target_revision.tenant_id
     AND member_model_state.model_connection_id = members.value->>'model_id'
), upserted_children AS (
    INSERT INTO agents (
        id, tenant_id, parent_agent_id, lifecycle_status,
        draft_revision_id, published_revision_id,
        next_revision_no, created_by_account_id, created_at, updated_at, archived_at
    )
    SELECT
        member.child_agent_id, member.tenant_id, member.parent_agent_id, 'active',
        NULL, NULL, GREATEST(member.revision_no + 1, 2),
        sqlc.arg(created_by_account_id), sqlc.arg(created_at), sqlc.arg(updated_at), NULL
    FROM requested_members member
    ON CONFLICT (id) DO UPDATE SET
        lifecycle_status = 'active',
        next_revision_no = GREATEST(agents.next_revision_no, EXCLUDED.next_revision_no),
        updated_at = EXCLUDED.updated_at,
        archived_at = NULL
    WHERE agents.tenant_id = EXCLUDED.tenant_id
      AND agents.parent_agent_id = EXCLUDED.parent_agent_id
    RETURNING *
), archived_children AS (
    UPDATE agents child
    SET lifecycle_status = 'archived',
        archived_at = COALESCE(child.archived_at, sqlc.arg(updated_at)),
        updated_at = sqlc.arg(updated_at)
    WHERE child.tenant_id = sqlc.arg(tenant_id)
      AND child.parent_agent_id = sqlc.arg(id)
      AND NOT EXISTS (
          SELECT 1 FROM requested_members member
          WHERE member.child_agent_id = child.id
      )
    RETURNING child.id
), inserted_child_revisions AS (
    INSERT INTO agent_revisions (
        id, tenant_id, agent_id, revision_no, ordinal,
        name, description, icon, category,
        visibility, visibility_targets, main_agent_role, system_prompt, welcome_message,
        suggested_questions, suggested_question_translations,
        model_connection_id, model_config_checksum, timeout_ms,
        config_schema_version, checksum, revision_note,
        created_by_account_id, created_at
    )
    SELECT
        member.child_revision_id, member.tenant_id, member.child_agent_id,
        member.revision_no, member.ordinal,
        member.name, '', 'AI', target_revision.category,
        target_revision.visibility, target_revision.visibility_targets,
        member.role, '', '', '[]'::jsonb, '[]'::jsonb,
        member.model_id, member.model_config_checksum, target_revision.timeout_ms,
        target_revision.config_schema_version,
        md5(concat_ws('|', target_revision.checksum, member.child_agent_id, member.role,
            member.model_id, member.model_config_checksum, member.tools::text,
            member.external_tool_ids::text, member.knowledge_base_ids::text)),
        target_revision.revision_note,
        target_revision.created_by_account_id, target_revision.created_at
    FROM requested_members member
    JOIN target_revision ON target_revision.id = member.parent_revision_id
    JOIN upserted_children child ON child.id = member.child_agent_id
    ON CONFLICT (tenant_id, agent_id, revision_no) DO NOTHING
    RETURNING *
), target_child_revisions AS (
    SELECT * FROM inserted_child_revisions
    UNION ALL
    SELECT revision.*
    FROM agent_revisions revision
    JOIN requested_members member
      ON member.tenant_id = revision.tenant_id
     AND member.child_agent_id = revision.agent_id
     AND member.revision_no = revision.revision_no
    WHERE NOT EXISTS (
        SELECT 1 FROM inserted_child_revisions inserted
        WHERE inserted.tenant_id = revision.tenant_id AND inserted.id = revision.id
    )
), inserted_tools AS (
    INSERT INTO agent_revision_builtin_tools (tenant_id, revision_id, tool_key, ordinal, config)
    SELECT target_revision.tenant_id, target_revision.id, tool.value,
           tool.ordinality::int - 1, '{}'::jsonb
    FROM target_revision
    CROSS JOIN LATERAL jsonb_array_elements_text(sqlc.arg(tools)::jsonb) WITH ORDINALITY AS tool(value, ordinality)
    ON CONFLICT DO NOTHING
    RETURNING *
), inserted_knowledge AS (
    INSERT INTO agent_revision_knowledge_bases (tenant_id, revision_id, knowledge_base_id, ordinal)
    SELECT target_revision.tenant_id, target_revision.id, knowledge.value,
           knowledge.ordinality::int - 1
    FROM target_revision
    CROSS JOIN LATERAL jsonb_array_elements_text(sqlc.arg(knowledge_base_ids)::jsonb) WITH ORDINALITY AS knowledge(value, ordinality)
    ON CONFLICT DO NOTHING
    RETURNING *
), inserted_external_tools AS (
    INSERT INTO agent_revision_external_tools (
        tenant_id, revision_id, external_tool_id, tool_schema_checksum, ordinal, config
    )
    SELECT target_revision.tenant_id, target_revision.id, requested.value,
           external_tools.schema_checksum, requested.ordinality::int - 1, '{}'::jsonb
    FROM target_revision
    CROSS JOIN LATERAL jsonb_array_elements_text(sqlc.arg(external_tool_ids)::jsonb)
        WITH ORDINALITY AS requested(value, ordinality)
    JOIN external_tools
      ON external_tools.tenant_id = target_revision.tenant_id
     AND external_tools.id = requested.value
     AND external_tools.enabled
     AND external_tools.archived_at IS NULL
    JOIN external_tool_connections external_connections
      ON external_connections.tenant_id = external_tools.tenant_id
     AND external_connections.id = external_tools.connection_id
     AND external_connections.status = 'active'
    ON CONFLICT DO NOTHING
    RETURNING *
), inserted_child_tools AS (
    INSERT INTO agent_revision_builtin_tools (
        tenant_id, revision_id, tool_key, ordinal, config
    )
    SELECT member.tenant_id, revision.id,
           tool.value, tool.ordinality::int - 1, '{}'::jsonb
    FROM requested_members member
    JOIN target_child_revisions revision
      ON revision.tenant_id = member.tenant_id
     AND revision.agent_id = member.child_agent_id
     AND revision.revision_no = member.revision_no
    CROSS JOIN LATERAL jsonb_array_elements_text(COALESCE(member.tools, '[]'::jsonb)) WITH ORDINALITY AS tool(value, ordinality)
    ON CONFLICT DO NOTHING
    RETURNING *
), inserted_child_external_tools AS (
    INSERT INTO agent_revision_external_tools (
        tenant_id, revision_id, external_tool_id, tool_schema_checksum, ordinal, config
    )
    SELECT member.tenant_id, revision.id,
           requested.value, external_tools.schema_checksum,
           requested.ordinality::int - 1, '{}'::jsonb
    FROM requested_members member
    JOIN target_child_revisions revision
      ON revision.tenant_id = member.tenant_id
     AND revision.agent_id = member.child_agent_id
     AND revision.revision_no = member.revision_no
    CROSS JOIN LATERAL jsonb_array_elements_text(COALESCE(member.external_tool_ids, '[]'::jsonb))
        WITH ORDINALITY AS requested(value, ordinality)
    JOIN external_tools
      ON external_tools.tenant_id = member.tenant_id
     AND external_tools.id = requested.value
     AND external_tools.enabled
     AND external_tools.archived_at IS NULL
    JOIN external_tool_connections external_connections
      ON external_connections.tenant_id = external_tools.tenant_id
     AND external_connections.id = external_tools.connection_id
     AND external_connections.status = 'active'
    ON CONFLICT DO NOTHING
    RETURNING *
), inserted_child_knowledge AS (
    INSERT INTO agent_revision_knowledge_bases (
        tenant_id, revision_id, knowledge_base_id, ordinal
    )
    SELECT member.tenant_id, revision.id,
           knowledge.value, knowledge.ordinality::int - 1
    FROM requested_members member
    JOIN target_child_revisions revision
      ON revision.tenant_id = member.tenant_id
     AND revision.agent_id = member.child_agent_id
     AND revision.revision_no = member.revision_no
    CROSS JOIN LATERAL jsonb_array_elements_text(COALESCE(member.knowledge_base_ids, '[]'::jsonb)) WITH ORDINALITY AS knowledge(value, ordinality)
    ON CONFLICT DO NOTHING
    RETURNING *
), updated_children AS (
    UPDATE agents child
    SET draft_revision_id = child_revision.id,
        published_revision_id = (
            SELECT published_child_revision.id
            FROM agent_revisions published_root_revision
            JOIN agent_revisions published_child_revision
              ON published_child_revision.tenant_id = published_root_revision.tenant_id
             AND published_child_revision.agent_id = child.id
             AND published_child_revision.revision_no = published_root_revision.revision_no
            WHERE published_root_revision.tenant_id = child.tenant_id
              AND published_root_revision.id = NULLIF(sqlc.arg(published_revision_id)::text, '')
        ),
        updated_at = sqlc.arg(updated_at)
    FROM target_child_revisions child_revision
    WHERE child.tenant_id = child_revision.tenant_id
      AND child.id = child_revision.agent_id
    RETURNING child.id
), updated_agent AS (
    UPDATE agents
    SET draft_revision_id = target_revision.id,
        published_revision_id = NULLIF(sqlc.arg(published_revision_id)::text, ''),
        updated_at = sqlc.arg(updated_at)
    FROM target_revision
    WHERE agents.tenant_id = target_revision.tenant_id
      AND agents.id = target_revision.agent_id
    RETURNING agents.*
)
SELECT updated_agent.id, updated_agent.tenant_id, target_revision.id AS revision_id
FROM updated_agent
JOIN target_revision ON target_revision.tenant_id = updated_agent.tenant_id AND target_revision.agent_id = updated_agent.id;

-- name: ClaimAgentDefinitionRevision :one
UPDATE agents
SET next_revision_no = next_revision_no
    + CASE WHEN sqlc.arg(create_revision)::boolean THEN 1 ELSE 0 END
WHERE agents.tenant_id = sqlc.arg(tenant_id)
  AND agents.id = sqlc.arg(id)
  AND agents.parent_agent_id IS NULL
  AND agents.lifecycle_status = 'active'
  AND EXISTS (
      SELECT 1
      FROM agent_revisions current_revision
      WHERE current_revision.tenant_id = agents.tenant_id
        AND current_revision.id = agents.draft_revision_id
        AND current_revision.revision_no = sqlc.arg(expected_version)
  )
RETURNING (
    next_revision_no
    - CASE WHEN sqlc.arg(create_revision)::boolean THEN 1 ELSE 0 END
)::integer AS revision_no;

-- name: GetAgentDefinition :one
WITH selected AS (
    SELECT
        agents.id AS agent_id,
        agents.tenant_id,
        agents.draft_revision_id,
        agents.published_revision_id,
        agents.created_by_account_id,
        agents.created_at,
        agents.updated_at,
        revisions.id,
        revisions.revision_no,
        revisions.name,
        revisions.description,
        revisions.icon,
        revisions.category,
        revisions.visibility,
        revisions.visibility_targets,
        revisions.main_agent_role,
        revisions.system_prompt,
        revisions.welcome_message,
        revisions.suggested_questions,
        revisions.suggested_question_translations,
        revisions.model_connection_id,
        revisions.timeout_ms
    FROM agents
    JOIN agent_revisions revisions
      ON revisions.tenant_id = agents.tenant_id
     AND revisions.agent_id = agents.id
     AND revisions.id = COALESCE(agents.draft_revision_id, agents.published_revision_id)
    WHERE agents.tenant_id = sqlc.arg(tenant_id)
      AND agents.id = sqlc.arg(id)
      AND agents.parent_agent_id IS NULL
)
SELECT
    selected.agent_id AS id, selected.tenant_id, selected.name, selected.description,
    selected.icon AS emoji, selected.category, selected.model_connection_id AS model_id,
    selected.main_agent_role,
    COALESCE((SELECT jsonb_agg(jsonb_build_object(
        'id', substring(child.id FROM char_length(selected.agent_id || ':member:') + 1),
        'name', child_revision.name, 'role', child_revision.main_agent_role,
        'model_id', child_revision.model_connection_id,
        'model_config_checksum', child_revision.model_config_checksum,
        'tools', COALESCE((SELECT jsonb_agg(tools.tool_key ORDER BY tools.ordinal) FROM agent_revision_builtin_tools tools WHERE tools.tenant_id = child_revision.tenant_id AND tools.revision_id = child_revision.id), '[]'::jsonb),
        'external_tool_ids', COALESCE((SELECT jsonb_agg(tools.external_tool_id ORDER BY tools.ordinal) FROM agent_revision_external_tools tools WHERE tools.tenant_id = child_revision.tenant_id AND tools.revision_id = child_revision.id), '[]'::jsonb),
        'knowledge_base_ids', COALESCE((SELECT jsonb_agg(kb.knowledge_base_id ORDER BY kb.ordinal) FROM agent_revision_knowledge_bases kb WHERE kb.tenant_id = child_revision.tenant_id AND kb.revision_id = child_revision.id), '[]'::jsonb)
    ) ORDER BY child_revision.ordinal)
    FROM agents child
    JOIN agent_revisions child_revision
      ON child_revision.tenant_id = child.tenant_id
     AND child_revision.agent_id = child.id
     AND child_revision.revision_no = selected.revision_no
    WHERE child.tenant_id = selected.tenant_id
      AND child.parent_agent_id = selected.agent_id), '[]'::jsonb) AS sub_agents,
    selected.system_prompt, selected.welcome_message, selected.suggested_questions,
    selected.suggested_question_translations,
    COALESCE((SELECT jsonb_agg(tools.tool_key ORDER BY tools.ordinal) FROM agent_revision_builtin_tools tools WHERE tools.tenant_id = selected.tenant_id AND tools.revision_id = selected.id), '[]'::jsonb) AS tools,
    COALESCE((SELECT jsonb_agg(tools.external_tool_id ORDER BY tools.ordinal) FROM agent_revision_external_tools tools WHERE tools.tenant_id = selected.tenant_id AND tools.revision_id = selected.id), '[]'::jsonb) AS external_tool_ids,
    COALESCE((SELECT jsonb_agg(kb.knowledge_base_id ORDER BY kb.ordinal) FROM agent_revision_knowledge_bases kb WHERE kb.tenant_id = selected.tenant_id AND kb.revision_id = selected.id), '[]'::jsonb) AS knowledge_base_ids,
    CASE WHEN selected.published_revision_id IS NOT NULL THEN 'published' ELSE 'draft' END::text AS status,
    selected.visibility, selected.visibility_targets,
    GREATEST(selected.timeout_ms / 1000, 1)::integer AS timeout_seconds,
    selected.revision_no AS version,
    COALESCE((SELECT revisions.revision_no FROM agent_revisions revisions WHERE revisions.tenant_id = selected.tenant_id AND revisions.id = selected.published_revision_id), 0)::integer AS published_version,
    selected.draft_revision_id, selected.published_revision_id,
    COALESCE(usage.total_runs, 0)::bigint AS usage_total_runs,
    COALESCE(usage.success_runs, 0)::bigint AS usage_success_runs,
    COALESCE(usage.failed_runs, 0)::bigint AS usage_failed_runs,
    COALESCE(usage.avg_latency_ms, 0)::integer AS usage_avg_latency_ms,
    usage.last_run_at AS usage_last_run_at,
    '[]'::jsonb AS usage_top_prompts,
    selected.created_by_account_id, selected.created_by_account_id AS updated_by_account_id,
    selected.created_at, selected.updated_at
FROM selected
LEFT JOIN LATERAL (
    SELECT count(*)::bigint AS total_runs,
           count(*) FILTER (WHERE status = 'completed')::bigint AS success_runs,
           count(*) FILTER (WHERE status = 'failed')::bigint AS failed_runs,
           COALESCE(avg(EXTRACT(EPOCH FROM (completed_at - started_at)) * 1000) FILTER (WHERE started_at IS NOT NULL AND completed_at IS NOT NULL), 0)::integer AS avg_latency_ms,
           max(updated_at)::timestamptz AS last_run_at
    FROM conversation_executions
    WHERE conversation_executions.tenant_id = selected.tenant_id AND conversation_executions.agent_id = selected.agent_id
) usage ON true;

-- name: ListAgentDefinitions :many
SELECT agents.id, agents.tenant_id
FROM agents
WHERE agents.tenant_id = sqlc.arg(tenant_id)
  AND agents.parent_agent_id IS NULL
  AND agents.lifecycle_status = 'active'
ORDER BY agents.updated_at DESC, agents.id ASC;

-- name: ListPublishedAgentDefinitions :many
SELECT agents.id, agents.tenant_id
FROM agents
WHERE agents.tenant_id = sqlc.arg(tenant_id)
  AND agents.parent_agent_id IS NULL
  AND agents.lifecycle_status = 'active'
  AND agents.published_revision_id IS NOT NULL
ORDER BY agents.updated_at DESC, agents.id ASC;

-- name: DeleteAgentDefinition :one
WITH archived AS (
    UPDATE agents
    SET lifecycle_status = 'archived', archived_at = COALESCE(archived_at, now()), updated_at = GREATEST(updated_at, now())
    WHERE agents.tenant_id = sqlc.arg(tenant_id)
      AND (agents.id = sqlc.arg(id) OR agents.parent_agent_id = sqlc.arg(id))
    RETURNING id, tenant_id
)
SELECT * FROM archived WHERE id = sqlc.arg(id);

-- name: InsertAgentDefinitionVersion :one
WITH upserted_revision AS (
    INSERT INTO agent_revisions (
        id, tenant_id, agent_id, revision_no, ordinal, name, description, icon, category,
        visibility, visibility_targets, main_agent_role, system_prompt, welcome_message,
        suggested_questions, suggested_question_translations,
        model_connection_id, model_config_checksum, timeout_ms,
        config_schema_version, checksum, revision_note,
        created_by_account_id, created_at
    ) VALUES (
        sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(agent_id), sqlc.arg(version), NULL,
        sqlc.arg(name), sqlc.arg(description), sqlc.arg(emoji), sqlc.arg(category),
        sqlc.arg(visibility), sqlc.arg(visibility_targets)::jsonb,
        sqlc.arg(main_agent_role), sqlc.arg(system_prompt), sqlc.arg(welcome_message),
        sqlc.arg(suggested_questions)::jsonb, sqlc.arg(suggested_question_translations)::jsonb,
        sqlc.arg(model_id), sqlc.arg(model_config_checksum),
        GREATEST(sqlc.arg(timeout_seconds)::int, 1) * 1000,
        GREATEST(sqlc.arg(config_schema_version)::int, 1), sqlc.arg(checksum), sqlc.arg(note),
        sqlc.arg(created_by_account_id), sqlc.arg(created_at)
    )
    ON CONFLICT (id) DO UPDATE SET
        model_config_checksum = EXCLUDED.model_config_checksum,
        config_schema_version = EXCLUDED.config_schema_version,
        checksum = EXCLUDED.checksum,
        revision_note = EXCLUDED.revision_note
    WHERE agent_revisions.tenant_id = EXCLUDED.tenant_id
      AND agent_revisions.agent_id = EXCLUDED.agent_id
      AND agent_revisions.revision_no = EXCLUDED.revision_no
      AND NOT EXISTS (
          SELECT 1
          FROM agents published_agent
          WHERE published_agent.tenant_id = agent_revisions.tenant_id
            AND published_agent.id = agent_revisions.agent_id
            AND published_agent.published_revision_id = agent_revisions.id
      )
    RETURNING *
), requested_members AS (
    SELECT
        revision.tenant_id,
        revision.agent_id AS parent_agent_id,
        revision.id AS parent_revision_id,
        revision.revision_no,
        revision.agent_id || ':member:' || (members.value->>'id') AS child_agent_id,
        revision.id || ':member:' || (members.value->>'id') AS child_revision_id,
        members.value->>'name' AS name,
        members.value->>'role' AS role,
        members.value->>'model_id' AS model_id,
        COALESCE(NULLIF(members.value->>'model_config_checksum', ''), member_model_state.synced_config_checksum, '') AS model_config_checksum,
        COALESCE(members.value->'tools', '[]'::jsonb) AS tools,
        COALESCE(members.value->'external_tool_ids', '[]'::jsonb) AS external_tool_ids,
        COALESCE(members.value->'knowledge_base_ids', '[]'::jsonb) AS knowledge_base_ids,
        members.ordinality::int - 1 AS ordinal
    FROM upserted_revision revision
    CROSS JOIN LATERAL jsonb_array_elements(sqlc.arg(sub_agents)::jsonb)
        WITH ORDINALITY AS members(value, ordinality)
    LEFT JOIN model_connection_state member_model_state
      ON member_model_state.tenant_id = revision.tenant_id
     AND member_model_state.model_connection_id = members.value->>'model_id'
), upserted_children AS (
    INSERT INTO agents (
        id, tenant_id, parent_agent_id, lifecycle_status,
        draft_revision_id, published_revision_id,
        next_revision_no, created_by_account_id, created_at, updated_at, archived_at
    )
    SELECT
        member.child_agent_id, member.tenant_id, member.parent_agent_id, 'active',
        NULL, NULL, GREATEST(member.revision_no + 1, 2),
        sqlc.arg(created_by_account_id), sqlc.arg(created_at), sqlc.arg(created_at), NULL
    FROM requested_members member
    ON CONFLICT (id) DO UPDATE SET
        lifecycle_status = 'active',
        next_revision_no = GREATEST(agents.next_revision_no, EXCLUDED.next_revision_no),
        updated_at = EXCLUDED.updated_at,
        archived_at = NULL
    WHERE agents.tenant_id = EXCLUDED.tenant_id
      AND agents.parent_agent_id = EXCLUDED.parent_agent_id
    RETURNING *
), inserted_child_revisions AS (
    INSERT INTO agent_revisions (
        id, tenant_id, agent_id, revision_no, ordinal,
        name, description, icon, category,
        visibility, visibility_targets, main_agent_role, system_prompt, welcome_message,
        suggested_questions, suggested_question_translations,
        model_connection_id, model_config_checksum, timeout_ms,
        config_schema_version, checksum, revision_note,
        created_by_account_id, created_at
    )
    SELECT
        member.child_revision_id, member.tenant_id, member.child_agent_id,
        member.revision_no, member.ordinal,
        member.name, '', 'AI', root.category,
        root.visibility, root.visibility_targets,
        member.role, '', '', '[]'::jsonb, '[]'::jsonb,
        member.model_id, member.model_config_checksum, root.timeout_ms,
        root.config_schema_version,
        md5(concat_ws('|', root.checksum, member.child_agent_id, member.role,
            member.model_id, member.model_config_checksum, member.tools::text,
            member.external_tool_ids::text, member.knowledge_base_ids::text)),
        root.revision_note, root.created_by_account_id, root.created_at
    FROM requested_members member
    JOIN upserted_revision root ON root.id = member.parent_revision_id
    JOIN upserted_children child ON child.id = member.child_agent_id
    ON CONFLICT (tenant_id, agent_id, revision_no) DO UPDATE SET
        model_config_checksum = EXCLUDED.model_config_checksum,
        config_schema_version = EXCLUDED.config_schema_version,
        checksum = EXCLUDED.checksum,
        revision_note = EXCLUDED.revision_note
    WHERE NOT EXISTS (
        SELECT 1 FROM agents published_child
        WHERE published_child.tenant_id = agent_revisions.tenant_id
          AND published_child.id = agent_revisions.agent_id
          AND published_child.published_revision_id = agent_revisions.id
    )
    RETURNING *
), target_child_revisions AS (
    SELECT * FROM inserted_child_revisions
    UNION ALL
    SELECT revision.*
    FROM agent_revisions revision
    JOIN requested_members member
      ON member.tenant_id = revision.tenant_id
     AND member.child_agent_id = revision.agent_id
     AND member.revision_no = revision.revision_no
    WHERE NOT EXISTS (
        SELECT 1 FROM inserted_child_revisions inserted
        WHERE inserted.tenant_id = revision.tenant_id AND inserted.id = revision.id
    )
), inserted_tools AS (
    INSERT INTO agent_revision_builtin_tools (tenant_id, revision_id, tool_key, ordinal, config)
    SELECT revision.tenant_id, revision.id, tool.value,
           tool.ordinality::int - 1, '{}'::jsonb
    FROM upserted_revision revision
    CROSS JOIN LATERAL jsonb_array_elements_text(sqlc.arg(tools)::jsonb)
        WITH ORDINALITY AS tool(value, ordinality)
    ON CONFLICT DO NOTHING
    RETURNING *
), inserted_external_tools AS (
    INSERT INTO agent_revision_external_tools (
        tenant_id, revision_id, external_tool_id, tool_schema_checksum, ordinal, config
    )
    SELECT revision.tenant_id, revision.id, requested.value,
           external_tools.schema_checksum, requested.ordinality::int - 1, '{}'::jsonb
    FROM upserted_revision revision
    CROSS JOIN LATERAL jsonb_array_elements_text(sqlc.arg(external_tool_ids)::jsonb)
        WITH ORDINALITY AS requested(value, ordinality)
    JOIN external_tools
      ON external_tools.tenant_id = revision.tenant_id
     AND external_tools.id = requested.value
     AND external_tools.enabled
     AND external_tools.archived_at IS NULL
    JOIN external_tool_connections external_connections
      ON external_connections.tenant_id = external_tools.tenant_id
     AND external_connections.id = external_tools.connection_id
     AND external_connections.status = 'active'
    ON CONFLICT DO NOTHING
    RETURNING *
), inserted_knowledge AS (
    INSERT INTO agent_revision_knowledge_bases (
        tenant_id, revision_id, knowledge_base_id, ordinal
    )
    SELECT revision.tenant_id, revision.id, knowledge.value,
           knowledge.ordinality::int - 1
    FROM upserted_revision revision
    CROSS JOIN LATERAL jsonb_array_elements_text(sqlc.arg(knowledge_base_ids)::jsonb)
        WITH ORDINALITY AS knowledge(value, ordinality)
    ON CONFLICT DO NOTHING
    RETURNING *
), inserted_child_tools AS (
    INSERT INTO agent_revision_builtin_tools (
        tenant_id, revision_id, tool_key, ordinal, config
    )
    SELECT member.tenant_id, revision.id,
           tool.value, tool.ordinality::int - 1, '{}'::jsonb
    FROM requested_members member
    JOIN target_child_revisions revision
      ON revision.tenant_id = member.tenant_id
     AND revision.agent_id = member.child_agent_id
     AND revision.revision_no = member.revision_no
    CROSS JOIN LATERAL jsonb_array_elements_text(COALESCE(member.tools, '[]'::jsonb))
        WITH ORDINALITY AS tool(value, ordinality)
    ON CONFLICT DO NOTHING
    RETURNING *
), inserted_child_external_tools AS (
    INSERT INTO agent_revision_external_tools (
        tenant_id, revision_id, external_tool_id, tool_schema_checksum, ordinal, config
    )
    SELECT member.tenant_id, revision.id,
           requested.value, external_tools.schema_checksum,
           requested.ordinality::int - 1, '{}'::jsonb
    FROM requested_members member
    JOIN target_child_revisions revision
      ON revision.tenant_id = member.tenant_id
     AND revision.agent_id = member.child_agent_id
     AND revision.revision_no = member.revision_no
    CROSS JOIN LATERAL jsonb_array_elements_text(COALESCE(member.external_tool_ids, '[]'::jsonb))
        WITH ORDINALITY AS requested(value, ordinality)
    JOIN external_tools
      ON external_tools.tenant_id = member.tenant_id
     AND external_tools.id = requested.value
     AND external_tools.enabled
     AND external_tools.archived_at IS NULL
    JOIN external_tool_connections external_connections
      ON external_connections.tenant_id = external_tools.tenant_id
     AND external_connections.id = external_tools.connection_id
     AND external_connections.status = 'active'
    ON CONFLICT DO NOTHING
    RETURNING *
), inserted_child_knowledge AS (
    INSERT INTO agent_revision_knowledge_bases (
        tenant_id, revision_id, knowledge_base_id, ordinal
    )
    SELECT member.tenant_id, revision.id,
           knowledge.value, knowledge.ordinality::int - 1
    FROM requested_members member
    JOIN target_child_revisions revision
      ON revision.tenant_id = member.tenant_id
     AND revision.agent_id = member.child_agent_id
     AND revision.revision_no = member.revision_no
    CROSS JOIN LATERAL jsonb_array_elements_text(COALESCE(member.knowledge_base_ids, '[]'::jsonb))
        WITH ORDINALITY AS knowledge(value, ordinality)
    ON CONFLICT DO NOTHING
    RETURNING *
)
SELECT id, tenant_id, agent_id, revision_no
FROM upserted_revision;

-- name: ListAgentDefinitionVersions :many
SELECT
    revisions.id, revisions.tenant_id, revisions.agent_id,
    revisions.revision_no AS version,
    revisions.name, revisions.description, revisions.icon AS emoji, revisions.category,
    revisions.visibility, revisions.visibility_targets, revisions.main_agent_role,
    COALESCE((SELECT jsonb_agg(jsonb_build_object(
        'id', substring(child.id FROM char_length(revisions.agent_id || ':member:') + 1),
        'name', child_revision.name, 'role', child_revision.main_agent_role,
        'model_id', child_revision.model_connection_id,
        'model_config_checksum', child_revision.model_config_checksum,
        'tools', COALESCE((SELECT jsonb_agg(tools.tool_key ORDER BY tools.ordinal) FROM agent_revision_builtin_tools tools WHERE tools.tenant_id = child_revision.tenant_id AND tools.revision_id = child_revision.id), '[]'::jsonb),
        'external_tool_ids', COALESCE((SELECT jsonb_agg(tools.external_tool_id ORDER BY tools.ordinal) FROM agent_revision_external_tools tools WHERE tools.tenant_id = child_revision.tenant_id AND tools.revision_id = child_revision.id), '[]'::jsonb),
        'knowledge_base_ids', COALESCE((SELECT jsonb_agg(kb.knowledge_base_id ORDER BY kb.ordinal) FROM agent_revision_knowledge_bases kb WHERE kb.tenant_id = child_revision.tenant_id AND kb.revision_id = child_revision.id), '[]'::jsonb)
    ) ORDER BY child_revision.ordinal)
    FROM agents child
    JOIN agent_revisions child_revision
      ON child_revision.tenant_id = child.tenant_id
     AND child_revision.agent_id = child.id
     AND child_revision.revision_no = revisions.revision_no
    WHERE child.tenant_id = revisions.tenant_id
      AND child.parent_agent_id = revisions.agent_id), '[]'::jsonb) AS sub_agents,
    revisions.system_prompt, revisions.welcome_message, revisions.suggested_questions,
    revisions.suggested_question_translations,
    COALESCE((SELECT jsonb_agg(tools.tool_key ORDER BY tools.ordinal) FROM agent_revision_builtin_tools tools WHERE tools.tenant_id = revisions.tenant_id AND tools.revision_id = revisions.id), '[]'::jsonb) AS tools,
    COALESCE((SELECT jsonb_agg(tools.external_tool_id ORDER BY tools.ordinal) FROM agent_revision_external_tools tools WHERE tools.tenant_id = revisions.tenant_id AND tools.revision_id = revisions.id), '[]'::jsonb) AS external_tool_ids,
    COALESCE((SELECT jsonb_agg(kb.knowledge_base_id ORDER BY kb.ordinal) FROM agent_revision_knowledge_bases kb WHERE kb.tenant_id = revisions.tenant_id AND kb.revision_id = revisions.id), '[]'::jsonb) AS knowledge_base_ids,
    revisions.model_connection_id AS model_id,
    revisions.model_config_checksum,
    GREATEST(revisions.timeout_ms / 1000, 1)::integer AS timeout_seconds,
    revisions.config_schema_version,
    revisions.checksum,
    revisions.revision_note AS note,
    revisions.created_by_account_id, revisions.created_at
FROM agent_revisions revisions
WHERE revisions.tenant_id = sqlc.arg(tenant_id) AND revisions.agent_id = sqlc.arg(agent_id)
ORDER BY revisions.revision_no DESC;

-- name: GetAgentDefinitionVersion :one
SELECT
    revisions.id, revisions.tenant_id, revisions.agent_id,
    revisions.revision_no AS version,
    revisions.name, revisions.description, revisions.icon AS emoji, revisions.category,
    revisions.visibility, revisions.visibility_targets, revisions.main_agent_role,
    COALESCE((SELECT jsonb_agg(jsonb_build_object(
        'id', substring(child.id FROM char_length(revisions.agent_id || ':member:') + 1),
        'name', child_revision.name, 'role', child_revision.main_agent_role,
        'model_id', child_revision.model_connection_id,
        'model_config_checksum', child_revision.model_config_checksum,
        'tools', COALESCE((SELECT jsonb_agg(tools.tool_key ORDER BY tools.ordinal) FROM agent_revision_builtin_tools tools WHERE tools.tenant_id = child_revision.tenant_id AND tools.revision_id = child_revision.id), '[]'::jsonb),
        'external_tool_ids', COALESCE((SELECT jsonb_agg(tools.external_tool_id ORDER BY tools.ordinal) FROM agent_revision_external_tools tools WHERE tools.tenant_id = child_revision.tenant_id AND tools.revision_id = child_revision.id), '[]'::jsonb),
        'knowledge_base_ids', COALESCE((SELECT jsonb_agg(kb.knowledge_base_id ORDER BY kb.ordinal) FROM agent_revision_knowledge_bases kb WHERE kb.tenant_id = child_revision.tenant_id AND kb.revision_id = child_revision.id), '[]'::jsonb)
    ) ORDER BY child_revision.ordinal)
    FROM agents child
    JOIN agent_revisions child_revision
      ON child_revision.tenant_id = child.tenant_id
     AND child_revision.agent_id = child.id
     AND child_revision.revision_no = revisions.revision_no
    WHERE child.tenant_id = revisions.tenant_id
      AND child.parent_agent_id = revisions.agent_id), '[]'::jsonb) AS sub_agents,
    revisions.system_prompt, revisions.welcome_message, revisions.suggested_questions,
    revisions.suggested_question_translations,
    COALESCE((SELECT jsonb_agg(tools.tool_key ORDER BY tools.ordinal) FROM agent_revision_builtin_tools tools WHERE tools.tenant_id = revisions.tenant_id AND tools.revision_id = revisions.id), '[]'::jsonb) AS tools,
    COALESCE((SELECT jsonb_agg(tools.external_tool_id ORDER BY tools.ordinal) FROM agent_revision_external_tools tools WHERE tools.tenant_id = revisions.tenant_id AND tools.revision_id = revisions.id), '[]'::jsonb) AS external_tool_ids,
    COALESCE((SELECT jsonb_agg(kb.knowledge_base_id ORDER BY kb.ordinal) FROM agent_revision_knowledge_bases kb WHERE kb.tenant_id = revisions.tenant_id AND kb.revision_id = revisions.id), '[]'::jsonb) AS knowledge_base_ids,
    revisions.model_connection_id AS model_id,
    revisions.model_config_checksum,
    GREATEST(revisions.timeout_ms / 1000, 1)::integer AS timeout_seconds,
    revisions.config_schema_version,
    revisions.checksum,
    revisions.revision_note AS note,
    revisions.created_by_account_id, revisions.created_at
FROM agent_revisions revisions
WHERE revisions.tenant_id = sqlc.arg(tenant_id)
  AND revisions.agent_id = sqlc.arg(agent_id)
  AND revisions.revision_no = sqlc.arg(version);


-- name: GetAgentExternalToolV2 :one
SELECT
    connections.id, connections.tenant_id, connections.name, connections.description,
    connections.kind, connections.transport, connections.endpoint_url,
    connections.auth_type, connections.auth_header_name, connections.auth_username,
    connections.auth_secret_ciphertext,
    GREATEST(connections.timeout_ms / 1000, 1)::integer AS timeout_seconds,
    connections.status, connections.last_tested_at, connections.last_test_status, connections.last_test_message,
    connections.created_by_account_id, connections.created_at, connections.updated_at, connections.archived_at
FROM external_tool_connections connections
WHERE connections.tenant_id = sqlc.arg(tenant_id) AND connections.id = sqlc.arg(id);

-- name: UpdateAgentExternalToolTestResultV2 :one
WITH updated AS (
    UPDATE external_tool_connections
    SET last_tested_at = sqlc.arg(last_tested_at),
        last_test_status = sqlc.arg(last_test_status),
        last_test_message = sqlc.arg(last_test_message),
        updated_at = sqlc.arg(last_tested_at)
    WHERE external_tool_connections.tenant_id = sqlc.arg(tenant_id)
      AND external_tool_connections.id = sqlc.arg(id)
    RETURNING *
)
SELECT
    updated.id, updated.tenant_id, updated.name, updated.description,
    updated.kind, updated.transport, updated.endpoint_url,
    updated.auth_type, updated.auth_header_name, updated.auth_username,
    updated.auth_secret_ciphertext,
    GREATEST(updated.timeout_ms / 1000, 1)::integer AS timeout_seconds,
    updated.status, updated.last_tested_at, updated.last_test_status, updated.last_test_message,
    updated.created_by_account_id, updated.created_at, updated.updated_at, updated.archived_at
FROM updated;

-- name: ArchiveAgentExternalToolCapabilitiesV2 :exec
UPDATE external_tools
SET enabled = false,
    archived_at = COALESCE(archived_at, sqlc.arg(archived_at)),
    updated_at = sqlc.arg(archived_at)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND connection_id = sqlc.arg(connection_id)
  AND archived_at IS NULL;

-- name: UpsertAgentExternalToolCapabilityV2 :one
INSERT INTO external_tools (
    id, tenant_id, connection_id, tool_name, description,
    http_method, http_path, input_schema, output_schema,
    readonly, enabled, schema_checksum, discovered_at, updated_at, archived_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(connection_id), sqlc.arg(tool_name), sqlc.arg(description),
    sqlc.arg(http_method), sqlc.arg(http_path), sqlc.arg(input_schema)::jsonb, sqlc.arg(output_schema)::jsonb,
    sqlc.arg(readonly), sqlc.arg(enabled), sqlc.arg(schema_checksum),
    sqlc.arg(discovered_at), sqlc.arg(updated_at), NULL
)
ON CONFLICT (tenant_id, connection_id, tool_name) DO UPDATE SET
    description = EXCLUDED.description,
    http_method = EXCLUDED.http_method,
    http_path = EXCLUDED.http_path,
    input_schema = EXCLUDED.input_schema,
    output_schema = EXCLUDED.output_schema,
    readonly = EXCLUDED.readonly,
    enabled = EXCLUDED.enabled,
    schema_checksum = EXCLUDED.schema_checksum,
    discovered_at = EXCLUDED.discovered_at,
    updated_at = EXCLUDED.updated_at,
    archived_at = NULL
RETURNING *;

-- name: GetAgentExternalToolCapabilityV2 :one
SELECT tools.*
FROM external_tools tools
JOIN external_tool_connections connections
  ON connections.tenant_id = tools.tenant_id
 AND connections.id = tools.connection_id
 AND connections.status = 'active'
WHERE tools.tenant_id = sqlc.arg(tenant_id)
  AND tools.id = sqlc.arg(id)
  AND tools.enabled
  AND tools.archived_at IS NULL;

-- name: ListAgentExternalToolCapabilitiesV2 :many
SELECT tools.*
FROM external_tools tools
JOIN external_tool_connections connections
  ON connections.tenant_id = tools.tenant_id
 AND connections.id = tools.connection_id
 AND connections.status = 'active'
WHERE tools.tenant_id = sqlc.arg(tenant_id)
  AND tools.connection_id = sqlc.arg(connection_id)
  AND tools.enabled
  AND tools.archived_at IS NULL
ORDER BY tools.tool_name, tools.id;

-- name: ListAgentExternalToolCapabilitiesAllV2 :many
SELECT *
FROM external_tools
WHERE tenant_id = sqlc.arg(tenant_id)
  AND connection_id = sqlc.arg(connection_id)
ORDER BY archived_at NULLS FIRST, tool_name, id;

-- name: ListAgentExternalToolCapabilitiesByIDsV2 :many
SELECT tools.*
FROM external_tools tools
JOIN external_tool_connections connections
  ON connections.tenant_id = tools.tenant_id
 AND connections.id = tools.connection_id
 AND connections.status = 'active'
WHERE tools.tenant_id = sqlc.arg(tenant_id)
  AND tools.id = ANY(sqlc.arg(ids)::text[])
  AND tools.enabled
  AND tools.archived_at IS NULL
ORDER BY tools.tool_name, tools.id;

-- name: ListAgentRevisionExternalToolBindingsV2 :many
SELECT *
FROM agent_revision_external_tools
WHERE tenant_id = sqlc.arg(tenant_id)
  AND revision_id = sqlc.arg(revision_id)
ORDER BY ordinal, external_tool_id;
