\if :{?tenant_id}
\else
\set tenant_id demo
\endif

BEGIN;

SELECT set_config('app.tenant_id', :'tenant_id', true);

CREATE TEMP TABLE updated_leave_agent ON COMMIT DROP AS
WITH desired AS (
    SELECT
        '你是 Nexus Pro 請假 Agent。先查詢假期餘額與可用請假表單，再確認假別、日期、時數、事由與代理人。若使用者未提供開始時間與結束時間，不要追問，後端會依 Asia/Shanghai 當天與考勤政策的標準上下班時間自動填入；建立草稿後必須向使用者說明實際採用的日期、時間與時數。只能建立或更新可撤銷草稿，提交前必須呼叫預覽工具並等待使用者在確認卡上確認。'::text AS system_prompt
), updated AS (
    UPDATE agent_definitions definition
    SET system_prompt = desired.system_prompt,
        version = definition.version + 1,
        published_version = definition.version + 1,
        updated_at = clock_timestamp()
    FROM desired
    WHERE definition.tenant_id = :'tenant_id'
      AND definition.id = 'adef-' || :'tenant_id' || '-leave'
      AND definition.system_prompt IS DISTINCT FROM desired.system_prompt
    RETURNING definition.*
)
SELECT * FROM updated;

INSERT INTO agent_definition_versions (
    id, tenant_id, agent_id, version, main_agent_role, sub_agents, system_prompt,
    tools, knowledge_base_ids, model_id, note, created_by_account_id, created_at
)
SELECT
    'adefv-' || tenant_id || '-leave-default-time-v' || version,
    tenant_id,
    id,
    version,
    main_agent_role,
    sub_agents,
    system_prompt,
    tools,
    knowledge_base_ids,
    model_id,
    'default missing leave times to today',
    updated_by_account_id,
    updated_at
FROM updated_leave_agent
ON CONFLICT (tenant_id, agent_id, version) DO NOTHING;

COMMIT;

SELECT id, name, status, version, published_version, system_prompt
FROM agent_definitions
WHERE tenant_id = :'tenant_id'
  AND id = 'adef-' || :'tenant_id' || '-leave';
