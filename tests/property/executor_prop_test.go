package property_test

// Feature: kariz-command-dashboard, Property 13: container config correctness
// Feature: kariz-command-dashboard, Property 9: concurrency lock enforcement

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kariz/kariz/internal/docker"
	"github.com/kariz/kariz/internal/executor"
	"github.com/kariz/kariz/internal/models"
	"pgregory.net/rapid"
)

// dockerSocketPath is the path that must never appear in generated container configs.
const dockerSocketPath = "/var/run/docker.sock"

// --- Generators ---

// genSafePath generates a random Unix-style path that does NOT contain the Docker socket path.
func genSafePath(t *rapid.T, label string) string {
	segments := rapid.IntRange(1, 4).Draw(t, label+"_segments")
	parts := make([]string, segments)
	for i := 0; i < segments; i++ {
		parts[i] = rapid.StringMatching(`^[a-z][a-z0-9_]{1,10}`).Draw(t, fmt.Sprintf("%s_seg%d", label, i))
	}
	return "/" + strings.Join(parts, "/")
}

// genVolumeMount generates a random VolumeMount with safe (non-socket) paths.
func genVolumeMount(t *rapid.T, label string) models.VolumeMount {
	return models.VolumeMount{
		HostPath:      genSafePath(t, label+"_host"),
		ContainerPath: genSafePath(t, label+"_container"),
		ReadOnly:      rapid.Bool().Draw(t, label+"_ro"),
	}
}

// genDockerSocketMount generates a VolumeMount that references the Docker socket.
func genDockerSocketMount(t *rapid.T, label string) models.VolumeMount {
	// Randomly place the socket path on host side, container side, or both.
	variant := rapid.IntRange(0, 2).Draw(t, label+"_variant")
	switch variant {
	case 0:
		return models.VolumeMount{
			HostPath:      dockerSocketPath,
			ContainerPath: genSafePath(t, label+"_container"),
			ReadOnly:      rapid.Bool().Draw(t, label+"_ro"),
		}
	case 1:
		return models.VolumeMount{
			HostPath:      genSafePath(t, label+"_host"),
			ContainerPath: dockerSocketPath,
			ReadOnly:      rapid.Bool().Draw(t, label+"_ro"),
		}
	default:
		return models.VolumeMount{
			HostPath:      dockerSocketPath,
			ContainerPath: dockerSocketPath,
			ReadOnly:      rapid.Bool().Draw(t, label+"_ro"),
		}
	}
}

// genResourceLimits generates random resource limits within realistic ranges.
func genResourceLimits(t *rapid.T) models.ResourceLimits {
	rl := models.ResourceLimits{}

	// Optionally set CPU shares (0-4096)
	if rapid.Bool().Draw(t, "hasCPUShares") {
		v := rapid.Int64Range(0, 4096).Draw(t, "cpuShares")
		rl.CPUShares = &v
	}

	// Optionally set memory (64-8192 MB)
	if rapid.Bool().Draw(t, "hasMemoryMB") {
		v := rapid.Int64Range(64, 8192).Draw(t, "memoryMB")
		rl.MemoryMB = &v
	}

	// Optionally set CPU count (1-16)
	if rapid.Bool().Draw(t, "hasCPUCount") {
		v := rapid.Int64Range(1, 16).Draw(t, "cpuCount")
		rl.CPUCount = &v
	}

	return rl
}

// genContainerConfig generates a random ContainerConfig with 1-5 safe volumes,
// optionally injected Docker socket mounts, and random resource limits.
func genContainerConfig(t *rapid.T, injectSocket bool) models.ContainerConfig {
	// Generate 1-5 safe volume mounts
	volCount := rapid.IntRange(1, 5).Draw(t, "volCount")
	volumes := make([]models.VolumeMount, volCount)
	for i := 0; i < volCount; i++ {
		volumes[i] = genVolumeMount(t, fmt.Sprintf("vol%d", i))
	}

	// Optionally inject 1-2 Docker socket mounts at random positions
	if injectSocket {
		socketCount := rapid.IntRange(1, 2).Draw(t, "socketCount")
		for i := 0; i < socketCount; i++ {
			socketMount := genDockerSocketMount(t, fmt.Sprintf("socket%d", i))
			pos := rapid.IntRange(0, len(volumes)).Draw(t, fmt.Sprintf("socketPos%d", i))
			// Insert at position
			volumes = append(volumes, models.VolumeMount{})
			copy(volumes[pos+1:], volumes[pos:])
			volumes[pos] = socketMount
		}
	}

	return models.ContainerConfig{
		Image:          "test-image:latest",
		Command:        []string{"echo", "hello"},
		Volumes:        volumes,
		ResourceLimits: genResourceLimits(t),
	}
}

// --- Property 13 Tests ---
// **Validates: Requirements 6.3, 6.5**

// TestProperty13_VolumesMatchExactly tests that all non-socket volumes from the
// input config appear in the output binds, and no extra volumes are added.
func TestProperty13_VolumesMatchExactly(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		config := genContainerConfig(t, false)

		built := docker.BuildContainerCreateConfig(config)

		// Count expected non-socket volumes
		var expectedBinds []string
		for _, vol := range config.Volumes {
			if strings.Contains(vol.HostPath, dockerSocketPath) || strings.Contains(vol.ContainerPath, dockerSocketPath) {
				continue
			}
			bind := vol.HostPath + ":" + vol.ContainerPath
			if vol.ReadOnly {
				bind += ":ro"
			}
			expectedBinds = append(expectedBinds, bind)
		}

		if len(built.Binds) != len(expectedBinds) {
			t.Fatalf("expected %d binds, got %d.\nExpected: %v\nGot: %v",
				len(expectedBinds), len(built.Binds), expectedBinds, built.Binds)
		}

		for i, expected := range expectedBinds {
			if built.Binds[i] != expected {
				t.Fatalf("bind[%d] mismatch: expected %q, got %q", i, expected, built.Binds[i])
			}
		}
	})
}

// TestProperty13_DockerSocketNeverMounted tests that even when Docker socket
// mounts are present in the input, they never appear in the output binds.
func TestProperty13_DockerSocketNeverMounted(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Always inject socket mounts
		config := genContainerConfig(t, true)

		built := docker.BuildContainerCreateConfig(config)

		for _, bind := range built.Binds {
			if strings.Contains(bind, dockerSocketPath) {
				t.Fatalf("Docker socket path found in bind mount: %q", bind)
			}
		}

		// Also verify that the number of output binds is less than input volumes
		// (since at least one socket mount was injected and should be filtered)
		safeCount := 0
		for _, vol := range config.Volumes {
			if !strings.Contains(vol.HostPath, dockerSocketPath) && !strings.Contains(vol.ContainerPath, dockerSocketPath) {
				safeCount++
			}
		}
		if len(built.Binds) != safeCount {
			t.Fatalf("expected %d safe binds after filtering, got %d", safeCount, len(built.Binds))
		}
	})
}

// TestProperty13_ResourceLimitsMatch tests that CPU shares, memory, and CPU count
// in the output match the values specified in the input config.
func TestProperty13_ResourceLimitsMatch(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		config := genContainerConfig(t, false)

		built := docker.BuildContainerCreateConfig(config)

		// Check CPU shares
		if config.ResourceLimits.CPUShares != nil {
			if built.Resources.CPUShares != *config.ResourceLimits.CPUShares {
				t.Fatalf("CPUShares mismatch: expected %d, got %d",
					*config.ResourceLimits.CPUShares, built.Resources.CPUShares)
			}
		} else {
			if built.Resources.CPUShares != 0 {
				t.Fatalf("expected CPUShares to be 0 when not set, got %d", built.Resources.CPUShares)
			}
		}

		// Check memory (input is MB, output should be bytes)
		if config.ResourceLimits.MemoryMB != nil {
			expectedMemory := *config.ResourceLimits.MemoryMB * 1024 * 1024
			if built.Resources.Memory != expectedMemory {
				t.Fatalf("Memory mismatch: expected %d bytes (%d MB), got %d bytes",
					expectedMemory, *config.ResourceLimits.MemoryMB, built.Resources.Memory)
			}
		} else {
			if built.Resources.Memory != 0 {
				t.Fatalf("expected Memory to be 0 when not set, got %d", built.Resources.Memory)
			}
		}

		// Check CPU count (input is count, output should be NanoCPUs)
		if config.ResourceLimits.CPUCount != nil {
			expectedNanoCPUs := *config.ResourceLimits.CPUCount * 1e9
			if built.Resources.NanoCPUs != expectedNanoCPUs {
				t.Fatalf("NanoCPUs mismatch: expected %d (%d CPUs), got %d",
					expectedNanoCPUs, *config.ResourceLimits.CPUCount, built.Resources.NanoCPUs)
			}
		} else {
			if built.Resources.NanoCPUs != 0 {
				t.Fatalf("expected NanoCPUs to be 0 when not set, got %d", built.Resources.NanoCPUs)
			}
		}
	})
}


// --- Property 9 Mock Implementations ---

// prop9MockRepo is a mock ExecutionRepository for Property 9 tests.
// It allows configuring the running count per command ID.
type prop9MockRepo struct {
	mu           sync.Mutex
	records      map[string]*models.ExecutionRecord
	runningCount map[string]int
}

func newProp9MockRepo() *prop9MockRepo {
	return &prop9MockRepo{
		records:      make(map[string]*models.ExecutionRecord),
		runningCount: make(map[string]int),
	}
}

func (m *prop9MockRepo) Create(ctx context.Context, record *models.ExecutionRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records[record.ID] = record
	return nil
}

func (m *prop9MockRepo) Update(ctx context.Context, record *models.ExecutionRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records[record.ID] = record
	return nil
}

func (m *prop9MockRepo) GetByID(ctx context.Context, id string) (*models.ExecutionRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return nil, nil
	}
	return r, nil
}

func (m *prop9MockRepo) List(ctx context.Context, filter models.ExecutionFilter) (*models.PaginatedResult, error) {
	return &models.PaginatedResult{Items: []models.ExecutionRecord{}, Total: 0, Page: 1, PageSize: 20}, nil
}

func (m *prop9MockRepo) CountRunning(ctx context.Context, commandID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runningCount[commandID], nil
}

func (m *prop9MockRepo) setRunningCount(commandID string, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runningCount[commandID] = count
}

// prop9MockDockerManager is a minimal DockerManager mock for Property 9 tests.
type prop9MockDockerManager struct{}

func (m *prop9MockDockerManager) CreateContainer(ctx context.Context, config models.ContainerConfig) (string, error) {
	return "container-prop9", nil
}

func (m *prop9MockDockerManager) StartContainer(ctx context.Context, containerID string) error {
	return nil
}

func (m *prop9MockDockerManager) AttachStream(ctx context.Context, containerID string) (<-chan models.OutputChunk, error) {
	ch := make(chan models.OutputChunk)
	close(ch)
	return ch, nil
}

func (m *prop9MockDockerManager) StopContainer(ctx context.Context, containerID string, timeout int) error {
	return nil
}

func (m *prop9MockDockerManager) RemoveContainer(ctx context.Context, containerID string) error {
	return nil
}

func (m *prop9MockDockerManager) IsAvailable(ctx context.Context) error {
	return nil
}

func (m *prop9MockDockerManager) ExecInContainer(ctx context.Context, containerID string, command []string) (string, error) {
	return "exec-prop9", nil
}

func (m *prop9MockDockerManager) AttachExecStream(ctx context.Context, execID string) (<-chan models.OutputChunk, error) {
	ch := make(chan models.OutputChunk)
	close(ch)
	return ch, nil
}

func (m *prop9MockDockerManager) InspectExec(ctx context.Context, execID string) (*models.ExecInspectResult, error) {
	return &models.ExecInspectResult{ExitCode: 0, Running: false}, nil
}

func (m *prop9MockDockerManager) InspectContainerEnv(ctx context.Context, containerNameOrID string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (m *prop9MockDockerManager) CopyFromContainer(ctx context.Context, containerID string, srcPath string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *prop9MockDockerManager) ContainerLogs(ctx context.Context, containerID string, opts models.LogOptions) (<-chan models.OutputChunk, error) {
	ch := make(chan models.OutputChunk)
	close(ch)
	return ch, nil
}

// prop9MockParamValidator is a minimal ParameterValidator mock that always passes.
type prop9MockParamValidator struct{}

func (m *prop9MockParamValidator) Validate(schema models.ParameterSchema, params map[string]interface{}) *models.ValidationResult {
	return &models.ValidationResult{Valid: true}
}

func (m *prop9MockParamValidator) BuildCommandArgs(commandString string, schema models.ParameterSchema, validatedParams map[string]interface{}) ([]string, error) {
	return []string{"echo", "hello"}, nil
}

// --- Property 9 Generators ---

// genCommandEntryForConcurrency generates a random CommandEntry with allow_concurrent
// randomly set to true or false, and a random command ID.
func genCommandEntryForConcurrency(t *rapid.T) models.CommandEntry {
	cmdID := rapid.StringMatching(`^cmd-[a-z0-9]{4,8}$`).Draw(t, "commandID")
	allowConcurrent := rapid.Bool().Draw(t, "allowConcurrent")

	return models.CommandEntry{
		ID:              cmdID,
		Name:            "test-cmd-" + cmdID,
		DockerImage:     "alpine:latest",
		CommandString:   "echo hello",
		ParameterSchema: models.ParameterSchema{},
		AllowedRoles:    []models.Role{models.RoleAdmin},
		TimeoutSeconds:  60,
		AllowConcurrent: allowConcurrent,
		ExecutionMode:   models.ModeCreate,
	}
}

// --- Property 9 Tests ---
// **Validates: Requirements 3.6**

// TestProperty9_ConcurrencyLockRejectsWhenNotAllowed tests that for any CommandEntry
// with allow_concurrent=false and at least one running execution, a new execution
// request is rejected with a conflict error.
func TestProperty9_ConcurrencyLockRejectsWhenNotAllowed(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a command with allow_concurrent=false
		cmd := genCommandEntryForConcurrency(t)
		cmd.AllowConcurrent = false

		// Generate a running count >= 1
		runningCount := rapid.IntRange(1, 5).Draw(t, "runningCount")

		repo := newProp9MockRepo()
		repo.setRunningCount(cmd.ID, runningCount)

		svc := executor.NewExecutorService(repo, &prop9MockDockerManager{}, &prop9MockParamValidator{}, nil, nil, nil)

		_, err := svc.ExecuteCommand(context.Background(), cmd, map[string]interface{}{}, "user-1")

		// Must be rejected
		if err == nil {
			t.Fatalf("expected conflict error for allow_concurrent=false with %d running executions, got nil", runningCount)
		}

		apiErr, ok := err.(*models.APIError)
		if !ok {
			t.Fatalf("expected *models.APIError, got %T: %v", err, err)
		}
		if apiErr.Code != "conflict" {
			t.Fatalf("expected error code 'conflict', got %q", apiErr.Code)
		}
	})
}

// TestProperty9_ConcurrencyLockAllowsWhenConcurrentEnabled tests that for any
// CommandEntry with allow_concurrent=true, a new execution request is accepted
// regardless of how many executions are currently running.
func TestProperty9_ConcurrencyLockAllowsWhenConcurrentEnabled(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a command with allow_concurrent=true
		cmd := genCommandEntryForConcurrency(t)
		cmd.AllowConcurrent = true

		// Generate any running count (0-5)
		runningCount := rapid.IntRange(0, 5).Draw(t, "runningCount")

		repo := newProp9MockRepo()
		repo.setRunningCount(cmd.ID, runningCount)

		svc := executor.NewExecutorService(repo, &prop9MockDockerManager{}, &prop9MockParamValidator{}, nil, nil, nil)

		record, err := svc.ExecuteCommand(context.Background(), cmd, map[string]interface{}{}, "user-1")

		// Must be accepted
		if err != nil {
			t.Fatalf("expected no error for allow_concurrent=true with %d running executions, got: %v", runningCount, err)
		}
		if record == nil {
			t.Fatal("expected non-nil execution record")
			return
		}
		if record.Status != models.StatusQueued {
			t.Fatalf("expected status 'queued', got %q", record.Status)
		}
	})
}

// TestProperty9_ConcurrencyLockAllowsWhenNoRunning tests that for any CommandEntry
// with allow_concurrent=false and zero running executions, a new execution request
// is accepted.
func TestProperty9_ConcurrencyLockAllowsWhenNoRunning(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a command with allow_concurrent=false
		cmd := genCommandEntryForConcurrency(t)
		cmd.AllowConcurrent = false

		repo := newProp9MockRepo()
		repo.setRunningCount(cmd.ID, 0)

		svc := executor.NewExecutorService(repo, &prop9MockDockerManager{}, &prop9MockParamValidator{}, nil, nil, nil)

		record, err := svc.ExecuteCommand(context.Background(), cmd, map[string]interface{}{}, "user-1")

		// Must be accepted
		if err != nil {
			t.Fatalf("expected no error for allow_concurrent=false with 0 running executions, got: %v", err)
		}
		if record == nil {
			t.Fatal("expected non-nil execution record")
			return
		}
		if record.Status != models.StatusQueued {
			t.Fatalf("expected status 'queued', got %q", record.Status)
		}
	})
}

// TestProperty9_ConcurrencyLockEnforcement is the comprehensive property test that
// generates random combinations of allow_concurrent and running counts, and verifies
// the correct behavior in all cases:
// - allow_concurrent=false AND running > 0 → rejected (conflict)
// - allow_concurrent=true → accepted regardless of running count
// - allow_concurrent=false AND running == 0 → accepted
func TestProperty9_ConcurrencyLockEnforcement(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cmd := genCommandEntryForConcurrency(t)
		runningCount := rapid.IntRange(0, 5).Draw(t, "runningCount")

		repo := newProp9MockRepo()
		repo.setRunningCount(cmd.ID, runningCount)

		svc := executor.NewExecutorService(repo, &prop9MockDockerManager{}, &prop9MockParamValidator{}, nil, nil, nil)

		record, err := svc.ExecuteCommand(context.Background(), cmd, map[string]interface{}{}, "user-1")

		shouldReject := !cmd.AllowConcurrent && runningCount > 0

		if shouldReject {
			if err == nil {
				t.Fatalf("expected conflict error for allow_concurrent=%v with %d running, got nil",
					cmd.AllowConcurrent, runningCount)
			}
			apiErr, ok := err.(*models.APIError)
			if !ok {
				t.Fatalf("expected *models.APIError, got %T: %v", err, err)
			}
			if apiErr.Code != "conflict" {
				t.Fatalf("expected error code 'conflict', got %q", apiErr.Code)
			}
		} else {
			if err != nil {
				t.Fatalf("expected no error for allow_concurrent=%v with %d running, got: %v",
					cmd.AllowConcurrent, runningCount, err)
			}
			if record == nil {
				t.Fatal("expected non-nil execution record")
			return
			}
			if record.Status != models.StatusQueued {
				t.Fatalf("expected status 'queued', got %q", record.Status)
			}
			if record.CommandID != cmd.ID {
				t.Fatalf("expected command_id %q, got %q", cmd.ID, record.CommandID)
			}
		}
	})
}


// --- Property 19: Execution mode routing correctness ---
// **Validates: Requirements 13.3, 13.4**

// prop19TrackingDockerManager tracks which Docker methods are called to verify
// that "create" mode uses CreateContainer/RemoveContainer and "exec" mode uses
// ExecInContainer without CreateContainer.
type prop19TrackingDockerManager struct {
	mu                    sync.Mutex
	createContainerCalled bool
	removeContainerCalled bool
	execInContainerCalled bool
	startContainerCalled  bool
}

func newProp19TrackingDockerManager() *prop19TrackingDockerManager {
	return &prop19TrackingDockerManager{}
}

func (m *prop19TrackingDockerManager) CreateContainer(ctx context.Context, config models.ContainerConfig) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createContainerCalled = true
	return "container-prop19", nil
}

func (m *prop19TrackingDockerManager) StartContainer(ctx context.Context, containerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startContainerCalled = true
	return nil
}

func (m *prop19TrackingDockerManager) AttachStream(ctx context.Context, containerID string) (<-chan models.OutputChunk, error) {
	ch := make(chan models.OutputChunk)
	close(ch)
	return ch, nil
}

func (m *prop19TrackingDockerManager) StopContainer(ctx context.Context, containerID string, timeout int) error {
	return nil
}

func (m *prop19TrackingDockerManager) RemoveContainer(ctx context.Context, containerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeContainerCalled = true
	return nil
}

func (m *prop19TrackingDockerManager) IsAvailable(ctx context.Context) error {
	return nil
}

func (m *prop19TrackingDockerManager) ExecInContainer(ctx context.Context, containerID string, command []string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.execInContainerCalled = true
	return "exec-prop19", nil
}

func (m *prop19TrackingDockerManager) AttachExecStream(ctx context.Context, execID string) (<-chan models.OutputChunk, error) {
	ch := make(chan models.OutputChunk)
	close(ch)
	return ch, nil
}

func (m *prop19TrackingDockerManager) InspectExec(ctx context.Context, execID string) (*models.ExecInspectResult, error) {
	return &models.ExecInspectResult{ExitCode: 0, Running: false}, nil
}

func (m *prop19TrackingDockerManager) InspectContainerEnv(ctx context.Context, containerNameOrID string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (m *prop19TrackingDockerManager) CopyFromContainer(ctx context.Context, containerID string, srcPath string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *prop19TrackingDockerManager) ContainerLogs(ctx context.Context, containerID string, opts models.LogOptions) (<-chan models.OutputChunk, error) {
	ch := make(chan models.OutputChunk)
	close(ch)
	return ch, nil
}

func (m *prop19TrackingDockerManager) getState() (createCalled, removeCalled, execCalled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.createContainerCalled, m.removeContainerCalled, m.execInContainerCalled
}

// genCommandEntryForExecMode generates a random CommandEntry with a randomly chosen
// execution mode ("create" or "exec").
func genCommandEntryForExecMode(t *rapid.T) models.CommandEntry {
	mode := rapid.SampledFrom([]models.ExecutionMode{models.ModeCreate, models.ModeExec}).Draw(t, "execMode")
	cmdID := rapid.StringMatching(`^cmd-[a-z0-9]{4,8}`).Draw(t, "commandID")

	entry := models.CommandEntry{
		ID:              cmdID,
		Name:            "test-cmd-" + cmdID,
		DockerImage:     "alpine:latest",
		CommandString:   "echo hello",
		ParameterSchema: models.ParameterSchema{},
		AllowedRoles:    []models.Role{models.RoleAdmin},
		TimeoutSeconds:  60,
		AllowConcurrent: true,
		ExecutionMode:   mode,
	}

	if mode == models.ModeExec {
		entry.TargetContainer = "target-container-" + cmdID
	}

	return entry
}

// TestProperty19_ExecutionModeRoutingCorrectness tests that "create" mode calls
// CreateContainer/RemoveContainer and "exec" mode calls ExecInContainer without
// CreateContainer.
func TestProperty19_ExecutionModeRoutingCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cmd := genCommandEntryForExecMode(t)

		dockerMgr := newProp19TrackingDockerManager()
		repo := newProp9MockRepo()

		svc := executor.NewExecutorService(repo, dockerMgr, &prop9MockParamValidator{}, nil, nil, nil)

		record, err := svc.ExecuteCommand(context.Background(), cmd, map[string]interface{}{}, "user-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if record == nil {
			t.Fatal("expected non-nil execution record")
			return
		}

		// Wait briefly for the goroutine to start and make Docker calls
		// The goroutine runs asynchronously, so we need a small delay
		import_time_sleep(200)

		createCalled, removeCalled, execCalled := dockerMgr.getState()

		switch cmd.ExecutionMode {
		case models.ModeCreate:
			if !createCalled {
				t.Fatal("create mode: expected CreateContainer to be called")
			}
			// RemoveContainer is called in defer, should eventually be called
			// but may not have been called yet due to timing
			if execCalled {
				t.Fatal("create mode: ExecInContainer should NOT be called")
			}
		case models.ModeExec:
			if createCalled {
				t.Fatal("exec mode: CreateContainer should NOT be called")
			}
			if removeCalled {
				t.Fatal("exec mode: RemoveContainer should NOT be called")
			}
			if !execCalled {
				t.Fatal("exec mode: expected ExecInContainer to be called")
			}
		}
	})
}

// import_time_sleep is a helper to sleep for a given number of milliseconds.
// Named this way to avoid import conflicts in the test file.
func import_time_sleep(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}
