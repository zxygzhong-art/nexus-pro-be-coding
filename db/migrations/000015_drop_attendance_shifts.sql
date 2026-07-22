-- +goose Up
-- Retire unused per-employee shift templates and assignments.
-- Clock window / work-date rules continue via attendance_policies.work_time.

ALTER TABLE attendance_clock_records
    DROP CONSTRAINT IF EXISTS attendance_clock_records_shift_assignment_fk,
    DROP CONSTRAINT IF EXISTS attendance_clock_records_shift_fk;

ALTER TABLE attendance_clock_records
    DROP COLUMN IF EXISTS shift_assignment_id,
    DROP COLUMN IF EXISTS shift_id;

DROP POLICY IF EXISTS tenant_isolation_attendance_shift_assignments ON attendance_shift_assignments;
DROP POLICY IF EXISTS tenant_isolation_attendance_shifts ON attendance_shifts;

DROP TABLE IF EXISTS attendance_shift_assignments;
DROP TABLE IF EXISTS attendance_shifts;

-- +goose Down
CREATE TABLE attendance_shifts (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    clock_in_start text NOT NULL,
    clock_in_end text NOT NULL,
    clock_out_start text NOT NULL,
    clock_out_end text NOT NULL,
    late_grace_minutes integer NOT NULL DEFAULT 0 CHECK (late_grace_minutes >= 0),
    early_leave_grace_minutes integer NOT NULL DEFAULT 0 CHECK (early_leave_grace_minutes >= 0),
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT attendance_shifts_tenant_id_id_idx UNIQUE (tenant_id, id)
);

CREATE INDEX attendance_shifts_tenant_status_idx ON attendance_shifts (tenant_id, status);

CREATE TABLE attendance_shift_assignments (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    shift_id text NOT NULL,
    worksite_id text NOT NULL,
    effective_from timestamptz NOT NULL,
    effective_to timestamptz,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT attendance_shift_assignments_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT attendance_shift_assignments_interval_check CHECK (effective_to IS NULL OR effective_to >= effective_from),
    CONSTRAINT attendance_shift_assignments_active_no_overlap EXCLUDE USING gist (
        tenant_id WITH =,
        employee_id WITH =,
        tstzrange(effective_from, effective_to, '[]') WITH &&
    ) WHERE (status = 'active'),
    CONSTRAINT attendance_shift_assignments_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT attendance_shift_assignments_shift_fk FOREIGN KEY (tenant_id, shift_id) REFERENCES attendance_shifts (tenant_id, id),
    CONSTRAINT attendance_shift_assignments_worksite_fk FOREIGN KEY (tenant_id, worksite_id) REFERENCES attendance_worksites (tenant_id, id)
);

CREATE INDEX attendance_shift_assignments_tenant_employee_idx ON attendance_shift_assignments (tenant_id, employee_id, effective_from DESC);
CREATE INDEX attendance_shift_assignments_shift_idx ON attendance_shift_assignments (tenant_id, shift_id);
CREATE INDEX attendance_shift_assignments_worksite_idx ON attendance_shift_assignments (tenant_id, worksite_id);

ALTER TABLE attendance_clock_records
    ADD COLUMN IF NOT EXISTS shift_assignment_id text,
    ADD COLUMN IF NOT EXISTS shift_id text;

ALTER TABLE attendance_clock_records
    ADD CONSTRAINT attendance_clock_records_shift_assignment_fk
        FOREIGN KEY (tenant_id, shift_assignment_id) REFERENCES attendance_shift_assignments (tenant_id, id),
    ADD CONSTRAINT attendance_clock_records_shift_fk
        FOREIGN KEY (tenant_id, shift_id) REFERENCES attendance_shifts (tenant_id, id);

ALTER TABLE attendance_shifts ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_shifts FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_shift_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_shift_assignments FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_attendance_shifts ON attendance_shifts
    USING (tenant_id = current_setting('app.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_shift_assignments ON attendance_shift_assignments
    USING (tenant_id = current_setting('app.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
