-- QA fixes: traditional Chinese labels, dedupe templates.
--   psql "postgres://$DB_USERNAME:$DB_PASSWORD@$DB_HOST:$DB_PORT/$DB_NAME?sslmode=${DB_SSLMODE:-disable}" -f tools/db/patch-demo-qa-fixes.sql

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
  AND name IN ('補卡申請', '請假申請', '在職證明');

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
