\if :{?tenant_id}
\else
\set tenant_id demo
\endif

BEGIN;

SELECT set_config('app.tenant_id', :'tenant_id', true);

CREATE TEMP TABLE seed_agent_context ON COMMIT DROP AS
SELECT
    t.id AS tenant_id,
    (
        SELECT m.id
        FROM agent_models m
        WHERE m.tenant_id = t.id
          AND m.status = 'active'
          AND m.sync_status = 'synced'
        ORDER BY m.updated_at DESC, m.id
        LIMIT 1
    ) AS model_id,
    (
        SELECT a.id
        FROM accounts a
        JOIN permission_sets ps
          ON ps.tenant_id = a.tenant_id
         AND ps.id = ANY(a.direct_permission_set_ids)
        WHERE a.tenant_id = t.id
          AND a.status = 'active'
          AND ps.name = 'Platform Admin'
        ORDER BY a.created_at, a.id
        LIMIT 1
    ) AS actor_id,
    (
        SELECT a.display_name
        FROM accounts a
        JOIN permission_sets ps
          ON ps.tenant_id = a.tenant_id
         AND ps.id = ANY(a.direct_permission_set_ids)
        WHERE a.tenant_id = t.id
          AND a.status = 'active'
          AND ps.name = 'Platform Admin'
        ORDER BY a.created_at, a.id
        LIMIT 1
    ) AS actor_name,
    clock_timestamp() AS seeded_at
FROM tenants t
WHERE t.id = :'tenant_id';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM seed_agent_context) THEN
        RAISE EXCEPTION 'tenant does not exist';
    END IF;
    IF EXISTS (SELECT 1 FROM seed_agent_context WHERE model_id IS NULL) THEN
        RAISE EXCEPTION 'tenant requires an active synced agent model before seeding assistants';
    END IF;
END
$$;

CREATE TEMP TABLE seeded_agent_ids ON COMMIT DROP AS
WITH definitions(
    suffix,
    name,
    description,
    emoji,
    main_agent_role,
    system_prompt,
    tools
) AS (
    VALUES
        (
            'assistant',
            '助理',
            '協助員工釐清需求、查找可用表單與待辦，並引導到合適的專用 Agent。',
            '🤖',
            '理解員工的 OA 與人資需求，優先回答通用問題，並在需要時引導到請假或補卡 Agent。',
            '你是 Nexus Pro 助理。先理解使用者目標，再透過可用工具查詢個人資料、待辦與已發布表單；遇到請假或補卡需求時，清楚建議使用對應的專用 Agent。不得編造公司制度或聲稱已完成未確認的操作。',
            '["get_my_profile","my_pending_reviews","list_published_form_templates","get_published_form_template"]'::jsonb
        ),
        (
            'leave',
            '請假 Agent',
            '查詢假期餘額、整理請假資料並建立可確認的請假表單草稿。',
            '🗓️',
            '專責處理請假查詢與申請草稿，提交前必須取得使用者確認。',
            '你是 Nexus Pro 請假 Agent。先查詢假期餘額與可用請假表單，再確認假別、日期、時數、事由與代理人。若使用者未提供開始時間與結束時間，不要追問，後端會依 Asia/Shanghai 當天與考勤政策的標準上下班時間自動填入；建立草稿後必須向使用者說明實際採用的日期、時間與時數。只能建立或更新可撤銷草稿，提交前必須呼叫預覽工具並等待使用者在確認卡上確認。',
            '["get_my_profile","my_leave_balances","list_employees","list_published_form_templates","get_published_form_template","create_form_draft","update_form_draft","preview_form_submission"]'::jsonb
        ),
        (
            'punch-correction',
            '補卡 Agent',
            '查詢打卡紀錄、定位漏打或異常時段，並建立可確認的補卡表單草稿。',
            '🕒',
            '專責處理打卡異常與補卡草稿，先以真實打卡紀錄確認缺口。',
            '你是 Nexus Pro 補卡 Agent。先查詢使用者的打卡紀錄與已發布補卡表單，確認補卡日期、上班或下班時段、正確時間與原因；只能建立或更新可撤銷草稿，提交前必須呼叫預覽工具並等待使用者在確認卡上確認。',
            '["get_my_profile","my_clock_records","list_published_form_templates","get_published_form_template","create_form_draft","update_form_draft","preview_form_submission"]'::jsonb
        )
), inserted AS (
    INSERT INTO agent_definitions (
        id,
        tenant_id,
        name,
        description,
        emoji,
        category,
        model_id,
        main_agent_role,
        sub_agents,
        system_prompt,
        tools,
        status,
        visibility,
        visibility_targets,
        timeout_seconds,
        version,
        published_version,
        created_by_account_id,
        updated_by_account_id,
        created_at,
        updated_at
    )
    SELECT
        'adef-' || context.tenant_id || '-' || definitions.suffix,
        context.tenant_id,
        definitions.name,
        definitions.description,
        definitions.emoji,
        'workflow',
        context.model_id,
        definitions.main_agent_role,
        '[]'::jsonb,
        definitions.system_prompt,
        definitions.tools,
        'published',
        'all',
        '[]'::jsonb,
        60,
        1,
        1,
        context.actor_id,
        context.actor_id,
        context.seeded_at,
        context.seeded_at
    FROM definitions
    CROSS JOIN seed_agent_context context
    ON CONFLICT (id) DO NOTHING
    RETURNING id, tenant_id, name
)
SELECT * FROM inserted;

INSERT INTO agent_definition_versions (
    id,
    tenant_id,
    agent_id,
    version,
    main_agent_role,
    sub_agents,
    system_prompt,
    tools,
    model_id,
    note,
    created_by_account_id,
    created_at
)
SELECT
    definition.id || '-v1',
    definition.tenant_id,
    definition.id,
    1,
    definition.main_agent_role,
    definition.sub_agents,
    definition.system_prompt,
    definition.tools,
    definition.model_id,
    'initial published configuration',
    definition.created_by_account_id,
    definition.created_at
FROM agent_definitions definition
JOIN seed_agent_context context ON context.tenant_id = definition.tenant_id
WHERE definition.id IN (
    'adef-' || context.tenant_id || '-assistant',
    'adef-' || context.tenant_id || '-leave',
    'adef-' || context.tenant_id || '-punch-correction'
)
ON CONFLICT (tenant_id, agent_id, version) DO NOTHING;

INSERT INTO agent_audits (
    id,
    tenant_id,
    entity_type,
    entity_id,
    entity_name,
    action,
    actor_account_id,
    actor_display_name,
    detail,
    created_at
)
SELECT
    'aaud-' || seeded.id,
    seeded.tenant_id,
    'agent',
    seeded.id,
    seeded.name,
    'create',
    context.actor_id,
    COALESCE(context.actor_name, 'demo seed'),
    'demo assistant seeded and published',
    context.seeded_at
FROM seeded_agent_ids seeded
JOIN seed_agent_context context ON context.tenant_id = seeded.tenant_id
ON CONFLICT (id) DO NOTHING;

COMMIT;

SELECT id, name, status, visibility, version, published_version
FROM agent_definitions
WHERE tenant_id = :'tenant_id'
ORDER BY updated_at DESC, id;
