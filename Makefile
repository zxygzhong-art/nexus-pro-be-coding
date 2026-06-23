GO ?= go
GOOSE ?= $(GO) run github.com/pressly/goose/v3/cmd/goose@latest
SQLC ?= $(GO) run github.com/sqlc-dev/sqlc/cmd/sqlc@latest
PSQL ?= psql
CREATEDB ?= createdb
CURL ?= curl

DATABASE_URL ?=
OPENFGA_API_URL ?= http://localhost:8081
OPENFGA_STORE_ID ?=
OPENFGA_MODEL_ID ?=
OPENFGA_MODEL_FILE ?= ops/openfga/model.json

.PHONY: dev test unit-test ci-local sqlc require-database-url require-openfga-store require-openfga-model-id db-create migrate-up migrate-down migrate-status migrate-validate openfga-apply-model openfga-check-model

dev:
	$(GO) run ./cmd/api

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

require-database-url:
	@if [ -z "$(strip $(DATABASE_URL))" ]; then \
		echo "DATABASE_URL is required. Example: make migrate-up DATABASE_URL=postgres://nexus:nexus@localhost:5432/nexus_pro_be?sslmode=disable" >&2; \
		exit 1; \
	fi

require-openfga-store:
	@if [ -z "$(strip $(OPENFGA_API_URL))" ]; then \
		echo "OPENFGA_API_URL is required. Example: make openfga-apply-model OPENFGA_API_URL=http://localhost:8081 OPENFGA_STORE_ID=<store-id>" >&2; \
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

db-create: require-database-url
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

migrate-up: require-database-url db-create
	$(GOOSE) -dir db/migrations postgres "$(DATABASE_URL)" up

migrate-down: require-database-url
	$(GOOSE) -dir db/migrations postgres "$(DATABASE_URL)" down

migrate-status: require-database-url
	$(GOOSE) -dir db/migrations postgres "$(DATABASE_URL)" status

migrate-validate:
	$(GOOSE) -dir db/migrations validate

openfga-apply-model: require-openfga-store
	$(CURL) -sS -X POST "$(OPENFGA_API_URL)/stores/$(OPENFGA_STORE_ID)/authorization-models" \
		-H "Content-Type: application/json" \
		--data-binary "@$(OPENFGA_MODEL_FILE)"

openfga-check-model: require-openfga-store require-openfga-model-id
	$(CURL) -sS "$(OPENFGA_API_URL)/stores/$(OPENFGA_STORE_ID)/authorization-models/$(OPENFGA_MODEL_ID)"
