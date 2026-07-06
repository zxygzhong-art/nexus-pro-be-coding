-- QA fixes: traditional Chinese labels, dedupe templates, shift assignment.
--   psql "$DATABASE_URL" -f tools/db/patch-demo-qa-fixes.sql

BEGIN;

UPDATE org_units SET name = '總部' WHERE tenant_id = 'demo' AND id = 'ou-hq';
UPDATE org_units SET name = '營運中心' WHERE tenant_id = 'demo' AND id = 'ou-ops';
UPDATE org_units SET name = '人力資源部' WHERE tenant_id = 'demo' AND id = 'ou-hr';
UPDATE org_units SET name = '財務中心' WHERE tenant_id = 'demo' AND id = 'ou-finance';
UPDATE org_units SET name = '銷售中心' WHERE tenant_id = 'demo' AND id = 'ou-sales';

UPDATE form_templates
SET name = '請假申請單', description = '請假與審批流程模板'
WHERE tenant_id = 'demo' AND key = 'leave-request';

UPDATE form_templates
SET name = 'HR-005 補卡單', description = '漏打卡或打卡異常補登'
WHERE tenant_id = 'demo' AND key = 'punch-fix';

UPDATE form_templates
SET name = '在職證明', description = '在職證明模板'
WHERE tenant_id = 'demo' AND key = 'employment-certificate';

DELETE FROM form_templates
WHERE tenant_id = 'demo'
  AND name IN ('补卡申请', '请假申请', '在职证明');

INSERT INTO attendance_shift_assignments (
  id, tenant_id, employee_id, shift_id, worksite_id, effective_from, effective_to, status, created_at, updated_at
)
SELECT
  'asa-emp-zxy1',
  'demo',
  'emp-zxy1',
  'ash-day',
  'aws-demo-hq',
  now() - interval '30 days',
  NULL,
  'active',
  now(),
  now()
WHERE EXISTS (SELECT 1 FROM employees WHERE tenant_id = 'demo' AND id = 'emp-zxy1')
  AND NOT EXISTS (
    SELECT 1 FROM attendance_shift_assignments
    WHERE tenant_id = 'demo' AND employee_id = 'emp-zxy1' AND status = 'active'
  );

UPDATE employees
SET employment_info = COALESCE(employment_info, '{}'::jsonb) || jsonb_build_object(
  'position', COALESCE(NULLIF(position, ''), '測試帳號'),
  'org_unit_id', COALESCE(NULLIF(org_unit_id, ''), 'ou-hq')
),
position = COALESCE(NULLIF(position, ''), '測試帳號'),
org_unit_id = COALESCE(NULLIF(org_unit_id, ''), 'ou-hq'),
updated_at = now()
WHERE tenant_id = 'demo' AND id = 'emp-zxy1';

COMMIT;
