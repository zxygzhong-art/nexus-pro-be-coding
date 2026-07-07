-- name: UpsertEmploymentContract :one
INSERT INTO employment_contracts (
    id, tenant_id, employee_id, contract_type, contract_no, start_date, end_date,
    status, attachment_object_key, notes, version, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    employee_id = EXCLUDED.employee_id,
    contract_type = EXCLUDED.contract_type,
    contract_no = EXCLUDED.contract_no,
    start_date = EXCLUDED.start_date,
    end_date = EXCLUDED.end_date,
    status = EXCLUDED.status,
    attachment_object_key = EXCLUDED.attachment_object_key,
    notes = EXCLUDED.notes,
    version = EXCLUDED.version,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetEmploymentContract :one
SELECT * FROM employment_contracts
WHERE tenant_id = $1 AND id = $2;

-- name: ListEmploymentContractsByEmployee :many
SELECT * FROM employment_contracts
WHERE tenant_id = $1 AND employee_id = $2
ORDER BY start_date DESC, created_at DESC, id ASC;

-- name: ListEmploymentContracts :many
SELECT * FROM employment_contracts
WHERE tenant_id = $1
ORDER BY start_date DESC, created_at DESC, id ASC;
