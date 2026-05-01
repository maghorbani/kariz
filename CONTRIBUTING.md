# Contributing to KARIZ

Thanks for your interest in contributing! Here's how to get started.

## Development Setup

### Prerequisites

- Go 1.22+
- Node.js 20+
- pnpm
- Docker & Docker Compose
- PostgreSQL (or use Docker Compose)

### Getting Started

```bash
# Clone the repo
git clone https://github.com/YOUR_USERNAME/kariz.git
cd kariz

# Start PostgreSQL
docker compose up postgres -d

# Run database migrations + backend
export SESSION_SECRET=dev-secret
export DATABASE_URL=postgres://kariz:kariz@localhost:5432/kariz?sslmode=disable
go run ./cmd/kariz

# In another terminal — start the frontend dev server
cd web
pnpm install
pnpm dev
```

The frontend dev server runs on `http://localhost:3000` and proxies API requests to the Go backend on port 8080.

### Create a test user

```bash
docker compose exec kariz /kariz createsuperuser admin admin123 admin@kariz.local
```

## Project Structure

```
cmd/kariz/          # Application entrypoint + CLI commands
internal/           # Go backend packages
  api/              # Admin + health handlers
  auth/             # Authentication, sessions, middleware
  catalog/          # Command catalog service + repository
  docker/           # Docker manager (create/exec modes)
  envvar/           # Environment variable resolver
  executor/         # Command execution service
  artifact/         # Artifact storage (local + S3)
  notification/     # Notification service
  scheduler/        # Cron scheduler service
  stream/           # SSE stream manager
  validator/        # Parameter validation
  models/           # Domain types
  config/           # App configuration
migrations/         # PostgreSQL migration files
web/                # React frontend (Vite + TypeScript + Ant Design)
tests/
  property/         # Property-based tests (rapid)
  integration/      # Integration tests
  smoke/            # Smoke tests
```

## Running Tests

```bash
# All unit tests
go test ./internal/...

# Property-based tests
go test ./tests/property/...

# Integration tests (needs Docker)
go test -tags integration ./tests/integration/...

# Frontend type check + build
cd web && pnpm build
```

## Pull Request Process

1. Fork the repo and create a branch from `main`
2. Make your changes
3. Add tests for new functionality
4. Ensure all tests pass: `go test ./...`
5. Ensure the frontend builds: `cd web && pnpm build`
6. Update documentation if needed
7. Open a PR with a clear description

## Code Style

- **Go**: Follow standard Go conventions. Run `gofmt` and `go vet`.
- **TypeScript**: Follow the existing patterns. Ant Design components preferred.
- **Commits**: Use [conventional commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, etc.)

## Reporting Issues

Use GitHub Issues. Include:
- What you expected to happen
- What actually happened
- Steps to reproduce
- KARIZ version / Docker image tag
- Relevant logs (`docker compose logs kariz`)

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
