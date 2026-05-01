# Implementation Tasks: KARIZ Command Dashboard

## Task 1: Project Scaffolding and Database Setup

- [x] 1.1 Initialize Go module (`go mod init`) with Gin, sqlx, golang-migrate, Docker client, robfig/cron, and minio-go dependencies
- [x] 1.2 Create project directory structure: `cmd/`, `internal/` (auth, catalog, executor, stream, notification, validator, docker, models, api, envvar, artifact, scheduler), `migrations/`, `web/`
- [x] 1.3 Create application config struct that reads from environment variables (DB connection, Docker socket path, auth settings, app port, session secret, session TTL)
- [x] 1.4 Create PostgreSQL migration files for all tables: users, user_roles, command_entries, command_roles, execution_records, execution_artifacts, sessions, notifications, schedules (matching the ER diagram in design.md)
- [x] 1.5 Create database connection setup with sqlx and migration runner that auto-migrates on startup
- [x] 1.6 Create Dockerfile: multi-stage build (Go build → minimal runtime image with React static files, PostgreSQL client libs)
- [x] 1.7 Create docker-compose.yml for local development (KARIZ app + PostgreSQL + Docker socket mount)

## Task 2: Data Models and Repository Layer

- [x] 2.1 Define Go structs for all domain models: `CommandEntry`, `ParameterSchema`, `ParameterDefinition`, `EnvVarMapping`, `ValidationRules`, `ResourceLimits`, `VolumeMount`, `ArtifactDeclare`, `ArtifactDestConfig`, `ExecutionRecord`, `ExecutionArtifact`, `User`, `Session`, `Notification`, `Schedule`, `ExecutionMode` (matching design.md Data Models)
- [x] 2.2 Implement `CommandRepository` with CRUD operations: Create, GetByID, GetByName, Update, Deactivate, List (with role filtering, pagination, search)
- [x] 2.3 Implement `ExecutionRepository` with operations: Create, Update, GetByID, List (with filtering by command name, user, date range, status; pagination; descending order)
- [x] 2.4 Implement `UserRepository` with operations: Create, GetByID, GetByUsername, UpdateRoles, List
- [x] 2.5 Implement `SessionRepository` with operations: Create, GetByToken, Delete, DeleteByUserID, CleanExpired
- [x] 2.6 Implement `NotificationRepository` with operations: Create, GetByUserID (with unread filter), MarkAsRead
- [x] 2.7 Implement `ExecutionArtifactRepository` with operations: Create, GetByExecutionID, GetByID, Delete
- [x] 2.8 Implement `ScheduleRepository` with operations: Create, Update, Delete, GetByID, List (with role filtering, pagination), GetDueSchedules (where next_run_at <= now and is_enabled = true), UpdateNextRunAt

## Task 3: Authentication and Session Management

- [x] 3.1 Implement `LocalAuthProvider` that authenticates against the users table with bcrypt password hashing
- [x] 3.2 Implement `SessionStore` using `SessionRepository` — create session with secure random token, validate on lookup, delete on logout
- [x] 3.3 Implement Gin auth middleware: extract session token from httpOnly cookie, validate session, attach `SessionData` to Gin context, return 401 if invalid/expired
- [x] 3.4 Implement `requireRole` middleware that checks the session's roles against required roles, returns 403 if no intersection
- [x] 3.5 Implement auth API handlers: POST `/api/auth/login`, POST `/api/auth/logout`, GET `/api/auth/me`
- [x] 3.6 Implement session cleanup background goroutine that periodically removes expired sessions

## Task 4: Command Catalog Service

- [x] 4.1 Implement `CatalogService.CreateCommand` — validate required fields (non-empty name, non-empty command string, at least one role), check name uniqueness, store with generated UUID and version=1
- [x] 4.2 Implement `CatalogService.UpdateCommand` — validate input, increment version, persist changes
- [x] 4.3 Implement `CatalogService.DeactivateCommand` — set `is_active=false`
- [x] 4.4 Implement `CatalogService.ListCommands` — filter by user roles (only active commands with overlapping roles), support pagination
- [x] 4.5 Implement `CatalogService.SearchCommands` — case-insensitive substring match on name or description, filtered by user roles
- [x] 4.6 Implement catalog API handlers: GET `/api/commands`, GET `/api/commands/:id`, POST `/api/commands`, PUT `/api/commands/:id`, DELETE `/api/commands/:id`

## Task 5: Parameter Validation and Safe Substitution

- [x] 5.1 Implement `ParameterValidator.Validate` — check required fields present, type validation (string/number/boolean/enum), constraint validation (pattern, min/max, enum values, max length), unknown parameter detection
- [x] 5.2 Implement `ParameterValidator.BuildCommandArgs` — parse command string template, substitute parameters as isolated array elements (not shell interpolation), prevent shell injection by never passing through a shell
- [x] 5.3 Write property-based tests for Property 8 (parameter validation rejects invalid inputs) and Property 14 (shell injection prevention)

## Task 6: Docker Manager

- [x] 6.1 Implement `DockerManager` using `github.com/docker/docker/client` — connect via Docker socket path from config
- [x] 6.2 Implement `DockerManager.CreateContainer` — translate `ContainerConfig` to Docker API create request with image, command args, volume mounts (never mounting Docker socket), resource limits (CPU, memory)
- [x] 6.3 Implement `DockerManager.StartContainer` and `DockerManager.AttachStream` — start container and return a channel of `OutputChunk` from stdout/stderr using Docker attach API
- [x] 6.4 Implement `DockerManager.StopContainer` and `DockerManager.RemoveContainer` — graceful stop with timeout, force remove
- [x] 6.5 Implement `DockerManager.IsAvailable` — ping Docker daemon, return descriptive error if unreachable
- [x] 6.6 Implement `DockerManager.ExecInContainer` — create exec instance in a running container using Docker exec API, return exec ID
- [x] 6.7 Implement `DockerManager.AttachExecStream` — attach to exec instance stdout/stderr, return channel of `OutputChunk`
- [x] 6.8 Implement `DockerManager.InspectExec` — inspect exec instance to get exit code and running status
- [x] 6.9 Implement `DockerManager.InspectContainerEnv` — use Docker ContainerInspect API to read environment variables from a running container, return as map[string]string
- [x] 6.10 Implement `DockerManager.CopyFromContainer` — use Docker CopyFromContainer API to copy a file from a container, return io.ReadCloser of tar archive content
- [x] 6.11 Write property-based test for Property 13 (container config correctness — volumes and resource limits)

## Task 7: Executor Service

- [x] 7.1 Implement concurrency lock mechanism — database-backed lock using execution_records status; check for running executions of same command_id before starting; respect `allow_concurrent` flag
- [x] 7.2 Implement `ExecutorService.ExecuteCommand` — validate parameters, resolve env var mappings, acquire lock, create execution record (queued), branch on execution mode: for "create" mode launch goroutine for Docker lifecycle (create → start → attach → stream → copy artifacts → cleanup); for "exec" mode launch goroutine for Docker exec lifecycle (exec → attach → stream → capture)
- [x] 7.3 Implement timeout enforcement — use `context.WithTimeout` based on command's `timeout_seconds`; on deadline exceeded, stop container (create mode) or cancel exec context (exec mode), record `timed_out` status
- [x] 7.4 Implement output capture — accumulate stdout/stderr from Docker stream, store in execution record on completion, publish chunks to StreamManager
- [x] 7.5 Implement `ExecutorService.CancelExecution` — stop running container, update status to `cancelled`
- [x] 7.6 Implement execution API handlers: POST `/api/commands/:id/execute`, GET `/api/executions`, GET `/api/executions/:id`, POST `/api/executions/:id/cancel`
- [x] 7.7 Write property-based test for Property 9 (concurrency lock enforcement)

## Task 8: SSE Stream Manager

- [x] 8.1 Implement `StreamManager` with in-memory event buffer per execution — store events with sequential IDs, support multiple subscribers per execution
- [x] 8.2 Implement `StreamManager.Subscribe` — return channel of SSE events; if `lastEventID` provided, replay from that point; otherwise replay all buffered events; then stream new events
- [x] 8.3 Implement `StreamManager.Publish` — assign sequential event ID, buffer the event, fan out to all active subscribers
- [x] 8.4 Implement `StreamManager.Complete` — send completion event to all subscribers, mark execution as done
- [x] 8.5 Implement SSE HTTP handler at GET `/api/executions/:id/stream` — set correct headers (`Content-Type: text/event-stream`, `Cache-Control: no-cache`), read `Last-Event-ID` header, pipe StreamManager channel to response writer
- [x] 8.6 Implement `StreamManager.Cleanup` — remove event buffer and subscriber list for completed executions after a configurable retention period
- [x] 8.7 Write property-based test for Property 15 (SSE stream replay and resume)

## Task 9: Notification Service

- [x] 9.1 Implement `NotificationService.NotifyExecutionComplete` — create in-app notification with type matching terminal status (completed/failed/timed_out), title, message, and execution ID reference
- [x] 9.2 Implement optional email notification — if SMTP is configured, send email with command name, execution status, and link to execution record
- [x] 9.3 Implement notification API handlers: GET `/api/notifications`, PUT `/api/notifications/:id/read`
- [x] 9.4 Write property-based test for Property 16 (notification creation on terminal status)

## Task 10: Admin User Management

- [x] 10.1 Implement admin API handlers: GET `/api/admin/users` (list all users), PUT `/api/admin/users/:id/roles` (assign/remove roles)
- [x] 10.2 Implement role update logic — update user_roles table, invalidate existing sessions for the user so new permissions take effect on next request (Req 5.4)
- [x] 10.3 Write property-based test for Property 12 (role-based execution denial)

## Task 11: Health Check Endpoint

- [x] 11.1 Implement GET `/api/health` — check database connectivity (ping), Docker daemon availability (IsAvailable), return JSON with status of each subsystem and overall status
- [x] 11.2 Configure Docker HEALTHCHECK instruction in Dockerfile pointing to `/api/health`

## Task 12: Frontend — React Application Setup

- [x] 12.1 Initialize React + TypeScript project with Vite in `web/` directory; install Ant Design, TanStack Query, React Router
- [x] 12.2 Set up API client module with fetch wrapper, session cookie handling, and error interceptor (401 → redirect to login)
- [x] 12.3 Set up TanStack Query provider and React Router with route definitions for: login, command list, command detail, execution form, execution detail/stream, execution history, notifications, admin panel

## Task 13: Frontend — Authentication Pages

- [x] 13.1 Build login page with username/password form, error display, redirect to command list on success
- [x] 13.2 Implement auth context provider — store current user profile and roles, provide `useAuth` hook
- [x] 13.3 Implement protected route wrapper that redirects to login if not authenticated

## Task 14: Frontend — Command Catalog UI

- [x] 14.1 Build command list page — fetch commands from GET `/api/commands`, display as cards/table grouped by category, show name, description, and allowed roles
- [x] 14.2 Build search/filter bar — text input that filters commands by name/description via API query parameter
- [x] 14.3 Build command detail view — display full command info including parameter schema, allowed roles, timeout, concurrency setting
- [x] 14.4 Build admin command registration form — fields for name, description, category, Docker image, command string, parameter schema builder, role selection, resource limits, volumes, timeout, concurrency flag

## Task 15: Frontend — Command Execution UI

- [x] 15.1 Build dynamic execution form — render input fields from Parameter_Schema (text input for string, number input for number, checkbox for boolean, select for enum), pre-populate defaults, client-side validation
- [x] 15.2 Build execution detail/stream view — display execution metadata (command, user, status, times), connect to SSE endpoint for real-time output, render stdout/stderr in a terminal-style panel
- [x] 15.3 Implement SSE client using EventSource API — handle `Last-Event-ID` for reconnection, display accumulated output on connect, append new output chunks
- [x] 15.4 Build execution status indicator component — show queued/running/completed/failed/timed_out with appropriate colors and icons

## Task 16: Frontend — Execution History and Notifications

- [x] 16.1 Build execution history page — paginated table of execution records, columns: command name, user, status, start time, duration
- [x] 16.2 Build history filter controls — filter by command name, user, date range, status
- [x] 16.3 Build notification dropdown/panel — show unread count badge, list notifications, mark as read on click, link to execution detail

## Task 17: Frontend — Admin Panel

- [x] 17.1 Build user management page — list users with their roles, provide role assignment/removal UI
- [x] 17.2 Build command management page — list all commands (including inactive), provide activate/deactivate toggle, edit button linking to registration form

## Task 18: Remaining Property-Based Tests

- [x] 18.1 Write property-based tests for Properties 1–5 (command catalog: round-trip, duplicate rejection, version increment, deactivation, validation)
- [x] 18.2 Write property-based tests for Properties 6–7 (role-based filtering, search correctness)
- [x] 18.3 Write property-based tests for Properties 10–11 (execution history pagination/ordering, filter correctness)

## Task 19: Integration and Smoke Tests

- [x] 19.1 Write integration tests for full command execution lifecycle (create command → execute → stream output → verify completion record)
- [x] 19.2 Write integration tests for Docker container lifecycle with real Docker daemon (create → start → attach → stop → remove)
- [x] 19.3 Write integration tests for SSE streaming end-to-end (execute command, connect SSE, verify output events)
- [x] 19.4 Write integration test for timeout enforcement (execute slow command, verify timed_out status and container cleanup)
- [x] 19.5 Write smoke tests for Docker image build, environment variable configuration, and auto-migration on startup

## Task 20: Deployment and Documentation

- [x] 20.1 Finalize Dockerfile with multi-stage build: stage 1 (Node.js — build React), stage 2 (Go — build binary), stage 3 (minimal runtime with binary + static files + migration files)
- [x] 20.2 Write README.md with setup instructions, environment variable reference, Docker run command, and development guide
- [x] 20.3 Implement graceful shutdown handler (SIGTERM/SIGINT) — stop accepting connections, wait for in-flight requests, cancel running executions, close DB
- [x] 20.4 Add structured logging (JSON format) throughout the application using Go's `slog` package

## Task 21: Environment Variable Resolver

- [x] 21.1 Implement `EnvVarResolver.ResolveParams` — iterate over parameter schema, for each parameter with an `EnvVarMapping` and no user-provided value, call `DockerManager.InspectContainerEnv` to read the env var from the source container
- [x] 21.2 Implement user-provided value precedence — if the user provides an explicit value for a parameter that also has an `EnvVarMapping`, use the user-provided value and skip the mapping
- [x] 21.3 Implement error handling — return `EnvVarResolutionError` when source container is not running or env var is not found on the container
- [x] 21.4 Integrate `EnvVarResolver` into `ExecutorService.ExecuteCommand` — call resolver before parameter validation and command arg building
- [x] 21.5 Write property-based test for Property 17 (env var resolution precedence)

## Task 22: Artifact Store

- [x] 22.1 Implement `LocalArtifactStore` — store artifacts on local filesystem under a configurable directory, organized by execution ID; serve downloads via Gin static file handler
- [x] 22.2 Implement `S3ArtifactStore` using `minio-go` — upload artifacts to S3-compatible storage (MinIO or AWS S3); generate presigned download URLs with configurable expiry
- [x] 22.3 Implement artifact factory — select `LocalArtifactStore` or `S3ArtifactStore` based on the command's `ArtifactDestConfig.Type`
- [x] 22.4 Implement artifact copy from container — after command execution completes, iterate over declared artifacts, call `DockerManager.CopyFromContainer` for each, extract file from tar stream, pass to `ArtifactStore.Store`
- [x] 22.5 Implement partial failure handling — if a declared artifact file does not exist in the container, create an `ExecutionArtifact` record with status `"failed"` and error message, continue processing remaining artifacts
- [x] 22.6 Implement artifact API handlers: GET `/api/executions/:id/artifacts` (list artifacts), GET `/api/executions/:id/artifacts/:artifactId/download` (download artifact — proxy local file or redirect to presigned URL)
- [x] 22.7 Write property-based tests for Property 18 (artifact storage round-trip) and Property 21 (partial artifact failure)

## Task 23: Docker Exec Mode Support

- [x] 23.1 Implement exec mode execution path in `ExecutorService` — when `CommandEntry.ExecutionMode == "exec"`, resolve target container, call `DockerManager.ExecInContainer`, attach stream, capture output
- [x] 23.2 Implement target container validation — before exec, verify the target container is running via Docker inspect; return descriptive error if not found or not running
- [x] 23.3 Implement exec timeout enforcement — use `context.WithTimeout` for exec mode; on deadline exceeded, the exec process is terminated by context cancellation
- [x] 23.4 Implement artifact copy for exec mode — after exec completes, copy declared artifacts from the target container (do NOT remove the target container)
- [x] 23.5 Update command registration form (frontend) — add execution mode selector ("create" / "exec"), conditionally show target container name field when "exec" is selected, hide Docker image field when "exec" is selected
- [x] 23.6 Write property-based test for Property 19 (execution mode routing correctness)

## Task 24: Scheduler Service

- [x] 24.1 Implement `SchedulerService.CreateSchedule` — validate cron expression using `robfig/cron` parser, compute initial `next_run_at`, store schedule in database
- [x] 24.2 Implement `SchedulerService.UpdateSchedule` — update cron expression or interval, recompute `next_run_at`
- [x] 24.3 Implement `SchedulerService.EnableSchedule` and `DisableSchedule` — toggle `is_enabled` flag, recompute `next_run_at` on enable
- [x] 24.4 Implement `SchedulerService.DeleteSchedule` — remove schedule from database
- [x] 24.5 Implement scheduler background loop — start a goroutine that runs every 60 seconds, queries `ScheduleRepository.GetDueSchedules`, triggers `ExecutorService.ExecuteCommand` for each due schedule with the schedule creator's user ID, updates `last_run_at` and computes next `next_run_at`
- [x] 24.6 Implement scheduled execution failure notification — when a scheduled execution fails, send notification to the schedule creator with failure details and link to execution record
- [x] 24.7 Implement schedule API handlers: GET `/api/schedules`, GET `/api/schedules/:id`, POST `/api/schedules`, PUT `/api/schedules/:id`, DELETE `/api/schedules/:id`, POST `/api/schedules/:id/enable`, POST `/api/schedules/:id/disable`
- [x] 24.8 Integrate scheduler startup and shutdown into application lifecycle — start scheduler after DB migration, stop scheduler on graceful shutdown
- [x] 24.9 Write property-based test for Property 20 (schedule triggers execution at correct time)

## Task 25: Frontend — Environment Variable Mapping UI

- [x] 25.1 Update admin command registration form — add env var mapping section to each parameter definition: optional source container name and env var name fields
- [x] 25.2 Display env var mapping info on command detail view — show which parameters are mapped to container env vars
- [x] 25.3 Update execution form — show indicator for parameters with env var mappings, allow user to override with explicit value

## Task 26: Frontend — Artifact UI

- [x] 26.1 Update execution detail view — display list of artifacts with label, filename, size, and status; add download button for each stored artifact
- [x] 26.2 Update admin command registration form — add artifact declaration section: list of container paths and labels, artifact destination configuration (type selector, bucket/path/endpoint fields)
- [x] 26.3 Implement artifact download — call download endpoint, handle presigned URL redirects for S3/MinIO artifacts

## Task 27: Frontend — Schedule Management UI

- [x] 27.1 Build schedule list page — display schedules with command name, cron expression, next run time, last run time, enabled/disabled status; filter by command
- [x] 27.2 Build create schedule form — command selector, cron expression input with human-readable preview (e.g., "Every day at 3:00 AM"), optional interval input, parameter values
- [x] 27.3 Build schedule detail view — show schedule info, execution history filtered by schedule_id, enable/disable toggle, edit and delete buttons
- [x] 27.4 Add schedule navigation — add "Schedules" link to main navigation, add "Schedule" button on command detail page

## Task 28: New Property-Based Tests and Integration Tests

- [x] 28.1 Write integration test for env var resolution end-to-end (start a container with known env vars → register command with mapping → execute → verify resolved values in command)
- [x] 28.2 Write integration test for Docker exec mode (start a container → register exec-mode command → execute → verify output captured without container removal)
- [x] 28.3 Write integration test for artifact lifecycle (execute command that produces files → verify artifacts copied and stored → download and verify content)
- [x] 28.4 Write integration test for schedule trigger (create schedule with short interval → wait → verify execution record created with schedule_id)
- [x] 28.5 Write property-based tests for Properties 17-21 (env var precedence, artifact round-trip, exec mode routing, schedule timing, partial artifact failure) — consolidation task if not already covered by individual task PBT subtasks
