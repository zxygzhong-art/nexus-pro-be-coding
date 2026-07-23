-- name: UpsertFormBusinessRecord :one
INSERT INTO form_business_records (
    id, tenant_id, form_instance_id, business_type, schema_version,
    subject_employee_id, effective_from, effective_to, business_date,
    data, effect_status, result, last_error, handler_key, handler_version,
    applied_at, lock_version, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(form_instance_id), sqlc.arg(business_type), sqlc.arg(schema_version),
    sqlc.narg(subject_employee_id), sqlc.narg(effective_from), sqlc.narg(effective_to), sqlc.narg(business_date),
    sqlc.arg(data)::jsonb, sqlc.arg(effect_status), sqlc.arg(result)::jsonb, sqlc.arg(last_error)::jsonb,
    sqlc.arg(handler_key), sqlc.arg(handler_version), sqlc.narg(applied_at), sqlc.arg(lock_version),
    sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (id) DO UPDATE SET
    form_instance_id = EXCLUDED.form_instance_id,
    business_type = EXCLUDED.business_type,
    schema_version = EXCLUDED.schema_version,
    subject_employee_id = EXCLUDED.subject_employee_id,
    effective_from = EXCLUDED.effective_from,
    effective_to = EXCLUDED.effective_to,
    business_date = EXCLUDED.business_date,
    data = EXCLUDED.data,
    effect_status = EXCLUDED.effect_status,
    result = EXCLUDED.result,
    last_error = EXCLUDED.last_error,
    handler_key = EXCLUDED.handler_key,
    handler_version = EXCLUDED.handler_version,
    applied_at = EXCLUDED.applied_at,
    lock_version = form_business_records.lock_version + 1,
    updated_at = EXCLUDED.updated_at
WHERE form_business_records.tenant_id = EXCLUDED.tenant_id
RETURNING *;

-- name: GetFormBusinessRecord :one
SELECT sqlc.embed(record), instance.status AS form_status,
       instance.approved_by AS form_approved_by, instance.updated_at AS form_updated_at
FROM form_business_records record
JOIN form_instances instance
  ON instance.tenant_id = record.tenant_id AND instance.id = record.form_instance_id
WHERE record.tenant_id = sqlc.arg(tenant_id) AND record.id = sqlc.arg(id);

-- name: GetFormBusinessRecordForUpdate :one
SELECT sqlc.embed(record), instance.status AS form_status,
       instance.approved_by AS form_approved_by, instance.updated_at AS form_updated_at
FROM form_business_records record
JOIN form_instances instance
  ON instance.tenant_id = record.tenant_id AND instance.id = record.form_instance_id
WHERE record.tenant_id = sqlc.arg(tenant_id) AND record.id = sqlc.arg(id)
FOR UPDATE OF record;

-- name: GetFormBusinessRecordByFormType :one
SELECT sqlc.embed(record), instance.status AS form_status,
       instance.approved_by AS form_approved_by, instance.updated_at AS form_updated_at
FROM form_business_records record
JOIN form_instances instance
  ON instance.tenant_id = record.tenant_id AND instance.id = record.form_instance_id
WHERE record.tenant_id = sqlc.arg(tenant_id)
  AND record.form_instance_id = sqlc.arg(form_instance_id)
  AND record.business_type = sqlc.arg(business_type)
LIMIT 1;

-- name: ListFormBusinessRecordsByType :many
SELECT sqlc.embed(record), instance.status AS form_status,
       instance.approved_by AS form_approved_by, instance.updated_at AS form_updated_at
FROM form_business_records record
JOIN form_instances instance
  ON instance.tenant_id = record.tenant_id AND instance.id = record.form_instance_id
WHERE record.tenant_id = sqlc.arg(tenant_id)
  AND record.business_type = sqlc.arg(business_type)
  AND (
      sqlc.arg(status)::text = ''
      OR lower(CASE
          WHEN lower(instance.status) IN ('approved', 'rejected', 'cancelled') THEN lower(instance.status)
          WHEN lower(instance.status) = 'returned' THEN 'rejected'
          WHEN record.business_type = 'attendance.clock_correction' THEN 'pending'
          ELSE 'pending_approval'
      END) = lower(sqlc.arg(status)::text)
  )
  AND (
      coalesce(cardinality(sqlc.arg(subject_employee_ids)::text[]), 0) = 0
      OR record.subject_employee_id = ANY(sqlc.arg(subject_employee_ids)::text[])
  )
  AND (
      NULLIF(sqlc.arg(from_date)::text, '') IS NULL
      OR coalesce(record.effective_to::date, record.business_date) >= NULLIF(sqlc.arg(from_date)::text, '')::date
  )
  AND (
      NULLIF(sqlc.arg(to_date)::text, '') IS NULL
      OR coalesce(record.effective_from::date, record.business_date) <= NULLIF(sqlc.arg(to_date)::text, '')::date
  )
ORDER BY record.created_at DESC, record.id ASC;

-- name: CountFormBusinessRecordsByType :one
SELECT count(*)
FROM form_business_records record
JOIN form_instances instance
  ON instance.tenant_id = record.tenant_id AND instance.id = record.form_instance_id
WHERE record.tenant_id = sqlc.arg(tenant_id)
  AND record.business_type = sqlc.arg(business_type)
  AND (
      sqlc.arg(status)::text = ''
      OR lower(CASE
          WHEN lower(instance.status) IN ('approved', 'rejected', 'cancelled') THEN lower(instance.status)
          WHEN lower(instance.status) = 'returned' THEN 'rejected'
          WHEN record.business_type = 'attendance.clock_correction' THEN 'pending'
          ELSE 'pending_approval'
      END) = lower(sqlc.arg(status)::text)
  )
  AND (
      coalesce(cardinality(sqlc.arg(subject_employee_ids)::text[]), 0) = 0
      OR record.subject_employee_id = ANY(sqlc.arg(subject_employee_ids)::text[])
  )
  AND (
      NULLIF(sqlc.arg(from_date)::text, '') IS NULL
      OR coalesce(record.effective_to::date, record.business_date) >= NULLIF(sqlc.arg(from_date)::text, '')::date
  )
  AND (
      NULLIF(sqlc.arg(to_date)::text, '') IS NULL
      OR coalesce(record.effective_from::date, record.business_date) <= NULLIF(sqlc.arg(to_date)::text, '')::date
  );

-- name: ListFormBusinessRecordPageByType :many
SELECT sqlc.embed(record), instance.status AS form_status,
       instance.approved_by AS form_approved_by, instance.updated_at AS form_updated_at
FROM form_business_records record
JOIN form_instances instance
  ON instance.tenant_id = record.tenant_id AND instance.id = record.form_instance_id
WHERE record.tenant_id = sqlc.arg(tenant_id)
  AND record.business_type = sqlc.arg(business_type)
  AND (
      sqlc.arg(status)::text = ''
      OR lower(CASE
          WHEN lower(instance.status) IN ('approved', 'rejected', 'cancelled') THEN lower(instance.status)
          WHEN lower(instance.status) = 'returned' THEN 'rejected'
          WHEN record.business_type = 'attendance.clock_correction' THEN 'pending'
          ELSE 'pending_approval'
      END) = lower(sqlc.arg(status)::text)
  )
  AND (
      coalesce(cardinality(sqlc.arg(subject_employee_ids)::text[]), 0) = 0
      OR record.subject_employee_id = ANY(sqlc.arg(subject_employee_ids)::text[])
  )
  AND (
      NULLIF(sqlc.arg(from_date)::text, '') IS NULL
      OR coalesce(record.effective_to::date, record.business_date) >= NULLIF(sqlc.arg(from_date)::text, '')::date
  )
  AND (
      NULLIF(sqlc.arg(to_date)::text, '') IS NULL
      OR coalesce(record.effective_from::date, record.business_date) <= NULLIF(sqlc.arg(to_date)::text, '')::date
  )
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'created_at_asc' THEN record.created_at END ASC,
  record.created_at DESC,
  record.id ASC
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;

-- name: ClaimFormBusinessRecordEffect :one
UPDATE form_business_records
SET effect_status = 'applying',
    result = result || sqlc.arg(claim_result)::jsonb,
    last_error = '{}'::jsonb,
    lock_version = lock_version + 1,
    updated_at = sqlc.arg(claimed_at)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND form_instance_id = sqlc.arg(form_instance_id)
  AND business_type = sqlc.arg(business_type)
  AND effect_status = 'not_applied'
RETURNING *;
