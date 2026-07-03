-- name: UpsertTenant :one
INSERT INTO tenants (
    id, name, created_at
) VALUES (
    $1, $2, $3
)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    created_at = EXCLUDED.created_at
RETURNING *;

-- name: GetTenant :one
SELECT * FROM tenants
WHERE id = $1;

-- name: ListTenants :many
SELECT * FROM tenants
ORDER BY created_at ASC;

-- name: UpsertAccount :one
INSERT INTO accounts (
    id, tenant_id, display_name, email, employee_id, status,
    user_group_ids, direct_permission_set_ids, active_assumable_role_id,
    created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    display_name = EXCLUDED.display_name,
    email = EXCLUDED.email,
    employee_id = EXCLUDED.employee_id,
    status = EXCLUDED.status,
    user_group_ids = EXCLUDED.user_group_ids,
    direct_permission_set_ids = EXCLUDED.direct_permission_set_ids,
    active_assumable_role_id = EXCLUDED.active_assumable_role_id,
    created_at = EXCLUDED.created_at
RETURNING *;

-- name: GetAccount :one
SELECT * FROM accounts
WHERE tenant_id = $1 AND id = $2;

-- name: ListAccounts :many
SELECT * FROM accounts
WHERE tenant_id = $1
ORDER BY created_at ASC;

-- name: UpsertUserGroup :one
INSERT INTO user_groups (
    id, tenant_id, name, description, member_account_ids,
    permission_set_ids, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    member_account_ids = EXCLUDED.member_account_ids,
    permission_set_ids = EXCLUDED.permission_set_ids,
    created_at = EXCLUDED.created_at
RETURNING *;

-- name: GetUserGroup :one
SELECT * FROM user_groups
WHERE tenant_id = $1 AND id = $2;

-- name: ListUserGroups :many
SELECT * FROM user_groups
WHERE tenant_id = $1
ORDER BY created_at ASC;

-- name: UpsertPermissionSet :one
INSERT INTO permission_sets (
    id, tenant_id, name, description, permissions, created_at
) VALUES (
    $1, $2, $3, $4, $5::jsonb, $6
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    permissions = EXCLUDED.permissions,
    created_at = EXCLUDED.created_at
RETURNING *;

-- name: GetPermissionSet :one
SELECT * FROM permission_sets
WHERE tenant_id = $1 AND id = $2;

-- name: ListPermissionSets :many
SELECT * FROM permission_sets
WHERE tenant_id = $1
ORDER BY created_at ASC;

-- name: UpsertAssumableRole :one
INSERT INTO assumable_roles (
    id, tenant_id, name, description, permission_set_ids, trusted,
    trust_policy, permission_boundary, session_duration_seconds, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, $9, $10
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    permission_set_ids = EXCLUDED.permission_set_ids,
    trusted = EXCLUDED.trusted,
    trust_policy = EXCLUDED.trust_policy,
    permission_boundary = EXCLUDED.permission_boundary,
    session_duration_seconds = EXCLUDED.session_duration_seconds,
    created_at = EXCLUDED.created_at
RETURNING *;

-- name: GetAssumableRole :one
SELECT * FROM assumable_roles
WHERE tenant_id = $1 AND id = $2;

-- name: ListAssumableRoles :many
SELECT * FROM assumable_roles
WHERE tenant_id = $1
ORDER BY created_at ASC;

-- name: UpsertOrgUnit :one
INSERT INTO org_units (
    id, tenant_id, code, name, parent_id, path, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    code = EXCLUDED.code,
    name = EXCLUDED.name,
    parent_id = EXCLUDED.parent_id,
    path = EXCLUDED.path,
    created_at = EXCLUDED.created_at
RETURNING *;

-- name: GetOrgUnit :one
SELECT * FROM org_units
WHERE tenant_id = $1 AND id = $2;

-- name: ListOrgUnits :many
SELECT * FROM org_units
WHERE tenant_id = $1
ORDER BY created_at ASC;

-- name: UpsertEmployee :one
INSERT INTO employees (
    id, tenant_id, employee_no, name, company_email, personal_email, phone,
    org_unit_id, account_id, manager_employee_id, position, category, status, employment_status,
    hire_date, resign_date, basic_info, employment_info, education_military_info,
    contact_info, insurance_info, internal_experiences, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
    $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    employee_no = EXCLUDED.employee_no,
    name = EXCLUDED.name,
    company_email = EXCLUDED.company_email,
    personal_email = EXCLUDED.personal_email,
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
    education_military_info = EXCLUDED.education_military_info,
    contact_info = EXCLUDED.contact_info,
    insurance_info = EXCLUDED.insurance_info,
    internal_experiences = EXCLUDED.internal_experiences,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetEmployee :one
SELECT * FROM employees
WHERE tenant_id = $1 AND id = $2;

-- name: GetEmployeeByEmployeeNo :one
SELECT * FROM employees
WHERE tenant_id = $1 AND employee_no = $2 AND employee_no <> '';

-- name: GetEmployeeByCompanyEmail :one
SELECT * FROM employees
WHERE tenant_id = sqlc.arg(tenant_id) AND lower(company_email) = lower(sqlc.arg(company_email)) AND company_email <> '';

-- name: GetEmployeeByPersonalEmail :one
SELECT * FROM employees
WHERE tenant_id = sqlc.arg(tenant_id) AND lower(personal_email) = lower(sqlc.arg(personal_email)) AND personal_email <> '';

-- name: GetEmployeeByAccountID :one
SELECT * FROM employees
WHERE tenant_id = $1 AND account_id = $2 AND account_id <> '';

-- name: GetEmployeeByBasicInfoField :one
SELECT * FROM employees
WHERE tenant_id = sqlc.arg(tenant_id)
  AND lower(coalesce(basic_info ->> sqlc.arg(field_name)::text, '')) = lower(sqlc.arg(field_value))
  AND coalesce(basic_info ->> sqlc.arg(field_name)::text, '') <> '';

-- name: ListEmployees :many
SELECT * FROM employees
WHERE tenant_id = $1
ORDER BY created_at ASC, id ASC;

-- name: ListEmployeesFiltered :many
SELECT employees.* FROM employees
LEFT JOIN accounts
  ON accounts.tenant_id = employees.tenant_id
 AND accounts.id = employees.account_id
WHERE employees.tenant_id = sqlc.arg(tenant_id)
  AND (
    sqlc.arg(keyword)::text = ''
    OR lower(
      coalesce(employees.employee_no, '') || ' ' ||
      coalesce(employees.name, '') || ' ' ||
      coalesce(employees.company_email, '') || ' ' ||
      coalesce(employees.personal_email, '') || ' ' ||
      coalesce(employees.phone, '')
    ) LIKE '%' || lower(sqlc.arg(keyword)::text) || '%'
    OR lower(coalesce(employees.account_id, '')) LIKE '%' || lower(sqlc.arg(keyword)::text) || '%'
    OR lower(
      coalesce(accounts.display_name, '') || ' ' ||
      coalesce(accounts.email, '') || ' ' ||
      coalesce(accounts.employee_id, '')
    ) LIKE '%' || lower(sqlc.arg(keyword)::text) || '%'
  )
  AND (sqlc.arg(department_id)::text = '' OR employees.org_unit_id = sqlc.arg(department_id))
  AND (
    sqlc.arg(employment_status)::text = ''
    OR coalesce(nullif(employees.employment_status, ''), employees.status) = sqlc.arg(employment_status)
  )
  AND (
    sqlc.arg(employment_status)::text = 'deleted'
    OR coalesce(nullif(employees.employment_status, ''), employees.status) <> 'deleted'
  )
  AND (sqlc.arg(category)::text = '' OR employees.category = sqlc.arg(category))
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'created_at_desc' THEN employees.created_at END DESC,
  CASE WHEN sqlc.arg(sort)::text = 'hire_date_desc' THEN employees.hire_date END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort)::text = 'hire_date_asc' THEN employees.hire_date END ASC NULLS LAST,
  employees.created_at ASC,
  employees.id ASC;

-- name: CountEmployeesFiltered :one
SELECT count(*) FROM employees
LEFT JOIN accounts
  ON accounts.tenant_id = employees.tenant_id
 AND accounts.id = employees.account_id
WHERE employees.tenant_id = sqlc.arg(tenant_id)
  AND (
    sqlc.arg(keyword)::text = ''
    OR lower(
      coalesce(employees.employee_no, '') || ' ' ||
      coalesce(employees.name, '') || ' ' ||
      coalesce(employees.company_email, '') || ' ' ||
      coalesce(employees.personal_email, '') || ' ' ||
      coalesce(employees.phone, '')
    ) LIKE '%' || lower(sqlc.arg(keyword)::text) || '%'
    OR lower(coalesce(employees.account_id, '')) LIKE '%' || lower(sqlc.arg(keyword)::text) || '%'
    OR lower(
      coalesce(accounts.display_name, '') || ' ' ||
      coalesce(accounts.email, '') || ' ' ||
      coalesce(accounts.employee_id, '')
    ) LIKE '%' || lower(sqlc.arg(keyword)::text) || '%'
  )
  AND (sqlc.arg(department_id)::text = '' OR employees.org_unit_id = sqlc.arg(department_id))
  AND (
    sqlc.arg(employment_status)::text = ''
    OR coalesce(nullif(employees.employment_status, ''), employees.status) = sqlc.arg(employment_status)
  )
  AND (
    sqlc.arg(employment_status)::text = 'deleted'
    OR coalesce(nullif(employees.employment_status, ''), employees.status) <> 'deleted'
  )
  AND (sqlc.arg(category)::text = '' OR employees.category = sqlc.arg(category));

-- name: ListEmployeesFilteredPage :many
SELECT employees.* FROM employees
LEFT JOIN accounts
  ON accounts.tenant_id = employees.tenant_id
 AND accounts.id = employees.account_id
WHERE employees.tenant_id = sqlc.arg(tenant_id)
  AND (
    sqlc.arg(keyword)::text = ''
    OR lower(
      coalesce(employees.employee_no, '') || ' ' ||
      coalesce(employees.name, '') || ' ' ||
      coalesce(employees.company_email, '') || ' ' ||
      coalesce(employees.personal_email, '') || ' ' ||
      coalesce(employees.phone, '')
    ) LIKE '%' || lower(sqlc.arg(keyword)::text) || '%'
    OR lower(coalesce(employees.account_id, '')) LIKE '%' || lower(sqlc.arg(keyword)::text) || '%'
    OR lower(
      coalesce(accounts.display_name, '') || ' ' ||
      coalesce(accounts.email, '') || ' ' ||
      coalesce(accounts.employee_id, '')
    ) LIKE '%' || lower(sqlc.arg(keyword)::text) || '%'
  )
  AND (sqlc.arg(department_id)::text = '' OR employees.org_unit_id = sqlc.arg(department_id))
  AND (
    sqlc.arg(employment_status)::text = ''
    OR coalesce(nullif(employees.employment_status, ''), employees.status) = sqlc.arg(employment_status)
  )
  AND (
    sqlc.arg(employment_status)::text = 'deleted'
    OR coalesce(nullif(employees.employment_status, ''), employees.status) <> 'deleted'
  )
  AND (sqlc.arg(category)::text = '' OR employees.category = sqlc.arg(category))
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'created_at_desc' THEN employees.created_at END DESC,
  CASE WHEN sqlc.arg(sort)::text = 'hire_date_desc' THEN employees.hire_date END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort)::text = 'hire_date_asc' THEN employees.hire_date END ASC NULLS LAST,
  employees.created_at ASC,
  employees.id ASC
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;

-- name: NextEmployeeNoSequence :one
INSERT INTO employee_number_sequences (
    tenant_id, prefix, next_value, updated_at
) VALUES (
    sqlc.arg(tenant_id), sqlc.arg(prefix), sqlc.arg(initial_next)::int + 1, now()
)
ON CONFLICT (tenant_id, prefix) DO UPDATE SET
    next_value = GREATEST(employee_number_sequences.next_value + 1, sqlc.arg(initial_next)::int + 1),
    updated_at = now()
RETURNING (next_value - 1)::int;

-- name: UpsertEmployeeImportSession :one
INSERT INTO employee_import_sessions (
    id, tenant_id, filename, object_provider, object_bucket, object_key,
    content_type, size_bytes, sha256, status, rows, summary,
    created_by_account_id, confirmed_by_account_id, created_at, expires_at, confirmed_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    filename = EXCLUDED.filename,
    object_provider = EXCLUDED.object_provider,
    object_bucket = EXCLUDED.object_bucket,
    object_key = EXCLUDED.object_key,
    content_type = EXCLUDED.content_type,
    size_bytes = EXCLUDED.size_bytes,
    sha256 = EXCLUDED.sha256,
    status = EXCLUDED.status,
    rows = EXCLUDED.rows,
    summary = EXCLUDED.summary,
    created_by_account_id = EXCLUDED.created_by_account_id,
    confirmed_by_account_id = EXCLUDED.confirmed_by_account_id,
    created_at = EXCLUDED.created_at,
    expires_at = EXCLUDED.expires_at,
    confirmed_at = EXCLUDED.confirmed_at
RETURNING *;

-- name: GetEmployeeImportSession :one
SELECT * FROM employee_import_sessions
WHERE tenant_id = $1 AND id = $2;

-- name: AppendOutboxEvent :one
INSERT INTO outbox_events (
    id, tenant_id, event_type, aggregate_type, aggregate_id,
    payload, status, retry_count, last_error, created_at, processed_at
) VALUES (
    $1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, $11
)
RETURNING *;

-- name: ListOutboxEvents :many
SELECT * FROM outbox_events
WHERE tenant_id = $1
ORDER BY created_at ASC, id ASC;

-- name: UpsertAttendancePolicy :one
INSERT INTO attendance_policies (
    id, tenant_id, work_time, leave_types, updated_by_account_id, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(work_time)::jsonb,
    sqlc.arg(leave_types)::jsonb, sqlc.arg(updated_by_account_id),
    sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (tenant_id) DO UPDATE SET
    id = EXCLUDED.id,
    work_time = EXCLUDED.work_time,
    leave_types = EXCLUDED.leave_types,
    updated_by_account_id = EXCLUDED.updated_by_account_id,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetAttendancePolicy :one
SELECT * FROM attendance_policies
WHERE tenant_id = sqlc.arg(tenant_id);

-- name: UpsertLeaveBalance :one
INSERT INTO leave_balances (
    id, tenant_id, employee_id, leave_type, remaining_hours, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    employee_id = EXCLUDED.employee_id,
    leave_type = EXCLUDED.leave_type,
    remaining_hours = EXCLUDED.remaining_hours,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetLeaveBalance :one
SELECT * FROM leave_balances
WHERE tenant_id = $1 AND id = $2;

-- name: ListLeaveBalances :many
SELECT * FROM leave_balances
WHERE tenant_id = $1
ORDER BY updated_at ASC;

-- name: ReserveLeaveBalance :one
UPDATE leave_balances
SET remaining_hours = remaining_hours - sqlc.arg(hours)::double precision,
    updated_at = sqlc.arg(updated_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)
  AND employee_id = sqlc.arg(employee_id)
  AND lower(leave_type) = lower(sqlc.arg(leave_type)::text)
  AND remaining_hours >= sqlc.arg(hours)::double precision
RETURNING *;

-- name: ReleaseLeaveBalance :one
UPDATE leave_balances
SET remaining_hours = remaining_hours + sqlc.arg(hours)::double precision,
    updated_at = sqlc.arg(updated_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)
  AND employee_id = sqlc.arg(employee_id)
  AND lower(leave_type) = lower(sqlc.arg(leave_type)::text)
RETURNING *;

-- name: UpsertFormTemplate :one
INSERT INTO form_templates (
    id, tenant_id, key, name, description, schema, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6::jsonb, $7
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    key = EXCLUDED.key,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    schema = EXCLUDED.schema,
    created_at = EXCLUDED.created_at
RETURNING *;

-- name: GetFormTemplate :one
SELECT * FROM form_templates
WHERE tenant_id = $1 AND id = $2;

-- name: GetFormTemplateByKey :one
SELECT * FROM form_templates
WHERE tenant_id = $1 AND key = $2;

-- name: ListFormTemplates :many
SELECT * FROM form_templates
WHERE tenant_id = $1
ORDER BY created_at ASC;

-- name: UpsertFormInstance :one
INSERT INTO form_instances (
    id, tenant_id, template_id, applicant_account_id, status,
    payload, submitted_at, approved_by, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    template_id = EXCLUDED.template_id,
    applicant_account_id = EXCLUDED.applicant_account_id,
    status = EXCLUDED.status,
    payload = EXCLUDED.payload,
    submitted_at = EXCLUDED.submitted_at,
    approved_by = EXCLUDED.approved_by,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetFormInstance :one
SELECT * FROM form_instances
WHERE tenant_id = $1 AND id = $2;

-- name: ListFormInstances :many
SELECT * FROM form_instances
WHERE tenant_id = $1
ORDER BY submitted_at ASC;

-- name: CountFormInstancesByQuery :one
SELECT count(*) FROM form_instances fi
WHERE fi.tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(status)::text = '' OR fi.status = sqlc.arg(status))
  AND (sqlc.arg(template_id)::text = '' OR fi.template_id = sqlc.arg(template_id))
  AND (sqlc.arg(template_key)::text = '' OR EXISTS (
    SELECT 1 FROM form_templates
    WHERE form_templates.tenant_id = fi.tenant_id
      AND form_templates.id = fi.template_id
      AND form_templates.key = sqlc.arg(template_key)
  ))
  AND (sqlc.arg(applicant_account_id)::text = '' OR fi.applicant_account_id = sqlc.arg(applicant_account_id));

-- name: ListFormInstancesByQuery :many
SELECT * FROM form_instances fi
WHERE fi.tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(status)::text = '' OR fi.status = sqlc.arg(status))
  AND (sqlc.arg(template_id)::text = '' OR fi.template_id = sqlc.arg(template_id))
  AND (sqlc.arg(template_key)::text = '' OR EXISTS (
    SELECT 1 FROM form_templates
    WHERE form_templates.tenant_id = fi.tenant_id
      AND form_templates.id = fi.template_id
      AND form_templates.key = sqlc.arg(template_key)
  ))
  AND (sqlc.arg(applicant_account_id)::text = '' OR fi.applicant_account_id = sqlc.arg(applicant_account_id))
ORDER BY fi.submitted_at ASC;

-- name: ListFormInstancePageByQuery :many
SELECT * FROM form_instances fi
WHERE fi.tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(status)::text = '' OR fi.status = sqlc.arg(status))
  AND (sqlc.arg(template_id)::text = '' OR fi.template_id = sqlc.arg(template_id))
  AND (sqlc.arg(template_key)::text = '' OR EXISTS (
    SELECT 1 FROM form_templates
    WHERE form_templates.tenant_id = fi.tenant_id
      AND form_templates.id = fi.template_id
      AND form_templates.key = sqlc.arg(template_key)
  ))
  AND (sqlc.arg(applicant_account_id)::text = '' OR fi.applicant_account_id = sqlc.arg(applicant_account_id))
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'submitted_at_asc' THEN fi.submitted_at END ASC,
  fi.submitted_at DESC,
  fi.id ASC
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;

-- name: DeleteFormInstance :exec
DELETE FROM form_instances
WHERE tenant_id = sqlc.arg(tenant_id) AND id = sqlc.arg(id);

-- name: UpsertLeaveRequest :one
INSERT INTO leave_requests (
    id, tenant_id, employee_id, leave_type, start_at, end_at,
    hours, reason, status, form_instance_id, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    employee_id = EXCLUDED.employee_id,
    leave_type = EXCLUDED.leave_type,
    start_at = EXCLUDED.start_at,
    end_at = EXCLUDED.end_at,
    hours = EXCLUDED.hours,
    reason = EXCLUDED.reason,
    status = EXCLUDED.status,
    form_instance_id = EXCLUDED.form_instance_id,
    created_at = EXCLUDED.created_at
RETURNING *;

-- name: GetLeaveRequest :one
SELECT * FROM leave_requests
WHERE tenant_id = $1 AND id = $2;

-- name: GetLeaveRequestByFormInstanceID :one
SELECT * FROM leave_requests
WHERE tenant_id = sqlc.arg(tenant_id) AND form_instance_id = sqlc.arg(form_instance_id)
LIMIT 1;

-- name: ListLeaveRequests :many
SELECT * FROM leave_requests
WHERE tenant_id = $1
ORDER BY created_at ASC;

-- name: CountLeaveRequestsByQuery :one
SELECT count(*) FROM leave_requests
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (coalesce(cardinality(sqlc.arg(employee_ids)::text[]), 0) = 0 OR employee_id = ANY(sqlc.arg(employee_ids)::text[]))
  AND (sqlc.arg(status)::text = '' OR lower(status) = lower(sqlc.arg(status)::text))
  AND (NULLIF(sqlc.arg(from_date)::text, '') IS NULL OR end_at::date >= NULLIF(sqlc.arg(from_date)::text, '')::date)
  AND (NULLIF(sqlc.arg(to_date)::text, '') IS NULL OR start_at::date <= NULLIF(sqlc.arg(to_date)::text, '')::date);

-- name: ListLeaveRequestsByQuery :many
SELECT * FROM leave_requests
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (coalesce(cardinality(sqlc.arg(employee_ids)::text[]), 0) = 0 OR employee_id = ANY(sqlc.arg(employee_ids)::text[]))
  AND (sqlc.arg(status)::text = '' OR lower(status) = lower(sqlc.arg(status)::text))
  AND (NULLIF(sqlc.arg(from_date)::text, '') IS NULL OR end_at::date >= NULLIF(sqlc.arg(from_date)::text, '')::date)
  AND (NULLIF(sqlc.arg(to_date)::text, '') IS NULL OR start_at::date <= NULLIF(sqlc.arg(to_date)::text, '')::date)
ORDER BY created_at ASC;

-- name: ListLeaveRequestPageByQuery :many
SELECT * FROM leave_requests
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (coalesce(cardinality(sqlc.arg(employee_ids)::text[]), 0) = 0 OR employee_id = ANY(sqlc.arg(employee_ids)::text[]))
  AND (sqlc.arg(status)::text = '' OR lower(status) = lower(sqlc.arg(status)::text))
  AND (NULLIF(sqlc.arg(from_date)::text, '') IS NULL OR end_at::date >= NULLIF(sqlc.arg(from_date)::text, '')::date)
  AND (NULLIF(sqlc.arg(to_date)::text, '') IS NULL OR start_at::date <= NULLIF(sqlc.arg(to_date)::text, '')::date)
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'created_at_asc' THEN created_at END ASC,
  created_at DESC,
  id ASC
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;

-- name: UpsertAttendanceWorksite :one
INSERT INTO attendance_worksites (
    id, tenant_id, name, address, latitude, longitude, radius_meters,
    status, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    name = EXCLUDED.name,
    address = EXCLUDED.address,
    latitude = EXCLUDED.latitude,
    longitude = EXCLUDED.longitude,
    radius_meters = EXCLUDED.radius_meters,
    status = EXCLUDED.status,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetAttendanceWorksite :one
SELECT * FROM attendance_worksites
WHERE tenant_id = $1 AND id = $2;

-- name: ListAttendanceWorksites :many
SELECT * FROM attendance_worksites
WHERE tenant_id = $1
ORDER BY created_at DESC, id ASC;

-- name: UpsertAttendanceShift :one
INSERT INTO attendance_shifts (
    id, tenant_id, name, clock_in_start, clock_in_end, clock_out_start,
    clock_out_end, late_grace_minutes, early_leave_grace_minutes,
    status, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    name = EXCLUDED.name,
    clock_in_start = EXCLUDED.clock_in_start,
    clock_in_end = EXCLUDED.clock_in_end,
    clock_out_start = EXCLUDED.clock_out_start,
    clock_out_end = EXCLUDED.clock_out_end,
    late_grace_minutes = EXCLUDED.late_grace_minutes,
    early_leave_grace_minutes = EXCLUDED.early_leave_grace_minutes,
    status = EXCLUDED.status,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetAttendanceShift :one
SELECT * FROM attendance_shifts
WHERE tenant_id = $1 AND id = $2;

-- name: ListAttendanceShifts :many
SELECT * FROM attendance_shifts
WHERE tenant_id = $1
ORDER BY created_at DESC, id ASC;

-- name: UpsertAttendanceShiftAssignment :one
INSERT INTO attendance_shift_assignments (
    id, tenant_id, employee_id, shift_id, worksite_id, effective_from,
    effective_to, status, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    employee_id = EXCLUDED.employee_id,
    shift_id = EXCLUDED.shift_id,
    worksite_id = EXCLUDED.worksite_id,
    effective_from = EXCLUDED.effective_from,
    effective_to = EXCLUDED.effective_to,
    status = EXCLUDED.status,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetAttendanceShiftAssignment :one
SELECT * FROM attendance_shift_assignments
WHERE tenant_id = $1 AND id = $2;

-- name: ListAttendanceShiftAssignments :many
SELECT * FROM attendance_shift_assignments
WHERE tenant_id = $1
ORDER BY effective_from DESC, created_at DESC, id ASC;

-- name: FindEffectiveAttendanceShiftAssignment :one
SELECT * FROM attendance_shift_assignments
WHERE tenant_id = $1
  AND employee_id = $2
  AND status = 'active'
  AND effective_from <= $3
  AND (effective_to IS NULL OR effective_to >= $3)
ORDER BY effective_from DESC, created_at DESC, id ASC
LIMIT 1;

-- name: UpsertAttendanceClockRecord :one
INSERT INTO attendance_clock_records (
    id, tenant_id, employee_id, shift_assignment_id, shift_id, worksite_id,
    work_date, direction, clocked_at, latitude, longitude, accuracy_meters,
    distance_meters, record_status, rejection_reason, source, device_id,
    device_info, correction_request_id, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
    $13, $14, $15, $16, $17, $18::jsonb, $19, $20
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    employee_id = EXCLUDED.employee_id,
    shift_assignment_id = EXCLUDED.shift_assignment_id,
    shift_id = EXCLUDED.shift_id,
    worksite_id = EXCLUDED.worksite_id,
    work_date = EXCLUDED.work_date,
    direction = EXCLUDED.direction,
    clocked_at = EXCLUDED.clocked_at,
    latitude = EXCLUDED.latitude,
    longitude = EXCLUDED.longitude,
    accuracy_meters = EXCLUDED.accuracy_meters,
    distance_meters = EXCLUDED.distance_meters,
    record_status = EXCLUDED.record_status,
    rejection_reason = EXCLUDED.rejection_reason,
    source = EXCLUDED.source,
    device_id = EXCLUDED.device_id,
    device_info = EXCLUDED.device_info,
    correction_request_id = EXCLUDED.correction_request_id,
    created_at = EXCLUDED.created_at
RETURNING *;

-- name: GetAttendanceClockRecord :one
SELECT * FROM attendance_clock_records
WHERE tenant_id = $1 AND id = $2;

-- name: GetAcceptedAttendanceClockRecord :one
SELECT * FROM attendance_clock_records
WHERE tenant_id = $1
  AND employee_id = $2
  AND work_date = $3
  AND direction = $4
  AND record_status = 'accepted'
LIMIT 1;

-- name: ListAttendanceClockRecords :many
SELECT * FROM attendance_clock_records
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(employee_id)::text = '' OR employee_id = sqlc.arg(employee_id))
  AND (sqlc.arg(from_date)::text = '' OR work_date >= sqlc.arg(from_date))
  AND (sqlc.arg(to_date)::text = '' OR work_date <= sqlc.arg(to_date))
  AND (sqlc.arg(direction)::text = '' OR direction = sqlc.arg(direction))
  AND (sqlc.arg(record_status)::text = '' OR record_status = sqlc.arg(record_status))
  AND (sqlc.arg(source)::text = '' OR source = sqlc.arg(source))
ORDER BY clocked_at DESC, created_at DESC, id ASC;

-- name: UpsertAttendanceCorrectionRequest :one
INSERT INTO attendance_correction_requests (
    id, tenant_id, employee_id, direction, requested_clocked_at, work_date,
    reason, status, form_instance_id, clock_record_id, reviewed_by_account_id,
    review_reason, reviewed_at, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    employee_id = EXCLUDED.employee_id,
    direction = EXCLUDED.direction,
    requested_clocked_at = EXCLUDED.requested_clocked_at,
    work_date = EXCLUDED.work_date,
    reason = EXCLUDED.reason,
    status = EXCLUDED.status,
    form_instance_id = EXCLUDED.form_instance_id,
    clock_record_id = EXCLUDED.clock_record_id,
    reviewed_by_account_id = EXCLUDED.reviewed_by_account_id,
    review_reason = EXCLUDED.review_reason,
    reviewed_at = EXCLUDED.reviewed_at,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetAttendanceCorrectionRequest :one
SELECT * FROM attendance_correction_requests
WHERE tenant_id = $1 AND id = $2;

-- name: ListAttendanceCorrectionRequests :many
SELECT * FROM attendance_correction_requests
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(employee_id)::text = '' OR employee_id = sqlc.arg(employee_id))
  AND (sqlc.arg(from_date)::text = '' OR work_date >= sqlc.arg(from_date))
  AND (sqlc.arg(to_date)::text = '' OR work_date <= sqlc.arg(to_date))
  AND (sqlc.arg(status)::text = '' OR status = sqlc.arg(status))
  AND (sqlc.arg(direction)::text = '' OR direction = sqlc.arg(direction))
ORDER BY created_at DESC, id ASC;

-- name: UpsertKnowledgeArticle :one
INSERT INTO knowledge_articles (
    id, tenant_id, title, content, tags, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    title = EXCLUDED.title,
    content = EXCLUDED.content,
    tags = EXCLUDED.tags,
    created_at = EXCLUDED.created_at
RETURNING *;

-- name: ListKnowledgeArticles :many
SELECT * FROM knowledge_articles
WHERE tenant_id = $1
ORDER BY created_at ASC;

-- name: UpsertPlatformTaskItem :one
INSERT INTO platform_task_items (
    id, tenant_id, account_id, work_date, title, category,
    product, hours, note, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(account_id), sqlc.arg(work_date),
    sqlc.arg(title), sqlc.arg(category), sqlc.arg(product), sqlc.arg(hours),
    sqlc.arg(note), sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (tenant_id, id) DO UPDATE SET
    work_date = EXCLUDED.work_date,
    title = EXCLUDED.title,
    category = EXCLUDED.category,
    product = EXCLUDED.product,
    hours = EXCLUDED.hours,
    note = EXCLUDED.note,
    updated_at = EXCLUDED.updated_at
WHERE platform_task_items.account_id = EXCLUDED.account_id
RETURNING *;

-- name: GetPlatformTaskItem :one
SELECT * FROM platform_task_items
WHERE tenant_id = sqlc.arg(tenant_id)
  AND account_id = sqlc.arg(account_id)
  AND id = sqlc.arg(id);

-- name: ListPlatformTaskItems :many
SELECT * FROM platform_task_items
WHERE tenant_id = sqlc.arg(tenant_id) AND account_id = sqlc.arg(account_id)
ORDER BY work_date DESC, created_at ASC, id ASC;

-- name: DeletePlatformTaskItem :exec
DELETE FROM platform_task_items
WHERE tenant_id = sqlc.arg(tenant_id)
  AND account_id = sqlc.arg(account_id)
  AND id = sqlc.arg(id);

-- name: UpsertPlatformTaskTodo :one
INSERT INTO platform_task_todos (
    id, tenant_id, account_id, text, due_date, status,
    converted_task_item_id, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(account_id), sqlc.arg(text),
    sqlc.arg(due_date), sqlc.arg(status), sqlc.arg(converted_task_item_id),
    sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (tenant_id, id) DO UPDATE SET
    text = EXCLUDED.text,
    due_date = EXCLUDED.due_date,
    status = EXCLUDED.status,
    converted_task_item_id = EXCLUDED.converted_task_item_id,
    updated_at = EXCLUDED.updated_at
WHERE platform_task_todos.account_id = EXCLUDED.account_id
RETURNING *;

-- name: GetPlatformTaskTodo :one
SELECT * FROM platform_task_todos
WHERE tenant_id = sqlc.arg(tenant_id)
  AND account_id = sqlc.arg(account_id)
  AND id = sqlc.arg(id);

-- name: ListPlatformTaskTodos :many
SELECT * FROM platform_task_todos
WHERE tenant_id = sqlc.arg(tenant_id) AND account_id = sqlc.arg(account_id)
ORDER BY CASE WHEN status = 'open' THEN 0 ELSE 1 END, created_at ASC, id ASC;

-- name: DeletePlatformTaskTodo :exec
DELETE FROM platform_task_todos
WHERE tenant_id = sqlc.arg(tenant_id)
  AND account_id = sqlc.arg(account_id)
  AND id = sqlc.arg(id);

-- name: UpsertAgentRun :one
INSERT INTO agent_runs (
    id, tenant_id, account_id, mode, prompt, answer,
    status, reference_items, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    account_id = EXCLUDED.account_id,
    mode = EXCLUDED.mode,
    prompt = EXCLUDED.prompt,
    answer = EXCLUDED.answer,
    status = EXCLUDED.status,
    reference_items = EXCLUDED.reference_items,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetAgentRun :one
SELECT * FROM agent_runs
WHERE tenant_id = $1 AND id = $2;

-- name: ListAgentRuns :many
SELECT * FROM agent_runs
WHERE tenant_id = $1
ORDER BY created_at ASC;

-- name: ListAgentRunsByAccount :many
SELECT * FROM agent_runs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND account_id = sqlc.arg(account_id)
ORDER BY created_at DESC, id ASC;

-- name: CountAgentRuns :one
SELECT count(*) FROM agent_runs
WHERE tenant_id = $1;

-- name: CountAgentRunsByAccount :one
SELECT count(*) FROM agent_runs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND account_id = sqlc.arg(account_id);

-- name: ListAgentRunsPage :many
SELECT * FROM agent_runs
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'created_at_asc' THEN created_at END ASC,
  created_at DESC,
  id ASC
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;

-- name: ListAgentRunsPageByAccount :many
SELECT * FROM agent_runs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND account_id = sqlc.arg(account_id)
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'created_at_asc' THEN created_at END ASC,
  created_at DESC,
  id ASC
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;

-- name: AppendAuditLog :one
INSERT INTO audit_logs (
    id, tenant_id, actor_account_id, action, resource,
    target, result, trace_id, severity, details, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11
)
RETURNING *;

-- name: ListAuditLogs :many
SELECT * FROM audit_logs
WHERE tenant_id = $1
ORDER BY created_at DESC;

-- name: CountAuditLogs :one
SELECT count(*) FROM audit_logs
WHERE tenant_id = $1;

-- name: ListAuditLogsPage :many
SELECT * FROM audit_logs
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'created_at_asc' THEN created_at END ASC,
  created_at DESC,
  id ASC
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;
