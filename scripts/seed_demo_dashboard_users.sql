-- Seed enough deterministic demo users to support dashboard and permission testing.
-- This script is idempotent for the acct-demo-bulk-* / emp-demo-bulk-* cohort and does not delete data.

BEGIN;

INSERT INTO tenants (id, name, created_at)
VALUES ('demo', 'Demo Tenant', TIMESTAMPTZ '2026-06-10 08:00:00+00')
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name;

INSERT INTO org_units (id, tenant_id, code, name, parent_id, path, created_at)
VALUES
  ('ou-hq', 'demo', 'HQ', '总部', '', ARRAY['ou-hq'], TIMESTAMPTZ '2026-06-10 08:00:00+00'),
  ('ou-ops', 'demo', 'OPS', '运营中心', 'ou-hq', ARRAY['ou-hq', 'ou-ops'], TIMESTAMPTZ '2026-06-10 08:01:00+00'),
  ('ou-hr', 'demo', 'HR', '人力资源部', 'ou-hq', ARRAY['ou-hq', 'ou-hr'], TIMESTAMPTZ '2026-06-10 08:02:00+00'),
  ('ou-finance', 'demo', 'FIN', '财务中心', 'ou-hq', ARRAY['ou-hq', 'ou-finance'], TIMESTAMPTZ '2026-06-10 08:03:00+00'),
  ('ou-sales', 'demo', 'SALES', '销售中心', 'ou-hq', ARRAY['ou-hq', 'ou-sales'], TIMESTAMPTZ '2026-06-10 08:04:00+00'),
  ('ou-security', 'demo', 'SEC', '安全合规部', 'ou-hq', ARRAY['ou-hq', 'ou-security'], TIMESTAMPTZ '2026-06-10 08:05:00+00'),
  ('ou-rd', 'demo', 'RD', '研发中心', 'ou-hq', ARRAY['ou-hq', 'ou-rd'], TIMESTAMPTZ '2026-06-10 08:06:00+00'),
  ('ou-product', 'demo', 'PD', '产品中心', 'ou-hq', ARRAY['ou-hq', 'ou-product'], TIMESTAMPTZ '2026-06-10 08:07:00+00'),
  ('ou-marketing', 'demo', 'MKT', '市场中心', 'ou-hq', ARRAY['ou-hq', 'ou-marketing'], TIMESTAMPTZ '2026-06-10 08:08:00+00'),
  ('ou-customer', 'demo', 'CS', '客户成功部', 'ou-hq', ARRAY['ou-hq', 'ou-customer'], TIMESTAMPTZ '2026-06-10 08:09:00+00')
ON CONFLICT (id) DO UPDATE
SET code = EXCLUDED.code,
    name = EXCLUDED.name,
    parent_id = EXCLUDED.parent_id,
    path = EXCLUDED.path;

INSERT INTO permission_sets (id, tenant_id, name, description, permissions, created_at)
VALUES
  (
    'ps-employee',
    'demo',
    'Employee Self Service',
    'Self-service dashboard, clock, leave, and own workflow access.',
    '[
      {"resource":"me","action":"read","scope":"all","menu_key":"workbench"},
      {"resource":"me","action":"create","scope":"all","menu_key":"workbench"},
      {"resource":"me","action":"update","scope":"all","menu_key":"workbench"},
      {"resource":"hr.employee","action":"read","scope":"self","menu_key":"hr.employees"},
      {"resource":"attendance.leave","action":"create","scope":"self","menu_key":"attendance.leave"},
      {"resource":"attendance.clock","action":"read","scope":"self","menu_key":"attendance.clock"},
      {"resource":"attendance.clock","action":"create","scope":"self","menu_key":"attendance.clock"},
      {"resource":"attendance.correction","action":"read","scope":"self","menu_key":"attendance.corrections"},
      {"resource":"attendance.correction","action":"create","scope":"self","menu_key":"attendance.corrections"},
      {"resource":"workflow.form_instance","action":"read","scope":"self","menu_key":"workflow.instances"},
      {"resource":"workflow.form_instance","action":"create","scope":"self","menu_key":"workflow.instances"},
      {"resource":"workflow.form_instance","action":"submit","scope":"self","menu_key":"workflow.instances"},
      {"resource":"agent.run","action":"read","scope":"own","menu_key":"agents.runs"},
      {"resource":"agent.run","action":"create","scope":"all","menu_key":"agents.runs"}
    ]'::jsonb,
    TIMESTAMPTZ '2026-06-10 08:01:00+00'
  ),
  (
    'ps-hr-readonly',
    'demo',
    'HR Readonly',
    'Read-only HR and dashboard access.',
    '[
      {"resource":"me","action":"read","scope":"all","menu_key":"workbench"},
      {"resource":"hr.employee","action":"read","scope":"all","menu_key":"hr.employees"},
      {"resource":"hr.org_unit","action":"read","scope":"all","menu_key":"hr.org_units"},
      {"resource":"attendance.leave","action":"read","scope":"all","menu_key":"attendance.leave"},
      {"resource":"attendance.clock","action":"read","scope":"all","menu_key":"attendance.clock"},
      {"resource":"workflow.form_template","action":"read","scope":"all","menu_key":"workflow.forms"},
      {"resource":"workflow.form_instance","action":"read","scope":"all","menu_key":"workflow.instances"}
    ]'::jsonb,
    TIMESTAMPTZ '2026-06-10 08:04:00+00'
  ),
  (
    'ps-attendance-manager',
    'demo',
    'Attendance Manager',
    'Attendance read/write and approval access.',
    '[
      {"resource":"me","action":"read","scope":"all","menu_key":"workbench"},
      {"resource":"hr.employee","action":"read","scope":"all","menu_key":"hr.employees"},
      {"resource":"hr.org_unit","action":"read","scope":"all","menu_key":"hr.org_units"},
      {"resource":"attendance.leave","action":"read","scope":"all","menu_key":"attendance.leave"},
      {"resource":"attendance.leave","action":"create","scope":"all","menu_key":"attendance.leave"},
      {"resource":"attendance.leave","action":"update","scope":"all","menu_key":"attendance.leave"},
      {"resource":"attendance.worksite","action":"read","scope":"all","menu_key":"attendance.worksites"},
      {"resource":"attendance.worksite","action":"create","scope":"all","menu_key":"attendance.worksites"},
      {"resource":"attendance.worksite","action":"update","scope":"all","menu_key":"attendance.worksites"},
      {"resource":"attendance.shift","action":"read","scope":"all","menu_key":"attendance.shifts"},
      {"resource":"attendance.shift","action":"create","scope":"all","menu_key":"attendance.shifts"},
      {"resource":"attendance.shift","action":"update","scope":"all","menu_key":"attendance.shifts"},
      {"resource":"attendance.shift_assignment","action":"read","scope":"all","menu_key":"attendance.shift_assignments"},
      {"resource":"attendance.shift_assignment","action":"create","scope":"all","menu_key":"attendance.shift_assignments"},
      {"resource":"attendance.clock","action":"read","scope":"all","menu_key":"attendance.clock"},
      {"resource":"attendance.clock","action":"create","scope":"all","menu_key":"attendance.clock"},
      {"resource":"attendance.correction","action":"read","scope":"all","menu_key":"attendance.corrections"},
      {"resource":"attendance.correction","action":"create","scope":"all","menu_key":"attendance.corrections"},
      {"resource":"attendance.correction","action":"approve","scope":"all","menu_key":"attendance.corrections"},
      {"resource":"attendance.correction","action":"update","scope":"all","menu_key":"attendance.corrections"}
    ]'::jsonb,
    TIMESTAMPTZ '2026-06-10 08:05:00+00'
  ),
  (
    'ps-workflow-approver',
    'demo',
    'Workflow Approver',
    'Workflow read, approve, and update access.',
    '[
      {"resource":"me","action":"read","scope":"all","menu_key":"workbench"},
      {"resource":"hr.employee","action":"read","scope":"all","menu_key":"hr.employees"},
      {"resource":"hr.org_unit","action":"read","scope":"all","menu_key":"hr.org_units"},
      {"resource":"attendance.leave","action":"read","scope":"all","menu_key":"attendance.leave"},
      {"resource":"attendance.clock","action":"read","scope":"all","menu_key":"attendance.clock"},
      {"resource":"workflow.form_template","action":"read","scope":"all","menu_key":"workflow.forms"},
      {"resource":"workflow.form_instance","action":"read","scope":"all","menu_key":"workflow.instances"},
      {"resource":"workflow.form_instance","action":"approve","scope":"all","menu_key":"workflow.instances"},
      {"resource":"workflow.form_instance","action":"update","scope":"all","menu_key":"workflow.instances"}
    ]'::jsonb,
    TIMESTAMPTZ '2026-06-10 08:06:00+00'
  ),
  (
    'ps-insights-viewer',
    'demo',
    'Insights Viewer',
    'Read-only people, attendance, workflow, and insights data.',
    '[
      {"resource":"me","action":"read","scope":"all","menu_key":"workbench"},
      {"resource":"hr.employee","action":"read","scope":"all","menu_key":"hr.employees"},
      {"resource":"hr.org_unit","action":"read","scope":"all","menu_key":"hr.org_units"},
      {"resource":"attendance.leave","action":"read","scope":"all","menu_key":"attendance.leave"},
      {"resource":"attendance.clock","action":"read","scope":"all","menu_key":"attendance.clock"},
      {"resource":"workflow.form_instance","action":"read","scope":"all","menu_key":"workflow.instances"},
      {"resource":"agent.run","action":"read","scope":"all","menu_key":"agents.runs"},
      {"resource":"agent.knowledge_article","action":"read","scope":"all","menu_key":"agents.runs"}
    ]'::jsonb,
    TIMESTAMPTZ '2026-06-10 08:08:00+00'
  )
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name,
    description = EXCLUDED.description,
    permissions = EXCLUDED.permissions;

INSERT INTO attendance_worksites (id, tenant_id, name, address, latitude, longitude, radius_meters, status, created_at, updated_at)
VALUES ('aws-demo-hq', 'demo', 'Demo HQ', 'Demo headquarters', 25.033964, 121.564468, 300, 'active', TIMESTAMPTZ '2026-06-10 08:00:00+00', TIMESTAMPTZ '2026-06-10 08:00:00+00')
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name,
    address = EXCLUDED.address,
    latitude = EXCLUDED.latitude,
    longitude = EXCLUDED.longitude,
    radius_meters = EXCLUDED.radius_meters,
    status = EXCLUDED.status,
    updated_at = EXCLUDED.updated_at;

INSERT INTO attendance_shifts (id, tenant_id, name, clock_in_start, clock_in_end, clock_out_start, clock_out_end, late_grace_minutes, early_leave_grace_minutes, status, created_at, updated_at)
VALUES ('ash-day', 'demo', 'Day Shift', '08:00', '10:00', '17:00', '19:00', 10, 10, 'active', TIMESTAMPTZ '2026-06-10 08:00:00+00', TIMESTAMPTZ '2026-06-10 08:00:00+00')
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name,
    clock_in_start = EXCLUDED.clock_in_start,
    clock_in_end = EXCLUDED.clock_in_end,
    clock_out_start = EXCLUDED.clock_out_start,
    clock_out_end = EXCLUDED.clock_out_end,
    status = EXCLUDED.status,
    updated_at = EXCLUDED.updated_at;

CREATE TEMP TABLE tmp_demo_bulk_users ON COMMIT DROP AS
WITH current_count AS (
  SELECT count(*)::integer AS employee_count
  FROM employees
  WHERE tenant_id = 'demo'
),
needed AS (
  SELECT greatest(100 - employee_count, 0)::integer AS count
  FROM current_count
),
candidate AS (
  SELECT n
  FROM generate_series(1, 100) AS s(n)
  WHERE NOT EXISTS (
    SELECT 1
    FROM employees e
    WHERE e.tenant_id = 'demo'
      AND e.id = 'emp-demo-bulk-' || to_char(s.n, 'FM000')
  )
  ORDER BY n
  LIMIT (SELECT count FROM needed)
)
SELECT
  n,
  'acct-demo-bulk-' || to_char(n, 'FM000') AS account_id,
  'emp-demo-bulk-' || to_char(n, 'FM000') AS employee_id,
  'DEMO' || to_char(n + 3, 'FM0000') AS employee_no,
  'Demo User ' || to_char(n, 'FM000') AS display_name,
  'demo.bulk' || to_char(n, 'FM000') || '@demo.local' AS email,
  (ARRAY['ou-hq','ou-ops','ou-hr','ou-finance','ou-sales','ou-security','ou-rd','ou-product','ou-marketing','ou-customer'])[((n - 1) % 10) + 1] AS org_unit_id,
  (ARRAY['full_time','part_time','intern','contractor','other'])[((n - 1) % 5) + 1] AS category,
  CASE
    WHEN n % 17 = 0 THEN 'resigned'
    WHEN n % 13 = 0 THEN 'leave_suspended'
    WHEN n % 11 = 0 THEN 'onboarding'
    WHEN n % 7 = 0 THEN 'probation'
    ELSE 'active'
  END AS employee_status,
  CASE
    WHEN n % 12 = 0 THEN 'ps-attendance-manager'
    WHEN n % 12 = 1 THEN 'ps-workflow-approver'
    WHEN n % 12 = 2 THEN 'ps-hr-readonly'
    WHEN n % 12 = 3 THEN 'ps-insights-viewer'
    ELSE 'ps-employee'
  END AS permission_set_id,
  DATE '2026-01-01' + ((n * 3) % 170) AS hire_date,
  CASE WHEN n % 17 = 0 THEN DATE '2026-07-01' - ((n % 5) * INTERVAL '1 day') ELSE NULL END AS resign_date,
  '0912' || lpad(n::text, 6, '0') AS phone,
  TIMESTAMPTZ '2026-06-10 09:00:00+00' + (n * INTERVAL '1 minute') AS created_at
FROM candidate;

INSERT INTO accounts (id, tenant_id, display_name, email, employee_id, status, user_group_ids, direct_permission_set_ids, active_assumable_role_id, created_at)
SELECT
  account_id,
  'demo',
  display_name,
  email,
  employee_id,
  CASE WHEN employee_status = 'resigned' THEN 'disabled' ELSE 'active' END,
  ARRAY[]::text[],
  ARRAY[permission_set_id]::text[],
  '',
  created_at
FROM tmp_demo_bulk_users
ON CONFLICT (id) DO UPDATE
SET display_name = EXCLUDED.display_name,
    email = EXCLUDED.email,
    employee_id = EXCLUDED.employee_id,
    status = EXCLUDED.status,
    direct_permission_set_ids = EXCLUDED.direct_permission_set_ids;

INSERT INTO employees (
  id,
  tenant_id,
  employee_no,
  name,
  company_email,
  phone,
  org_unit_id,
  account_id,
  manager_employee_id,
  position,
  category,
  status,
  employment_status,
  hire_date,
  resign_date,
  basic_info,
  employment_info,
  contact_info,
  insurance_info,
  created_at,
  updated_at
)
SELECT
  employee_id,
  'demo',
  employee_no,
  display_name,
  email,
  phone,
  org_unit_id,
  account_id,
  CASE
    WHEN EXISTS (SELECT 1 FROM employees m WHERE m.tenant_id = 'demo' AND m.id = 'emp-admin') THEN 'emp-admin'
    ELSE NULL
  END,
  CASE
    WHEN permission_set_id = 'ps-attendance-manager' THEN 'Attendance Coordinator'
    WHEN permission_set_id = 'ps-workflow-approver' THEN 'Workflow Approver'
    WHEN permission_set_id = 'ps-hr-readonly' THEN 'HR Analyst'
    WHEN permission_set_id = 'ps-insights-viewer' THEN 'Business Analyst'
    ELSE 'Demo Specialist'
  END,
  category,
  employee_status,
  employee_status,
  hire_date,
  resign_date,
  jsonb_build_object(
    'name', display_name,
    'company_email', email,
    'nationality_type', 'local',
    'national_id', 'Z' || lpad(n::text, 9, '0')
  ),
  jsonb_build_object(
    'org_unit_id', org_unit_id,
    'position',
    CASE
      WHEN permission_set_id = 'ps-attendance-manager' THEN 'Attendance Coordinator'
      WHEN permission_set_id = 'ps-workflow-approver' THEN 'Workflow Approver'
      WHEN permission_set_id = 'ps-hr-readonly' THEN 'HR Analyst'
      WHEN permission_set_id = 'ps-insights-viewer' THEN 'Business Analyst'
      ELSE 'Demo Specialist'
    END,
    'category', category,
    'manager_employee_id',
    CASE
      WHEN EXISTS (SELECT 1 FROM employees m WHERE m.tenant_id = 'demo' AND m.id = 'emp-admin') THEN 'emp-admin'
      ELSE ''
    END
  ),
  jsonb_build_object('mobile_phone', phone),
  jsonb_build_object('labor_insurance_salary', (36000 + ((n % 12) * 2500))::text),
  created_at,
  created_at
FROM tmp_demo_bulk_users
ON CONFLICT (id) DO UPDATE
SET employee_no = EXCLUDED.employee_no,
    name = EXCLUDED.name,
    company_email = EXCLUDED.company_email,
    phone = EXCLUDED.phone,
    org_unit_id = EXCLUDED.org_unit_id,
    account_id = EXCLUDED.account_id,
    manager_employee_id = EXCLUDED.manager_employee_id,
    position = EXCLUDED.position,
    category = EXCLUDED.category,
    status = EXCLUDED.status,
    employment_status = EXCLUDED.employment_status,
    hire_date = EXCLUDED.hire_date,
    resign_date = EXCLUDED.resign_date,
    basic_info = EXCLUDED.basic_info,
    employment_info = EXCLUDED.employment_info,
    contact_info = EXCLUDED.contact_info,
    insurance_info = EXCLUDED.insurance_info,
    updated_at = EXCLUDED.updated_at;

INSERT INTO user_identities (id, tenant_id, account_id, provider, subject, email, created_at)
SELECT
  'uid-keycloak-demo-bulk-' || to_char(n, 'FM000'),
  'demo',
  account_id,
  'keycloak',
  account_id,
  email,
  created_at
FROM tmp_demo_bulk_users
ON CONFLICT (tenant_id, provider, subject) DO UPDATE
SET account_id = EXCLUDED.account_id,
    email = EXCLUDED.email;

INSERT INTO attendance_shift_assignments (id, tenant_id, employee_id, shift_id, worksite_id, effective_from, status, created_at, updated_at)
SELECT
  'asa-' || employee_id,
  'demo',
  employee_id,
  'ash-day',
  'aws-demo-hq',
  TIMESTAMPTZ '2026-06-01 00:00:00+00',
  'active',
  created_at,
  created_at
FROM tmp_demo_bulk_users
WHERE employee_status <> 'resigned'
ON CONFLICT (id) DO UPDATE
SET shift_id = EXCLUDED.shift_id,
    worksite_id = EXCLUDED.worksite_id,
    status = EXCLUDED.status,
    updated_at = EXCLUDED.updated_at;

INSERT INTO leave_balances (id, tenant_id, employee_id, leave_type, remaining_hours, updated_at)
SELECT
  'lb-' || employee_id || '-annual',
  'demo',
  employee_id,
  'annual',
  32 + ((n % 12) * 8),
  created_at
FROM tmp_demo_bulk_users
WHERE employee_status <> 'resigned'
ON CONFLICT (tenant_id, employee_id, leave_type) DO UPDATE
SET remaining_hours = EXCLUDED.remaining_hours,
    updated_at = EXCLUDED.updated_at;

INSERT INTO leave_requests (id, tenant_id, employee_id, leave_type, start_at, end_at, hours, reason, status, form_instance_id, created_at)
SELECT
  'lr-' || employee_id || '-20260701',
  'demo',
  employee_id,
  'annual',
  TIMESTAMPTZ '2026-07-01 09:00:00+00',
  TIMESTAMPTZ '2026-07-01 18:00:00+00',
  8,
  'Seeded dashboard leave sample',
  'approved',
  '',
  created_at
FROM tmp_demo_bulk_users
WHERE employee_status IN ('active', 'probation') AND n % 9 = 0
ON CONFLICT (id) DO UPDATE
SET status = EXCLUDED.status,
    start_at = EXCLUDED.start_at,
    end_at = EXCLUDED.end_at,
    hours = EXCLUDED.hours;

INSERT INTO attendance_clock_records (
  id,
  tenant_id,
  employee_id,
  shift_assignment_id,
  shift_id,
  worksite_id,
  work_date,
  direction,
  clocked_at,
  latitude,
  longitude,
  accuracy_meters,
  distance_meters,
  record_status,
  rejection_reason,
  source,
  device_id,
  device_info,
  correction_request_id,
  created_at
)
SELECT
  'acr-' || employee_id || '-20260701-in',
  'demo',
  employee_id,
  'asa-' || employee_id,
  'ash-day',
  'aws-demo-hq',
  '2026-07-01',
  'clock_in',
  TIMESTAMPTZ '2026-07-01 08:30:00+00' + ((n % 80) * INTERVAL '1 minute'),
  25.033964,
  121.564468,
  10 + (n % 30),
  0,
  'accepted',
  '',
  'geofence',
  'seeded-dashboard-db',
  jsonb_build_object('seed', 'dashboard_100_users'),
  '',
  created_at
FROM tmp_demo_bulk_users
WHERE employee_status IN ('active', 'probation') AND n % 4 <> 0
ON CONFLICT (id) DO UPDATE
SET clocked_at = EXCLUDED.clocked_at,
    record_status = EXCLUDED.record_status,
    device_info = EXCLUDED.device_info;

INSERT INTO attendance_clock_records (
  id,
  tenant_id,
  employee_id,
  shift_assignment_id,
  shift_id,
  worksite_id,
  work_date,
  direction,
  clocked_at,
  latitude,
  longitude,
  accuracy_meters,
  distance_meters,
  record_status,
  rejection_reason,
  source,
  device_id,
  device_info,
  correction_request_id,
  created_at
)
SELECT
  'acr-' || employee_id || '-20260701-out',
  'demo',
  employee_id,
  'asa-' || employee_id,
  'ash-day',
  'aws-demo-hq',
  '2026-07-01',
  'clock_out',
  TIMESTAMPTZ '2026-07-01 17:35:00+00' + ((n % 70) * INTERVAL '1 minute'),
  25.033964,
  121.564468,
  10 + (n % 30),
  0,
  'accepted',
  '',
  'geofence',
  'seeded-dashboard-db',
  jsonb_build_object('seed', 'dashboard_100_users'),
  '',
  created_at
FROM tmp_demo_bulk_users
WHERE employee_status IN ('active', 'probation') AND n % 6 = 0
ON CONFLICT (id) DO UPDATE
SET clocked_at = EXCLUDED.clocked_at,
    record_status = EXCLUDED.record_status,
    device_info = EXCLUDED.device_info;

INSERT INTO authz_permission_versions (tenant_id, version, updated_at)
VALUES ('demo', 1, now())
ON CONFLICT (tenant_id) DO UPDATE
SET version = authz_permission_versions.version + 1,
    updated_at = EXCLUDED.updated_at;

COMMIT;

SELECT
  'demo_employees' AS metric,
  count(*)::text AS value
FROM employees
WHERE tenant_id = 'demo'
UNION ALL
SELECT
  'demo_bulk_employees',
  count(*)::text
FROM employees
WHERE tenant_id = 'demo'
  AND id LIKE 'emp-demo-bulk-%'
UNION ALL
SELECT
  'demo_accounts',
  count(*)::text
FROM accounts
WHERE tenant_id = 'demo'
UNION ALL
SELECT
  'demo_leave_requests_20260701',
  count(*)::text
FROM leave_requests
WHERE tenant_id = 'demo'
  AND start_at::date = DATE '2026-07-01'
UNION ALL
SELECT
  'demo_clock_records_20260701',
  count(*)::text
FROM attendance_clock_records
WHERE tenant_id = 'demo'
  AND work_date = '2026-07-01';

SELECT status, count(*) AS employees
FROM employees
WHERE tenant_id = 'demo'
GROUP BY status
ORDER BY status;

SELECT org_unit_id, count(*) AS employees
FROM employees
WHERE tenant_id = 'demo'
GROUP BY org_unit_id
ORDER BY org_unit_id;
