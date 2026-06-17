GO ?= go
GOOSE ?= $(GO) run github.com/pressly/goose/v3/cmd/goose@latest
SQLC ?= $(GO) run github.com/sqlc-dev/sqlc/cmd/sqlc@latest

DATABASE_URL ?= postgres://postgres:12345678@192.168.100.100:5432/nexus_pro?sslmode=disable

.PHONY: dev test unit-test sqlc migrate-up migrate-down migrate-status migrate-validate

dev:
	$(GO) run ./cmd/api

test:
	$(GO) test ./...

unit-test:
	$(GO) test ./tests/unit/...

sqlc:
	$(SQLC) generate

migrate-up:
	$(GOOSE) -dir db/migrations postgres "$(DATABASE_URL)" up

migrate-down:
	$(GOOSE) -dir db/migrations postgres "$(DATABASE_URL)" down

migrate-status:
	$(GOOSE) -dir db/migrations postgres "$(DATABASE_URL)" status

migrate-validate:
	$(GOOSE) -dir db/migrations validate
