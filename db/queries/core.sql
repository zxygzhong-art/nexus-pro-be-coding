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
-- expected_version = 0 表示盲寫(新建或無條件覆蓋);> 0 時執行樂觀鎖檢查,
-- 版本不符會因 WHERE 不成立而回傳零列(呼叫端轉為 Conflict)。
INSERT INTO accounts (
    id, tenant_id, display_name, email, employee_id, status,
    user_group_ids, direct_permission_set_ids, active_assumable_role_id,
    preferred_locale, version, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(display_name), sqlc.arg(email), sqlc.arg(employee_id), sqlc.arg(status),
    sqlc.arg(user_group_ids), sqlc.arg(direct_permission_set_ids), sqlc.arg(active_assumable_role_id),
    sqlc.arg(preferred_locale), 1, sqlc.arg(created_at)
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    display_name = EXCLUDED.display_name,
    email = EXCLUDED.email,
    employee_id = EXCLUDED.employee_id,
    status = EXCLUDED.status,
    direct_permission_set_ids = EXCLUDED.direct_permission_set_ids,
    active_assumable_role_id = EXCLUDED.active_assumable_role_id,
    preferred_locale = EXCLUDED.preferred_locale,
    version = accounts.version + 1,
    created_at = EXCLUDED.created_at
WHERE sqlc.arg(expected_version)::bigint = 0 OR accounts.version = sqlc.arg(expected_version)::bigint
RETURNING *;

-- name: GetAccount :one
SELECT * FROM accounts
WHERE tenant_id = $1 AND id = $2;

-- name: UpdateAccountPreferredLocale :one
UPDATE accounts
SET preferred_locale = sqlc.arg(preferred_locale),
    version = version + 1
WHERE tenant_id = sqlc.arg(tenant_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: ListAccounts :many
SELECT * FROM accounts
WHERE tenant_id = $1
ORDER BY created_at ASC;

-- name: UpsertUserGroup :one
-- expected_version 語義同 UpsertAccount。
INSERT INTO user_groups (
    id, tenant_id, name, description, member_account_ids,
    permission_set_ids, source_template_key, source_package_version, version, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(name), sqlc.arg(description), sqlc.arg(member_account_ids),
    sqlc.arg(permission_set_ids), sqlc.arg(source_template_key), sqlc.arg(source_package_version), 1, sqlc.arg(created_at)
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    permission_set_ids = EXCLUDED.permission_set_ids,
    source_template_key = EXCLUDED.source_template_key,
    source_package_version = EXCLUDED.source_package_version,
    version = user_groups.version + 1,
    created_at = EXCLUDED.created_at
WHERE sqlc.arg(expected_version)::bigint = 0 OR user_groups.version = sqlc.arg(expected_version)::bigint
RETURNING *;

-- name: GetUserGroup :one
SELECT * FROM user_groups
WHERE tenant_id = $1 AND id = $2;

-- name: ListUserGroups :many
SELECT * FROM user_groups
WHERE tenant_id = $1
ORDER BY created_at ASC;

-- name: DeleteUserGroup :one
DELETE FROM user_groups
WHERE tenant_id = $1 AND id = $2
RETURNING *;

-- name: UpsertGroupMembership :one
INSERT INTO authz_group_memberships (
    id, tenant_id, user_group_id, account_id, valid_from, valid_until,
    source, approval_instance_id, created_by, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(user_group_id), sqlc.arg(account_id),
    sqlc.arg(valid_from), sqlc.narg(valid_until), sqlc.arg(source),
    sqlc.arg(approval_instance_id), sqlc.arg(created_by), sqlc.arg(created_at)
)
ON CONFLICT (id) DO UPDATE SET
    valid_from = EXCLUDED.valid_from,
    valid_until = EXCLUDED.valid_until,
    source = EXCLUDED.source,
    approval_instance_id = EXCLUDED.approval_instance_id,
    created_by = EXCLUDED.created_by
RETURNING *;

-- name: CloseGroupMembership :one
UPDATE authz_group_memberships
SET valid_until = sqlc.arg(valid_until)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND user_group_id = sqlc.arg(user_group_id)
  AND account_id = sqlc.arg(account_id)
  AND valid_from <= sqlc.arg(valid_until)
  AND (valid_until IS NULL OR valid_until > sqlc.arg(valid_until))
RETURNING *;

-- name: DeleteGroupMembership :one
DELETE FROM authz_group_memberships
WHERE tenant_id = $1 AND user_group_id = $2 AND account_id = $3
RETURNING *;

-- name: GetGroupMembership :one
SELECT * FROM authz_group_memberships
WHERE tenant_id = $1 AND user_group_id = $2 AND account_id = $3
ORDER BY valid_from DESC, created_at DESC
LIMIT 1;

-- name: ListGroupMembershipsForGroup :many
SELECT * FROM authz_group_memberships
WHERE tenant_id = $1 AND user_group_id = $2
ORDER BY created_at ASC;

-- name: ListActiveGroupMembershipsForAccount :many
SELECT * FROM authz_group_memberships
WHERE tenant_id = $1
  AND account_id = $2
  AND valid_from <= $3
  AND (valid_until IS NULL OR valid_until > $3)
ORDER BY created_at ASC;

-- name: UpsertPermissionSet :one
INSERT INTO permission_sets (
    id, tenant_id, name, description, permissions, source_template_key, source_package_version, created_at
) VALUES (
    $1, $2, $3, $4, $5::jsonb, $6, $7, $8
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    permissions = EXCLUDED.permissions,
    source_template_key = EXCLUDED.source_template_key,
    source_package_version = EXCLUDED.source_package_version,
    created_at = EXCLUDED.created_at
RETURNING *;

-- name: GetPermissionSet :one
SELECT * FROM permission_sets
WHERE tenant_id = $1 AND id = $2;

-- name: ListPermissionSets :many
SELECT * FROM permission_sets
WHERE tenant_id = $1
ORDER BY created_at ASC;

-- name: DeletePermissionSet :one
DELETE FROM permission_sets
WHERE tenant_id = $1 AND id = $2
RETURNING *;

-- name: UpsertAssumableRole :one
INSERT INTO assumable_roles (
    id, tenant_id, name, description, permission_set_ids, trusted,
    trust_policy, permission_boundary, session_duration_seconds, source_template_key, source_package_version, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, $9, $10, $11, $12
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
    source_template_key = EXCLUDED.source_template_key,
    source_package_version = EXCLUDED.source_package_version,
    created_at = EXCLUDED.created_at
RETURNING *;

-- name: GetAssumableRole :one
SELECT * FROM assumable_roles
WHERE tenant_id = $1 AND id = $2;

-- name: ListAssumableRoles :many
SELECT * FROM assumable_roles
WHERE tenant_id = $1
ORDER BY created_at ASC;

-- name: DeleteAssumableRole :one
DELETE FROM assumable_roles
WHERE tenant_id = $1 AND id = $2
RETURNING *;

-- name: UpsertOrgUnit :one
INSERT INTO org_units (
    id, tenant_id, code, name, name_en, parent_id, path, source, closed, manager_position_id, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
ON CONFLICT (id) DO UPDATE SET
    code = EXCLUDED.code,
    name = EXCLUDED.name,
    name_en = EXCLUDED.name_en,
    parent_id = EXCLUDED.parent_id,
    path = EXCLUDED.path,
    source = EXCLUDED.source,
    closed = EXCLUDED.closed,
    manager_position_id = EXCLUDED.manager_position_id,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at
WHERE org_units.tenant_id = EXCLUDED.tenant_id
RETURNING *;

-- name: GetOrgUnit :one
SELECT * FROM org_units
WHERE tenant_id = $1 AND id = $2;

-- name: UpdateOrgUnitOrgChartVisibility :exec
UPDATE org_units
SET show_in_org_chart = sqlc.arg(show_in_org_chart),
    updated_at = sqlc.arg(updated_at)
WHERE tenant_id = sqlc.arg(tenant_id) AND id = sqlc.arg(id);

-- name: ListOrgUnits :many
SELECT * FROM org_units
WHERE tenant_id = $1
ORDER BY closed ASC, code ASC, name ASC, created_at ASC, id ASC;

-- name: UpsertEmployee :one
INSERT INTO employees (
    id, tenant_id, employee_no, name, company_email, personal_email, phone,
    org_unit_id, account_id, manager_employee_id, position_id, position, category, status, employment_status,
    hire_date, resign_date, basic_info, employment_info, education_military_info,
    contact_info, insurance_info, internal_experiences, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
    $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25
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
    position_id = EXCLUDED.position_id,
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

-- name: UpdateEmployeeOrgChartVisibility :exec
UPDATE employees
SET show_in_org_chart = sqlc.arg(show_in_org_chart),
    updated_at = sqlc.arg(updated_at)
WHERE tenant_id = sqlc.arg(tenant_id) AND id = sqlc.arg(id);

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
ORDER BY
  CASE coalesce(nullif(employment_status, ''), status)
    WHEN 'active' THEN 0
    WHEN 'probation' THEN 1
    WHEN 'leave_suspended' THEN 2
    WHEN 'onboarding' THEN 3
    WHEN 'resigned' THEN 4
    WHEN 'deleted' THEN 5
    ELSE 6
  END ASC,
  created_at ASC,
  id ASC;

-- name: ListEmployeesFiltered :many
SELECT employees.* FROM employees
LEFT JOIN accounts
  ON accounts.tenant_id = employees.tenant_id
 AND accounts.id = employees.account_id
WHERE employees.tenant_id = sqlc.arg(tenant_id)
	AND NOT sqlc.arg(scope_deny_all)::boolean
	AND (
		(
			NOT sqlc.arg(scope_match_any)::boolean
			AND (coalesce(cardinality(sqlc.arg(scope_employee_ids)::text[]), 0) = 0 OR employees.id = ANY(sqlc.arg(scope_employee_ids)::text[]))
			AND (coalesce(cardinality(sqlc.arg(scope_org_unit_ids)::text[]), 0) = 0 OR employees.org_unit_id = ANY(sqlc.arg(scope_org_unit_ids)::text[]))
		)
		OR (
			sqlc.arg(scope_match_any)::boolean
			AND (
				(coalesce(cardinality(sqlc.arg(scope_employee_ids)::text[]), 0) > 0 AND employees.id = ANY(sqlc.arg(scope_employee_ids)::text[]))
				OR (coalesce(cardinality(sqlc.arg(scope_org_unit_ids)::text[]), 0) > 0 AND employees.org_unit_id = ANY(sqlc.arg(scope_org_unit_ids)::text[]))
			)
		)
	)
	AND (coalesce(cardinality(sqlc.arg(scope_statuses)::text[]), 0) = 0 OR coalesce(nullif(employees.employment_status, ''), employees.status) = ANY(sqlc.arg(scope_statuses)::text[]))
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
  AND (
    sqlc.arg(present_from)::text = ''
    OR coalesce(nullif(employees.employment_status, ''), employees.status) <> 'resigned'
    OR employees.resign_date IS NOT NULL
  )
  AND (NULLIF(sqlc.arg(present_to)::text, '') IS NULL OR employees.hire_date IS NULL OR employees.hire_date < NULLIF(sqlc.arg(present_to)::text, '')::timestamptz)
  AND (NULLIF(sqlc.arg(present_from)::text, '') IS NULL OR employees.resign_date IS NULL OR employees.resign_date >= NULLIF(sqlc.arg(present_from)::text, '')::timestamptz)
ORDER BY
  CASE WHEN sqlc.arg(sort)::text <> 'attendance_asc' THEN CASE coalesce(nullif(employees.employment_status, ''), employees.status)
    WHEN 'active' THEN 0
    WHEN 'probation' THEN 1
    WHEN 'leave_suspended' THEN 2
    WHEN 'onboarding' THEN 3
    WHEN 'resigned' THEN 4
    WHEN 'deleted' THEN 5
    ELSE 6
  END END ASC,
  CASE WHEN sqlc.arg(sort)::text = 'attendance_asc' THEN lower(coalesce(nullif(employees.employee_no, ''), employees.id)) END ASC,
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
  AND NOT sqlc.arg(scope_deny_all)::boolean
  AND (
    (
      NOT sqlc.arg(scope_match_any)::boolean
      AND (coalesce(cardinality(sqlc.arg(scope_employee_ids)::text[]), 0) = 0 OR employees.id = ANY(sqlc.arg(scope_employee_ids)::text[]))
      AND (coalesce(cardinality(sqlc.arg(scope_org_unit_ids)::text[]), 0) = 0 OR employees.org_unit_id = ANY(sqlc.arg(scope_org_unit_ids)::text[]))
    )
    OR (
      sqlc.arg(scope_match_any)::boolean
      AND (
        (coalesce(cardinality(sqlc.arg(scope_employee_ids)::text[]), 0) > 0 AND employees.id = ANY(sqlc.arg(scope_employee_ids)::text[]))
        OR (coalesce(cardinality(sqlc.arg(scope_org_unit_ids)::text[]), 0) > 0 AND employees.org_unit_id = ANY(sqlc.arg(scope_org_unit_ids)::text[]))
      )
    )
  )
  AND (coalesce(cardinality(sqlc.arg(scope_statuses)::text[]), 0) = 0 OR coalesce(nullif(employees.employment_status, ''), employees.status) = ANY(sqlc.arg(scope_statuses)::text[]))
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
  AND (
    sqlc.arg(present_from)::text = ''
    OR coalesce(nullif(employees.employment_status, ''), employees.status) <> 'resigned'
    OR employees.resign_date IS NOT NULL
  )
  AND (NULLIF(sqlc.arg(present_to)::text, '') IS NULL OR employees.hire_date IS NULL OR employees.hire_date < NULLIF(sqlc.arg(present_to)::text, '')::timestamptz)
  AND (NULLIF(sqlc.arg(present_from)::text, '') IS NULL OR employees.resign_date IS NULL OR employees.resign_date >= NULLIF(sqlc.arg(present_from)::text, '')::timestamptz);

-- name: ListEmployeesFilteredPage :many
SELECT employees.* FROM employees
LEFT JOIN accounts
  ON accounts.tenant_id = employees.tenant_id
 AND accounts.id = employees.account_id
WHERE employees.tenant_id = sqlc.arg(tenant_id)
  AND NOT sqlc.arg(scope_deny_all)::boolean
  AND (
    (
      NOT sqlc.arg(scope_match_any)::boolean
      AND (coalesce(cardinality(sqlc.arg(scope_employee_ids)::text[]), 0) = 0 OR employees.id = ANY(sqlc.arg(scope_employee_ids)::text[]))
      AND (coalesce(cardinality(sqlc.arg(scope_org_unit_ids)::text[]), 0) = 0 OR employees.org_unit_id = ANY(sqlc.arg(scope_org_unit_ids)::text[]))
    )
    OR (
      sqlc.arg(scope_match_any)::boolean
      AND (
        (coalesce(cardinality(sqlc.arg(scope_employee_ids)::text[]), 0) > 0 AND employees.id = ANY(sqlc.arg(scope_employee_ids)::text[]))
        OR (coalesce(cardinality(sqlc.arg(scope_org_unit_ids)::text[]), 0) > 0 AND employees.org_unit_id = ANY(sqlc.arg(scope_org_unit_ids)::text[]))
      )
    )
  )
  AND (coalesce(cardinality(sqlc.arg(scope_statuses)::text[]), 0) = 0 OR coalesce(nullif(employees.employment_status, ''), employees.status) = ANY(sqlc.arg(scope_statuses)::text[]))
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
  AND (
    sqlc.arg(present_from)::text = ''
    OR coalesce(nullif(employees.employment_status, ''), employees.status) <> 'resigned'
    OR employees.resign_date IS NOT NULL
  )
  AND (NULLIF(sqlc.arg(present_to)::text, '') IS NULL OR employees.hire_date IS NULL OR employees.hire_date < NULLIF(sqlc.arg(present_to)::text, '')::timestamptz)
  AND (NULLIF(sqlc.arg(present_from)::text, '') IS NULL OR employees.resign_date IS NULL OR employees.resign_date >= NULLIF(sqlc.arg(present_from)::text, '')::timestamptz)
ORDER BY
  CASE WHEN sqlc.arg(sort)::text <> 'attendance_asc' THEN CASE coalesce(nullif(employees.employment_status, ''), employees.status)
    WHEN 'active' THEN 0
    WHEN 'probation' THEN 1
    WHEN 'leave_suspended' THEN 2
    WHEN 'onboarding' THEN 3
    WHEN 'resigned' THEN 4
    WHEN 'deleted' THEN 5
    ELSE 6
  END END ASC,
  CASE WHEN sqlc.arg(sort)::text = 'attendance_asc' THEN lower(coalesce(nullif(employees.employee_no, ''), employees.id)) END ASC,
  CASE WHEN sqlc.arg(sort)::text = 'created_at_desc' THEN employees.created_at END DESC,
  CASE WHEN sqlc.arg(sort)::text = 'hire_date_desc' THEN employees.hire_date END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort)::text = 'hire_date_asc' THEN employees.hire_date END ASC NULLS LAST,
  employees.created_at ASC,
  employees.id ASC
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;

-- name: AppendOutboxEvent :one
INSERT INTO outbox_events (
    id, tenant_id, event_type, aggregate_type, aggregate_id,
    payload, payload_version, idempotency_key, status, retry_count,
    attempt_count, max_attempts, last_error, next_attempt_at,
    claim_owner, claim_token, claim_expires_at, last_attempt_at,
    created_at, updated_at, processed_at, dead_lettered_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(event_type), sqlc.arg(aggregate_type), sqlc.arg(aggregate_id),
    sqlc.arg(payload)::jsonb, COALESCE(NULLIF(sqlc.arg(payload_version)::int, 0), 1), sqlc.arg(idempotency_key),
    sqlc.arg(status), sqlc.arg(retry_count), sqlc.arg(attempt_count), COALESCE(sqlc.narg(max_attempts)::int, 5),
    sqlc.arg(last_error), COALESCE(sqlc.narg(next_attempt_at)::timestamptz, sqlc.arg(created_at)::timestamptz),
    sqlc.arg(claim_owner), sqlc.arg(claim_token), sqlc.narg(claim_expires_at), sqlc.narg(last_attempt_at),
    sqlc.arg(created_at), COALESCE(sqlc.narg(updated_at)::timestamptz, sqlc.arg(created_at)::timestamptz),
    sqlc.narg(processed_at), sqlc.narg(dead_lettered_at)
)
ON CONFLICT (tenant_id, event_type, idempotency_key) WHERE idempotency_key <> ''
DO UPDATE SET updated_at = outbox_events.updated_at
RETURNING
    id, tenant_id, event_type, aggregate_type, aggregate_id, payload,
    payload_version, idempotency_key, status, retry_count, attempt_count,
    max_attempts, last_error, next_attempt_at, claim_owner, claim_token,
    claim_expires_at, last_attempt_at, created_at, updated_at, processed_at,
    dead_lettered_at;

-- name: ListOutboxEvents :many
SELECT
    id, tenant_id, event_type, aggregate_type, aggregate_id, payload,
    payload_version, idempotency_key, status, retry_count, attempt_count,
    max_attempts, last_error, next_attempt_at, claim_owner, claim_token,
    claim_expires_at, last_attempt_at, created_at, updated_at, processed_at,
    dead_lettered_at
FROM outbox_events
WHERE tenant_id = $1
ORDER BY created_at ASC, id ASC;

-- name: GetOutboxEventByID :one
SELECT
    id, tenant_id, event_type, aggregate_type, aggregate_id, payload,
    payload_version, idempotency_key, status, retry_count, attempt_count,
    max_attempts, last_error, next_attempt_at, claim_owner, claim_token,
    claim_expires_at, last_attempt_at, created_at, updated_at, processed_at,
    dead_lettered_at
FROM outbox_events
WHERE tenant_id = $1 AND id = $2;

-- name: CountOutboxEventsFiltered :one
SELECT count(*) FROM outbox_events
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(status)::text = '' OR status = sqlc.arg(status))
  AND (sqlc.arg(event_type)::text = '' OR event_type = sqlc.arg(event_type))
  AND (
    sqlc.arg(last_error)::text = ''
    OR lower(outbox_events.last_error) LIKE '%' || lower(sqlc.arg(last_error)::text) || '%'
  )
  AND (NOT sqlc.arg(has_retry_count)::bool OR retry_count = sqlc.arg(retry_count)::int)
  AND (NOT sqlc.arg(filter_has_error)::bool OR (btrim(outbox_events.last_error) <> '') = sqlc.arg(has_error)::bool);

-- name: ListOutboxEventPage :many
SELECT
    id, tenant_id, event_type, aggregate_type, aggregate_id, payload,
    payload_version, idempotency_key, status, retry_count, attempt_count,
    max_attempts, last_error, next_attempt_at, claim_owner, claim_token,
    claim_expires_at, last_attempt_at, created_at, updated_at, processed_at,
    dead_lettered_at
FROM outbox_events
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(status)::text = '' OR status = sqlc.arg(status))
  AND (sqlc.arg(event_type)::text = '' OR event_type = sqlc.arg(event_type))
  AND (
    sqlc.arg(last_error)::text = ''
    OR lower(outbox_events.last_error) LIKE '%' || lower(sqlc.arg(last_error)::text) || '%'
  )
  AND (NOT sqlc.arg(has_retry_count)::bool OR retry_count = sqlc.arg(retry_count)::int)
  AND (NOT sqlc.arg(filter_has_error)::bool OR (btrim(outbox_events.last_error) <> '') = sqlc.arg(has_error)::bool)
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'created_at_asc' THEN created_at END ASC,
  created_at DESC,
  id ASC
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;

-- name: ClaimOutboxEvents :many
-- Atomically claim due rows and recover expired leases. The per-row token fences stale workers.
WITH candidates AS (
    SELECT candidate.id
    FROM outbox_events AS candidate
    WHERE candidate.tenant_id = sqlc.arg(tenant_id)
      AND (
        (candidate.status IN ('pending', 'failed') AND candidate.next_attempt_at <= sqlc.arg(claimed_at))
        OR (candidate.status = 'processing' AND candidate.claim_expires_at <= sqlc.arg(claimed_at))
      )
      AND (candidate.max_attempts = 0 OR candidate.attempt_count < candidate.max_attempts)
    ORDER BY candidate.next_attempt_at ASC, candidate.created_at ASC, candidate.id ASC
    FOR UPDATE SKIP LOCKED
    LIMIT sqlc.arg(batch_limit)
)
UPDATE outbox_events AS claimed
SET status = 'processing',
    attempt_count = claimed.attempt_count + 1,
    last_error = '',
    claim_owner = sqlc.arg(claim_owner),
    claim_token = sqlc.arg(claim_token)::text || ':' || claimed.id,
    claim_expires_at = sqlc.arg(lease_until),
    last_attempt_at = sqlc.arg(claimed_at),
    updated_at = sqlc.arg(claimed_at),
    processed_at = NULL,
    dead_lettered_at = NULL
FROM candidates
WHERE claimed.tenant_id = sqlc.arg(tenant_id)
  AND claimed.id = candidates.id
RETURNING
    claimed.id, claimed.tenant_id, claimed.event_type, claimed.aggregate_type,
    claimed.aggregate_id, claimed.payload, claimed.payload_version,
    claimed.idempotency_key, claimed.status, claimed.retry_count,
    claimed.attempt_count, claimed.max_attempts, claimed.last_error,
    claimed.next_attempt_at, claimed.claim_owner, claimed.claim_token,
    claimed.claim_expires_at, claimed.last_attempt_at, claimed.created_at,
    claimed.updated_at, claimed.processed_at, claimed.dead_lettered_at;

-- name: FinalizeOutboxEvent :one
UPDATE outbox_events
SET status = sqlc.arg(status),
    retry_count = sqlc.arg(retry_count),
    attempt_count = sqlc.arg(attempt_count),
    last_error = sqlc.arg(last_error),
    next_attempt_at = sqlc.arg(next_attempt_at),
    claim_owner = '',
    claim_token = '',
    claim_expires_at = NULL,
    updated_at = sqlc.arg(updated_at),
    processed_at = sqlc.narg(processed_at),
    dead_lettered_at = sqlc.narg(dead_lettered_at)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND status = 'processing'
  AND claim_token = sqlc.arg(claim_token)
RETURNING
    id, tenant_id, event_type, aggregate_type, aggregate_id, payload,
    payload_version, idempotency_key, status, retry_count, attempt_count,
    max_attempts, last_error, next_attempt_at, claim_owner, claim_token,
    claim_expires_at, last_attempt_at, created_at, updated_at, processed_at,
    dead_lettered_at;

-- name: RetryOutboxEvent :one
UPDATE outbox_events
SET status = 'pending',
    retry_count = 0,
    attempt_count = 0,
    last_error = '',
    next_attempt_at = sqlc.arg(retried_at),
    claim_owner = '',
    claim_token = '',
    claim_expires_at = NULL,
    last_attempt_at = NULL,
    updated_at = sqlc.arg(retried_at),
    processed_at = NULL,
    dead_lettered_at = NULL
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND status IN ('failed', 'parked', 'dead_lettered')
RETURNING
    id, tenant_id, event_type, aggregate_type, aggregate_id, payload,
    payload_version, idempotency_key, status, retry_count, attempt_count,
    max_attempts, last_error, next_attempt_at, claim_owner, claim_token,
    claim_expires_at, last_attempt_at, created_at, updated_at, processed_at,
    dead_lettered_at;

-- name: DeleteSucceededOutboxEventsBefore :execrows
DELETE FROM outbox_events
WHERE tenant_id = sqlc.arg(tenant_id)
  AND status = 'succeeded'
  AND created_at < sqlc.arg(before);

-- name: InsertAttendancePolicyVersion :one
INSERT INTO attendance_policy_versions (
    tenant_id, version, work_time, effective_from,
    published_by_account_id, published_at
) VALUES (
    sqlc.arg(tenant_id), sqlc.arg(version), sqlc.arg(work_time)::jsonb,
    sqlc.arg(effective_from), sqlc.arg(published_by_account_id),
    sqlc.arg(published_at)
)
RETURNING *;

-- name: GetAttendancePolicy :one
SELECT * FROM attendance_policy_versions
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY version DESC
LIMIT 1;

-- name: GetAttendancePolicyAsOf :one
SELECT * FROM attendance_policy_versions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND effective_from <= sqlc.arg(as_of)::timestamptz
ORDER BY effective_from DESC, version DESC
LIMIT 1;

-- name: UpsertLeaveBalance :exec
INSERT INTO leave_balances (
    id, tenant_id, employee_id, leave_type_id, entitlement_year,
    granted_minutes, used_minutes, remaining_minutes, source, last_synced_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(employee_id), sqlc.arg(leave_type_id), sqlc.arg(entitlement_year),
    sqlc.arg(granted_minutes), sqlc.arg(used_minutes), sqlc.arg(remaining_minutes), sqlc.arg(source),
    sqlc.narg(last_synced_at), sqlc.arg(updated_at)
)
ON CONFLICT (tenant_id, employee_id, leave_type_id, entitlement_year) DO UPDATE SET
    remaining_minutes = EXCLUDED.remaining_minutes,
    granted_minutes = EXCLUDED.granted_minutes,
    used_minutes = EXCLUDED.used_minutes,
    source = EXCLUDED.source,
    last_synced_at = EXCLUDED.last_synced_at,
    updated_at = EXCLUDED.updated_at;

-- name: EnsureLocalLeaveBalanceAnchor :one
INSERT INTO leave_balances (
    id, tenant_id, employee_id, leave_type_id, entitlement_year,
    granted_minutes, used_minutes, remaining_minutes, source, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(employee_id), sqlc.arg(leave_type_id), sqlc.arg(entitlement_year),
    0, 0, 0, 'nexus', sqlc.arg(updated_at)
)
ON CONFLICT (tenant_id, employee_id, leave_type_id, entitlement_year) DO UPDATE SET
    id = leave_balances.id
RETURNING *;

-- name: UpsertLeaveType :one
INSERT INTO leave_types (
    id, tenant_id, code, kind, parent_id, parent_code, name, name_zh, name_en, category, source_of_truth, status,
    requires_balance, max_balance_minutes, unit, display_order, raw_payload,
    last_synced_at, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(code), sqlc.arg(kind), sqlc.narg(parent_id), sqlc.narg(parent_code),
    sqlc.arg(name), sqlc.arg(name_zh), sqlc.arg(name_en),
    sqlc.arg(category), sqlc.arg(source_of_truth), sqlc.arg(status),
    sqlc.arg(requires_balance), sqlc.arg(max_balance_minutes), sqlc.arg(unit), sqlc.arg(display_order),
    sqlc.arg(raw_payload)::jsonb, sqlc.narg(last_synced_at), sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (tenant_id, id) DO UPDATE SET
    code = EXCLUDED.code,
    kind = EXCLUDED.kind,
    parent_id = EXCLUDED.parent_id,
    parent_code = EXCLUDED.parent_code,
    name = EXCLUDED.name,
    name_zh = EXCLUDED.name_zh,
    name_en = EXCLUDED.name_en,
    category = EXCLUDED.category,
    source_of_truth = EXCLUDED.source_of_truth,
    status = EXCLUDED.status,
    requires_balance = EXCLUDED.requires_balance,
    max_balance_minutes = EXCLUDED.max_balance_minutes,
    unit = EXCLUDED.unit,
    display_order = EXCLUDED.display_order,
    raw_payload = EXCLUDED.raw_payload,
    last_synced_at = EXCLUDED.last_synced_at,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: DeactivateMissingLeaveTypes :execrows
UPDATE leave_types
SET status = 'inactive', updated_at = sqlc.arg(updated_at)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND source_of_truth = 'ehrms'
  AND NOT (id = ANY (sqlc.arg(active_ids)::text[]));

-- name: AppendLeaveBalanceEntry :one
INSERT INTO leave_balance_entries (
    id, tenant_id, balance_id, leave_record_id, employee_id, leave_type_id,
    entitlement_year, entry_type, amount_minutes, idempotency_key, occurred_at, created_at
) SELECT
    sqlc.arg(id), balance.tenant_id, balance.id, record.id, balance.employee_id, balance.leave_type_id,
    balance.entitlement_year, sqlc.arg(entry_type), sqlc.arg(amount_minutes), sqlc.arg(idempotency_key),
    sqlc.arg(occurred_at), sqlc.arg(created_at)
FROM leave_balances balance
LEFT JOIN leave_records record
  ON record.tenant_id = balance.tenant_id
 AND record.id = sqlc.narg(leave_record_id)
 AND record.balance_id = balance.id
WHERE balance.tenant_id = sqlc.arg(tenant_id)
  AND balance.id = sqlc.arg(balance_id)
  AND (sqlc.narg(leave_record_id)::text IS NULL OR record.id IS NOT NULL)
ON CONFLICT (tenant_id, idempotency_key) DO NOTHING
RETURNING *;

-- name: AppendStandaloneLeaveBalanceEntry :one
INSERT INTO leave_balance_entries (
    id, tenant_id, balance_id, employee_id, leave_type_id, entitlement_year,
    entry_type, amount_minutes, idempotency_key, occurred_at, created_at
) SELECT
    sqlc.arg(id), balance.tenant_id, balance.id, balance.employee_id, balance.leave_type_id, balance.entitlement_year,
    sqlc.arg(entry_type), sqlc.arg(amount_minutes), sqlc.arg(idempotency_key),
    sqlc.arg(occurred_at), sqlc.arg(created_at)
FROM leave_balances balance
WHERE balance.tenant_id = sqlc.arg(tenant_id)
  AND balance.id = sqlc.arg(balance_id)
ON CONFLICT (tenant_id, idempotency_key) DO NOTHING
RETURNING *;

-- name: ListLeaveBalanceEntries :many
SELECT * FROM leave_balance_entries
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY occurred_at ASC, id ASC;

-- name: ListLeaveBalanceEntriesByBalance :many
SELECT * FROM leave_balance_entries
WHERE tenant_id = sqlc.arg(tenant_id)
  AND balance_id = sqlc.arg(balance_id)
ORDER BY occurred_at ASC, id ASC;

-- name: GetLeaveBalance :one
SELECT sqlc.embed(balance), leave_type.code AS leave_type
FROM leave_balances balance
JOIN leave_types leave_type
  ON leave_type.tenant_id = balance.tenant_id
 AND leave_type.id = balance.leave_type_id
WHERE balance.tenant_id = $1 AND balance.id = $2;

-- name: GetLeaveBalanceForOverlay :one
SELECT sqlc.embed(balance), leave_type.code AS leave_type
FROM leave_balances balance
JOIN leave_types leave_type
  ON leave_type.tenant_id = balance.tenant_id
 AND leave_type.id = balance.leave_type_id
WHERE balance.tenant_id = sqlc.arg(tenant_id)
  AND balance.employee_id = sqlc.arg(employee_id)
  AND balance.leave_type_id = sqlc.arg(leave_type_id)
  AND balance.entitlement_year = EXTRACT(
      YEAR FROM sqlc.arg(as_of)::timestamptz AT TIME ZONE 'Asia/Shanghai'
  )::integer
LIMIT 1
FOR UPDATE OF balance;

-- name: ListLeaveBalancesForOverlay :many
SELECT sqlc.embed(balance), leave_type.code AS leave_type
FROM leave_balances balance
JOIN leave_types leave_type
  ON leave_type.tenant_id = balance.tenant_id
 AND leave_type.id = balance.leave_type_id
WHERE balance.tenant_id = sqlc.arg(tenant_id)
  AND balance.employee_id = sqlc.arg(employee_id)
  AND balance.leave_type_id = sqlc.arg(leave_type_id)
  AND balance.entitlement_year = EXTRACT(
      YEAR FROM sqlc.arg(as_of)::timestamptz AT TIME ZONE 'Asia/Shanghai'
  )::integer
ORDER BY balance.id
FOR UPDATE OF balance;

-- name: ListLeaveBalances :many
SELECT sqlc.embed(balance), leave_type.code AS leave_type
FROM leave_balances balance
JOIN leave_types leave_type
  ON leave_type.tenant_id = balance.tenant_id
 AND leave_type.id = balance.leave_type_id
WHERE balance.tenant_id = $1
ORDER BY balance.updated_at ASC;

-- name: UpsertFormDefinitionDraft :one
INSERT INTO form_definition_drafts (
    id, tenant_id, owner_account_id, base_template_id, schema_version, authoring_schema, compiled_schema,
    status, revision, source, agent_id, agent_run_id, agent_session_id, tool_call_id,
    validation_result, submitted_at, published_template_id, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(owner_account_id), sqlc.arg(base_template_id), sqlc.arg(schema_version),
    sqlc.arg(authoring_schema)::jsonb, sqlc.arg(compiled_schema)::jsonb, sqlc.arg(status), sqlc.arg(revision),
    sqlc.arg(source), sqlc.arg(agent_id), sqlc.arg(agent_run_id), sqlc.arg(agent_session_id), sqlc.arg(tool_call_id),
    sqlc.arg(validation_result)::jsonb, sqlc.narg(submitted_at), sqlc.arg(published_template_id),
    sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (id) DO UPDATE SET
    owner_account_id = EXCLUDED.owner_account_id,
    base_template_id = EXCLUDED.base_template_id,
    schema_version = EXCLUDED.schema_version,
    authoring_schema = EXCLUDED.authoring_schema,
    compiled_schema = EXCLUDED.compiled_schema,
    status = EXCLUDED.status,
    revision = form_definition_drafts.revision + 1,
    source = EXCLUDED.source,
    agent_id = EXCLUDED.agent_id,
    agent_run_id = EXCLUDED.agent_run_id,
    agent_session_id = EXCLUDED.agent_session_id,
    tool_call_id = EXCLUDED.tool_call_id,
    validation_result = EXCLUDED.validation_result,
    submitted_at = EXCLUDED.submitted_at,
    published_template_id = EXCLUDED.published_template_id,
    updated_at = EXCLUDED.updated_at
WHERE form_definition_drafts.tenant_id = EXCLUDED.tenant_id
  AND form_definition_drafts.revision = sqlc.arg(revision)
RETURNING *;

-- name: GetFormDefinitionDraft :one
SELECT * FROM form_definition_drafts WHERE tenant_id = $1 AND id = $2;

-- name: GetFormDefinitionDraftByAgentCall :one
SELECT * FROM form_definition_drafts
WHERE tenant_id = $1 AND agent_run_id = $2 AND tool_call_id = $3;

-- name: ListFormDefinitionDrafts :many
SELECT * FROM form_definition_drafts
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(owner_account_id)::text = '' OR owner_account_id = sqlc.arg(owner_account_id)::text)
  AND (sqlc.arg(status)::text = '' OR status = sqlc.arg(status)::text)
ORDER BY updated_at DESC;

-- name: UpsertFormTemplate :one
INSERT INTO form_templates (
    id, tenant_id, key, name, description, schema, status, current_version, published_version, created_at, updated_at, deleted_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(key), sqlc.arg(name), sqlc.arg(description), sqlc.arg(schema)::jsonb,
    sqlc.arg(status), sqlc.arg(current_version), sqlc.arg(published_version), sqlc.arg(created_at), sqlc.arg(updated_at), sqlc.narg(deleted_at)
)
ON CONFLICT (id) DO UPDATE SET
    key = EXCLUDED.key,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    schema = EXCLUDED.schema,
    status = EXCLUDED.status,
    current_version = EXCLUDED.current_version,
    published_version = EXCLUDED.published_version,
    updated_at = EXCLUDED.updated_at,
    deleted_at = EXCLUDED.deleted_at
WHERE form_templates.tenant_id = EXCLUDED.tenant_id
RETURNING *;

-- name: GetFormTemplate :one
SELECT * FROM form_templates
WHERE tenant_id = $1 AND id = $2;

-- name: GetFormTemplateForUpdate :one
SELECT * FROM form_templates
WHERE tenant_id = $1 AND id = $2
FOR UPDATE;

-- name: GetFormTemplateByKey :one
SELECT * FROM form_templates
WHERE tenant_id = $1 AND key = $2;

-- name: GetFormTemplateByKeyForUpdate :one
SELECT * FROM form_templates
WHERE tenant_id = $1 AND key = $2
FOR UPDATE;

-- name: ListFormTemplates :many
SELECT * FROM form_templates
WHERE tenant_id = $1
ORDER BY created_at ASC;

-- name: InsertFormTemplateVersion :exec
INSERT INTO form_template_versions (
    id, tenant_id, template_id, version, schema, status, created_at, published_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(template_id), sqlc.arg(version), sqlc.arg(schema)::jsonb,
    sqlc.arg(status), sqlc.arg(created_at), sqlc.narg(published_at)
)
ON CONFLICT (tenant_id, template_id, version) DO UPDATE SET
    status = EXCLUDED.status,
    published_at = COALESCE(form_template_versions.published_at, EXCLUDED.published_at);

-- name: GetFormTemplateVersion :one
SELECT * FROM form_template_versions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- name: GetFormTemplateVersionByNumber :one
SELECT * FROM form_template_versions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND template_id = sqlc.arg(template_id)
  AND version = sqlc.arg(version);

-- name: UpsertFormInstance :one
-- expected_version 語義同 UpsertAccount。
INSERT INTO form_instances (
    id, tenant_id, template_id, template_version_id, applicant_account_id, status,
    payload, submitted_at, approved_by, current_run_id, version, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(template_id), sqlc.arg(template_version_id), sqlc.arg(applicant_account_id), sqlc.arg(status),
    sqlc.arg(payload)::jsonb, sqlc.arg(submitted_at), sqlc.arg(approved_by), sqlc.arg(current_run_id), 1, sqlc.arg(updated_at)
)
ON CONFLICT (id) DO UPDATE SET
    template_id = EXCLUDED.template_id,
    template_version_id = EXCLUDED.template_version_id,
    applicant_account_id = EXCLUDED.applicant_account_id,
    status = EXCLUDED.status,
    payload = EXCLUDED.payload,
    submitted_at = EXCLUDED.submitted_at,
    approved_by = EXCLUDED.approved_by,
    current_run_id = EXCLUDED.current_run_id,
    version = form_instances.version + 1,
    updated_at = EXCLUDED.updated_at
WHERE form_instances.tenant_id = EXCLUDED.tenant_id
  AND (sqlc.arg(expected_version)::bigint = 0 OR form_instances.version = sqlc.arg(expected_version)::bigint)
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
  AND (sqlc.arg(applicant_account_id)::text = '' OR fi.applicant_account_id = sqlc.arg(applicant_account_id))
  AND (sqlc.arg(search)::text = '' OR fi.payload::text ILIKE '%' || sqlc.arg(search) || '%' OR EXISTS (
    SELECT 1 FROM form_templates fts
    WHERE fts.tenant_id = fi.tenant_id
      AND fts.id = fi.template_id
      AND (fts.name ILIKE '%' || sqlc.arg(search) || '%' OR fts.key ILIKE '%' || sqlc.arg(search) || '%')
  ) OR EXISTS (
    SELECT 1 FROM accounts
    WHERE accounts.tenant_id = fi.tenant_id
      AND accounts.id = fi.applicant_account_id
      AND accounts.display_name ILIKE '%' || sqlc.arg(search) || '%'
  ));

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
  AND (sqlc.arg(search)::text = '' OR fi.payload::text ILIKE '%' || sqlc.arg(search) || '%' OR EXISTS (
    SELECT 1 FROM form_templates fts
    WHERE fts.tenant_id = fi.tenant_id
      AND fts.id = fi.template_id
      AND (fts.name ILIKE '%' || sqlc.arg(search) || '%' OR fts.key ILIKE '%' || sqlc.arg(search) || '%')
  ) OR EXISTS (
    SELECT 1 FROM accounts
    WHERE accounts.tenant_id = fi.tenant_id
      AND accounts.id = fi.applicant_account_id
      AND accounts.display_name ILIKE '%' || sqlc.arg(search) || '%'
  ))
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
  AND (sqlc.arg(search)::text = '' OR fi.payload::text ILIKE '%' || sqlc.arg(search) || '%' OR EXISTS (
    SELECT 1 FROM form_templates fts
    WHERE fts.tenant_id = fi.tenant_id
      AND fts.id = fi.template_id
      AND (fts.name ILIKE '%' || sqlc.arg(search) || '%' OR fts.key ILIKE '%' || sqlc.arg(search) || '%')
  ) OR EXISTS (
    SELECT 1 FROM accounts
    WHERE accounts.tenant_id = fi.tenant_id
      AND accounts.id = fi.applicant_account_id
      AND accounts.display_name ILIKE '%' || sqlc.arg(search) || '%'
  ))
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'submitted_at_asc' THEN fi.submitted_at END ASC,
  fi.submitted_at DESC,
  fi.id ASC
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;

-- name: DeleteFormInstanceFieldValues :exec
DELETE FROM form_instance_field_values
WHERE tenant_id = sqlc.arg(tenant_id)
  AND form_instance_id = sqlc.arg(form_instance_id);

-- name: InsertFormInstanceFieldValue :exec
INSERT INTO form_instance_field_values (
    tenant_id, form_instance_id, template_id, template_version_id, field_id, value_type,
    value_text, value_number, value_boolean, value_date, value_timestamp, value_json, created_at
) VALUES (
    sqlc.arg(tenant_id), sqlc.arg(form_instance_id), sqlc.arg(template_id), sqlc.arg(template_version_id),
    sqlc.arg(field_id), sqlc.arg(value_type),
    CASE WHEN sqlc.arg(value_type)::text = 'text' THEN sqlc.arg(value_text)::text ELSE NULL END,
    CASE WHEN sqlc.arg(value_type)::text = 'number' THEN NULLIF(sqlc.arg(value_number)::text, '')::numeric ELSE NULL END,
    CASE WHEN sqlc.arg(value_type)::text = 'boolean' THEN sqlc.narg(value_boolean)::boolean ELSE NULL END,
    CASE WHEN sqlc.arg(value_type)::text = 'date' THEN NULLIF(sqlc.arg(value_date)::text, '')::date ELSE NULL END,
    CASE WHEN sqlc.arg(value_type)::text = 'timestamp' THEN NULLIF(sqlc.arg(value_timestamp)::text, '')::timestamptz ELSE NULL END,
    CASE WHEN sqlc.arg(value_type)::text = 'json' AND sqlc.arg(value_json)::text <> '' THEN sqlc.arg(value_json)::jsonb ELSE NULL END,
    sqlc.arg(created_at)
)
ON CONFLICT (tenant_id, form_instance_id, field_id) DO UPDATE SET
    template_id = EXCLUDED.template_id,
    template_version_id = EXCLUDED.template_version_id,
    value_type = EXCLUDED.value_type,
    value_text = EXCLUDED.value_text,
    value_number = EXCLUDED.value_number,
    value_boolean = EXCLUDED.value_boolean,
    value_date = EXCLUDED.value_date,
    value_timestamp = EXCLUDED.value_timestamp,
    value_json = EXCLUDED.value_json,
    created_at = EXCLUDED.created_at;

-- name: ListFormInstanceFieldValues :many
SELECT * FROM form_instance_field_values
WHERE tenant_id = sqlc.arg(tenant_id)
  AND form_instance_id = sqlc.arg(form_instance_id)
ORDER BY field_id ASC;

-- name: DeleteFormInstance :exec
DELETE FROM form_instances
WHERE tenant_id = sqlc.arg(tenant_id) AND id = sqlc.arg(id);

-- name: UpsertLeaveRecord :one
INSERT INTO leave_records (
    id, tenant_id, employee_id, leave_type_id, balance_id, entitlement_year,
    source, event_date, start_at, end_at, net_minutes, remark, status,
    matched_record_id, reconciliation_status, last_seen_at, deleted_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(employee_id), sqlc.arg(leave_type_id),
    sqlc.arg(balance_id), sqlc.arg(entitlement_year), sqlc.arg(source), sqlc.arg(event_date),
    sqlc.arg(start_at), sqlc.arg(end_at), sqlc.arg(net_minutes), sqlc.arg(remark), sqlc.arg(status),
    sqlc.narg(matched_record_id), sqlc.arg(reconciliation_status),
    sqlc.narg(last_seen_at), sqlc.narg(deleted_at), sqlc.arg(updated_at)
)
ON CONFLICT (id) DO UPDATE SET
    employee_id = EXCLUDED.employee_id,
    leave_type_id = EXCLUDED.leave_type_id,
    balance_id = EXCLUDED.balance_id,
    entitlement_year = EXCLUDED.entitlement_year,
    source = EXCLUDED.source,
    event_date = EXCLUDED.event_date,
    start_at = EXCLUDED.start_at,
    end_at = EXCLUDED.end_at,
    net_minutes = EXCLUDED.net_minutes,
    remark = EXCLUDED.remark,
    status = EXCLUDED.status,
    matched_record_id = EXCLUDED.matched_record_id,
    reconciliation_status = EXCLUDED.reconciliation_status,
    last_seen_at = EXCLUDED.last_seen_at,
    deleted_at = EXCLUDED.deleted_at,
    updated_at = EXCLUDED.updated_at
WHERE leave_records.tenant_id = EXCLUDED.tenant_id
RETURNING *;

-- name: GetLeaveRecord :one
SELECT * FROM leave_records
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
LIMIT 1;

-- name: ListLeaveRecords :many
SELECT * FROM leave_records
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY event_date ASC, id ASC;

-- name: ListActiveLeaveRecordsByQuery :many
SELECT * FROM leave_records
WHERE tenant_id = sqlc.arg(tenant_id)
  AND status = 'active'
  AND deleted_at IS NULL
  AND employee_id = ANY(sqlc.arg(employee_ids)::text[])
  AND start_at < sqlc.arg(to_at)::timestamptz
  AND end_at > sqlc.arg(from_at)::timestamptz
  AND (
      source = 'nexus'
      OR (source = 'ehrms' AND matched_record_id IS NULL)
  )
ORDER BY start_at ASC, id ASC;

-- name: UpsertWorkflowRun :one
INSERT INTO workflow_runs (
    id, tenant_id, form_instance_id, template_id, version, status,
    current_stage_instance_id, stage_definitions_json,
    temporal_start_status, temporal_workflow_id, temporal_run_id,
    temporal_start_event_id, temporal_started_at, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8::jsonb,
    $9, $10, $11, $12, $13, $14, $15
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    form_instance_id = EXCLUDED.form_instance_id,
    template_id = EXCLUDED.template_id,
    version = EXCLUDED.version,
    status = EXCLUDED.status,
    current_stage_instance_id = EXCLUDED.current_stage_instance_id,
    stage_definitions_json = EXCLUDED.stage_definitions_json,
    temporal_start_status = EXCLUDED.temporal_start_status,
    temporal_workflow_id = EXCLUDED.temporal_workflow_id,
    temporal_run_id = EXCLUDED.temporal_run_id,
    temporal_start_event_id = EXCLUDED.temporal_start_event_id,
    temporal_started_at = EXCLUDED.temporal_started_at,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at
RETURNING id, tenant_id, form_instance_id, template_id, version, status,
    current_stage_instance_id, stage_definitions_json, temporal_start_status,
    temporal_workflow_id, temporal_run_id, temporal_start_event_id,
    temporal_started_at, created_at, updated_at;

-- name: GetWorkflowRun :one
SELECT id, tenant_id, form_instance_id, template_id, version, status,
    current_stage_instance_id, stage_definitions_json, temporal_start_status,
    temporal_workflow_id, temporal_run_id, temporal_start_event_id,
    temporal_started_at, created_at, updated_at
FROM workflow_runs
WHERE tenant_id = $1 AND id = $2;

-- name: ListWorkflowRunsByFormInstance :many
SELECT id, tenant_id, form_instance_id, template_id, version, status,
    current_stage_instance_id, stage_definitions_json, temporal_start_status,
    temporal_workflow_id, temporal_run_id, temporal_start_event_id,
    temporal_started_at, created_at, updated_at
FROM workflow_runs
WHERE tenant_id = $1 AND form_instance_id = $2
ORDER BY version ASC, created_at ASC;

-- name: ListPendingWorkflowRuns :many
SELECT id, tenant_id, form_instance_id, template_id, version, status,
    current_stage_instance_id, stage_definitions_json, temporal_start_status,
    temporal_workflow_id, temporal_run_id, temporal_start_event_id,
    temporal_started_at, created_at, updated_at
FROM workflow_runs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (
    temporal_start_status = 'pending_start'
    OR (
      temporal_start_status = 'starting'
      AND updated_at <= sqlc.arg(stale_before)::timestamptz
    )
  )
ORDER BY updated_at ASC, id ASC
LIMIT sqlc.arg(limit_count)::int;

-- name: ClaimWorkflowRunTemporalStart :one
UPDATE workflow_runs
SET temporal_start_status = 'starting',
    updated_at = sqlc.arg(claimed_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND (
    temporal_start_status = 'pending_start'
    OR (
      temporal_start_status = 'starting'
      AND updated_at <= sqlc.arg(stale_before)::timestamptz
    )
  )
RETURNING id, tenant_id, form_instance_id, template_id, version, status,
    current_stage_instance_id, stage_definitions_json, temporal_start_status,
    temporal_workflow_id, temporal_run_id, temporal_start_event_id,
    temporal_started_at, created_at, updated_at;

-- name: ReleaseWorkflowRunTemporalStart :one
UPDATE workflow_runs
SET temporal_start_status = 'pending_start',
    updated_at = sqlc.arg(released_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND temporal_start_status = 'starting'
  AND updated_at = sqlc.arg(claimed_at)::timestamptz
RETURNING id;

-- name: MarkWorkflowRunTemporalStarted :one
UPDATE workflow_runs
SET temporal_start_status = 'started',
    temporal_workflow_id = sqlc.arg(temporal_workflow_id),
    temporal_run_id = sqlc.arg(temporal_run_id),
    temporal_started_at = sqlc.arg(started_at)::timestamptz,
    updated_at = sqlc.arg(started_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND temporal_start_status = 'starting'
  AND updated_at = sqlc.arg(claimed_at)::timestamptz
RETURNING id, tenant_id, form_instance_id, template_id, version, status,
    current_stage_instance_id, stage_definitions_json, temporal_start_status,
    temporal_workflow_id, temporal_run_id, temporal_start_event_id,
    temporal_started_at, created_at, updated_at;

-- name: AbandonPendingWorkflowRunTemporalStart :one
UPDATE workflow_runs
SET temporal_start_status = 'abandoned',
    updated_at = sqlc.arg(abandoned_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND temporal_start_status = 'pending_start'
RETURNING id, tenant_id, form_instance_id, template_id, version, status,
    current_stage_instance_id, stage_definitions_json, temporal_start_status,
    temporal_workflow_id, temporal_run_id, temporal_start_event_id,
    temporal_started_at, created_at, updated_at;

-- name: AbandonClaimedWorkflowRunTemporalStart :one
UPDATE workflow_runs
SET temporal_start_status = 'abandoned',
    updated_at = sqlc.arg(abandoned_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND temporal_start_status = 'starting'
  AND updated_at = sqlc.arg(claimed_at)::timestamptz
RETURNING id, tenant_id, form_instance_id, template_id, version, status,
    current_stage_instance_id, stage_definitions_json, temporal_start_status,
    temporal_workflow_id, temporal_run_id, temporal_start_event_id,
    temporal_started_at, created_at, updated_at;

-- name: UpsertWorkflowStageInstance :one
INSERT INTO workflow_stage_instances (
    id, tenant_id, run_id, stage_id, stage_type, label, status, sequence,
    result, started_at, completed_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    run_id = EXCLUDED.run_id,
    stage_id = EXCLUDED.stage_id,
    stage_type = EXCLUDED.stage_type,
    label = EXCLUDED.label,
    status = EXCLUDED.status,
    sequence = EXCLUDED.sequence,
    result = EXCLUDED.result,
    started_at = EXCLUDED.started_at,
    completed_at = EXCLUDED.completed_at
RETURNING *;

-- name: GetWorkflowStageInstance :one
SELECT * FROM workflow_stage_instances
WHERE tenant_id = $1 AND id = $2;

-- name: GetWorkflowStageInstanceForUpdate :one
SELECT * FROM workflow_stage_instances
WHERE tenant_id = sqlc.arg(tenant_id) AND id = sqlc.arg(id)
FOR UPDATE;

-- name: ListWorkflowStageInstancesByRun :many
SELECT * FROM workflow_stage_instances
WHERE tenant_id = $1 AND run_id = $2
ORDER BY sequence ASC;

-- name: UpsertWorkflowStageAssignee :exec
INSERT INTO workflow_stage_assignees (
    tenant_id, stage_instance_id, account_id, status
) VALUES (
    $1, $2, $3, $4
)
ON CONFLICT (tenant_id, stage_instance_id, account_id) DO UPDATE SET
    status = EXCLUDED.status;

-- name: ListWorkflowStageAssignees :many
SELECT * FROM workflow_stage_assignees
WHERE tenant_id = $1 AND stage_instance_id = $2
ORDER BY account_id ASC;

-- name: ListPendingAssigneeStageInstanceIDs :many
SELECT DISTINCT stage_instance_id FROM workflow_stage_assignees
WHERE tenant_id = $1 AND account_id = $2 AND status = 'pending'
ORDER BY stage_instance_id ASC;

-- name: InsertWorkflowAction :one
INSERT INTO workflow_actions (
    id, tenant_id, run_id, stage_instance_id, account_id, action, comment,
    idempotency_key, command_fingerprint, request_id, trace_id, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
RETURNING id, tenant_id, run_id, stage_instance_id, account_id, action, comment,
    idempotency_key, command_fingerprint, request_id, trace_id, created_at;

-- name: ListWorkflowActionsByRun :many
SELECT id, tenant_id, run_id, stage_instance_id, account_id, action, comment,
    idempotency_key, command_fingerprint, request_id, trace_id, created_at
FROM workflow_actions
WHERE tenant_id = $1 AND run_id = $2
ORDER BY created_at ASC;

-- name: GetWorkflowActionByIdempotencyKey :one
SELECT id, tenant_id, run_id, stage_instance_id, account_id, action, comment,
    idempotency_key, command_fingerprint, request_id, trace_id, created_at
FROM workflow_actions
WHERE tenant_id = $1 AND run_id = $2 AND idempotency_key = $3;


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

-- name: UpsertAttendanceClockRecord :one
INSERT INTO attendance_clock_records (
    id, tenant_id, employee_id, worksite_id,
    work_date, direction, client_event_id, clocked_at, latitude, longitude, accuracy_meters,
    distance_meters, record_status, rejection_reason, source, device_id,
    device_info, correction_request_id, voided, voided_at, voided_by_account_id,
    void_reason, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(employee_id), sqlc.narg(worksite_id),
    sqlc.arg(work_date)::date, sqlc.arg(direction), sqlc.arg(client_event_id), sqlc.arg(clocked_at),
    sqlc.narg(latitude), sqlc.narg(longitude), sqlc.narg(accuracy_meters),
    sqlc.narg(distance_meters), sqlc.arg(record_status), sqlc.arg(rejection_reason),
    sqlc.arg(source), sqlc.arg(device_id), sqlc.arg(device_info)::jsonb,
    sqlc.narg(correction_request_id), sqlc.arg(voided), sqlc.narg(voided_at),
    sqlc.narg(voided_by_account_id), sqlc.narg(void_reason), sqlc.arg(created_at)
)
ON CONFLICT (id) DO UPDATE SET
    worksite_id = EXCLUDED.worksite_id,
    work_date = EXCLUDED.work_date,
    direction = EXCLUDED.direction,
    client_event_id = EXCLUDED.client_event_id,
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
    voided = EXCLUDED.voided,
    voided_at = EXCLUDED.voided_at,
    voided_by_account_id = EXCLUDED.voided_by_account_id,
    void_reason = EXCLUDED.void_reason
WHERE attendance_clock_records.tenant_id = EXCLUDED.tenant_id
  AND attendance_clock_records.employee_id = EXCLUDED.employee_id
RETURNING *;

-- name: GetAttendanceClockRecordByClientEventID :one
SELECT * FROM attendance_clock_records
WHERE tenant_id = $1
  AND client_event_id = $2
  AND client_event_id <> ''
LIMIT 1;

-- name: GetEarliestAcceptedAttendanceClockIn :one
SELECT * FROM attendance_clock_records
WHERE tenant_id = $1
  AND employee_id = $2
  AND work_date = sqlc.arg(work_date)::date
  AND direction = 'clock_in'
  AND record_status = 'accepted'
  AND voided = false
ORDER BY clocked_at ASC, created_at ASC, id ASC
LIMIT 1;

-- name: GetLatestAcceptedAttendanceClockOut :one
SELECT * FROM attendance_clock_records
WHERE tenant_id = $1
  AND employee_id = $2
  AND work_date = sqlc.arg(work_date)::date
  AND direction = 'clock_out'
  AND record_status = 'accepted'
  AND voided = false
ORDER BY clocked_at DESC, created_at DESC, id DESC
LIMIT 1;

-- name: GetLatestAcceptedAttendanceClockRecord :one
SELECT * FROM attendance_clock_records
WHERE tenant_id = $1
  AND employee_id = $2
  AND work_date = sqlc.arg(work_date)::date
  AND record_status = 'accepted'
  AND voided = false
ORDER BY clocked_at DESC, created_at DESC, id DESC
LIMIT 1;

-- name: ListAttendanceClockRecords :many
SELECT * FROM attendance_clock_records
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(employee_id)::text = '' OR employee_id = sqlc.arg(employee_id))
  AND (coalesce(cardinality(sqlc.arg(employee_ids)::text[]), 0) = 0 OR employee_id = ANY(sqlc.arg(employee_ids)::text[]))
  AND (NULLIF(sqlc.arg(from_date)::text, '') IS NULL OR work_date >= NULLIF(sqlc.arg(from_date)::text, '')::date)
  AND (NULLIF(sqlc.arg(to_date)::text, '') IS NULL OR work_date <= NULLIF(sqlc.arg(to_date)::text, '')::date)
  AND (sqlc.arg(direction)::text = '' OR direction = sqlc.arg(direction))
  AND (sqlc.arg(record_status)::text = '' OR record_status = sqlc.arg(record_status))
  AND (sqlc.arg(source)::text = '' OR source = sqlc.arg(source))
ORDER BY clocked_at DESC, created_at DESC, id ASC;

-- name: UpsertAttendanceDailySummary :one
INSERT INTO attendance_daily_summaries (
    tenant_id, employee_id, work_date, shift_start, shift_end,
    shift_hours, daily_hours, clock_hours, clock_start, clock_end,
    payload, source, external_ref, created_at, updated_at
) VALUES (
    sqlc.arg(tenant_id), sqlc.arg(employee_id), sqlc.arg(work_date)::date,
    sqlc.arg(shift_start), sqlc.arg(shift_end), sqlc.arg(shift_hours), sqlc.arg(daily_hours),
    sqlc.arg(clock_hours), sqlc.arg(clock_start), sqlc.arg(clock_end),
    sqlc.arg(payload)::jsonb, sqlc.arg(source), sqlc.arg(external_ref),
    sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (tenant_id, employee_id, work_date) DO UPDATE SET
    shift_start = EXCLUDED.shift_start,
    shift_end = EXCLUDED.shift_end,
    shift_hours = EXCLUDED.shift_hours,
    daily_hours = EXCLUDED.daily_hours,
    clock_hours = EXCLUDED.clock_hours,
    clock_start = EXCLUDED.clock_start,
    clock_end = EXCLUDED.clock_end,
    payload = EXCLUDED.payload,
    source = EXCLUDED.source,
    external_ref = EXCLUDED.external_ref,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetAttendanceDailySummaryByExternalRef :one
SELECT * FROM attendance_daily_summaries
WHERE tenant_id = $1 AND external_ref = $2 AND external_ref <> ''
LIMIT 1;

-- name: GetAttendanceDailySummaryByEmployeeDate :one
SELECT * FROM attendance_daily_summaries
WHERE tenant_id = $1 AND employee_id = $2 AND work_date = sqlc.arg(work_date)::date
LIMIT 1;

-- name: ListAttendanceDailySummaries :many
SELECT * FROM attendance_daily_summaries
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(employee_id)::text = '' OR employee_id = sqlc.arg(employee_id))
  AND (coalesce(cardinality(sqlc.arg(employee_ids)::text[]), 0) = 0 OR employee_id = ANY(sqlc.arg(employee_ids)::text[]))
  AND (NULLIF(sqlc.arg(from_date)::text, '') IS NULL OR work_date >= NULLIF(sqlc.arg(from_date)::text, '')::date)
  AND (NULLIF(sqlc.arg(to_date)::text, '') IS NULL OR work_date <= NULLIF(sqlc.arg(to_date)::text, '')::date)
  AND (sqlc.arg(source)::text = '' OR source = sqlc.arg(source))
ORDER BY work_date ASC, employee_id ASC;

-- name: UpsertAttendanceDayProjection :one
INSERT INTO attendance_day_projections (
    tenant_id, employee_id, work_date, policy_version,
    scheduled_start_at, scheduled_end_at, clock_in_record_id, clock_out_record_id,
    last_punch_record_id, punch_count, worked_minutes, approved_leave_minutes,
    pending_leave_minutes, required_minutes, overtime_minutes, day_status,
    anomaly_reasons, input_fingerprint, payload, computed_at, updated_at
) VALUES (
    sqlc.arg(tenant_id), sqlc.arg(employee_id), sqlc.arg(work_date)::date, sqlc.arg(policy_version),
    sqlc.narg(scheduled_start_at), sqlc.narg(scheduled_end_at), sqlc.narg(clock_in_record_id), sqlc.narg(clock_out_record_id),
    sqlc.narg(last_punch_record_id), sqlc.arg(punch_count), sqlc.arg(worked_minutes), sqlc.arg(approved_leave_minutes),
    sqlc.arg(pending_leave_minutes), sqlc.arg(required_minutes), sqlc.arg(overtime_minutes), sqlc.arg(day_status),
    sqlc.arg(anomaly_reasons)::text[], sqlc.arg(input_fingerprint), sqlc.arg(payload)::jsonb,
    sqlc.arg(computed_at), sqlc.arg(updated_at)
)
ON CONFLICT (tenant_id, employee_id, work_date) DO UPDATE SET
    policy_version = EXCLUDED.policy_version,
    scheduled_start_at = EXCLUDED.scheduled_start_at,
    scheduled_end_at = EXCLUDED.scheduled_end_at,
    clock_in_record_id = EXCLUDED.clock_in_record_id,
    clock_out_record_id = EXCLUDED.clock_out_record_id,
    last_punch_record_id = EXCLUDED.last_punch_record_id,
    punch_count = EXCLUDED.punch_count,
    worked_minutes = EXCLUDED.worked_minutes,
    approved_leave_minutes = EXCLUDED.approved_leave_minutes,
    pending_leave_minutes = EXCLUDED.pending_leave_minutes,
    required_minutes = EXCLUDED.required_minutes,
    overtime_minutes = EXCLUDED.overtime_minutes,
    day_status = EXCLUDED.day_status,
    anomaly_reasons = EXCLUDED.anomaly_reasons,
    input_fingerprint = EXCLUDED.input_fingerprint,
    payload = EXCLUDED.payload,
    computed_at = EXCLUDED.computed_at,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetAttendanceDayProjection :one
SELECT * FROM attendance_day_projections
WHERE tenant_id = sqlc.arg(tenant_id)
  AND employee_id = sqlc.arg(employee_id)
  AND work_date = sqlc.arg(work_date)::date;

-- name: GetAttendanceDayProjectionForUpdate :one
SELECT * FROM attendance_day_projections
WHERE tenant_id = sqlc.arg(tenant_id)
  AND employee_id = sqlc.arg(employee_id)
  AND work_date = sqlc.arg(work_date)::date
FOR UPDATE;

-- name: ListAttendanceDayProjections :many
SELECT * FROM attendance_day_projections
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (coalesce(cardinality(sqlc.arg(employee_ids)::text[]), 0) = 0 OR employee_id = ANY(sqlc.arg(employee_ids)::text[]))
  AND (NULLIF(sqlc.arg(from_date)::text, '') IS NULL OR work_date >= NULLIF(sqlc.arg(from_date)::text, '')::date)
  AND (NULLIF(sqlc.arg(to_date)::text, '') IS NULL OR work_date <= NULLIF(sqlc.arg(to_date)::text, '')::date)
ORDER BY work_date, employee_id;

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
  AND (sqlc.arg(from_created_at)::timestamptz IS NULL OR created_at >= sqlc.arg(from_created_at)::timestamptz)
  AND (sqlc.arg(to_created_at)::timestamptz IS NULL OR created_at < sqlc.arg(to_created_at)::timestamptz)
  AND (
    NOT sqlc.arg(has_cursor)::boolean
    OR created_at < sqlc.arg(cursor_created_at)::timestamptz
    OR (created_at = sqlc.arg(cursor_created_at)::timestamptz AND id < sqlc.arg(cursor_id))
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count)::int;

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
  AND (sqlc.arg(from_created_at)::timestamptz IS NULL OR created_at >= sqlc.arg(from_created_at)::timestamptz)
  AND (sqlc.arg(to_created_at)::timestamptz IS NULL OR created_at < sqlc.arg(to_created_at)::timestamptz)
  AND (
    NOT sqlc.arg(has_cursor)::boolean
    OR created_at < sqlc.arg(cursor_created_at)::timestamptz
    OR (created_at = sqlc.arg(cursor_created_at)::timestamptz AND id < sqlc.arg(cursor_id))
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count)::int;

-- name: DeletePlatformTaskTodo :exec
DELETE FROM platform_task_todos
WHERE tenant_id = sqlc.arg(tenant_id)
  AND account_id = sqlc.arg(account_id)
  AND id = sqlc.arg(id);

-- name: UpsertAgentRun :one
WITH inserted_conversation AS (
    INSERT INTO conversations (
        id, tenant_id, owner_account_id, agent_id, current_segment_id,
        next_message_sequence, title, status, last_message_at,
        created_at, updated_at, archived_at
    )
    SELECT
        'execution:' || sqlc.arg(id)::text,
        sqlc.arg(tenant_id),
        sqlc.arg(account_id),
        (SELECT agents.id FROM agents WHERE agents.tenant_id = sqlc.arg(tenant_id) AND agents.id = NULLIF(sqlc.arg(agent_id)::text, '') AND agents.lifecycle_status = 'active'),
        NULL, 1, '', 'active', NULL,
        sqlc.arg(created_at), sqlc.arg(updated_at), NULL
    WHERE sqlc.arg(session_id)::text = ''
    ON CONFLICT (id) DO NOTHING
    RETURNING *
), target_conversation AS (
    SELECT * FROM inserted_conversation
    UNION ALL
    SELECT conversations.*
    FROM conversations
    WHERE conversations.tenant_id = sqlc.arg(tenant_id)
      AND conversations.id = NULLIF(sqlc.arg(session_id)::text, '')
    LIMIT 1
), inserted_segment AS (
    INSERT INTO conversation_segments (
        id, tenant_id, conversation_id, ordinal, start_reason, created_at
    )
    SELECT target_conversation.id || ':segment:1', target_conversation.tenant_id,
           target_conversation.id, 1, 'initial', sqlc.arg(created_at)
    FROM target_conversation
    WHERE target_conversation.current_segment_id IS NULL
    ON CONFLICT (tenant_id, conversation_id, ordinal) DO NOTHING
    RETURNING *
), target_segment AS (
    SELECT id, tenant_id, conversation_id FROM inserted_segment
    UNION ALL
    SELECT segments.id, segments.tenant_id, segments.conversation_id
    FROM conversation_segments segments
    JOIN target_conversation conversations
      ON conversations.tenant_id = segments.tenant_id
     AND conversations.id = segments.conversation_id
     AND conversations.current_segment_id = segments.id
    UNION ALL
    SELECT segments.id, segments.tenant_id, segments.conversation_id
    FROM conversation_segments segments
    JOIN target_conversation conversations
      ON conversations.tenant_id = segments.tenant_id
     AND conversations.id = segments.conversation_id
    WHERE conversations.current_segment_id IS NULL AND segments.ordinal = 1
    LIMIT 1
), allocated_input AS (
    UPDATE conversations
    SET current_segment_id = target_segment.id,
        next_message_sequence = conversations.next_message_sequence + 1,
        last_message_at = GREATEST(COALESCE(conversations.last_message_at, sqlc.arg(created_at)::timestamptz), sqlc.arg(created_at)::timestamptz),
        updated_at = GREATEST(conversations.updated_at, sqlc.arg(updated_at)::timestamptz)
    FROM target_segment
    WHERE conversations.tenant_id = target_segment.tenant_id
      AND conversations.id = target_segment.conversation_id
    RETURNING conversations.*, conversations.next_message_sequence - 1 AS allocated_sequence
), input_message AS (
    INSERT INTO conversation_messages (
        id, tenant_id, conversation_id, segment_id, sequence_no,
        role, content, content_json, execution_id, execution_step_id, created_at
    )
    SELECT
        sqlc.arg(id)::text || ':input', allocated_input.tenant_id,
        allocated_input.id, allocated_input.current_segment_id,
        allocated_input.allocated_sequence, 'user', sqlc.arg(prompt),
        '{}'::jsonb, NULL, NULL, sqlc.arg(created_at)
    FROM allocated_input
    ON CONFLICT (id) DO UPDATE SET
        content = EXCLUDED.content,
        content_json = EXCLUDED.content_json
    WHERE conversation_messages.tenant_id = EXCLUDED.tenant_id
    RETURNING *
), binding AS (
    SELECT
        input_message.*,
        CASE WHEN revisions.id IS NULL THEN NULL ELSE agents.id END AS bound_agent_id,
        revisions.id AS bound_revision_id,
        revisions.model_connection_id AS bound_model_connection_id
    FROM input_message
    JOIN conversations
      ON conversations.tenant_id = input_message.tenant_id
     AND conversations.id = input_message.conversation_id
    LEFT JOIN agents
      ON agents.tenant_id = conversations.tenant_id
     AND agents.id = conversations.agent_id
    LEFT JOIN agent_revisions revisions
      ON revisions.tenant_id = agents.tenant_id
     AND revisions.agent_id = agents.id
     AND revisions.id = COALESCE(agents.published_revision_id, agents.draft_revision_id)
), upserted_execution AS (
    INSERT INTO conversation_executions (
        id, tenant_id, account_id, conversation_id, segment_id, input_message_id,
        agent_id, agent_revision_id, model_connection_id,
        mode, trigger_type, status, queued_at, started_at, completed_at,
        error_code, error_category, safe_error_message,
        llm_call_count, input_tokens, cached_tokens, output_tokens, total_tokens,
        usage_complete, created_at, updated_at
    )
    SELECT
        sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(account_id),
        binding.conversation_id, binding.segment_id, binding.id,
        binding.bound_agent_id, binding.bound_revision_id, binding.bound_model_connection_id,
        sqlc.arg(mode), 'chat', sqlc.arg(status), sqlc.arg(created_at),
        CASE WHEN sqlc.arg(status)::text = 'queued' THEN NULL ELSE sqlc.arg(created_at)::timestamptz END,
        CASE WHEN sqlc.arg(status)::text IN ('completed', 'failed', 'cancelled') THEN sqlc.arg(updated_at)::timestamptz ELSE NULL END,
        '', '', CASE WHEN sqlc.arg(status)::text = 'failed' THEN sqlc.arg(answer)::text ELSE '' END,
        sqlc.arg(llm_call_count), sqlc.arg(input_tokens), sqlc.arg(cached_tokens),
        sqlc.arg(output_tokens), sqlc.arg(total_tokens), sqlc.arg(usage_complete),
        sqlc.arg(created_at), sqlc.arg(updated_at)
    FROM binding
    ON CONFLICT (id) DO UPDATE SET
        status = EXCLUDED.status,
        started_at = CASE
            WHEN EXCLUDED.status = 'queued' THEN NULL
            ELSE COALESCE(conversation_executions.started_at, EXCLUDED.started_at)
        END,
        completed_at = EXCLUDED.completed_at,
        safe_error_message = EXCLUDED.safe_error_message,
        llm_call_count = EXCLUDED.llm_call_count,
        input_tokens = EXCLUDED.input_tokens,
        cached_tokens = EXCLUDED.cached_tokens,
        output_tokens = EXCLUDED.output_tokens,
        total_tokens = EXCLUDED.total_tokens,
        usage_complete = EXCLUDED.usage_complete,
        updated_at = EXCLUDED.updated_at
    WHERE conversation_executions.tenant_id = EXCLUDED.tenant_id
    RETURNING *
), allocated_answer AS (
    UPDATE conversations
    SET next_message_sequence = conversations.next_message_sequence + 1,
        last_message_at = GREATEST(COALESCE(conversations.last_message_at, sqlc.arg(updated_at)::timestamptz), sqlc.arg(updated_at)::timestamptz),
        updated_at = GREATEST(conversations.updated_at, sqlc.arg(updated_at)::timestamptz)
    FROM upserted_execution
    WHERE conversations.tenant_id = upserted_execution.tenant_id
      AND conversations.id = upserted_execution.conversation_id
      AND sqlc.arg(answer)::text <> ''
    RETURNING conversations.next_message_sequence - 1 AS allocated_sequence
), answer_message AS (
    INSERT INTO conversation_messages (
        id, tenant_id, conversation_id, segment_id, sequence_no,
        role, content, content_json, execution_id, execution_step_id, created_at
    )
    SELECT
        upserted_execution.id || ':answer', upserted_execution.tenant_id,
        upserted_execution.conversation_id, upserted_execution.segment_id,
        allocated_answer.allocated_sequence, 'assistant', sqlc.arg(answer),
        jsonb_build_object('reference_items', sqlc.arg(reference_items)::jsonb),
        upserted_execution.id, NULL, sqlc.arg(updated_at)
    FROM upserted_execution
    CROSS JOIN allocated_answer
    ON CONFLICT (id) DO UPDATE SET
        content = EXCLUDED.content,
        content_json = EXCLUDED.content_json,
        execution_id = EXCLUDED.execution_id
    WHERE conversation_messages.tenant_id = EXCLUDED.tenant_id
    RETURNING *
)
SELECT
    conversation_executions.id, conversation_executions.tenant_id, conversation_executions.account_id, conversation_executions.agent_id,
    conversation_executions.conversation_id AS session_id, conversation_executions.segment_id,
    conversation_executions.input_message_id, conversation_executions.agent_revision_id, conversation_executions.model_connection_id,
    conversation_executions.mode, input_message.content AS prompt,
    COALESCE(answer_message.content, sqlc.arg(answer)::text, '')::text AS answer,
    conversation_executions.status,
    COALESCE(answer_message.content_json->'reference_items', sqlc.arg(reference_items)::jsonb, '[]'::jsonb) AS reference_items,
    conversation_executions.llm_call_count, conversation_executions.input_tokens, conversation_executions.cached_tokens,
    conversation_executions.output_tokens, conversation_executions.total_tokens, conversation_executions.usage_complete,
    conversation_executions.queued_at, conversation_executions.started_at, conversation_executions.completed_at,
    conversation_executions.error_code, conversation_executions.error_category, conversation_executions.safe_error_message,
    conversation_executions.created_at, conversation_executions.updated_at
FROM upserted_execution conversation_executions
JOIN input_message ON input_message.tenant_id = conversation_executions.tenant_id AND input_message.id = conversation_executions.input_message_id
LEFT JOIN answer_message ON answer_message.tenant_id = conversation_executions.tenant_id AND answer_message.execution_id = conversation_executions.id;

-- name: ListAgentRuns :many
SELECT
    conversation_executions.id, conversation_executions.tenant_id, conversation_executions.account_id, conversation_executions.agent_id,
    conversation_executions.conversation_id AS session_id, conversation_executions.segment_id,
    conversation_executions.input_message_id, conversation_executions.agent_revision_id, conversation_executions.model_connection_id,
    conversation_executions.mode, input_message.content AS prompt,
    COALESCE(answer_message.content, '')::text AS answer, conversation_executions.status,
    COALESCE(answer_message.content_json->'reference_items', '[]'::jsonb) AS reference_items,
    conversation_executions.llm_call_count, conversation_executions.input_tokens, conversation_executions.cached_tokens,
    conversation_executions.output_tokens, conversation_executions.total_tokens, conversation_executions.usage_complete,
    conversation_executions.queued_at, conversation_executions.started_at, conversation_executions.completed_at,
    conversation_executions.error_code, conversation_executions.error_category, conversation_executions.safe_error_message,
    conversation_executions.created_at, conversation_executions.updated_at
FROM conversation_executions
JOIN conversation_messages input_message ON input_message.tenant_id = conversation_executions.tenant_id AND input_message.id = conversation_executions.input_message_id
LEFT JOIN LATERAL (
    SELECT conversation_messages.* FROM conversation_messages
    WHERE conversation_messages.tenant_id = conversation_executions.tenant_id AND conversation_messages.execution_id = conversation_executions.id AND conversation_messages.role = 'assistant'
    ORDER BY conversation_messages.sequence_no DESC LIMIT 1
) answer_message ON true
WHERE conversation_executions.tenant_id = $1
ORDER BY conversation_executions.created_at ASC;

-- name: ListAgentRunsByAccount :many
SELECT
    conversation_executions.id, conversation_executions.tenant_id, conversation_executions.account_id, conversation_executions.agent_id,
    conversation_executions.conversation_id AS session_id, conversation_executions.segment_id,
    conversation_executions.input_message_id, conversation_executions.agent_revision_id, conversation_executions.model_connection_id,
    conversation_executions.mode, input_message.content AS prompt,
    COALESCE(answer_message.content, '')::text AS answer, conversation_executions.status,
    COALESCE(answer_message.content_json->'reference_items', '[]'::jsonb) AS reference_items,
    conversation_executions.llm_call_count, conversation_executions.input_tokens, conversation_executions.cached_tokens,
    conversation_executions.output_tokens, conversation_executions.total_tokens, conversation_executions.usage_complete,
    conversation_executions.queued_at, conversation_executions.started_at, conversation_executions.completed_at,
    conversation_executions.error_code, conversation_executions.error_category, conversation_executions.safe_error_message,
    conversation_executions.created_at, conversation_executions.updated_at
FROM conversation_executions
JOIN conversation_messages input_message ON input_message.tenant_id = conversation_executions.tenant_id AND input_message.id = conversation_executions.input_message_id
LEFT JOIN LATERAL (
    SELECT conversation_messages.* FROM conversation_messages
    WHERE conversation_messages.tenant_id = conversation_executions.tenant_id AND conversation_messages.execution_id = conversation_executions.id AND conversation_messages.role = 'assistant'
    ORDER BY conversation_messages.sequence_no DESC LIMIT 1
) answer_message ON true
WHERE conversation_executions.tenant_id = sqlc.arg(tenant_id)
  AND conversation_executions.account_id = sqlc.arg(account_id)
ORDER BY conversation_executions.created_at DESC, conversation_executions.id ASC;

-- name: CountAgentRuns :one
SELECT count(*) FROM conversation_executions
WHERE tenant_id = $1;

-- name: CountAgentRunsByAccount :one
SELECT count(*) FROM conversation_executions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND account_id = sqlc.arg(account_id);

-- name: ListAgentRunsPage :many
SELECT
    conversation_executions.id, conversation_executions.tenant_id, conversation_executions.account_id, conversation_executions.agent_id,
    conversation_executions.conversation_id AS session_id, conversation_executions.segment_id,
    conversation_executions.input_message_id, conversation_executions.agent_revision_id, conversation_executions.model_connection_id,
    conversation_executions.mode, input_message.content AS prompt,
    COALESCE(answer_message.content, '')::text AS answer, conversation_executions.status,
    COALESCE(answer_message.content_json->'reference_items', '[]'::jsonb) AS reference_items,
    conversation_executions.llm_call_count, conversation_executions.input_tokens, conversation_executions.cached_tokens,
    conversation_executions.output_tokens, conversation_executions.total_tokens, conversation_executions.usage_complete,
    conversation_executions.queued_at, conversation_executions.started_at, conversation_executions.completed_at,
    conversation_executions.error_code, conversation_executions.error_category, conversation_executions.safe_error_message,
    conversation_executions.created_at, conversation_executions.updated_at
FROM conversation_executions
JOIN conversation_messages input_message ON input_message.tenant_id = conversation_executions.tenant_id AND input_message.id = conversation_executions.input_message_id
LEFT JOIN LATERAL (
    SELECT conversation_messages.* FROM conversation_messages
    WHERE conversation_messages.tenant_id = conversation_executions.tenant_id AND conversation_messages.execution_id = conversation_executions.id AND conversation_messages.role = 'assistant'
    ORDER BY conversation_messages.sequence_no DESC LIMIT 1
) answer_message ON true
WHERE conversation_executions.tenant_id = sqlc.arg(tenant_id)
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'created_at_asc' THEN conversation_executions.created_at END ASC,
  conversation_executions.created_at DESC,
  conversation_executions.id ASC
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;

-- name: ListAgentRunsPageByAccount :many
SELECT
    conversation_executions.id, conversation_executions.tenant_id, conversation_executions.account_id, conversation_executions.agent_id,
    conversation_executions.conversation_id AS session_id, conversation_executions.segment_id,
    conversation_executions.input_message_id, conversation_executions.agent_revision_id, conversation_executions.model_connection_id,
    conversation_executions.mode, input_message.content AS prompt,
    COALESCE(answer_message.content, '')::text AS answer, conversation_executions.status,
    COALESCE(answer_message.content_json->'reference_items', '[]'::jsonb) AS reference_items,
    conversation_executions.llm_call_count, conversation_executions.input_tokens, conversation_executions.cached_tokens,
    conversation_executions.output_tokens, conversation_executions.total_tokens, conversation_executions.usage_complete,
    conversation_executions.queued_at, conversation_executions.started_at, conversation_executions.completed_at,
    conversation_executions.error_code, conversation_executions.error_category, conversation_executions.safe_error_message,
    conversation_executions.created_at, conversation_executions.updated_at
FROM conversation_executions
JOIN conversation_messages input_message ON input_message.tenant_id = conversation_executions.tenant_id AND input_message.id = conversation_executions.input_message_id
LEFT JOIN LATERAL (
    SELECT conversation_messages.* FROM conversation_messages
    WHERE conversation_messages.tenant_id = conversation_executions.tenant_id AND conversation_messages.execution_id = conversation_executions.id AND conversation_messages.role = 'assistant'
    ORDER BY conversation_messages.sequence_no DESC LIMIT 1
) answer_message ON true
WHERE conversation_executions.tenant_id = sqlc.arg(tenant_id)
  AND conversation_executions.account_id = sqlc.arg(account_id)
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'created_at_asc' THEN conversation_executions.created_at END ASC,
  conversation_executions.created_at DESC,
  conversation_executions.id ASC
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

-- name: ListAuditLogFacetSources :many
SELECT DISTINCT actor_account_id, action, resource
FROM audit_logs
WHERE tenant_id = $1
ORDER BY actor_account_id, resource, action;

-- name: CountAuditLogs :one
SELECT count(*) FROM audit_logs
WHERE tenant_id = $1;

-- name: CountAuditLogsFiltered :one
SELECT count(*)
FROM audit_logs al
LEFT JOIN accounts a ON a.tenant_id = al.tenant_id AND a.id = al.actor_account_id
LEFT JOIN employees e ON e.tenant_id = al.tenant_id AND e.id = a.employee_id
WHERE al.tenant_id = sqlc.arg(tenant_id)
  AND (
    sqlc.arg(operator_id)::text = ''
    OR (lower(sqlc.arg(operator_id)::text) = '__system__' AND btrim(al.actor_account_id) = '')
    OR lower(al.actor_account_id) = lower(sqlc.arg(operator_id)::text)
    OR lower(coalesce(a.id, '')) = lower(sqlc.arg(operator_id)::text)
    OR lower(coalesce(a.employee_id, '')) = lower(sqlc.arg(operator_id)::text)
    OR lower(coalesce(a.display_name, '')) = lower(sqlc.arg(operator_id)::text)
    OR lower(coalesce(a.email, '')) = lower(sqlc.arg(operator_id)::text)
    OR lower(coalesce(e.id, '')) = lower(sqlc.arg(operator_id)::text)
    OR lower(coalesce(e.employee_no, '')) = lower(sqlc.arg(operator_id)::text)
    OR lower(coalesce(e.name, '')) = lower(sqlc.arg(operator_id)::text)
  )
  AND (NOT sqlc.arg(has_from)::bool OR al.created_at >= sqlc.arg(from_time))
  AND (NOT sqlc.arg(has_to)::bool OR al.created_at < sqlc.arg(to_time))
  AND (
    sqlc.arg(type)::text = ''
    OR lower(
      CASE
        WHEN lower(al.resource || ' ' || al.action) LIKE '%employee%' THEN '員工管理'
        WHEN lower(al.resource || ' ' || al.action) LIKE '%org%' OR lower(al.resource || ' ' || al.action) LIKE '%position%' THEN '組織架構'
        WHEN lower(al.resource || ' ' || al.action) LIKE '%attendance%' OR lower(al.resource || ' ' || al.action) LIKE '%leave%' OR lower(al.resource || ' ' || al.action) LIKE '%clock%' OR lower(al.resource || ' ' || al.action) LIKE '%shift%' THEN '假勤制度'
        WHEN lower(al.resource || ' ' || al.action) LIKE '%form%' OR lower(al.resource || ' ' || al.action) LIKE '%workflow%' THEN '表單設計'
        WHEN lower(al.resource || ' ' || al.action) LIKE '%iam%' OR lower(al.resource || ' ' || al.action) LIKE '%authz%' OR lower(al.resource || ' ' || al.action) LIKE '%permission%' OR lower(al.resource || ' ' || al.action) LIKE '%admin%' THEN '管理員設定'
        ELSE '系統'
      END || ' ' || al.resource || ' ' || al.action
    ) LIKE '%' || lower(sqlc.arg(type)::text) || '%'
  )
  AND (
    sqlc.arg(keyword)::text = ''
    OR lower(
      coalesce(a.display_name, '') || ' ' ||
      coalesce(a.email, '') || ' ' ||
      coalesce(e.employee_no, '') || ' ' ||
      coalesce(e.name, '') || ' ' ||
      al.action || ' ' ||
      al.resource || ' ' ||
      coalesce(al.target, '') || ' ' ||
      coalesce(al.details::text, '')
    ) LIKE '%' || lower(sqlc.arg(keyword)::text) || '%'
  );

-- name: ListAuditLogsPage :many
SELECT * FROM audit_logs
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'created_at_asc' THEN created_at END ASC,
  created_at DESC,
  id ASC
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;

-- name: ListAuditLogsFilteredPage :many
SELECT al.id, al.tenant_id, al.actor_account_id, al.action, al.resource, al.target, al.result, al.trace_id, al.severity, al.details, al.created_at
FROM audit_logs al
LEFT JOIN accounts a ON a.tenant_id = al.tenant_id AND a.id = al.actor_account_id
LEFT JOIN employees e ON e.tenant_id = al.tenant_id AND e.id = a.employee_id
WHERE al.tenant_id = sqlc.arg(tenant_id)
  AND (
    sqlc.arg(operator_id)::text = ''
    OR (lower(sqlc.arg(operator_id)::text) = '__system__' AND btrim(al.actor_account_id) = '')
    OR lower(al.actor_account_id) = lower(sqlc.arg(operator_id)::text)
    OR lower(coalesce(a.id, '')) = lower(sqlc.arg(operator_id)::text)
    OR lower(coalesce(a.employee_id, '')) = lower(sqlc.arg(operator_id)::text)
    OR lower(coalesce(a.display_name, '')) = lower(sqlc.arg(operator_id)::text)
    OR lower(coalesce(a.email, '')) = lower(sqlc.arg(operator_id)::text)
    OR lower(coalesce(e.id, '')) = lower(sqlc.arg(operator_id)::text)
    OR lower(coalesce(e.employee_no, '')) = lower(sqlc.arg(operator_id)::text)
    OR lower(coalesce(e.name, '')) = lower(sqlc.arg(operator_id)::text)
  )
  AND (NOT sqlc.arg(has_from)::bool OR al.created_at >= sqlc.arg(from_time))
  AND (NOT sqlc.arg(has_to)::bool OR al.created_at < sqlc.arg(to_time))
  AND (
    sqlc.arg(type)::text = ''
    OR lower(
      CASE
        WHEN lower(al.resource || ' ' || al.action) LIKE '%employee%' THEN '員工管理'
        WHEN lower(al.resource || ' ' || al.action) LIKE '%org%' OR lower(al.resource || ' ' || al.action) LIKE '%position%' THEN '組織架構'
        WHEN lower(al.resource || ' ' || al.action) LIKE '%attendance%' OR lower(al.resource || ' ' || al.action) LIKE '%leave%' OR lower(al.resource || ' ' || al.action) LIKE '%clock%' OR lower(al.resource || ' ' || al.action) LIKE '%shift%' THEN '假勤制度'
        WHEN lower(al.resource || ' ' || al.action) LIKE '%form%' OR lower(al.resource || ' ' || al.action) LIKE '%workflow%' THEN '表單設計'
        WHEN lower(al.resource || ' ' || al.action) LIKE '%iam%' OR lower(al.resource || ' ' || al.action) LIKE '%authz%' OR lower(al.resource || ' ' || al.action) LIKE '%permission%' OR lower(al.resource || ' ' || al.action) LIKE '%admin%' THEN '管理員設定'
        ELSE '系統'
      END || ' ' || al.resource || ' ' || al.action
    ) LIKE '%' || lower(sqlc.arg(type)::text) || '%'
  )
  AND (
    sqlc.arg(keyword)::text = ''
    OR lower(
      coalesce(a.display_name, '') || ' ' ||
      coalesce(a.email, '') || ' ' ||
      coalesce(e.employee_no, '') || ' ' ||
      coalesce(e.name, '') || ' ' ||
      al.action || ' ' ||
      al.resource || ' ' ||
      coalesce(al.target, '') || ' ' ||
      coalesce(al.details::text, '')
    ) LIKE '%' || lower(sqlc.arg(keyword)::text) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'created_at_asc' THEN al.created_at END ASC,
  al.created_at DESC,
  al.id ASC
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;
