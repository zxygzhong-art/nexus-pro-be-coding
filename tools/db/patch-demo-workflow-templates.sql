-- Patch demo tenant form templates with executable workflow stages for live E2E.
-- Run after migrations:
--   psql "$DATABASE_URL" -f tools/db/patch-demo-workflow-templates.sql

UPDATE form_templates
SET schema = schema || '{
  "workspace_design": {
    "enabled": true,
    "stages": [{
      "id": "stage-admin",
      "type": "approver",
      "label": "審核",
      "detail": "由管理員審核",
      "config": {"account_ids": ["acct-zxy1", "acct-admin"]}
    }]
  }
}'::jsonb
WHERE tenant_id = 'demo'
  AND key IN ('leave-request', 'memo', 'general', 'punch-fix', 'overtime-approval', 'job-change', 'headcount-request', 'resignation', 'expense-claim', 'prepayment', 'advance-reimburse', 'travel-request', 'business-card', 'employment-certificate');
