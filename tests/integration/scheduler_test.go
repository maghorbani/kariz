package integration

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/kariz/kariz/internal/models"
	"github.com/kariz/kariz/internal/scheduler"
)

// --- In-memory ScheduleRepository ---

type inMemoryScheduleRepo struct {
	mu        sync.Mutex
	schedules map[string]*models.Schedule
}

func newInMemoryScheduleRepo() *inMemoryScheduleRepo {
	return &inMemoryScheduleRepo{
		schedules: make(map[string]*models.Schedule),
	}
}

func (r *inMemoryScheduleRepo) Create(ctx context.Context, s *models.Schedule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.schedules[s.ID] = s
	return nil
}

func (r *inMemoryScheduleRepo) Update(ctx context.Context, s *models.Schedule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.schedules[s.ID] = s
	return nil
}

func (r *inMemoryScheduleRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.schedules, id)
	return nil
}

func (r *inMemoryScheduleRepo) GetByID(ctx context.Context, id string) (*models.Schedule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.schedules[id]
	if !ok {
		return nil, nil
	}
	return s, nil
}

func (r *inMemoryScheduleRepo) List(ctx context.Context, filter models.ScheduleFilter) ([]models.Schedule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []models.Schedule
	for _, s := range r.schedules {
		result = append(result, *s)
	}
	return result, nil
}

func (r *inMemoryScheduleRepo) GetDueSchedules(ctx context.Context) ([]models.Schedule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	var result []models.Schedule
	for _, s := range r.schedules {
		if s.IsEnabled && s.NextRunAt != nil && !s.NextRunAt.After(now) {
			result = append(result, *s)
		}
	}
	return result, nil
}

func (r *inMemoryScheduleRepo) UpdateNextRunAt(ctx context.Context, id string, nextRunAt *time.Time, lastRunAt *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.schedules[id]
	if !ok {
		return nil
	}
	s.NextRunAt = nextRunAt
	s.LastRunAt = lastRunAt
	return nil
}

// --- Mock ExecutorService for scheduler ---

type mockSchedulerExecutor struct {
	mu         sync.Mutex
	executions []scheduledExecution
}

type scheduledExecution struct {
	CommandID string
	UserID    string
	Params    map[string]interface{}
}

func (m *mockSchedulerExecutor) ExecuteCommand(ctx context.Context, cmd models.CommandEntry, params map[string]interface{}, userID string) (*models.ExecutionRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executions = append(m.executions, scheduledExecution{
		CommandID: cmd.ID,
		UserID:    userID,
		Params:    params,
	})
	return &models.ExecutionRecord{
		ID:          "exec-sched-" + cmd.ID,
		CommandID:   cmd.ID,
		CommandName: cmd.Name,
		UserID:      userID,
		Status:      models.StatusQueued,
		CreatedAt:   time.Now().UTC(),
	}, nil
}

func (m *mockSchedulerExecutor) getExecutions() []scheduledExecution {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]scheduledExecution, len(m.executions))
	copy(result, m.executions)
	return result
}

// --- Mock CatalogService for scheduler ---

type mockCatalogService struct {
	commands map[string]*models.CommandEntry
}

func (m *mockCatalogService) GetCommand(ctx context.Context, id string) (*models.CommandEntry, error) {
	cmd, ok := m.commands[id]
	if !ok {
		return nil, &models.APIError{Code: "not_found", Message: "command not found"}
	}
	return cmd, nil
}

// TestScheduleCreateAndTrigger tests creating a schedule with a short interval
// and verifying it triggers an execution.
func TestScheduleCreateAndTrigger(t *testing.T) {
	schedRepo := newInMemoryScheduleRepo()
	mockExec := &mockSchedulerExecutor{}
	mockCatalog := &mockCatalogService{
		commands: map[string]*models.CommandEntry{
			"cmd-sched-1": {
				ID:              "cmd-sched-1",
				Name:            "scheduled-command",
				DockerImage:     "alpine:latest",
				CommandString:   "echo scheduled",
				ParameterSchema: models.ParameterSchema{},
				AllowedRoles:    []models.Role{models.RoleAdmin},
				TimeoutSeconds:  30,
				AllowConcurrent: true,
				ExecutionMode:   models.ModeCreate,
				IsActive:        true,
				Version:         1,
			},
		},
	}

	svc := scheduler.NewSchedulerService(schedRepo, mockExec, nil, mockCatalog)

	ctx := context.Background()

	// 1. Create a schedule with a very short interval (1 second).
	intervalSec := 1
	schedule, err := svc.CreateSchedule(ctx, models.CreateScheduleInput{
		CommandID:   "cmd-sched-1",
		IntervalSec: &intervalSec,
		Parameters:  json.RawMessage(`{"key": "value"}`),
	}, "user-sched-1", "scheduled-command")

	if err != nil {
		t.Fatalf("CreateSchedule failed: %v", err)
	}

	if schedule.ID == "" {
		t.Fatal("expected non-empty schedule ID")
	}
	if !schedule.IsEnabled {
		t.Error("expected schedule to be enabled by default")
	}
	if schedule.NextRunAt == nil {
		t.Fatal("expected next_run_at to be set")
	}

	// 2. Verify the schedule is stored.
	retrieved, err := svc.GetSchedule(ctx, schedule.ID)
	if err != nil {
		t.Fatalf("GetSchedule failed: %v", err)
	}
	if retrieved.CommandID != "cmd-sched-1" {
		t.Errorf("command_id = %q, want cmd-sched-1", retrieved.CommandID)
	}

	// 3. Wait for the interval to pass so the schedule becomes due.
	time.Sleep(2 * time.Second)

	// 4. Manually check for due schedules (simulating the scheduler loop).
	dueSchedules, err := schedRepo.GetDueSchedules(ctx)
	if err != nil {
		t.Fatalf("GetDueSchedules failed: %v", err)
	}
	if len(dueSchedules) == 0 {
		t.Fatal("expected at least 1 due schedule after interval elapsed")
	}

	// 5. Verify the due schedule matches our created schedule.
	found := false
	for _, ds := range dueSchedules {
		if ds.ID == schedule.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("created schedule not found in due schedules")
	}
}

// TestScheduleEnableDisable tests toggling a schedule's enabled state.
func TestScheduleEnableDisable(t *testing.T) {
	schedRepo := newInMemoryScheduleRepo()
	mockExec := &mockSchedulerExecutor{}
	mockCatalog := &mockCatalogService{
		commands: map[string]*models.CommandEntry{
			"cmd-toggle-1": {
				ID:              "cmd-toggle-1",
				Name:            "toggle-command",
				DockerImage:     "alpine:latest",
				CommandString:   "echo toggle",
				ParameterSchema: models.ParameterSchema{},
				AllowedRoles:    []models.Role{models.RoleAdmin},
				TimeoutSeconds:  30,
				AllowConcurrent: true,
				ExecutionMode:   models.ModeCreate,
				IsActive:        true,
				Version:         1,
			},
		},
	}

	svc := scheduler.NewSchedulerService(schedRepo, mockExec, nil, mockCatalog)
	ctx := context.Background()

	intervalSec := 60
	schedule, err := svc.CreateSchedule(ctx, models.CreateScheduleInput{
		CommandID:   "cmd-toggle-1",
		IntervalSec: &intervalSec,
	}, "user-toggle-1", "toggle-command")
	if err != nil {
		t.Fatalf("CreateSchedule failed: %v", err)
	}

	// Disable the schedule.
	if err := svc.DisableSchedule(ctx, schedule.ID); err != nil {
		t.Fatalf("DisableSchedule failed: %v", err)
	}

	disabled, err := svc.GetSchedule(ctx, schedule.ID)
	if err != nil {
		t.Fatalf("GetSchedule after disable failed: %v", err)
	}
	if disabled.IsEnabled {
		t.Error("expected schedule to be disabled")
	}

	// Enable the schedule.
	if err := svc.EnableSchedule(ctx, schedule.ID); err != nil {
		t.Fatalf("EnableSchedule failed: %v", err)
	}

	enabled, err := svc.GetSchedule(ctx, schedule.ID)
	if err != nil {
		t.Fatalf("GetSchedule after enable failed: %v", err)
	}
	if !enabled.IsEnabled {
		t.Error("expected schedule to be enabled")
	}
	if enabled.NextRunAt == nil {
		t.Error("expected next_run_at to be recomputed on enable")
	}
}

// TestScheduleDelete tests deleting a schedule.
func TestScheduleDelete(t *testing.T) {
	schedRepo := newInMemoryScheduleRepo()
	svc := scheduler.NewSchedulerService(schedRepo, &mockSchedulerExecutor{}, nil, &mockCatalogService{
		commands: map[string]*models.CommandEntry{
			"cmd-del-1": {
				ID:   "cmd-del-1",
				Name: "delete-command",
			},
		},
	})
	ctx := context.Background()

	intervalSec := 60
	schedule, err := svc.CreateSchedule(ctx, models.CreateScheduleInput{
		CommandID:   "cmd-del-1",
		IntervalSec: &intervalSec,
	}, "user-del-1", "delete-command")
	if err != nil {
		t.Fatalf("CreateSchedule failed: %v", err)
	}

	if err := svc.DeleteSchedule(ctx, schedule.ID); err != nil {
		t.Fatalf("DeleteSchedule failed: %v", err)
	}

	// Verify it's gone.
	_, err = svc.GetSchedule(ctx, schedule.ID)
	if err == nil {
		t.Error("expected error when getting deleted schedule")
	}
}
