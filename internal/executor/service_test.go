package executor

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/kariz/kariz/internal/models"
)

// --- Mock implementations ---

// mockExecutionRepository is a test double for ExecutionRepository.
type mockExecutionRepository struct {
	mu       sync.Mutex
	records  map[string]*models.ExecutionRecord
	running  map[string]int // commandID -> count of running
	createFn func(ctx context.Context, record *models.ExecutionRecord) error
	updateFn func(ctx context.Context, record *models.ExecutionRecord) error
}

func newMockRepo() *mockExecutionRepository {
	return &mockExecutionRepository{
		records: make(map[string]*models.ExecutionRecord),
		running: make(map[string]int),
	}
}

func (m *mockExecutionRepository) Create(ctx context.Context, record *models.ExecutionRecord) error {
	if m.createFn != nil {
		return m.createFn(ctx, record)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records[record.ID] = record
	return nil
}

func (m *mockExecutionRepository) Update(ctx context.Context, record *models.ExecutionRecord) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, record)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records[record.ID] = record
	return nil
}

func (m *mockExecutionRepository) GetByID(ctx context.Context, id string) (*models.ExecutionRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return nil, nil
	}
	return r, nil
}

func (m *mockExecutionRepository) List(ctx context.Context, filter models.ExecutionFilter) (*models.PaginatedResult, error) {
	return &models.PaginatedResult{Items: []models.ExecutionRecord{}, Total: 0, Page: 1, PageSize: 20}, nil
}

func (m *mockExecutionRepository) CountRunning(ctx context.Context, commandID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running[commandID], nil
}

func (m *mockExecutionRepository) setRunningCount(commandID string, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running[commandID] = count
}

func (m *mockExecutionRepository) getRecord(id string) *models.ExecutionRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.records[id]
}

// mockDockerManager is a test double for DockerManager.
type mockDockerManager struct {
	createContainerFn   func(ctx context.Context, config models.ContainerConfig) (string, error)
	startContainerFn    func(ctx context.Context, containerID string) error
	attachStreamFn      func(ctx context.Context, containerID string) (<-chan models.OutputChunk, error)
	stopContainerFn     func(ctx context.Context, containerID string, timeout int) error
	removeContainerFn   func(ctx context.Context, containerID string) error
	isAvailableFn       func(ctx context.Context) error
	execInContainerFn   func(ctx context.Context, containerID string, command []string) (string, error)
	attachExecStreamFn  func(ctx context.Context, execID string) (<-chan models.OutputChunk, error)
	inspectExecFn       func(ctx context.Context, execID string) (*models.ExecInspectResult, error)
	inspectContainerEnvFn func(ctx context.Context, containerNameOrID string) (map[string]string, error)
	copyFromContainerFn func(ctx context.Context, containerID string, srcPath string) (io.ReadCloser, error)
}

func (m *mockDockerManager) CreateContainer(ctx context.Context, config models.ContainerConfig) (string, error) {
	if m.createContainerFn != nil {
		return m.createContainerFn(ctx, config)
	}
	return "container-123", nil
}

func (m *mockDockerManager) StartContainer(ctx context.Context, containerID string) error {
	if m.startContainerFn != nil {
		return m.startContainerFn(ctx, containerID)
	}
	return nil
}

func (m *mockDockerManager) AttachStream(ctx context.Context, containerID string) (<-chan models.OutputChunk, error) {
	if m.attachStreamFn != nil {
		return m.attachStreamFn(ctx, containerID)
	}
	ch := make(chan models.OutputChunk)
	close(ch)
	return ch, nil
}

func (m *mockDockerManager) StopContainer(ctx context.Context, containerID string, timeout int) error {
	if m.stopContainerFn != nil {
		return m.stopContainerFn(ctx, containerID, timeout)
	}
	return nil
}

func (m *mockDockerManager) RemoveContainer(ctx context.Context, containerID string) error {
	if m.removeContainerFn != nil {
		return m.removeContainerFn(ctx, containerID)
	}
	return nil
}

func (m *mockDockerManager) IsAvailable(ctx context.Context) error {
	if m.isAvailableFn != nil {
		return m.isAvailableFn(ctx)
	}
	return nil
}

func (m *mockDockerManager) ExecInContainer(ctx context.Context, containerID string, command []string) (string, error) {
	if m.execInContainerFn != nil {
		return m.execInContainerFn(ctx, containerID, command)
	}
	return "exec-123", nil
}

func (m *mockDockerManager) AttachExecStream(ctx context.Context, execID string) (<-chan models.OutputChunk, error) {
	if m.attachExecStreamFn != nil {
		return m.attachExecStreamFn(ctx, execID)
	}
	ch := make(chan models.OutputChunk)
	close(ch)
	return ch, nil
}

func (m *mockDockerManager) InspectExec(ctx context.Context, execID string) (*models.ExecInspectResult, error) {
	if m.inspectExecFn != nil {
		return m.inspectExecFn(ctx, execID)
	}
	return &models.ExecInspectResult{ExitCode: 0, Running: false}, nil
}

func (m *mockDockerManager) InspectContainerEnv(ctx context.Context, containerNameOrID string) (map[string]string, error) {
	if m.inspectContainerEnvFn != nil {
		return m.inspectContainerEnvFn(ctx, containerNameOrID)
	}
	return map[string]string{}, nil
}

func (m *mockDockerManager) CopyFromContainer(ctx context.Context, containerID string, srcPath string) (io.ReadCloser, error) {
	if m.copyFromContainerFn != nil {
		return m.copyFromContainerFn(ctx, containerID, srcPath)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockDockerManager) ContainerLogs(ctx context.Context, containerID string, opts models.LogOptions) (<-chan models.OutputChunk, error) {
	ch := make(chan models.OutputChunk)
	close(ch)
	return ch, nil
}

// mockParameterValidator is a test double for ParameterValidator.
type mockParameterValidator struct {
	validateFn       func(schema models.ParameterSchema, params map[string]interface{}) *models.ValidationResult
	buildCommandArgsFn func(commandString string, schema models.ParameterSchema, validatedParams map[string]interface{}) ([]string, error)
}

func (m *mockParameterValidator) Validate(schema models.ParameterSchema, params map[string]interface{}) *models.ValidationResult {
	if m.validateFn != nil {
		return m.validateFn(schema, params)
	}
	return &models.ValidationResult{Valid: true}
}

func (m *mockParameterValidator) BuildCommandArgs(commandString string, schema models.ParameterSchema, validatedParams map[string]interface{}) ([]string, error) {
	if m.buildCommandArgsFn != nil {
		return m.buildCommandArgsFn(commandString, schema, validatedParams)
	}
	return []string{"echo", "hello"}, nil
}

// mockStreamManager is a test double for StreamManager.
type mockStreamManager struct {
	mu        sync.Mutex
	published []models.OutputChunk
	completed map[string]models.ExecutionStatus
}

func newMockStreamManager() *mockStreamManager {
	return &mockStreamManager{
		completed: make(map[string]models.ExecutionStatus),
	}
}

func (m *mockStreamManager) Publish(executionID string, chunk models.OutputChunk) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published = append(m.published, chunk)
}

func (m *mockStreamManager) Complete(executionID string, finalStatus models.ExecutionStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completed[executionID] = finalStatus
}

// mockNotificationService is a test double for NotificationService.
type mockNotificationService struct {
	mu       sync.Mutex
	notified []models.ExecutionRecord
}

func newMockNotificationService() *mockNotificationService {
	return &mockNotificationService{}
}

func (m *mockNotificationService) NotifyExecutionComplete(ctx context.Context, execution models.ExecutionRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notified = append(m.notified, execution)
	return nil
}

// --- Helper ---

func newTestCommand() models.CommandEntry {
	return models.CommandEntry{
		ID:              "cmd-1",
		Name:            "test-command",
		DockerImage:     "alpine:latest",
		CommandString:   "echo hello",
		ParameterSchema: models.ParameterSchema{},
		AllowedRoles:    []models.Role{models.RoleAdmin},
		TimeoutSeconds:  60,
		AllowConcurrent: false,
		ExecutionMode:   models.ModeCreate,
	}
}

// --- Tests ---

func TestCheckConcurrencyLock_BlocksWhenNotAllowed(t *testing.T) {
	repo := newMockRepo()
	repo.setRunningCount("cmd-1", 1)

	svc := &executorService{repo: repo}

	cmd := newTestCommand()
	cmd.AllowConcurrent = false

	err := svc.checkConcurrencyLock(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error for concurrent execution, got nil")
	}

	apiErr, ok := err.(*models.APIError)
	if !ok {
		t.Fatalf("expected *models.APIError, got %T", err)
	}
	if apiErr.Code != "conflict" {
		t.Errorf("expected code 'conflict', got %q", apiErr.Code)
	}
}

func TestCheckConcurrencyLock_AllowsWhenConcurrentEnabled(t *testing.T) {
	repo := newMockRepo()
	repo.setRunningCount("cmd-1", 3)

	svc := &executorService{repo: repo}

	cmd := newTestCommand()
	cmd.AllowConcurrent = true

	err := svc.checkConcurrencyLock(context.Background(), cmd)
	if err != nil {
		t.Fatalf("expected no error for concurrent execution, got %v", err)
	}
}

func TestCheckConcurrencyLock_AllowsWhenNoRunning(t *testing.T) {
	repo := newMockRepo()
	repo.setRunningCount("cmd-1", 0)

	svc := &executorService{repo: repo}

	cmd := newTestCommand()
	cmd.AllowConcurrent = false

	err := svc.checkConcurrencyLock(context.Background(), cmd)
	if err != nil {
		t.Fatalf("expected no error when no running executions, got %v", err)
	}
}

func TestExecuteCommand_ValidationFailure(t *testing.T) {
	repo := newMockRepo()
	paramVal := &mockParameterValidator{
		validateFn: func(schema models.ParameterSchema, params map[string]interface{}) *models.ValidationResult {
			return &models.ValidationResult{
				Valid: false,
				Errors: []models.ValidationError{
					{ParameterName: "env", Message: "required", Code: "required"},
				},
			}
		},
	}

	svc := NewExecutorService(repo, &mockDockerManager{}, paramVal, nil, nil, nil)

	cmd := newTestCommand()
	_, err := svc.ExecuteCommand(context.Background(), cmd, map[string]interface{}{}, "user-1")
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	apiErr, ok := err.(*models.APIError)
	if !ok {
		t.Fatalf("expected *models.APIError, got %T", err)
	}
	if apiErr.Code != "validation_error" {
		t.Errorf("expected code 'validation_error', got %q", apiErr.Code)
	}
}

func TestExecuteCommand_ConcurrencyBlocked(t *testing.T) {
	repo := newMockRepo()
	repo.setRunningCount("cmd-1", 1)

	svc := NewExecutorService(repo, &mockDockerManager{}, &mockParameterValidator{}, nil, nil, nil)

	cmd := newTestCommand()
	cmd.AllowConcurrent = false

	_, err := svc.ExecuteCommand(context.Background(), cmd, map[string]interface{}{}, "user-1")
	if err == nil {
		t.Fatal("expected concurrency error, got nil")
	}

	apiErr, ok := err.(*models.APIError)
	if !ok {
		t.Fatalf("expected *models.APIError, got %T", err)
	}
	if apiErr.Code != "conflict" {
		t.Errorf("expected code 'conflict', got %q", apiErr.Code)
	}
}

func TestExecuteCommand_ReturnsQueuedRecord(t *testing.T) {
	repo := newMockRepo()
	dockerMgr := &mockDockerManager{}
	paramVal := &mockParameterValidator{}

	svc := NewExecutorService(repo, dockerMgr, paramVal, nil, nil, nil)

	cmd := newTestCommand()
	record, err := svc.ExecuteCommand(context.Background(), cmd, map[string]interface{}{}, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if record.Status != models.StatusQueued {
		t.Errorf("expected status 'queued', got %q", record.Status)
	}
	if record.CommandID != "cmd-1" {
		t.Errorf("expected command_id 'cmd-1', got %q", record.CommandID)
	}
	if record.UserID != "user-1" {
		t.Errorf("expected user_id 'user-1', got %q", record.UserID)
	}
	if record.ID == "" {
		t.Error("expected non-empty execution ID")
	}
}

func TestExecuteCommand_CreateMode_CompletesSuccessfully(t *testing.T) {
	repo := newMockRepo()
	streamMgr := newMockStreamManager()
	notifySvc := newMockNotificationService()

	outputCh := make(chan models.OutputChunk, 2)
	outputCh <- models.OutputChunk{Stream: "stdout", Data: "hello world\n", Timestamp: time.Now()}
	outputCh <- models.OutputChunk{Stream: "stderr", Data: "warning\n", Timestamp: time.Now()}
	close(outputCh)

	dockerMgr := &mockDockerManager{
		attachStreamFn: func(ctx context.Context, containerID string) (<-chan models.OutputChunk, error) {
			return outputCh, nil
		},
	}

	svc := NewExecutorService(repo, dockerMgr, &mockParameterValidator{}, streamMgr, notifySvc, nil)

	cmd := newTestCommand()
	record, err := svc.ExecuteCommand(context.Background(), cmd, map[string]interface{}{}, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Poll for completion instead of sleeping to avoid race conditions.
	var finalRecord *models.ExecutionRecord
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for execution to complete")
		case <-time.After(50 * time.Millisecond):
			finalRecord = repo.getRecord(record.ID)
			if finalRecord != nil && finalRecord.Status == models.StatusCompleted {
				goto done
			}
		}
	}
done:

	if finalRecord.Stdout != "hello world\n" {
		t.Errorf("expected stdout 'hello world\\n', got %q", finalRecord.Stdout)
	}
	if finalRecord.Stderr != "warning\n" {
		t.Errorf("expected stderr 'warning\\n', got %q", finalRecord.Stderr)
	}

	// Verify stream manager received chunks.
	streamMgr.mu.Lock()
	if len(streamMgr.published) != 2 {
		t.Errorf("expected 2 published chunks, got %d", len(streamMgr.published))
	}
	if _, ok := streamMgr.completed[record.ID]; !ok {
		t.Error("expected stream completion signal")
	}
	streamMgr.mu.Unlock()

	// Verify notification was sent.
	notifySvc.mu.Lock()
	if len(notifySvc.notified) != 1 {
		t.Errorf("expected 1 notification, got %d", len(notifySvc.notified))
	}
	notifySvc.mu.Unlock()
}

func TestExecuteCommand_ExecMode_CompletesSuccessfully(t *testing.T) {
	repo := newMockRepo()

	outputCh := make(chan models.OutputChunk, 1)
	outputCh <- models.OutputChunk{Stream: "stdout", Data: "exec output\n", Timestamp: time.Now()}
	close(outputCh)

	dockerMgr := &mockDockerManager{
		attachExecStreamFn: func(ctx context.Context, execID string) (<-chan models.OutputChunk, error) {
			return outputCh, nil
		},
		inspectExecFn: func(ctx context.Context, execID string) (*models.ExecInspectResult, error) {
			return &models.ExecInspectResult{ExitCode: 0, Running: false}, nil
		},
	}

	svc := NewExecutorService(repo, dockerMgr, &mockParameterValidator{}, nil, nil, nil)

	cmd := newTestCommand()
	cmd.ExecutionMode = models.ModeExec
	cmd.TargetContainer = "my-container"

	record, err := svc.ExecuteCommand(context.Background(), cmd, map[string]interface{}{}, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	finalRecord := repo.getRecord(record.ID)
	if finalRecord == nil {
		t.Fatal("expected record to exist in repo")
	}
	if finalRecord.Status != models.StatusCompleted {
		t.Errorf("expected status 'completed', got %q", finalRecord.Status)
	}
	if finalRecord.Stdout != "exec output\n" {
		t.Errorf("expected stdout 'exec output\\n', got %q", finalRecord.Stdout)
	}
}

func TestExecuteCommand_ExecMode_NonZeroExitCode(t *testing.T) {
	repo := newMockRepo()

	outputCh := make(chan models.OutputChunk)
	close(outputCh)

	dockerMgr := &mockDockerManager{
		attachExecStreamFn: func(ctx context.Context, execID string) (<-chan models.OutputChunk, error) {
			return outputCh, nil
		},
		inspectExecFn: func(ctx context.Context, execID string) (*models.ExecInspectResult, error) {
			return &models.ExecInspectResult{ExitCode: 1, Running: false}, nil
		},
	}

	svc := NewExecutorService(repo, dockerMgr, &mockParameterValidator{}, nil, nil, nil)

	cmd := newTestCommand()
	cmd.ExecutionMode = models.ModeExec
	cmd.TargetContainer = "my-container"

	record, err := svc.ExecuteCommand(context.Background(), cmd, map[string]interface{}{}, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	finalRecord := repo.getRecord(record.ID)
	if finalRecord == nil {
		t.Fatal("expected record to exist in repo")
	}
	if finalRecord.Status != models.StatusFailed {
		t.Errorf("expected status 'failed', got %q", finalRecord.Status)
	}
	if finalRecord.ExitCode == nil || *finalRecord.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %v", finalRecord.ExitCode)
	}
}

func TestCancelExecution_RunningExecution(t *testing.T) {
	repo := newMockRepo()

	// Pre-populate a running execution.
	record := &models.ExecutionRecord{
		ID:        "exec-1",
		CommandID: "cmd-1",
		Status:    models.StatusRunning,
	}
	repo.records["exec-1"] = record

	stopCalled := false
	dockerMgr := &mockDockerManager{
		stopContainerFn: func(ctx context.Context, containerID string, timeout int) error {
			stopCalled = true
			return nil
		},
	}

	svc := &executorService{
		repo:      repo,
		dockerMgr: dockerMgr,
	}

	// Track a container for this execution.
	svc.runningContainers.Store("exec-1", "container-abc")

	err := svc.CancelExecution(context.Background(), "exec-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !stopCalled {
		t.Error("expected StopContainer to be called")
	}

	finalRecord := repo.getRecord("exec-1")
	if finalRecord.Status != models.StatusCancelled {
		t.Errorf("expected status 'cancelled', got %q", finalRecord.Status)
	}
}

func TestCancelExecution_NotFound(t *testing.T) {
	repo := newMockRepo()
	svc := &executorService{repo: repo, dockerMgr: &mockDockerManager{}}

	err := svc.CancelExecution(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent execution")
	}

	apiErr, ok := err.(*models.APIError)
	if !ok {
		t.Fatalf("expected *models.APIError, got %T", err)
	}
	if apiErr.Code != "not_found" {
		t.Errorf("expected code 'not_found', got %q", apiErr.Code)
	}
}

func TestCancelExecution_AlreadyCompleted(t *testing.T) {
	repo := newMockRepo()
	repo.records["exec-1"] = &models.ExecutionRecord{
		ID:     "exec-1",
		Status: models.StatusCompleted,
	}

	svc := &executorService{repo: repo, dockerMgr: &mockDockerManager{}}

	err := svc.CancelExecution(context.Background(), "exec-1")
	if err == nil {
		t.Fatal("expected error for completed execution")
	}

	apiErr, ok := err.(*models.APIError)
	if !ok {
		t.Fatalf("expected *models.APIError, got %T", err)
	}
	if apiErr.Code != "conflict" {
		t.Errorf("expected code 'conflict', got %q", apiErr.Code)
	}
}

func TestExecuteCommand_CreateMode_ContainerCreateFailure(t *testing.T) {
	repo := newMockRepo()

	dockerMgr := &mockDockerManager{
		createContainerFn: func(ctx context.Context, config models.ContainerConfig) (string, error) {
			return "", fmt.Errorf("image not found")
		},
	}

	svc := NewExecutorService(repo, dockerMgr, &mockParameterValidator{}, nil, nil, nil)

	cmd := newTestCommand()
	record, err := svc.ExecuteCommand(context.Background(), cmd, map[string]interface{}{}, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	finalRecord := repo.getRecord(record.ID)
	if finalRecord == nil {
		t.Fatal("expected record to exist in repo")
	}
	if finalRecord.Status != models.StatusFailed {
		t.Errorf("expected status 'failed', got %q", finalRecord.Status)
	}
}

func TestExecuteCommand_NilOptionalDependencies(t *testing.T) {
	// Verify the service works when StreamManager and NotificationService are nil.
	repo := newMockRepo()
	dockerMgr := &mockDockerManager{}

	svc := NewExecutorService(repo, dockerMgr, &mockParameterValidator{}, nil, nil, nil)

	cmd := newTestCommand()
	record, err := svc.ExecuteCommand(context.Background(), cmd, map[string]interface{}{}, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	finalRecord := repo.getRecord(record.ID)
	if finalRecord == nil {
		t.Fatal("expected record to exist in repo")
	}
	// Should complete without panicking on nil stream/notification.
	if finalRecord.Status != models.StatusCompleted {
		t.Errorf("expected status 'completed', got %q", finalRecord.Status)
	}
}

func TestCaptureOutput(t *testing.T) {
	streamMgr := newMockStreamManager()
	svc := &executorService{streamMgr: streamMgr}

	ch := make(chan models.OutputChunk, 3)
	ch <- models.OutputChunk{Stream: "stdout", Data: "line1\n"}
	ch <- models.OutputChunk{Stream: "stderr", Data: "err1\n"}
	ch <- models.OutputChunk{Stream: "stdout", Data: "line2\n"}
	close(ch)

	stdout, stderr := svc.captureOutput("exec-1", ch)

	if stdout != "line1\nline2\n" {
		t.Errorf("expected stdout 'line1\\nline2\\n', got %q", stdout)
	}
	if stderr != "err1\n" {
		t.Errorf("expected stderr 'err1\\n', got %q", stderr)
	}

	streamMgr.mu.Lock()
	if len(streamMgr.published) != 3 {
		t.Errorf("expected 3 published chunks, got %d", len(streamMgr.published))
	}
	streamMgr.mu.Unlock()
}
