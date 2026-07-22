-- +goose Up
-- Add durable retry scheduling and fenced leases to the shared outbox.

ALTER TABLE outbox_events
    ADD COLUMN payload_version integer NOT NULL DEFAULT 1 CHECK (payload_version > 0),
    ADD COLUMN idempotency_key text NOT NULL DEFAULT '',
    ADD COLUMN next_attempt_at timestamptz,
    ADD COLUMN attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    ADD COLUMN max_attempts integer NOT NULL DEFAULT 5 CHECK (max_attempts >= 0),
    ADD COLUMN claim_owner text NOT NULL DEFAULT '',
    ADD COLUMN claim_token text NOT NULL DEFAULT '',
    ADD COLUMN claim_expires_at timestamptz,
    ADD COLUMN last_attempt_at timestamptz,
    ADD COLUMN updated_at timestamptz,
    ADD COLUMN dead_lettered_at timestamptz;

UPDATE outbox_events
SET next_attempt_at = COALESCE(processed_at, created_at, NOW()),
    attempt_count = retry_count,
    updated_at = COALESCE(processed_at, created_at, NOW());

-- Rows claimed by an old binary have no lease and would otherwise remain stuck forever.
UPDATE outbox_events
SET status = 'failed',
    last_error = CASE
        WHEN btrim(last_error) = '' THEN 'legacy processing claim recovered during outbox reliability migration'
        ELSE last_error
    END,
    next_attempt_at = NOW(),
    claim_owner = '',
    claim_token = '',
    claim_expires_at = NULL,
    processed_at = NULL,
    updated_at = NOW()
WHERE status = 'processing';

ALTER TABLE outbox_events
    ALTER COLUMN next_attempt_at SET NOT NULL,
    ALTER COLUMN next_attempt_at SET DEFAULT NOW(),
    ALTER COLUMN updated_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT NOW(),
    DROP CONSTRAINT IF EXISTS outbox_events_status_check;

ALTER TABLE outbox_events
    ADD CONSTRAINT outbox_events_status_check
    CHECK (status IN ('pending', 'processing', 'succeeded', 'failed', 'parked', 'dead_lettered'));

-- Preserve the old finite retry limit without leaving already-exhausted rows
-- permanently stuck in a non-dispatchable failed state.
UPDATE outbox_events
SET status = 'dead_lettered',
    dead_lettered_at = COALESCE(processed_at, updated_at),
    processed_at = COALESCE(processed_at, updated_at),
    last_error = CASE
        WHEN btrim(last_error) = '' THEN 'attempt limit reached before outbox reliability migration'
        ELSE last_error
    END,
    updated_at = NOW()
WHERE status = 'failed'
  AND max_attempts > 0
  AND attempt_count >= max_attempts;

CREATE INDEX outbox_events_dispatch_due_idx
    ON outbox_events (tenant_id, next_attempt_at, created_at, id)
    WHERE status IN ('pending', 'failed');

CREATE INDEX outbox_events_expired_claim_idx
    ON outbox_events (tenant_id, claim_expires_at, created_at, id)
    WHERE status = 'processing';

CREATE UNIQUE INDEX outbox_events_idempotency_idx
    ON outbox_events (tenant_id, event_type, idempotency_key)
    WHERE idempotency_key <> '';

-- +goose Down
DROP INDEX IF EXISTS outbox_events_idempotency_idx;
DROP INDEX IF EXISTS outbox_events_expired_claim_idx;
DROP INDEX IF EXISTS outbox_events_dispatch_due_idx;

ALTER TABLE outbox_events
    DROP CONSTRAINT IF EXISTS outbox_events_status_check;

UPDATE outbox_events
SET status = 'failed',
    retry_count = GREATEST(retry_count, attempt_count),
    last_error = CASE
        WHEN btrim(last_error) = '' THEN 'dead-letter state normalized by migration rollback'
        ELSE last_error
    END,
    processed_at = NULL
WHERE status = 'dead_lettered';

ALTER TABLE outbox_events
    ADD CONSTRAINT outbox_events_status_check
    CHECK (status IN ('pending', 'processing', 'succeeded', 'failed', 'parked')),
    DROP COLUMN dead_lettered_at,
    DROP COLUMN updated_at,
    DROP COLUMN last_attempt_at,
    DROP COLUMN claim_expires_at,
    DROP COLUMN claim_token,
    DROP COLUMN claim_owner,
    DROP COLUMN max_attempts,
    DROP COLUMN attempt_count,
    DROP COLUMN next_attempt_at,
    DROP COLUMN idempotency_key,
    DROP COLUMN payload_version;
