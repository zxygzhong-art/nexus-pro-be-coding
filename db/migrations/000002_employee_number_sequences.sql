-- +goose Up

CREATE TABLE employee_number_sequences (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    prefix text NOT NULL,
    next_value integer NOT NULL DEFAULT 1 CHECK (next_value > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, prefix)
);

ALTER TABLE employee_number_sequences ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_employee_number_sequences
    ON employee_number_sequences
    USING (tenant_id = current_setting('app.tenant_id', true));

-- +goose Down

DROP TABLE IF EXISTS employee_number_sequences;
