.PHONY: help build run dev lint test test-unit test-property test-integration test-smoke \
       test-all frontend frontend-dev docker docker-up docker-down migrate superuser clean

# Default
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------

build: ## Build the Go binary
	CGO_ENABLED=0 go build -o bin/kariz ./cmd/kariz

run: build ## Build and run the server
	./bin/kariz

dev: ## Run the server with go run (no build step)
	go run ./cmd/kariz

# ---------------------------------------------------------------------------
# Lint
# ---------------------------------------------------------------------------

lint: ## Run golangci-lint
	golangci-lint run

lint-fix: ## Run golangci-lint with auto-fix
	golangci-lint run --fix

lint-install: ## Install golangci-lint v2 from source (requires Go 1.25)
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6

# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

test: test-unit ## Alias for test-unit

test-unit: ## Run unit tests
	go test -race ./internal/...

test-property: ## Run property-based tests
	go test -race -count=1 ./tests/property/...

test-integration: ## Run integration tests (needs Docker + PostgreSQL)
	go test -race ./tests/integration/...

test-smoke: ## Run smoke tests
	go test -race ./tests/smoke/...

test-all: test-unit test-property test-integration test-smoke ## Run all tests

test-cover: ## Run unit tests with coverage report
	go test -race -coverprofile=coverage.out -covermode=atomic ./internal/...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# ---------------------------------------------------------------------------
# Frontend
# ---------------------------------------------------------------------------

frontend: ## Build the frontend for production
	cd web && pnpm install --frozen-lockfile && pnpm run build

frontend-dev: ## Start the frontend dev server
	cd web && pnpm install && pnpm dev

frontend-install: ## Install frontend dependencies
	cd web && pnpm install

# ---------------------------------------------------------------------------
# Docker
# ---------------------------------------------------------------------------

docker: ## Build the Docker image
	docker build -t kariz:local .

docker-up: ## Start all services with Docker Compose
	docker compose up --build -d

docker-down: ## Stop all services
	docker compose down

docker-logs: ## Follow container logs
	docker compose logs -f kariz

# ---------------------------------------------------------------------------
# Database
# ---------------------------------------------------------------------------

migrate: ## Run database migrations
	go run ./cmd/kariz migrate 2>/dev/null || go run ./cmd/kariz

# ---------------------------------------------------------------------------
# Admin
# ---------------------------------------------------------------------------

superuser: ## Create a superuser (usage: make superuser USER=admin PASS=admin123 EMAIL=admin@kariz.local)
	docker compose exec kariz /kariz createsuperuser $(or $(USER),admin) $(or $(PASS),admin123) $(or $(EMAIL),admin@kariz.local)

superuser-local: ## Create a superuser locally (usage: make superuser-local USER=admin PASS=admin123 EMAIL=admin@kariz.local)
	go run ./cmd/kariz createsuperuser $(or $(USER),admin) $(or $(PASS),admin123) $(or $(EMAIL),admin@kariz.local)

# ---------------------------------------------------------------------------
# Clean
# ---------------------------------------------------------------------------

clean: ## Remove build artifacts
	rm -rf bin/ coverage.out coverage.html web/dist
