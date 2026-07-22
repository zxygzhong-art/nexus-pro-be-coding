GO ?= go
GOOSE_VERSION ?= v3.27.2
SQLC_VERSION ?= v1.31.1
GOOSE ?= $(GO) run github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
SQLC ?= $(GO) run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
PSQL ?= psql
CREATEDB ?= createdb
CURL ?= curl

DB_HOST ?=
DB_PORT ?= 5432
DB_USERNAME ?=
DB_PASSWORD ?=
DB_NAME ?=
DB_SSLMODE ?= disable
DATABASE_URL ?= postgres://$(DB_USERNAME):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSLMODE)
# API runtime credentials remain in DB_*; migrations and role provisioning use
# a separate administrator/owner connection. When the new variable is omitted,
# keep the existing DB_*-based workflow working for local development.
MIGRATION_DATABASE_URL ?=
MIGRATION_DATABASE_URL_EFFECTIVE = $(if $(strip $(MIGRATION_DATABASE_URL)),$(MIGRATION_DATABASE_URL),$(DATABASE_URL))
RUNTIME_DB_USERNAME ?= nexus_app
RUNTIME_DB_PASSWORD ?=
MIGRATION_DB_OWNER ?=
MIGRATION_DATABASE_URL_PROVIDED := $(if $(strip $(MIGRATION_DATABASE_URL)),1,0)
DB_CONNECTION_FIELDS_PROVIDED := $(if $(and $(strip $(DB_HOST)),$(strip $(DB_USERNAME)),$(strip $(DB_NAME))),1,0)
RUNTIME_ROLE_FIELDS_PROVIDED := $(if $(and $(strip $(RUNTIME_DB_USERNAME)),$(strip $(RUNTIME_DB_PASSWORD))),1,0)
export MIGRATION_DATABASE_URL_EFFECTIVE RUNTIME_DB_USERNAME RUNTIME_DB_PASSWORD MIGRATION_DB_OWNER PSQL
OPENFGA_BASE_URL ?= http://localhost:8081
OPENFGA_STORE_ID ?=
OPENFGA_MODEL_ID ?=
OPENFGA_MODEL_FILE ?= ops/openfga/model.json

.PHONY: dev test unit-test ci-local sqlc tenant-provision db-provision-runtime-role require-database require-migration-database require-runtime-database-role require-openfga-store require-openfga-model-id db-create migrate-up migrate-down migrate-status migrate-validate openfga-apply-model openfga-check-model

dev:
	$(GO) run ./cmd/api

tenant-provision: require-database
	$(GO) run ./cmd/tenantctl provision --database-url "$(DATABASE_URL)" $(TENANT_PROVISION_FLAGS)

db-provision-runtime-role: require-migration-database require-runtime-database-role
	bash ops/postgres/provision-runtime-role.sh

test:
	$(GO) test ./...

unit-test:
	$(GO) test ./tests/unit/...

ci-local:
	$(GO) vet ./...
	golangci-lint run ./...
	$(GO) test ./tests/unit/...
	$(MAKE) migrate-validate
	$(MAKE) sqlc
	git diff --exit-code

sqlc:
	$(SQLC) generate

require-database:
	@if [ -z "$(strip $(DB_HOST))" ] || [ -z "$(strip $(DB_USERNAME))" ] || [ -z "$(strip $(DB_NAME))" ]; then \
		echo "DB_HOST, DB_USERNAME, and DB_NAME are required. Example: make migrate-up DB_HOST=localhost DB_PORT=5432 DB_USERNAME=nexus DB_PASSWORD=nexus DB_NAME=nexus_pro_be" >&2; \
		exit 1; \
	fi

require-migration-database:
	@if [ "$(MIGRATION_DATABASE_URL_PROVIDED)" != "1" ] && [ "$(DB_CONNECTION_FIELDS_PROVIDED)" != "1" ]; then \
		echo "MIGRATION_DATABASE_URL is required (DB_* remains a backwards-compatible fallback)." >&2; \
		exit 1; \
	fi

require-runtime-database-role:
	@if [ "$(RUNTIME_ROLE_FIELDS_PROVIDED)" != "1" ]; then \
		echo "RUNTIME_DB_USERNAME and RUNTIME_DB_PASSWORD are required. Example: make db-provision-runtime-role MIGRATION_DATABASE_URL='postgres://migration-admin:secret@localhost:5432/nexus_pro_be?sslmode=disable' RUNTIME_DB_PASSWORD='replace-me'" >&2; \
		exit 1; \
	fi

require-openfga-store:
	@if [ -z "$(strip $(OPENFGA_BASE_URL))" ]; then \
		echo "OPENFGA_BASE_URL is required. Example: make openfga-apply-model OPENFGA_BASE_URL=http://localhost:8081 OPENFGA_STORE_ID=<store-id>" >&2; \
		exit 1; \
	fi
	@if [ -z "$(strip $(OPENFGA_STORE_ID))" ]; then \
		echo "OPENFGA_STORE_ID is required. Create a store first, then pass OPENFGA_STORE_ID=<store-id>" >&2; \
		exit 1; \
	fi
	@if [ ! -f "$(OPENFGA_MODEL_FILE)" ]; then \
		echo "OPENFGA_MODEL_FILE does not exist: $(OPENFGA_MODEL_FILE)" >&2; \
		exit 1; \
	fi

require-openfga-model-id:
	@if [ -z "$(strip $(OPENFGA_MODEL_ID))" ]; then \
		echo "OPENFGA_MODEL_ID is required. Run make openfga-apply-model and export the returned authorization_model_id." >&2; \
		exit 1; \
	fi

db-create: require-migration-database
	@set -e; \
	url="$(MIGRATION_DATABASE_URL_EFFECTIVE)"; \
	base="$${url%%\?*}"; \
	query=""; \
	if [ "$$base" != "$$url" ]; then query="$${url#*\?}"; fi; \
	db_name="$${base##*/}"; \
	maintenance_url="$${base%/*}/postgres"; \
	if [ -n "$$query" ]; then maintenance_url="$$maintenance_url?$$query"; fi; \
	case "$$db_name" in \
		""|*[!A-Za-z0-9_]*) echo "invalid database name: $$db_name" >&2; exit 1 ;; \
	esac; \
	if $(PSQL) "$$maintenance_url" -v ON_ERROR_STOP=1 -Atqc "SELECT 1 FROM pg_database WHERE datname = '$$db_name'" | grep -qx 1; then \
		echo "database $$db_name already exists"; \
	else \
		echo "creating database $$db_name"; \
		$(CREATEDB) --maintenance-db="$$maintenance_url" "$$db_name"; \
	fi

migrate-up: require-migration-database db-create
	$(GOOSE) -dir db/migrations postgres "$(MIGRATION_DATABASE_URL_EFFECTIVE)" up

migrate-down: require-migration-database
	$(GOOSE) -dir db/migrations postgres "$(MIGRATION_DATABASE_URL_EFFECTIVE)" down

migrate-status: require-migration-database
	$(GOOSE) -dir db/migrations postgres "$(MIGRATION_DATABASE_URL_EFFECTIVE)" status

migrate-validate:
	$(GOOSE) -dir db/migrations validate

openfga-apply-model: require-openfga-store
	$(CURL) -sS -X POST "$(OPENFGA_BASE_URL)/stores/$(OPENFGA_STORE_ID)/authorization-models" \
		-H "Content-Type: application/json" \
		--data-binary "@$(OPENFGA_MODEL_FILE)"

openfga-check-model: require-openfga-store require-openfga-model-id
	$(CURL) -sS "$(OPENFGA_BASE_URL)/stores/$(OPENFGA_STORE_ID)/authorization-models/$(OPENFGA_MODEL_ID)"
