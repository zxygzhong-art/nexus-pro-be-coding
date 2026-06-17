# API Smoke Tests

`full_api_smoke.py` runs the public HTTP API smoke suite and a multi-role authorization matrix.

Default local mode starts `go run ./cmd/api` with the in-memory demo store and header-based identity:

```bash
tools/api-smoke/full_api_smoke.py
```

Real Keycloak login E2E mode uses password-grant login for three backend seed roles and then calls the API with Bearer tokens:

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

tools/api-smoke/full_api_smoke.py --auth-mode keycloak
```

Each Keycloak user must emit access-token claims matching the backend demo seed data:

| Role | tenant_id | account_id |
| --- | --- | --- |
| admin | demo | acct-admin |
| employee | demo | acct-employee |
| audit | demo | acct-audit |

The script verifies those claims before calling the API. It then covers every OpenAPI route plus role expectations for HR, IAM, audit, attendance, and agent endpoints.
