package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kariz/kariz/internal/models"
)

// CommandRepository defines the data access interface for command entries.
type CommandRepository interface {
	Create(ctx context.Context, entry *models.CommandEntry, roles []models.Role) error
	GetByID(ctx context.Context, id string) (*models.CommandEntry, error)
	GetByName(ctx context.Context, name string) (*models.CommandEntry, error)
	Update(ctx context.Context, entry *models.CommandEntry, roles []models.Role) error
	Deactivate(ctx context.Context, id string) error
	List(ctx context.Context, filter models.CommandFilter) (*models.PaginatedResult, error)
}

// commandEntryRow is an internal struct used for scanning command_entries rows.
// It uses the JSONB-aware list types for volumes and artifacts columns.
type commandEntryRow struct {
	ID                  string                    `db:"id"`
	Name                string                    `db:"name"`
	Description         string                    `db:"description"`
	Category            string                    `db:"category"`
	DockerImage         string                    `db:"docker_image"`
	CommandString       string                    `db:"command_string"`
	ParameterSchema     models.ParameterSchema    `db:"parameter_schema"`
	ResourceLimits      models.ResourceLimits     `db:"resource_limits"`
	Volumes             models.VolumeMountList    `db:"volumes"`
	TimeoutSeconds      int                       `db:"timeout_seconds"`
	AllowConcurrent     bool                      `db:"allow_concurrent"`
	ExecutionMode       models.ExecutionMode      `db:"execution_mode"`
	TargetContainer     string                    `db:"target_container"`
	Artifacts           models.ArtifactDeclareList `db:"artifacts"`
	ArtifactDestination *models.ArtifactDestConfig `db:"artifact_destination"`
	IsActive            bool                      `db:"is_active"`
	Version             int                       `db:"version"`
	CreatedAt           time.Time                 `db:"created_at"`
	UpdatedAt           time.Time                 `db:"updated_at"`
}

// toCommandEntry converts a commandEntryRow to a models.CommandEntry.
func (r *commandEntryRow) toCommandEntry() *models.CommandEntry {
	return &models.CommandEntry{
		ID:                  r.ID,
		Name:                r.Name,
		Description:         r.Description,
		Category:            r.Category,
		DockerImage:         r.DockerImage,
		CommandString:       r.CommandString,
		ParameterSchema:     r.ParameterSchema,
		ResourceLimits:      r.ResourceLimits,
		Volumes:             []models.VolumeMount(r.Volumes),
		TimeoutSeconds:      r.TimeoutSeconds,
		AllowConcurrent:     r.AllowConcurrent,
		ExecutionMode:       r.ExecutionMode,
		TargetContainer:     r.TargetContainer,
		Artifacts:           []models.ArtifactDeclare(r.Artifacts),
		ArtifactDestination: r.ArtifactDestination,
		IsActive:            r.IsActive,
		Version:             r.Version,
		CreatedAt:           r.CreatedAt,
		UpdatedAt:           r.UpdatedAt,
	}
}

// commandRepository implements CommandRepository using sqlx.
type commandRepository struct {
	db *sqlx.DB
}

// NewCommandRepository creates a new CommandRepository backed by the given database.
func NewCommandRepository(db *sqlx.DB) CommandRepository {
	return &commandRepository{db: db}
}

// Create inserts a new command entry and its associated roles within a transaction.
func (r *commandRepository) Create(ctx context.Context, entry *models.CommandEntry, roles []models.Role) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	volList := models.VolumeMountList(entry.Volumes)
	artList := models.ArtifactDeclareList(entry.Artifacts)

	// Serialize JSONB fields manually to avoid sqlx named-parameter issues with pointer types.
	paramSchemaJSON, _ := entry.ParameterSchema.Value()
	resourceLimitsJSON, _ := entry.ResourceLimits.Value()
	volumesJSON, _ := volList.Value()
	artifactsJSON, _ := artList.Value()

	var artifactDestJSON interface{}
	if entry.ArtifactDestination != nil {
		artifactDestJSON, _ = entry.ArtifactDestination.Value()
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO command_entries (
			id, name, description, category, docker_image, command_string,
			parameter_schema, resource_limits, volumes, timeout_seconds,
			allow_concurrent, execution_mode, target_container, artifacts,
			artifact_destination, is_active, version, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10,
			$11, $12, $13, $14,
			$15, $16, $17, $18, $19
		)`,
		entry.ID, entry.Name, entry.Description, entry.Category,
		entry.DockerImage, entry.CommandString,
		paramSchemaJSON, resourceLimitsJSON, volumesJSON, entry.TimeoutSeconds,
		entry.AllowConcurrent, entry.ExecutionMode, entry.TargetContainer, artifactsJSON,
		artifactDestJSON, entry.IsActive, entry.Version, entry.CreatedAt, entry.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert command entry: %w", err)
	}

	if err := insertRoles(ctx, tx, entry.ID, roles); err != nil {
		return err
	}

	return tx.Commit()
}

// GetByID retrieves a command entry by its ID and populates its allowed roles.
func (r *commandRepository) GetByID(ctx context.Context, id string) (*models.CommandEntry, error) {
	var row commandEntryRow
	err := r.db.GetContext(ctx, &row,
		`SELECT id, name, description, category, docker_image, command_string,
			parameter_schema, resource_limits, volumes, timeout_seconds,
			allow_concurrent, execution_mode, target_container, artifacts,
			artifact_destination, is_active, version, created_at, updated_at
		FROM command_entries WHERE id = $1`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get command by id: %w", err)
	}

	entry := row.toCommandEntry()

	roles, err := r.getRoles(ctx, entry.ID)
	if err != nil {
		return nil, err
	}
	entry.AllowedRoles = roles

	return entry, nil
}

// GetByName retrieves a command entry by its unique name and populates its allowed roles.
func (r *commandRepository) GetByName(ctx context.Context, name string) (*models.CommandEntry, error) {
	var row commandEntryRow
	err := r.db.GetContext(ctx, &row,
		`SELECT id, name, description, category, docker_image, command_string,
			parameter_schema, resource_limits, volumes, timeout_seconds,
			allow_concurrent, execution_mode, target_container, artifacts,
			artifact_destination, is_active, version, created_at, updated_at
		FROM command_entries WHERE name = $1`, name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get command by name: %w", err)
	}

	entry := row.toCommandEntry()

	roles, err := r.getRoles(ctx, entry.ID)
	if err != nil {
		return nil, err
	}
	entry.AllowedRoles = roles

	return entry, nil
}

// Update modifies an existing command entry and replaces its roles within a transaction.
func (r *commandRepository) Update(ctx context.Context, entry *models.CommandEntry, roles []models.Role) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	volList := models.VolumeMountList(entry.Volumes)
	artList := models.ArtifactDeclareList(entry.Artifacts)

	paramSchemaJSON, _ := entry.ParameterSchema.Value()
	resourceLimitsJSON, _ := entry.ResourceLimits.Value()
	volumesJSON, _ := volList.Value()
	artifactsJSON, _ := artList.Value()

	var artifactDestJSON interface{}
	if entry.ArtifactDestination != nil {
		artifactDestJSON, _ = entry.ArtifactDestination.Value()
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE command_entries SET
			name = $1, description = $2, category = $3,
			docker_image = $4, command_string = $5,
			parameter_schema = $6, resource_limits = $7, volumes = $8,
			timeout_seconds = $9, allow_concurrent = $10,
			execution_mode = $11, target_container = $12,
			artifacts = $13, artifact_destination = $14,
			is_active = $15, version = $16, updated_at = $17
		WHERE id = $18`,
		entry.Name, entry.Description, entry.Category,
		entry.DockerImage, entry.CommandString,
		paramSchemaJSON, resourceLimitsJSON, volumesJSON,
		entry.TimeoutSeconds, entry.AllowConcurrent,
		entry.ExecutionMode, entry.TargetContainer,
		artifactsJSON, artifactDestJSON,
		entry.IsActive, entry.Version, entry.UpdatedAt,
		entry.ID,
	)
	if err != nil {
		return fmt.Errorf("update command entry: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("command entry not found: %s", entry.ID)
	}

	// Delete existing roles and insert the new set.
	_, err = tx.ExecContext(ctx, `DELETE FROM command_roles WHERE command_id = $1`, entry.ID)
	if err != nil {
		return fmt.Errorf("delete old command roles: %w", err)
	}

	if err := insertRoles(ctx, tx, entry.ID, roles); err != nil {
		return err
	}

	return tx.Commit()
}

// Deactivate sets is_active to false for the given command entry.
func (r *commandRepository) Deactivate(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE command_entries SET is_active = false, updated_at = $2 WHERE id = $1`,
		id, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("deactivate command: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("command entry not found: %s", id)
	}

	return nil
}

// List returns a paginated list of command entries with optional filtering by role,
// active status, category, and search term.
func (r *commandRepository) List(ctx context.Context, filter models.CommandFilter) (*models.PaginatedResult, error) {
	var (
		conditions []string
		args       []interface{}
		argIdx     = 1
	)

	// Base query parts
	selectFrom := `SELECT DISTINCT ce.id, ce.name, ce.description, ce.category,
		ce.docker_image, ce.command_string, ce.parameter_schema, ce.resource_limits,
		ce.volumes, ce.timeout_seconds, ce.allow_concurrent, ce.execution_mode,
		ce.target_container, ce.artifacts, ce.artifact_destination, ce.is_active,
		ce.version, ce.created_at, ce.updated_at
		FROM command_entries ce`

	countFrom := `SELECT COUNT(DISTINCT ce.id) FROM command_entries ce`

	joinClause := ""

	// Role filtering: join with command_roles
	if len(filter.Roles) > 0 {
		joinClause = ` JOIN command_roles cr ON ce.id = cr.command_id`
		placeholders := make([]string, len(filter.Roles))
		for i, role := range filter.Roles {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, string(role))
			argIdx++
		}
		conditions = append(conditions, fmt.Sprintf("cr.role IN (%s)", strings.Join(placeholders, ", ")))
	}

	// Active status filter
	if filter.IsActive != nil {
		conditions = append(conditions, fmt.Sprintf("ce.is_active = $%d", argIdx))
		args = append(args, *filter.IsActive)
		argIdx++
	}

	// Category filter
	if filter.Category != "" {
		conditions = append(conditions, fmt.Sprintf("ce.category = $%d", argIdx))
		args = append(args, filter.Category)
		argIdx++
	}

	// Search filter (ILIKE on name or description)
	if filter.Search != "" {
		searchPattern := "%" + filter.Search + "%"
		conditions = append(conditions, fmt.Sprintf("(ce.name ILIKE $%d OR ce.description ILIKE $%d)", argIdx, argIdx))
		args = append(args, searchPattern)
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total matching rows
	countQuery := countFrom + joinClause + whereClause
	var total int
	err := r.db.GetContext(ctx, &total, countQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("count commands: %w", err)
	}

	// Apply pagination
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
		fmt.Sprintf(" ORDER BY ce.name ASC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	dataArgs := append(args, pageSize, offset)

	var rows []commandEntryRow
	err = r.db.SelectContext(ctx, &rows, dataQuery, dataArgs...)
	if err != nil {
		return nil, fmt.Errorf("list commands: %w", err)
	}

	// Convert rows and load roles for each entry
	entries := make([]models.CommandEntry, 0, len(rows))
	for i := range rows {
		entry := rows[i].toCommandEntry()
		roles, err := r.getRoles(ctx, entry.ID)
		if err != nil {
			return nil, err
		}
		entry.AllowedRoles = roles
		entries = append(entries, *entry)
	}

	return &models.PaginatedResult{
		Items:    entries,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// getRoles fetches the allowed roles for a command entry.
func (r *commandRepository) getRoles(ctx context.Context, commandID string) ([]models.Role, error) {
	var roleStrings []string
	err := r.db.SelectContext(ctx, &roleStrings,
		`SELECT role FROM command_roles WHERE command_id = $1 ORDER BY role`, commandID)
	if err != nil {
		return nil, fmt.Errorf("get command roles: %w", err)
	}

	roles := make([]models.Role, len(roleStrings))
	for i, s := range roleStrings {
		roles[i] = models.Role(s)
	}
	return roles, nil
}

// insertRoles inserts role records for a command entry within the given transaction.
func insertRoles(ctx context.Context, tx *sqlx.Tx, commandID string, roles []models.Role) error {
	if len(roles) == 0 {
		return nil
	}

	for _, role := range roles {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO command_roles (command_id, role) VALUES ($1, $2)`,
			commandID, string(role))
		if err != nil {
			return fmt.Errorf("insert command role %s: %w", role, err)
		}
	}
	return nil
}
