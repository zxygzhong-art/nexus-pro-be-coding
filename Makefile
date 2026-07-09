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
OPENFGA_BASE_URL ?= http://localhost:8081
OPENFGA_STORE_ID ?=
OPENFGA_MODEL_ID ?=
OPENFGA_MODEL_FILE ?= ops/openfga/model.json

.PHONY: dev dev-adk test unit-test ci-local sqlc tenant-provision require-database require-openfga-store require-openfga-model-id db-create migrate-up migrate-down migrate-status migrate-validate openfga-apply-model openfga-check-model

dev:
	$(GO) run ./cmd/api

# Agent chat 真實 ADK + LiteLLM runtime（需先 go get google.golang.org/adk/v2 相關依賴）。
dev-adk:
	$(GO) run -tags adk ./cmd/api

tenant-provision: require-database
	$(GO) run ./cmd/tenantctl provision --database-url "$(DATABASE_URL)" $(TENANT_PROVISION_FLAGS)

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

db-create: require-database
	@set -e; \
	url="$(DATABASE_URL)"; \
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

migrate-up: require-database db-create
	$(GOOSE) -dir db/migrations postgres "$(DATABASE_URL)" up

migrate-down: require-database
	$(GOOSE) -dir db/migrations postgres "$(DATABASE_URL)" down

migrate-status: require-database
	$(GOOSE) -dir db/migrations postgres "$(DATABASE_URL)" status

migrate-validate:
	$(GOOSE) -dir db/migrations validate

openfga-apply-model: require-openfga-store
	$(CURL) -sS -X POST "$(OPENFGA_BASE_URL)/stores/$(OPENFGA_STORE_ID)/authorization-models" \
		-H "Content-Type: application/json" \
		--data-binary "@$(OPENFGA_MODEL_FILE)"

openfga-check-model: require-openfga-store require-openfga-model-id
	$(CURL) -sS "$(OPENFGA_BASE_URL)/stores/$(OPENFGA_STORE_ID)/authorization-models/$(OPENFGA_MODEL_ID)"
