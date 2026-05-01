# KARIZ Command Dashboard

[![CI](https://github.com/YOUR_USERNAME/kariz/actions/workflows/ci.yml/badge.svg)](https://github.com/YOUR_USERNAME/kariz/actions/workflows/ci.yml)
[![Release](https://github.com/YOUR_USERNAME/kariz/actions/workflows/release.yml/badge.svg)](https://github.com/YOUR_USERNAME/kariz/actions/workflows/release.yml)
[![Docker Image](https://img.shields.io/docker/v/YOUR_USERNAME/kariz?sort=semver&label=Docker%20Hub)](https://hub.docker.com/r/YOUR_USERNAME/kariz)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/YOUR_USERNAME/kariz)](https://goreportcard.com/report/github.com/YOUR_USERNAME/kariz)

KARIZ is a self-service command execution platform that provides a web-based dashboard for QA, Data, and Operations teams to run pre-approved production commands on sibling Docker containers. It follows a **Docker-out-of-Docker** pattern — communicating with the host Docker daemon through a mounted socket to create, execute, and clean up ephemeral containers.

## Key Features

- **Command Catalog** — Register and manage approved commands with parameter schemas, role-based access, and resource limits
- **Real-Time Streaming** — Monitor command output live via Server-Sent Events (SSE) with automatic reconnection
- **Role-Based Access Control** — Session-based auth with admin, QA, data, and operations roles
- **Docker Execution** — Run commands in ephemeral containers ("create" mode) or inside running containers ("exec" mode)
- **Environment Variable Mapping** — Resolve parameters from sibling container environment variables at execution time
- **Artifact Storage** — Collect file outputs from containers and store them locally or in S3/MinIO
- **Scheduling** — Automate recurring commands with cron expressions or fixed intervals
- **Notifications** — In-app and optional email notifications on execution completion

---

## Quick Start

The fastest way to run KARIZ is with Docker Compose:

```bash
git clone <repository-url>
cd kariz

# Set a session secret (required)
export SESSION_SECRET=your-secret-key-here

# Start KARIZ + PostgreSQL
docker compose up --build
```

The dashboard is available at [http://localhost:8080](http://localhost:8080).

To stop:

```bash
docker compose down       # Stop containers
docker compose down -v    # Stop containers and remove volumes
```

---

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | No | `postgres://kariz:kariz@localhost:5432/kariz?sslmode=disable` | PostgreSQL connection string |
| `DOCKER_SOCKET_PATH` | No | `/var/run/docker.sock` | Path to the Docker daemon socket |
| `APP_PORT` | No | `8080` | HTTP server listen port |
| `SESSION_SECRET` | **Yes** | — | Secret key for session token signing. Must be set. |
| `SESSION_TTL` | No | `24h` | Session time-to-live (Go duration format, e.g. `12h`, `30m`) |
| `SMTP_HOST` | No | `""` | SMTP server hostname for email notifications |
| `SMTP_PORT` | No | `587` | SMTP server port |
| `SMTP_FROM` | No | `""` | Sender email address for notifications |
| `ARTIFACT_STORE_PATH` | No | `/data/artifacts` | Local filesystem path for artifact storage |
| `APP_URL` | No | `http://localhost:8080` | Base URL used in notification links |

---

## Docker Run

Pull from Docker Hub:

```bash
docker pull YOUR_USERNAME/kariz:latest
```

Or from GitHub Container Registry:

```bash
docker pull ghcr.io/YOUR_USERNAME/kariz:latest
```

Run with an external PostgreSQL instance:

```bash
docker run -d \
  --name kariz \
  -p 8080:8080 \
  -e DATABASE_URL=postgres://user:pass@db-host:5432/kariz?sslmode=disable \
  -e SESSION_SECRET=your-secret-key-here \
  -e APP_URL=http://localhost:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v kariz_artifacts:/data/artifacts \
  YOUR_USERNAME/kariz:latest
```

The Docker socket mount (`-v /var/run/docker.sock:/var/run/docker.sock`) is required for KARIZ to manage sibling containers on the host.

---

## Development Guide

### Prerequisites

- Go 1.22+
- Node.js 20+
- pnpm
- Docker
- PostgreSQL

### Backend

```bash
# Start the Go backend (reads config from environment variables)
export SESSION_SECRET=dev-secret
export DATABASE_URL=postgres://kariz:kariz@localhost:5432/kariz?sslmode=disable

go run ./cmd/kariz
```

### Frontend

```bash
cd web
pnpm install
pnpm dev
```

The Vite dev server proxies API requests to the Go backend on port 8080.

### Tests

```bash
# Unit tests
go test ./...

# Integration tests (requires Docker and PostgreSQL)
go test -tags integration ./tests/integration/...

# Smoke tests (Docker build, config validation)
go test -tags smoke ./tests/smoke/...
```

---

## Architecture Overview

KARIZ is a **monolithic application** packaged as a single Docker image:

- **Backend** — Go with Gin framework. Serves the REST API, manages Docker container lifecycle, handles SSE streaming, and runs background tasks (session cleanup, scheduler).
- **Frontend** — React + TypeScript SPA built with Vite. Bundled as static files and served by the Go backend.
- **Database** — PostgreSQL for persistent storage (users, commands, executions, sessions, notifications, schedules). Migrations run automatically on startup via `golang-migrate`.
- **Docker-out-of-Docker** — The KARIZ container mounts the host's Docker socket. Sibling containers are created on the host daemon, not nested inside KARIZ. The Docker socket is never mounted into sibling containers.

```
┌─────────────────────────────────────────────┐
│              KARIZ Container                │
│                                             │
│  React SPA ──► Gin REST API                 │
│                  │                          │
│          ┌───────┼───────┐                  │
│          │       │       │                  │
│       Catalog  Executor  Auth               │
│          │       │       │                  │
│          └───────┼───────┘                  │
│                  │                          │
│              PostgreSQL ◄── sqlx            │
│                  │                          │
│          Docker Socket Mount                │
└──────────────────┼──────────────────────────┘
                   │
           Host Docker Daemon
                   │
        ┌──────────┼──────────┐
        │          │          │
   Container 1  Container 2  Container N
```

---

## API Endpoints

### Authentication

| Method | Path | Description | Auth |
|---|---|---|---|
| `POST` | `/api/auth/login` | Authenticate user | Public |
| `POST` | `/api/auth/logout` | End session | Authenticated |
| `GET` | `/api/auth/me` | Get current user profile | Authenticated |

### Command Catalog

| Method | Path | Description | Auth |
|---|---|---|---|
| `GET` | `/api/commands` | List commands (filtered by role) | Authenticated |
| `GET` | `/api/commands/:id` | Get command details | Authenticated |
| `POST` | `/api/commands` | Register new command | Admin |
| `PUT` | `/api/commands/:id` | Update command | Admin |
| `DELETE` | `/api/commands/:id` | Deactivate command | Admin |

### Execution

| Method | Path | Description | Auth |
|---|---|---|---|
| `POST` | `/api/commands/:id/execute` | Execute a command | Role-based |
| `GET` | `/api/executions` | List execution history | Authenticated |
| `GET` | `/api/executions/:id` | Get execution details | Authenticated |
| `GET` | `/api/executions/:id/stream` | SSE stream for execution output | Authenticated |
| `POST` | `/api/executions/:id/cancel` | Cancel running execution | Authenticated |

### Artifacts

| Method | Path | Description | Auth |
|---|---|---|---|
| `GET` | `/api/executions/:id/artifacts` | List artifacts for an execution | Authenticated |
| `GET` | `/api/executions/:id/artifacts/:artifactId/download` | Download an artifact file | Authenticated |

### Notifications

| Method | Path | Description | Auth |
|---|---|---|---|
| `GET` | `/api/notifications` | Get user notifications | Authenticated |
| `PUT` | `/api/notifications/:id/read` | Mark notification as read | Authenticated |

### Schedules

| Method | Path | Description | Auth |
|---|---|---|---|
| `GET` | `/api/schedules` | List schedules (filtered by role) | Authenticated |
| `GET` | `/api/schedules/:id` | Get schedule details | Authenticated |
| `POST` | `/api/schedules` | Create a new schedule | Role-based |
| `PUT` | `/api/schedules/:id` | Update a schedule | Role-based |
| `DELETE` | `/api/schedules/:id` | Delete a schedule | Role-based |
| `POST` | `/api/schedules/:id/enable` | Enable a schedule | Role-based |
| `POST` | `/api/schedules/:id/disable` | Disable a schedule | Role-based |

### Admin

| Method | Path | Description | Auth |
|---|---|---|---|
| `GET` | `/api/admin/users` | List all users | Admin |
| `PUT` | `/api/admin/users/:id/roles` | Update user roles | Admin |

### System

| Method | Path | Description | Auth |
|---|---|---|---|
| `GET` | `/api/health` | Health check (database + Docker) | Public |
