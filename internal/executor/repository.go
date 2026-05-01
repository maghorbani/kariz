package executor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kariz/kariz/internal/models"
)

// ExecutionRepository defines the data access interface for execution records.
type ExecutionRepository interface {
	Create(ctx context.Context, record *models.ExecutionRecord) error
	Update(ctx context.Context, record *models.ExecutionRecord) error
	GetByID(ctx context.Context, id string) (*models.ExecutionRecord, error)
	List(ctx context.Context, filter models.ExecutionFilter) (*models.PaginatedResult, error)
	CountRunning(ctx context.Context, commandID string) (int, error)
}

// executionRepository implements ExecutionRepository using sqlx.
type executionRepository struct {
	db *sqlx.DB
}

// NewExecutionRepository creates a new ExecutionRepository backed by the given database.
func NewExecutionRepository(db *sqlx.DB) ExecutionRepository {
	return &executionRepository{db: db}
}

// Create inserts a new execution record into the database.
func (r *executionRepository) Create(ctx context.Context, record *models.ExecutionRecord) error {
	params := record.Parameters
	if params == nil {
		params = json.RawMessage("{}")
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO execution_records (
			id, command_id, user_id, schedule_id, command_name, parameters,
			status, exit_code, stdout, stderr, container_id,
			started_at, completed_at, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11,
			$12, $13, $14
		)`,
		record.ID, record.CommandID, record.UserID, record.ScheduleID,
		record.CommandName, params,
		record.Status, record.ExitCode, record.Stdout, record.Stderr, record.ContainerID,
		record.StartedAt, record.CompletedAt, record.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert execution record: %w", err)
	}

	return nil
}

// Update modifies an existing execution record's mutable fields.
func (r *executionRepository) Update(ctx context.Context, record *models.ExecutionRecord) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE execution_records SET
			status = $2,
			exit_code = $3,
			stdout = $4,
			stderr = $5,
			container_id = $6,
			started_at = $7,
			completed_at = $8
		WHERE id = $1`,
		record.ID, record.Status, record.ExitCode,
		record.Stdout, record.Stderr, record.ContainerID,
		record.StartedAt, record.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("update execution record: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("execution record not found: %s", record.ID)
	}

	return nil
}

// GetByID retrieves an execution record by its ID. Returns nil, nil if not found.
func (r *executionRepository) GetByID(ctx context.Context, id string) (*models.ExecutionRecord, error) {
	var record models.ExecutionRecord
	err := r.db.GetContext(ctx, &record, `
		SELECT id, command_id, user_id, schedule_id, command_name, parameters,
			status, exit_code, stdout, stderr, container_id,
			started_at, completed_at, created_at
		FROM execution_records WHERE id = $1`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get execution record by id: %w", err)
	}

	return &record, nil
}

// List returns a paginated list of execution records with optional filtering.
// Results are ordered by created_at descending (most recent first).
func (r *executionRepository) List(ctx context.Context, filter models.ExecutionFilter) (*models.PaginatedResult, error) {
	var (
		conditions []string
		args       []interface{}
		argIdx     = 1
	)

	// command_name ILIKE filter
	if filter.CommandName != "" {
		conditions = append(conditions, fmt.Sprintf("command_name ILIKE $%d", argIdx))
		args = append(args, "%"+filter.CommandName+"%")
		argIdx++
	}

	// user_id exact match
	if filter.UserID != "" {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", argIdx))
		args = append(args, filter.UserID)
		argIdx++
	}

	// date_from range on created_at
	if filter.DateFrom != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, *filter.DateFrom)
		argIdx++
	}

	// date_to range on created_at
	if filter.DateTo != nil {
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", argIdx))
		args = append(args, *filter.DateTo)
		argIdx++
	}

	// status exact match
	if filter.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, string(filter.Status))
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total matching rows
	countQuery := "SELECT COUNT(*) FROM execution_records" + whereClause
	var total int
	err := r.db.GetContext(ctx, &total, countQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("count execution records: %w", err)
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

	dataQuery := fmt.Sprintf(`
		SELECT id, command_id, user_id, schedule_id, command_name, parameters,
			status, exit_code, stdout, stderr, container_id,
			started_at, completed_at, created_at
		FROM execution_records%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)

	dataArgs := append(args, pageSize, offset)

	var records []models.ExecutionRecord
	err = r.db.SelectContext(ctx, &records, dataQuery, dataArgs...)
	if err != nil {
		return nil, fmt.Errorf("list execution records: %w", err)
	}

	if records == nil {
		records = []models.ExecutionRecord{}
	}

	return &models.PaginatedResult{
		Items:    records,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// CountRunning returns the number of currently running executions for a given command.
// Used by the executor for concurrency lock checking.
func (r *executionRepository) CountRunning(ctx context.Context, commandID string) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM execution_records WHERE command_id = $1 AND status = 'running'`,
		commandID)
	if err != nil {
		return 0, fmt.Errorf("count running executions: %w", err)
	}

	return count, nil
}
