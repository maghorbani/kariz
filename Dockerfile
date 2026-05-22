# =============================================================================
# KARIZ Command Dashboard — Multi-stage Dockerfile
# =============================================================================
# Stage 1: Build React frontend
# Stage 2: Build Go backend binary
# Stage 3: Minimal runtime image
# =============================================================================

# ---------------------------------------------------------------------------
# Stage 1: Frontend build
# ---------------------------------------------------------------------------
FROM node:22-alpine AS frontend

WORKDIR /app/web

# Install dependencies first for better layer caching.
COPY web/package.json web/pnpm-lock.yaml ./
RUN corepack enable && corepack install && pnpm install --frozen-lockfile

# Copy frontend source and build the production bundle.
COPY web/ ./
RUN pnpm run build

# Output: /app/web/dist/

# ---------------------------------------------------------------------------
# Stage 2: Go build
# ---------------------------------------------------------------------------
FROM golang:1.25-alpine AS backend

# git is required for go mod download with VCS-based modules.
RUN apk add --no-cache git

WORKDIR /app

# Download Go dependencies first for better layer caching.
COPY go.mod go.sum ./
RUN go mod download

# Copy all Go source code and migration files.
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY migrations/ ./migrations/

# Build a statically-linked binary.
RUN CGO_ENABLED=0 GOOS=linux go build -o /kariz ./cmd/kariz

# ---------------------------------------------------------------------------
# Stage 3: Minimal runtime image
# ---------------------------------------------------------------------------
FROM alpine:3.20

# ca-certificates: required for HTTPS calls (S3/MinIO, external services).
RUN apk add --no-cache ca-certificates

# Copy the compiled Go binary.
COPY --from=backend /kariz /kariz

# Copy React static build output — served by Gin at runtime.
COPY --from=frontend /app/web/dist/ /app/web/dist/

# Copy migration files as a fallback reference.
# (Migrations are embedded in the binary via Go's embed directive,
#  but we keep them on disk for debugging and manual use.)
COPY --from=backend /app/migrations/ /app/migrations/

EXPOSE 8080

# Health check against the /api/health endpoint.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1

ENTRYPOINT ["/kariz"]
