DELETE FROM iam_permission_set_permissions WHERE permission_id IN ('hr.employee.import','hr.employee.delete');
DELETE FROM iam_permissions WHERE id IN ('hr.employee.import','hr.employee.delete');
