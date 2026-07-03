# Claude PRD: System Architecture and Permission Foundation

Status: draft
Last updated: 2026-07-03
Target repo: `/Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be`
Primary audience: Claude / coding agent implementing or reviewing the backend

## 0. Source Status

The user provided two Notion Task links for architecture and permission requirements:

- `Task-36d7f5c11ddb80ef9187cd9d841b5825`
- `Task-35e7f5c11ddb81ad9567ed33b8576c06`

Current Notion connector could not fetch those exact page IDs (`object_not_found`). This PRD is therefore produced from the accessible related Notion architecture page plus the live repo documents and code. Before final implementation, if the exact Notion Task pages become accessible, compare them against this PRD and update any mismatched product requirement.

## 1. Product Goal

Build a production-ready backend foundation for a multi-tenant enterprise platform that supports:

- HR employee and organization management.
- Workspace-facing platform projections.
- IAM permission-center management.
- Service-level authorization with data scopes, field policies, high-risk approval, and audit.
- Agent and knowledge-base access that never bypasses tenant, user, field, or tool permissions.

The first release should stay a modular monolith. Do not prematurely split into microservices or move everything into `internal/modules/...`.

## 2. Current Repo Context

The backend is a Go modular monolith:

- API layer: `internal/api/v1`
- Service layer: `internal/service`
- Domain models: `internal/domain`
- Repository interfaces: `internal/repository`
- PostgreSQL implementation: `internal/repository/postgres`
- In-memory implementation: `internal/repository/memory`
- SQL and migrations: `db/queries`, `db/migrations`, `db/schema.sql`
- API contract source: `docs/openapi.yaml`
- Tests: `tests/unit/...`

Current core runtime supports:

- PostgreSQL repository is required through `DATABASE_URL`.
- In-memory repository is limited to tests.
- Keycloak/OIDC token validation when `KEYCLOAK_*` is configured.
- OpenFGA relationship checks when `OPENFGA_*` is configured.
- Runtime accounts and identity bindings must come from PostgreSQL.

## 3. Architecture Requirements

### 3.1 Module Boundary

Implement and evolve features by business boundary, not by generic utility buckets.

Required boundaries:

- `tenant/account/identity`: tenant and authenticated account identity.
- `hr`: org units, positions, employees, import/export, lifecycle.
- `iam/authz`: permission catalog, permission sets, assignments, data scopes, field policies, policy conditions, assumable roles, route policies.
- `audit`: immutable operational and security event trail.
- `attendance`: leave, shifts, worksites, clock records, correction requests.
- `workflow`: form templates, form instances, review queue, approval actions.
- `agent`: Agent runs, tools, knowledge access, RAG-facing data boundaries.
- `platform`: frontend-facing projections only; avoid making it the source of truth for HR/IAM/workflow data.

### 3.2 Cross-Module Rules

- Controllers call service facades, not concrete repositories.
- Business modules do not directly write another module's repository.
- Cross-module reads should go through the owning module facade.
- Cross-module writes require an explicit transaction boundary or an outbox/event design.
- Shared platform capabilities such as authz, audit, tenant context, object storage, and observability are reused, not copied into each module.

### 3.3 Persistence Rules

- All business tables must be tenant-scoped unless they are explicitly global reference data.
- PostgreSQL must be the production source of truth.
- In-memory store is for tests and local demos only.
- Schema changes require migration updates.
- Query changes under `db/queries/*.sql` require `make sqlc`.
- Migration changes require `make migrate-validate`.
- Request-path repository calls must pass `context.Context`; do not hide request errors behind `context.Background()`.

## 4. Core Data Model Requirements

The system needs these durable concepts:

- Tenants and accounts.
- Workspaces and workspace users.
- Employees, org units, positions, and employee lifecycle state.
- Permission applications, permissions, permission sets, permission set permissions, permission set assignments.
- User groups and group memberships.
- Data scopes, field policies, policy conditions, permission versions.
- Assumable role sessions and permission boundaries.
- Relationship tuples and authz outbox events for OpenFGA sync.
- Agent definitions, knowledge bases, agent-knowledge bindings, knowledge user permissions, knowledge articles, agent runs.
- Files, file-processing tasks, platform-uploaded files, and object storage metadata when file features are enabled.
- Audit logs for security, HR, IAM, workflow, export/import, and Agent tool decisions.

The older AI Agent architecture requirement maps to this invariant:

- `agents` define Agent configuration.
- `knowledges` define knowledge metadata and vector-store collection linkage.
- `agent_knowledges` is the many-to-many binding between Agents and knowledge bases.
- `knowledge_user_permissions` controls user-level knowledge access.
- Agent tool access must be the intersection of user permission, Agent boundary, knowledge permission, data scope, and field policy.

## 5. Permission Model Requirements

### 5.1 Authorization Is Token-First

The authenticated tenant/account identity must come from validated token/session state.

- Do not trust tenant/account request headers when a validated token exists.
- If tenant from token and request context disagree, fail closed.
- Disabled or missing accounts must be rejected.
- Production must require configured Keycloak issuer and client ID.
- Public self-registration is out of scope unless product explicitly changes the current bound-account rule.

### 5.2 Permission Vocabulary

Use stable permission keys that are not tied to UI text.

Canonical shape:

```text
<application>.<resource>.<action>
```

Examples:

- `hr.employee.read`
- `hr.employee.create`
- `hr.employee.export`
- `iam.permission_set_assignment.create`
- `workflow.form_instance.approve`
- `agent.run.create`
- `audit.audit_log.read`

Route policy, service authorization, permission fixtures, OpenAPI behavior, audit event names, and tests must stay aligned.

### 5.3 Decision Inputs

An authorization decision must consider:

- Tenant ID.
- Account ID.
- Requested application/resource/action.
- Target resource ID when present.
- Permission sets assigned directly to account.
- Permission sets assigned through user groups.
- Explicit deny entries.
- Data scope attached to assignment.
- Field policies attached to permission/resource.
- Policy conditions.
- Active assumable role session and session policy.
- Permission boundary.
- OpenFGA relationship check when the permission is relation-scoped.
- Approval evidence for high-risk operations.
- Permission version/cache invalidation.

### 5.4 Decision Rules

- Explicit deny wins over allow.
- Permission boundary can only shrink privileges.
- Session policy can only shrink assumable role privileges.
- Missing `data_scope_id` on scoped writes must fail closed.
- Route-level authorization is required for non-public routes.
- Service-level authorization is required for write paths and high-risk read/export paths.
- Frontend menu/button hiding is never sufficient authorization.
- Permission simulation must not mutate real assignments.
- Authorization explain output must be useful but must not leak cross-tenant data.

### 5.5 Data Scopes

Support and test these scope concepts:

- `self`
- `own`
- `tenant`
- `object`
- `department`
- `department_subtree`
- `direct_reports`
- `assigned_org_units`
- `custom_condition`
- `system`

Employee lists, org options, exports, audit queries, workflow queues, and Agent tool results must apply the effective data scope before returning data.

### 5.6 Field Policies

Support field-level effects:

- `allow`
- `deny`
- `mask`
- `hide`
- `readonly`

Field policies must apply consistently to:

- API list/detail responses.
- Export output.
- Audit-visible summaries.
- Agent tool results.
- Prompt/RAG context.
- Logs and traces where sensitive fields could appear.

Sensitive employee fields include salary, national ID, passport, ARC, tax ID, mobile, address, insurance data, and other personal identifiers.

## 6. High-Risk Operation Requirements

High-risk operations must require approval confirmation or a real approval instance, and must be audited.

P0 high-risk examples:

- Employee import confirm.
- Employee export.
- Single employee delete.
- Batch employee delete.
- Employee lifecycle/status transition.
- Employee invite when it creates or links an account.
- IAM permission-set assignment create/update/delete.
- User group, data scope, field policy, assumable role changes.
- Assumable role assume.
- Authz simulation when it can expose sensitive policy information.
- Agent run/tool call when it can access business or personal data.
- Audit-log reads.

For production, prefer real workflow-backed approval over simple headers. If an MVP uses approval headers, document it explicitly as non-final behavior.

## 7. HR and Workspace Functional Requirements

### 7.1 Employee Management

Support:

- List/search/filter/sort/pagination.
- Detail grouped by profile sections.
- Create aggregate.
- Preview create/update without mutation.
- Patch selected sections.
- Avatar upload/remove when storage is configured.
- Import template.
- CSV/XLSX import preview.
- Import confirm.
- CSV/API export.
- Invite account.
- Quick status update.
- Lifecycle transition.
- Single soft delete.
- Batch soft delete.
- Stats and employee options.

P0 invariants:

- Tenant isolation applies everywhere.
- Employee number and email uniqueness must be tenant-scoped unless product says otherwise.
- Deleted employees are hidden by default.
- Import preview never mutates data.
- Import confirm is idempotent or returns conflict safely.
- XLSX employee import must preserve the 10-column contract, including column J `manager employee ID` / `主管員工ID`.
- Export applies the same filters, data scopes, and field policies as list/detail.
- Delete is soft delete or offboarding; never silently hard-delete HR records.

### 7.2 Workspace Projections

The platform/workspace APIs are frontend-facing projections. They may aggregate HR, IAM, workflow, attendance, and audit data, but they must not become separate sources of truth.

Current important workspace surfaces:

- Overview.
- Employees.
- Organization.
- Attendance.
- Turnover.
- Administrators.
- Form design.
- Audit logs.

Workspace administrator APIs must use `permission_set_assignment` resource/action semantics, not generic permission-set semantics.

## 8. Agent and Knowledge Requirements

Agent access must be secure-by-default.

Required behavior:

- Agent run without tool permission is rejected before business data access.
- Agent cannot call export/delete/admin tools unless explicitly granted and approved.
- Agent effective access is `user permissions ∩ Agent boundary ∩ knowledge permissions ∩ data scope ∩ field policy`.
- Unauthorized fields must not enter tool result, prompt context, RAG context, model output, logs, or audit details.
- Cross-tenant knowledge search must return only tenant-scoped chunks.
- Agent tool calls must emit audit events containing tool name, actor, tenant, run ID, data scope, field policy decision, authz result, and trace ID.

Implementation note:

- Long-running Python Agent runtime can be separate from the Go monolith.
- The Go backend should own tenant/account/authz/audit/projection boundaries for Agent integration.

## 9. Audit and Observability Requirements

Audit events must be durable and queryable by authorized security users only.

Minimum audit events:

- Employee create/update/status/delete/batch delete.
- Import preview/confirm.
- Export.
- Permission changes.
- User group changes.
- Assumable role assume/deny/expire.
- Authz deny.
- Approval required/approved/denied.
- Agent tool call.
- Cross-tenant denied attempt.

Minimum audit fields:

- Tenant ID.
- Actor account ID.
- Action/event name.
- Resource type and ID.
- Decision/result.
- Data scope and field policy summary when relevant.
- Approval marker/session ID when relevant.
- Request ID and trace ID.
- Timestamp.

Do not store secrets, raw tokens, full sensitive personal fields, or hidden prompt content in audit logs.

Observability requirements:

- Structured logs include request ID, trace ID, tenant ID, account ID, route, status, latency, and error code.
- Traces should cover HTTP, service authz, DB, Redis, OpenFGA, and Agent adapter boundaries where enabled.
- Readiness should fail when required production dependencies such as DB, Redis, Keycloak, or OpenFGA are configured but unavailable.

## 10. OpenAPI and Contract Requirements

`docs/openapi.yaml` is the public API contract source.

When changing any API route, request/response shape, error code, route policy, or public behavior:

- Update `docs/openapi.yaml`.
- Update route policy metadata in `internal/domain/authz.go`.
- Update permission fixtures if needed.
- Update service authorization checks.
- Update tests.

If a code change does not require OpenAPI updates, the final implementation report must explicitly say why.

## 11. Implementation Phases for Claude

### Phase 0: Discovery and Gap Map

Claude must first inspect the live repo before editing.

Required reads:

- `README.md`
- `AGENTS.md`
- `docs/openapi.yaml`
- `docs/module-development.md`
- `docs/code-organization.md`
- `docs/people-domain-employee-iam-test-plan.md`
- `internal/domain/authz.go`
- `internal/service/authz_service.go`
- Current route/controller/service/store files for the target change

Deliverable:

- A short gap map separating existing behavior, missing implementation, unclear product decisions, and test-only gaps.

### Phase 1: Permission Contract Closure

Goal:

- Align route policies, IAM resources/actions, permission fixtures, service authorization, OpenAPI, and tests.

Acceptance:

- Every non-public route has route policy metadata.
- Every write/high-risk path has service-level authz.
- High-risk paths require approval evidence.
- `permission_set_assignment` has dedicated resource semantics.
- Authz explain/check/simulate behavior is tested.

### Phase 2: Data Scope and Field Policy Closure

Goal:

- Ensure employee/workspace/audit/export/Agent paths return only authorized data.

Acceptance:

- List/detail/options/export use effective data scope.
- Sensitive fields are masked/hidden consistently.
- Unauthorized fields do not leak into Agent tool results or logs.
- Tests cover self, direct reports, department subtree, tenant, and denied cases.

### Phase 3: Tenant and PostgreSQL RLS Proof

Goal:

- Prove tenant isolation against real PostgreSQL, not only memory store.

Acceptance:

- Migrations apply cleanly.
- sqlc generated code is current.
- Repository tests prove tenant-scoped uniqueness and no cross-tenant reads.
- RLS integration tests prove `app.tenant_id` is set safely on every request/repository path.
- Background jobs/outbox set tenant context before DB access.

### Phase 4: HR Import/Export and Lifecycle Hardening

Goal:

- Make employee high-risk operations production-safe.

Acceptance:

- Import preview handles CSV/XLSX, duplicate-in-file, row limits, row errors, expiry, and no mutation.
- Import confirm is transactional or has explicit all-or-nothing/partial semantics.
- Export enforces approval, data scope, field policy, filters, encoding, and audit.
- Delete/status/invite workflows are high-risk, audited, and tenant-safe.

### Phase 5: Agent Boundary

Goal:

- Wire Agent run/tool authorization without leaking HR/IAM/knowledge data.

Acceptance:

- Agent runs require `agent.run.create`.
- Tool execution checks tool permission and target resource permissions.
- Knowledge access checks tenant, agent binding, and `knowledge_user_permissions`.
- Tool outputs and prompt context apply field policies.
- Agent tool calls are audited.

## 12. Non-Goals

- Do not add public password registration unless product explicitly changes the bound-account/OIDC requirement.
- Do not split the backend into microservices for this PRD.
- Do not introduce a new frontend design system.
- Do not replace `docs/openapi.yaml` with controller-comment generation.
- Do not bypass authz through platform projection endpoints.
- Do not treat OpenFGA as optional for production relation-scoped permissions when `OPENFGA_*` is configured.
- Do not claim production readiness without PostgreSQL, Redis/authz-cache, Keycloak/OIDC, OpenFGA, audit, and RLS failure-mode evidence.

## 13. Verification Strategy

Use layered verification. Start with the smallest relevant checks and expand only when touched boundaries require it.

Recommended commands:

```sh
GOCACHE=$PWD/.gocache go test ./internal/api/v1 ./tests/unit/api/v1
GOCACHE=$PWD/.gocache go test ./internal/service ./tests/unit/service
GOCACHE=$PWD/.gocache go test ./tests/unit/...
make sqlc
make migrate-validate
GOCACHE=$PWD/.gocache go test ./...
make test
```

Run `make sqlc` when `db/queries/*.sql` changes.

Run `make migrate-validate` when migrations change.

For frontend-facing contract work, also verify the relevant `/v1/platform/*`, `/v1/hr/*`, `/v1/workflows/*`, or `/v1/attendance/*` endpoint path through smoke/API tests.

## 14. Definition of Done

A change is done only when:

- Requirement is mapped to API, service behavior, data model, permission point, audit event, and test case.
- Code follows existing modular monolith boundaries.
- OpenAPI and route policies are aligned.
- Tenant identity is token-first and fail-closed.
- Data scope and field policy are applied at service result boundaries.
- High-risk operations require approval and emit audit.
- Unit/API tests cover the changed behavior.
- Schema/query changes have migration/sqlc validation.
- Residual risks and unimplemented product decisions are explicitly listed.

## 15. Open Product Decisions

These must be resolved before production sign-off:

- Whether employee create is a strict HR record or can be a draft with minimal required fields.
- Whether employee company email uniqueness is tenant-scoped or global.
- Exact department filter behavior: exact department only or department subtree.
- Exact delete semantics: soft delete, resignation, account disablement, or all of them.
- Whether high-risk MVP may use approval headers or must always use real workflow approval.
- Whether sales/finance insights remain projections or need dedicated business fact tables.
- Whether Agent conversations live entirely in Python service, Go backend, or split by runtime vs governance.
- Exact Notion Task requirements once the two original Task pages are accessible.

## 16. Claude Working Instructions

When Claude uses this PRD:

- Treat this file as product context, not as permission to ignore repo rules.
- Start by reading the live repo and current diff.
- Do not revert user changes.
- Keep changes scoped to the phase/task being implemented.
- Prefer existing patterns and helpers.
- Update `docs/openapi.yaml` for API contract changes.
- Keep tests under `tests/unit/...` unless package visibility requires otherwise.
- Use `GOCACHE=$PWD/.gocache` for Go tests in this environment.
- Do not push unless explicitly asked.
