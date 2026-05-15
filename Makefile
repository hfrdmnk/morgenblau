# Morgenblau — Go API + Vite/React frontend.

all: build test

build: frontend-build
	@echo "Building Go binary..."
	@go build -o main cmd/api/main.go

# Cross-compile a self-contained Linux binary for deploys.
# Override arch with: make build-linux GOARCH=arm64
GOARCH ?= amd64
build-linux: frontend-build
	@echo "Building Go binary for linux/$(GOARCH)..."
	@CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) go build -ldflags="-s -w" -o main-linux-$(GOARCH) cmd/api/main.go

frontend-build:
	@echo "Building frontend..."
	@bun install --cwd ./frontend --frozen-lockfile
	@bun run --cwd ./frontend build

# Run the Go server and Vite frontend together (no tunnel). Prefer `make dev`.
run:
	@go run cmd/api/main.go &
	@bun install --cwd ./frontend
	@bun run --cwd ./frontend dev

# One-command dev: Go (air) + Vite + cloudflared tunnel via mprocs.
dev:
	@if ! command -v mprocs > /dev/null; then \
		echo "mprocs is required. Install with: brew install mprocs"; \
		exit 1; \
	fi
	@if ! command -v air > /dev/null; then \
		echo "air is required. Install with: go install github.com/air-verse/air@latest"; \
		exit 1; \
	fi
	@if ! command -v cloudflared > /dev/null; then \
		echo "cloudflared is required. Install with: brew install cloudflared"; \
		exit 1; \
	fi
	@mprocs

test:
	@echo "Testing..."
	@go test ./... -v

itest:
	@echo "Running integration tests..."
	@go test ./internal/database -v

clean:
	@echo "Cleaning..."
	@rm -f main main-linux-*

# --- Database ---
# Goose for migrations, sqlc for type-safe queries. Both source DB_* from .env.

GOOSE_DIR := internal/database/migrations
GOOSE_RUN := set -a && . ./.env && set +a && goose -dir $(GOOSE_DIR) postgres "postgres://$$DB_USERNAME:$$DB_PASSWORD@$$DB_HOST:$$DB_PORT/$$DB_DATABASE?sslmode=disable"

migrate-up:
	@$(GOOSE_RUN) up

migrate-down:
	@$(GOOSE_RUN) down

migrate-status:
	@$(GOOSE_RUN) status

# Usage: make migrate-create NAME=add_users_table
migrate-create:
	@if [ -z "$(NAME)" ]; then echo "NAME is required: make migrate-create NAME=add_users_table"; exit 1; fi
	@goose -dir $(GOOSE_DIR) create $(NAME) sql

sqlc:
	@sqlc generate

# Live-reload Go only.
watch:
	@if command -v air > /dev/null; then \
		air; \
	else \
		read -p "Go's 'air' is not installed. Install? [Y/n] " choice; \
		if [ "$$choice" != "n" ] && [ "$$choice" != "N" ]; then \
			go install github.com/air-verse/air@latest; \
			air; \
		else \
			echo "Exiting..."; \
			exit 1; \
		fi; \
	fi

.PHONY: all build build-linux frontend-build run dev test itest clean watch migrate-up migrate-down migrate-status migrate-create sqlc
