-- +goose Up
CREATE UNIQUE INDEX IF NOT EXISTS agent_runs_active_session_unique
    ON agent_runs (tenant_id, session_id)
    WHERE session_id <> '' AND status IN ('queued', 'running');

-- +goose Down
DROP INDEX IF EXISTS agent_runs_active_session_unique;
