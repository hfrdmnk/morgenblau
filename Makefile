# Morgenblau — Go API + Vite/React frontend.

all: build test

build:
	@echo "Building..."
	@go build -o main cmd/api/main.go

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

docker-run:
	@if docker compose up --build 2>/dev/null; then \
		: ; \
	else \
		echo "Falling back to Docker Compose V1"; \
		docker-compose up --build; \
	fi

docker-down:
	@if docker compose down 2>/dev/null; then \
		: ; \
	else \
		echo "Falling back to Docker Compose V1"; \
		docker-compose down; \
	fi

test:
	@echo "Testing..."
	@go test ./... -v

itest:
	@echo "Running integration tests..."
	@go test ./internal/database -v

clean:
	@echo "Cleaning..."
	@rm -f main

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

.PHONY: all build run dev test itest clean watch docker-run docker-down
