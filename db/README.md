# PostgreSQL persistence

This project uses PostgreSQL with `pgxpool` and `sqlc`.

## Run migrations

```sh
make migrate-up
```

Integration tests read PostgreSQL configuration directly from the `DATABASE_URL`
environment variable:

```sh
DATABASE_URL=postgres://nexus:nexus@localhost:5432/nexus_pro_be?sslmode=disable go test ./tests/integration/postgres
```

## Generate query code

```sh
make sqlc
```

Generated code is written to `internal/platform/postgres/db`.
