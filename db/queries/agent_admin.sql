-- name: UpsertAgentModel :one
INSERT INTO agent_models (
    id, tenant_id, name, provider, model_name, litellm_model,
    api_base_url, api_key, rate_limit_rpm,
    status, timeout_seconds,
    monthly_quota, used_quota, last_tested_at, last_test_status,
    last_test_message, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(name), sqlc.arg(provider),
    sqlc.arg(model_name), sqlc.arg(litellm_model), sqlc.arg(api_base_url),
    sqlc.arg(api_key), sqlc.arg(rate_limit_rpm), sqlc.arg(status),
    sqlc.arg(timeout_seconds),
    sqlc.arg(monthly_quota), sqlc.arg(used_quota), sqlc.arg(last_tested_at),
    sqlc.arg(last_test_status), sqlc.arg(last_test_message),
    sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    name = EXCLUDED.name,
    provider = EXCLUDED.provider,
    model_name = EXCLUDED.model_name,
    litellm_model = EXCLUDED.litellm_model,
    api_base_url = EXCLUDED.api_base_url,
    api_key = EXCLUDED.api_key,
    rate_limit_rpm = EXCLUDED.rate_limit_rpm,
    status = EXCLUDED.status,
    timeout_seconds = EXCLUDED.timeout_seconds,
    monthly_quota = EXCLUDED.monthly_quota,
    used_quota = EXCLUDED.used_quota,
    last_tested_at = EXCLUDED.last_tested_at,
    last_test_status = EXCLUDED.last_test_status,
    last_test_message = EXCLUDED.last_test_message,
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

-- name: CountAgentDefinitionsByModel :one
SELECT count(*) FROM agent_definitions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND model_id = sqlc.arg(model_id);

-- name: UpsertAgentDefinition :one
INSERT INTO agent_definitions (
    id, tenant_id, name, description, emoji, category, model_id,
    system_prompt, tools, status, visibility, visibility_targets, timeout_seconds,
    version, usage_total_runs, usage_success_runs, usage_failed_runs, usage_avg_latency_ms,
    usage_last_run_at, usage_top_prompts, created_by_account_id, updated_by_account_id,
    created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(name), sqlc.arg(description),
    sqlc.arg(emoji), sqlc.arg(category), sqlc.arg(model_id),
    sqlc.arg(system_prompt), sqlc.arg(tools)::jsonb, sqlc.arg(status), sqlc.arg(visibility),
    sqlc.arg(visibility_targets)::jsonb, sqlc.arg(timeout_seconds), sqlc.arg(version),
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
    system_prompt = EXCLUDED.system_prompt,
    tools = EXCLUDED.tools,
    status = EXCLUDED.status,
    visibility = EXCLUDED.visibility,
    visibility_targets = EXCLUDED.visibility_targets,
    timeout_seconds = EXCLUDED.timeout_seconds,
    version = EXCLUDED.version,
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
    id, tenant_id, agent_id, version, system_prompt, tools, model_id, note,
    created_by_account_id, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(agent_id), sqlc.arg(version),
    sqlc.arg(system_prompt), sqlc.arg(tools)::jsonb, sqlc.arg(model_id), sqlc.arg(note),
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

-- name: InsertAgentAudit :one
INSERT INTO agent_audits (
    id, tenant_id, entity_type, entity_id, entity_name, action,
    actor_account_id, actor_display_name, detail, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(entity_type), sqlc.arg(entity_id),
    sqlc.arg(entity_name), sqlc.arg(action), sqlc.arg(actor_account_id),
    sqlc.arg(actor_display_name), sqlc.arg(detail), sqlc.arg(created_at)
)
RETURNING *;

-- name: ListAgentAudits :many
SELECT * FROM agent_audits
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY created_at DESC, id DESC;
