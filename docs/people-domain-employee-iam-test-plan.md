# People Domain, IAM, and Employee Management Test Plan

Last refreshed: 2026-06-11

## 1. Purpose

This plan covers production-readiness testing for the three Notion requirements:

- Task: 系统架构设计
- Task: 系统权限设计
- Feature: 员工管理

The test goal is not only to prove that happy paths work. The release must prove that employee data, permission decisions, tenant isolation, high-risk operations, audit trails, import/export behavior, and Agent tool access are safe enough for an online production environment.

## 2. Release Gate Summary

Do not release to production unless all P0 gates pass:

| Gate | Required evidence |
| --- | --- |
| Requirement traceability | Every employee-management story maps to API, service behavior, permission point, data model, audit event, and automated/manual test case. |
| Unit and API tests | `go test ./...` passes; all changed domain/service/API paths have focused tests. |
| PostgreSQL integration | Migrations apply cleanly; sqlc code is up to date; tenant isolation and key constraints pass against real PostgreSQL. |
| Authz enforcement | Menu/button experience may be frontend-only, but every business API enforces backend permission, data scope, field policy, and high-risk confirmation. |
| Multi-tenant isolation | No API, import session, export, audit log, Agent run, or relationship tuple can read or mutate another tenant. |
| Employee CRUD and lifecycle | Create, read, update, preview/detail, invite, status transition, soft delete, and batch delete match the Employee feature. |
| Import/export safety | CSV/XLSX preview, row errors, confirmation idempotency, size limits, duplicate handling, encoding, and high-risk approval behavior pass. |
| Auditability | Permission changes, employee high-risk actions, export, import confirm, delete, AssumableRole assume, denied cross-tenant access, and Agent tool calls produce useful audit records. |
| Observability | Request IDs, trace IDs, structured logs, health checks, error rates, latency, DB/Redis dependencies, and high-risk audit failures are visible. |
| Production config | Demo seed disabled by default in production; secrets are not logged; Keycloak/OpenFGA/Redis/Postgres config failure modes are tested. |

## 3. Current Implementation Context

Current branch: `feat/2026-6/yannis/people-domain-foundation`.

Current relevant files:

- `internal/api/v1/routes.go` and the `internal/api/v1/*` controllers: HTTP routes and request authorization wrapper.
- `internal/service/hr_service.go`: employee aggregate, import, export, lifecycle, invite, and batch operations.
- `internal/service/authz_service.go`: runtime permission calculation, data scopes, field policies, AssumableRole handling, and audit decision payloads.
- `internal/domain/authz.go`: route policy metadata and high-risk markers.
- `db/schema.sql` and `db/migrations/000001_init.sql`: IAM, HR, audit, Agent, RLS, and outbox schema.
- `docs/openapi.yaml`: current employee API contract.
- `tests/unit/...`: current unit/API coverage.

Current employee API surface to cover:

| Capability | API |
| --- | --- |
| List and filters | `GET /v1/hr/employees` |
| Create aggregate | `POST /v1/hr/employees` |
| Detail / preview data | `GET /v1/hr/employees/{id}` |
| Patch aggregate sections | `PATCH /v1/hr/employees/{id}` |
| Dashboard counts | `GET /v1/hr/employees/stats` |
| Form options | `GET /v1/hr/employee-options` |
| Import preview | `POST /v1/hr/employees/import/preview` |
| Import confirm | `POST /v1/hr/employees/import/{session_id}/confirm` |
| CSV export | `GET /v1/hr/employees/export` |
| API export | `POST /v1/hr/employees/export` |
| Batch delete | `POST /v1/hr/employees/batch-delete` |
| Single delete | `DELETE /v1/hr/employees/{id}` |
| Invite account | `POST /v1/hr/employees/{id}/invite` |
| Quick status update | `PATCH /v1/hr/employees/{id}/status` |
| Lifecycle transition | `POST /v1/hr/employees/{id}/status-transition` |

## 4. Test Strategy

Use layered validation. Run the cheapest checks first, expand only when a layer finds risk or when the change crosses boundaries.

| Layer | Scope | When required |
| --- | --- | --- |
| Static review | Requirement traceability, OpenAPI diff, schema diff, permission policy diff | Every PR touching these requirements |
| Unit tests | Pure authz rules, service validation, import parsing, employee lifecycle, data-scope filtering | Every behavior change |
| API tests | Gin routes, request parsing, authz headers, approval headers, error format, OpenAPI contract | Every route or handler change |
| PostgreSQL integration | Migration, sqlc queries, unique constraints, RLS, transaction behavior, repository parity | Every schema/query/repository change |
| Redis/cache integration | Permission snapshot keying, version invalidation, TTL, stale permissions | Every authz cache change |
| External integration | Keycloak token validation, OpenFGA tuple sync/check, object storage, Agent runtime | Before production wiring or when adapters change |
| E2E/UI | Employee page workflows and permission-based UI states | Before release candidate |
| Non-functional | Performance, concurrency, security, observability, backup/restore | Before production release |

## 5. Test Data Matrix

Create a deterministic test fixture set and reuse it across unit, API, integration, and E2E layers.

### Tenants

| Tenant | Purpose |
| --- | --- |
| `tenant-a` | Main HR tenant with complete org tree and employees. |
| `tenant-b` | Isolation tenant with same employee numbers/emails to prove tenant-scoped uniqueness. |
| `tenant-empty` | Empty-state UI/API behavior. |

### Organization

| Org | Relationships |
| --- | --- |
| `dept-root` | Tenant root. |
| `dept-product` | Child of root. |
| `dept-platform` | Child of product. |
| `dept-sales` | Sibling of product. |
| `dept-hr` | HR admin department. |

### Accounts and Permission Profiles

| Account | Expected access |
| --- | --- |
| Employee self-service | Can read only self employee record and create own leave request. |
| Direct manager | Can read direct reports. |
| Department manager | Can read department subtree. |
| HR admin | Can manage employees in tenant scope, but high-risk actions still require approval. |
| HR readonly | Can read employees with sensitive fields masked. |
| Security admin | Can manage permission sets, user groups, AssumableRole, and audit logs. |
| Platform support assumed role | Can read tenant data under strict boundary, no export or write. |
| Agent service account | Can run only allowed tools under user permission intersection. |
| Disabled / missing account | Must be rejected. |

### Seeded Demo Login Accounts

All accounts below are seeded by `SeedDemo` for cross-page manual testing and should not be deleted from local demo data. The local frontend demo login password is `Password123!`.

| Email | Account ID | Permission profile | Recommended cross-test pages | Expected boundary |
| --- | --- | --- | --- | --- |
| `admin@demo.local` | `acct-admin` | Platform Admin | Home, Workspace, Employee, Attendance, Workflow, IAM, Audit | Full demo tenant access. |
| `employee@demo.local` | `acct-employee` | Employee Self Service | Home, Tasks, Forms, Clock, own workflow forms | Self-scoped employee, leave, clock, and workflow access only. |
| `audit@demo.local` | `acct-audit` | Audit Viewer | Audit logs, IAM permission-set read, forbidden HR pages | Audit and permission-set read only; HR dashboard/list should be denied. |
| `hr.manager@demo.local` | `acct-hr-manager` | HR Manager | Workspace overview, employees, organization, insights, workspace settings read | HR employee create/update/invite and dashboard read; no delete/export. |
| `hr.readonly@demo.local` | `acct-hr-readonly` | HR Readonly | Workspace overview, employees, organization, insights | Read-only HR/dashboard access; create/update/delete APIs should be denied. |
| `attendance.manager@demo.local` | `acct-attendance-manager` | Attendance Manager | Attendance clock, leave, corrections, worksites, shifts, workspace attendance | Attendance write/approve access; HR employee writes and IAM pages should be denied. |
| `workflow.approver@demo.local` | `acct-workflow-approver` | Workflow Approver | Forms, workflow review queue, workspace overview, notifications | Workflow approve/update access; HR and attendance writes should be denied. |
| `security.admin@demo.local` | `acct-security-admin` | Security Admin | IAM user groups, permission sets, workspace admins, audit logs | IAM/audit administration; HR employee writes should be denied. |
| `insights.viewer@demo.local` | `acct-insights-viewer` | Insights Viewer | Workspace overview, attendance read, turnover, insights | Data dashboard read-only account; writes should be denied. |
| `disabled@demo.local` | `acct-disabled` | Disabled / missing account | Login negative path, `/v1/me`, any protected API | Frontend demo login can issue a local token, but backend must reject the disabled account. |

### Seeded Dashboard User Cohort

For a DB-backed dashboard smoke, run `scripts/seed_demo_dashboard_users.sql` after migrations. It keeps existing test data, fills the `demo` tenant to 100 employees, and adds deterministic login-capable accounts:

| Pattern | Range | Password | Account ID pattern | Purpose |
| --- | --- | --- | --- | --- |
| `demo.bulkNNN@demo.local` | `001` to `097` | `Password123!` | `acct-demo-bulk-NNN` | 100-user dashboard/list/permission smoke data. |

The seeded DB cohort currently covers:

| Dimension | Distribution |
| --- | --- |
| Status | active 69, probation 11, onboarding 8, leave_suspended 7, resigned 5 |
| Departments | 10 org units: HQ, Ops, HR, Finance, Sales, Security, R&D, Product, Marketing, Customer Success |
| July 1 samples | 10 approved leave requests and 71 accepted clock records |

### Employees

Cover every status and category from the Notion feature:

- Status: 在职 / active, 试用中 / probation, 留停 / leave_suspended, 待加入 / onboarding, 离职 / resigned, deleted.
- Category: 全职 / full_time, 兼职 / part_time, 实习 / intern, 约聘 / contractor, other.
- Identity type: local and foreign.
- Required special cases: duplicate company email, duplicate employee number, missing org unit, missing required local ID, missing foreign fields, resignation without date/reason, leave suspension without start/end date.

## 6. Functional Test Plan: Employee Management

### 6.1 Page Entry, Menu, and Permissions

| Case | Expected result | Priority |
| --- | --- | --- |
| User without `hr.employee.read` requests `GET /v1/me/menus` | Employee management menu hidden. | P0 |
| User without `hr.employee.read` calls employee list directly | API returns forbidden, not empty fake success. | P0 |
| User with read but without create/export/delete permissions | Page loads list; create/export/delete API calls are rejected. | P0 |
| Permission revoked while user is active | Menu, batch-check, API, field policies, and cache snapshot all reflect revocation after version bump. | P0 |

### 6.2 Dashboard Counts

| Case | Expected result | Priority |
| --- | --- | --- |
| Active count | Counts status in active/probation only, matching Notion 在职/试用 definition. | P0 |
| Hired this month | Uses hire date in current month and tenant timezone policy. | P1 |
| Left this month | Uses resign date in current month; deleted employees excluded unless product explicitly includes them. | P1 |
| Filters applied | Dashboard uses the same department/status/category/keyword filters as the table. | P1 |

### 6.3 List, Search, Filter, Sort, and Pagination

| Case | Expected result | Priority |
| --- | --- | --- |
| Empty tenant | Returns empty list, `total=0`, stable page metadata. | P0 |
| Keyword search by name/email/employee number | Matches only scoped employee rows; case and whitespace behavior defined. | P0 |
| Department filter | Uses org-unit ID, not display name; department subtree behavior must be explicit. | P0 |
| Status/category filters | Match normalized internal values and display labels. | P0 |
| Pagination | Page size min/max enforced; out-of-range page returns empty items, not error. | P1 |
| Sorting | `created_at` and `hire_date` ascending/descending deterministic with tie breaker. | P1 |
| Field policy masking | Masked fields are not leaked in list response, export, or Agent tool result. | P0 |

### 6.4 Employee Create: Six Profile Sections

| Section | Required tests | Priority |
| --- | --- | --- |
| Basic info | Required name/company email; local employee national ID; foreign employee passport, ARC, tax ID, work permit, contract expiry, broker; email format; duplicate email; duplicate employee number. | P0 |
| Employment info | Company, department, position, superior, agent, hire date, employment status, shift, category, tenure start date; org unit must exist. | P0 |
| Education/military | Required highest education and school if product requires them; conditional graduation/withdrawal date. | P1 |
| Contact info | Mobile, communication address, emergency contact relationship/name/phone. | P1 |
| Insurance info | Labor insurance date/level/salary; health insurance date/level/amount; numeric bounds. | P1 |
| Internal experience | Hidden for new create in UI; after create, latest row must be current and history must update on transfer/status changes. | P1 |
| Avatar | Image format/size validation, remove/replace behavior, object storage policy. | P1 |

Production note: current OpenAPI marks only `name` as required, while the Notion feature lists many required fields. Before production, decide whether the backend should enforce the full HR-required set now or whether some fields are UI-only draft validation. If full HR compliance is required, expand backend validation before release.

### 6.5 Detail, Preview, and Edit

| Case | Expected result | Priority |
| --- | --- | --- |
| Detail response | Groups data into all profile sections and respects data scope/field policy. | P0 |
| Preview then edit | Preview mode does not mutate; edit patch updates only provided fields. | P0 |
| Patch empty fields | Clearing optional fields works; clearing required fields is rejected. | P0 |
| Patch cross-tenant employee ID | Returns not found or forbidden without revealing existence. | P0 |
| Concurrent edit | Last-write policy is explicit; if optimistic locking is required, stale update fails. | P1 |
| Account link changes | Updating employee/account relation keeps `account.employee_id` and employee account ID consistent. | P0 |

### 6.6 Invitation and Account Creation

| Case | Expected result | Priority |
| --- | --- | --- |
| Invite employee with company email | Creates or updates account in same tenant; links employee ID. | P0 |
| Invite with explicit email | Validates format and does not hijack another employee's account. | P0 |
| Re-invite existing employee | Idempotent enough for production; audit records resend/update. | P1 |
| Invite deleted/resigned employee | Product decision required; test expected behavior once defined. | P1 |

### 6.7 Lifecycle Status and Internal Experience

| Transition | Expected checks | Priority |
| --- | --- | --- |
| onboarding -> probation | Hire date and current experience created. | P1 |
| probation -> active | Probation fields and internal experience history update. | P1 |
| active -> leave_suspended | Requires start and end date. | P0 |
| active/probation -> resigned | Requires resign date and reason; disables linked account if required. | P0 |
| active -> deleted through delete/batch delete | Soft delete, no hard data loss, high-risk approval, audit. | P0 |
| invalid transition | Rejects invalid status values and impossible date order. | P0 |

### 6.8 Import Preview and Confirm

| Case | Expected result | Priority |
| --- | --- | --- |
| Download template | UTF-8 with BOM, exactly 9 columns, one sample row if required. | P1 |
| CSV preview valid rows | Creates preview session with parsed row count and no mutation. | P0 |
| XLSX preview valid rows | Same behavior as CSV. | P0 |
| Invalid file type | Rejected before parsing. | P0 |
| Over 500 rows | Rejected with clear error. | P0 |
| Missing required columns | Rejected with file-level error. | P0 |
| Row errors | Email format, missing name/email, nonexistent department, duplicate employee number, duplicate email, invalid date, invalid category/status show row-level errors. | P0 |
| Duplicate inside same import file | Must be detected, not only duplicates against existing DB. | P0 |
| Preview session expiry | Confirm after expiry fails. | P0 |
| Confirm with row errors | Must fail unless product defines partial import. | P0 |
| Confirm idempotency | Second confirm returns conflict and does not duplicate employees. | P0 |
| Import audit | Preview is medium risk; confirm is high risk and audited with row counts and actor. | P0 |
| Generated employee number | Blank employee number must auto-generate if this is required by Notion. Current behavior must be verified/fixed before release. | P0 |

### 6.9 Export

| Case | Expected result | Priority |
| --- | --- | --- |
| Export without approval confirmation | High-risk action blocked with `requires_approval` or forbidden response. | P0 |
| Export with approval | CSV returned only for permitted data scope. | P0 |
| Field policies | Masked/hidden sensitive fields are not present in CSV. | P0 |
| Filtered export | Applies same keyword/department/status/category filters as list. | P0 |
| CSV encoding | UTF-8 with BOM if frontend/download spec requires Excel compatibility. | P1 |
| Audit | Actor, tenant, filters, row count, field policy, data scope, trace/request ID recorded. | P0 |
| Large export | Uses streaming or safe memory limits; timeout behavior defined. | P1 |

### 6.10 Delete and Batch Delete

| Case | Expected result | Priority |
| --- | --- | --- |
| Single delete without approval | Blocked as high-risk. | P0 |
| Single delete with approval | Soft deletes employee; list hides by default; detail behavior defined. | P0 |
| Batch delete mixed valid/invalid IDs | Per-row result includes success/code/message; no cross-tenant leakage. | P0 |
| Batch delete missing reason | Rejected. | P0 |
| Batch delete over practical limit | Rejected or chunked by documented limit. | P1 |
| Delete with linked account | Account disabled/unlinked per product rule and audited. | P0 |

## 7. Functional Test Plan: Permission Center

### 7.1 Permission Model

| Case | Expected result | Priority |
| --- | --- | --- |
| User direct assignment | Direct permission set grants expected permissions. | P0 |
| User group assignment | Group membership grants permission set; group is primary management path. | P0 |
| Multiple groups | Permissions and data scopes union where allowed. | P0 |
| Explicit deny | Deny wins over all allow sources. | P0 |
| PermissionBoundary | Boundary can only shrink final permissions and scopes. | P0 |
| High-risk permission | `requires_approval=true` for high-risk actions. | P0 |
| Missing permission | Response contains explainable denial and missing permission. | P0 |
| Permission package naming | `application.resource.action` remains stable and not bound to UI text/path. | P1 |

### 7.2 Runtime Authz APIs

| API | Required tests | Priority |
| --- | --- | --- |
| `POST /v1/authz/check` | allow, deny, missing permission, field policy, data scope, boundary, assumed role. | P0 |
| `POST /v1/authz/batch-check` | Mixed allow/deny results preserve order and do not short-circuit incorrectly. | P0 |
| `POST /v1/authz/explain` | Returns permission sources and denial reason without leaking other tenants. | P0 |
| `POST /v1/authz/simulate` | High-risk protected; simulation never mutates real assignments. | P0 |
| `GET /v1/me/menus` | Menus are trimmed by permission and version/cache invalidation. | P0 |

### 7.3 Permission Management APIs

| Area | Required tests | Priority |
| --- | --- | --- |
| User groups | Create/list, duplicate names/codes, membership effects, audit. | P0 |
| Permission sets | Create/list, permission expansion, template/version source, audit. | P0 |
| Assignments | Principal types account/user_group/assumable_role/service/agent; data scope and condition association. | P0 |
| Field policies | Visible/masked/hidden/readonly behavior at service response, export, and Agent tool boundary. | P0 |
| Data scopes | self, direct_reports, department, department_subtree, assigned_org_units, custom_condition, tenant, system. | P0 |
| Permission version | Changes increment version; Redis old snapshots stop being used. | P0 |
| Legacy roles | If `/roles` or role bindings remain for migration, tests must prove mapping to UserGroup/AssumableRole. | P1 |

### 7.4 AssumableRole

| Case | Expected result | Priority |
| --- | --- | --- |
| TrustPolicy satisfied | Role can be assumed and session is created with TTL. | P0 |
| TrustPolicy not satisfied | Assume rejected and audited. | P0 |
| MFA/approval required | Session not active until requirement satisfied. | P0 |
| SessionPolicy | Can deny/restrict but cannot add permission outside role/boundary. | P0 |
| Expired session | No longer affects permission decisions. | P0 |
| Platform support readonly | Can read only scoped data; cannot export, update, delete, or manage permissions. | P0 |
| Audit | Assume, denied assume, expiry/end, and all high-risk role activity include session ID. | P0 |

## 8. Multi-Tenant and RLS Test Plan

| Case | Expected result | Priority |
| --- | --- | --- |
| API tenant header/token mismatch | Reject request. | P0 |
| Same employee number in two tenants | Allowed if unique constraints are tenant-scoped. | P0 |
| Same company email in two tenants | Product decision required; current schema appears tenant-scoped. Test expected policy. | P0 |
| Cross-tenant employee detail | No existence leakage. | P0 |
| Cross-tenant import session confirm | Rejected. | P0 |
| Cross-tenant audit query | Rejected or returns only tenant-scoped logs. | P0 |
| PostgreSQL RLS without `app.tenant_id` | Returns no rows or fails safely. | P0 |
| PostgreSQL RLS with tenant A | Cannot see tenant B rows through repository or raw query integration test. | P0 |
| Background jobs/outbox | Always set tenant context before DB access. | P0 |

Production warning: schema includes RLS policies, but production readiness requires proving that every request/repository path sets `app.tenant_id` correctly for PostgreSQL sessions. This must be covered by integration tests, not only unit tests.

## 9. Agent and AI Tool Access Test Plan

| Case | Expected result | Priority |
| --- | --- | --- |
| Agent run without tool permission | Rejected before business data access. | P0 |
| Agent user has read but no export | Agent cannot generate export or call export tool. | P0 |
| Agent boundary narrower than user | Final tool access is user permission intersect Agent AssumableRole boundary. | P0 |
| Unauthorized fields | Do not enter tool result, prompt context, RAG context, model output, or logs. | P0 |
| High-risk tool | Returns approval required; no mutation before human/approval confirmation. | P0 |
| Cross-tenant knowledge search | Tenant-specific RAG chunks only. | P0 |
| Audit | Tool name, actor, tenant, data scope, field policy, decision, trace ID recorded. | P0 |
| Streaming response | Go adapter preserves cancellation, timeout, partial failure, and trace linkage. | P1 |

## 10. Audit and Governance Test Plan

Every audit event must be queryable by authorized security users and protected from normal HR users.

| Event | Minimum fields |
| --- | --- |
| Employee create/update/status/delete/batch delete | tenant, actor, action, employee/resource ID, before/after or change summary, request ID, trace ID. |
| Import preview/confirm | filename, row count, error count, success count, session ID, actor, tenant. |
| Export | filters, row count, field policy, data scope, approval marker, actor, tenant. |
| Permission changes | changed object, before/after diff, matched admin permission, actor, tenant. |
| AssumableRole assume | role ID, trust result, session ID, boundary, expiry, reason, approval. |
| Authz deny | missing permission, denied action, resource, tenant, actor, source IP if available. |
| Agent tool call | run ID, tool, authz result, data scope, field policy, high-risk approval. |

Negative tests:

- Audit write failure for high-risk action must fail closed or trigger compensating alert; it cannot silently allow high-risk mutation.
- Audit logs must not store secrets, raw tokens, full sensitive personal fields, or full prompts containing hidden data.

## 11. API and Contract Testing

Required:

- Validate `docs/openapi.yaml` against OpenAPI schema in CI.
- Generate sample requests/responses for each documented employee endpoint.
- Compare the Gin route table registered from `internal/api/v1/routes.go` with OpenAPI paths; fail CI if a public route is undocumented.
- Validate error format for validation errors, row errors, forbidden, not found, conflict, and approval required.
- Ensure path params, query params, and request bodies reject unknown dangerous/malformed values where appropriate.

Current contract gaps to decide:

- OpenAPI marks only `name` required for `EmployeeInput`, while Notion requires many fields. Decide whether the API supports drafts or strict HR records.
- OpenAPI documents import/export employee APIs, but full authz/IAM APIs should also be documented before external integration.
- If frontend uses `POST /v1/hr/employees/export` while download uses `GET`, document both response schemas and approval behavior.

## 12. Integration Testing

### 12.1 PostgreSQL

Required tests:

- `make migrate-up` on empty DB.
- Migration rollback strategy documented or tested if rollback is supported.
- `make sqlc` produces no diff after query/schema change.
- Repository parity: memory store and PostgreSQL store return equivalent employee/authz behavior.
- Unique indexes: employee number, company email, account ID scoped by tenant and non-empty rules.
- JSON fields round-trip for all six employee sections.
- Soft-delete status and list filtering behavior.
- RLS: all tenant tables enforce tenant isolation with `app.tenant_id`.

### 12.2 Redis

Required tests:

- Permission snapshot key includes tenant, account, app/action/resource/target, assumed role/session, approval confirmation, and permission version.
- Permission changes invalidate tenant snapshots.
- Expired AssumableRole sessions do not survive cache.
- Redis outage behavior is safe: fallback recompute or fail closed for permission checks.

### 12.3 OpenFGA

Required before production wiring:

- Tuple sync from org membership, resource ownership, user group membership, AssumableRole trust, Agent tool runner.
- Outbox retry, poison event visibility, idempotent tuple writes.
- FGA check timeout returns safe denial for protected APIs.
- Relationship model supports department subtree and resource owner cases.

### 12.4 Keycloak

Required before production wiring:

- OIDC token validation, issuer/audience/expiry.
- Tenant and account mapping from token claims.
- MFA marker for AssumableRole requiring MFA.
- Disabled user or removed group revokes access after sync/cache invalidation.

## 13. Security and Privacy Testing

P0 security tests:

- Broken access control: direct URL/API calls for every employee/IAM/audit/Agent endpoint.
- Tenant isolation: headers, token claims, path IDs, import session IDs, and assumed role sessions cannot switch tenant.
- Field-level privacy: salary, national ID, passport, ARC, tax ID, phone, address, insurance info are masked/hidden when policy says so.
- Injection: SQL injection through filters/search/sort, JSON condition injection, CSV formula injection in import/export cells.
- File upload: MIME/extension mismatch, oversized file, malformed XLSX zip, CSV with invalid encoding, virus scanning policy if files persist.
- Rate limiting: authz check, import preview, export, Agent run, and login-adjacent endpoints.
- CSRF/CORS if cookies are used; otherwise ensure bearer tokens only.
- Secrets: no Keycloak tokens, model keys, DB URLs, Redis URLs, or Notion/API tokens in logs/audit/errors.
- Audit tampering: normal tenant admin cannot edit/delete audit logs.

## 14. Performance and Load Testing

Define production targets before load test. Suggested minimum gates:

| Scenario | Target |
| --- | --- |
| Employee list with 10k employees | p95 under 500 ms with indexed filters. |
| Employee keyword search | p95 under 800 ms or documented search limitation. |
| Authz check | p95 under 50 ms cached, p95 under 150 ms uncached. |
| Batch authz for page | p95 under 200 ms for 50 checks. |
| Import preview 500 rows | Completes under 5 s, memory stable. |
| Import confirm 500 rows | Completes safely or uses async job; no partial silent failure. |
| Export 10k rows | Streams or chunks; does not exceed memory budget. |
| Concurrent HR admins | 50 concurrent create/update/import/list flows without data races or duplicate employee numbers. |
| Agent run | Timeout/cancellation controlled; backend resources released. |

Concurrency tests:

- Two creates with same email/employee number.
- Two import confirms on same session.
- Delete while update is in flight.
- Permission revoke while export is in progress.
- AssumableRole session expiry during long-running operation.

## 15. Observability and Operations

Required checks:

- `/healthz` exposes application health; readiness should include DB/Redis/OpenFGA/Keycloak dependency state if deployed behind Kubernetes.
- Structured logs include request ID, tenant ID, account ID, route, status, latency, error code.
- Traces connect frontend/API/authz/DB/Redis/OpenFGA/Agent where available.
- Metrics include request rate, error rate, p95/p99 latency, authz deny count, import failures, export count, audit write failures, cache hit ratio.
- Alerts for cross-tenant denied attempts spike, audit write failure, import/export error spike, DB pool exhaustion, Redis unavailable, OpenFGA unavailable, Agent tool denied spike.
- Runbooks exist for failed import sessions, stuck outbox events, corrupted permission package, RLS misconfiguration, and emergency permission revoke.

## 16. Deployment, Migration, and Rollback

Pre-production checklist:

- `APP_ENV=production` disables demo seed unless explicitly overridden.
- Migrations tested on production-like backup.
- Blue/green or rolling deployment does not mix incompatible permission snapshot versions.
- Permission package versioning and migration plan documented.
- Backfill scripts for role -> user group / AssumableRole migration are idempotent.
- Rollback strategy: if schema migration is not reversible, deploy must include forward-fix plan.
- Backups and restore rehearsal include employees, authz tables, audit logs, import sessions, knowledge articles, and object storage metadata.
- Object storage bucket policies tested for tenant isolation.

## 17. Automated Test Suite Proposal

### Unit Tests

Add or maintain focused unit tests for:

- Employee validation: local/foreign required fields, status-specific fields, duplicate checks.
- Employee hot-field derivation from profile sections.
- Import parser: CSV BOM, XLSX, malformed rows, row limit, duplicate-in-file.
- Import confirm: row errors blocked, expiry, idempotency, generated employee number.
- Employee query filtering/sorting/pagination/stats.
- Soft delete and batch delete results.
- Authz: direct/group/assumed grants, deny precedence, boundary, data scope, field policy, high risk.
- Agent tool gateway authorization.

### API Tests

Add or maintain API tests for:

- Every employee endpoint in `docs/openapi.yaml`.
- Multipart and JSON import preview.
- Approval header behavior for export/delete/batch delete/import confirm/status transition/Agent run.
- Error schemas and status codes.
- Production request context: tenant/account headers or bearer claims.

### Integration Tests

Add a separate integration target, for example:

```sh
docker compose up -d postgres redis
make migrate-up
go test ./tests/integration/... -count=1
```

Integration suite should cover PostgreSQL repository behavior, RLS, Redis authz snapshots, and migration/sqlc drift.

### E2E Tests

Use frontend E2E once the UI is wired:

- HR admin opens Employee Management, filters list, opens preview, edits employee, saves, sees updated row.
- HR admin imports CSV with mixed valid/invalid rows, fixes file, previews, confirms, sees new rows.
- HR admin exports with approval and receives scoped CSV.
- HR admin selects 3 interns and batch deletes with reason.
- Readonly user cannot see create/export/delete buttons and direct API calls fail.
- Self-service user can only see own employee data.

## 18. Manual UAT Scenarios

Run these with product/HR stakeholders:

- Six-tab create form matches HR process and wording.
- Conditional local/foreign identity sections are legally correct for target market.
- Employee status labels and backend normalized values match business language.
- Import template columns are enough for real onboarding; missing 6-tab fields are accepted or added.
- Batch delete semantics are acceptable: soft delete vs resignation vs disabled account.
- Export columns, masking, and audit are acceptable for compliance.
- Permission explain/simulate output is understandable to administrators.
- AssumableRole is not visible as a normal business role.
- Agent can explain policies but cannot leak hidden personal data.

## 19. Requirement Ambiguities and Risks

| Issue | Risk | Recommendation |
| --- | --- | --- |
| Employee required fields mismatch | Notion requires many six-tab fields; current OpenAPI only requires `name`, and current service appears to require name/company email plus selected conditions. | Decide whether backend stores draft employees or production-complete HR records. If production-complete, enforce all required fields server-side. |
| Employee number auto-generation | Notion says employee number is auto-generated and import can leave it blank. | Add deterministic, tenant-scoped, concurrency-safe generator and tests. |
| Status label mapping | Notion uses Chinese/Taiwan labels; backend uses normalized values such as active/probation/onboarding/resigned. | Freeze mapping table in API docs and test every mapping. |
| Delete semantics | Notion says remove/delete, HR systems often require retention and account disablement. | Treat delete as soft delete/offboarding; document whether it differs from resignation. |
| Import row limit | Notion says max 500 rows; must be enforced both frontend and backend. | Add parser/service tests for 501 rows. |
| Department filter | Notion says department dropdown from org settings, but subtree vs exact department is not explicit. | Decide exact-match or subtree per role/filter; test both if both exist. |
| Approval workflow | Requirements say high-risk needs approval, current behavior may use approval confirmation header for some routes. | For production, replace simple confirmation with real approval flow or explicitly scope MVP as admin confirmation only. |
| Keycloak/OpenFGA | Architecture says Keycloak/OpenFGA, README notes they are reserved/not fully wired. | Do not claim production security until real adapters and failure-mode tests pass. |
| RLS session variable | Schema has RLS policies; production requires per-request tenant session variable. | Add PostgreSQL integration tests proving app sets `app.tenant_id` safely on every DB operation. |
| Agent data leakage | Requirements forbid unauthorized fields in prompt/tool/RAG/model output. | Add red-team tests and logging review before enabling Agent with real employee data. |
| Audit durability | High-risk action audit must not silently fail. | Fail closed or write to durable fallback/outbox with alert. |

## 20. Better Implementation Suggestions

- Treat Employee as an aggregate with explicit section schemas instead of fully open JSON for production-required fields. Keep JSON extension fields only for future/custom fields.
- Introduce an `employee_number_sequences` table or transactional sequence strategy per tenant for `IKL030` style numbers.
- Add a strict permission package loader for `hr` that validates permissions, menus, fields, scopes, and route policies at startup.
- Make high-risk approval a first-class workflow state, not only a request header, before real production.
- Add repository-level tenant context wrapper so PostgreSQL `app.tenant_id` cannot be forgotten by individual queries.
- Put import confirm in a transaction or job with clear all-or-nothing/partial-success semantics.
- Store export jobs and generated files for large exports instead of synchronous huge response bodies.
- Add `If-Match` / version field for employee patch if concurrent HR edits are realistic.
- Keep HR service logic in `internal/service/hr_service.go` unless a separate module boundary emerges; do not split purely because the file grows.

## 21. Production Exit Criteria

Production-ready means:

- All P0 test cases in this document pass.
- All requirement ambiguities marked P0 are resolved in product docs or implementation.
- Automated tests cover the critical employee, authz, tenant, audit, and import/export paths.
- Integration tests prove PostgreSQL + Redis behavior, not only in-memory store behavior.
- Security review has no unresolved P0/P1 issues.
- Load test meets agreed targets or has documented capacity limits.
- Observability dashboards and alerts are available before traffic.
- Rollback/restore runbook is tested.
- Product owner, engineering owner, QA owner, and security owner sign off.
