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

INSERT INTO attendance_worksites (
  id, tenant_id, name, address, latitude, longitude, radius_meters, status, created_at, updated_at
)
VALUES (
  'aws-demo-hq',
  'demo',
  'Demo HQ',
  'Demo office',
  25.03408,
  121.56442,
  300,
  'active',
  now(),
  now()
)
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  address = EXCLUDED.address,
  latitude = EXCLUDED.latitude,
  longitude = EXCLUDED.longitude,
  radius_meters = EXCLUDED.radius_meters,
  status = EXCLUDED.status,
  updated_at = now();

INSERT INTO attendance_shifts (
  id, tenant_id, name, clock_in_start, clock_in_end, clock_out_start, clock_out_end,
  late_grace_minutes, early_leave_grace_minutes, status, created_at, updated_at
)
VALUES (
  'ash-day',
  'demo',
  'Demo 日班',
  '07:00',
  '11:00',
  '16:00',
  '22:00',
  10,
  10,
  'active',
  now(),
  now()
)
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  clock_in_start = EXCLUDED.clock_in_start,
  clock_in_end = EXCLUDED.clock_in_end,
  clock_out_start = EXCLUDED.clock_out_start,
  clock_out_end = EXCLUDED.clock_out_end,
  late_grace_minutes = EXCLUDED.late_grace_minutes,
  early_leave_grace_minutes = EXCLUDED.early_leave_grace_minutes,
  status = EXCLUDED.status,
  updated_at = now();

INSERT INTO attendance_shift_assignments (
  id, tenant_id, employee_id, shift_id, worksite_id, effective_from, effective_to, status, created_at, updated_at
)
SELECT
  'asa-' || demo_clock_employees.id,
  'demo',
  demo_clock_employees.id,
  'ash-day',
  'aws-demo-hq',
  now() - interval '30 days',
  NULL,
  'active',
  now(),
  now()
FROM (
  SELECT e.id
  FROM employees e
  LEFT JOIN accounts a ON a.tenant_id = e.tenant_id AND a.id = e.account_id
  WHERE e.tenant_id = 'demo'
    AND (
      e.id = 'emp-zxy1'
      OR e.company_email IN ('zxy@gmail.com', 'zxy1@a.com')
      OR a.email IN ('zxy@gmail.com', 'zxy1@a.com')
    )
) AS demo_clock_employees
WHERE NOT EXISTS (
    SELECT 1 FROM attendance_shift_assignments
    WHERE tenant_id = 'demo' AND employee_id = demo_clock_employees.id AND status = 'active'
  );

UPDATE employees
SET employment_info = COALESCE(employment_info, '{}'::jsonb) || jsonb_build_object(
  'position', COALESCE(NULLIF(position, ''), '測試帳號'),
  'org_unit_id', COALESCE(NULLIF(org_unit_id, ''), 'ou-hq')
),
position = COALESCE(NULLIF(position, ''), '測試帳號'),
org_unit_id = COALESCE(NULLIF(org_unit_id, ''), 'ou-hq'),
updated_at = now()
WHERE tenant_id = 'demo'
  AND (
    id = 'emp-zxy1'
    OR company_email IN ('zxy@gmail.com', 'zxy1@a.com')
  );

COMMIT;
