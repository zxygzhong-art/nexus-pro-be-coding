-- +goose Up
-- Attendance/leave v2 is an intentional clean-slate cutover. Snapshot balances
-- stay immutable between upstream refreshes; every local delta is append-only.

DROP TABLE IF EXISTS attendance_day_projections;
DROP TABLE IF EXISTS attendance_correction_requests CASCADE;
DROP TABLE IF EXISTS attendance_clock_records CASCADE;
DROP TABLE IF EXISTS attendance_daily_summaries;

DROP TABLE IF EXISTS leave_balance_entries;
DROP TABLE IF EXISTS leave_case_sources;
DROP TABLE IF EXISTS external_leave_records;
DROP TABLE IF EXISTS leave_cases;
DROP TABLE IF EXISTS leave_request_allocations;
DROP TABLE IF EXISTS leave_requests;
DROP TABLE IF EXISTS leave_type_external_refs;
DROP TABLE IF EXISTS leave_balance_ledger;

DROP TRIGGER IF EXISTS leave_balances_ledger_trigger ON leave_balances;
DROP FUNCTION IF EXISTS append_leave_balance_ledger();
DROP TABLE IF EXISTS leave_balances;

CREATE TABLE leave_type_external_refs (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    source_system text NOT NULL,
    external_code text NOT NULL,
    external_category_code text NOT NULL DEFAULT '',
    leave_type_id text NOT NULL,
    effective_from date,
    effective_to date,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT leave_type_external_refs_type_fk FOREIGN KEY (tenant_id, leave_type_id) REFERENCES leave_types (tenant_id, id),
    CONSTRAINT leave_type_external_refs_period_check CHECK (effective_from IS NULL OR effective_to IS NULL OR effective_to >= effective_from),
    CONSTRAINT leave_type_external_refs_normalized_check CHECK (
        source_system <> ''
        AND source_system = lower(btrim(source_system))
        AND external_code <> ''
        AND external_code = lower(btrim(external_code))
        AND external_category_code = lower(btrim(external_category_code))
    ),
    CONSTRAINT leave_type_external_refs_no_overlap EXCLUDE USING gist (
        tenant_id WITH =,
        source_system WITH =,
        external_category_code WITH =,
        external_code WITH =,
        daterange(effective_from, effective_to, '[]') WITH &&
    )
);

CREATE UNIQUE INDEX leave_type_external_refs_identity_idx
ON leave_type_external_refs (tenant_id, source_system, external_category_code, external_code, effective_from) NULLS NOT DISTINCT;

CREATE TABLE leave_balances (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    leave_type_id text NOT NULL,
    remaining_minutes integer NOT NULL,
    period_start date,
    period_end date,
    granted_minutes integer NOT NULL DEFAULT 0,
    used_minutes integer NOT NULL DEFAULT 0,
    source text NOT NULL CHECK (source IN ('ehrms', 'explicit_snapshot', 'manual_snapshot', 'local_anchor')),
    external_leave_code text NOT NULL DEFAULT '',
    external_category_code text NOT NULL DEFAULT '',
    entitlement_year integer,
    carry_in_minutes integer NOT NULL DEFAULT 0 CHECK (carry_in_minutes >= 0),
    carry_expire date,
    raw_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    last_synced_at timestamptz,
    updated_at timestamptz NOT NULL,
    CONSTRAINT leave_balances_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT leave_balances_period_check CHECK (period_start IS NULL OR period_end IS NULL OR period_end >= period_start),
    CONSTRAINT leave_balances_nonnegative_check CHECK (remaining_minutes >= 0 AND granted_minutes >= 0 AND used_minutes >= 0),
    CONSTRAINT leave_balances_local_anchor_zero_check CHECK (
        source <> 'local_anchor'
        OR (remaining_minutes = 0 AND granted_minutes = 0 AND used_minutes = 0 AND carry_in_minutes = 0
            AND period_start IS NULL AND period_end IS NULL)
    ),
    CONSTRAINT leave_balances_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT leave_balances_leave_type_fk FOREIGN KEY (tenant_id, leave_type_id) REFERENCES leave_types (tenant_id, id),
    CONSTRAINT leave_balances_tenant_identity_idx UNIQUE (tenant_id, id, employee_id, leave_type_id)
);

CREATE INDEX leave_balances_fefo_idx
ON leave_balances (
    tenant_id,
    employee_id,
    leave_type_id,
    ((source = 'local_anchor')),
    period_end ASC NULLS LAST,
    period_start ASC NULLS FIRST,
    id
);

CREATE UNIQUE INDEX leave_balances_local_anchor_idx
ON leave_balances (tenant_id, employee_id, leave_type_id)
WHERE source = 'local_anchor';

CREATE TABLE leave_requests (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    leave_type text NOT NULL,
    leave_type_id text NOT NULL,
    policy_version integer NOT NULL DEFAULT 0 CHECK (policy_version >= 0),
    rule_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    evaluation_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    start_at timestamptz NOT NULL,
    end_at timestamptz NOT NULL,
    requested_minutes integer NOT NULL CHECK (requested_minutes > 0),
    reason text NOT NULL DEFAULT '',
    status text NOT NULL CHECK (status IN ('pending_approval', 'approved', 'rejected', 'cancelled')),
    form_instance_id text NOT NULL,
    reconciliation_status text NOT NULL DEFAULT 'not_required' CHECK (reconciliation_status IN ('not_required', 'nexus_only', 'matched', 'pending_balance_confirmation', 'ambiguous', 'mismatch', 'manually_confirmed')),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT leave_requests_interval_check CHECK (end_at > start_at),
    CONSTRAINT leave_requests_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT leave_requests_tenant_identity_idx UNIQUE (tenant_id, id, employee_id, leave_type_id),
    CONSTRAINT leave_requests_form_instance_idx UNIQUE (tenant_id, form_instance_id),
    CONSTRAINT leave_requests_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT leave_requests_type_fk FOREIGN KEY (tenant_id, leave_type_id) REFERENCES leave_types (tenant_id, id),
    CONSTRAINT leave_requests_form_instance_fk FOREIGN KEY (tenant_id, form_instance_id) REFERENCES form_instances (tenant_id, id)
);

CREATE INDEX leave_requests_tenant_id_idx ON leave_requests (tenant_id);
CREATE INDEX leave_requests_tenant_employee_status_dates_idx ON leave_requests (tenant_id, employee_id, status, start_at, end_at);

CREATE TABLE leave_request_allocations (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    leave_request_id text NOT NULL,
    leave_balance_id text NOT NULL,
    employee_id text NOT NULL,
    leave_type_id text NOT NULL,
    cycle integer NOT NULL CHECK (cycle > 0),
    reserved_minutes integer NOT NULL CHECK (reserved_minutes > 0),
    created_at timestamptz NOT NULL,
    CONSTRAINT leave_request_allocations_request_balance_cycle_idx UNIQUE (tenant_id, leave_request_id, leave_balance_id, cycle),
    CONSTRAINT leave_request_allocations_identity_idx UNIQUE (
        tenant_id, id, leave_request_id, leave_balance_id, employee_id, leave_type_id
    ),
    CONSTRAINT leave_request_allocations_request_fk FOREIGN KEY (
        tenant_id, leave_request_id, employee_id, leave_type_id
    ) REFERENCES leave_requests (tenant_id, id, employee_id, leave_type_id) ON DELETE CASCADE,
    CONSTRAINT leave_request_allocations_balance_fk FOREIGN KEY (
        tenant_id, leave_balance_id, employee_id, leave_type_id
    ) REFERENCES leave_balances (tenant_id, id, employee_id, leave_type_id)
);

CREATE INDEX leave_request_allocations_tenant_balance_idx ON leave_request_allocations (tenant_id, leave_balance_id);

CREATE TABLE leave_cases (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    leave_type_id text NOT NULL,
    start_at timestamptz NOT NULL,
    end_at timestamptz NOT NULL,
    net_minutes integer NOT NULL CHECK (net_minutes > 0),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'cancelled', 'corrected')),
    origin text NOT NULL CHECK (origin IN ('nexus', 'ehrms', 'both')),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT leave_cases_interval_check CHECK (end_at > start_at),
    CONSTRAINT leave_cases_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT leave_cases_type_fk FOREIGN KEY (tenant_id, leave_type_id) REFERENCES leave_types (tenant_id, id),
    CONSTRAINT leave_cases_tenant_id_id_idx UNIQUE (tenant_id, id)
);

CREATE INDEX leave_cases_employee_interval_idx ON leave_cases (tenant_id, employee_id, start_at, end_at);

CREATE TABLE external_leave_records (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    source_system text NOT NULL,
    external_ref text NOT NULL,
    external_leave_code text NOT NULL DEFAULT '',
    external_category_code text NOT NULL DEFAULT '',
    leave_type_id text NOT NULL,
    leave_name text NOT NULL DEFAULT '',
    start_at timestamptz NOT NULL,
    end_at timestamptz NOT NULL,
    gross_minutes integer NOT NULL CHECK (gross_minutes > 0),
    deduct_minutes integer NOT NULL DEFAULT 0 CHECK (deduct_minutes >= 0),
    net_minutes integer NOT NULL CHECK (net_minutes > 0),
    remark text NOT NULL DEFAULT '',
    source_label text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'cancelled', 'corrected')),
    raw_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    payload_hash text NOT NULL DEFAULT '',
    first_seen_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    deleted_at timestamptz,
    CONSTRAINT external_leave_records_interval_check CHECK (end_at > start_at),
    CONSTRAINT external_leave_records_duration_check CHECK (gross_minutes >= net_minutes AND deduct_minutes + net_minutes <= gross_minutes),
    CONSTRAINT external_leave_records_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT external_leave_records_type_fk FOREIGN KEY (tenant_id, leave_type_id) REFERENCES leave_types (tenant_id, id),
    CONSTRAINT external_leave_records_identity_idx UNIQUE (tenant_id, source_system, external_ref),
    CONSTRAINT external_leave_records_tenant_id_id_idx UNIQUE (tenant_id, id)
);

CREATE INDEX external_leave_records_employee_interval_idx ON external_leave_records (tenant_id, employee_id, start_at, end_at);

CREATE TABLE leave_case_sources (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    leave_case_id text NOT NULL,
    leave_request_id text,
    external_leave_record_id text,
    match_method text NOT NULL DEFAULT 'direct' CHECK (match_method IN ('direct', 'exact', 'heuristic', 'manual')),
    match_status text NOT NULL DEFAULT 'confirmed' CHECK (match_status IN ('proposed', 'confirmed', 'rejected', 'ambiguous')),
    created_at timestamptz NOT NULL,
    CONSTRAINT leave_case_sources_case_fk FOREIGN KEY (tenant_id, leave_case_id) REFERENCES leave_cases (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT leave_case_sources_source_xor_check CHECK (num_nonnulls(leave_request_id, external_leave_record_id) = 1),
    CONSTRAINT leave_case_sources_request_fk FOREIGN KEY (tenant_id, leave_request_id) REFERENCES leave_requests (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT leave_case_sources_external_fk FOREIGN KEY (tenant_id, external_leave_record_id) REFERENCES external_leave_records (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX leave_case_sources_case_idx ON leave_case_sources (tenant_id, leave_case_id);
CREATE UNIQUE INDEX leave_case_sources_request_idx ON leave_case_sources (tenant_id, leave_request_id) WHERE leave_request_id IS NOT NULL;
CREATE UNIQUE INDEX leave_case_sources_external_idx ON leave_case_sources (tenant_id, external_leave_record_id) WHERE external_leave_record_id IS NOT NULL;

CREATE TABLE attendance_clock_records (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    worksite_id text,
    work_date date NOT NULL,
    direction text NOT NULL CHECK (direction IN ('clock_in', 'clock_out')),
    client_event_id text NOT NULL DEFAULT '',
    clocked_at timestamptz NOT NULL,
    latitude double precision CHECK (latitude >= -90 AND latitude <= 90),
    longitude double precision CHECK (longitude >= -180 AND longitude <= 180),
    accuracy_meters double precision CHECK (accuracy_meters >= 0),
    distance_meters double precision CHECK (distance_meters >= 0),
    record_status text NOT NULL CHECK (record_status IN ('accepted', 'abnormal', 'rejected')),
    rejection_reason text NOT NULL DEFAULT '',
    source text NOT NULL CHECK (source IN ('geofence', 'manual_correction')),
    device_id text NOT NULL DEFAULT '',
    device_info jsonb NOT NULL DEFAULT '{}'::jsonb,
    correction_request_id text,
    voided boolean NOT NULL DEFAULT false,
    voided_at timestamptz,
    voided_by_account_id text,
    void_reason text,
    created_at timestamptz NOT NULL,
    CONSTRAINT attendance_clock_records_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT attendance_clock_records_employee_identity_idx UNIQUE (tenant_id, id, employee_id),
    CONSTRAINT attendance_clock_records_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT attendance_clock_records_worksite_fk FOREIGN KEY (tenant_id, worksite_id) REFERENCES attendance_worksites (tenant_id, id),
    CONSTRAINT attendance_clock_records_gps_pair_check CHECK ((latitude IS NULL) = (longitude IS NULL)),
    CONSTRAINT attendance_clock_records_source_shape_check CHECK (
        (source = 'geofence' AND latitude IS NOT NULL AND longitude IS NOT NULL
            AND correction_request_id IS NULL)
        OR (source = 'manual_correction' AND correction_request_id IS NOT NULL)
    ),
    CONSTRAINT attendance_clock_records_void_shape_check CHECK (
        (voided = false AND voided_at IS NULL AND voided_by_account_id IS NULL AND void_reason IS NULL)
        OR (voided = true AND voided_at IS NOT NULL AND voided_by_account_id IS NOT NULL AND btrim(void_reason) <> '')
    )
);

CREATE INDEX attendance_clock_records_tenant_employee_date_idx ON attendance_clock_records (tenant_id, employee_id, work_date DESC);
CREATE INDEX attendance_clock_records_tenant_status_idx ON attendance_clock_records (tenant_id, record_status, clocked_at DESC);
CREATE INDEX attendance_clock_records_effective_boundary_idx ON attendance_clock_records (tenant_id, employee_id, work_date, direction, clocked_at, created_at, id) WHERE record_status = 'accepted' AND voided = false;
CREATE INDEX attendance_clock_records_effective_latest_idx ON attendance_clock_records (tenant_id, employee_id, work_date, clocked_at, created_at, id) WHERE record_status = 'accepted' AND voided = false;
CREATE UNIQUE INDEX attendance_clock_records_client_event_idx ON attendance_clock_records (tenant_id, client_event_id) WHERE client_event_id <> '';

CREATE TABLE attendance_daily_summaries (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    work_date date NOT NULL,
    shift_start text NOT NULL DEFAULT '',
    shift_end text NOT NULL DEFAULT '',
    shift_hours double precision NOT NULL DEFAULT 0,
    daily_hours double precision NOT NULL DEFAULT 0,
    clock_hours double precision NOT NULL DEFAULT 0,
    clock_start text NOT NULL DEFAULT '',
    clock_end text NOT NULL DEFAULT '',
    attend_start text NOT NULL DEFAULT '',
    attend_end text NOT NULL DEFAULT '',
    attend_hours double precision NOT NULL DEFAULT 0,
    attend_counted boolean NOT NULL DEFAULT false,
    leave_type text NOT NULL DEFAULT '',
    leave_start text NOT NULL DEFAULT '',
    leave_end text NOT NULL DEFAULT '',
    leave_hours double precision NOT NULL DEFAULT 0,
    leave_counted boolean NOT NULL DEFAULT false,
    leave2_type text NOT NULL DEFAULT '',
    leave2_start text NOT NULL DEFAULT '',
    leave2_end text NOT NULL DEFAULT '',
    leave2_hours double precision NOT NULL DEFAULT 0,
    leave2_counted boolean NOT NULL DEFAULT false,
    overtime_start text NOT NULL DEFAULT '',
    overtime_end text NOT NULL DEFAULT '',
    overtime_hours double precision NOT NULL DEFAULT 0,
    overtime_counted boolean NOT NULL DEFAULT false,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
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

CREATE TABLE attendance_day_projections (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    work_date date NOT NULL,
    policy_version integer NOT NULL CHECK (policy_version > 0),
    scheduled_start_at timestamptz,
    scheduled_end_at timestamptz,
    clock_in_record_id text,
    clock_out_record_id text,
    last_punch_record_id text,
    punch_count integer NOT NULL DEFAULT 0 CHECK (punch_count >= 0),
    required_minutes integer NOT NULL DEFAULT 0 CHECK (required_minutes >= 0),
    worked_minutes integer NOT NULL DEFAULT 0 CHECK (worked_minutes >= 0),
    approved_leave_minutes integer NOT NULL DEFAULT 0 CHECK (approved_leave_minutes >= 0),
    pending_leave_minutes integer NOT NULL DEFAULT 0 CHECK (pending_leave_minutes >= 0),
    overtime_minutes integer NOT NULL DEFAULT 0 CHECK (overtime_minutes >= 0),
    day_status text NOT NULL CHECK (day_status IN ('not_started', 'working', 'complete', 'pending_leave', 'abnormal')),
    anomaly_reasons text[] NOT NULL DEFAULT '{}'::text[],
    input_fingerprint text NOT NULL CHECK (btrim(input_fingerprint) <> ''),
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    computed_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, employee_id, work_date),
    CONSTRAINT attendance_day_projections_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT attendance_day_projections_policy_fk FOREIGN KEY (tenant_id, policy_version) REFERENCES attendance_policy_versions (tenant_id, version),
    CONSTRAINT attendance_day_projections_clock_in_fk FOREIGN KEY (tenant_id, clock_in_record_id, employee_id) REFERENCES attendance_clock_records (tenant_id, id, employee_id),
    CONSTRAINT attendance_day_projections_clock_out_fk FOREIGN KEY (tenant_id, clock_out_record_id, employee_id) REFERENCES attendance_clock_records (tenant_id, id, employee_id),
    CONSTRAINT attendance_day_projections_last_punch_fk FOREIGN KEY (tenant_id, last_punch_record_id, employee_id) REFERENCES attendance_clock_records (tenant_id, id, employee_id),
    CONSTRAINT attendance_day_projections_schedule_check CHECK (
        scheduled_start_at IS NULL OR scheduled_end_at IS NULL OR scheduled_end_at > scheduled_start_at
    )
);

CREATE INDEX attendance_day_projections_tenant_date_status_idx
ON attendance_day_projections (tenant_id, work_date, day_status, employee_id);

CREATE TABLE attendance_correction_requests (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    direction text NOT NULL CHECK (direction IN ('clock_in', 'clock_out')),
    requested_clocked_at timestamptz NOT NULL,
    work_date date NOT NULL,
    correction_type text NOT NULL DEFAULT 'add_record' CHECK (correction_type IN ('add_record', 'void_record', 'replace_record')),
    target_clock_record_id text,
    replacement_clock_record_id text,
    reason text NOT NULL DEFAULT '',
    status text NOT NULL CHECK (status IN ('pending', 'reviewing', 'approved', 'rejected', 'cancelled')),
    form_instance_id text NOT NULL,
    clock_record_id text,
    reviewed_by_account_id text,
    review_reason text NOT NULL DEFAULT '',
    reviewed_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT attendance_correction_requests_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT attendance_correction_requests_employee_identity_idx UNIQUE (tenant_id, id, employee_id),
    CONSTRAINT attendance_correction_requests_form_instance_idx UNIQUE (tenant_id, form_instance_id),
    CONSTRAINT attendance_correction_requests_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT attendance_correction_requests_form_instance_fk FOREIGN KEY (tenant_id, form_instance_id) REFERENCES form_instances (tenant_id, id),
    CONSTRAINT attendance_correction_requests_reviewer_fk FOREIGN KEY (tenant_id, reviewed_by_account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT attendance_correction_requests_target_fk FOREIGN KEY (tenant_id, target_clock_record_id, employee_id) REFERENCES attendance_clock_records (tenant_id, id, employee_id),
    CONSTRAINT attendance_correction_requests_replacement_fk FOREIGN KEY (tenant_id, replacement_clock_record_id, employee_id) REFERENCES attendance_clock_records (tenant_id, id, employee_id),
    CONSTRAINT attendance_correction_requests_clock_fk FOREIGN KEY (tenant_id, clock_record_id, employee_id) REFERENCES attendance_clock_records (tenant_id, id, employee_id),
    CONSTRAINT attendance_correction_requests_target_shape_check CHECK (
        (correction_type = 'add_record' AND target_clock_record_id IS NULL)
        OR (correction_type IN ('void_record', 'replace_record') AND target_clock_record_id IS NOT NULL)
    ),
    CONSTRAINT attendance_correction_requests_output_shape_check CHECK (
        (status <> 'approved' AND replacement_clock_record_id IS NULL AND clock_record_id IS NULL)
        OR (status = 'approved' AND correction_type = 'void_record' AND replacement_clock_record_id IS NULL AND clock_record_id IS NULL)
        OR (status = 'approved' AND correction_type = 'add_record' AND replacement_clock_record_id IS NULL AND clock_record_id IS NOT NULL)
        OR (status = 'approved' AND correction_type = 'replace_record' AND replacement_clock_record_id IS NOT NULL AND replacement_clock_record_id = clock_record_id)
    ),
    CONSTRAINT attendance_correction_requests_review_shape_check CHECK (
        (status = 'pending' AND reviewed_by_account_id IS NULL AND reviewed_at IS NULL)
        OR (status = 'reviewing' AND reviewed_by_account_id IS NOT NULL AND reviewed_at IS NULL)
        OR (status IN ('approved', 'rejected', 'cancelled') AND reviewed_by_account_id IS NOT NULL AND reviewed_at IS NOT NULL)
    )
);

CREATE INDEX attendance_correction_requests_tenant_employee_date_idx ON attendance_correction_requests (tenant_id, employee_id, work_date DESC);
CREATE INDEX attendance_correction_requests_tenant_status_idx ON attendance_correction_requests (tenant_id, status, created_at DESC);

ALTER TABLE overtime_requests
    ADD CONSTRAINT overtime_requests_tenant_id_id_idx UNIQUE (tenant_id, id),
    ADD CONSTRAINT overtime_requests_employee_identity_idx UNIQUE (tenant_id, id, employee_id);

ALTER TABLE attendance_clock_records
    ADD CONSTRAINT attendance_clock_records_correction_fk
    FOREIGN KEY (tenant_id, correction_request_id, employee_id)
    REFERENCES attendance_correction_requests (tenant_id, id, employee_id);

ALTER TABLE attendance_clock_records
    ADD CONSTRAINT attendance_clock_records_voided_by_fk
    FOREIGN KEY (tenant_id, voided_by_account_id)
    REFERENCES accounts (tenant_id, id);

CREATE TABLE leave_balance_entries (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    leave_type_id text NOT NULL,
    balance_id text NOT NULL,
    allocation_id bigint,
    leave_request_id text,
    leave_case_id text,
    overtime_request_id text,
    entry_type text NOT NULL CHECK (entry_type IN (
        'reserve', 'release', 'local_consume', 'local_refund',
        'external_reconcile', 'external_reversal', 'overtime_credit', 'manual_adjust'
    )),
    amount_minutes integer NOT NULL CHECK (amount_minutes <> 0),
    idempotency_key text NOT NULL CHECK (btrim(idempotency_key) <> ''),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT leave_balance_entries_sign_check CHECK (
        (entry_type IN ('reserve', 'local_consume', 'external_reversal') AND amount_minutes < 0)
        OR (entry_type IN ('release', 'local_refund', 'external_reconcile', 'overtime_credit') AND amount_minutes > 0)
        OR (entry_type = 'manual_adjust' AND amount_minutes <> 0)
    ),
    CONSTRAINT leave_balance_entries_reference_shape_check CHECK (
        (entry_type IN ('reserve', 'release', 'local_consume', 'local_refund', 'external_reconcile', 'external_reversal')
            AND allocation_id IS NOT NULL AND leave_request_id IS NOT NULL
            AND overtime_request_id IS NULL)
        OR (entry_type = 'overtime_credit'
            AND allocation_id IS NULL AND leave_request_id IS NULL
            AND leave_case_id IS NULL AND overtime_request_id IS NOT NULL)
        OR (entry_type = 'manual_adjust'
            AND allocation_id IS NULL AND leave_request_id IS NULL
            AND leave_case_id IS NULL AND overtime_request_id IS NULL)
    ),
    CONSTRAINT leave_balance_entries_reconciliation_case_check CHECK (
        entry_type NOT IN ('external_reconcile', 'external_reversal') OR leave_case_id IS NOT NULL
    ),
    CONSTRAINT leave_balance_entries_allocation_fk FOREIGN KEY (
        tenant_id, allocation_id, leave_request_id, balance_id, employee_id, leave_type_id
    ) REFERENCES leave_request_allocations (
        tenant_id, id, leave_request_id, leave_balance_id, employee_id, leave_type_id
    ),
    CONSTRAINT leave_balance_entries_balance_fk FOREIGN KEY (
        tenant_id, balance_id, employee_id, leave_type_id
    ) REFERENCES leave_balances (tenant_id, id, employee_id, leave_type_id),
    CONSTRAINT leave_balance_entries_case_fk FOREIGN KEY (tenant_id, leave_case_id) REFERENCES leave_cases (tenant_id, id),
    CONSTRAINT leave_balance_entries_overtime_request_fk FOREIGN KEY (tenant_id, overtime_request_id, employee_id) REFERENCES overtime_requests (tenant_id, id, employee_id),
    CONSTRAINT leave_balance_entries_idempotency_idx UNIQUE (tenant_id, idempotency_key)
);

CREATE INDEX leave_balance_entries_balance_idx ON leave_balance_entries (tenant_id, balance_id, occurred_at, id);
CREATE INDEX leave_balance_entries_request_idx ON leave_balance_entries (tenant_id, leave_request_id, occurred_at, id);
CREATE INDEX leave_balance_entries_case_idx ON leave_balance_entries (tenant_id, leave_case_id, occurred_at, id) WHERE leave_case_id IS NOT NULL;
CREATE INDEX leave_balance_entries_overtime_request_idx ON leave_balance_entries (tenant_id, overtime_request_id) WHERE overtime_request_id IS NOT NULL;

ALTER TABLE leave_type_external_refs ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_type_external_refs FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_balances ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_balances FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_requests FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_request_allocations ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_request_allocations FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_cases ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_cases FORCE ROW LEVEL SECURITY;
ALTER TABLE external_leave_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE external_leave_records FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_case_sources ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_case_sources FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_balance_entries ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_balance_entries FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_clock_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_clock_records FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_daily_summaries ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_daily_summaries FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_day_projections ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_day_projections FORCE ROW LEVEL SECURITY;
ALTER TABLE attendance_correction_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_correction_requests FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_leave_type_external_refs ON leave_type_external_refs USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_balances ON leave_balances USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_requests ON leave_requests USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_request_allocations ON leave_request_allocations USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_cases ON leave_cases USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_external_leave_records ON external_leave_records USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_case_sources ON leave_case_sources USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_balance_entries ON leave_balance_entries USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_clock_records ON attendance_clock_records USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_daily_summaries ON attendance_daily_summaries USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_day_projections ON attendance_day_projections USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_attendance_correction_requests ON attendance_correction_requests USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- +goose Down
-- The Up migration deliberately discards legacy attendance/leave rows and is
-- therefore not representable as a safe downgrade.
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION '000021_attendance_leave_clean_slate is irreversible because legacy attendance and leave data was discarded';
END $$;
-- +goose StatementEnd
