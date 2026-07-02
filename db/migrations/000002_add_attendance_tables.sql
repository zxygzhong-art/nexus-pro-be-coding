-- +goose Up

CREATE TABLE IF NOT EXISTS attendance_worksites (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    address text NOT NULL DEFAULT '',
    latitude double precision NOT NULL CHECK (latitude >= -90 AND latitude <= 90),
    longitude double precision NOT NULL CHECK (longitude >= -180 AND longitude <= 180),
    radius_meters integer NOT NULL CHECK (radius_meters > 0),
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT attendance_worksites_tenant_id_id_idx UNIQUE (tenant_id, id)
);

CREATE INDEX IF NOT EXISTS attendance_worksites_tenant_status_idx ON attendance_worksites (tenant_id, status);

CREATE TABLE IF NOT EXISTS attendance_shifts (
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

CREATE INDEX IF NOT EXISTS attendance_shifts_tenant_status_idx ON attendance_shifts (tenant_id, status);

CREATE TABLE IF NOT EXISTS attendance_shift_assignments (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    shift_id text NOT NULL,
    worksite_id text NOT NULL,
    effective_from timestamptz NOT NULL,
    effective_to timestamptz,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT attendance_shift_assignments_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT attendance_shift_assignments_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT attendance_shift_assignments_shift_fk FOREIGN KEY (tenant_id, shift_id) REFERENCES attendance_shifts (tenant_id, id),
    CONSTRAINT attendance_shift_assignments_worksite_fk FOREIGN KEY (tenant_id, worksite_id) REFERENCES attendance_worksites (tenant_id, id)
);

CREATE INDEX IF NOT EXISTS attendance_shift_assignments_tenant_employee_idx ON attendance_shift_assignments (tenant_id, employee_id, effective_from DESC);
CREATE INDEX IF NOT EXISTS attendance_shift_assignments_shift_idx ON attendance_shift_assignments (tenant_id, shift_id);
CREATE INDEX IF NOT EXISTS attendance_shift_assignments_worksite_idx ON attendance_shift_assignments (tenant_id, worksite_id);

CREATE TABLE IF NOT EXISTS attendance_clock_records (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    shift_assignment_id text NOT NULL,
    shift_id text NOT NULL,
    worksite_id text NOT NULL,
    work_date text NOT NULL,
    direction text NOT NULL,
    clocked_at timestamptz NOT NULL,
    latitude double precision NOT NULL CHECK (latitude >= -90 AND latitude <= 90),
    longitude double precision NOT NULL CHECK (longitude >= -180 AND longitude <= 180),
    accuracy_meters double precision NOT NULL DEFAULT 0 CHECK (accuracy_meters >= 0),
    distance_meters double precision NOT NULL DEFAULT 0 CHECK (distance_meters >= 0),
    record_status text NOT NULL,
    rejection_reason text NOT NULL DEFAULT '',
    source text NOT NULL,
    device_id text NOT NULL DEFAULT '',
    device_info jsonb NOT NULL DEFAULT '{}'::jsonb,
    correction_request_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    CONSTRAINT attendance_clock_records_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT attendance_clock_records_shift_assignment_fk FOREIGN KEY (tenant_id, shift_assignment_id) REFERENCES attendance_shift_assignments (tenant_id, id),
    CONSTRAINT attendance_clock_records_shift_fk FOREIGN KEY (tenant_id, shift_id) REFERENCES attendance_shifts (tenant_id, id),
    CONSTRAINT attendance_clock_records_worksite_fk FOREIGN KEY (tenant_id, worksite_id) REFERENCES attendance_worksites (tenant_id, id)
);

CREATE INDEX IF NOT EXISTS attendance_clock_records_tenant_employee_date_idx ON attendance_clock_records (tenant_id, employee_id, work_date DESC);
CREATE INDEX IF NOT EXISTS attendance_clock_records_tenant_status_idx ON attendance_clock_records (tenant_id, record_status, clocked_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS attendance_clock_records_one_accepted_idx ON attendance_clock_records (tenant_id, employee_id, work_date, direction) WHERE record_status = 'accepted';

CREATE TABLE IF NOT EXISTS attendance_correction_requests (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    direction text NOT NULL,
    requested_clocked_at timestamptz NOT NULL,
    work_date text NOT NULL,
    reason text NOT NULL DEFAULT '',
    status text NOT NULL,
    form_instance_id text NOT NULL DEFAULT '',
    clock_record_id text NOT NULL DEFAULT '',
    reviewed_by_account_id text NOT NULL DEFAULT '',
    review_reason text NOT NULL DEFAULT '',
    reviewed_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT attendance_correction_requests_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id)
);

CREATE INDEX IF NOT EXISTS attendance_correction_requests_tenant_employee_date_idx ON attendance_correction_requests (tenant_id, employee_id, work_date DESC);
CREATE INDEX IF NOT EXISTS attendance_correction_requests_tenant_status_idx ON attendance_correction_requests (tenant_id, status, created_at DESC);

-- +goose Down

-- Intentionally no-op. This migration is used to bring older demo databases forward
-- without dropping attendance tables or test data.
