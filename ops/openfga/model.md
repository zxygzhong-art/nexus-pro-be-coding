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
  - `admin`: direct tenant administrator account. There is no structured
    product source of truth in this batch; grant it explicitly with
    `tenantctl openfga-grant-tenant-admin`.
  - `security_admin`: direct tenant security administrator account. Grant it
    explicitly with `tenantctl openfga-grant-tenant-security-admin`.

- `org_unit`
  - `tenant`: direct tenant relation, stored as
    `org_unit:<id>#tenant@tenant:<tenant_id>`. This enables tenant-admin
    inheritance without changing the first-batch parent/member semantics.
  - `parent`: direct parent org unit. The tuple is stored on the child object,
    for example `org_unit:child#parent@org_unit:parent`.
  - `member`: direct `account` membership in this exact org unit, derived from
    the linked employee account and `employees.org_unit_id`.
  - `manager`: direct `account` manager of this org unit. The first batch keeps
    the relation in the model; source-of-truth wiring can be expanded when org
    manager data exists.
  - `member_recursive`: `member` union parent `member_recursive`. This lets a
    member of a parent org unit match descendants through tuple-to-userset.
  - `viewer`: `member` or `manager` or parent `viewer` or tenant `admin`.
  - `editor`: `manager` or tenant `admin`.

- `user_group`
  - `member`: direct `account` membership in an IAM user group.
  - `manager`: direct `account` manager of a user group. The model relation is
    present for governance checks; this batch does not infer managers from IAM
    permissions.

- `assumable_role`
  - `tenant`: direct tenant relation, stored as
    `assumable_role:<id>#tenant@tenant:<tenant_id>`.
  - `trusted_user`: direct trusted `account`, derived from trust policy
    `accounts` / `account_ids`.
  - `trusted_group`: trusted user group members, stored as
    `assumable_role:<id>#trusted_group@user_group:<group_id>#member`, derived
    from trust policy `user_groups` / `user_group_ids`.
  - `approver`: direct approval account. This batch adds the relation to the
    model but does not infer it from trust policy.
  - `can_assume`: `trusted_user` or `trusted_group` or tenant `admin`.
  - `can_approve`: `approver` or tenant `security_admin`.

- `agent_tool`
  - `tenant`: direct tenant relation for registered backend tools. The current
    built-in tool is `knowledge.search`.
  - `runner`: direct `account` allowed to run the tool. Grant it explicitly with
    `tenantctl openfga-grant-agent-tool --relation runner`.
  - `can_run`: `runner` or tenant `admin`.

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
- Assumable role checks remain guarded by the same switch. When enabled,
  `assumable_role#can_assume` is checked after the existing trust-policy
  whitelist; OpenFGA errors fall back to the whitelist path.
- Agent tool checks also use the same switch. `agent_tool#can_run` is the object
  relationship gate when OpenFGA is available; risk level is retained only for
  audit classification and does not add a second approval gate.

## Model Upgrade Flow

1. Start or connect to the OpenFGA store.
2. Apply the model:

   ```sh
   make openfga-apply-model OPENFGA_BASE_URL=http://127.0.0.1:24081 OPENFGA_STORE_ID=<store-id>
   ```

3. Update the API environment with the returned model ID:

   ```sh
   export OPENFGA_MODEL_ID=<authorization_model_id>
   ```

4. Backfill tuples for each tenant. This computes tenant membership,
   org-unit tenant/parent/member tuples, employee tuples, user-group membership,
   assumable-role trust tuples, and built-in agent-tool tenant tuples:

   ```sh
   go run ./cmd/tenantctl openfga-backfill --tenant-id <tenant-id>
   ```

5. Manually grant tenant and tool operational tuples where needed:

   ```sh
   go run ./cmd/tenantctl openfga-grant-tenant-admin --tenant-id <tenant-id> --account-id <account-id>
   go run ./cmd/tenantctl openfga-grant-tenant-security-admin --tenant-id <tenant-id> --account-id <account-id>
   go run ./cmd/tenantctl openfga-grant-agent-tool --tenant-id <tenant-id> --tool-id knowledge.search --account-id <account-id>
   ```

6. Start the API with `OPENFGA_BASE_URL`, `OPENFGA_STORE_ID`, and
   `OPENFGA_MODEL_ID`. Readiness verifies the configured model ID.
7. Enable FGA-backed checks only after the model ID is updated and backfill is
   complete:

   ```sh
   export OPENFGA_SCOPE_CHECK_ENABLED=true
   ```
