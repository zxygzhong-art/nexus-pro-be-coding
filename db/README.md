# PostgreSQL persistence

This project uses PostgreSQL with `pgxpool` and `sqlc`.

## Run migrations

```sh
make migrate-up
```

Integration tests read PostgreSQL configuration from discrete `DB_*` fields
environment variable:

```sh
DB_HOST=localhost DB_PORT=5432 DB_USERNAME=nexus DB_PASSWORD=nexus DB_NAME=nexus_pro_be DB_SSLMODE=disable go test ./tests/integration/postgres
```

## Generate query code

```sh
make sqlc
```

Generated code is written to `internal/platform/postgres/db`.
