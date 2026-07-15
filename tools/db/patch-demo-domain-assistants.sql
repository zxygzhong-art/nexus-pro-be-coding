\if :{?tenant_id}
\else
\set tenant_id demo
\endif

-- Reconfigure the existing domain Agent IDs without breaking sessions or deep links.
BEGIN;

SELECT set_config('app.tenant_id', :'tenant_id', true);

CREATE TEMP TABLE domain_agent_context ON COMMIT DROP AS
SELECT
    tenant.id AS tenant_id,
    (
        SELECT account.id
        FROM accounts account
        JOIN permission_sets permission_set
          ON permission_set.tenant_id = account.tenant_id
         AND permission_set.id = ANY(account.direct_permission_set_ids)
         AND permission_set.name = 'Platform Admin'
        WHERE account.tenant_id = tenant.id
          AND account.status = 'active'
        ORDER BY account.created_at, account.id
        LIMIT 1
    ) AS actor_id,
    clock_timestamp() AS patched_at
FROM tenants tenant
WHERE tenant.id = :'tenant_id';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM domain_agent_context) THEN
        RAISE EXCEPTION 'tenant does not exist';
    END IF;
    IF EXISTS (SELECT 1 FROM domain_agent_context WHERE actor_id IS NULL) THEN
        RAISE EXCEPTION 'tenant requires an active Platform Admin account';
    END IF;
END
$$;

CREATE TEMP TABLE updated_domain_assistants ON COMMIT DROP AS
WITH desired(
    suffix,
    name,
    description,
    emoji,
    main_agent_role,
    system_prompt,
    welcome_message,
    suggested_questions,
    tools
) AS (
    VALUES
        (
            'leave',
            '考勤助理',
            '處理請假、加班、補卡，以及假期餘額、歷史請假與本月考勤摘要。',
            '🕒',
            '專責員工考勤自助：建立請假、加班與補卡草稿，查詢假期餘額、歷史申請及本月出勤摘要；提交前必須取得使用者確認。',
            '你是 Nexus Pro 考勤助理，只處理員工考勤自助。你可以建立 leave-request 請假單、overtime-approval 加班單與 punch-fix 補卡單，並查詢假期餘額、本人表單歷史、打卡紀錄與本月考勤摘要。查詢歷史請假時必須呼叫 my_form_history 並指定 template_key=leave-request；查詢本月工時、出勤天數、已核准請假或加班時數時必須呼叫 my_attendance_summary。建立任何單據前先呼叫 get_published_form_template 取得真實欄位與資料來源，不得猜測欄位 ID。建立請假草稿前必須呼叫 check_leave_eligibility；my_leave_balances 的 initialized=false 表示餘額尚未初始化，不是餘額為零。若請假未提供開始與結束時間，不要追問，後端會依 Asia/Shanghai 當天與考勤政策自動填入，建立草稿後說明實際採用的日期、時間與時數。建立補卡前先查 my_clock_records 核對缺口。只能建立或更新可撤銷草稿，完成必填欄位後必須呼叫 preview_form_submission，等待使用者在確認卡上確認，絕不能聲稱已自動提交。',
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
            '你是 Nexus Pro 人事助理，只處理人事流程申請。你可以建立 job-change 人事變動單、headcount-request 人員增補申請單與 resignation 離職及退休申請單。建立單據前必須呼叫 get_published_form_template 取得真實欄位、選項、資料來源與審批路徑；涉及員工時使用 list_employees 或 get_employee 核對真實對象，不得猜測員工、部門、職務或薪資資料，不得把未取得的敏感資料補成預設值。只能建立或更新可撤銷草稿，完成必填欄位後必須呼叫 preview_form_submission，等待使用者在確認卡上確認，絕不能聲稱已自動提交。',
            '您好，我是人事助理，可以協助人事異動、人員增補與離職申請。',
            '["幫我建立人事變動單","我要申請人員增補","幫我準備離職申請單"]'::jsonb,
            '["get_my_profile","list_employees","get_employee","my_form_history","list_published_form_templates","get_published_form_template","create_form_draft","update_form_draft","preview_form_submission"]'::jsonb
        )
), updated AS (
    UPDATE agent_definitions definition
    SET name = desired.name,
        description = desired.description,
        emoji = desired.emoji,
        main_agent_role = desired.main_agent_role,
        system_prompt = desired.system_prompt,
        welcome_message = desired.welcome_message,
        suggested_questions = desired.suggested_questions,
        tools = desired.tools,
        status = 'published',
        visibility = 'all',
        version = definition.version + 1,
        published_version = definition.version + 1,
        updated_by_account_id = context.actor_id,
        updated_at = context.patched_at
    FROM desired
    CROSS JOIN domain_agent_context context
    WHERE definition.tenant_id = context.tenant_id
      AND definition.id = 'adef-' || context.tenant_id || '-' || desired.suffix
      AND (
          definition.name IS DISTINCT FROM desired.name
          OR definition.description IS DISTINCT FROM desired.description
          OR definition.emoji IS DISTINCT FROM desired.emoji
          OR definition.main_agent_role IS DISTINCT FROM desired.main_agent_role
          OR definition.system_prompt IS DISTINCT FROM desired.system_prompt
          OR definition.welcome_message IS DISTINCT FROM desired.welcome_message
          OR definition.suggested_questions IS DISTINCT FROM desired.suggested_questions
          OR definition.tools IS DISTINCT FROM desired.tools
          OR definition.status IS DISTINCT FROM 'published'
          OR definition.visibility IS DISTINCT FROM 'all'
      )
    RETURNING definition.*
)
SELECT * FROM updated;

CREATE TEMP TABLE updated_main_assistant ON COMMIT DROP AS
WITH desired AS (
    SELECT
        '理解員工的 OA 與人資需求；請假、加班、補卡與出勤查詢委派給考勤助理，人事異動、增補與離職申請委派給人事助理，並在取得結果後繼續完成目前對話。'::text AS main_agent_role,
        '你是 Nexus Pro 助理。先理解使用者目標，再透過可用工具查詢個人資料、待辦與已發布表單；遇到請假、加班、補卡或考勤查詢時，必須呼叫考勤助理；遇到人事異動、人員增補、離職或退休申請時，必須呼叫人事助理。不要只推薦、不要要求使用者手動切換，也不要聲稱已完成未確認的操作。'::text AS system_prompt,
        '["查看我的待辦","我要處理考勤","我要發起人事申請"]'::jsonb AS suggested_questions
), updated AS (
    UPDATE agent_definitions definition
    SET description = '協助員工釐清需求、查找可用表單與待辦，並委派給考勤或人事專用助理。',
        main_agent_role = desired.main_agent_role,
        sub_agents = jsonb_build_array(
            jsonb_build_object(
                'id', 'leave',
                'name', '考勤助理',
                'role', '處理請假、加班與補卡申請，以及假期餘額、歷史請假、打卡紀錄與本月考勤摘要。建立請假前呼叫 check_leave_eligibility；所有申請只建立草稿並呼叫 preview_form_submission，等待使用者確認。',
                'model_id', definition.model_id,
                'tools', '["get_my_profile","my_attendance_summary","my_form_history","my_leave_balances","check_leave_eligibility","my_clock_records","list_employees","list_published_form_templates","get_published_form_template","create_form_draft","update_form_draft","preview_form_submission"]'::jsonb,
                'knowledge_base_ids', '[]'::jsonb
            ),
            jsonb_build_object(
                'id', 'punch-correction',
                'name', '人事助理',
                'role', '處理人事異動、人員增補、離職與退休等人事申請。依已發布表單與資料來源收集欄位，只建立草稿並呼叫 preview_form_submission，等待使用者確認。',
                'model_id', definition.model_id,
                'tools', '["get_my_profile","list_employees","get_employee","my_form_history","list_published_form_templates","get_published_form_template","create_form_draft","update_form_draft","preview_form_submission"]'::jsonb,
                'knowledge_base_ids', '[]'::jsonb
            )
        ),
        system_prompt = desired.system_prompt,
        suggested_questions = desired.suggested_questions,
        version = definition.version + 1,
        published_version = definition.version + 1,
        updated_by_account_id = context.actor_id,
        updated_at = context.patched_at
    FROM desired
    CROSS JOIN domain_agent_context context
    WHERE definition.tenant_id = context.tenant_id
      AND definition.id = 'adef-' || context.tenant_id || '-assistant'
      AND (
          definition.description IS DISTINCT FROM '協助員工釐清需求、查找可用表單與待辦，並委派給考勤或人事專用助理。'
          OR definition.main_agent_role IS DISTINCT FROM desired.main_agent_role
          OR definition.system_prompt IS DISTINCT FROM desired.system_prompt
          OR definition.suggested_questions IS DISTINCT FROM desired.suggested_questions
          OR definition.sub_agents -> 0 ->> 'name' IS DISTINCT FROM '考勤助理'
          OR definition.sub_agents -> 1 ->> 'name' IS DISTINCT FROM '人事助理'
      )
    RETURNING definition.*
)
SELECT * FROM updated;

INSERT INTO agent_definition_versions (
    id, tenant_id, agent_id, version, main_agent_role, sub_agents, system_prompt,
    welcome_message, suggested_questions, tools, knowledge_base_ids, model_id,
    note, created_by_account_id, created_at
)
SELECT
    'adefv-' || definition.tenant_id || '-domain-assistants-' || definition.id || '-v' || definition.version,
    definition.tenant_id,
    definition.id,
    definition.version,
    definition.main_agent_role,
    definition.sub_agents,
    definition.system_prompt,
    definition.welcome_message,
    definition.suggested_questions,
    definition.tools,
    definition.knowledge_base_ids,
    definition.model_id,
    'consolidate attendance and HR domain assistants',
    definition.updated_by_account_id,
    definition.updated_at
FROM (
    SELECT * FROM updated_domain_assistants
    UNION ALL
    SELECT * FROM updated_main_assistant
) definition
ON CONFLICT (tenant_id, agent_id, version) DO NOTHING;

COMMIT;

SELECT id, name, status, version, published_version, tools
FROM agent_definitions
WHERE tenant_id = :'tenant_id'
  AND id IN (
      'adef-' || :'tenant_id' || '-assistant',
      'adef-' || :'tenant_id' || '-leave',
      'adef-' || :'tenant_id' || '-punch-correction'
  )
ORDER BY id;
