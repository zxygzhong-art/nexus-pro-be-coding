# API Smoke Tests

`full_api_smoke.py` runs the public HTTP API smoke suite and a multi-role authorization matrix.

The smoke suite starts `go run ./cmd/api` against the configured PostgreSQL database and uses Keycloak password-grant login for three DB-backed roles. `DATABASE_URL` must point at a database that already contains the test accounts, and every request is sent with a real Bearer token:

```bash
export SMOKE_KEYCLOAK_ISSUER_URL="http://localhost:18080/realms/nexus-pro"
export SMOKE_KEYCLOAK_CLIENT_ID="nexus-pro-connect-api"

export SMOKE_ADMIN_USERNAME="local-admin"
export SMOKE_ADMIN_PASSWORD="..."
export SMOKE_ADMIN_ACCOUNT_ID="acct-admin"

export SMOKE_EMPLOYEE_USERNAME="local-employee"
export SMOKE_EMPLOYEE_PASSWORD="..."
export SMOKE_EMPLOYEE_ACCOUNT_ID="acct-employee"

export SMOKE_AUDIT_USERNAME="local-audit"
export SMOKE_AUDIT_PASSWORD="..."
export SMOKE_AUDIT_ACCOUNT_ID="acct-audit"

tools/api-smoke/full_api_smoke.py
```

Each Keycloak user must emit access-token claims matching rows in the backend `user_identities` / `accounts` tables:

| Role | tenant_id | account_id |
| --- | --- | --- |
| admin | demo | acct-admin |
| employee | demo | acct-employee |
| audit | demo | acct-audit |

The script verifies those claims before calling the API. It then covers every OpenAPI route plus role expectations for HR, IAM, audit, attendance, and agent endpoints.
