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
      coalesce(employees.phone, '') || ' ' ||
      coalesce(employees.account_id, '') || ' ' ||
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
      coalesce(employees.phone, '') || ' ' ||
      coalesce(employees.account_id, '') || ' ' ||
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
      coalesce(employees.phone, '') || ' ' ||
      coalesce(employees.account_id, '') || ' ' ||
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

-- name: ListLeaveRequests :many
SELECT * FROM leave_requests
WHERE tenant_id = $1
ORDER BY created_at ASC;

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

-- name: CountAgentRuns :one
SELECT count(*) FROM agent_runs
WHERE tenant_id = $1;

-- name: ListAgentRunsPage :many
SELECT * FROM agent_runs
WHERE tenant_id = sqlc.arg(tenant_id)
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
