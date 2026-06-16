# PostgreSQL persistence

This project uses PostgreSQL with `pgxpool` and `sqlc`.

## Run migrations

```sh
make migrate-up
```

## Generate query code

```sh
make sqlc
```

Generated code is written to `internal/platform/postgres/db`.
