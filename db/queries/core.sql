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

-- name: GetOutboxEventByID :one
SELECT * FROM outbox_events
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
SELECT * FROM outbox_events
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
-- Atomically claim a batch of dispatchable outbox rows for multi-replica workers.
UPDATE outbox_events AS claimed
SET status = 'processing',
    last_error = '',
    processed_at = NULL
WHERE claimed.tenant_id = sqlc.arg(tenant_id)
  AND claimed.id IN (
    SELECT candidate.id
    FROM outbox_events AS candidate
    WHERE candidate.tenant_id = sqlc.arg(tenant_id)
      AND candidate.status IN ('pending', 'failed')
      AND candidate.retry_count < sqlc.arg(max_retries)
    ORDER BY candidate.created_at ASC, candidate.id ASC
    FOR UPDATE SKIP LOCKED
    LIMIT sqlc.arg(batch_limit)
  )
RETURNING claimed.*;

-- name: UpdateOutboxEvent :one
UPDATE outbox_events
SET status = $3,
    retry_count = $4,
    last_error = $5,
    processed_at = $6
WHERE tenant_id = $1
  AND id = $2
RETURNING *;

-- name: DeleteSucceededOutboxEventsBefore :execrows
DELETE FROM outbox_events
WHERE tenant_id = sqlc.arg(tenant_id)
  AND status = 'succeeded'
  AND created_at < sqlc.arg(before);

-- name: UpsertAttendancePolicy :one
INSERT INTO attendance_policies (
    id, tenant_id, work_time, leave_types, version, effective_from, updated_by_account_id, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(work_time)::jsonb,
    sqlc.arg(leave_types)::jsonb, sqlc.arg(version), sqlc.narg(effective_from),
    sqlc.arg(updated_by_account_id),
    sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (tenant_id) DO UPDATE SET
    id = EXCLUDED.id,
    work_time = EXCLUDED.work_time,
    version = EXCLUDED.version,
    effective_from = EXCLUDED.effective_from,
    updated_by_account_id = EXCLUDED.updated_by_account_id,
    updated_at = EXCLUDED.updated_at
WHERE attendance_policies.version < EXCLUDED.version
RETURNING *;

-- name: GetAttendancePolicy :one
SELECT * FROM attendance_policies
WHERE tenant_id = sqlc.arg(tenant_id);

-- name: UpsertLeaveBalance :one
INSERT INTO leave_balances (
    id, tenant_id, employee_id, leave_type, leave_type_id, remaining_hours,
    period_start, period_end, granted_hours, used_hours, source, policy_version, prorate_ratio,
    updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(employee_id), sqlc.arg(leave_type), sqlc.arg(leave_type_id), sqlc.arg(remaining_hours)::numeric(12,2),
    sqlc.narg(period_start), sqlc.narg(period_end), sqlc.arg(granted_hours)::numeric(12,2), sqlc.arg(used_hours)::numeric(12,2), sqlc.arg(source), sqlc.arg(policy_version), sqlc.arg(prorate_ratio),
    sqlc.arg(updated_at)
)
ON CONFLICT (tenant_id, employee_id, leave_type, period_start, period_end) DO UPDATE SET
    leave_type_id = EXCLUDED.leave_type_id,
    remaining_hours = EXCLUDED.remaining_hours,
    granted_hours = EXCLUDED.granted_hours,
    used_hours = EXCLUDED.used_hours,
    source = EXCLUDED.source,
    policy_version = EXCLUDED.policy_version,
    prorate_ratio = EXCLUDED.prorate_ratio,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetLeaveTypeExternalMapping :one
SELECT mapping.*, leave_type.code AS leave_type_code
FROM leave_type_external_mappings mapping
JOIN leave_types leave_type
  ON leave_type.tenant_id = mapping.tenant_id AND leave_type.id = mapping.leave_type_id
WHERE mapping.tenant_id = sqlc.arg(tenant_id)
  AND lower(mapping.source) = lower(sqlc.arg(source)::text)
  AND lower(mapping.external_code) = lower(sqlc.arg(external_code)::text)
  AND (mapping.effective_from IS NULL OR mapping.effective_from <= sqlc.arg(as_of)::date)
  AND (mapping.effective_to IS NULL OR mapping.effective_to > sqlc.arg(as_of)::date)
ORDER BY mapping.effective_from DESC NULLS LAST, mapping.updated_at DESC
LIMIT 1;

-- name: ListLeaveTypeExternalMappings :many
SELECT mapping.*, leave_type.code AS leave_type_code
FROM leave_type_external_mappings mapping
JOIN leave_types leave_type
  ON leave_type.tenant_id = mapping.tenant_id AND leave_type.id = mapping.leave_type_id
WHERE mapping.tenant_id = sqlc.arg(tenant_id)
ORDER BY lower(mapping.source), lower(mapping.external_code), mapping.effective_from DESC NULLS LAST, mapping.updated_at DESC;

-- name: UpsertLeaveTypeExternalMapping :exec
INSERT INTO leave_type_external_mappings (
    id, tenant_id, source, external_code, leave_type_id,
    effective_from, effective_to, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(source), sqlc.arg(external_code), sqlc.arg(leave_type_id),
    sqlc.narg(effective_from), sqlc.narg(effective_to), sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (id) DO UPDATE SET
    source = EXCLUDED.source,
    external_code = EXCLUDED.external_code,
    leave_type_id = EXCLUDED.leave_type_id,
    effective_from = EXCLUDED.effective_from,
    effective_to = EXCLUDED.effective_to,
    updated_at = EXCLUDED.updated_at
WHERE leave_type_external_mappings.tenant_id = EXCLUDED.tenant_id;

-- name: EnsureLeaveTypeCatalog :exec
INSERT INTO leave_types (
    id, tenant_id, code, name, category, source_of_truth, status, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(code), sqlc.arg(code),
    'company', 'system_default', 'active', sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (tenant_id, id) DO NOTHING;

-- name: ExpireLeaveTypeExternalMapping :execrows
UPDATE leave_type_external_mappings
SET effective_to = GREATEST(coalesce(effective_from, sqlc.arg(effective_to)::date), sqlc.arg(effective_to)::date),
    updated_at = sqlc.arg(updated_at)
WHERE tenant_id = sqlc.arg(tenant_id) AND id = sqlc.arg(id);

-- name: UpsertLeaveTypeSyncIssue :exec
INSERT INTO leave_type_sync_issues (
    id, tenant_id, source, external_code, issue_code, message,
    occurrences, status, first_seen_at, last_seen_at, resolved_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(source), sqlc.arg(external_code), sqlc.arg(issue_code), sqlc.arg(message),
    1, 'open', sqlc.arg(first_seen_at), sqlc.arg(last_seen_at), NULL
)
ON CONFLICT (tenant_id, source, external_code, issue_code) DO UPDATE SET
    message = EXCLUDED.message,
    occurrences = leave_type_sync_issues.occurrences + 1,
    status = 'open',
    last_seen_at = EXCLUDED.last_seen_at,
    resolved_at = NULL;

-- name: ListOpenLeaveTypeSyncIssues :many
SELECT * FROM leave_type_sync_issues
WHERE tenant_id = sqlc.arg(tenant_id) AND status = 'open'
ORDER BY last_seen_at DESC, external_code ASC;

-- name: ResolveLeaveTypeSyncIssues :exec
UPDATE leave_type_sync_issues
SET status = 'resolved', resolved_at = sqlc.arg(resolved_at), last_seen_at = sqlc.arg(resolved_at)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND lower(source) = lower(sqlc.arg(source)::text)
  AND lower(external_code) = lower(sqlc.arg(external_code)::text)
  AND status = 'open';

-- name: GetLeaveBalance :one
SELECT * FROM leave_balances
WHERE tenant_id = $1 AND id = $2;

-- name: ListLeaveBalances :many
SELECT * FROM leave_balances
WHERE tenant_id = $1
ORDER BY updated_at ASC;

-- name: ReserveLeaveBalance :one
UPDATE leave_balances
SET remaining_hours = remaining_hours - sqlc.arg(hours)::numeric(12,2),
    used_hours = used_hours + sqlc.arg(hours)::numeric(12,2),
    updated_at = sqlc.arg(updated_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)
  AND employee_id = sqlc.arg(employee_id)
  AND lower(leave_type) = lower(sqlc.arg(leave_type)::text)
  AND (NULLIF(period_start::text, '') IS NULL OR NULLIF(period_start::text, '')::date <= sqlc.arg(as_of)::date)
  AND (NULLIF(period_end::text, '') IS NULL OR NULLIF(period_end::text, '')::date >= sqlc.arg(as_of)::date)
  AND remaining_hours >= sqlc.arg(hours)::numeric(12,2)
RETURNING *;

-- name: ReleaseLeaveBalanceByID :one
UPDATE leave_balances
SET remaining_hours = remaining_hours + sqlc.arg(hours)::numeric(12,2),
    used_hours = GREATEST(0, used_hours - sqlc.arg(hours)::numeric(12,2)),
    updated_at = sqlc.arg(updated_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(balance_id)
RETURNING *;

-- name: ReleaseLeaveBalance :one
UPDATE leave_balances
SET remaining_hours = remaining_hours + sqlc.arg(hours)::numeric(12,2),
    used_hours = GREATEST(0, used_hours - sqlc.arg(hours)::numeric(12,2)),
    updated_at = sqlc.arg(updated_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)
  AND employee_id = sqlc.arg(employee_id)
  AND lower(leave_type) = lower(sqlc.arg(leave_type)::text)
RETURNING *;

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
    id, tenant_id, key, name, description, schema, status, current_version, created_at, updated_at, deleted_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(key), sqlc.arg(name), sqlc.arg(description), sqlc.arg(schema)::jsonb,
    sqlc.arg(status), sqlc.arg(current_version), sqlc.arg(created_at), sqlc.arg(updated_at), sqlc.narg(deleted_at)
)
ON CONFLICT (id) DO UPDATE SET
    key = EXCLUDED.key,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    schema = EXCLUDED.schema,
    status = EXCLUDED.status,
    current_version = EXCLUDED.current_version,
    updated_at = EXCLUDED.updated_at,
    deleted_at = EXCLUDED.deleted_at
WHERE form_templates.tenant_id = EXCLUDED.tenant_id
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

-- name: InsertFormTemplateVersion :exec
INSERT INTO form_template_versions (
    id, tenant_id, template_id, version, schema, status, created_at, published_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(template_id), sqlc.arg(version), sqlc.arg(schema)::jsonb,
    sqlc.arg(status), sqlc.arg(created_at), sqlc.narg(published_at)
)
ON CONFLICT (tenant_id, template_id, version) DO NOTHING;

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

-- name: UpsertLeaveRequest :one
INSERT INTO leave_requests (
    id, tenant_id, employee_id, leave_type, leave_type_id, policy_version,
    rule_snapshot, evaluation_snapshot, start_at, end_at,
    hours, reason, status, form_instance_id, leave_balance_id, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(employee_id), sqlc.arg(leave_type), sqlc.arg(leave_type_id), sqlc.arg(policy_version),
    sqlc.arg(rule_snapshot)::jsonb, sqlc.arg(evaluation_snapshot)::jsonb, sqlc.arg(start_at), sqlc.arg(end_at),
    sqlc.arg(hours), sqlc.arg(reason), sqlc.arg(status), sqlc.arg(form_instance_id), sqlc.narg(leave_balance_id), sqlc.arg(created_at)
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    employee_id = EXCLUDED.employee_id,
    leave_type = EXCLUDED.leave_type,
    leave_type_id = EXCLUDED.leave_type_id,
    policy_version = EXCLUDED.policy_version,
    rule_snapshot = EXCLUDED.rule_snapshot,
    evaluation_snapshot = EXCLUDED.evaluation_snapshot,
    start_at = EXCLUDED.start_at,
    end_at = EXCLUDED.end_at,
    hours = EXCLUDED.hours,
    reason = EXCLUDED.reason,
    status = EXCLUDED.status,
    form_instance_id = EXCLUDED.form_instance_id,
    leave_balance_id = EXCLUDED.leave_balance_id,
    created_at = EXCLUDED.created_at
RETURNING *;

-- name: UpsertLeaveRequestAllocation :one
INSERT INTO leave_request_allocations (
    tenant_id, leave_request_id, leave_balance_id, reserved_hours, created_at
) VALUES (
    sqlc.arg(tenant_id), sqlc.arg(leave_request_id), sqlc.arg(leave_balance_id),
    sqlc.arg(reserved_hours)::numeric(12,2), sqlc.arg(created_at)
)
ON CONFLICT (tenant_id, leave_request_id, leave_balance_id) DO UPDATE SET
    reserved_hours = EXCLUDED.reserved_hours
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

-- name: UpsertWorkflowRun :one
INSERT INTO workflow_runs (
    id, tenant_id, form_instance_id, template_id, version, status,
    current_stage_instance_id, stage_definitions_json, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    form_instance_id = EXCLUDED.form_instance_id,
    template_id = EXCLUDED.template_id,
    version = EXCLUDED.version,
    status = EXCLUDED.status,
    current_stage_instance_id = EXCLUDED.current_stage_instance_id,
    stage_definitions_json = EXCLUDED.stage_definitions_json,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetWorkflowRun :one
SELECT * FROM workflow_runs
WHERE tenant_id = $1 AND id = $2;

-- name: ListWorkflowRunsByFormInstance :many
SELECT * FROM workflow_runs
WHERE tenant_id = $1 AND form_instance_id = $2
ORDER BY version ASC, created_at ASC;

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
    id, tenant_id, run_id, stage_instance_id, account_id, action, comment, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: ListWorkflowActionsByRun :many
SELECT * FROM workflow_actions
WHERE tenant_id = $1 AND run_id = $2
ORDER BY created_at ASC;


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
    work_date, direction, client_event_id, clocked_at, latitude, longitude, accuracy_meters,
    distance_meters, record_status, rejection_reason, source, device_id,
    device_info, correction_request_id, voided, voided_at, voided_by_account_id,
    void_reason, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
    $13, $14, $15, $16, $17, $18, $19::jsonb, $20, $21, $22, $23,
    $24, $25
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    employee_id = EXCLUDED.employee_id,
    shift_assignment_id = EXCLUDED.shift_assignment_id,
    shift_id = EXCLUDED.shift_id,
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
    void_reason = EXCLUDED.void_reason,
    created_at = EXCLUDED.created_at
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
  AND work_date = $3
  AND direction = 'clock_in'
  AND record_status = 'accepted'
  AND voided = false
ORDER BY clocked_at ASC, created_at ASC, id ASC
LIMIT 1;

-- name: GetLatestAcceptedAttendanceClockOut :one
SELECT * FROM attendance_clock_records
WHERE tenant_id = $1
  AND employee_id = $2
  AND work_date = $3
  AND direction = 'clock_out'
  AND record_status = 'accepted'
  AND voided = false
ORDER BY clocked_at DESC, created_at DESC, id DESC
LIMIT 1;

-- name: GetLatestAcceptedAttendanceClockRecord :one
SELECT * FROM attendance_clock_records
WHERE tenant_id = $1
  AND employee_id = $2
  AND work_date = $3
  AND record_status = 'accepted'
  AND voided = false
ORDER BY clocked_at DESC, created_at DESC, id DESC
LIMIT 1;

-- name: ListAttendanceClockRecords :many
SELECT * FROM attendance_clock_records
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(employee_id)::text = '' OR employee_id = sqlc.arg(employee_id))
  AND (coalesce(cardinality(sqlc.arg(employee_ids)::text[]), 0) = 0 OR employee_id = ANY(sqlc.arg(employee_ids)::text[]))
  AND (sqlc.arg(from_date)::text = '' OR work_date >= sqlc.arg(from_date))
  AND (sqlc.arg(to_date)::text = '' OR work_date <= sqlc.arg(to_date))
  AND (sqlc.arg(direction)::text = '' OR direction = sqlc.arg(direction))
  AND (sqlc.arg(record_status)::text = '' OR record_status = sqlc.arg(record_status))
  AND (sqlc.arg(source)::text = '' OR source = sqlc.arg(source))
ORDER BY clocked_at DESC, created_at DESC, id ASC;

-- name: UpsertAttendanceDailySummary :one
INSERT INTO attendance_daily_summaries (
    id, tenant_id, employee_id, work_date, shift_start, shift_end,
    shift_hours, daily_hours, clock_hours, clock_start, clock_end, attend_start,
    attend_end, attend_hours, attend_counted, leave_type, leave_start, leave_end,
    leave_hours, leave_counted, leave2_type, leave2_start, leave2_end, leave2_hours,
    leave2_counted, overtime_start, overtime_end, overtime_hours, overtime_counted,
    payload, source, external_ref, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
    $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24,
    $25, $26, $27, $28, $29, $30::jsonb, $31, $32, $33, $34
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    employee_id = EXCLUDED.employee_id,
    work_date = EXCLUDED.work_date,
    shift_start = EXCLUDED.shift_start,
    shift_end = EXCLUDED.shift_end,
    shift_hours = EXCLUDED.shift_hours,
    daily_hours = EXCLUDED.daily_hours,
    clock_hours = EXCLUDED.clock_hours,
    clock_start = EXCLUDED.clock_start,
    clock_end = EXCLUDED.clock_end,
    attend_start = EXCLUDED.attend_start,
    attend_end = EXCLUDED.attend_end,
    attend_hours = EXCLUDED.attend_hours,
    attend_counted = EXCLUDED.attend_counted,
    leave_type = EXCLUDED.leave_type,
    leave_start = EXCLUDED.leave_start,
    leave_end = EXCLUDED.leave_end,
    leave_hours = EXCLUDED.leave_hours,
    leave_counted = EXCLUDED.leave_counted,
    leave2_type = EXCLUDED.leave2_type,
    leave2_start = EXCLUDED.leave2_start,
    leave2_end = EXCLUDED.leave2_end,
    leave2_hours = EXCLUDED.leave2_hours,
    leave2_counted = EXCLUDED.leave2_counted,
    overtime_start = EXCLUDED.overtime_start,
    overtime_end = EXCLUDED.overtime_end,
    overtime_hours = EXCLUDED.overtime_hours,
    overtime_counted = EXCLUDED.overtime_counted,
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
WHERE tenant_id = $1 AND employee_id = $2 AND work_date = $3
LIMIT 1;

-- name: ListAttendanceDailySummaries :many
SELECT * FROM attendance_daily_summaries
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(employee_id)::text = '' OR employee_id = sqlc.arg(employee_id))
  AND (coalesce(cardinality(sqlc.arg(employee_ids)::text[]), 0) = 0 OR employee_id = ANY(sqlc.arg(employee_ids)::text[]))
  AND (sqlc.arg(from_date)::text = '' OR work_date >= sqlc.arg(from_date))
  AND (sqlc.arg(to_date)::text = '' OR work_date <= sqlc.arg(to_date))
  AND (sqlc.arg(source)::text = '' OR source = sqlc.arg(source))
ORDER BY work_date ASC, employee_id ASC, id ASC;

-- name: UpsertAttendanceCorrectionRequest :one
INSERT INTO attendance_correction_requests (
    id, tenant_id, employee_id, direction, requested_clocked_at, work_date,
    correction_type, target_clock_record_id, replacement_clock_record_id, reason,
    status, form_instance_id, clock_record_id, reviewed_by_account_id,
    review_reason, reviewed_at, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
    $15, $16, $17, $18
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    employee_id = EXCLUDED.employee_id,
    direction = EXCLUDED.direction,
    requested_clocked_at = EXCLUDED.requested_clocked_at,
    work_date = EXCLUDED.work_date,
    correction_type = EXCLUDED.correction_type,
    target_clock_record_id = EXCLUDED.target_clock_record_id,
    replacement_clock_record_id = EXCLUDED.replacement_clock_record_id,
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

-- name: GetAttendanceCorrectionRequestByFormInstanceID :one
SELECT * FROM attendance_correction_requests
WHERE tenant_id = sqlc.arg(tenant_id) AND form_instance_id = sqlc.arg(form_instance_id)
LIMIT 1;

-- name: UpsertOvertimeRequest :one
INSERT INTO overtime_requests (
    id, tenant_id, employee_id, work_date, start_at, end_at,
    hours, overtime_type, compensation_type, reason, status,
    form_instance_id, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    employee_id = EXCLUDED.employee_id,
    work_date = EXCLUDED.work_date,
    start_at = EXCLUDED.start_at,
    end_at = EXCLUDED.end_at,
    hours = EXCLUDED.hours,
    overtime_type = EXCLUDED.overtime_type,
    compensation_type = EXCLUDED.compensation_type,
    reason = EXCLUDED.reason,
    status = EXCLUDED.status,
    form_instance_id = EXCLUDED.form_instance_id,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetOvertimeRequest :one
SELECT * FROM overtime_requests
WHERE tenant_id = $1 AND id = $2;

-- name: GetOvertimeRequestByFormInstanceID :one
SELECT * FROM overtime_requests
WHERE tenant_id = sqlc.arg(tenant_id) AND form_instance_id = sqlc.arg(form_instance_id)
LIMIT 1;

-- name: ListOvertimeRequestsByQuery :many
SELECT * FROM overtime_requests
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (coalesce(cardinality(sqlc.arg(employee_ids)::text[]), 0) = 0 OR employee_id = ANY(sqlc.arg(employee_ids)::text[]))
  AND (sqlc.arg(status)::text = '' OR lower(status) = lower(sqlc.arg(status)::text))
  AND (NULLIF(sqlc.arg(from_date)::text, '') IS NULL OR end_at::date >= NULLIF(sqlc.arg(from_date)::text, '')::date)
  AND (NULLIF(sqlc.arg(to_date)::text, '') IS NULL OR start_at::date <= NULLIF(sqlc.arg(to_date)::text, '')::date)
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
INSERT INTO agent_runs (
    id, tenant_id, account_id, agent_id, session_id, mode, prompt, answer,
    status, reference_items, llm_call_count, input_tokens, cached_tokens,
    output_tokens, total_tokens, usage_complete, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(account_id), sqlc.arg(agent_id),
    sqlc.arg(session_id), sqlc.arg(mode), sqlc.arg(prompt), sqlc.arg(answer),
    sqlc.arg(status), sqlc.arg(reference_items)::jsonb, sqlc.arg(llm_call_count),
    sqlc.arg(input_tokens), sqlc.arg(cached_tokens), sqlc.arg(output_tokens),
    sqlc.arg(total_tokens), sqlc.arg(usage_complete), sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    account_id = EXCLUDED.account_id,
    agent_id = EXCLUDED.agent_id,
    session_id = EXCLUDED.session_id,
    mode = EXCLUDED.mode,
    prompt = EXCLUDED.prompt,
    answer = EXCLUDED.answer,
    status = EXCLUDED.status,
    reference_items = EXCLUDED.reference_items,
    llm_call_count = EXCLUDED.llm_call_count,
    input_tokens = EXCLUDED.input_tokens,
    cached_tokens = EXCLUDED.cached_tokens,
    output_tokens = EXCLUDED.output_tokens,
    total_tokens = EXCLUDED.total_tokens,
    usage_complete = EXCLUDED.usage_complete,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at
RETURNING *;

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
