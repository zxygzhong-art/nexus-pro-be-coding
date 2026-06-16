# Code Organization Preferences

This project prefers business-domain boundaries over large catch-all files.

## Domain Models

Keep domain models split by product area:

- `internal/domain/tenant.go`: tenant-level objects.
- `internal/domain/account.go`: account and identity objects.
- `internal/domain/iam.go`: permission-center objects and authorization request/response types.
- `internal/domain/hr.go`: organization, position, employee, and HR core objects.
- `internal/domain/attendance.go`: leave, attendance, and balance objects.
- `internal/domain/workflow.go`: form, approval, and workflow objects.
- `internal/domain/agent.go`: knowledge, Agent Run, and AI-facing objects.
- `internal/domain/audit.go`: audit and governance objects.
- `internal/domain/inputs.go`: small request DTOs while they are still compact.
- `internal/domain/responses.go`: shared response DTOs.

When a file grows large, split it again by a narrower business concept instead of creating a generic bucket. For example, split `hr.go` into `employee.go`, `org_unit.go`, and `position.go` when those models become substantial.

## Service And Repository Code

Prefer the same business boundary in service, repository, and test code:

- HR core logic should stay near employee and organization behavior.
- Attendance logic should not be mixed into generic HR helpers once it has its own rules.
- Permission-center logic should stay separate from business modules, even when business APIs call it.
- Agent runtime, tool authorization, and RAG logic should be separated from normal CRUD flows.

Use shared helpers only when they are genuinely cross-cutting. Avoid a broad `utils.go` for business behavior.

## Tests

Tests should follow the behavior being protected:

- Domain or service behavior tests belong near the relevant module under `tests/unit`.
- Security-sensitive behavior, such as tenant isolation, data scope filtering, and high-risk confirmation, should get focused tests when changed.
- Prefer small targeted tests first; expand to broader integration tests only when the change crosses repository, API, and service boundaries.
