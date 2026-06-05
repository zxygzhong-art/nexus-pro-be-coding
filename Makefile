.PHONY: tidy build run test lint migrate-up migrate-down migrate-force migrate-version db-up compose-up compose-down seed

COMPOSE := docker compose -f deploy/docker-compose.yml

tidy:
	go mod tidy

build:
	go build ./...

run:
	go run ./cmd/api

test:
	go test ./...

lint:
	golangci-lint run ./... || echo "golangci-lint not installed; skipping"

# Database migrations (golang-migrate via cmd/migrate). Uses MIGRATE_DSN/DB_DSN.
migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

migrate-force:
	go run ./cmd/migrate force $(V)

migrate-version:
	go run ./cmd/migrate version

# Seed is the last migration (000014); migrate-up applies it.
seed: migrate-up

# Local infra
db-up:
	$(COMPOSE) up -d postgres redis

compose-up:
	$(COMPOSE) up -d

compose-down:
	$(COMPOSE) down
