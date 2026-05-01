# Requirements Document

## Introduction

KARIZ is a self-service command execution platform that empowers QA, Data, and Operations teams to run pre-approved production commands without depending on DevOps. The system provides a web-based dashboard where teams can browse a catalog of registered commands, execute them on production environments via sibling Docker containers (Docker-out-of-Docker pattern), and monitor execution results in real time. The goal is to reduce DevOps bottlenecks, improve team autonomy, and maintain security through controlled command registration and role-based access.

## Glossary

- **Dashboard**: The web-based user interface that allows authorized users to browse, execute, and monitor registered commands.
- **Command_Catalog**: The registry of all approved commands, including their metadata, parameters, and execution constraints.
- **Command_Entry**: A single registered command in the Command_Catalog, consisting of a name, description, Docker target, script or command string, allowed parameters, and access roles.
- **Executor**: The backend service responsible for launching and managing command execution on sibling Docker containers.
- **Sibling_Container**: A Docker container running alongside the KARIZ container on the same Docker host, targeted for command execution using the Docker-out-of-Docker pattern.
- **Execution_Record**: A persistent log entry capturing the details and outcome of a single command execution, including who ran it, when, with what parameters, and the output.
- **User**: An authenticated person who interacts with the Dashboard to browse or execute commands.
- **Admin**: A User with elevated privileges who can register, modify, and deactivate commands in the Command_Catalog.
- **Role**: A named permission group (e.g., QA, Data, Operations, Admin) that determines which commands a User can view and execute.
- **Parameter_Schema**: The definition of allowed input parameters for a Command_Entry, including types, constraints, and default values.
- **Docker_Socket**: The Unix socket (`/var/run/docker.sock`) mounted into the KARIZ container to enable communication with the Docker daemon on the host, enabling the Docker-out-of-Docker pattern.
- **Environment_Variable_Mapping**: A configuration on a Command_Entry parameter that binds the parameter's value to an environment variable read from a running Sibling_Container at execution time.
- **Artifact**: A file produced by a command execution (e.g., a `.sql` dump, a `.csv` export) that is stored and made available for download after execution completes.
- **Artifact_Destination**: A configured storage backend for Artifacts, such as local filesystem storage within the KARIZ container, or external object storage (MinIO, S3).
- **Execution_Mode**: The method used to run a command — either "create" (launch a new ephemeral Sibling_Container) or "exec" (run inside an already-running Sibling_Container via Docker exec API).
- **Schedule**: A recurring or one-time trigger configuration that automatically executes a Command_Entry at specified times using a cron expression or fixed interval.

## Requirements

### Requirement 1: Command Registration

**User Story:** As an Admin, I want to register new commands in the catalog, so that teams can discover and execute approved commands without DevOps involvement.

#### Acceptance Criteria

1. WHEN an Admin submits a new command definition, THE Command_Catalog SHALL store the Command_Entry with a unique identifier, name, description, target Sibling_Container image, command string, Parameter_Schema, and allowed Roles.
2. WHEN an Admin submits a command definition with a name that already exists in the Command_Catalog, THE Command_Catalog SHALL reject the submission and return a descriptive duplicate-name error.
3. WHEN an Admin updates an existing Command_Entry, THE Command_Catalog SHALL persist the changes and increment a version number for the Command_Entry.
4. WHEN an Admin deactivates a Command_Entry, THE Command_Catalog SHALL mark the Command_Entry as inactive and THE Dashboard SHALL stop displaying the Command_Entry to non-Admin Users.
5. THE Command_Catalog SHALL validate that every Command_Entry includes a non-empty name, a non-empty command string, and at least one allowed Role before accepting the registration.

### Requirement 2: Command Catalog Browsing

**User Story:** As a User, I want to browse the catalog of available commands, so that I can find and understand the commands relevant to my team.

#### Acceptance Criteria

1. WHEN a User opens the Dashboard, THE Dashboard SHALL display a list of all active Command_Entries that the User's Role is authorized to view.
2. WHEN a User selects a Command_Entry from the list, THE Dashboard SHALL display the command name, description, Parameter_Schema, and allowed Roles for that Command_Entry.
3. WHEN a User applies a search filter, THE Dashboard SHALL filter the displayed Command_Entries by name or description matching the search term.
4. THE Dashboard SHALL group or tag Command_Entries by category (e.g., "DB Dumps", "Data Extraction", "Reconciliation") to aid discoverability.

### Requirement 3: Command Execution

**User Story:** As a User, I want to execute a registered command from the Dashboard, so that I can perform production operations without waiting for DevOps.

#### Acceptance Criteria

1. WHEN a User submits a command execution request with valid parameters, THE Executor SHALL launch the specified command on the target Sibling_Container using the Docker_Socket.
2. WHEN a User submits a command execution request with parameters that violate the Parameter_Schema, THE Dashboard SHALL reject the request and display a validation error describing each invalid parameter.
3. WHILE a command is executing, THE Dashboard SHALL display a real-time status indicator showing the current state (queued, running, completed, failed).
4. WHEN a command execution completes, THE Executor SHALL capture the exit code and full standard output and standard error streams and store them in the Execution_Record.
5. IF a command execution exceeds the configured timeout for that Command_Entry, THEN THE Executor SHALL terminate the running process on the Sibling_Container and record the Execution_Record with a "timed out" status.
6. THE Executor SHALL enforce that only one instance of the same Command_Entry executes at a time, unless the Command_Entry is explicitly marked as allowing concurrent execution.

### Requirement 4: Execution History and Audit

**User Story:** As a User, I want to view the history of command executions, so that I can review past results and troubleshoot issues.

#### Acceptance Criteria

1. WHEN a User navigates to the execution history view, THE Dashboard SHALL display a paginated list of Execution_Records that the User's Role is authorized to view, ordered by most recent first.
2. WHEN a User selects an Execution_Record, THE Dashboard SHALL display the command name, parameters used, executing User, start time, end time, exit code, and captured output.
3. THE Dashboard SHALL allow Users to filter Execution_Records by command name, executing User, date range, and execution status.
4. THE Command_Catalog SHALL retain all Execution_Records for a minimum of 90 days.

### Requirement 5: Authentication and Role-Based Access Control

**User Story:** As an Admin, I want to control which teams can access which commands, so that production operations remain secure and auditable.

#### Acceptance Criteria

1. WHEN an unauthenticated request reaches the Dashboard, THE Dashboard SHALL redirect the request to the authentication page.
2. WHEN a User authenticates with valid credentials, THE Dashboard SHALL establish a session and grant access based on the User's assigned Roles.
3. WHEN a User attempts to execute a Command_Entry that the User's Role is not authorized for, THE Executor SHALL reject the request and THE Dashboard SHALL display an "access denied" message.
4. WHEN an Admin assigns or removes a Role from a User, THE Dashboard SHALL enforce the updated permissions on the next request from that User.
5. IF an authenticated session expires or is invalidated, THEN THE Dashboard SHALL require the User to re-authenticate before processing further requests.

### Requirement 6: Docker-out-of-Docker Execution

**User Story:** As a DevOps engineer, I want the system to execute commands on sibling Docker containers, so that the platform runs in a containerized environment without nested Docker daemons.

#### Acceptance Criteria

1. THE Executor SHALL communicate with the host Docker daemon through the mounted Docker_Socket to manage Sibling_Containers.
2. WHEN the Executor launches a command, THE Executor SHALL create a new Sibling_Container from the image specified in the Command_Entry, execute the command, capture the output, and remove the Sibling_Container after completion.
3. THE Executor SHALL mount only the volumes explicitly declared in the Command_Entry configuration into the Sibling_Container, and SHALL NOT mount the Docker_Socket into the Sibling_Container.
4. IF the Docker daemon is unreachable through the Docker_Socket, THEN THE Executor SHALL return a descriptive connectivity error and record the failure in the Execution_Record.
5. THE Executor SHALL enforce resource limits (CPU and memory) on each Sibling_Container as defined in the Command_Entry configuration.

### Requirement 7: Containerized Deployment

**User Story:** As a DevOps engineer, I want the entire KARIZ platform to be deployable as a Docker container, so that it integrates seamlessly into existing container infrastructure.

#### Acceptance Criteria

1. THE KARIZ system SHALL be packaged as a single Docker image containing the Dashboard, Executor, and Command_Catalog services.
2. THE KARIZ Docker image SHALL accept configuration through environment variables for database connection, Docker_Socket path, authentication provider settings, and application port.
3. WHEN the KARIZ container starts, THE system SHALL run database migrations automatically before accepting requests.
4. THE KARIZ Docker image SHALL expose a health-check endpoint that returns the operational status of the Dashboard, Executor, and database connectivity.

### Requirement 8: Parameter Handling and Validation

**User Story:** As a User, I want to provide parameters when executing a command, so that I can customize the command behavior for my specific needs (e.g., specifying a database name for a dump, or a date range for data extraction).

#### Acceptance Criteria

1. WHEN an Admin registers a Command_Entry, THE Command_Catalog SHALL accept a Parameter_Schema defining each parameter's name, type (string, number, boolean, enum), required/optional status, default value, and validation constraints.
2. WHEN a User opens the execution form for a Command_Entry, THE Dashboard SHALL render input fields matching the Parameter_Schema, pre-populated with default values where defined.
3. WHEN the Executor constructs the command for execution, THE Executor SHALL inject validated parameters into the command string using a safe substitution mechanism that prevents shell injection.
4. IF a required parameter is missing from the execution request, THEN THE Dashboard SHALL reject the request and identify the missing parameter by name.

### Requirement 9: Real-Time Output Streaming

**User Story:** As a User, I want to see command output in real time as it executes, so that I can monitor progress and detect issues early without waiting for completion.

#### Acceptance Criteria

1. WHILE a command is executing, THE Dashboard SHALL stream standard output and standard error from the Sibling_Container to the User's browser in real time.
2. WHEN a User opens the detail view of a currently running command, THE Dashboard SHALL display the output accumulated so far and continue streaming new output.
3. WHEN the real-time connection between the Dashboard and the User's browser is interrupted, THE Dashboard SHALL allow the User to reconnect and resume viewing from the last received output position.

### Requirement 10: Notifications

**User Story:** As a User, I want to be notified when my command execution completes, so that I do not have to watch the Dashboard continuously.

#### Acceptance Criteria

1. WHEN a command execution completes or fails, THE Dashboard SHALL send an in-app notification to the User who initiated the execution.
2. WHERE email notifications are configured, THE Dashboard SHALL send an email to the initiating User with the command name, execution status, and a link to the Execution_Record.

### Requirement 11: Environment Variable Mapping from Sibling Containers

**User Story:** As an Admin, I want to bind command parameters to environment variables from already-running sibling containers, so that commands can reference credentials and configuration from other services without hardcoding secrets.

#### Acceptance Criteria

1. WHEN an Admin registers or updates a Command_Entry, THE Command_Catalog SHALL accept an optional Environment_Variable_Mapping for each parameter, specifying a source container name and environment variable name.
2. WHEN the Executor prepares a command execution that includes parameters with Environment_Variable_Mappings, THE Executor SHALL read the specified environment variable from the running source container using the Docker inspect API and inject the resolved value into the command.
3. IF the source container specified in an Environment_Variable_Mapping is not running at execution time, THEN THE Executor SHALL reject the execution request and record a descriptive error in the Execution_Record identifying the missing container.
4. IF the specified environment variable does not exist on the source container, THEN THE Executor SHALL reject the execution request and record a descriptive error in the Execution_Record identifying the missing variable.
5. WHEN a parameter has both a user-provided value and an Environment_Variable_Mapping, THE Executor SHALL use the user-provided value and ignore the mapping.

### Requirement 12: File Artifact Handling

**User Story:** As a User, I want to download file outputs produced by command executions (e.g., database dumps, CSV exports), so that I can use the generated files without SSH access to production servers.

#### Acceptance Criteria

1. WHEN an Admin registers a Command_Entry, THE Command_Catalog SHALL accept an optional list of declared output Artifacts, each specifying a file path inside the container and a human-readable label.
2. WHEN a command execution completes and the Command_Entry declares output Artifacts, THE Executor SHALL copy the declared files from the container before removing the container and store them at the configured Artifact_Destination.
3. WHEN a User requests to download an Artifact from a completed Execution_Record, THE Dashboard SHALL serve the file through a download endpoint with the correct content type and filename.
4. WHEN an Admin registers a Command_Entry with Artifacts, THE Command_Catalog SHALL accept an Artifact_Destination configuration specifying the storage backend (local filesystem, MinIO, or S3) and backend-specific settings (bucket name, path prefix, credentials reference).
5. IF the Executor fails to copy a declared Artifact file from the container, THEN THE Executor SHALL record a warning in the Execution_Record identifying the missing file and continue processing remaining Artifacts.
6. THE Dashboard SHALL display the list of available Artifacts on the Execution_Record detail view with download links for each successfully stored Artifact.

### Requirement 13: Execute on Existing Running Containers

**User Story:** As an Admin, I want to register commands that execute inside already-running containers (e.g., the backend service), so that teams can run diagnostics and operations on live services without creating new containers.

#### Acceptance Criteria

1. WHEN an Admin registers a Command_Entry, THE Command_Catalog SHALL accept an Execution_Mode field with a value of "create" or "exec".
2. WHEN the Execution_Mode is "exec", THE Command_Catalog SHALL require a target container name or ID identifying the running Sibling_Container where the command will execute.
3. WHEN the Executor processes a command with Execution_Mode "exec", THE Executor SHALL use the Docker exec API to run the command inside the specified running container and capture the standard output and standard error streams.
4. WHEN the Executor processes a command with Execution_Mode "create", THE Executor SHALL follow the existing behavior of creating a new ephemeral Sibling_Container, executing the command, capturing output, and removing the container.
5. IF the target container specified for an "exec" mode command is not running at execution time, THEN THE Executor SHALL reject the execution request and record a descriptive error in the Execution_Record.
6. WHILE executing a command in "exec" mode, THE Executor SHALL enforce the same timeout policy as "create" mode commands, terminating the exec process if the configured timeout is exceeded.

### Requirement 14: Command Scheduling

**User Story:** As a User, I want to schedule commands to run automatically at specified times or intervals, so that recurring operations (e.g., nightly database dumps, hourly health checks) execute without manual intervention.

#### Acceptance Criteria

1. WHEN a User creates a Schedule for a Command_Entry, THE Dashboard SHALL accept a cron expression or a fixed interval defining the recurrence pattern.
2. THE Executor SHALL evaluate all active Schedules and trigger command executions at the times specified by each Schedule's cron expression or interval.
3. WHEN a scheduled execution is triggered, THE Executor SHALL create an Execution_Record linked to the Schedule that triggered the execution and to the User who created the Schedule.
4. WHEN a User views the Schedule management page, THE Dashboard SHALL display all Schedules the User's Role is authorized to view, including the cron expression, next scheduled run time, and enabled/disabled status.
5. WHEN a User enables or disables a Schedule, THE Dashboard SHALL update the Schedule status and THE Executor SHALL respect the updated status on the next evaluation cycle.
6. WHEN a User deletes a Schedule, THE Dashboard SHALL remove the Schedule and THE Executor SHALL stop triggering executions for that Schedule.
7. IF a scheduled execution fails, THEN THE Dashboard SHALL send a notification to the User who created the Schedule with the failure details and a link to the Execution_Record.

## Technology Research

This section outlines the key technology decisions and research areas that must be investigated before implementation begins. Since KARIZ is a brand-new project, these choices will shape the entire architecture.

### Research Area 1: Backend Framework

**Question:** Which backend framework and language should KARIZ use for the Dashboard API and Executor service?

**Considerations:**
- Must support WebSocket or SSE for real-time output streaming (Requirement 9)
- Must have mature Docker SDK/library support for Docker-out-of-Docker execution (Requirement 6)
- Must support background task execution for long-running commands (Requirement 3)
- Candidates to evaluate: Node.js (Express/Fastify), Python (FastAPI/Django), Go (Gin/Echo)

### Research Area 2: Frontend Framework

**Question:** Which frontend framework should KARIZ use for the Dashboard UI?

**Considerations:**
- Must support real-time data updates and WebSocket/SSE consumption (Requirement 9)
- Must support dynamic form generation from Parameter_Schema (Requirement 8)
- Must provide a component library suitable for admin dashboards
- Candidates to evaluate: React, Vue.js, Svelte

### Research Area 3: Database

**Question:** Which database should KARIZ use for storing the Command_Catalog, Execution_Records, and User data?

**Considerations:**
- Must support structured data with relationships (commands, users, roles, execution records)
- Must handle potentially large text blobs for command output storage (Requirement 4)
- Must be easily containerized alongside the KARIZ application
- Candidates to evaluate: PostgreSQL, SQLite, MySQL

### Research Area 4: Authentication Strategy

**Question:** How should KARIZ handle authentication and session management?

**Considerations:**
- Must support role-based access control with multiple roles per user (Requirement 5)
- Should integrate with existing organizational identity providers if possible
- Must support session expiration and invalidation (Requirement 5, AC 5)
- Candidates to evaluate: JWT-based auth, session-based auth, OAuth2/OIDC integration, LDAP

### Research Area 5: Docker SDK and Communication

**Question:** Which Docker client library should KARIZ use for Docker-out-of-Docker operations?

**Considerations:**
- Must support container creation, execution, output streaming, and cleanup via Docker_Socket (Requirement 6)
- Must support resource limit enforcement (CPU, memory) (Requirement 6, AC 5)
- Must support volume mounting configuration (Requirement 6, AC 3)
- Library choice depends on backend language decision (Research Area 1)

### Research Area 6: Real-Time Communication

**Question:** Which protocol and library should KARIZ use for streaming command output to the browser?

**Considerations:**
- Must support bidirectional or server-push communication for live output (Requirement 9)
- Must handle reconnection gracefully (Requirement 9, AC 3)
- Candidates to evaluate: WebSockets, Server-Sent Events (SSE), Socket.IO

### Research Area 7: Cron Scheduling Library for Go

**Question:** Which Go library should KARIZ use for evaluating cron expressions and scheduling recurring command executions?

**Considerations:**
- Must support standard cron expression syntax (5-field and optional 6-field with seconds) (Requirement 14)
- Must support calculating next run times for display in the Dashboard (Requirement 14, AC 4)
- Must be safe for concurrent use from multiple goroutines
- Candidates to evaluate: `robfig/cron`, `go-co-op/gocron`, custom ticker-based implementation

### Research Area 8: Object Storage SDK for Artifact Storage

**Question:** Which Go SDK should KARIZ use for uploading command output artifacts to external object storage?

**Considerations:**
- Must support MinIO and AWS S3 as storage backends (Requirement 12, AC 4)
- Must support streaming uploads to avoid buffering large files in memory
- Must support presigned URLs or direct download proxying for artifact retrieval (Requirement 12, AC 3)
- Candidates to evaluate: `minio/minio-go` (S3-compatible, works with both MinIO and AWS S3), `aws/aws-sdk-go-v2`

### Research Area 9: Docker Inspect and Exec APIs

**Question:** How should KARIZ use the Docker API to inspect running container environment variables and execute commands inside running containers?

**Considerations:**
- Must support reading environment variables from running containers via Docker inspect (Requirement 11)
- Must support executing commands inside running containers via Docker exec API (Requirement 13)
- Must support attaching to exec instance stdout/stderr for output streaming (Requirement 13, AC 3)
- The canonical Go Docker client (`github.com/docker/docker/client`) supports both `ContainerInspect` and `ContainerExecCreate`/`ContainerExecAttach`