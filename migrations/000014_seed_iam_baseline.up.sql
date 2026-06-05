-- Baseline seed reproducing the prototype (services/api/seed.go) in the target
-- model, so the existing platform-ui frontend renders identically. Runs as the
-- migration owner (BYPASSRLS), so tenant_id is written directly. Idempotent.

-- Tenants ---------------------------------------------------------------------
INSERT INTO iam_tenants (id, name, status) VALUES
    ('tenant-ikala', 'iKala Demo', 'active'),
    ('tenant-acme',  'Acme Corp',  'active')
ON CONFLICT (id) DO NOTHING;

-- Applications ----------------------------------------------------------------
INSERT INTO iam_applications (id, application_code, name, resource_types, actions) VALUES
    ('app-hr', 'hr', 'HR Core',
        '["employee","org_unit","form_template","workflow.form","agent","menu"]',
        '["read","write","export","submit","run","view"]'),
    ('app-iam', 'iam', 'Permission Center',
        '["iam","audit_log"]', '["read","write"]')
ON CONFLICT (id) DO NOTHING;

-- Permissions (standard application.resource.action naming) -------------------
INSERT INTO iam_permissions (id, tenant_id, application_code, resource_type, action, default_scope, risk_level, high_risk, description) VALUES
    ('menu.home',                 'tenant-ikala', 'hr',  'menu',         'view',   'tenant',     'normal',   false, '主頁菜单'),
    ('menu.forms',                'tenant-ikala', 'hr',  'menu',         'view',   'tenant',     'normal',   false, '表單菜单'),
    ('menu.agents',               'tenant-ikala', 'hr',  'menu',         'view',   'tenant',     'normal',   false, 'AI 助理菜单'),
    ('menu.workspace',            'tenant-ikala', 'hr',  'menu',         'view',   'tenant',     'high',     true,  '工作區設定菜单'),
    ('hr.employee.read',          'tenant-ikala', 'hr',  'employee',     'read',   'department', 'normal',   false, '查看員工'),
    ('hr.employee.write',         'tenant-ikala', 'hr',  'employee',     'write',  'tenant',     'high',     true,  '編輯員工'),
    ('hr.employee.export',        'tenant-ikala', 'hr',  'employee',     'export', 'tenant',     'high',     true,  '導出員工'),
    ('hr.org_unit.read',          'tenant-ikala', 'hr',  'org_unit',     'read',   'tenant',     'normal',   false, '查看組織'),
    ('hr.org_unit.write',         'tenant-ikala', 'hr',  'org_unit',     'write',  'tenant',     'high',     true,  '編輯組織'),
    ('hr.form_template.read',     'tenant-ikala', 'hr',  'form_template','read',   'tenant',     'normal',   false, '查看表單模板'),
    ('hr.form_template.write',    'tenant-ikala', 'hr',  'form_template','write',  'tenant',     'normal',   false, '編輯表單模板'),
    ('hr.workflow.form.submit',   'tenant-ikala', 'hr',  'workflow.form','submit', 'own',        'normal',   false, '提交表單申請'),
    ('hr.agent.run',              'tenant-ikala', 'hr',  'agent',        'run',    'tenant',     'normal',   false, '使用 AI 助理'),
    ('agent.tool.execute_high_risk','tenant-ikala','hr', 'agent',        'execute_high_risk','tenant','high', true, 'Agent 高危工具'),
    ('iam.read',                  'tenant-ikala', 'iam', 'iam',          'read',   'tenant',     'high',     true,  '查看權限配置'),
    ('iam.write',                 'tenant-ikala', 'iam', 'iam',          'write',  'tenant',     'high',     true,  '修改權限配置'),
    ('iam.audit_log.read',        'tenant-ikala', 'iam', 'audit_log',    'read',   'tenant',     'high',     true,  '查看審計日誌')
ON CONFLICT (id) DO NOTHING;

-- Permission sets -------------------------------------------------------------
INSERT INTO iam_permission_sets (id, tenant_id, name, description) VALUES
    ('ps-employee',     'tenant-ikala', '員工基礎權限', '員工自助、表單申請、AI 問答'),
    ('ps-hr-admin',     'tenant-ikala', 'HR 管理權限',  'HR 後台管理'),
    ('ps-tenant-admin', 'tenant-ikala', '租戶管理員權限','租戶全量管理含權限配置')
ON CONFLICT (id) DO NOTHING;

INSERT INTO iam_permission_set_permissions (tenant_id, permission_set_id, permission_id) VALUES
    -- ps-employee
    ('tenant-ikala','ps-employee','menu.home'),
    ('tenant-ikala','ps-employee','menu.forms'),
    ('tenant-ikala','ps-employee','menu.agents'),
    ('tenant-ikala','ps-employee','hr.employee.read'),
    ('tenant-ikala','ps-employee','hr.org_unit.read'),
    ('tenant-ikala','ps-employee','hr.form_template.read'),
    ('tenant-ikala','ps-employee','hr.workflow.form.submit'),
    ('tenant-ikala','ps-employee','hr.agent.run'),
    -- ps-hr-admin (employee + HR management)
    ('tenant-ikala','ps-hr-admin','menu.home'),
    ('tenant-ikala','ps-hr-admin','menu.forms'),
    ('tenant-ikala','ps-hr-admin','menu.agents'),
    ('tenant-ikala','ps-hr-admin','menu.workspace'),
    ('tenant-ikala','ps-hr-admin','hr.employee.read'),
    ('tenant-ikala','ps-hr-admin','hr.employee.write'),
    ('tenant-ikala','ps-hr-admin','hr.employee.export'),
    ('tenant-ikala','ps-hr-admin','hr.org_unit.read'),
    ('tenant-ikala','ps-hr-admin','hr.org_unit.write'),
    ('tenant-ikala','ps-hr-admin','hr.form_template.read'),
    ('tenant-ikala','ps-hr-admin','hr.form_template.write'),
    ('tenant-ikala','ps-hr-admin','hr.workflow.form.submit'),
    ('tenant-ikala','ps-hr-admin','hr.agent.run'),
    ('tenant-ikala','ps-hr-admin','agent.tool.execute_high_risk'),
    ('tenant-ikala','ps-hr-admin','iam.audit_log.read'),
    -- ps-tenant-admin (hr-admin + iam config)
    ('tenant-ikala','ps-tenant-admin','menu.home'),
    ('tenant-ikala','ps-tenant-admin','menu.forms'),
    ('tenant-ikala','ps-tenant-admin','menu.agents'),
    ('tenant-ikala','ps-tenant-admin','menu.workspace'),
    ('tenant-ikala','ps-tenant-admin','hr.employee.read'),
    ('tenant-ikala','ps-tenant-admin','hr.employee.write'),
    ('tenant-ikala','ps-tenant-admin','hr.employee.export'),
    ('tenant-ikala','ps-tenant-admin','hr.org_unit.read'),
    ('tenant-ikala','ps-tenant-admin','hr.org_unit.write'),
    ('tenant-ikala','ps-tenant-admin','hr.form_template.read'),
    ('tenant-ikala','ps-tenant-admin','hr.form_template.write'),
    ('tenant-ikala','ps-tenant-admin','hr.workflow.form.submit'),
    ('tenant-ikala','ps-tenant-admin','hr.agent.run'),
    ('tenant-ikala','ps-tenant-admin','agent.tool.execute_high_risk'),
    ('tenant-ikala','ps-tenant-admin','iam.read'),
    ('tenant-ikala','ps-tenant-admin','iam.write'),
    ('tenant-ikala','ps-tenant-admin','iam.audit_log.read')
ON CONFLICT DO NOTHING;

-- User groups (migrated from prototype roles, §17) -----------------------------
INSERT INTO iam_user_groups (id, tenant_id, code, name, description) VALUES
    ('group-employee',     'tenant-ikala', 'employee',     '員工基礎組',    '默認員工組'),
    ('group-hr-admin',     'tenant-ikala', 'hr-admin',     'HR 管理組',     '人力資源後台管理組'),
    ('group-tenant-admin', 'tenant-ikala', 'tenant-admin', '租戶管理員組',  '租戶最高權限組')
ON CONFLICT (id) DO NOTHING;

-- Data scopes -----------------------------------------------------------------
INSERT INTO iam_data_scopes (id, tenant_id, name, scope_type, conditions) VALUES
    ('ds-department', 'tenant-ikala', '本部门', 'department', '{}'),
    ('ds-tenant',     'tenant-ikala', '全租户', 'tenant',     '{}')
ON CONFLICT (id) DO NOTHING;

-- Group -> permission set assignments -----------------------------------------
INSERT INTO iam_permission_set_assignments (id, tenant_id, permission_set_id, subject_type, subject_id, effect, data_scope_id) VALUES
    ('psa-employee',     'tenant-ikala', 'ps-employee',     'group', 'group-employee',     'allow', 'ds-department'),
    ('psa-hr-admin',     'tenant-ikala', 'ps-hr-admin',     'group', 'group-hr-admin',     'allow', 'ds-tenant'),
    ('psa-tenant-admin', 'tenant-ikala', 'ps-tenant-admin', 'group', 'group-tenant-admin', 'allow', 'ds-tenant')
ON CONFLICT (id) DO NOTHING;

-- Accounts --------------------------------------------------------------------
INSERT INTO iam_accounts (id, tenant_id, email, display_name, status) VALUES
    ('acct-tammy',    'tenant-ikala', 'tammy@ikala.ai',    'Tammy Chen',    'active'),
    ('acct-hr-admin', 'tenant-ikala', 'hr-admin@ikala.ai', 'Hui-Ling Wang', 'active'),
    ('acct-super',    'tenant-ikala', 'super@ikala.ai',    'Tenant Admin',  'active'),
    ('acct-acme',     'tenant-acme',  'admin@acme.test',   'Acme Admin',    'active')
ON CONFLICT (id) DO NOTHING;

-- Group memberships (migrated from accounts.role_ids, §17) ---------------------
INSERT INTO iam_group_memberships (id, tenant_id, account_id, group_id) VALUES
    ('gm-tammy',    'tenant-ikala', 'acct-tammy',    'group-employee'),
    ('gm-hr-admin', 'tenant-ikala', 'acct-hr-admin', 'group-hr-admin'),
    ('gm-super',    'tenant-ikala', 'acct-super',    'group-tenant-admin')
ON CONFLICT (id) DO NOTHING;

-- Menu items ------------------------------------------------------------------
INSERT INTO iam_menu_items (id, tenant_id, application_code, label, route, icon, required_permission_id, sort_order) VALUES
    ('home',      'tenant-ikala', 'hr', '主頁',      '/',         'house',     'menu.home',      10),
    ('forms',     'tenant-ikala', 'hr', '表單申請',  '/forms',    'file-text', 'menu.forms',     20),
    ('agents',    'tenant-ikala', 'hr', 'AI 助理',   '/agents',   'bot',       'menu.agents',    30),
    ('workspace', 'tenant-ikala', 'hr', '工作區設定','/workspace','settings',  'menu.workspace', 90)
ON CONFLICT (id) DO NOTHING;

-- Field policies (so authz/check returns non-empty field_policies, §13) --------
INSERT INTO iam_field_policies (id, tenant_id, application_code, resource_type, field, effect, sensitivity) VALUES
    ('fp-employee-salary', 'tenant-ikala', 'hr', 'employee', 'salary', 'masked',  'high'),
    ('fp-employee-email',  'tenant-ikala', 'hr', 'employee', 'email',  'visible', 'low')
ON CONFLICT (id) DO NOTHING;

-- AssumableRole + boundary (validates assume flow; not wired into normal groups)
INSERT INTO iam_permission_boundaries (id, tenant_id, name, allowed_permissions, scope_type) VALUES
    ('boundary.platform-support-readonly', 'tenant-ikala', '平台只讀排障邊界',
        '["hr.employee.read","hr.org_unit.read","iam.audit_log.read"]', 'tenant')
ON CONFLICT (id) DO NOTHING;

INSERT INTO iam_assumable_roles (id, tenant_id, name, description, permission_boundary_id, max_session_minutes, requires_approval) VALUES
    ('assumable.platform-support-readonly', 'tenant-ikala', '平台只讀排障身份',
        '平台運維臨時只讀進入租戶排障', 'boundary.platform-support-readonly', 60, false)
ON CONFLICT (id) DO NOTHING;

INSERT INTO iam_permission_set_assignments (id, tenant_id, permission_set_id, subject_type, subject_id, effect) VALUES
    ('psa-support-ro', 'tenant-ikala', 'ps-hr-admin', 'assumable_role', 'assumable.platform-support-readonly', 'allow')
ON CONFLICT (id) DO NOTHING;
