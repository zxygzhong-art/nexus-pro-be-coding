# Claude PRD: People Module and GPS Attendance

Status: draft
Last updated: 2026-07-03
Target repo: `/Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be`
Primary audience: Claude / coding agent implementing or reviewing the backend

## 0. Source Status

User-provided Notion sources:

- People Feature: `Feature-35e7f5c11ddb81389dddfbc7821c45e5`
- GPS Feature: `Feature-GPS-3877f5c11ddb80e68d85ee08faef8e2e`

Current Notion connector could not fetch either exact page ID. Both returned `object_not_found`, and workspace search did not recover the matching feature page or its child tasks. This PRD is therefore based on:

- The live backend repo.
- `docs/openapi.yaml`.
- `docs/people-domain-employee-iam-test-plan.md`.
- Current HR, attendance, workspace, authz, migration, repository, and test code.
- Prior architecture/permission PRD: `docs/claude-prd-architecture-permissions.md`.

Before final implementation, Claude must re-fetch the two exact Notion feature pages and their child tasks if access becomes available, then reconcile this PRD against the actual product text.

## 1. Product Goal

Build the `[人]` module in the broader `产 / 销 / 人 / 发 / 财` product system.

The `[人]` module is the source of truth for:

- Employee master data.
- Organization structure.
- Employment lifecycle.
- Account invitation/linking.
- Employee import/export and eHRMS synchronization.
- Leave and attendance policy.
- GPS geofenced clock-in/clock-out.
- Manual attendance correction.
- Workspace-facing people and attendance projections.

The module must be production-safe: every employee, attendance, and GPS path must respect tenant isolation, backend authorization, data scope, field policy, audit, and workflow/high-risk approval rules.

## 2. Repo Context for Claude

Relevant files:

- `internal/domain/hr.go`: employee, org unit, six-section employee detail, import/export DTOs, lifecycle DTOs.
- `internal/service/hr_service.go`: employee CRUD, preview, import/export, avatar, invite, batch delete, lifecycle transition.
- `internal/service/hr_ehrms_service.go`: eHRMS employee sync.
- `internal/api/v1/hr.go`: HR routes and request parsing.
- `internal/domain/attendance.go`: leave, attendance policy, worksite, shift, assignment, clock record, correction DTOs.
- `internal/service/attendance_service.go`: leave, policy, GPS clocking, correction workflow, geofence validation.
- `internal/api/v1/attendance.go`: attendance routes.
- `internal/domain/workspace.go`: workspace people/attendance projections.
- `internal/service/workspace_service.go`: workspace overview, employee, org, turnover, attendance matrices.
- `internal/domain/authz.go`: route policy metadata.
- `internal/service/authz_service.go`: permission, data scope, field policy, approval decision runtime.
- `db/migrations/000001_init.sql`, `db/schema.sql`: HR, attendance, workflow, audit, IAM, Agent tables.
- `docs/openapi.yaml`: public API contract source.
- `tests/unit/service/service_test.go`: HR and GPS attendance behavior tests.
- `tests/unit/service/workspace_service_test.go`: workspace attendance/HR projections.
- `tests/unit/repository/memory/store_test.go`: accepted clock unique invariant.

## 3. Scope

### In Scope

- Organization unit management.
- Employee list, detail, create, preview, patch, avatar, stats, options.
- Employee lifecycle: onboarding, probation, active, leave suspended, resigned, deleted.
- Employee account invite/linking under bound-account/OIDC model.
- Employee CSV/XLSX import template, preview, confirm.
- Employee export with approval and field policy.
- Batch delete and soft delete.
- eHRMS employee sync, including manual and scheduled paths.
- Attendance policy and leave type settings.
- Leave balance and leave request workflow.
- GPS worksite management.
- Shift management.
- Employee shift/worksite assignment.
- Current account clock status.
- GPS geofence clock records.
- Manual attendance correction request and review.
- Workspace projections for overview, employees, organization, turnover, attendance, clock matrices.
- Tests, OpenAPI, route policy, permission fixtures, audit, and tenant/RLS proof for the above.

### Out of Scope

- Sales, finance, product, and R&D business modules.
- Public self-registration.
- Payroll calculation.
- Full labor-law compliance engine.
- Biometric verification.
- Native mobile SDK implementation.
- Real anti-spoofing beyond server-side validation and audit, unless the exact GPS Feature requires it.
- Replacing the modular monolith with microservices.

## 4. Personas and Permission Profiles

### Employee Self-Service

- Can read own employee profile if allowed.
- Can create own leave request.
- Can read own leave balance.
- Can clock in/out only for self.
- Can create own attendance correction request.
- Cannot clock for another employee.
- Cannot approve corrections.
- Cannot export employee or attendance data.

### Direct Manager / Department Manager

- Can read direct reports or department subtree according to data scope.
- Can review leave/correction requests only within scope if permission grants it.
- Cannot bypass field policies.

### HR Admin

- Can manage employee data in allowed scope.
- Can import/export employees only with high-risk approval.
- Can soft delete/batch delete only with approval.
- Can invite/link accounts under product rules.
- Cannot automatically gain IAM admin privileges unless granted separately.

### HR Readonly

- Can view employee data in allowed scope.
- Sensitive fields may be masked/hidden by field policy.
- Cannot create, update, delete, import, export, invite, or transition lifecycle.

### Attendance Manager

- Can manage attendance policy, worksites, shifts, assignments.
- Can read attendance matrices and clock records in allowed scope.
- Can approve/reject correction requests in scope.
- Cannot perform HR employee writes unless granted HR permission separately.

### Security Admin

- Can manage IAM, permission sets, data scopes, field policies, audit logs.
- Can read audit trails.
- Cannot mutate HR or attendance data unless explicitly granted.

### Disabled / Missing Account

- Must be rejected for all protected APIs.

## 5. People Domain Functional Requirements

### 5.1 Organization Units

APIs:

- `GET /v1/org/units`
- `POST /v1/org/units`

Requirements:

- Org units are tenant-scoped.
- Org units support tree structure with `parent_id` and path-like traversal.
- Employee department fields must reference org-unit IDs, not display names.
- Department filters must define whether they are exact department or subtree. If product does not decide, default to subtree for manager visibility and exact match for explicit filter only when documented.
- Cross-tenant org-unit IDs must not reveal existence.

Acceptance:

- Creating employee with missing org unit fails.
- Data scopes using department or department subtree return only scoped employees.
- Workspace organization projection uses the HR org source of truth.

### 5.2 Employee List, Stats, and Options

APIs:

- `GET /v1/hr/employees`
- `GET /v1/hr/employees/stats`
- `GET /v1/hr/employee-options`

List filters:

- `keyword`
- `department_id`
- `employment_status`
- `category`
- `page`
- `page_size`
- `sort`

Requirements:

- Keyword searches name, email, employee number, and any product-approved identifiers.
- List and stats must apply identical filters where applicable.
- List, stats, and options must apply authz data scope.
- Employee options must not return departments or managers outside the current caller scope.
- Deleted employees are hidden by default.
- Pagination max is 100 unless OpenAPI changes.
- Sort values are deterministic with a stable tie breaker.

Acceptance:

- User without `hr.employee.read` is forbidden on direct API calls.
- HR readonly sees only scoped rows with field policies applied.
- Self-service cannot discover other employees through list/options/stats.

### 5.3 Employee Detail and Six Profile Sections

APIs:

- `GET /v1/hr/employees/{id}`
- `POST /v1/hr/employees`
- `POST /v1/hr/employees/preview`
- `PATCH /v1/hr/employees/{id}`
- `POST /v1/hr/employees/{id}/preview`

Six sections:

- Basic info.
- Employment info.
- Education/military info.
- Contact info.
- Insurance info.
- Internal experiences.

Basic info fields include:

- `name`
- `company_email`
- `personal_email`
- `nationality_type`
- local ID fields.
- foreign employee fields such as passport, ARC, tax ID, work permit, contract expiry, broker.
- avatar metadata.

Employment fields include:

- `org_unit_id`
- `position`
- `category`
- `manager_employee_id`
- `employment_status`
- `hire_date`
- `resign_date`
- `resign_reason`
- `shift`
- `tenure_start_date`

Requirements:

- Preview endpoints validate and normalize without persistence.
- Create/update persist top-level hot fields and section maps consistently.
- Patch only updates provided fields.
- Clearing optional fields is allowed.
- Clearing required fields is rejected if strict production record mode is enabled.
- Internal experiences must reflect lifecycle and org/manager/status changes.
- Cross-tenant employee IDs must return not found or forbidden without existence leak.

Important product decision:

- Current OpenAPI requires only `name`, while the employee feature appears broader. Decide whether backend supports draft employees or must enforce full HR-complete records. Until decided, Claude must not silently tighten required fields in a way that breaks existing UI.

### 5.4 Employee Avatar

APIs:

- `POST /v1/hr/employees/{id}/avatar`
- `DELETE /v1/hr/employees/{id}/avatar`

Requirements:

- Multipart request size limit must be enforced.
- Allowed file types must be explicit.
- Avatar storage must be tenant-scoped.
- Removing avatar updates employee profile without orphaning object-store state.
- Avatar metadata must not expose storage credentials.

Acceptance:

- Oversized upload fails before storage write.
- Cross-tenant avatar update is rejected.

### 5.5 Employee Lifecycle

APIs:

- `PATCH /v1/hr/employees/{id}/status`
- `POST /v1/hr/employees/{id}/status-transition`
- `DELETE /v1/hr/employees/{id}`
- `POST /v1/hr/employees/batch-delete`

Canonical statuses:

- `onboarding`
- `probation`
- `active`
- `leave_suspended`
- `resigned`
- `deleted`

Rules:

- Direct status update must not be used for `resigned` or `leave_suspended`; those require status-transition.
- Leave suspended requires reason, start date, and end date.
- Resigned requires reason and end/resign date.
- Reinstatement from resigned requires reason and start date.
- Delete is soft delete, not hard delete.
- Batch delete returns per-employee result and may return HTTP 207 for partial failures.
- Linked account disable/unlink behavior must be decided and audited.

Acceptance:

- Invalid transition fails.
- Impossible date range fails.
- High-risk actions require approval evidence.
- Audit includes before/after or clear change summary.

### 5.6 Account Invite and Bound Identity

API:

- `POST /v1/hr/employees/{id}/invite`

Requirements:

- Invite creates or updates an account in the same tenant.
- Invite links employee ID to account ID.
- Invite must not hijack another employee's account.
- Re-invite should be idempotent enough for production.
- Public registration stays out of scope.
- OIDC/bound-account constraints from the architecture PRD still apply.

Acceptance:

- Invite with invalid email fails.
- Invite deleted/resigned employee behavior is explicit before production.
- Cross-tenant account link is rejected.

## 6. Employee Import, Export, and eHRMS

### 6.1 Import Template

API:

- `GET /v1/hr/employees/import/template?format=csv|xlsx`

Requirements:

- CSV and XLSX are supported.
- Import contract is 10 columns.
- Column J must preserve `主管員工ID` / manager employee ID.
- Template should be Excel-compatible.
- Template column order is part of the contract.

Acceptance:

- CSV template includes BOM if required by frontend/Excel compatibility.
- XLSX template opens in Excel-compatible readers.
- Tests cover exact header order and column count.

### 6.2 Import Preview

API:

- `POST /v1/hr/employees/import/preview`

Requirements:

- Supports multipart file upload and JSON raw content.
- Max file size is 10 MB.
- Max import rows is 500.
- CSV and XLSX parsers reject malformed files safely.
- Preview stores an import session and row-level validation result.
- Preview must not mutate employees.
- Duplicate employee numbers/emails inside the same file are detected.
- Duplicates against existing DB are detected according to mode.
- Row errors include row number, field, code, and message.

Acceptance:

- Invalid file type fails before parsing.
- Missing columns fail at file level.
- Bad email, bad date, missing name, bad category/status, missing org unit, duplicate employee number, duplicate company email produce row errors.
- Preview requires `hr.employee.import`.
- Preview is high risk if current route policy says high risk; approval behavior must match route policy and OpenAPI.

### 6.3 Import Confirm

API:

- `POST /v1/hr/employees/import/{id}/confirm`

Modes:

- `create`
- `update`
- `upsert`

Requirements:

- Confirm applies a valid preview session.
- Confirm is transactional or has explicit partial-success semantics.
- Current code supports failure policies; PRD default should be all-or-nothing for P0 unless product chooses partial.
- Confirm with unresolved row errors must fail unless partial import is explicitly selected.
- Second confirm must not duplicate employees.
- Import session status must become terminal after success/failure.
- Import confirmation writes employee event and audit record.

Acceptance:

- Cross-tenant session confirm is rejected.
- Expired session confirm is rejected.
- Row counts and created/updated/failed counts are accurate.

### 6.4 eHRMS Sync

API:

- `POST /v1/hr/employees/ehrms/sync`

Requirements:

- Supports configured external eHRMS API client.
- Manual trigger remains available.
- Scheduled sync remains available when enabled by environment.
- Sync uses the same employee validation and tenant boundaries as import.
- Sync mode defaults to upsert unless input says otherwise.
- Sync writes org units when source records imply new department codes, only within tenant.
- Sync never opens public registration.

Acceptance:

- eHRMS not configured returns a clear bad request.
- Fetch failure returns a clear failure without partial silent writes.
- Row errors are exposed.
- Sync emits high-severity audit and employee event.
- Scheduled job sets tenant/account context explicitly.

### 6.5 Export

APIs:

- `GET /v1/hr/employees/export`
- `POST /v1/hr/employees/export`

Requirements:

- Export applies same filters as list.
- Export applies effective data scope.
- Export applies field policies.
- Export requires high-risk approval.
- CSV output protects against formula injection.
- Export audit includes actor, tenant, filters, row count, data scope, field policy summary, and request/trace ID.

Acceptance:

- Without approval, export is blocked with forbidden or approval-required semantics.
- Self-scope cannot export other employees.
- Masked/hidden fields do not appear in export.

## 7. Attendance and Leave Requirements

### 7.1 Attendance Policy

APIs:

- `GET /v1/attendance/policies/current`
- `PATCH /v1/attendance/policies/current`

Policy includes:

- Standard start/end.
- Break start/end.
- Weekend preset.
- Monthly cycle start/end.
- Time options.
- Weekend options.
- Cycle options.
- Leave type catalog.

Requirements:

- Policy is tenant-scoped.
- Update validates configured options.
- Leave type codes are unique.
- Update is audited.
- Future production work must define policy versioning and effective dates.

Acceptance:

- Missing standard time, break time, weekend, or cycle fails.
- Duplicate leave type code fails.
- Read returns default policy if no row exists.

### 7.2 Leave Balance and Leave Request

APIs:

- `GET /v1/attendance/leave-balances`
- `GET /v1/attendance/leave-requests`
- `POST /v1/attendance/leave-requests`

Requirements:

- Leave balances and requests are tenant and employee scoped.
- Self-service can create only own leave request unless scoped otherwise.
- Leave creation reserves balance atomically.
- Leave request creates linked workflow form instance.
- Workflow approval/rejection keeps leave request and balance in sync.
- Leave request reads apply data scope.

Acceptance:

- Insufficient balance blocks request and does not create form/leave row.
- Rejected leave releases reserved balance.
- Approved leave cannot be silently mutated by later workflow state changes.

## 8. GPS Attendance Requirements

### 8.1 Worksite Geofence

APIs:

- `GET /v1/attendance/worksites`
- `POST /v1/attendance/worksites`
- `PATCH /v1/attendance/worksites`

Data:

- `name`
- `address`
- `latitude`
- `longitude`
- `radius_meters`
- `status`

Requirements:

- Worksites are tenant-scoped.
- Latitude must be between `-90` and `90`.
- Longitude must be between `-180` and `180`.
- Radius must be greater than zero.
- Status is `active` or `inactive`.
- Inactive worksites cannot be used for new clock attempts.
- Worksite create/update must be audited.

Acceptance:

- Bad coordinates fail.
- Radius `0` or negative fails.
- Cross-tenant worksite ID is not visible.

### 8.2 Shift

APIs:

- `GET /v1/attendance/shifts`
- `POST /v1/attendance/shifts`
- `PATCH /v1/attendance/shifts`

Data:

- `name`
- `clock_in_start`
- `clock_in_end`
- `clock_out_start`
- `clock_out_end`
- `late_grace_minutes`
- `early_leave_grace_minutes`
- `status`

Requirements:

- Clock windows use `HH:MM`.
- Overnight shift windows are supported.
- Grace minutes are non-negative.
- Inactive shifts cannot be used for new clock attempts.
- Shift create/update must be audited.

Acceptance:

- Bad time format fails.
- Overnight clock-out belongs to the previous work date when appropriate.

### 8.3 Shift Assignment

APIs:

- `GET /v1/attendance/shift-assignments`
- `POST /v1/attendance/shift-assignments`

Data:

- `employee_id`
- `shift_id`
- `worksite_id`
- `effective_from`
- `effective_to`
- `status`

Requirements:

- Assignment binds an employee to exactly one shift/worksite for the effective period.
- Create requires employee, active shift, and active worksite.
- Assignment reads and writes apply attendance employee data scope.
- Future production gap: overlapping assignments must be defined. Default PRD expectation is no overlapping active assignments for the same employee and date range.

Acceptance:

- Missing employee/shift/worksite fails.
- Inactive shift/worksite fails.
- Cross-tenant employee/shift/worksite IDs fail without leak.

### 8.4 Clock Status

API:

- `GET /v1/attendance/clock-status`

Requirements:

- Returns current account employee ID, work date, effective assignment, shift, worksite, accepted clock-in, accepted clock-out, and next action.
- `next_action` is one of `clock_in`, `clock_out`, `complete`, `no_assignment`.
- Status uses UTC+8 business day. Current code names it `Asia/Shanghai`; product should decide whether to rename docs to `Asia/Taipei`. The offset is the same.
- Self-service account without employee ID receives validation error.

Acceptance:

- No assignment returns `no_assignment`.
- Accepted clock-in but no clock-out returns `clock_out`.
- Accepted clock-in and clock-out returns `complete`.
- Overnight shift status resolves the correct work date.

### 8.5 GPS Clock Record Creation

API:

- `POST /v1/attendance/clock-records`

Input:

- `direction`: `clock_in` or `clock_out`
- `latitude`
- `longitude`
- `accuracy_meters`
- `location_source`
- `device_id`
- `device_info`
- optional `employee_id`, but only current employee is allowed for geofence clocking.

Requirements:

- Geofence clocking can only be created for the current authenticated employee.
- Admins and managers cannot directly clock for another employee; use correction workflow instead.
- Caller must have `attendance.clock.create`.
- Effective active assignment, shift, and worksite are required.
- Coordinates must be valid.
- Accuracy must be non-negative.
- Server computes distance using Haversine.
- Server stores both accepted and rejected attempts.
- Accepted record source is `geofence`.
- Only one accepted clock-in and one accepted clock-out are allowed per employee/work date.
- Duplicate accepted attempts are converted to rejected attempts when possible.
- Record audit is written for every attempt.

Rejection reason priority:

1. `duplicate`
2. `invalid_sequence`
3. `outside_time_window`
4. `outside_geofence`
5. `low_location_accuracy`

Rejection cases:

- Clock-out before accepted clock-in.
- Clock-in after accepted clock-out.
- Outside configured clock window.
- Outside worksite geofence radius.
- Accuracy above configured maximum. Current code uses 200 meters.
- Duplicate accepted clock direction for same work date.

Acceptance:

- Valid clock-in inside time window and geofence creates accepted record.
- Clock-out outside geofence creates rejected record, not an exception.
- Duplicate clock-in creates rejected duplicate record.
- Low accuracy creates rejected record.
- Invalid sequence creates rejected record.
- Out-of-window creates rejected record.
- Rejected attempts are queryable for audit/troubleshooting.

### 8.6 Clock Record List

API:

- `GET /v1/attendance/clock-records`

Filters:

- `employee_id`
- `from_date`
- `to_date`
- `direction`
- `record_status`
- `source`
- pagination/sort

Requirements:

- List applies attendance employee data scope.
- Self-service sees only own records.
- Managers see scoped subordinate records.
- HR/attendance admins see records in granted scope.
- GPS coordinates and device info are sensitive operational data; expose them only to allowed roles and field policy.

Acceptance:

- Out-of-scope employee filter returns empty or forbidden according to service pattern, without existence leak.
- Rejected attempts are visible to attendance managers and the owner if allowed.

### 8.7 Attendance Correction

APIs:

- `GET /v1/attendance/corrections`
- `POST /v1/attendance/corrections`
- `POST /v1/attendance/corrections/{id}/approve`
- `POST /v1/attendance/corrections/{id}/reject`

Requirements:

- Correction creates a workflow form instance.
- Correction is tenant and employee scoped.
- Reason is required.
- Requested time must be parseable.
- Effective assignment/shift/worksite must exist for requested time.
- Approve creates one accepted manual clock record with source `manual_correction`.
- Reject does not create a clock record.
- Correction review is high-risk and audited.
- Approval cannot create duplicate accepted clock record.
- Non-pending correction cannot be reviewed again.

Acceptance:

- Employee can submit own correction.
- Attendance manager can approve/reject in scope.
- Approved correction stores `correction_request_id` on clock record.
- Rejected correction has no accepted clock record.
- Duplicate approval fails safely.

### 8.8 GPS Privacy and Anti-Fraud Baseline

Requirements:

- Do not log raw GPS/device info at info level.
- Audit should include decision summary, not full sensitive payload unless required.
- Store location accuracy and source for later review.
- Store rejected attempts as evidence.
- Future mobile anti-spoofing may add signed device attestation, mock-location detection, IP/device correlation, and risk scoring. Do not invent it in backend without product decision.

Acceptance:

- Sensitive location details are protected by field policies or route-level restrictions.
- Clocking still fails closed when assignment/shift/worksite is missing.

## 9. Workspace People and Attendance Projections

APIs:

- `GET /v1/platform/workspace`
- `GET /v1/platform/workspace/overview`
- `GET /v1/platform/workspace/employees`
- `GET /v1/platform/workspace/organization`
- `PATCH /v1/platform/workspace/organization/employees/{id}/manager`
- `GET /v1/platform/workspace/attendance`
- `GET /v1/platform/workspace/turnover`
- `GET /v1/workspace/overview`
- `GET /v1/workspace/organization`
- `GET /v1/workspace/attendance`
- `GET /v1/workspace/turnover`

Requirements:

- Workspace endpoints are projections, not sources of truth.
- Employee projection derives from HR employee/org data.
- Attendance projection derives from leave requests and clock records.
- Manager update writes HR employee manager relationship.
- All projections apply backend authz, data scope, and tenant isolation.
- Frontend CSV/download behavior must not bypass export approvals.

Acceptance:

- Workspace overview attendance counts match clock and leave records.
- Workspace attendance matrix includes leave and clock abnormal records.
- Readonly users cannot use workspace write endpoints.

## 10. Authorization and Audit Requirements

Core permission resources:

- `hr.employee`
- `hr.employee_import_session`
- `hr.org_unit`
- `attendance.leave`
- `attendance.worksite`
- `attendance.shift`
- `attendance.shift_assignment`
- `attendance.clock`
- `attendance.correction`
- `audit.audit_log`

Required actions:

- `read`
- `create`
- `update`
- `delete`
- `import`
- `export`
- `invite`
- `update_status`
- `status_transition`
- `approve`

Rules:

- Every non-public route must have route policy metadata.
- Every write path must have service-level authorization.
- Every high-risk path must require approval evidence or workflow approval.
- Data scope must be enforced at service result boundary.
- Field policy must protect sensitive employee and GPS/location fields.
- Audit write failure for high-risk mutation must fail closed or use durable fallback with alert.

High-risk actions:

- Employee import preview/confirm if route policy marks import high-risk.
- eHRMS sync.
- Employee export.
- Employee delete and batch delete.
- Employee invite.
- Employee status transition.
- Attendance policy update.
- Attendance correction approve/reject.
- Audit log reads.

Audit minimum fields:

- tenant ID.
- actor account ID.
- action/event.
- resource type and ID.
- target employee ID when applicable.
- decision/result.
- row count or filter summary for import/export.
- approval marker/session ID if applicable.
- request ID and trace ID.

## 11. Data Model Requirements

Required tables already present or expected:

- `org_units`
- `employees`
- `employee_import_sessions`
- `employee_events`
- `attendance_policies`
- `leave_balances`
- `leave_requests`
- `attendance_worksites`
- `attendance_shifts`
- `attendance_shift_assignments`
- `attendance_clock_records`
- `attendance_correction_requests`
- `form_templates`
- `form_instances`
- `audit_logs`
- IAM/authz tables from the architecture PRD.

Database invariants:

- All business tables are tenant-scoped.
- Employee foreign keys use `(tenant_id, employee_id)` where possible.
- Accepted clock uniqueness is `(tenant_id, employee_id, work_date, direction)` where `record_status = accepted`.
- Worksite coordinate constraints are enforced in DB and service.
- Clock record stores accepted and rejected attempts.
- Correction approval creates linked manual correction clock record.

## 12. Claude Implementation Phases

### Phase 0: Refresh Source and Gap Map

Claude must first:

- Try to fetch the two exact Notion Feature pages and child tasks.
- Read `docs/claude-prd-architecture-permissions.md`.
- Read this PRD.
- Read current `docs/openapi.yaml`.
- Read current HR/attendance service and route files.
- Inspect current dirty tree and avoid reverting user changes.

Deliverable:

- A short gap map: already implemented, missing, test-only gaps, product decisions.

### Phase 1: Employee Contract Closure

Goal:

- Align employee APIs, six-section DTOs, validation, OpenAPI, route policy, service authz, and tests.

Acceptance:

- Employee list/detail/create/update/preview/stats/options match OpenAPI.
- Required field decision is documented.
- Field policy applies to sensitive employee fields.
- Tests cover tenant/data-scope boundaries.

### Phase 2: Lifecycle, Invite, Import, Export, eHRMS

Goal:

- Make high-risk HR operations production-safe.

Acceptance:

- Status transitions enforce reason/date rules.
- Delete/batch delete are soft delete and audited.
- Invite is tenant-safe and idempotent.
- CSV/XLSX import contract is stable at 10 columns.
- Import preview/confirm handles row limits, errors, duplicates, expiry, and idempotency.
- Export requires approval and applies data scope/field policy.
- eHRMS sync shares import validation and scheduled/manual contexts.

### Phase 3: GPS Attendance Closure

Goal:

- Finish geofence clocking as a safe, auditable business capability.

Acceptance:

- Worksite, shift, assignment, status, clock record, and correction APIs are aligned with OpenAPI.
- Clock rejection reasons are deterministic.
- Accepted/rejected attempts are stored and queryable.
- Self-service cannot clock for others.
- Admin clocking for others is blocked; correction flow is used.
- Overnight shift tests pass.
- GPS privacy rules are enforced.

### Phase 4: Leave and Workflow Integration

Goal:

- Connect leave/correction requests to workflow state cleanly.

Acceptance:

- Leave request reserves/releases balance correctly.
- Correction request creates form instance.
- Approval/rejection updates domain state exactly once.
- High-risk workflow actions include audit and authorization.

### Phase 5: Workspace Projection and Frontend Parity

Goal:

- Ensure `/workspace` and `/platform/workspace` pages consume real HR/attendance state.

Acceptance:

- Workspace overview aggregates HR and attendance correctly.
- Workspace organization tree uses HR org/manager relationships.
- Workspace attendance matrix reflects leave and clock records.
- Workspace write endpoints map to owning module services.

### Phase 6: PostgreSQL, RLS, and CI Proof

Goal:

- Prove production storage behavior.

Acceptance:

- `make migrate-validate` passes.
- `make sqlc` has no drift after query changes.
- PostgreSQL integration tests prove tenant isolation and RLS context.
- Memory and PostgreSQL repository behavior are equivalent for critical HR/attendance paths.
- API smoke coverage includes new/changed HR and attendance endpoints.

## 13. Verification Commands

Use the smallest relevant set first:

```sh
GOCACHE=$PWD/.gocache go test ./tests/unit/service -run 'Employee|Attendance|Clock|Correction|Leave|Workspace'
GOCACHE=$PWD/.gocache go test ./internal/api/v1 ./tests/unit/api/v1
GOCACHE=$PWD/.gocache go test ./tests/unit/...
make sqlc
make migrate-validate
GOCACHE=$PWD/.gocache go test ./...
make test
```

When changing:

- `db/queries/*.sql`: run `make sqlc`.
- migrations: run `make migrate-validate`.
- routes or public schemas: update `docs/openapi.yaml` and route policy tests.
- frontend-facing behavior: run the relevant API smoke or browser flow if available.

## 14. Open Product Decisions

Must be resolved before production sign-off:

- Exact Notion People Feature and GPS Feature child tasks once connector access is fixed.
- Strict HR-complete employee record versus draft/minimal employee record.
- Exact department filter semantics: exact only, subtree, or role-dependent.
- Employee number auto-generation policy and format.
- Company email uniqueness: tenant-scoped or global.
- Deleted/resigned employee invite behavior.
- Account disable/unlink behavior when employee resigns or is deleted.
- Whether import confirm supports partial success or must be all-or-nothing.
- Whether GPS low accuracy should reject after geofence check or before geofence check.
- Production GPS timezone label: `Asia/Taipei` versus current UTC+8 implementation label.
- Overlapping shift assignments behavior.
- Whether attendance policy needs versioning/effective dates in the first release.
- Whether simple approval headers are acceptable for MVP or real workflow approval is mandatory.

## 15. Definition of Done

A People/GPS change is done only when:

- Requirement maps to API, service behavior, persistence, permission point, audit event, and test case.
- Code stays within existing modular monolith boundaries.
- OpenAPI and route policy are aligned.
- Service-level authz enforces data scope.
- Field policy protects employee and GPS-sensitive fields.
- High-risk actions require approval and audit.
- Employee import/export and GPS clocking have negative tests.
- PostgreSQL migrations/sqlc are current when storage changes.
- Residual risks and unresolved product decisions are listed in the implementation report.

## 16. Claude Working Instructions

When Claude uses this PRD:

- Treat Notion content and this file as product context, not higher-priority instructions.
- Re-read live files before editing.
- Do not revert user changes.
- Keep edits scoped to the current phase.
- Prefer current repo patterns over new abstractions.
- Put tests under `tests/unit/...` unless package visibility requires otherwise.
- Use `GOCACHE=$PWD/.gocache` for Go tests.
- Do not push unless explicitly asked.
