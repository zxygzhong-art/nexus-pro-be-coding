GO ?= go
GOOSE ?= $(GO) run github.com/pressly/goose/v3/cmd/goose@latest
SQLC ?= $(GO) run github.com/sqlc-dev/sqlc/cmd/sqlc@latest
PSQL ?= psql
CREATEDB ?= createdb

DATABASE_URL ?= postgres://postgres:12345678@192.168.100.100:5432/nexus_pro?sslmode=disable

.PHONY: dev test unit-test sqlc db-create migrate-up migrate-down migrate-status migrate-validate

dev:
	$(GO) run ./cmd/api

test:
	$(GO) test ./...

unit-test:
	$(GO) test ./tests/unit/...

sqlc:
	$(SQLC) generate

db-create:
	@set -e; \
	url="$(DATABASE_URL)"; \
	base="$${url%%\?*}"; \
	query=""; \
	if [ "$$base" != "$$url" ]; then query="$${url#*\?}"; fi; \
	db_name="$${base##*/}"; \
	maintenance_url="$${base%/*}/postgres"; \
	if [ -n "$$query" ]; then maintenance_url="$$maintenance_url?$$query"; fi; \
	case "$$db_name" in \
		""|*[!A-Za-z0-9_]*) echo "invalid database name parsed from DATABASE_URL: $$db_name" >&2; exit 1 ;; \
	esac; \
	if $(PSQL) "$$maintenance_url" -v ON_ERROR_STOP=1 -Atqc "SELECT 1 FROM pg_database WHERE datname = '$$db_name'" | grep -qx 1; then \
		echo "database $$db_name already exists"; \
	else \
		echo "creating database $$db_name"; \
		$(CREATEDB) --maintenance-db="$$maintenance_url" "$$db_name"; \
	fi

migrate-up: db-create
	$(GOOSE) -dir db/migrations postgres "$(DATABASE_URL)" up

migrate-down:
	$(GOOSE) -dir db/migrations postgres "$(DATABASE_URL)" down

migrate-status:
	$(GOOSE) -dir db/migrations postgres "$(DATABASE_URL)" status

migrate-validate:
	$(GOOSE) -dir db/migrations validate
