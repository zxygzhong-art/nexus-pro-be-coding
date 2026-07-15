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
		            '你是 Nexus Pro 考勤助理，只處理員工考勤自助。你可以建立 leave-request 請假單、overtime-approval 加班單與 punch-fix 補卡單，並查詢假期餘額、本人表單歷史、打卡紀錄與本月考勤摘要。查詢歷史請假時必須呼叫 my_form_history 並指定 template_key=leave-request；查詢本月工時、出勤天數、已核准請假或加班時數時必須呼叫 my_attendance_summary。建立任何單據前先呼叫 get_published_form_template 取得真實欄位與資料來源，不得猜測欄位 ID。建立請假草稿前必須呼叫 check_leave_eligibility；若餘額未初始化或不足，仍必須按不使用餘額繼續建立草稿，不得只回覆說明文字；my_leave_balances 的 initialized=false 表示餘額尚未初始化，不是餘額為零。若請假未提供開始與結束時間，不要追問，後端會依 Asia/Shanghai 當天與考勤政策自動填入，建立草稿後說明實際採用的日期、時間與時數。建立補卡前先查 my_clock_records 核對缺口。只能建立或更新可撤銷草稿，完成必填欄位後必須呼叫 preview_form_submission，等待使用者在確認卡上確認，絕不能聲稱已自動提交。',
	            '您好，我是考勤助理，可以協助請假、加班、補卡與考勤查詢。',
	            '["幫我申請下週一特休","幫我建立加班單","查看本月考勤摘要"]'::jsonb,
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
        welcome_message,
        suggested_questions,
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
	        COALESCE((
	            SELECT jsonb_agg(member || jsonb_build_object('model_id', context.model_id))
	            FROM jsonb_array_elements(definitions.sub_agents) AS member
        ), '[]'::jsonb),
        definitions.system_prompt,
        definitions.welcome_message,
        definitions.suggested_questions,
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
    ON CONFLICT (id) DO UPDATE SET
        welcome_message = CASE
            WHEN agent_definitions.welcome_message = '' THEN EXCLUDED.welcome_message
            ELSE agent_definitions.welcome_message
        END,
        suggested_questions = CASE
            WHEN agent_definitions.suggested_questions = '[]'::jsonb THEN EXCLUDED.suggested_questions
            ELSE agent_definitions.suggested_questions
        END
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
    welcome_message,
    suggested_questions,
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
    definition.welcome_message,
    definition.suggested_questions,
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
ON CONFLICT (tenant_id, agent_id, version) DO UPDATE SET
    welcome_message = CASE
        WHEN agent_definition_versions.welcome_message = '' THEN EXCLUDED.welcome_message
        ELSE agent_definition_versions.welcome_message
    END,
    suggested_questions = CASE
        WHEN agent_definition_versions.suggested_questions = '[]'::jsonb THEN EXCLUDED.suggested_questions
        ELSE agent_definition_versions.suggested_questions
    END;

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

SELECT id, name, status, visibility, version, published_version
FROM agent_definitions
WHERE tenant_id = :'tenant_id'
ORDER BY updated_at DESC, id;
