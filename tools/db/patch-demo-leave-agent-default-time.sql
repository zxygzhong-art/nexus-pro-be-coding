\if :{?tenant_id}
\else
\set tenant_id demo
\endif

BEGIN;

SELECT set_config('app.tenant_id', :'tenant_id', true);

CREATE TEMP TABLE updated_leave_agent ON COMMIT DROP AS
	WITH desired AS (
	    SELECT
		        '你是 Nexus Pro 考勤助理，只處理員工考勤自助。你可以建立 leave-request 請假單、overtime-approval 加班單與 punch-fix 補卡單，並查詢假期餘額、本人表單歷史、打卡紀錄與本月考勤摘要。查詢歷史請假時必須呼叫 my_form_history 並指定 template_key=leave-request；查詢本月工時、出勤天數、已覈準請假或加班時數時必須呼叫 my_attendance_summary。建立任何單據前先呼叫 get_published_form_template 取得真實欄位與資料來源，不得猜測欄位 ID。建立請假草稿前必須呼叫 check_leave_eligibility；若餘額未初始化或不足，仍必須按不使用餘額繼續建立草稿，不得只回覆說明文字；my_leave_balances 的 initialized=false 表示餘額尚未初始化，不是餘額為零。若請假未提供開始與結束時間，不要追問，後端會依 Asia/Shanghai 當天與考勤政策自動填入，建立草稿後說明實際採用的日期、時間與時數。建立補卡前先查 my_clock_records 核對缺口。只能建立或更新可撤銷草稿，完成必填欄位後必須呼叫 preview_form_submission，等待使用者在確認卡上確認，絕不能聲稱已自動提交。'::text AS system_prompt,
	        '["get_my_profile","my_attendance_summary","my_form_history","my_leave_balances","check_leave_eligibility","my_clock_records","list_employees","list_published_form_templates","get_published_form_template","create_form_draft","update_form_draft","preview_form_submission"]'::jsonb AS tools
	), updated AS (
	    UPDATE agent_definitions definition
	    SET system_prompt = desired.system_prompt,
	        tools = desired.tools,
	        version = definition.version + 1,
        published_version = definition.version + 1,
        updated_at = clock_timestamp()
    FROM desired
    WHERE definition.tenant_id = :'tenant_id'
      AND definition.id = 'adef-' || :'tenant_id' || '-leave'
	      AND (
	          definition.system_prompt IS DISTINCT FROM desired.system_prompt
	          OR definition.tools IS DISTINCT FROM desired.tools
	      )
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
	    'default missing leave times and deterministic leave eligibility',
    updated_by_account_id,
    updated_at
FROM updated_leave_agent
ON CONFLICT (tenant_id, agent_id, version) DO NOTHING;

COMMIT;

SELECT id, name, status, version, published_version, system_prompt
FROM agent_definitions
WHERE tenant_id = :'tenant_id'
  AND id = 'adef-' || :'tenant_id' || '-leave';
