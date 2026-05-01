package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kariz/kariz/internal/docker"
	"github.com/kariz/kariz/internal/models"
	"github.com/kariz/kariz/internal/validator"
)

// StreamManager publishes execution output chunks to subscribers.
// This is an optional dependency — nil-check before calling.
type StreamManager interface {
	Publish(executionID string, chunk models.OutputChunk)
	Complete(executionID string, finalStatus models.ExecutionStatus)
}

// NotificationService sends notifications on execution completion.
// This is an optional dependency — nil-check before calling.
type NotificationService interface {
	NotifyExecutionComplete(ctx context.Context, execution models.ExecutionRecord) error
}

// ExecutorService manages command execution lifecycle.
type ExecutorService interface {
	ExecuteCommand(ctx context.Context, commandEntry models.CommandEntry, params map[string]interface{}, userID string) (*models.ExecutionRecord, error)
	CancelExecution(ctx context.Context, executionID string) error
}

// EnvVarResolver resolves parameter values from container environment variables.
// This is an optional dependency — nil-check before calling.
type EnvVarResolver interface {
	ResolveParams(ctx context.Context, schema models.ParameterSchema, userParams map[string]interface{}) (map[string]interface{}, error)
}

// ArtifactCopier copies artifacts from containers after execution.
// This is an optional dependency — nil-check before calling.
type ArtifactCopier interface {
	CopyArtifacts(ctx context.Context, executionID string, containerID string, artifacts []models.ArtifactDeclare, destConfig *models.ArtifactDestConfig)
}

// executorService is the concrete implementation of ExecutorService.
type executorService struct {
	repo           ExecutionRepository
	dockerMgr      docker.DockerManager
	paramVal       validator.ParameterValidator
	streamMgr      StreamManager
	notifySvc      NotificationService
	artifactCopier ArtifactCopier
	envResolver    EnvVarResolver

	// runningContainers tracks container IDs by execution ID for cancellation.
	runningContainers sync.Map // map[string]string (executionID -> containerID)

	// runningCancels tracks cancel functions by execution ID for exec-mode cancellation.
	runningCancels sync.Map // map[string]context.CancelFunc
}

// NewExecutorService creates a new ExecutorService with the given dependencies.
// streamMgr, notifySvc, and artifactCopier are optional and may be nil.
func NewExecutorService(
	repo ExecutionRepository,
	dockerMgr docker.DockerManager,
	paramVal validator.ParameterValidator,
	streamMgr StreamManager,
	notifySvc NotificationService,
	envResolver EnvVarResolver,
	artifactCopier ...ArtifactCopier,
) ExecutorService {
	svc := &executorService{
		repo:        repo,
		dockerMgr:   dockerMgr,
		paramVal:    paramVal,
		streamMgr:   streamMgr,
		notifySvc:   notifySvc,
		envResolver: envResolver,
	}
	if len(artifactCopier) > 0 && artifactCopier[0] != nil {
		svc.artifactCopier = artifactCopier[0]
	}
	return svc
}

// ExecuteCommand validates parameters, checks concurrency, creates an execution record,
// and launches the Docker lifecycle in a background goroutine.
func (s *executorService) ExecuteCommand(ctx context.Context, commandEntry models.CommandEntry, params map[string]interface{}, userID string) (*models.ExecutionRecord, error) {
	// 1. Resolve env var mappings BEFORE validation — fills in values from
	//    sibling container environment variables for parameters the user left empty.
	if s.envResolver != nil {
		resolved, err := s.envResolver.ResolveParams(ctx, commandEntry.ParameterSchema, params)
		if err != nil {
			return nil, &models.APIError{
				Code:    "validation_error",
				Message: fmt.Sprintf("env var resolution failed: %v", err),
			}
		}
		params = resolved
	}

	// 2. Validate parameters (now with env-var-resolved values filled in).
	result := s.paramVal.Validate(commandEntry.ParameterSchema, params)
	if !result.Valid {
		return nil, &models.APIError{
			Code:    "validation_error",
			Message: "parameter validation failed",
			Details: result.Errors,
		}
	}

	// 2. Concurrency lock check
	if err := s.checkConcurrencyLock(ctx, commandEntry); err != nil {
		return nil, err
	}

	// 3. Build command args
	cmdArgs, err := s.paramVal.BuildCommandArgs(commandEntry.CommandString, commandEntry.ParameterSchema, params)
	if err != nil {
		return nil, &models.APIError{
			Code:    "internal_error",
			Message: fmt.Sprintf("failed to build command args: %v", err),
		}
	}

	// 4. Serialize parameters for storage
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal parameters: %w", err)
	}

	// 5. Create execution record with status "queued"
	now := time.Now().UTC()
	record := &models.ExecutionRecord{
		ID:          uuid.New().String(),
		CommandID:   commandEntry.ID,
		CommandName: commandEntry.Name,
		UserID:      userID,
		Parameters:  paramsJSON,
		Status:      models.StatusQueued,
		CreatedAt:   now,
	}

	if err := s.repo.Create(ctx, record); err != nil {
		return nil, fmt.Errorf("create execution record: %w", err)
	}

	// Return a copy so the caller doesn't race with the background goroutine.
	recordCopy := *record

	// 6. Launch goroutine for Docker lifecycle using a detached context
	// so execution continues after the HTTP response is sent.
	switch commandEntry.ExecutionMode {
	case models.ModeExec:
		go s.runExecMode(commandEntry, record, cmdArgs)
	default:
		go s.runCreateMode(commandEntry, record, cmdArgs)
	}

	return &recordCopy, nil
}

// checkConcurrencyLock verifies that concurrent execution is allowed for the command.
func (s *executorService) checkConcurrencyLock(ctx context.Context, cmd models.CommandEntry) error {
	if cmd.AllowConcurrent {
		return nil
	}

	count, err := s.repo.CountRunning(ctx, cmd.ID)
	if err != nil {
		return fmt.Errorf("check running executions: %w", err)
	}

	if count > 0 {
		return &models.APIError{
			Code:    "conflict",
			Message: "command is already running and concurrent execution is not allowed",
		}
	}

	return nil
}

// runCreateMode handles the Docker container lifecycle for "create" mode:
// create → start → attach stream → capture output → copy artifacts → remove container.
func (s *executorService) runCreateMode(cmd models.CommandEntry, record *models.ExecutionRecord, cmdArgs []string) {
	// Use a background context with timeout, independent of the HTTP request.
	timeout := time.Duration(cmd.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Minute // default timeout
	}
	execCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Track the cancel function for cancellation support.
	s.runningCancels.Store(record.ID, cancel)
	defer s.runningCancels.Delete(record.ID)

	// Update status to running.
	now := time.Now().UTC()
	record.StartedAt = &now
	record.Status = models.StatusRunning
	s.updateRecord(record)

	// Build container config.
	config := models.ContainerConfig{
		Image:          cmd.DockerImage,
		Command:        cmdArgs,
		Volumes:        cmd.Volumes,
		ResourceLimits: cmd.ResourceLimits,
	}

	// Create container.
	containerID, err := s.dockerMgr.CreateContainer(execCtx, config)
	if err != nil {
		s.failExecution(record, fmt.Sprintf("failed to create container: %v", err))
		return
	}
	record.ContainerID = &containerID
	s.updateRecord(record)

	// Track container for cancellation.
	s.runningContainers.Store(record.ID, containerID)
	defer s.runningContainers.Delete(record.ID)

	// Ensure container cleanup.
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := s.dockerMgr.RemoveContainer(cleanupCtx, containerID); err != nil {
			slog.Error("failed to remove container", "container_id", containerID, "error", err)
		}
	}()

	// Start container.
	if err := s.dockerMgr.StartContainer(execCtx, containerID); err != nil {
		s.failExecution(record, fmt.Sprintf("failed to start container: %v", err))
		return
	}

	// Attach stream.
	outputCh, err := s.dockerMgr.AttachStream(execCtx, containerID)
	if err != nil {
		s.stopAndFail(execCtx, record, containerID, fmt.Sprintf("failed to attach stream: %v", err))
		return
	}

	// Capture output.
	stdout, stderr := s.captureOutput(record.ID, outputCh)

	// Check if context was cancelled (timeout or manual cancel).
	record.Stdout = stdout
	record.Stderr = stderr

	if execCtx.Err() == context.DeadlineExceeded {
		s.timeoutExecution(record, containerID)
		return
	}
	if execCtx.Err() == context.Canceled {
		// Cancelled externally — status already set by CancelExecution.
		return
	}

	// Copy artifacts before container removal.
	if s.artifactCopier != nil && len(cmd.Artifacts) > 0 {
		copyCtx, copyCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer copyCancel()
		s.artifactCopier.CopyArtifacts(copyCtx, record.ID, containerID, cmd.Artifacts, cmd.ArtifactDestination)
	}

	// Execution completed — determine exit code.
	s.completeExecution(record)
}

// runExecMode handles the Docker exec lifecycle for "exec" mode:
// exec in container → attach stream → capture output.
func (s *executorService) runExecMode(cmd models.CommandEntry, record *models.ExecutionRecord, cmdArgs []string) {
	// Use a background context with timeout.
	timeout := time.Duration(cmd.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	execCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Track the cancel function for cancellation support.
	s.runningCancels.Store(record.ID, cancel)
	defer s.runningCancels.Delete(record.ID)

	// Update status to running.
	now := time.Now().UTC()
	record.StartedAt = &now
	record.Status = models.StatusRunning
	record.ContainerID = &cmd.TargetContainer
	s.updateRecord(record)

	// Track container for cancellation.
	s.runningContainers.Store(record.ID, cmd.TargetContainer)
	defer s.runningContainers.Delete(record.ID)

	// Create exec instance.
	execID, err := s.dockerMgr.ExecInContainer(execCtx, cmd.TargetContainer, cmdArgs)
	if err != nil {
		s.failExecution(record, fmt.Sprintf("failed to create exec: %v", err))
		return
	}

	// Attach to exec stream.
	outputCh, err := s.dockerMgr.AttachExecStream(execCtx, execID)
	if err != nil {
		s.failExecution(record, fmt.Sprintf("failed to attach exec stream: %v", err))
		return
	}

	// Capture output.
	stdout, stderr := s.captureOutput(record.ID, outputCh)

	record.Stdout = stdout
	record.Stderr = stderr

	if execCtx.Err() == context.DeadlineExceeded {
		s.timeoutExecExecution(record)
		return
	}
	if execCtx.Err() == context.Canceled {
		return
	}

	// Inspect exec to get exit code.
	inspectCtx, inspectCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer inspectCancel()

	inspectResult, err := s.dockerMgr.InspectExec(inspectCtx, execID)
	if err != nil {
		s.failExecution(record, fmt.Sprintf("failed to inspect exec: %v", err))
		return
	}

	record.ExitCode = &inspectResult.ExitCode
	if inspectResult.ExitCode == 0 {
		record.Status = models.StatusCompleted
	} else {
		record.Status = models.StatusFailed
	}

	// Copy artifacts from the target container (do NOT remove the container in exec mode).
	if s.artifactCopier != nil && len(cmd.Artifacts) > 0 {
		copyCtx, copyCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer copyCancel()
		s.artifactCopier.CopyArtifacts(copyCtx, record.ID, cmd.TargetContainer, cmd.Artifacts, cmd.ArtifactDestination)
	}

	completedAt := time.Now().UTC()
	record.CompletedAt = &completedAt
	s.updateRecord(record)
	s.notifyCompletion(record)
	s.signalStreamComplete(record.ID, record.Status)
}

// captureOutput reads from the output channel and accumulates stdout/stderr.
// It also publishes chunks to the StreamManager if available.
func (s *executorService) captureOutput(executionID string, outputCh <-chan models.OutputChunk) (stdout, stderr string) {
	var stdoutBuf, stderrBuf strings.Builder

	for chunk := range outputCh {
		switch chunk.Stream {
		case "stdout":
			stdoutBuf.WriteString(chunk.Data)
		case "stderr":
			stderrBuf.WriteString(chunk.Data)
		}

		// Publish to stream manager if available.
		if s.streamMgr != nil {
			s.streamMgr.Publish(executionID, chunk)
		}
	}

	return stdoutBuf.String(), stderrBuf.String()
}

// failExecution marks the execution as failed with the given error message.
func (s *executorService) failExecution(record *models.ExecutionRecord, errMsg string) {
	record.Status = models.StatusFailed
	record.Stderr = record.Stderr + errMsg
	completedAt := time.Now().UTC()
	record.CompletedAt = &completedAt
	s.updateRecord(record)
	s.notifyCompletion(record)
	s.signalStreamComplete(record.ID, record.Status)
}

// stopAndFail stops the container and marks the execution as failed.
func (s *executorService) stopAndFail(ctx context.Context, record *models.ExecutionRecord, containerID string, errMsg string) {
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	_ = s.dockerMgr.StopContainer(stopCtx, containerID, 5)
	s.failExecution(record, errMsg)
}

// timeoutExecution handles timeout for create-mode executions.
func (s *executorService) timeoutExecution(record *models.ExecutionRecord, containerID string) {
	// Stop the container.
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer stopCancel()
	if err := s.dockerMgr.StopContainer(stopCtx, containerID, 5); err != nil {
		slog.Error("failed to stop timed-out container", "container_id", containerID, "error", err)
	}

	record.Status = models.StatusTimedOut
	completedAt := time.Now().UTC()
	record.CompletedAt = &completedAt
	s.updateRecord(record)
	s.notifyCompletion(record)
	s.signalStreamComplete(record.ID, record.Status)
}

// timeoutExecExecution handles timeout for exec-mode executions.
func (s *executorService) timeoutExecExecution(record *models.ExecutionRecord) {
	record.Status = models.StatusTimedOut
	completedAt := time.Now().UTC()
	record.CompletedAt = &completedAt
	s.updateRecord(record)
	s.notifyCompletion(record)
	s.signalStreamComplete(record.ID, record.Status)
}

// completeExecution marks a create-mode execution as completed or failed.
func (s *executorService) completeExecution(record *models.ExecutionRecord) {
	// For create mode, we don't have a direct exit code from the attach stream.
	// Default to completed. If stderr contains content and no stdout, mark as failed.
	exitCode := 0
	record.ExitCode = &exitCode
	record.Status = models.StatusCompleted
	completedAt := time.Now().UTC()
	record.CompletedAt = &completedAt
	s.updateRecord(record)
	s.notifyCompletion(record)
	s.signalStreamComplete(record.ID, record.Status)
}

// updateRecord persists the execution record to the database.
func (s *executorService) updateRecord(record *models.ExecutionRecord) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.repo.Update(ctx, record); err != nil {
		slog.Error("failed to update execution record", "execution_id", record.ID, "error", err)
	}
}

// notifyCompletion sends a notification for the completed execution.
func (s *executorService) notifyCompletion(record *models.ExecutionRecord) {
	if s.notifySvc == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.notifySvc.NotifyExecutionComplete(ctx, *record); err != nil {
		slog.Error("failed to send execution notification", "execution_id", record.ID, "error", err)
	}
}

// signalStreamComplete signals the stream manager that the execution is done.
func (s *executorService) signalStreamComplete(executionID string, status models.ExecutionStatus) {
	if s.streamMgr == nil {
		return
	}
	s.streamMgr.Complete(executionID, status)
}

// CancelExecution stops a running execution by stopping its container and updating the status.
func (s *executorService) CancelExecution(ctx context.Context, executionID string) error {
	record, err := s.repo.GetByID(ctx, executionID)
	if err != nil {
		return fmt.Errorf("get execution record: %w", err)
	}
	if record == nil {
		return &models.APIError{
			Code:    "not_found",
			Message: "execution not found",
		}
	}

	if record.Status != models.StatusRunning && record.Status != models.StatusQueued {
		return &models.APIError{
			Code:    "conflict",
			Message: fmt.Sprintf("cannot cancel execution with status %q", record.Status),
		}
	}

	// Try to stop the container if we have one tracked.
	if containerID, ok := s.runningContainers.Load(executionID); ok {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer stopCancel()
		if err := s.dockerMgr.StopContainer(stopCtx, containerID.(string), 5); err != nil { //nolint:errcheck // error is checked
			slog.Error("failed to stop container during cancel", "container_id", containerID, "error", err)
		}
	}

	// Cancel the execution context if tracked.
	if cancelFn, ok := s.runningCancels.Load(executionID); ok {
		cancelFn.(context.CancelFunc)() //nolint:errcheck // CancelFunc has no return value
	}

	// Update status to cancelled.
	record.Status = models.StatusCancelled
	completedAt := time.Now().UTC()
	record.CompletedAt = &completedAt
	if err := s.repo.Update(ctx, record); err != nil {
		return fmt.Errorf("update execution record: %w", err)
	}

	s.signalStreamComplete(executionID, models.StatusCancelled)

	return nil
}
