package artifact

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kariz/kariz/internal/models"
)

// ExecutionArtifactRepository defines the data access interface for execution artifacts.
type ExecutionArtifactRepository interface {
	Create(ctx context.Context, artifact *models.ExecutionArtifact) error
	GetByExecutionID(ctx context.Context, executionID string) ([]models.ExecutionArtifact, error)
	GetByID(ctx context.Context, id string) (*models.ExecutionArtifact, error)
	Delete(ctx context.Context, id string) error
}

// executionArtifactRepository implements ExecutionArtifactRepository using sqlx.
type executionArtifactRepository struct {
	db *sqlx.DB
}

// NewExecutionArtifactRepository creates a new ExecutionArtifactRepository backed by the given database.
func NewExecutionArtifactRepository(db *sqlx.DB) ExecutionArtifactRepository {
	return &executionArtifactRepository{db: db}
}

// Create inserts a new execution artifact into the execution_artifacts table.
func (r *executionArtifactRepository) Create(ctx context.Context, artifact *models.ExecutionArtifact) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO execution_artifacts (
			id, execution_id, label, file_name, file_size_bytes,
			content_type, storage_path, storage_type, status,
			error_message, created_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11
		)`,
		artifact.ID, artifact.ExecutionID, artifact.Label,
		artifact.FileName, artifact.FileSizeBytes,
		artifact.ContentType, artifact.StoragePath, artifact.StorageType,
		artifact.Status, artifact.ErrorMessage, artifact.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert execution artifact: %w", err)
	}
	return nil
}

// GetByExecutionID retrieves all artifacts for a given execution, ordered by created_at ascending.
// Returns an empty slice if no artifacts are found.
func (r *executionArtifactRepository) GetByExecutionID(ctx context.Context, executionID string) ([]models.ExecutionArtifact, error) {
	var artifacts []models.ExecutionArtifact
	err := r.db.SelectContext(ctx, &artifacts, `
		SELECT id, execution_id, label, file_name, file_size_bytes,
			content_type, storage_path, storage_type, status,
			error_message, created_at
		FROM execution_artifacts
		WHERE execution_id = $1
		ORDER BY created_at ASC`, executionID)
	if err != nil {
		return nil, fmt.Errorf("get artifacts by execution id: %w", err)
	}

	if artifacts == nil {
		artifacts = []models.ExecutionArtifact{}
	}

	return artifacts, nil
}

// GetByID retrieves a single artifact by its ID. Returns nil, nil if not found.
func (r *executionArtifactRepository) GetByID(ctx context.Context, id string) (*models.ExecutionArtifact, error) {
	var artifact models.ExecutionArtifact
	err := r.db.GetContext(ctx, &artifact, `
		SELECT id, execution_id, label, file_name, file_size_bytes,
			content_type, storage_path, storage_type, status,
			error_message, created_at
		FROM execution_artifacts
		WHERE id = $1`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get artifact by id: %w", err)
	}

	return &artifact, nil
}

// Delete removes an artifact by its ID. Returns an error if the artifact is not found.
func (r *executionArtifactRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM execution_artifacts WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete artifact: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("artifact not found: %w", sql.ErrNoRows)
	}

	return nil
}
