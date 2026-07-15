-- name: AppendIdentityProvisioningOutboxEvent :one
INSERT INTO identity_provisioning_outbox (
    id, tenant_id, account_id, employee_id, employee_no, email,
    display_name, enabled, send_invite, status, retry_count,
    last_error, next_attempt_at, claim_expires_at, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(account_id), sqlc.arg(employee_id), sqlc.arg(employee_no), sqlc.arg(email),
    sqlc.arg(display_name), sqlc.arg(enabled), sqlc.arg(send_invite), sqlc.arg(status), sqlc.arg(retry_count),
    sqlc.arg(last_error), sqlc.arg(next_attempt_at), sqlc.narg(claim_expires_at), sqlc.arg(created_at), sqlc.arg(updated_at)
)
RETURNING *;

-- name: ListPendingIdentityProvisioningOutboxEvents :many
SELECT * FROM identity_provisioning_outbox
WHERE tenant_id = $1 AND status = 'pending'
ORDER BY created_at ASC;

-- name: ClaimIdentityProvisioningOutboxEvents :many
WITH candidates AS (
    SELECT id
    FROM identity_provisioning_outbox
    WHERE tenant_id = sqlc.arg(tenant_id)
      AND identity_provisioning_outbox.retry_count < sqlc.arg(max_retries)
      AND (
        (identity_provisioning_outbox.status = 'pending' AND identity_provisioning_outbox.next_attempt_at <= sqlc.arg(claimed_at))
        OR (identity_provisioning_outbox.status = 'processing' AND identity_provisioning_outbox.claim_expires_at <= sqlc.arg(claimed_at))
      )
    ORDER BY identity_provisioning_outbox.next_attempt_at ASC, identity_provisioning_outbox.created_at ASC
    FOR UPDATE SKIP LOCKED
    LIMIT sqlc.arg(batch_size)
)
UPDATE identity_provisioning_outbox AS event
SET status = 'processing',
    claim_expires_at = sqlc.arg(lease_until),
    updated_at = sqlc.arg(claimed_at)
FROM candidates
WHERE event.tenant_id = sqlc.arg(tenant_id)
  AND event.id = candidates.id
RETURNING event.*;

-- name: UpdateIdentityProvisioningOutboxEvent :one
UPDATE identity_provisioning_outbox
SET status = $3,
    retry_count = $4,
    last_error = $5,
    next_attempt_at = $6,
    claim_expires_at = $7,
    updated_at = $8
WHERE tenant_id = $1
  AND id = $2
RETURNING *;
