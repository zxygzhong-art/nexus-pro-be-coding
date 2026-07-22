-- +goose Up
ALTER TABLE leave_balances DISABLE TRIGGER leave_balances_ledger_trigger;

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM leave_balances
        WHERE trim(leave_type) = ''
    ) THEN
        RAISE EXCEPTION 'cannot migrate leave_balances with an empty leave_type';
    END IF;
END;
$$;
-- +goose StatementEnd

INSERT INTO leave_types (
    id, tenant_id, code, name, category, source_of_truth, status, created_at, updated_at
)
SELECT DISTINCT ON (balance.tenant_id, lower(trim(balance.leave_type)))
    'lt_' || lower(trim(balance.leave_type)),
    balance.tenant_id,
    lower(trim(balance.leave_type)),
    lower(trim(balance.leave_type)),
    'company',
    CASE
        WHEN lower(trim(balance.source)) = 'ehrms' THEN 'ehrms'
        WHEN lower(trim(balance.source)) = 'overtime' THEN 'overtime'
        ELSE 'manual'
    END,
    'active',
    balance.updated_at,
    balance.updated_at
FROM leave_balances balance
LEFT JOIN leave_types leave_type
  ON leave_type.tenant_id = balance.tenant_id
 AND lower(leave_type.code) = lower(trim(balance.leave_type))
WHERE leave_type.id IS NULL
ORDER BY balance.tenant_id, lower(trim(balance.leave_type)), balance.updated_at
ON CONFLICT (tenant_id, code) DO NOTHING;

UPDATE leave_balances balance
SET leave_type_id = leave_type.id
FROM leave_types leave_type
WHERE leave_type.tenant_id = balance.tenant_id
  AND lower(leave_type.code) = lower(trim(balance.leave_type));

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM leave_balances balance
        LEFT JOIN leave_types leave_type
          ON leave_type.tenant_id = balance.tenant_id
         AND leave_type.id = balance.leave_type_id
        WHERE balance.leave_type_id = '' OR leave_type.id IS NULL
    ) THEN
        RAISE EXCEPTION 'cannot migrate leave_balances with an unresolved leave_type_id';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM leave_balances
        GROUP BY tenant_id, employee_id, leave_type_id, period_start, period_end
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot migrate duplicate leave balance buckets to canonical leave_type_id';
    END IF;
END;
$$;
-- +goose StatementEnd

ALTER TABLE leave_balance_ledger ADD COLUMN leave_type_id text;

UPDATE leave_balance_ledger ledger
SET leave_type_id = balance.leave_type_id
FROM leave_balances balance
WHERE balance.tenant_id = ledger.tenant_id
  AND balance.id = ledger.balance_id;

ALTER TABLE leave_balance_ledger ALTER COLUMN leave_type_id SET NOT NULL;

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

ALTER TABLE leave_balances DROP CONSTRAINT leave_balances_employee_type_period_idx;
ALTER TABLE leave_balances DROP CONSTRAINT leave_balances_period_no_overlap;
ALTER TABLE leave_balances ALTER COLUMN leave_type_id DROP DEFAULT;
ALTER TABLE leave_balances DROP COLUMN leave_type;
ALTER TABLE leave_balances DROP COLUMN policy_version;
ALTER TABLE leave_balances DROP COLUMN prorate_ratio;

ALTER TABLE leave_balances
    ADD CONSTRAINT leave_balances_employee_type_period_idx
    UNIQUE NULLS NOT DISTINCT (tenant_id, employee_id, leave_type_id, period_start, period_end);
ALTER TABLE leave_balances
    ADD CONSTRAINT leave_balances_period_no_overlap EXCLUDE USING gist (
        tenant_id WITH =,
        employee_id WITH =,
        leave_type_id WITH =,
        daterange(period_start, period_end, '[]') WITH &&
    );
ALTER TABLE leave_balances
    ADD CONSTRAINT leave_balances_leave_type_fk
    FOREIGN KEY (tenant_id, leave_type_id) REFERENCES leave_types (tenant_id, id);

DROP INDEX leave_balances_tenant_id_idx;
DROP INDEX leave_balance_ledger_employee_period_idx;
ALTER TABLE leave_balance_ledger DROP COLUMN leave_type;
ALTER TABLE leave_balance_ledger
    ADD CONSTRAINT leave_balance_ledger_leave_type_fk
    FOREIGN KEY (tenant_id, leave_type_id) REFERENCES leave_types (tenant_id, id);
CREATE INDEX leave_balance_ledger_employee_period_idx
ON leave_balance_ledger (tenant_id, employee_id, leave_type_id, period_start, occurred_at);

ALTER TABLE leave_balances ENABLE TRIGGER leave_balances_ledger_trigger;

-- +goose Down
ALTER TABLE leave_balances DISABLE TRIGGER leave_balances_ledger_trigger;

ALTER TABLE leave_balances ADD COLUMN leave_type text;
ALTER TABLE leave_balances ADD COLUMN policy_version integer NOT NULL DEFAULT 0;
ALTER TABLE leave_balances ADD COLUMN prorate_ratio double precision;

UPDATE leave_balances balance
SET leave_type = leave_type.code
FROM leave_types leave_type
WHERE leave_type.tenant_id = balance.tenant_id
  AND leave_type.id = balance.leave_type_id;

ALTER TABLE leave_balances ALTER COLUMN leave_type SET NOT NULL;
ALTER TABLE leave_balances ALTER COLUMN leave_type_id SET DEFAULT '';

ALTER TABLE leave_balance_ledger ADD COLUMN leave_type text;
UPDATE leave_balance_ledger ledger
SET leave_type = leave_type.code
FROM leave_types leave_type
WHERE leave_type.tenant_id = ledger.tenant_id
  AND leave_type.id = ledger.leave_type_id;
ALTER TABLE leave_balance_ledger ALTER COLUMN leave_type SET NOT NULL;

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
        tenant_id, balance_id, employee_id, leave_type, period_start, period_end,
        event_type, delta_hours, remaining_hours, granted_hours, used_hours, source, occurred_at
    ) VALUES (
        NEW.tenant_id, NEW.id, NEW.employee_id, NEW.leave_type, NEW.period_start, NEW.period_end,
        ledger_event_type, NEW.remaining_hours - CASE WHEN TG_OP = 'INSERT' THEN 0 ELSE OLD.remaining_hours END,
        NEW.remaining_hours, NEW.granted_hours, NEW.used_hours, NEW.source, NEW.updated_at
    );
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

ALTER TABLE leave_balances DROP CONSTRAINT leave_balances_leave_type_fk;
ALTER TABLE leave_balances DROP CONSTRAINT leave_balances_employee_type_period_idx;
ALTER TABLE leave_balances DROP CONSTRAINT leave_balances_period_no_overlap;
ALTER TABLE leave_balances
    ADD CONSTRAINT leave_balances_employee_type_period_idx
    UNIQUE NULLS NOT DISTINCT (tenant_id, employee_id, leave_type, period_start, period_end);
ALTER TABLE leave_balances
    ADD CONSTRAINT leave_balances_period_no_overlap EXCLUDE USING gist (
        tenant_id WITH =,
        employee_id WITH =,
        leave_type WITH =,
        daterange(period_start, period_end, '[]') WITH &&
    );
CREATE INDEX leave_balances_tenant_id_idx ON leave_balances (tenant_id);

DROP INDEX leave_balance_ledger_employee_period_idx;
ALTER TABLE leave_balance_ledger DROP CONSTRAINT leave_balance_ledger_leave_type_fk;
ALTER TABLE leave_balance_ledger DROP COLUMN leave_type_id;
CREATE INDEX leave_balance_ledger_employee_period_idx
ON leave_balance_ledger (tenant_id, employee_id, leave_type, period_start, occurred_at);

ALTER TABLE leave_balances ENABLE TRIGGER leave_balances_ledger_trigger;
