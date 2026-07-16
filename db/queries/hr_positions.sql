-- name: UpsertPosition :one
INSERT INTO positions (
    id, tenant_id, code, name, name_en, org_unit_id, level, status, description, source, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
ON CONFLICT (id) DO UPDATE SET
    code = EXCLUDED.code,
    name = EXCLUDED.name,
    name_en = EXCLUDED.name_en,
    org_unit_id = EXCLUDED.org_unit_id,
    level = EXCLUDED.level,
    status = EXCLUDED.status,
    description = EXCLUDED.description,
    source = EXCLUDED.source,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at
WHERE positions.tenant_id = EXCLUDED.tenant_id
RETURNING *;

-- name: GetPosition :one
SELECT * FROM positions
WHERE tenant_id = $1 AND id = $2;

-- name: GetPositionByCode :one
SELECT * FROM positions
WHERE tenant_id = $1 AND lower(code) = lower($2);

-- name: GetPositionByName :one
SELECT * FROM positions
WHERE tenant_id = $1 AND lower(name) = lower($2);

-- name: ListPositions :many
SELECT * FROM positions
WHERE tenant_id = $1
ORDER BY
  CASE WHEN status = 'active' THEN 0 ELSE 1 END,
  name ASC,
  created_at ASC,
  id ASC;
