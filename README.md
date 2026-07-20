# nexus-pro-api

`nexus-pro-api` is the Go backend for the first-stage multi-tenant HR platform foundation.

The first implementation phase focuses on a modular monolith for HR core data, attendance, workflow forms, audit, AI Agent adapters, and the permission-center foundation. Keycloak OIDC token validation is wired into request handling when configured, and OpenFGA can be enabled as a relationship-check adapter.

## Stack

- Go `1.26.4`
- Gin for HTTP routing and middleware
- PostgreSQL as the primary business database
- `pgxpool` for PostgreSQL connections
- `sqlc` for type-safe SQL-first data access
- Redis via `go-redis/v9` for cache, short-lived state, and future rate limiting
- `slog` JSON logs written directly to stdout / console
- OpenTelemetry SDK + Grafana Tempo for distributed tracing

Migration execution uses `goose` as a command-line tool. It is intentionally not part of the application runtime dependency graph.

## Local Services

```sh
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-api/ops
./render-configs.sh
docker compose --env-file .env up -d
```

The ops stack reads `ops/.env`. Copy the application sample environment file from the repository root if you want local API defaults:

```sh
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-api
cp .env.example .env
```

The application reads environment variables directly. Export them in your shell or load `.env` with your preferred local tool.

Grafana is available at `http://localhost:24000` with the local credentials `admin` / `admin`. Prometheus and Tempo data sources are provisioned automatically.

### Containerized API

The repository root contains a multi-stage `Dockerfile` (Go builder, distroless non-root runtime, port 8080). An optional `api` compose service builds it and reads environment from `.env`:

```sh
docker compose --profile api up -d --build api
```

Inside the compose network, point `DB_HOST` / `REDIS_HOST` at the service names (`postgres`, `redis`) instead of `localhost`.

Application logs are structured JSON written directly to stdout / console. Request logs include `trace_id`, `request_id`, `tenant_id`, `account_id`, method, path, status, elapsed time, and client IP.

OpenTelemetry tracing is disabled by default. To send traces directly to local Tempo, enable it before starting the API:

```sh
export OTEL_ENABLED=true
export OTEL_SERVICE_NAME=nexus-pro-api
export OTEL_BASE_URL=localhost:24317
export OTEL_EXPORTER_OTLP_INSECURE=true
export METRICS_ADDR=0.0.0.0:9091
go run ./cmd/api
```

HTTP requests, Keycloak discovery calls, and OpenFGA relationship checks are instrumented when tracing is enabled. Trace IDs are also written into request logs.

When running the API directly with `go run`, read logs from the console:

```sh
go run ./cmd/api
```

Set `LOG_LEVEL=debug`, `info`, `warn`, or `error` to tune application log verbosity.

### Agent Chat Runtime

The default `go run ./cmd/api`, `go test`, and Docker build all compile the real ADK Agent runtime and LiteLLM adapter. No build tag is required. Set `LITELLM_BASE_URL` and `LITELLM_API_KEY` to enable chat at runtime; when the API key is absent, the rest of the API stays available and agent chat returns `agent_chat_disabled`.

Agent model API keys, MCP/external-tool credentials, and other persisted secrets use `ENCRYPTION_KEY`, a standard-base64 encoded 32-byte key (`openssl rand -base64 32`). The API rejects credential-bearing writes when this key is absent or invalid; secrets are encrypted before persistence, bound to their tenant and resource ID, and never returned by JSON responses.

### Temporal Workflow Engine

Temporal is a required runtime dependency for form approval. Form submit starts a Temporal workflow, and approve / reject / return / withdraw API actions only send Temporal signals. The existing `form_instances` and `workflow_runs` tables remain the query projection used by the API, updated by workflow activities. There is no API fallback to the legacy synchronous state machine when a workflow execution is missing.

Start the local Temporal profile:

```sh
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-api/ops
./render-configs.sh
docker compose --profile temporal --env-file .env up -d temporal temporal-ui temporal-admin-tools
```

Then configure Temporal before startup:

```sh
export TEMPORAL_BASE_URL=127.0.0.1:27233
export TEMPORAL_NAMESPACE=default
export TEMPORAL_TASK_QUEUE=nexus-workflows
go run ./cmd/api
```

The API dials Temporal during startup and fails fast if the connection is unavailable. The worker starts in the API process and shuts down with the existing runtime shutdown path.

Backfill in-flight approvals created before Temporal-only rollout:

```sh
go run ./cmd/tenantctl temporal-backfill-form-workflows --tenant-id <tenant-id> --dry-run
go run ./cmd/tenantctl temporal-backfill-form-workflows --tenant-id <tenant-id>
```

Run the dry-run first to inspect candidate form instances. After backfill, retry approval actions that previously returned `workflow_not_found`.

### NATS JetStream Event Bus

NATS JetStream is optional and disabled by default. When enabled, the existing `outbox_events` table remains the source of truth: the outbox dispatcher publishes mapped events to JetStream, then marks the outbox row `succeeded` after the publish ack. The first durable consumer is OpenFGA tuple sync.

Start the local NATS profile:

```sh
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-api/ops
docker compose --profile nats --env-file .env up -d nats
```

Then enable the API integration:

```sh
export NATS_ENABLED=true
export NATS_BASE_URL=nats://127.0.0.1:24222
export NATS_STREAM=NEXUS_EVENTS
export NATS_CONSUMER_PREFIX=nexus
go run ./cmd/api
```

The stream is `NEXUS_EVENTS` with subjects `events.>`. Event subjects use `events.{domain}.{resource}.{action}`; tenant and idempotency data are headers: `Nexus-Tenant-Id`, `Nexus-Event-Id`, and `Nexus-Event-Type`. The OpenFGA durable consumer is `nexus-openfga` and filters `events.iam.relationship.*`.

## Database

Run migrations:

```sh
make migrate-up
```

Generate SQL access code:

```sh
make sqlc
```

Generated files live in `internal/platform/postgres/db`.

## Tenant Provisioning

Run tenant provisioning after the business database migrations. Creating the tables alone intentionally leaves tenant-scoped permission and account tables empty because the migration does not yet know the tenant ID or the first administrator identity.

Provisioning is transactional and creates or updates the following tenant resources:

- the tenant and its root organization unit
- the first active administrator account and employee record
- the Keycloak/OIDC identity binding for the administrator
- the tenant permission and menu catalog
- a `Platform Admin` permission set directly assigned to the administrator
- six default common form templates for leave, overtime, punch correction, job change, headcount, and resignation
- the authorization permission version and relationship/outbox events

Provisioning uses stable generated IDs and is safe to rerun with the same tenant and identity. A successful rerun does not duplicate the tenant, administrator, employee, root organization, permission set, identity binding, or default form templates. It does increment the permission version and append a new `tenant.provisioned` event.

### 1. Load the database environment and migrate

The application and CLI read environment variables directly; they do not load `.env` automatically.

```sh
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-api
test -f .env || cp .env.example .env

set -a
source ./.env
set +a

make migrate-up
make migrate-status
```

At minimum, `DB_HOST`, `DB_PORT`, `DB_USERNAME`, `DB_PASSWORD`, `DB_NAME`, and `DB_SSLMODE` must describe the target business database. In production, use `DB_SSLMODE=require`, `verify-ca`, or `verify-full` as appropriate.

### 2. Choose the administrator identity path

Use one of the following mutually exclusive paths:

1. Bind an existing Keycloak user with `--keycloak-sub`.
2. Let `tenantctl` create or update the Keycloak user with `--provision-keycloak`.

The tenant ID should be stable because it is persisted as `tenant_id` and used by authorization, RLS, identity binding, and token claims. Keycloak should have protocol mappers that expose the user attributes `tenant_id` and `account_id` as token claims; see [ops/docs/keycloak.md](ops/docs/keycloak.md).

#### Path A: bind an existing Keycloak user

Obtain the immutable Keycloak user ID/OIDC `sub` from the Keycloak Admin Console or a verified access token. Do not use the username or email as `--keycloak-sub`.

Ensure the existing Keycloak user belongs to the same tenant and that its token contains the expected `tenant_id` claim, then run:

```sh
export KEYCLOAK_USER_ID='replace-with-keycloak-user-id'

make tenant-provision \
  TENANT_PROVISION_FLAGS="--tenant-id tenant-acme --tenant-name 'Acme Corp' --admin-email admin@acme.example --admin-name 'Acme Admin' --admin-employee-no ADMIN001 --keycloak-sub ${KEYCLOAK_USER_ID}"
```

This path only creates the local binding. It does not modify the existing Keycloak user's attributes, password, or required actions.

#### Path B: create or update the administrator in Keycloak

Configure a Keycloak service-account client with permission to manage users:

```sh
export KEYCLOAK_BASE_URL=https://keycloak.example.com/realms/nexus-pro
export KEYCLOAK_ADMIN_CLIENT_ID=nexus-pro-admin
export KEYCLOAK_ADMIN_CLIENT_SECRET='replace-with-service-account-secret'
```

Then run:

```sh
make tenant-provision \
  TENANT_PROVISION_FLAGS='--tenant-id tenant-acme --tenant-name "Acme Corp" --admin-email admin@acme.example --admin-name "Acme Admin" --admin-employee-no ADMIN001 --provision-keycloak'
```

`--provision-keycloak` finds the Keycloak user by email, validates that it is not owned by another tenant/account, and writes the generated `tenant_id`, `account_id`, `employee_id`, and `employee_no` attributes. The one-time CLI flag works independently of `KEYCLOAK_PROVISION_USERS`; that environment switch controls later employee create/import/invite flows in the running API.

The CLI does not set an initial password. For a newly created user, either set a password in the Keycloak Admin Console or enable the invite flow:

```sh
export KEYCLOAK_SEND_INVITE_EMAIL=true
export KEYCLOAK_INVITE_CLIENT_ID=nexus-pro-connect-api
export KEYCLOAK_INVITE_REDIRECT_URL=https://app.example.com/login

make tenant-provision \
  TENANT_PROVISION_FLAGS='--tenant-id tenant-acme --tenant-name "Acme Corp" --admin-email admin@acme.example --admin-name "Acme Admin" --admin-employee-no ADMIN001 --provision-keycloak --send-invite'
```

`--send-invite` adds the Keycloak `UPDATE_PASSWORD` required action. The email is sent only when `KEYCLOAK_SEND_INVITE_EMAIL=true` and Keycloak SMTP is configured; otherwise set the password or trigger the required-actions email from Keycloak manually.

### Provision command options

| Option | Required | Default / meaning |
| --- | --- | --- |
| `--tenant-id` | Yes | Stable tenant ID used in storage and auth context |
| `--tenant-name` | No | Defaults to `tenant-id` |
| `--admin-email` | Yes | Lowercased and used for the local account and Keycloak lookup |
| `--admin-name` | No | Defaults to the email local part |
| `--admin-employee-no` | No | Defaults to `ADMIN001` |
| `--provider` | No | Defaults to `keycloak` |
| `--keycloak-sub` | Path A | Existing immutable Keycloak user ID/OIDC subject |
| `--provision-keycloak` | Path B | Creates or updates the user through the Keycloak Admin API |
| `--send-invite` | No | Adds `UPDATE_PASSWORD`; combine with invite email configuration to send mail |
| `--database-url` | No | Derived from `DB_*` variables by the Make target |
| `--timeout` | No | Defaults to `30s` |

The command prints a JSON result containing the stable IDs needed for later operations, including `tenant_id`, `root_org_unit_id`, `admin_account_id`, `admin_employee_id`, `admin_permission_set_id`, `identity_subject`, and `permission_version`. Save this output with the deployment record, but do not treat it as a credentials file.

### 3. Start the API and verify the administrator

Start the API with the same database and Keycloak configuration:

```sh
go run ./cmd/api
```

The API startup registers the built-in permission package and synchronizes the permission/menu catalog for existing tenants. Check dependency readiness:

```sh
curl --fail http://127.0.0.1:18080/readyz
```

After obtaining an access token for the administrator, verify the resolved tenant, account, and effective menus:

```sh
export ACCESS_TOKEN='replace-with-administrator-access-token'
curl --fail \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  http://127.0.0.1:18080/v1/me
```

The response should resolve to the provisioned tenant/account and include administrator menus such as `workbench`, `hr.employees`, `iam.permission_sets`, and `audit`.

### 4. Import the optional built-in permission templates

Tenant provisioning currently creates the directly assigned `Platform Admin` permission set and the complete permission catalog. It does not automatically instantiate the built-in package templates for `員工基礎權限`, `HR 管理權限`, and `平臺唯讀排障權限`, or their user groups/data scopes.

After the API is running, list the registered packages and import the built-in package when those reusable templates are needed:

```sh
curl --fail \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  http://127.0.0.1:18080/v1/iam/permission-packages

export PERMISSION_PACKAGE_ID='replace-with-package-id-from-the-list-response'

curl --fail -X POST \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  "http://127.0.0.1:18080/v1/iam/permission-packages/${PERMISSION_PACKAGE_ID}/import"
```

Package import is idempotent for the same tenant, package ID, and version. It creates the package-defined permission sets, user groups, data scopes, and assumable roles without assigning ordinary employees to privileged groups automatically.

### Troubleshooting

- `relation ... does not exist`: run `make migrate-up` against the same `DB_*` target used by `tenantctl`.
- `DB_HOST/DB_USERNAME/DB_NAME ... required`: load `.env` with `set -a; source ./.env; set +a`, or pass the database variables explicitly.
- `keycloak issuer url must include /realms/{realm}`: set `KEYCLOAK_BASE_URL` to the realm issuer URL, not the Keycloak server root.
- `keycloak admin client ... required`: set `KEYCLOAK_ADMIN_CLIENT_ID` and `KEYCLOAK_ADMIN_CLIENT_SECRET` for `--provision-keycloak`.
- `keycloak user is already owned by another tenant/account`: do not rebind that realm-global user; use the correct existing tenant or a different Keycloak user.
- `external identity is not linked to a local account`: confirm that the token `sub` matches `identity_subject` and that its `tenant_id` identifies the provisioned tenant.
- `authenticated tenant/account context is required`: verify the Keycloak protocol mappers and inspect the access token claims before retrying.

## Development

Run the API:

```sh
export DB_HOST=localhost DB_PORT=5432 DB_USERNAME=nexus DB_PASSWORD=nexus DB_NAME=nexus_pro_be DB_SSLMODE=disable
go run ./cmd/api
```

Useful endpoints:

```sh
curl http://127.0.0.1:18080/healthz
curl http://127.0.0.1:18080/v1/me
curl http://127.0.0.1:9091/metrics
```

`/metrics` is a Prometheus endpoint exposing `http_requests_total` and `http_request_duration_seconds` labeled by method, route template, and status. It is served on a dedicated listener configured by `METRICS_ADDR` (default `127.0.0.1:9091`) instead of the business port; set `METRICS_ADDR=` (empty) to disable it. For local Docker-based Prometheus scraping, bind it to an address the Prometheus container can reach, such as `METRICS_ADDR=0.0.0.0:9091`. Request metrics are still collected on the business router.

Connection pool sizing is configurable through `DB_MAX_CONNS` (default `10`), `DB_MIN_CONNS` (default `1`), and `DB_MAX_CONN_LIFETIME` (default `1h`).

In production (`APP_ENV=production`), startup validation requires `DB_SSLMODE` to be `require`, `verify-ca`, or `verify-full`; `disable` or an unspecified mode is rejected.

### CORS

CORS is disabled unless `CORS_ALLOWED_ORIGINS` is set to a comma-separated list of exact origins (for example `https://app.example.com,https://admin.example.com`). Allowed origins receive `Access-Control-Allow-Origin/Methods/Headers/Credentials`, and `OPTIONS` preflight requests are answered with `204`. Origins are matched exactly; no wildcard or prefix matching is performed.

### Rate Limiting

Set `RATE_LIMIT_ENABLED=true` to rate limit requests per client IP. `RATE_LIMIT_RPS` (default `20`) is the sustained request rate and `RATE_LIMIT_BURST` (default `40`) the tolerated burst. When `REDIS_HOST` is configured the limiter uses a shared Redis fixed-window counter so limits hold across replicas; otherwise an in-process token bucket is used. Rejected requests receive `429` with error code `10070`. The limiter fails open if Redis becomes unavailable. Note that the API trusts no proxy headers by default, so behind a reverse proxy set `TRUSTED_PROXIES` to a comma-separated list of proxy CIDRs/IPs (for example `10.0.0.0/8,192.168.1.1`) so client IPs used in logs and rate limiting are derived from `X-Forwarded-For` safely; when unset, the peer address is used.

Swagger UI is available at `http://127.0.0.1:18080/swagger/index.html`, backed by the embedded OpenAPI spec at `http://127.0.0.1:18080/openapi.yaml`.

### Error Code Design

API error envelopes expose a numeric `error.code` for stable client handling. The canonical definitions live in `internal/domain/error_codes.go`, and `docs/openapi.yaml` must be updated whenever the public code set changes.

Prefix allocation:

| Prefix | Owner |
| --- | --- |
| `1xxxx` | Common platform, request parsing, authentication, not-found, conflict, and fallback errors |
| `2xxxx` | IAM and authorization errors |
| `3xxxx` | People-domain and HR errors |
| `4xxxx` | Attendance errors |
| `5xxxx` | Workflow errors |
| `6xxxx` | Agent errors |

Within a prefix, keep low numbers for generic fallbacks and reserve narrower ranges for more specific cases. Do not reuse a retired code with a different meaning. The top-level `error.code` is numeric; `reason_code`, `field_errors[].code`, and `row_errors[].field_errors[].code` remain semantic strings for diagnostics and UI copy.

Expected business failures must be returned as `domain.AppError`. Generic bad-request, not-found, and conflict errors are assigned to the owning Attendance (`4xxxx`), Workflow (`5xxxx`), or Agent (`6xxxx`) range at the API route boundary; important recovery cases should additionally set a semantic `reason_code` and a dedicated numeric code. Raw repository, dependency, and panic errors remain `10000` with a safe message and `trace_id`; never expose database or upstream response details to clients.

Run the minimal validation suite:

```sh
go test ./...
```

All Go test files live outside production packages: unit tests mirror the business package structure under `tests/unit`, while database-backed suites live under `tests/integration`. Unit tests can be run independently:

```sh
go test ./tests/unit/...
make unit-test
```

The same checks are available as:

```sh
make test
make migrate-validate
```

## Project Layout

The codebase is organized by responsibility so new modules can be added without mixing transport, business logic, and persistence:

- `cmd/api` starts the HTTP server and wires dependencies.
- `internal/api/v1` contains Gin routes, handlers, middleware, request parsing, and response rendering.
- `internal/service` contains application services and business orchestration.
- `internal/domain` contains shared domain models, request/response types, and application errors.
- `internal/domain/authz` contains route policy metadata used by the service authorization runtime.
- `internal/repository` contains repository interfaces.
- `internal/repository/memory` contains the in-memory repository implementation for tests.
- `internal/jobs` is reserved for scheduled and background task entrypoints.
- `internal/platform` contains infrastructure clients such as PostgreSQL and Redis.
- `tests/unit` mirrors production package paths for unit tests; do not place `_test.go` files under `internal` or `cmd`.
- `tests/integration` contains database-backed and cross-boundary integration tests.

Project code-organization preferences are documented in `docs/code-organization.md`.

## Current Architecture Boundary

The API runtime uses PostgreSQL as the source of truth:

- PostgreSQL-backed repository is required through `DB_HOST` / `DB_USERNAME` / `DB_NAME`.
- In-memory repository remains available only for focused tests.

The project has the production persistence foundation in place:

- schema migrations in `db/migrations`
- sqlc queries in `db/queries`
- generated data-access code in `internal/platform/postgres/db`
- PostgreSQL pool setup in `internal/platform/postgres`
- PostgreSQL repository implementation in `internal/repository/postgres`
- Redis client setup in `internal/platform/redis`
- environment config in `internal/config`
- permission route metadata in `internal/domain/authz`

Runtime accounts keep business profile and authorization state in PostgreSQL, while login credentials live in Keycloak. When `KEYCLOAK_PROVISION_USERS=true`, employee creation/import/invite flows create or update the Keycloak user through the Admin API and bind its `sub` into `user_identities`.

## Permission Foundation

The permission-center foundation is included without forcing external infrastructure into the first runnable version.

Current scope:

- Authz schema tables for applications, permission catalog, normalized group memberships, permission-set assignments, data scopes, field policies, policy conditions, assumable-role sessions, and relationship tuples for future OpenFGA sync.
- `internal/domain/authz` defines default route policy metadata and high-risk markers used by the service-level authorization path.
- `KEYCLOAK_*` enables Keycloak/OIDC bearer-token validation. In production, `KEYCLOAK_BASE_URL` and `KEYCLOAK_CLIENT_ID` are required at startup; enabling `KEYCLOAK_PROVISION_USERS` additionally requires `KEYCLOAK_ADMIN_CLIENT_ID` and `KEYCLOAK_ADMIN_CLIENT_SECRET`.
- `OPENFGA_*` enables relationship checks and starts the relationship tuple outbox worker. In production, `OPENFGA_BASE_URL`, `OPENFGA_STORE_ID`, and `OPENFGA_MODEL_ID` are required at startup because relation-scoped permissions depend on this adapter.
- `OPENFGA_SCOPE_CHECK_ENABLED=true` switches department and department-subtree data-scope filtering to OpenFGA checks after the model and tuple backfill are ready. The default is `false`, which keeps the existing SQL scope path.
- `ops/openfga/model.json` is the versioned authorization model. Apply it explicitly with `make openfga-apply-model`, then set `OPENFGA_MODEL_ID` to the returned `authorization_model_id`; the API readiness check verifies that model ID.
- Employee authz-subject changes emit local relationship tuples and OpenFGA write/delete outbox events with retryable status tracking.

Not included yet:

- Automatic OpenFGA model migrations and full relationship coverage beyond the current employee owner/manager and agent knowledge article viewer tuples.
- End-to-end PostgreSQL RLS integration tests for every request/repository path.
