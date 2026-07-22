-- +goose Up
-- eHRMS remains authoritative for granted/used/remaining snapshots. Nexus writes
-- only an overlay ledger for pending and Nexus-only leave, and reconciles that
-- overlay when the same logical leave later appears in eHRMS.

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
    CONSTRAINT leave_type_external_refs_period_check CHECK (effective_from IS NULL OR effective_to IS NULL OR effective_to >= effective_from)
);

CREATE UNIQUE INDEX leave_type_external_refs_identity_idx
ON leave_type_external_refs (tenant_id, source_system, lower(external_category_code), lower(external_code), effective_from) NULLS NOT DISTINCT;

ALTER TABLE leave_balances
    ADD COLUMN external_leave_code text NOT NULL DEFAULT '',
    ADD COLUMN external_category_code text NOT NULL DEFAULT '',
    ADD COLUMN entitlement_year integer,
    ADD COLUMN carry_in_hours numeric(12,2) NOT NULL DEFAULT 0 CHECK (carry_in_hours >= 0),
    ADD COLUMN carry_expire date,
    ADD COLUMN raw_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN last_synced_at timestamptz;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION append_leave_balance_ledger()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    ledger_event_type text;
BEGIN
    ledger_event_type := CASE
        WHEN NEW.source = 'ehrms' THEN 'snapshot'
        WHEN TG_OP = 'INSERT' THEN 'snapshot'
        WHEN NEW.remaining_hours < OLD.remaining_hours THEN 'reserve'
        WHEN NEW.remaining_hours > OLD.remaining_hours AND NEW.used_hours < OLD.used_hours THEN 'release'
        ELSE 'adjustment'
    END;
    INSERT INTO leave_balance_ledger (
        tenant_id, balance_id, employee_id, leave_type_id, period_start, period_end,
        event_type, delta_hours, remaining_hours, granted_hours, used_hours, source, occurred_at
    ) VALUES (
        NEW.tenant_id, NEW.id, NEW.employee_id, NEW.leave_type_id, NEW.period_start, NEW.period_end,
        ledger_event_type, NEW.remaining_hours - CASE WHEN TG_OP = 'INSERT' THEN 0 ELSE OLD.remaining_hours END,
        NEW.remaining_hours, NEW.granted_hours, NEW.used_hours, NEW.source, NEW.updated_at
    );
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

ALTER TABLE leave_requests
    ADD COLUMN reconciliation_status text NOT NULL DEFAULT 'not_required'
        CHECK (reconciliation_status IN ('not_required', 'nexus_only', 'matched', 'ambiguous', 'mismatch', 'manually_confirmed')),
    ADD COLUMN updated_at timestamptz;

UPDATE leave_requests SET updated_at = created_at WHERE updated_at IS NULL;
ALTER TABLE leave_requests ALTER COLUMN updated_at SET NOT NULL;

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
    source_type text NOT NULL CHECK (source_type IN ('nexus_request', 'ehrms_record')),
    source_id text NOT NULL,
    match_method text NOT NULL DEFAULT 'direct' CHECK (match_method IN ('direct', 'exact', 'heuristic', 'manual')),
    match_status text NOT NULL DEFAULT 'confirmed' CHECK (match_status IN ('proposed', 'confirmed', 'rejected', 'ambiguous')),
    created_at timestamptz NOT NULL,
    CONSTRAINT leave_case_sources_case_fk FOREIGN KEY (tenant_id, leave_case_id) REFERENCES leave_cases (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT leave_case_sources_identity_idx UNIQUE (tenant_id, source_type, source_id)
);

CREATE INDEX leave_case_sources_case_idx ON leave_case_sources (tenant_id, leave_case_id);

CREATE TABLE leave_balance_entries (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    leave_type_id text NOT NULL,
    balance_id text NOT NULL,
    leave_request_id text REFERENCES leave_requests(id) ON DELETE SET NULL,
    leave_case_id text,
    entry_type text NOT NULL CHECK (entry_type IN ('reserve', 'release', 'local_consume', 'local_refund', 'external_reconcile', 'manual_adjust')),
    amount_minutes integer NOT NULL CHECK (amount_minutes <> 0),
    idempotency_key text NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT leave_balance_entries_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id),
    CONSTRAINT leave_balance_entries_type_fk FOREIGN KEY (tenant_id, leave_type_id) REFERENCES leave_types (tenant_id, id),
    CONSTRAINT leave_balance_entries_balance_fk FOREIGN KEY (tenant_id, balance_id) REFERENCES leave_balances (tenant_id, id),
    CONSTRAINT leave_balance_entries_case_fk FOREIGN KEY (tenant_id, leave_case_id) REFERENCES leave_cases (tenant_id, id),
    CONSTRAINT leave_balance_entries_idempotency_idx UNIQUE (tenant_id, idempotency_key)
);

CREATE INDEX leave_balance_entries_balance_idx ON leave_balance_entries (tenant_id, balance_id, occurred_at, id);
CREATE INDEX leave_balance_entries_request_idx ON leave_balance_entries (tenant_id, leave_request_id, occurred_at, id);

ALTER TABLE leave_type_external_refs ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_type_external_refs FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_cases ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_cases FORCE ROW LEVEL SECURITY;
ALTER TABLE external_leave_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE external_leave_records FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_case_sources ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_case_sources FORCE ROW LEVEL SECURITY;
ALTER TABLE leave_balance_entries ENABLE ROW LEVEL SECURITY;
ALTER TABLE leave_balance_entries FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_leave_type_external_refs ON leave_type_external_refs USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_cases ON leave_cases USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_external_leave_records ON external_leave_records USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_case_sources ON leave_case_sources USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_leave_balance_entries ON leave_balance_entries USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- +goose Down
DROP TABLE IF EXISTS leave_balance_entries;
DROP TABLE IF EXISTS leave_case_sources;
DROP TABLE IF EXISTS external_leave_records;
DROP TABLE IF EXISTS leave_cases;
ALTER TABLE leave_requests DROP COLUMN IF EXISTS updated_at;
ALTER TABLE leave_requests DROP COLUMN IF EXISTS reconciliation_status;
ALTER TABLE leave_balances
    DROP COLUMN IF EXISTS last_synced_at,
    DROP COLUMN IF EXISTS raw_payload,
    DROP COLUMN IF EXISTS carry_expire,
    DROP COLUMN IF EXISTS carry_in_hours,
    DROP COLUMN IF EXISTS entitlement_year,
    DROP COLUMN IF EXISTS external_category_code,
    DROP COLUMN IF EXISTS external_leave_code;
DROP TABLE IF EXISTS leave_type_external_refs;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION append_leave_balance_ledger()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    ledger_event_type text;
BEGIN
    ledger_event_type := CASE
        WHEN TG_OP = 'INSERT' THEN 'snapshot'
        WHEN NEW.remaining_hours < OLD.remaining_hours THEN 'reserve'
        WHEN NEW.remaining_hours > OLD.remaining_hours AND NEW.used_hours < OLD.used_hours THEN 'release'
        ELSE 'adjustment'
    END;
    INSERT INTO leave_balance_ledger (
        tenant_id, balance_id, employee_id, leave_type_id, period_start, period_end,
        event_type, delta_hours, remaining_hours, granted_hours, used_hours, source, occurred_at
    ) VALUES (
        NEW.tenant_id, NEW.id, NEW.employee_id, NEW.leave_type_id, NEW.period_start, NEW.period_end,
        ledger_event_type, NEW.remaining_hours - CASE WHEN TG_OP = 'INSERT' THEN 0 ELSE OLD.remaining_hours END,
        NEW.remaining_hours, NEW.granted_hours, NEW.used_hours, NEW.source, NEW.updated_at
    );
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
