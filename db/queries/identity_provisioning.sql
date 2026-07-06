-- name: AppendIdentityProvisioningOutboxEvent :one
INSERT INTO identity_provisioning_outbox (
    id, tenant_id, account_id, employee_id, employee_no, email,
    display_name, enabled, send_invite, status, retry_count,
    last_error, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
)
RETURNING *;

-- name: ListPendingIdentityProvisioningOutboxEvents :many
SELECT * FROM identity_provisioning_outbox
WHERE tenant_id = $1 AND status = 'pending'
ORDER BY created_at ASC;

-- name: UpdateIdentityProvisioningOutboxEvent :one
UPDATE identity_provisioning_outbox
SET status = $3,
    retry_count = $4,
    last_error = $5,
    updated_at = $6
WHERE tenant_id = $1
  AND id = $2
RETURNING *;
