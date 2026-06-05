-- Additional HR employee permission points implied by the 员工管理 PRD
-- (batch import, delete). read/write/export already exist from the seed.

INSERT INTO iam_permissions (id, tenant_id, application_code, resource_type, action, default_scope, risk_level, high_risk, description) VALUES
    ('hr.employee.import', 'tenant-ikala', 'hr', 'employee', 'import', 'tenant', 'high', true,  '批次匯入員工'),
    ('hr.employee.delete', 'tenant-ikala', 'hr', 'employee', 'delete', 'tenant', 'high', true,  '刪除員工（含批次）')
ON CONFLICT (id) DO NOTHING;

-- Grant to HR admin and tenant admin permission sets.
INSERT INTO iam_permission_set_permissions (tenant_id, permission_set_id, permission_id) VALUES
    ('tenant-ikala','ps-hr-admin','hr.employee.import'),
    ('tenant-ikala','ps-hr-admin','hr.employee.delete'),
    ('tenant-ikala','ps-tenant-admin','hr.employee.import'),
    ('tenant-ikala','ps-tenant-admin','hr.employee.delete')
ON CONFLICT DO NOTHING;
