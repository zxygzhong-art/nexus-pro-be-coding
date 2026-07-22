-- +goose Up
-- Retire the unused employment-contract domain and its persisted data.

UPDATE permission_sets
SET permissions = (
    SELECT COALESCE(jsonb_agg(entry ORDER BY ordinal), '[]'::jsonb)
    FROM jsonb_array_elements(permission_sets.permissions) WITH ORDINALITY AS items(entry, ordinal)
    WHERE entry->>'resource' <> 'hr.employment_contract'
)
WHERE permissions @> '[{"resource":"hr.employment_contract"}]'::jsonb;

DELETE FROM permissions
WHERE resource = 'hr.employment_contract';

DROP TABLE IF EXISTS employment_contracts;

-- +goose Down
CREATE TABLE employment_contracts (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id text NOT NULL,
    contract_type text NOT NULL CHECK (contract_type IN ('fulltime', 'parttime', 'contractor', 'intern')),
    contract_no text NOT NULL DEFAULT '',
    start_date timestamptz NOT NULL,
    end_date timestamptz,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'expired', 'terminated', 'renewed')),
    attachment_object_key text NOT NULL DEFAULT '',
    notes text NOT NULL DEFAULT '',
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT employment_contracts_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT employment_contracts_employee_fk FOREIGN KEY (tenant_id, employee_id) REFERENCES employees (tenant_id, id)
);

CREATE INDEX employment_contracts_tenant_employee_idx ON employment_contracts (tenant_id, employee_id, start_date DESC);
CREATE INDEX employment_contracts_tenant_status_idx ON employment_contracts (tenant_id, status, end_date);

ALTER TABLE employment_contracts ENABLE ROW LEVEL SECURITY;
ALTER TABLE employment_contracts FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_employment_contracts ON employment_contracts
    USING (tenant_id = current_setting('app.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
