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
        SELECT model.id
        FROM model_connections model
        JOIN model_connection_state state
          ON state.tenant_id = model.tenant_id
         AND state.model_connection_id = model.id
        WHERE model.tenant_id = t.id
          AND model.status = 'active'
          AND state.sync_status = 'synced'
        ORDER BY model.updated_at DESC, model.id
        LIMIT 1
    ) AS model_connection_id,
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
    IF EXISTS (SELECT 1 FROM seed_agent_context WHERE model_connection_id IS NULL) THEN
        RAISE EXCEPTION 'tenant requires an active synced agent model before seeding assistants';
    END IF;
END
$$;

CREATE TEMP TABLE seed_agent_definitions ON COMMIT DROP AS
WITH definitions(
    suffix,
    name,
    description,
    emoji,
	main_agent_role,
	sub_agents,
    system_prompt,
    welcome_message,
    suggested_questions,
    tools
) AS (
    VALUES
        (
            'assistant',
            '助理',
            '協助員工釐清需求、查找可用表單與待辦，並委派給考勤或人事專用助理。',
            '🤖',
	            '理解員工的 OA 與人資需求；請假、加班、補卡與出勤查詢委派給考勤助理，人事異動、增補與離職申請委派給人事助理，並在取得結果後繼續完成目前對話。',
	            '[
	              {
	                "id": "leave",
	                "name": "考勤助理",
	                "role": "處理請假、加班與補卡申請，以及假期餘額、歷史請假、打卡紀錄與本月考勤摘要。建立請假前呼叫 check_leave_eligibility；所有申請只建立草稿並呼叫 preview_form_submission，等待使用者確認。",
	                "tools": ["get_my_profile","my_attendance_summary","my_form_history","my_leave_balances","check_leave_eligibility","my_clock_records","list_employees","list_published_form_templates","get_published_form_template","create_form_draft","update_form_draft","preview_form_submission"],
	                "knowledge_base_ids": []
	              },
	              {
	                "id": "punch-correction",
	                "name": "人事助理",
	                "role": "處理人事異動、人員增補、離職與退休等人事申請。依已發布表單與資料來源收集欄位，只建立草稿並呼叫 preview_form_submission，等待使用者確認。",
	                "tools": ["get_my_profile","list_employees","get_employee","my_form_history","list_published_form_templates","get_published_form_template","create_form_draft","update_form_draft","preview_form_submission"],
	                "knowledge_base_ids": []
	              }
	            ]'::jsonb,
	            '你是 Nexus Pro 助理。先理解使用者目標，再透過可用工具查詢個人資料、待辦與已發布表單；遇到請假、加班、補卡或考勤查詢時，必須呼叫考勤助理；遇到人事異動、人員增補、離職或退休申請時，必須呼叫人事助理。不要只推薦、不要要求使用者手動切換，也不要聲稱已完成未確認的操作。',
            '您好，我是您的 Nexus Pro 助理。請問今天想處理什麼？',
            '["查看我的待辦","我要處理考勤","我要發起人事申請"]'::jsonb,
            '["get_my_profile","my_pending_reviews","list_published_form_templates","get_published_form_template"]'::jsonb
        ),
        (
            'leave',
	            '考勤助理',
	            '處理請假、加班、補卡，以及假期餘額、歷史請假與本月考勤摘要。',
	            '🕒',
	            '專責員工考勤自助：建立請假、加班與補卡草稿，查詢假期餘額、歷史申請及本月出勤摘要；提交前必須取得使用者確認。',
	            '[]'::jsonb,
		            '你是 Nexus Pro 考勤助理，只處理員工考勤自助。你可以建立 leave-request 請假單、overtime-approval 加班單與 punch-fix 補卡單，並查詢假期餘額、本人表單歷史、打卡紀錄與本月考勤摘要。查詢歷史請假時必須呼叫 my_form_history 並指定 template_key=leave-request；查詢本月工時、出勤天數、已覈準請假或加班時數時必須呼叫 my_attendance_summary。建立任何單據前先呼叫 get_published_form_template 取得真實欄位與資料來源，不得猜測欄位 ID。建立請假草稿前必須呼叫 check_leave_eligibility；若餘額未初始化或不足，仍必須按不使用餘額繼續建立草稿，不得只回覆說明文字；my_leave_balances 的 initialized=false 表示餘額尚未初始化，不是餘額為零。若請假未提供開始與結束時間，不要追問，後端會依 Asia/Shanghai 當天與考勤政策自動填入，建立草稿後說明實際採用的日期、時間與時數。建立補卡前先查 my_clock_records 核對缺口。只能建立或更新可撤銷草稿，完成必填欄位後必須呼叫 preview_form_submission，等待使用者在確認卡上確認，絕不能聲稱已自動提交。',
	            '您好，我是考勤助理，可以協助請假、加班、補卡與考勤查詢。',
	            '["幫我申請下週一特休","幫我建立加班單","查看本月考勤摘要","查詢我的請假紀錄"]'::jsonb,
	            '["get_my_profile","my_attendance_summary","my_form_history","my_leave_balances","check_leave_eligibility","my_clock_records","list_employees","list_published_form_templates","get_published_form_template","create_form_draft","update_form_draft","preview_form_submission"]'::jsonb
        ),
        (
            'punch-correction',
	            '人事助理',
	            '處理人事異動、人員增補、離職與退休等人事申請，建立可確認的表單草稿。',
	            '👥',
	            '專責人事流程申請：依已發布表單建立人事異動、增補、離職或退休草稿，提交前必須取得使用者確認。',
	            '[]'::jsonb,
	            '你是 Nexus Pro 人事助理，只處理人事流程申請。你可以建立 job-change 人事變動單、headcount-request 人員增補申請單與 resignation 離職及退休申請單。建立單據前必須呼叫 get_published_form_template 取得真實欄位、選項、資料來源與審批路徑；涉及員工時使用 list_employees 或 get_employee 核對真實對象，不得猜測員工、部門、職務或薪資資料，不得把未取得的敏感資料補成預設值。只能建立或更新可撤銷草稿，完成必填欄位後必須呼叫 preview_form_submission，等待使用者在確認卡上確認，絕不能聲稱已自動提交。',
	            '您好，我是人事助理，可以協助人事異動、人員增補與離職申請。',
	            '["幫我建立人事變動單","我要申請人員增補","幫我準備離職申請單"]'::jsonb,
	            '["get_my_profile","list_employees","get_employee","my_form_history","list_published_form_templates","get_published_form_template","create_form_draft","update_form_draft","preview_form_submission"]'::jsonb
        )
)
SELECT * FROM definitions;

-- Agent v2 keeps stable identity separate from immutable runtime revisions.
INSERT INTO agents (
    id, tenant_id, parent_agent_id, lifecycle_status, draft_revision_id, published_revision_id,
    next_revision_no, created_by_account_id, created_at, updated_at, archived_at
)
SELECT
    'adef-' || context.tenant_id || '-' || definition.suffix,
    context.tenant_id,
    NULL,
    'active',
    NULL,
    NULL,
    2,
    context.actor_id,
    context.seeded_at,
    context.seeded_at,
    NULL
FROM seed_agent_definitions definition
CROSS JOIN seed_agent_context context
ON CONFLICT (id) DO UPDATE SET
    lifecycle_status = 'active',
    next_revision_no = GREATEST(agents.next_revision_no, 2),
    updated_at = EXCLUDED.updated_at,
    archived_at = NULL
WHERE agents.tenant_id = EXCLUDED.tenant_id;

INSERT INTO agent_revisions (
    id, tenant_id, agent_id, revision_no, ordinal, name, description, icon, category,
    visibility, visibility_targets, main_agent_role, system_prompt, welcome_message,
    suggested_questions, suggested_question_translations,
    model_connection_id, model_config_checksum, timeout_ms,
    config_schema_version, checksum, revision_note, created_by_account_id, created_at
)
SELECT
    'arev-' || context.tenant_id || '-' || definition.suffix || '-v1',
    context.tenant_id,
    'adef-' || context.tenant_id || '-' || definition.suffix,
    1,
    NULL,
    definition.name,
    definition.description,
    definition.emoji,
    'workflow',
    'all',
    '[]'::jsonb,
    definition.main_agent_role,
    definition.system_prompt,
    definition.welcome_message,
    definition.suggested_questions,
    '[]'::jsonb,
    context.model_connection_id,
    state.synced_config_checksum,
    60000,
    1,
    md5(concat_ws('|', context.model_connection_id, definition.main_agent_role,
        definition.system_prompt, definition.tools::text, definition.sub_agents::text)),
    'initial published demo configuration',
    context.actor_id,
    context.seeded_at
FROM seed_agent_definitions definition
CROSS JOIN seed_agent_context context
JOIN model_connection_state state
  ON state.tenant_id = context.tenant_id
 AND state.model_connection_id = context.model_connection_id
ON CONFLICT (tenant_id, agent_id, revision_no) DO NOTHING;

INSERT INTO agents (
    id, tenant_id, parent_agent_id, lifecycle_status,
    draft_revision_id, published_revision_id, next_revision_no,
    created_by_account_id, created_at, updated_at, archived_at
)
SELECT
    'adef-' || context.tenant_id || '-' || definition.suffix || ':member:' || member.id,
    context.tenant_id,
    'adef-' || context.tenant_id || '-' || definition.suffix,
    'active',
    NULL,
    NULL,
    2,
    context.actor_id,
    context.seeded_at,
    context.seeded_at,
    NULL
FROM seed_agent_definitions definition
CROSS JOIN seed_agent_context context
CROSS JOIN LATERAL jsonb_to_recordset(definition.sub_agents) WITH ORDINALITY
    AS member(id text, name text, role text, tools jsonb, knowledge_base_ids jsonb, ordinality bigint)
ON CONFLICT (id) DO UPDATE SET
    lifecycle_status = 'active',
    updated_at = EXCLUDED.updated_at,
    archived_at = NULL
WHERE agents.tenant_id = EXCLUDED.tenant_id
  AND agents.parent_agent_id = EXCLUDED.parent_agent_id;

INSERT INTO agent_revisions (
    id, tenant_id, agent_id, revision_no, ordinal,
    name, description, icon, category,
    visibility, visibility_targets, main_agent_role,
    system_prompt, welcome_message,
    suggested_questions, suggested_question_translations,
    model_connection_id, model_config_checksum, timeout_ms,
    config_schema_version, checksum, revision_note,
    created_by_account_id, created_at
)
SELECT
    revision.id || ':member:' || member.id,
    context.tenant_id,
    revision.agent_id || ':member:' || member.id,
    revision.revision_no,
    member.ordinality::int - 1,
    member.name,
    '',
    'AI',
    revision.category,
    revision.visibility,
    revision.visibility_targets,
    member.role,
    '',
    '',
    '[]'::jsonb,
    '[]'::jsonb,
    context.model_connection_id,
    state.synced_config_checksum,
    revision.timeout_ms,
    revision.config_schema_version,
    md5(concat_ws('|', revision.checksum, member.id, member.role, member.tools::text, member.knowledge_base_ids::text)),
    revision.revision_note,
    context.actor_id,
    context.seeded_at
FROM seed_agent_definitions definition
CROSS JOIN seed_agent_context context
JOIN model_connection_state state
  ON state.tenant_id = context.tenant_id
 AND state.model_connection_id = context.model_connection_id
JOIN agent_revisions revision
  ON revision.tenant_id = context.tenant_id
 AND revision.agent_id = 'adef-' || context.tenant_id || '-' || definition.suffix
 AND revision.id = 'arev-' || context.tenant_id || '-' || definition.suffix || '-v1'
 AND revision.revision_no = 1
CROSS JOIN LATERAL jsonb_to_recordset(definition.sub_agents) WITH ORDINALITY
    AS member(id text, name text, role text, tools jsonb, knowledge_base_ids jsonb, ordinality bigint)
ON CONFLICT (tenant_id, agent_id, revision_no) DO NOTHING;

INSERT INTO agent_revision_builtin_tools (tenant_id, revision_id, tool_key, ordinal, config)
SELECT
    context.tenant_id,
    revision.id,
    tool.value,
    tool.ordinality::int - 1,
    '{}'::jsonb
FROM seed_agent_definitions definition
CROSS JOIN seed_agent_context context
JOIN agent_revisions revision
  ON revision.tenant_id = context.tenant_id
 AND revision.agent_id = 'adef-' || context.tenant_id || '-' || definition.suffix
 AND revision.id = 'arev-' || context.tenant_id || '-' || definition.suffix || '-v1'
 AND revision.revision_no = 1
CROSS JOIN LATERAL jsonb_array_elements_text(definition.tools) WITH ORDINALITY AS tool(value, ordinality)
ON CONFLICT DO NOTHING;

INSERT INTO agent_revision_builtin_tools (
    tenant_id, revision_id, tool_key, ordinal, config
)
SELECT
    context.tenant_id,
    revision.id || ':member:' || member.id,
    tool.value,
    tool.ordinality::int - 1,
    '{}'::jsonb
FROM seed_agent_definitions definition
CROSS JOIN seed_agent_context context
JOIN agent_revisions revision
  ON revision.tenant_id = context.tenant_id
 AND revision.agent_id = 'adef-' || context.tenant_id || '-' || definition.suffix
 AND revision.id = 'arev-' || context.tenant_id || '-' || definition.suffix || '-v1'
 AND revision.revision_no = 1
CROSS JOIN LATERAL jsonb_to_recordset(definition.sub_agents)
    AS member(id text, tools jsonb, knowledge_base_ids jsonb)
CROSS JOIN LATERAL jsonb_array_elements_text(COALESCE(member.tools, '[]'::jsonb)) WITH ORDINALITY AS tool(value, ordinality)
ON CONFLICT DO NOTHING;

INSERT INTO agent_revision_knowledge_bases (
    tenant_id, revision_id, knowledge_base_id, ordinal
)
SELECT
    context.tenant_id,
    revision.id || ':member:' || member.id,
    knowledge.value,
    knowledge.ordinality::int - 1
FROM seed_agent_definitions definition
CROSS JOIN seed_agent_context context
JOIN agent_revisions revision
  ON revision.tenant_id = context.tenant_id
 AND revision.agent_id = 'adef-' || context.tenant_id || '-' || definition.suffix
 AND revision.id = 'arev-' || context.tenant_id || '-' || definition.suffix || '-v1'
 AND revision.revision_no = 1
CROSS JOIN LATERAL jsonb_to_recordset(definition.sub_agents)
    AS member(id text, tools jsonb, knowledge_base_ids jsonb)
CROSS JOIN LATERAL jsonb_array_elements_text(COALESCE(member.knowledge_base_ids, '[]'::jsonb)) WITH ORDINALITY AS knowledge(value, ordinality)
ON CONFLICT DO NOTHING;

UPDATE agents agent
SET draft_revision_id = COALESCE(agent.draft_revision_id, revision.id),
    published_revision_id = COALESCE(agent.published_revision_id, revision.id),
    next_revision_no = GREATEST(agent.next_revision_no, 2)
FROM seed_agent_definitions definition
CROSS JOIN seed_agent_context context
JOIN agent_revisions revision
  ON revision.tenant_id = context.tenant_id
 AND revision.agent_id = 'adef-' || context.tenant_id || '-' || definition.suffix
 AND revision.id = 'arev-' || context.tenant_id || '-' || definition.suffix || '-v1'
 AND revision.revision_no = 1
WHERE agent.tenant_id = context.tenant_id
  AND agent.id = revision.agent_id;

UPDATE agents child
SET draft_revision_id = child_revision.id,
    published_revision_id = child_revision.id,
    next_revision_no = GREATEST(child.next_revision_no, 2),
    updated_at = context.seeded_at
FROM seed_agent_context context
JOIN agents parent
  ON parent.tenant_id = context.tenant_id
 AND parent.parent_agent_id IS NULL
JOIN agent_revisions parent_revision
  ON parent_revision.tenant_id = parent.tenant_id
 AND parent_revision.id = parent.published_revision_id
JOIN agent_revisions child_revision
  ON child_revision.tenant_id = parent_revision.tenant_id
 AND child_revision.revision_no = parent_revision.revision_no
WHERE child.tenant_id = context.tenant_id
  AND child.parent_agent_id = parent.id
  AND child_revision.agent_id = child.id;

CREATE TEMP TABLE seeded_agent_ids ON COMMIT DROP AS
SELECT agent.id, agent.tenant_id, revision.name
FROM agents agent
JOIN agent_revisions revision
  ON revision.tenant_id = agent.tenant_id
 AND revision.agent_id = agent.id
 AND revision.id = agent.published_revision_id
JOIN seed_agent_context context ON context.tenant_id = agent.tenant_id
WHERE agent.id IN (
    'adef-' || context.tenant_id || '-assistant',
    'adef-' || context.tenant_id || '-leave',
    'adef-' || context.tenant_id || '-punch-correction'
);

INSERT INTO audit_logs (
    id,
    tenant_id,
    actor_account_id,
    action,
    resource,
    target,
    result,
    trace_id,
    severity,
    details,
    created_at
)
SELECT
    'aud-seed-' || seeded.id,
    seeded.tenant_id,
    context.actor_id,
    'ai.agent.agent.create',
    'agent_agent',
    seeded.id,
    'success',
    'seed:demo-assistants',
    'high',
    jsonb_build_object(
        'entity_type', 'agent',
        'entity_id', seeded.id,
        'entity_name', seeded.name,
        'actor_display_name', COALESCE(context.actor_name, 'demo seed'),
        'detail', 'demo assistant seeded and published'
    ),
    context.seeded_at
FROM seeded_agent_ids seeded
JOIN seed_agent_context context ON context.tenant_id = seeded.tenant_id
ON CONFLICT (id) DO NOTHING;

COMMIT;

SELECT
    agent.id,
    revision.name,
    CASE WHEN agent.published_revision_id = revision.id THEN 'published' ELSE 'draft' END AS status,
    revision.visibility,
    revision.revision_no AS version,
    published.revision_no AS published_version
FROM agents agent
JOIN agent_revisions revision
  ON revision.tenant_id = agent.tenant_id
 AND revision.agent_id = agent.id
 AND revision.id = COALESCE(agent.draft_revision_id, agent.published_revision_id)
LEFT JOIN agent_revisions published
  ON published.tenant_id = agent.tenant_id
 AND published.agent_id = agent.id
 AND published.id = agent.published_revision_id
WHERE agent.tenant_id = :'tenant_id'
  AND agent.id IN (
      'adef-' || :'tenant_id' || '-assistant',
      'adef-' || :'tenant_id' || '-leave',
      'adef-' || :'tenant_id' || '-punch-correction'
  )
ORDER BY agent.updated_at DESC, agent.id;
