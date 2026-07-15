# API Smoke Tests

`full_api_smoke.py` runs the public HTTP API smoke suite and a multi-role authorization matrix.

The smoke suite starts `go run ./cmd/api` against the configured PostgreSQL database and uses Keycloak password-grant login for three DB-backed roles. `DB_*` must point at a database that already contains the test accounts, and every request is sent with a real Bearer token:

```bash
export SMOKE_KEYCLOAK_BASE_URL="http://localhost:18080/realms/nexus-pro"
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

The script verifies those claims before calling the API. Coverage is reported in separate categories:

- **Behavioral coverage** contains hand-written success, validation, persistence, and response-shape checks.
- **Generated auth-boundary coverage** sends no credentials or request body. It verifies that every remaining documented `/v1` route is registered and rejects unauthenticated requests before a handler can mutate state.
- **Combined coverage** is the union of those two categories; it does not claim that every route has full business-behavior coverage.

Validate the OpenAPI inventory and generated plan without PostgreSQL, Keycloak, or a running API:

```bash
python3 tools/api-smoke/full_api_smoke.py --check-coverage
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests/unit/tools/api-smoke -p 'test_*.py'
```
