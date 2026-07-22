-- +goose Up
ALTER TABLE leave_type_definitions
    DROP COLUMN IF EXISTS unit,
    DROP COLUMN IF EXISTS paid_ratio;

-- +goose Down
ALTER TABLE leave_type_definitions
    ADD COLUMN unit text NOT NULL DEFAULT 'hour' CHECK (unit IN ('hour', 'day')),
    ADD COLUMN paid_ratio numeric(5,4) NOT NULL DEFAULT 1 CHECK (paid_ratio >= 0 AND paid_ratio <= 1);
