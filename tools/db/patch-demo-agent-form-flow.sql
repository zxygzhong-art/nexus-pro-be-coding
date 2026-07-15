\if :{?tenant_id}
\else
\set tenant_id demo
\endif

\if :{?account_email}
\else
\set account_email zxy1@gmail.com
\endif

-- Complete the demo-only Agent -> leave form -> manager review prerequisites.
BEGIN;

SELECT set_config('app.tenant_id', :'tenant_id', true);

CREATE TEMP TABLE demo_agent_flow_context ON COMMIT DROP AS
SELECT
    target.tenant_id,
    target.id AS account_id,
    target.employee_id,
    target.email,
    (
        SELECT employee.id
        FROM accounts manager_account
        JOIN permission_sets permission_set
          ON permission_set.tenant_id = manager_account.tenant_id
         AND permission_set.id = ANY(manager_account.direct_permission_set_ids)
         AND permission_set.name = 'Platform Admin'
        JOIN employees employee
          ON employee.tenant_id = manager_account.tenant_id
         AND (employee.account_id = manager_account.id OR employee.id = manager_account.employee_id)
        WHERE manager_account.tenant_id = target.tenant_id
          AND manager_account.id <> target.id
          AND manager_account.status = 'active'
          AND employee.status = 'active'
        ORDER BY manager_account.created_at, manager_account.id
        LIMIT 1
    ) AS manager_employee_id,
    (
        SELECT model.id
        FROM agent_models model
        WHERE model.tenant_id = target.tenant_id
          AND model.status = 'active'
          AND model.sync_status = 'synced'
        ORDER BY model.updated_at DESC, model.id
        LIMIT 1
    ) AS model_id,
    clock_timestamp() AS patched_at
FROM accounts target
WHERE target.tenant_id = :'tenant_id'
  AND lower(target.email) = lower(:'account_email')
  AND target.status = 'active';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM demo_agent_flow_context) THEN
        RAISE EXCEPTION 'active demo account was not found';
    END IF;
    IF EXISTS (SELECT 1 FROM demo_agent_flow_context WHERE employee_id IS NULL OR employee_id = '') THEN
        RAISE EXCEPTION 'demo account is not linked to an employee';
    END IF;
    IF EXISTS (SELECT 1 FROM demo_agent_flow_context WHERE manager_employee_id IS NULL) THEN
        RAISE EXCEPTION 'demo tenant requires another active Platform Admin employee as manager';
    END IF;
    IF EXISTS (SELECT 1 FROM demo_agent_flow_context WHERE model_id IS NULL) THEN
        RAISE EXCEPTION 'demo tenant requires an active synced Agent model';
    END IF;
END
$$;

-- Demo identities need a stable tenure date and a resolvable manager stage.
UPDATE employees employee
SET manager_employee_id = context.manager_employee_id,
    hire_date = COALESCE(employee.hire_date, make_date(EXTRACT(YEAR FROM current_date)::int - 3, 1, 1)),
    updated_at = context.patched_at
FROM demo_agent_flow_context context
WHERE employee.tenant_id = context.tenant_id
  AND employee.id = context.employee_id;

-- Initialize the annual-grant leave types from the built-in fallback policy without erasing used hours.
INSERT INTO leave_balances (
    id, tenant_id, employee_id, leave_type, remaining_hours,
    period_start, period_end, granted_hours, used_hours, source, policy_version, prorate_ratio, updated_at
)
SELECT
    'lb-' || context.employee_id || '-' || entitlement.leave_type,
    context.tenant_id,
    context.employee_id,
    entitlement.leave_type,
    entitlement.granted_hours,
    make_date(EXTRACT(YEAR FROM current_date)::int, 1, 1)::text,
    make_date(EXTRACT(YEAR FROM current_date)::int, 12, 31)::text,
    entitlement.granted_hours,
    0,
    'demo_policy_grant',
    COALESCE((SELECT policy.version FROM attendance_policies policy WHERE policy.tenant_id = context.tenant_id), 1),
    1,
    context.patched_at
FROM demo_agent_flow_context context
CROSS JOIN (VALUES
    ('sick_full', 240::double precision),
    ('flexible', 0::double precision),
    ('personal', 112::double precision),
    ('family_care', 56::double precision),
    ('sick_half', 240::double precision),
    ('annual', 112::double precision)
) AS entitlement(leave_type, granted_hours)
ON CONFLICT (tenant_id, employee_id, leave_type) DO UPDATE SET
    remaining_hours = GREATEST(EXCLUDED.granted_hours - leave_balances.used_hours, 0),
    period_start = EXCLUDED.period_start,
    period_end = EXCLUDED.period_end,
    granted_hours = EXCLUDED.granted_hours,
    source = EXCLUDED.source,
    policy_version = EXCLUDED.policy_version,
    prorate_ratio = EXCLUDED.prorate_ratio,
    updated_at = EXCLUDED.updated_at;

CREATE TEMP TABLE updated_demo_assistant ON COMMIT DROP AS
WITH desired AS (
    SELECT
        '理解員工的 OA 與人資需求；請假、加班、補卡與出勤查詢委派給考勤助理，人事異動、增補與離職申請委派給人事助理，並在取得結果後繼續完成目前對話。'::text AS main_agent_role,
        '你是 Nexus Pro 助理。先理解使用者目標，再透過可用工具查詢個人資料、待辦與已發布表單；遇到請假、加班、補卡或考勤查詢時，必須呼叫考勤助理；遇到人事異動、人員增補、離職或退休申請時，必須呼叫人事助理。不要只推薦、不要要求使用者手動切換，也不要聲稱已完成未確認的操作。'::text AS system_prompt,
        jsonb_build_array(
            jsonb_build_object(
                'id', 'leave',
                'name', '考勤助理',
                'role', '處理請假、加班與補卡申請，以及假期餘額、歷史請假、打卡紀錄與本月考勤摘要。建立請假前呼叫 check_leave_eligibility；所有申請只建立草稿並呼叫 preview_form_submission，等待使用者確認。',
                'model_id', context.model_id,
                'tools', '["get_my_profile","my_attendance_summary","my_form_history","my_leave_balances","check_leave_eligibility","my_clock_records","list_employees","list_published_form_templates","get_published_form_template","create_form_draft","update_form_draft","preview_form_submission"]'::jsonb,
                'knowledge_base_ids', '[]'::jsonb
            ),
            jsonb_build_object(
                'id', 'punch-correction',
                'name', '人事助理',
                'role', '處理人事異動、人員增補、離職與退休等人事申請。依已發布表單與資料來源收集欄位，只建立草稿並呼叫 preview_form_submission，等待使用者確認。',
                'model_id', context.model_id,
                'tools', '["get_my_profile","list_employees","get_employee","my_form_history","list_published_form_templates","get_published_form_template","create_form_draft","update_form_draft","preview_form_submission"]'::jsonb,
                'knowledge_base_ids', '[]'::jsonb
            )
        ) AS sub_agents
    FROM demo_agent_flow_context context
), updated AS (
    UPDATE agent_definitions definition
    SET main_agent_role = desired.main_agent_role,
        sub_agents = desired.sub_agents,
        system_prompt = desired.system_prompt,
        version = definition.version + 1,
        published_version = definition.version + 1,
        updated_at = clock_timestamp()
    FROM desired
    WHERE definition.tenant_id = :'tenant_id'
      AND definition.id = 'adef-' || :'tenant_id' || '-assistant'
      AND (
          definition.main_agent_role IS DISTINCT FROM desired.main_agent_role
          OR definition.sub_agents IS DISTINCT FROM desired.sub_agents
          OR definition.system_prompt IS DISTINCT FROM desired.system_prompt
      )
    RETURNING definition.*
)
SELECT * FROM updated;

INSERT INTO agent_definition_versions (
    id, tenant_id, agent_id, version, main_agent_role, sub_agents, system_prompt,
    tools, knowledge_base_ids, model_id, note, created_by_account_id, created_at
)
SELECT
    'adefv-' || tenant_id || '-assistant-form-flow-v' || version,
    tenant_id,
    id,
    version,
    main_agent_role,
    sub_agents,
    system_prompt,
    tools,
    knowledge_base_ids,
    model_id,
    'delegate leave and punch tasks to executable sub-agents',
    updated_by_account_id,
    updated_at
FROM updated_demo_assistant
ON CONFLICT (tenant_id, agent_id, version) DO NOTHING;

SELECT
    context.email,
    employee.id AS employee_id,
    employee.manager_employee_id,
    employee.hire_date,
    (SELECT count(*) FROM leave_balances balance WHERE balance.tenant_id = context.tenant_id AND balance.employee_id = context.employee_id) AS leave_balance_count,
    assistant.version AS assistant_version,
    jsonb_array_length(assistant.sub_agents) AS assistant_sub_agent_count
FROM demo_agent_flow_context context
JOIN employees employee ON employee.tenant_id = context.tenant_id AND employee.id = context.employee_id
JOIN agent_definitions assistant ON assistant.tenant_id = context.tenant_id AND assistant.id = 'adef-' || context.tenant_id || '-assistant';

COMMIT;
