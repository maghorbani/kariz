package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kariz/kariz/internal/models"
	"github.com/robfig/cron/v3"
)

// ExecutorService is the subset of the executor interface needed by the scheduler.
type ExecutorService interface {
	ExecuteCommand(ctx context.Context, commandEntry models.CommandEntry, params map[string]interface{}, userID string) (*models.ExecutionRecord, error)
}

// NotificationService is the subset of the notification interface needed by the scheduler.
type NotificationService interface {
	NotifyExecutionComplete(ctx context.Context, execution models.ExecutionRecord) error
}

// CatalogService is the subset of the catalog interface needed by the scheduler.
type CatalogService interface {
	GetCommand(ctx context.Context, id string) (*models.CommandEntry, error)
}

// SchedulerService manages command schedules and triggers automatic executions.
type SchedulerService interface {
	CreateSchedule(ctx context.Context, input models.CreateScheduleInput, userID string, commandName string) (*models.Schedule, error)
	UpdateSchedule(ctx context.Context, id string, input models.UpdateScheduleInput) (*models.Schedule, error)
	DeleteSchedule(ctx context.Context, id string) error
	EnableSchedule(ctx context.Context, id string) error
	DisableSchedule(ctx context.Context, id string) error
	ListSchedules(ctx context.Context, filter models.ScheduleFilter) ([]models.Schedule, error)
	GetSchedule(ctx context.Context, id string) (*models.Schedule, error)
	Start(ctx context.Context) error
	Stop() error
}

// schedulerService is the concrete implementation of SchedulerService.
type schedulerService struct {
	repo       ScheduleRepository
	executorSvc ExecutorService
	notifySvc   NotificationService
	catalogSvc  CatalogService
	cronParser cron.Parser

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewSchedulerService creates a new SchedulerService with the given dependencies.
func NewSchedulerService(
	repo ScheduleRepository,
	executorSvc ExecutorService,
	notifySvc NotificationService,
	catalogSvc CatalogService,
) SchedulerService {
	return &schedulerService{
		repo:        repo,
		executorSvc: executorSvc,
		notifySvc:   notifySvc,
		catalogSvc:  catalogSvc,
		cronParser:  cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
	}
}

// CreateSchedule validates the cron expression, computes the initial next_run_at,
// and stores the schedule in the database.
func (s *schedulerService) CreateSchedule(ctx context.Context, input models.CreateScheduleInput, userID string, commandName string) (*models.Schedule, error) {
	// Validate: must have either cron expression or interval
	if input.CronExpression == "" && input.IntervalSec == nil {
		return nil, &models.APIError{
			Code:    "validation_error",
			Message: "either cron_expression or interval_seconds is required",
		}
	}

	// Validate cron expression if provided
	var nextRun *time.Time
	now := time.Now().UTC()

	if input.CronExpression != "" {
		sched, err := s.cronParser.Parse(input.CronExpression)
		if err != nil {
			return nil, &models.APIError{
				Code:    "validation_error",
				Message: fmt.Sprintf("invalid cron expression: %v", err),
			}
		}
		next := sched.Next(now)
		nextRun = &next
	} else if input.IntervalSec != nil {
		if *input.IntervalSec <= 0 {
			return nil, &models.APIError{
				Code:    "validation_error",
				Message: "interval_seconds must be positive",
			}
		}
		next := now.Add(time.Duration(*input.IntervalSec) * time.Second)
		nextRun = &next
	}

	params := input.Parameters
	if params == nil {
		params = json.RawMessage("{}")
	}

	schedule := &models.Schedule{
		ID:             uuid.New().String(),
		CommandID:      input.CommandID,
		CommandName:    commandName,
		CreatedByUser:  userID,
		CronExpression: input.CronExpression,
		IntervalSec:    input.IntervalSec,
		Parameters:     params,
		IsEnabled:      true,
		NextRunAt:      nextRun,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.repo.Create(ctx, schedule); err != nil {
		return nil, fmt.Errorf("create schedule: %w", err)
	}

	return schedule, nil
}

// UpdateSchedule updates the cron expression or interval and recomputes next_run_at.
func (s *schedulerService) UpdateSchedule(ctx context.Context, id string, input models.UpdateScheduleInput) (*models.Schedule, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get schedule for update: %w", err)
	}
	if existing == nil {
		return nil, &models.APIError{
			Code:    "not_found",
			Message: "schedule not found",
		}
	}

	// Apply updates
	if input.CronExpression != nil {
		if *input.CronExpression != "" {
			// Validate new cron expression
			if _, err := s.cronParser.Parse(*input.CronExpression); err != nil {
				return nil, &models.APIError{
					Code:    "validation_error",
					Message: fmt.Sprintf("invalid cron expression: %v", err),
				}
			}
		}
		existing.CronExpression = *input.CronExpression
	}

	if input.IntervalSec != nil {
		if *input.IntervalSec <= 0 {
			return nil, &models.APIError{
				Code:    "validation_error",
				Message: "interval_seconds must be positive",
			}
		}
		existing.IntervalSec = input.IntervalSec
	}

	if input.Parameters != nil {
		existing.Parameters = *input.Parameters
	}

	existing.UpdatedAt = time.Now().UTC()

	// Recompute next_run_at if schedule is enabled
	if existing.IsEnabled {
		nextRun := s.computeNextRun(existing)
		existing.NextRunAt = nextRun
	}

	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("update schedule: %w", err)
	}

	// Also update next_run_at in the DB
	if err := s.repo.UpdateNextRunAt(ctx, existing.ID, existing.NextRunAt, existing.LastRunAt); err != nil {
		slog.Error("failed to update next_run_at after schedule update", "schedule_id", id, "error", err)
	}

	return existing, nil
}

// EnableSchedule activates a schedule and recomputes next_run_at.
func (s *schedulerService) EnableSchedule(ctx context.Context, id string) error {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get schedule for enable: %w", err)
	}
	if existing == nil {
		return &models.APIError{
			Code:    "not_found",
			Message: "schedule not found",
		}
	}

	existing.IsEnabled = true
	existing.UpdatedAt = time.Now().UTC()

	// Recompute next_run_at
	nextRun := s.computeNextRun(existing)
	existing.NextRunAt = nextRun

	if err := s.repo.UpdateNextRunAt(ctx, id, nextRun, existing.LastRunAt); err != nil {
		return fmt.Errorf("update next_run_at on enable: %w", err)
	}

	// Update is_enabled in the DB via a direct query
	return s.setEnabled(ctx, id, true, nextRun)
}

// DisableSchedule deactivates a schedule without deleting it.
func (s *schedulerService) DisableSchedule(ctx context.Context, id string) error {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get schedule for disable: %w", err)
	}
	if existing == nil {
		return &models.APIError{
			Code:    "not_found",
			Message: "schedule not found",
		}
	}

	return s.setEnabled(ctx, id, false, nil)
}

// DeleteSchedule removes a schedule from the database.
func (s *schedulerService) DeleteSchedule(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete schedule: %w", err)
	}
	return nil
}

// ListSchedules returns schedules matching the given filter.
func (s *schedulerService) ListSchedules(ctx context.Context, filter models.ScheduleFilter) ([]models.Schedule, error) {
	schedules, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	return schedules, nil
}

// GetSchedule returns a single schedule by ID.
func (s *schedulerService) GetSchedule(ctx context.Context, id string) (*models.Schedule, error) {
	schedule, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get schedule: %w", err)
	}
	if schedule == nil {
		return nil, &models.APIError{
			Code:    "not_found",
			Message: "schedule not found",
		}
	}
	return schedule, nil
}

// Start begins the background scheduler loop that polls for due schedules every 60 seconds.
func (s *schedulerService) Start(ctx context.Context) error {
	s.stopCh = make(chan struct{})
	s.wg.Add(1)

	go s.runLoop(ctx)

	slog.Info("scheduler service started")
	return nil
}

// Stop gracefully shuts down the scheduler.
func (s *schedulerService) Stop() error {
	if s.stopCh != nil {
		close(s.stopCh)
		s.wg.Wait()
	}
	slog.Info("scheduler service stopped")
	return nil
}

// runLoop is the background goroutine that polls for due schedules every 60 seconds.
func (s *schedulerService) runLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Run immediately on start
	s.processDueSchedules(ctx)

	for {
		select {
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processDueSchedules(ctx)
		}
	}
}

// processDueSchedules queries for due schedules and triggers execution for each.
func (s *schedulerService) processDueSchedules(ctx context.Context) {
	queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	schedules, err := s.repo.GetDueSchedules(queryCtx)
	if err != nil {
		slog.Error("failed to get due schedules", "error", err)
		return
	}

	for _, schedule := range schedules {
		s.triggerSchedule(ctx, schedule)
	}
}

// triggerSchedule executes a command for a due schedule and updates timing.
func (s *schedulerService) triggerSchedule(ctx context.Context, schedule models.Schedule) {
	slog.Info("triggering scheduled execution",
		"schedule_id", schedule.ID,
		"command_id", schedule.CommandID,
		"command_name", schedule.CommandName,
	)

	// Get the command entry from the catalog
	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd, err := s.catalogSvc.GetCommand(execCtx, schedule.CommandID)
	if err != nil {
		slog.Error("failed to get command for scheduled execution",
			"schedule_id", schedule.ID,
			"command_id", schedule.CommandID,
			"error", err,
		)
		return
	}

	// Parse parameters from the schedule
	var params map[string]interface{}
	if len(schedule.Parameters) > 0 {
		if err := json.Unmarshal(schedule.Parameters, &params); err != nil {
			slog.Error("failed to unmarshal schedule parameters",
				"schedule_id", schedule.ID,
				"error", err,
			)
			params = make(map[string]interface{})
		}
	} else {
		params = make(map[string]interface{})
	}

	// Execute the command as the schedule creator
	record, err := s.executorSvc.ExecuteCommand(execCtx, *cmd, params, schedule.CreatedByUser)
	if err != nil {
		slog.Error("scheduled execution failed",
			"schedule_id", schedule.ID,
			"command_id", schedule.CommandID,
			"error", err,
		)

		// Notify the schedule creator about the failure
		s.notifyScheduleFailure(ctx, schedule, err)
		// Still update timing even on failure
	} else {
		slog.Info("scheduled execution started",
			"schedule_id", schedule.ID,
			"execution_id", record.ID,
		)
	}

	// Update last_run_at and compute next next_run_at
	now := time.Now().UTC()
	nextRun := s.computeNextRun(&schedule)

	if err := s.repo.UpdateNextRunAt(ctx, schedule.ID, nextRun, &now); err != nil {
		slog.Error("failed to update schedule timing",
			"schedule_id", schedule.ID,
			"error", err,
		)
	}
}

// notifyScheduleFailure sends a notification to the schedule creator when a scheduled
// execution fails to start.
func (s *schedulerService) notifyScheduleFailure(ctx context.Context, schedule models.Schedule, execErr error) {
	if s.notifySvc == nil {
		return
	}

	// Create a synthetic execution record for the notification
	failedRecord := models.ExecutionRecord{
		ID:          uuid.New().String(),
		CommandID:   schedule.CommandID,
		CommandName: schedule.CommandName,
		UserID:      schedule.CreatedByUser,
		Status:      models.StatusFailed,
		Stderr:      fmt.Sprintf("Scheduled execution failed to start: %v", execErr),
		ScheduleID:  &schedule.ID,
	}

	notifyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := s.notifySvc.NotifyExecutionComplete(notifyCtx, failedRecord); err != nil {
		slog.Error("failed to notify schedule failure",
			"schedule_id", schedule.ID,
			"error", err,
		)
	}
}

// computeNextRun calculates the next run time for a schedule based on its cron expression
// or interval.
func (s *schedulerService) computeNextRun(schedule *models.Schedule) *time.Time {
	now := time.Now().UTC()

	if schedule.CronExpression != "" {
		sched, err := s.cronParser.Parse(schedule.CronExpression)
		if err != nil {
			slog.Error("failed to parse cron expression for next run",
				"schedule_id", schedule.ID,
				"cron", schedule.CronExpression,
				"error", err,
			)
			return nil
		}
		next := sched.Next(now)
		return &next
	}

	if schedule.IntervalSec != nil && *schedule.IntervalSec > 0 {
		next := now.Add(time.Duration(*schedule.IntervalSec) * time.Second)
		return &next
	}

	return nil
}

// setEnabled updates the is_enabled flag and next_run_at for a schedule.
// This is done via the repository's UpdateNextRunAt plus a direct update.
func (s *schedulerService) setEnabled(ctx context.Context, id string, enabled bool, nextRunAt *time.Time) error {
	schedule, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get schedule: %w", err)
	}
	if schedule == nil {
		return &models.APIError{
			Code:    "not_found",
			Message: "schedule not found",
		}
	}

	schedule.IsEnabled = enabled
	schedule.NextRunAt = nextRunAt
	schedule.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, schedule); err != nil {
		return fmt.Errorf("update schedule enabled state: %w", err)
	}

	if err := s.repo.UpdateNextRunAt(ctx, id, nextRunAt, schedule.LastRunAt); err != nil {
		return fmt.Errorf("update next_run_at: %w", err)
	}

	return nil
}
