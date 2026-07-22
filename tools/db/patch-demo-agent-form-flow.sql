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
        FROM model_connections model
        JOIN model_connection_state state
          ON state.tenant_id = model.tenant_id
         AND state.model_connection_id = model.id
        WHERE model.tenant_id = target.tenant_id
          AND model.status = 'active'
          AND state.sync_status = 'synced'
        ORDER BY model.updated_at DESC, model.id
        LIMIT 1
    ) AS model_connection_id,
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
    IF EXISTS (SELECT 1 FROM demo_agent_flow_context WHERE model_connection_id IS NULL) THEN
        RAISE EXCEPTION 'demo tenant requires an active synced Agent model';
    END IF;
    IF NOT EXISTS (
        SELECT 1
        FROM demo_agent_flow_context context
        JOIN agents assistant
          ON assistant.tenant_id = context.tenant_id
         AND assistant.id = 'adef-' || context.tenant_id || '-assistant'
        JOIN agent_revisions revision
          ON revision.tenant_id = assistant.tenant_id
         AND revision.agent_id = assistant.id
         AND revision.id = assistant.published_revision_id
    ) THEN
        RAISE EXCEPTION 'published demo assistant is missing; run tools/db/seed-demo-assistants.sql first';
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
    id, tenant_id, employee_id, leave_type_id, remaining_hours,
    period_start, period_end, granted_hours, used_hours, source, updated_at
)
SELECT
    'lb-' || context.employee_id || '-' || entitlement.leave_type,
    context.tenant_id,
    context.employee_id,
    'lt_' || entitlement.leave_type,
    entitlement.granted_hours,
    make_date(EXTRACT(YEAR FROM current_date)::int, 1, 1)::text,
    make_date(EXTRACT(YEAR FROM current_date)::int, 12, 31)::text,
    entitlement.granted_hours,
    0,
    'demo_policy_grant',
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
ON CONFLICT (tenant_id, employee_id, leave_type_id, period_start, period_end) DO UPDATE SET
    remaining_hours = GREATEST(EXCLUDED.granted_hours - leave_balances.used_hours, 0),
    period_start = EXCLUDED.period_start,
    period_end = EXCLUDED.period_end,
    granted_hours = EXCLUDED.granted_hours,
    source = EXCLUDED.source,
    updated_at = EXCLUDED.updated_at;

-- Published Agent v2 revisions are immutable. The canonical demo assistant
-- configuration and its member/tool bindings are owned by seed-demo-assistants.sql.

SELECT
    context.email,
    employee.id AS employee_id,
    employee.manager_employee_id,
    employee.hire_date,
    (SELECT count(*) FROM leave_balances balance WHERE balance.tenant_id = context.tenant_id AND balance.employee_id = context.employee_id) AS leave_balance_count,
    assistant_revision.revision_no AS assistant_version,
    (SELECT count(*)
     FROM agent_revision_members member
     WHERE member.tenant_id = assistant_revision.tenant_id
       AND member.revision_id = assistant_revision.id) AS assistant_sub_agent_count
FROM demo_agent_flow_context context
JOIN employees employee ON employee.tenant_id = context.tenant_id AND employee.id = context.employee_id
JOIN agents assistant ON assistant.tenant_id = context.tenant_id AND assistant.id = 'adef-' || context.tenant_id || '-assistant'
JOIN agent_revisions assistant_revision
  ON assistant_revision.tenant_id = assistant.tenant_id
 AND assistant_revision.agent_id = assistant.id
 AND assistant_revision.id = assistant.published_revision_id;

COMMIT;
