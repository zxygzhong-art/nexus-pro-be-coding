# OpenFGA Model Notes

This file documents `ops/openfga/model.json`. Apply the JSON model with
`make openfga-apply-model`, then set `OPENFGA_MODEL_ID` to the returned
`authorization_model_id`.

## Types and Relations

- `account`
  - Authenticated principal object. It has no relations in this batch.

- `tenant`
  - `member`: direct `account` membership in a tenant. Account creation writes
    this tuple; account disablement deletes it.

- `org_unit`
  - `parent`: direct parent org unit. The tuple is stored on the child object,
    for example `org_unit:child#parent@org_unit:parent`.
  - `member`: direct `account` membership in this exact org unit, derived from
    the linked employee account and `employees.org_unit_id`.
  - `manager`: direct `account` manager of this org unit. The first batch keeps
    the relation in the model; source-of-truth wiring can be expanded when org
    manager data exists.
  - `member_recursive`: `member` union parent `member_recursive`. This lets a
    member of a parent org unit match descendants through tuple-to-userset.

- `user_group`
  - `member`: direct `account` membership in an IAM user group.

- `hr.employee`
  - `owner`: existing direct owner account relation.
  - `manager`: existing direct manager account relation derived from
    `manager_employee_id -> account_id`.
  - `org`: direct org unit relation, stored as
    `hr.employee:<id>#org@org_unit:<org_unit_id>`.
  - `read`: existing `owner` and `manager` readers, plus readers derived through
    `org.manager` and `org.member_recursive`.
  - `update`, `delete`, `invite`, `update_status`, `status_transition`: unchanged
    owner-derived relations.

- `agent.knowledge_article`
  - `viewer` and `read`: unchanged from the previous model.

## Data Scope Mapping

- `department`: exact-department scope. When FGA scope checks are enabled, the
  service checks the target employee org unit with `org_unit#member` and
  `org_unit#manager`.
- `department_subtree`: subtree scope. When FGA scope checks are enabled, the
  service checks target employees with `hr.employee#read`, which includes
  `org.member_recursive`.
- SQL scope filtering remains the default and fallback path. FGA scope checks
  are controlled by `OPENFGA_SCOPE_CHECK_ENABLED=false` by default.

## Model Upgrade Flow

1. Start or connect to the OpenFGA store.
2. Apply the model:

   ```sh
   make openfga-apply-model OPENFGA_API_URL=http://127.0.0.1:24081 OPENFGA_STORE_ID=<store-id>
   ```

3. Export the returned model ID for the API:

   ```sh
   export OPENFGA_MODEL_ID=<authorization_model_id>
   ```

4. Backfill tuples for each tenant:

   ```sh
   go run ./cmd/tenantctl openfga-backfill --tenant-id <tenant-id>
   ```

5. Start the API with `OPENFGA_API_URL`, `OPENFGA_STORE_ID`, and
   `OPENFGA_MODEL_ID`. Readiness verifies the configured model ID.
6. Enable FGA-backed data-scope checks only after backfill is complete:

   ```sh
   export OPENFGA_SCOPE_CHECK_ENABLED=true
   ```

