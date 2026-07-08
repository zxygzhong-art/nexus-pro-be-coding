-- +goose Up
CREATE TABLE attendance_daily_summaries (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    work_date text NOT NULL,
    shift_start text NOT NULL DEFAULT '',
    shift_end text NOT NULL DEFAULT '',
    shift_hours double precision NOT NULL DEFAULT 0,
    daily_hours double precision NOT NULL DEFAULT 0,
    clock_hours double precision NOT NULL DEFAULT 0,
    source text NOT NULL DEFAULT 'manual',
    external_ref text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT attendance_daily_summaries_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT attendance_daily_summaries_employee_date_idx UNIQUE (tenant_id, employee_id, work_date)
);

CREATE INDEX attendance_daily_summaries_tenant_employee_date_idx ON attendance_daily_summaries (tenant_id, employee_id, work_date DESC);
CREATE INDEX attendance_daily_summaries_tenant_source_date_idx ON attendance_daily_summaries (tenant_id, source, work_date DESC);
CREATE UNIQUE INDEX attendance_daily_summaries_external_ref_idx ON attendance_daily_summaries (tenant_id, external_ref) WHERE external_ref <> '';

ALTER TABLE attendance_daily_summaries ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_daily_summaries FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_attendance_daily_summaries ON attendance_daily_summaries USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- +goose Down
DROP TABLE IF EXISTS attendance_daily_summaries;
