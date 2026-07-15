-- name: UpsertAgentModel :one
INSERT INTO agent_models (
    id, tenant_id, name, provider, model_name, litellm_model,
    api_base_url, api_key_ciphertext, api_key_preview, rate_limit_rpm,
    status, timeout_seconds,
    monthly_quota, used_quota, last_tested_at, last_test_status,
    last_test_message, sync_status, last_synced_at, last_sync_error,
    synced_config_hash, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(name), sqlc.arg(provider),
    sqlc.arg(model_name), sqlc.arg(litellm_model), sqlc.arg(api_base_url),
    sqlc.arg(api_key_ciphertext), sqlc.arg(api_key_preview), sqlc.arg(rate_limit_rpm), sqlc.arg(status),
    sqlc.arg(timeout_seconds),
    sqlc.arg(monthly_quota), sqlc.arg(used_quota), sqlc.arg(last_tested_at),
    sqlc.arg(last_test_status), sqlc.arg(last_test_message),
    sqlc.arg(sync_status), sqlc.arg(last_synced_at), sqlc.arg(last_sync_error),
    sqlc.arg(synced_config_hash),
    sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    name = EXCLUDED.name,
    provider = EXCLUDED.provider,
    model_name = EXCLUDED.model_name,
    litellm_model = EXCLUDED.litellm_model,
    api_base_url = EXCLUDED.api_base_url,
    api_key_ciphertext = EXCLUDED.api_key_ciphertext,
    api_key_preview = EXCLUDED.api_key_preview,
    rate_limit_rpm = EXCLUDED.rate_limit_rpm,
    status = EXCLUDED.status,
    timeout_seconds = EXCLUDED.timeout_seconds,
    monthly_quota = EXCLUDED.monthly_quota,
    used_quota = EXCLUDED.used_quota,
    last_tested_at = EXCLUDED.last_tested_at,
    last_test_status = EXCLUDED.last_test_status,
    last_test_message = EXCLUDED.last_test_message,
    sync_status = EXCLUDED.sync_status,
    last_synced_at = EXCLUDED.last_synced_at,
    last_sync_error = EXCLUDED.last_sync_error,
    synced_config_hash = EXCLUDED.synced_config_hash,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetAgentModel :one
SELECT * FROM agent_models
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- name: ListAgentModels :many
SELECT * FROM agent_models
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY updated_at DESC, id ASC;

-- name: DeleteAgentModel :one
DELETE FROM agent_models
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
RETURNING *;

-- name: UpdateAgentModelTestResult :one
UPDATE agent_models
SET last_tested_at = sqlc.arg(last_tested_at),
    last_test_status = sqlc.arg(last_test_status),
    last_test_message = sqlc.arg(last_test_message),
    updated_at = sqlc.arg(updated_at)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
RETURNING *;

-- name: UpdateAgentModelSyncResult :one
UPDATE agent_models
SET sync_status = sqlc.arg(sync_status),
    last_synced_at = sqlc.arg(last_synced_at),
    last_sync_error = sqlc.arg(last_sync_error),
    synced_config_hash = sqlc.arg(synced_config_hash),
    updated_at = sqlc.arg(updated_at)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
RETURNING *;

-- name: CountAgentDefinitionsByModel :one
SELECT (
    SELECT count(*) FROM agent_definitions
    WHERE agent_definitions.tenant_id = sqlc.arg(tenant_id)
      AND (
        agent_definitions.model_id = sqlc.arg(model_id)
        OR EXISTS (
            SELECT 1 FROM jsonb_array_elements(agent_definitions.sub_agents) member
            WHERE member->>'model_id' = sqlc.arg(model_id)
        )
      )
) + (
    SELECT count(*) FROM agent_definition_versions
    WHERE agent_definition_versions.tenant_id = sqlc.arg(tenant_id)
      AND EXISTS (
        SELECT 1 FROM jsonb_array_elements(agent_definition_versions.sub_agents) member
        WHERE member->>'model_id' = sqlc.arg(model_id)
      )
);

-- name: InsertAgentExternalTool :one
INSERT INTO agent_external_tools (
    id, tenant_id, name, description, kind, transport, endpoint_url,
    auth_type, auth_header_name, auth_username, auth_secret_ciphertext,
    created_by_account_id, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(name), sqlc.arg(description), sqlc.arg(kind),
    sqlc.arg(transport), sqlc.arg(endpoint_url), sqlc.arg(auth_type), sqlc.arg(auth_header_name),
    sqlc.arg(auth_username), sqlc.arg(auth_secret_ciphertext), sqlc.arg(created_by_account_id), sqlc.arg(created_at)
)
RETURNING *;

-- name: ListAgentExternalTools :many
SELECT * FROM agent_external_tools
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY created_at DESC, id ASC;

-- name: DeleteAgentExternalTool :one
DELETE FROM agent_external_tools
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
RETURNING *;

-- name: CountAgentDefinitionsByKnowledgeBase :one
SELECT (
    SELECT count(*) FROM agent_definitions
    WHERE agent_definitions.tenant_id = sqlc.arg(tenant_id)
      AND (
        agent_definitions.knowledge_base_ids ? sqlc.arg(knowledge_base_id)::text
        OR EXISTS (
            SELECT 1 FROM jsonb_array_elements(agent_definitions.sub_agents) member
            WHERE COALESCE(member->'knowledge_base_ids', '[]'::jsonb) ? sqlc.arg(knowledge_base_id)::text
        )
      )
) + (
    SELECT count(*) FROM agent_definition_versions
    WHERE agent_definition_versions.tenant_id = sqlc.arg(tenant_id)
      AND (
        agent_definition_versions.knowledge_base_ids ? sqlc.arg(knowledge_base_id)::text
        OR EXISTS (
            SELECT 1 FROM jsonb_array_elements(agent_definition_versions.sub_agents) member
            WHERE COALESCE(member->'knowledge_base_ids', '[]'::jsonb) ? sqlc.arg(knowledge_base_id)::text
        )
      )
);

-- name: UpsertAgentDefinition :one
INSERT INTO agent_definitions (
    id, tenant_id, name, description, emoji, category, model_id,
    main_agent_role, sub_agents, system_prompt, welcome_message, suggested_questions, suggested_question_translations, tools, knowledge_base_ids, status, visibility, visibility_targets, timeout_seconds,
    version, published_version, usage_total_runs, usage_success_runs, usage_failed_runs, usage_avg_latency_ms,
    usage_last_run_at, usage_top_prompts, created_by_account_id, updated_by_account_id,
    created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(name), sqlc.arg(description),
    sqlc.arg(emoji), sqlc.arg(category), sqlc.arg(model_id),
    sqlc.arg(main_agent_role), sqlc.arg(sub_agents)::jsonb, sqlc.arg(system_prompt), sqlc.arg(welcome_message), sqlc.arg(suggested_questions)::jsonb, sqlc.arg(suggested_question_translations)::jsonb, sqlc.arg(tools)::jsonb, sqlc.arg(knowledge_base_ids)::jsonb, sqlc.arg(status), sqlc.arg(visibility),
    sqlc.arg(visibility_targets)::jsonb, sqlc.arg(timeout_seconds), sqlc.arg(version), sqlc.arg(published_version),
    sqlc.arg(usage_total_runs), sqlc.arg(usage_success_runs), sqlc.arg(usage_failed_runs),
    sqlc.arg(usage_avg_latency_ms), sqlc.arg(usage_last_run_at), sqlc.arg(usage_top_prompts)::jsonb,
    sqlc.arg(created_by_account_id), sqlc.arg(updated_by_account_id),
    sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    emoji = EXCLUDED.emoji,
    category = EXCLUDED.category,
    model_id = EXCLUDED.model_id,
	main_agent_role = EXCLUDED.main_agent_role,
    sub_agents = EXCLUDED.sub_agents,
    system_prompt = EXCLUDED.system_prompt,
    welcome_message = EXCLUDED.welcome_message,
    suggested_questions = EXCLUDED.suggested_questions,
    suggested_question_translations = EXCLUDED.suggested_question_translations,
    tools = EXCLUDED.tools,
    knowledge_base_ids = EXCLUDED.knowledge_base_ids,
    status = EXCLUDED.status,
    visibility = EXCLUDED.visibility,
    visibility_targets = EXCLUDED.visibility_targets,
    timeout_seconds = EXCLUDED.timeout_seconds,
    version = EXCLUDED.version,
	published_version = EXCLUDED.published_version,
    usage_total_runs = EXCLUDED.usage_total_runs,
    usage_success_runs = EXCLUDED.usage_success_runs,
    usage_failed_runs = EXCLUDED.usage_failed_runs,
    usage_avg_latency_ms = EXCLUDED.usage_avg_latency_ms,
    usage_last_run_at = EXCLUDED.usage_last_run_at,
    usage_top_prompts = EXCLUDED.usage_top_prompts,
    created_by_account_id = EXCLUDED.created_by_account_id,
    updated_by_account_id = EXCLUDED.updated_by_account_id,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetAgentDefinition :one
SELECT * FROM agent_definitions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- name: ListAgentDefinitions :many
SELECT * FROM agent_definitions
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY status ASC, updated_at DESC, id ASC;

-- name: ListPublishedAgentDefinitions :many
SELECT * FROM agent_definitions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND status = 'published'
ORDER BY updated_at DESC, id ASC;

-- name: DeleteAgentDefinition :one
DELETE FROM agent_definitions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
RETURNING *;

-- name: UpdateAgentDefinitionUsage :one
UPDATE agent_definitions
SET usage_total_runs = usage_total_runs + 1,
    usage_success_runs = usage_success_runs + CASE WHEN sqlc.arg(success)::boolean THEN 1 ELSE 0 END,
    usage_failed_runs = usage_failed_runs + CASE WHEN sqlc.arg(success)::boolean THEN 0 ELSE 1 END,
    usage_avg_latency_ms = CASE
        WHEN sqlc.arg(latency_ms)::int <= 0 THEN usage_avg_latency_ms
        ELSE ((usage_avg_latency_ms::bigint * usage_total_runs) + sqlc.arg(latency_ms)::int) / (usage_total_runs + 1)
    END,
    usage_last_run_at = sqlc.arg(run_at),
    usage_top_prompts = CASE
        WHEN sqlc.arg(prompt)::text = '' THEN usage_top_prompts
        ELSE (
            SELECT COALESCE(jsonb_agg(to_jsonb(value)), '[]'::jsonb)
            FROM (
                SELECT value
                FROM (
                    SELECT jsonb_array_elements_text(
                        jsonb_build_array(sqlc.arg(prompt)::text) || COALESCE(usage_top_prompts, '[]'::jsonb)
                    ) AS value
                ) ranked
                LIMIT 5
            ) limited
        )
    END,
    updated_at = sqlc.arg(run_at)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
RETURNING *;

-- name: InsertAgentDefinitionVersion :one
INSERT INTO agent_definition_versions (
    id, tenant_id, agent_id, version, main_agent_role, sub_agents, system_prompt, welcome_message, suggested_questions, suggested_question_translations, tools, knowledge_base_ids, model_id, note,
    created_by_account_id, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(agent_id), sqlc.arg(version),
    sqlc.arg(main_agent_role), sqlc.arg(sub_agents)::jsonb, sqlc.arg(system_prompt), sqlc.arg(welcome_message), sqlc.arg(suggested_questions)::jsonb, sqlc.arg(suggested_question_translations)::jsonb, sqlc.arg(tools)::jsonb, sqlc.arg(knowledge_base_ids)::jsonb, sqlc.arg(model_id), sqlc.arg(note),
    sqlc.arg(created_by_account_id), sqlc.arg(created_at)
)
RETURNING *;

-- name: ListAgentDefinitionVersions :many
SELECT * FROM agent_definition_versions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND agent_id = sqlc.arg(agent_id)
ORDER BY version DESC;

-- name: GetAgentDefinitionVersion :one
SELECT * FROM agent_definition_versions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND agent_id = sqlc.arg(agent_id)
  AND version = sqlc.arg(version);
