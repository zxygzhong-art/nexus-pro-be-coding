-- +goose Up
-- Retire workflow stage type "parallel" (會簽): migrate rows/JSON to approver+mode=all, then tighten CHECK.

UPDATE workflow_stage_instances
SET stage_type = 'approver'
WHERE stage_type = 'parallel';

-- Normalize parallel nodes inside run stage definition snapshots.
UPDATE workflow_runs
SET stage_definitions_json = (
    SELECT COALESCE(jsonb_agg(
        CASE
            WHEN COALESCE(elem->>'type', '') = 'parallel' THEN
                jsonb_set(
                    jsonb_set(elem, '{type}', '"approver"', true),
                    '{config,mode}',
                    '"all"',
                    true
                )
            ELSE elem
        END
        ORDER BY ordinality
    ), '[]'::jsonb)
    FROM jsonb_array_elements(
        CASE
            WHEN jsonb_typeof(stage_definitions_json) = 'array' THEN stage_definitions_json
            ELSE '[]'::jsonb
        END
    ) WITH ORDINALITY AS t(elem, ordinality)
)
WHERE stage_definitions_json::text LIKE '%parallel%';

-- Normalize parallel nodes in form template workspace_design.stages.
UPDATE form_templates
SET schema = jsonb_set(
    schema,
    '{workspace_design,stages}',
    (
        SELECT COALESCE(jsonb_agg(
            CASE
                WHEN COALESCE(elem->>'type', '') = 'parallel' THEN
                    jsonb_set(
                        jsonb_set(elem, '{type}', '"approver"', true),
                        '{config,mode}',
                        '"all"',
                        true
                    )
                ELSE elem
            END
            ORDER BY ordinality
        ), '[]'::jsonb)
        FROM jsonb_array_elements(
            CASE
                WHEN jsonb_typeof(schema #> '{workspace_design,stages}') = 'array'
                    THEN schema #> '{workspace_design,stages}'
                ELSE '[]'::jsonb
            END
        ) WITH ORDINALITY AS t(elem, ordinality)
    ),
    true
),
updated_at = NOW()
WHERE schema::text LIKE '%"parallel"%';

UPDATE form_template_versions
SET schema = jsonb_set(
    schema,
    '{workspace_design,stages}',
    (
        SELECT COALESCE(jsonb_agg(
            CASE
                WHEN COALESCE(elem->>'type', '') = 'parallel' THEN
                    jsonb_set(
                        jsonb_set(elem, '{type}', '"approver"', true),
                        '{config,mode}',
                        '"all"',
                        true
                    )
                ELSE elem
            END
            ORDER BY ordinality
        ), '[]'::jsonb)
        FROM jsonb_array_elements(
            CASE
                WHEN jsonb_typeof(schema #> '{workspace_design,stages}') = 'array'
                    THEN schema #> '{workspace_design,stages}'
                ELSE '[]'::jsonb
            END
        ) WITH ORDINALITY AS t(elem, ordinality)
    ),
    true
)
WHERE schema::text LIKE '%"parallel"%';

ALTER TABLE workflow_stage_instances
    DROP CONSTRAINT IF EXISTS workflow_stage_instances_stage_type_check;

ALTER TABLE workflow_stage_instances
    ADD CONSTRAINT workflow_stage_instances_stage_type_check
    CHECK (stage_type IN ('notify', 'condition', 'approver'));

-- +goose Down
ALTER TABLE workflow_stage_instances
    DROP CONSTRAINT IF EXISTS workflow_stage_instances_stage_type_check;

ALTER TABLE workflow_stage_instances
    ADD CONSTRAINT workflow_stage_instances_stage_type_check
    CHECK (stage_type IN ('notify', 'condition', 'approver', 'parallel'));
