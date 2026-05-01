# KARIZ Command Dashboard

Self-service command execution platform for QA, Data, and Operations teams to run pre-approved production commands on sibling Docker containers.

## Quick Start

```bash
# Start with Docker Compose (includes PostgreSQL)
curl -O https://raw.githubusercontent.com/maghorbani/kariz/main/docker-compose.yml
SESSION_SECRET=change-me docker compose up -d

# Create an admin user
docker compose exec kariz /kariz createsuperuser admin admin123 admin@kariz.local
```

Open http://localhost:8080 and log in.

## Standalone

```bash
docker run -d \
  --name kariz \
  -p 8080:8080 \
  -e DATABASE_URL=postgres://user:pass@db:5432/kariz?sslmode=disable \
  -e SESSION_SECRET=change-me \
  -v /var/run/docker.sock:/var/run/docker.sock \
  maghorbani759/kariz:latest
```

## Features

- **Command Catalog** — register approved commands with parameter schemas and role-based access
- **Real-Time Streaming** — live command output via SSE
- **Two Execution Modes** — create ephemeral containers or exec into running ones
- **Env Var Mapping** — resolve parameters from sibling container environment variables
- **Artifact Download** — collect and download file outputs (local or S3/MinIO)
- **Scheduling** — automate commands with cron expressions
- **RBAC** — admin, QA, data, and operations roles

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `SESSION_SECRET` | **Yes** | — | Session signing key |
| `DATABASE_URL` | No | `postgres://kariz:kariz@localhost:5432/kariz?sslmode=disable` | PostgreSQL connection |
| `DOCKER_SOCKET_PATH` | No | `/var/run/docker.sock` | Docker socket path |
| `APP_PORT` | No | `8080` | HTTP port |
| `SESSION_TTL` | No | `24h` | Session time-to-live |
| `ARTIFACT_STORE_PATH` | No | `/data/artifacts` | Local artifact storage path |
| `APP_URL` | No | `http://localhost:8080` | Base URL for notification links |
| `SMTP_HOST` | No | — | SMTP server for email notifications |
| `SMTP_PORT` | No | `587` | SMTP port |
| `SMTP_FROM` | No | — | Sender email address |

## Supported Architectures

| Architecture | Tag |
|---|---|
| linux/amd64 | `maghorbani759/kariz:latest` |
| linux/arm64 | `maghorbani759/kariz:latest` |

## Links

- **GitHub**: [github.com/maghorbani/kariz](https://github.com/maghorbani/kariz)
- **Issues**: [github.com/maghorbani/kariz/issues](https://github.com/maghorbani/kariz/issues)
- **Full Documentation**: [README](https://github.com/maghorbani/kariz#readme)
- **License**: MIT
