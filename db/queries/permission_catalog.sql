-- name: UpsertPermissionCatalogItem :one
INSERT INTO permissions (
    id, tenant_id, application, resource, action, permission_type,
    menu_key, name, description, high_risk, severity, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(application), sqlc.arg(resource), sqlc.arg(action), sqlc.arg(permission_type),
    sqlc.arg(menu_key), sqlc.arg(name), sqlc.arg(description), sqlc.arg(high_risk), sqlc.arg(severity), sqlc.arg(created_at)
)
ON CONFLICT (tenant_id, application, resource, action, permission_type) DO UPDATE SET
    menu_key = EXCLUDED.menu_key,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    high_risk = EXCLUDED.high_risk,
    severity = EXCLUDED.severity
RETURNING *;

-- name: GetPermissionCatalogItemByKey :one
SELECT * FROM permissions
WHERE tenant_id = $1
  AND application = $2
  AND resource = $3
  AND action = $4
  AND permission_type = $5;

-- name: ListPermissionCatalogItems :many
SELECT * FROM permissions
WHERE tenant_id = $1
ORDER BY application ASC, resource ASC, action ASC, permission_type ASC;

-- name: DeletePermissionCatalogItem :exec
DELETE FROM permissions
WHERE tenant_id = $1 AND id = $2;

-- name: UpsertMenuItem :one
INSERT INTO menu_items (
    id, tenant_id, key, label, path, icon, parent_key, sort_order, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(key), sqlc.arg(label), sqlc.arg(path),
    sqlc.arg(icon), sqlc.arg(parent_key), sqlc.arg(sort_order), sqlc.arg(created_at)
)
ON CONFLICT (tenant_id, key) DO UPDATE SET
    label = EXCLUDED.label,
    path = EXCLUDED.path,
    icon = EXCLUDED.icon,
    parent_key = EXCLUDED.parent_key,
    sort_order = EXCLUDED.sort_order
RETURNING *;

-- name: ListMenuItems :many
SELECT * FROM menu_items
WHERE tenant_id = $1
ORDER BY parent_key ASC, sort_order ASC, key ASC;

-- name: DeleteMenuItem :exec
DELETE FROM menu_items
WHERE tenant_id = $1 AND id = $2;

-- name: DeletePermissionSetItemsForSet :exec
DELETE FROM permission_set_items
WHERE tenant_id = $1 AND permission_set_id = $2;

-- name: UpsertPermissionSetItem :one
INSERT INTO permission_set_items (
    id, tenant_id, permission_set_id, permission_id, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(permission_set_id), sqlc.arg(permission_id), sqlc.arg(created_at)
)
ON CONFLICT (tenant_id, permission_set_id, permission_id) DO UPDATE SET
    created_at = permission_set_items.created_at
RETURNING *;

-- name: ListPermissionSetItems :many
SELECT * FROM permission_set_items
WHERE tenant_id = $1
ORDER BY created_at ASC;

-- name: ListPermissionSetItemsForSet :many
SELECT * FROM permission_set_items
WHERE tenant_id = $1 AND permission_set_id = $2
ORDER BY created_at ASC;
