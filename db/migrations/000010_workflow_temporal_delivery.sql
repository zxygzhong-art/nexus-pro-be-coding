-- +goose Up
-- Track durable Temporal delivery independently from the business workflow status.

ALTER TABLE workflow_runs
    ADD COLUMN temporal_start_status text NOT NULL DEFAULT 'started'
        CHECK (temporal_start_status IN ('pending_start', 'starting', 'started', 'abandoned')),
    ADD COLUMN temporal_workflow_id text NOT NULL DEFAULT '',
    ADD COLUMN temporal_run_id text NOT NULL DEFAULT '',
    ADD COLUMN temporal_start_event_id text NOT NULL DEFAULT '',
    ADD COLUMN temporal_started_at timestamptz;

UPDATE workflow_runs
SET temporal_start_status = CASE
        WHEN status = 'start_failed' THEN 'abandoned'
        ELSE 'started'
    END,
    temporal_workflow_id = tenant_id || ':' || form_instance_id,
    temporal_started_at = CASE
        WHEN status = 'start_failed' THEN NULL
        ELSE updated_at
    END;

CREATE INDEX workflow_runs_temporal_start_claimable_idx
    ON workflow_runs (tenant_id, updated_at, id)
    WHERE temporal_start_status IN ('pending_start', 'starting');

CREATE UNIQUE INDEX workflow_runs_temporal_start_event_uidx
    ON workflow_runs (tenant_id, temporal_start_event_id)
    WHERE temporal_start_event_id <> '';

ALTER TABLE workflow_actions
    ADD COLUMN idempotency_key text NOT NULL DEFAULT '',
    ADD COLUMN command_fingerprint text NOT NULL DEFAULT '',
    ADD COLUMN request_id text NOT NULL DEFAULT '',
    ADD COLUMN trace_id text NOT NULL DEFAULT '';

CREATE UNIQUE INDEX workflow_actions_run_idempotency_uidx
    ON workflow_actions (tenant_id, run_id, idempotency_key)
    WHERE idempotency_key <> '';

-- +goose Down
-- Refuse to erase delivery identity while an execution can still receive a
-- command. Operators must disable new starts and drain these rows first.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM workflow_runs
        WHERE temporal_start_status IN ('pending_start', 'starting')
           OR (
                status = 'running'
                AND temporal_workflow_id <> ''
                AND temporal_workflow_id <> tenant_id || ':' || form_instance_id
           )
    ) THEN
        RAISE EXCEPTION 'cannot downgrade 000010 while Temporal starts are pending/starting or run-scoped executions are active';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM outbox_events
        WHERE event_type = 'workflow.form_approval.start_requested'
          AND status <> 'succeeded'
    ) THEN
        RAISE EXCEPTION 'cannot downgrade 000010 while workflow start outbox events are undrained';
    END IF;
END $$;
-- +goose StatementEnd

DROP INDEX IF EXISTS workflow_actions_run_idempotency_uidx;

ALTER TABLE workflow_actions
    DROP COLUMN IF EXISTS trace_id,
    DROP COLUMN IF EXISTS request_id,
    DROP COLUMN IF EXISTS command_fingerprint,
    DROP COLUMN IF EXISTS idempotency_key;

DROP INDEX IF EXISTS workflow_runs_temporal_start_event_uidx;
DROP INDEX IF EXISTS workflow_runs_temporal_start_claimable_idx;

ALTER TABLE workflow_runs
    DROP COLUMN IF EXISTS temporal_started_at,
    DROP COLUMN IF EXISTS temporal_start_event_id,
    DROP COLUMN IF EXISTS temporal_run_id,
    DROP COLUMN IF EXISTS temporal_workflow_id,
    DROP COLUMN IF EXISTS temporal_start_status;
