# nexus-pro-be

`nexus-pro-be` is the Go backend for the first-stage multi-tenant HR platform foundation.

The first implementation phase focuses on a modular monolith for HR core data, attendance, workflow forms, audit, AI Agent adapters, and the permission-center foundation. Keycloak OIDC token validation is wired into request handling when configured, and OpenFGA can be enabled as a relationship-check adapter.

## Stack

- Go `1.26.4`
- Gin for HTTP routing and middleware
- PostgreSQL as the primary business database
- `pgxpool` for PostgreSQL connections
- `sqlc` for type-safe SQL-first data access
- Redis via `go-redis/v9` for cache, short-lived state, and future rate limiting
- `slog` for structured logs
- Grafana + Loki + Promtail for local log aggregation and exploration
- OpenTelemetry + Grafana Tempo for distributed tracing

Migration execution uses `goose` as a command-line tool. It is intentionally not part of the application runtime dependency graph.

## Local Services

```sh
docker compose up -d postgres redis loki tempo promtail grafana
```

Copy the sample environment file if you want local defaults:

```sh
cp .env.example .env
```

The application reads environment variables directly. Export them in your shell or load `.env` with your preferred local tool.

Grafana is available at `http://localhost:3001` with the local credentials `admin` / `admin`. The Loki and Tempo data sources are provisioned automatically.

Application logs are structured JSON written to stdout, which keeps the runtime simple and lets Promtail forward container logs into Loki. Request logs include `trace_id`, `request_id`, `tenant_id`, `account_id`, method, path, status, elapsed time, and client IP.

OpenTelemetry tracing is disabled by default. To send traces to the local Tempo service, enable it before starting the API:

```sh
export OTEL_ENABLED=true
export OTEL_SERVICE_NAME=nexus-pro-be
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
export OTEL_EXPORTER_OTLP_INSECURE=true
go run ./cmd/api
```

HTTP requests, Keycloak discovery calls, and OpenFGA relationship checks are instrumented when tracing is enabled. Trace IDs are also written into request logs, so Grafana can jump between Tempo traces and Loki logs.

When running the API directly with `go run`, stream stdout into the local log directory mounted by Promtail:

```sh
mkdir -p logs
go run ./cmd/api 2>&1 | tee logs/nexus-pro-be.log
```

Useful Grafana Loki queries:

```logql
{compose_service="api"} | json
{compose_service="api"} | json | trace_id="req_xxx"
{compose_service="api"} | json | tenant_id="demo"
```

Set `LOG_LEVEL=debug`, `info`, `warn`, or `error` to tune application log verbosity.

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

## Development

Run the API:

```sh
export DATABASE_URL='postgres://nexus:nexus@localhost:5432/nexus_pro_be?sslmode=disable'
go run ./cmd/api
```

Useful endpoints:

```sh
curl http://localhost:8080/healthz
curl http://localhost:8080/v1/me
```

Swagger UI is available at `http://localhost:8080/swagger/index.html`, backed by the embedded OpenAPI spec at `http://localhost:8080/openapi.yaml`.

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

Run the minimal validation suite:

```sh
go test ./...
```

Unit tests live under `tests/unit` and can be run independently:

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
- `tests/unit` contains unit tests outside production packages.

Project code-organization preferences are documented in `docs/code-organization.md`.

## Current Architecture Boundary

The API runtime uses PostgreSQL as the source of truth:

- PostgreSQL-backed repository is required through `DATABASE_URL`.
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

Runtime accounts are database-backed only. Accounts and identity bindings must already be present in PostgreSQL before login.

## Permission Foundation

The permission-center foundation is included without forcing external infrastructure into the first runnable version.

Current scope:

- Authz schema tables for applications, permission catalog, normalized group memberships, permission-set assignments, data scopes, field policies, policy conditions, assumable-role sessions, and relationship tuples for future OpenFGA sync.
- `internal/domain/authz` defines default route policy metadata and high-risk markers used by the service-level authorization path.
- `KEYCLOAK_*` enables Keycloak/OIDC bearer-token validation. In production, `KEYCLOAK_ISSUER_URL` and `KEYCLOAK_CLIENT_ID` are required at startup.
- `OPENFGA_*` enables relationship checks and starts the relationship tuple outbox worker. In production, `OPENFGA_API_URL`, `OPENFGA_STORE_ID`, and `OPENFGA_MODEL_ID` are required at startup because relation-scoped permissions depend on this adapter.
- `ops/openfga/model.json` is the versioned authorization model. Apply it explicitly with `make openfga-apply-model`, then set `OPENFGA_MODEL_ID` to the returned `authorization_model_id`; the API readiness check verifies that model ID.
- Employee authz-subject changes emit local relationship tuples and OpenFGA write/delete outbox events with retryable status tracking.

Not included yet:

- Automatic OpenFGA model migrations and full relationship coverage beyond the current employee owner/manager and agent knowledge article viewer tuples.
- End-to-end PostgreSQL RLS integration tests for every request/repository path.
