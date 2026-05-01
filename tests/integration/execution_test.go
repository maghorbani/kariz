package integration

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/kariz/kariz/internal/executor"
	"github.com/kariz/kariz/internal/models"
	"github.com/kariz/kariz/internal/stream"
	"github.com/kariz/kariz/internal/validator"
)

// --- In-memory ExecutionRepository ---

type inMemoryExecutionRepo struct {
	mu      sync.Mutex
	records map[string]*models.ExecutionRecord
	running map[string]int
}

func newInMemoryExecutionRepo() *inMemoryExecutionRepo {
	return &inMemoryExecutionRepo{
		records: make(map[string]*models.ExecutionRecord),
		running: make(map[string]int),
	}
}

func (r *inMemoryExecutionRepo) Create(ctx context.Context, record *models.ExecutionRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records[record.ID] = record
	return nil
}

func (r *inMemoryExecutionRepo) Update(ctx context.Context, record *models.ExecutionRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records[record.ID] = record
	return nil
}

func (r *inMemoryExecutionRepo) GetByID(ctx context.Context, id string) (*models.ExecutionRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.records[id]
	if !ok {
		return nil, nil
	}
	return rec, nil
}

func (r *inMemoryExecutionRepo) List(ctx context.Context, filter models.ExecutionFilter) (*models.PaginatedResult, error) {
	return &models.PaginatedResult{Items: []models.ExecutionRecord{}, Total: 0, Page: 1, PageSize: 20}, nil
}

func (r *inMemoryExecutionRepo) CountRunning(ctx context.Context, commandID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running[commandID], nil
}

func (r *inMemoryExecutionRepo) getRecord(id string) *models.ExecutionRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.records[id]
}

// --- Mock DockerManager ---

type mockDockerManager struct {
	createContainerFn     func(ctx context.Context, config models.ContainerConfig) (string, error)
	startContainerFn      func(ctx context.Context, containerID string) error
	attachStreamFn        func(ctx context.Context, containerID string) (<-chan models.OutputChunk, error)
	stopContainerFn       func(ctx context.Context, containerID string, timeout int) error
	removeContainerFn     func(ctx context.Context, containerID string) error
	isAvailableFn         func(ctx context.Context) error
	execInContainerFn     func(ctx context.Context, containerID string, command []string) (string, error)
	attachExecStreamFn    func(ctx context.Context, execID string) (<-chan models.OutputChunk, error)
	inspectExecFn         func(ctx context.Context, execID string) (*models.ExecInspectResult, error)
	inspectContainerEnvFn func(ctx context.Context, containerNameOrID string) (map[string]string, error)
	copyFromContainerFn   func(ctx context.Context, containerID string, srcPath string) (io.ReadCloser, error)

	mu             sync.Mutex
	removedContIDs []string
}

func (m *mockDockerManager) CreateContainer(ctx context.Context, config models.ContainerConfig) (string, error) {
	if m.createContainerFn != nil {
		return m.createContainerFn(ctx, config)
	}
	return "mock-container-001", nil
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
	m.mu.Lock()
	m.removedContIDs = append(m.removedContIDs, containerID)
	m.mu.Unlock()
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
	return "exec-001", nil
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

// --- Mock NotificationService ---

type mockNotificationService struct {
	mu       sync.Mutex
	notified []models.ExecutionRecord
}

func (m *mockNotificationService) NotifyExecutionComplete(ctx context.Context, execution models.ExecutionRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notified = append(m.notified, execution)
	return nil
}

// --- Tests ---

// TestFullCommandExecutionLifecycle tests the end-to-end flow:
// Create command → Execute via ExecutorService → Verify StreamManager receives output
// → Wait for completion → Verify ExecutionRecord has correct status, stdout, stderr.
func TestFullCommandExecutionLifecycle(t *testing.T) {
	// 1. Set up in-memory repo, real StreamManager, real ParameterValidator.
	repo := newInMemoryExecutionRepo()
	streamMgr := stream.NewStreamManager()
	paramVal := validator.NewParameterValidator()
	notifySvc := &mockNotificationService{}

	// 2. Set up mock Docker that returns predefined output chunks.
	outputChunks := []models.OutputChunk{
		{Stream: "stdout", Data: "Starting process...\n", Timestamp: time.Now()},
		{Stream: "stdout", Data: "Processing data...\n", Timestamp: time.Now()},
		{Stream: "stderr", Data: "WARN: deprecated flag\n", Timestamp: time.Now()},
		{Stream: "stdout", Data: "Done.\n", Timestamp: time.Now()},
	}

	dockerMgr := &mockDockerManager{
		attachStreamFn: func(ctx context.Context, containerID string) (<-chan models.OutputChunk, error) {
			ch := make(chan models.OutputChunk, len(outputChunks))
			for _, chunk := range outputChunks {
				ch <- chunk
			}
			close(ch)
			return ch, nil
		},
	}

	// 3. Create ExecutorService with real service implementations.
	execSvc := executor.NewExecutorService(repo, dockerMgr, paramVal, streamMgr, notifySvc, nil)

	// 4. Define a command entry (simulating what CatalogService would create).
	cmd := models.CommandEntry{
		ID:              "cmd-lifecycle-1",
		Name:            "data-export",
		DockerImage:     "alpine:latest",
		CommandString:   "echo {{message}}",
		ParameterSchema: models.ParameterSchema{
			Parameters: []models.ParameterDefinition{
				{
					Name:     "message",
					Type:     models.ParamString,
					Required: true,
				},
			},
		},
		AllowedRoles:    []models.Role{models.RoleAdmin},
		TimeoutSeconds:  60,
		AllowConcurrent: true,
		ExecutionMode:   models.ModeCreate,
		IsActive:        true,
		Version:         1,
	}

	// 5. Subscribe to the stream before execution to capture all events.
	// We'll subscribe after execution starts since the stream is created on first publish.
	params := map[string]interface{}{"message": "hello"}

	// 6. Execute the command.
	record, err := execSvc.ExecuteCommand(context.Background(), cmd, params, "user-integration-1")
	if err != nil {
		t.Fatalf("ExecuteCommand failed: %v", err)
	}

	if record.Status != models.StatusQueued {
		t.Errorf("initial status = %q, want queued", record.Status)
	}
	if record.ID == "" {
		t.Fatal("expected non-empty execution ID")
	}

	// 7. Subscribe to SSE stream to verify events are received.
	eventCh, unsub := streamMgr.Subscribe(record.ID, "")
	defer unsub()

	// 8. Collect all SSE events.
	var events []models.SSEEvent
	done := make(chan struct{})
	go func() {
		defer close(done)
		for evt := range eventCh {
			events = append(events, evt)
		}
	}()

	// 9. Wait for the background goroutine to complete.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for execution to complete")
	}

	// 10. Verify the final execution record.
	finalRecord := repo.getRecord(record.ID)
	if finalRecord == nil {
		t.Fatal("expected execution record in repo")
	}

	if finalRecord.Status != models.StatusCompleted {
		t.Errorf("final status = %q, want completed", finalRecord.Status)
	}

	expectedStdout := "Starting process...\nProcessing data...\nDone.\n"
	if finalRecord.Stdout != expectedStdout {
		t.Errorf("stdout = %q, want %q", finalRecord.Stdout, expectedStdout)
	}

	expectedStderr := "WARN: deprecated flag\n"
	if finalRecord.Stderr != expectedStderr {
		t.Errorf("stderr = %q, want %q", finalRecord.Stderr, expectedStderr)
	}

	if finalRecord.StartedAt == nil {
		t.Error("expected StartedAt to be set")
	}
	if finalRecord.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
	if finalRecord.ContainerID == nil || *finalRecord.ContainerID != "mock-container-001" {
		t.Errorf("container_id = %v, want mock-container-001", finalRecord.ContainerID)
	}

	// 11. Verify SSE events: should have 4 output events + 1 complete event.
	if len(events) < 4 {
		t.Errorf("expected at least 4 SSE events (output chunks), got %d", len(events))
	}

	// Check that the last event is a completion event.
	lastEvent := events[len(events)-1]
	if lastEvent.Event != "complete" {
		t.Errorf("last event type = %q, want complete", lastEvent.Event)
	}

	// 12. Verify notification was sent.
	notifySvc.mu.Lock()
	if len(notifySvc.notified) != 1 {
		t.Errorf("expected 1 notification, got %d", len(notifySvc.notified))
	}
	notifySvc.mu.Unlock()

	// 13. Verify container was cleaned up (removed).
	dockerMgr.mu.Lock()
	if len(dockerMgr.removedContIDs) == 0 {
		t.Error("expected container to be removed after execution")
	}
	dockerMgr.mu.Unlock()
}

// TestFullCommandExecutionLifecycle_FailedExecution tests that a Docker failure
// results in a failed execution record with appropriate error in stderr.
func TestFullCommandExecutionLifecycle_FailedExecution(t *testing.T) {
	repo := newInMemoryExecutionRepo()
	streamMgr := stream.NewStreamManager()
	paramVal := validator.NewParameterValidator()

	dockerMgr := &mockDockerManager{
		createContainerFn: func(ctx context.Context, config models.ContainerConfig) (string, error) {
			return "", fmt.Errorf("image not found: alpine:nonexistent")
		},
	}

	execSvc := executor.NewExecutorService(repo, dockerMgr, paramVal, streamMgr, nil, nil)

	cmd := models.CommandEntry{
		ID:              "cmd-fail-1",
		Name:            "failing-command",
		DockerImage:     "alpine:nonexistent",
		CommandString:   "echo test",
		ParameterSchema: models.ParameterSchema{},
		AllowedRoles:    []models.Role{models.RoleAdmin},
		TimeoutSeconds:  30,
		AllowConcurrent: true,
		ExecutionMode:   models.ModeCreate,
		IsActive:        true,
		Version:         1,
	}

	record, err := execSvc.ExecuteCommand(context.Background(), cmd, map[string]interface{}{}, "user-1")
	if err != nil {
		t.Fatalf("ExecuteCommand failed: %v", err)
	}

	// Wait for background goroutine.
	time.Sleep(1 * time.Second)

	finalRecord := repo.getRecord(record.ID)
	if finalRecord == nil {
		t.Fatal("expected execution record in repo")
	}

	if finalRecord.Status != models.StatusFailed {
		t.Errorf("final status = %q, want failed", finalRecord.Status)
	}

	if finalRecord.Stderr == "" {
		t.Error("expected stderr to contain error message")
	}
}
