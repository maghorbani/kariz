package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kariz/kariz/internal/models"
)

// ScheduleRepository defines the data access interface for schedules.
type ScheduleRepository interface {
	Create(ctx context.Context, schedule *models.Schedule) error
	Update(ctx context.Context, schedule *models.Schedule) error
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*models.Schedule, error)
	List(ctx context.Context, filter models.ScheduleFilter) ([]models.Schedule, error)
	GetDueSchedules(ctx context.Context) ([]models.Schedule, error)
	UpdateNextRunAt(ctx context.Context, id string, nextRunAt *time.Time, lastRunAt *time.Time) error
}

// scheduleRepository implements ScheduleRepository using sqlx.
type scheduleRepository struct {
	db *sqlx.DB
}

// NewScheduleRepository creates a new ScheduleRepository backed by the given database.
func NewScheduleRepository(db *sqlx.DB) ScheduleRepository {
	return &scheduleRepository{db: db}
}

// Create inserts a new schedule into the schedules table.
func (r *scheduleRepository) Create(ctx context.Context, schedule *models.Schedule) error {
	params := schedule.Parameters
	if params == nil {
		params = json.RawMessage("{}")
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO schedules (
			id, command_id, created_by_user, command_name,
			cron_expression, interval_seconds, parameters,
			is_enabled, next_run_at, last_run_at,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9, $10,
			$11, $12
		)`,
		schedule.ID, schedule.CommandID, schedule.CreatedByUser, schedule.CommandName,
		schedule.CronExpression, schedule.IntervalSec, params,
		schedule.IsEnabled, schedule.NextRunAt, schedule.LastRunAt,
		schedule.CreatedAt, schedule.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert schedule: %w", err)
	}

	return nil
}

// Update modifies an existing schedule's mutable fields.
func (r *scheduleRepository) Update(ctx context.Context, schedule *models.Schedule) error {
	params := schedule.Parameters
	if params == nil {
		params = json.RawMessage("{}")
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE schedules SET
			cron_expression = $2,
			interval_seconds = $3,
			parameters = $4,
			updated_at = $5
		WHERE id = $1`,
		schedule.ID, schedule.CronExpression, schedule.IntervalSec,
		params, schedule.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update schedule: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("schedule not found: %s", schedule.ID)
	}

	return nil
}

// Delete removes a schedule from the database.
func (r *scheduleRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM schedules WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete schedule: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("schedule not found: %w", sql.ErrNoRows)
	}

	return nil
}

// GetByID retrieves a schedule by its ID. Returns nil, nil if not found.
func (r *scheduleRepository) GetByID(ctx context.Context, id string) (*models.Schedule, error) {
	var schedule models.Schedule
	err := r.db.GetContext(ctx, &schedule, `
		SELECT id, command_id, created_by_user, command_name,
			cron_expression, interval_seconds, parameters,
			is_enabled, next_run_at, last_run_at,
			created_at, updated_at
		FROM schedules WHERE id = $1`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get schedule by id: %w", err)
	}

	return &schedule, nil
}

// List returns a list of schedules with optional filtering by command_id, is_enabled,
// and roles. Results are ordered by created_at descending with pagination.
func (r *scheduleRepository) List(ctx context.Context, filter models.ScheduleFilter) ([]models.Schedule, error) {
	var (
		conditions []string
		args       []interface{}
		argIdx     = 1
	)

	selectFrom := `SELECT DISTINCT s.id, s.command_id, s.created_by_user, s.command_name,
		s.cron_expression, s.interval_seconds, s.parameters,
		s.is_enabled, s.next_run_at, s.last_run_at,
		s.created_at, s.updated_at
		FROM schedules s`

	joinClause := ""

	// Role filtering: join with command_roles on the schedule's command_id
	if len(filter.Roles) > 0 {
		joinClause = ` JOIN command_roles cr ON s.command_id = cr.command_id`
		placeholders := make([]string, len(filter.Roles))
		for i, role := range filter.Roles {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, string(role))
			argIdx++
		}
		conditions = append(conditions, fmt.Sprintf("cr.role IN (%s)", strings.Join(placeholders, ", ")))
	}

	// Command ID filter
	if filter.CommandID != "" {
		conditions = append(conditions, fmt.Sprintf("s.command_id = $%d", argIdx))
		args = append(args, filter.CommandID)
		argIdx++
	}

	// Enabled filter
	if filter.IsEnabled != nil {
		conditions = append(conditions, fmt.Sprintf("s.is_enabled = $%d", argIdx))
		args = append(args, *filter.IsEnabled)
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	// Apply pagination defaults
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	dataQuery := selectFrom + joinClause + whereClause +
		fmt.Sprintf(" ORDER BY s.created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	dataArgs := append(args, pageSize, offset)

	var schedules []models.Schedule
	err := r.db.SelectContext(ctx, &schedules, dataQuery, dataArgs...)
	if err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}

	if schedules == nil {
		schedules = []models.Schedule{}
	}

	return schedules, nil
}

// GetDueSchedules returns all enabled schedules whose next_run_at is at or before the current time.
func (r *scheduleRepository) GetDueSchedules(ctx context.Context) ([]models.Schedule, error) {
	var schedules []models.Schedule
	err := r.db.SelectContext(ctx, &schedules, `
		SELECT id, command_id, created_by_user, command_name,
			cron_expression, interval_seconds, parameters,
			is_enabled, next_run_at, last_run_at,
			created_at, updated_at
		FROM schedules
		WHERE is_enabled = true AND next_run_at <= NOW()`)
	if err != nil {
		return nil, fmt.Errorf("get due schedules: %w", err)
	}

	if schedules == nil {
		schedules = []models.Schedule{}
	}

	return schedules, nil
}

// UpdateNextRunAt updates the next_run_at and last_run_at timestamps for a schedule.
func (r *scheduleRepository) UpdateNextRunAt(ctx context.Context, id string, nextRunAt *time.Time, lastRunAt *time.Time) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE schedules SET
			next_run_at = $2,
			last_run_at = $3,
			updated_at = NOW()
		WHERE id = $1`,
		id, nextRunAt, lastRunAt,
	)
	if err != nil {
		return fmt.Errorf("update schedule next run at: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("schedule not found: %s", id)
	}

	return nil
}
