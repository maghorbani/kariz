package catalog

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kariz/kariz/internal/models"
)

// CatalogService defines the interface for command catalog operations.
type CatalogService interface {
	CreateCommand(ctx context.Context, input models.CreateCommandInput) (*models.CommandEntry, error)
	UpdateCommand(ctx context.Context, id string, input models.UpdateCommandInput) (*models.CommandEntry, error)
	DeactivateCommand(ctx context.Context, id string) error
	GetCommand(ctx context.Context, id string) (*models.CommandEntry, error)
	ListCommands(ctx context.Context, filter models.CommandFilter) (*models.PaginatedResult, error)
	SearchCommands(ctx context.Context, query string, userRoles []models.Role) ([]models.CommandEntry, error)
}

// catalogService implements CatalogService using a CommandRepository.
type catalogService struct {
	repo CommandRepository
}

// NewCatalogService creates a new CatalogService backed by the given repository.
func NewCatalogService(repo CommandRepository) CatalogService {
	return &catalogService{repo: repo}
}

// CreateCommand validates the input, checks name uniqueness, and stores a new command entry
// with a generated UUID and version=1.
func (s *catalogService) CreateCommand(ctx context.Context, input models.CreateCommandInput) (*models.CommandEntry, error) {
	// Validate required fields
	if strings.TrimSpace(input.Name) == "" {
		return nil, &models.APIError{
			Code:    "validation_error",
			Message: "name is required",
		}
	}
	if len(input.AllowedRoles) == 0 {
		return nil, &models.APIError{
			Code:    "validation_error",
			Message: "at least one allowed role is required",
		}
	}

	// Check name uniqueness
	existing, err := s.repo.GetByName(ctx, input.Name)
	if err != nil {
		return nil, fmt.Errorf("check name uniqueness: %w", err)
	}
	if existing != nil {
		return nil, &models.APIError{
			Code:    "conflict",
			Message: fmt.Sprintf("command with name %q already exists", input.Name),
		}
	}

	// Default execution_mode to "create" if empty
	execMode := input.ExecutionMode
	if execMode == "" {
		execMode = models.ModeCreate
	}

	logOpts := input.LogOptions
	if execMode == models.ModeLogs && logOpts == nil {
		logOpts = models.DefaultLogOptions()
	}

	if err := validateCommandFields(
		execMode, input.CommandString, input.DockerImage,
		input.TargetContainer, logOpts, input.TimeoutSeconds,
	); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	entry := &models.CommandEntry{
		ID:                  uuid.New().String(),
		Name:                input.Name,
		Description:         input.Description,
		Category:            input.Category,
		DockerImage:         input.DockerImage,
		CommandString:       input.CommandString,
		ParameterSchema:     input.ParameterSchema,
		AllowedRoles:        input.AllowedRoles,
		ResourceLimits:      input.ResourceLimits,
		Volumes:             input.Volumes,
		TimeoutSeconds:      input.TimeoutSeconds,
		AllowConcurrent:     input.AllowConcurrent,
		ExecutionMode:       execMode,
		TargetContainer:     input.TargetContainer,
		LogOptions:          logOpts,
		Artifacts:           input.Artifacts,
		ArtifactDestination: input.ArtifactDestination,
		IsActive:            true,
		Version:             1,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if err := s.repo.Create(ctx, entry, input.AllowedRoles); err != nil {
		return nil, fmt.Errorf("create command: %w", err)
	}

	return entry, nil
}

// UpdateCommand retrieves the existing command, applies partial updates from the input,
// increments the version, and persists the changes.
func (s *catalogService) UpdateCommand(ctx context.Context, id string, input models.UpdateCommandInput) (*models.CommandEntry, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get command for update: %w", err)
	}
	if existing == nil {
		return nil, &models.APIError{
			Code:    "not_found",
			Message: "command not found",
		}
	}

	// Apply partial updates
	if input.Name != nil {
		if strings.TrimSpace(*input.Name) == "" {
			return nil, &models.APIError{
				Code:    "validation_error",
				Message: "name cannot be empty",
			}
		}
		// Check uniqueness if name is changing
		if *input.Name != existing.Name {
			dup, err := s.repo.GetByName(ctx, *input.Name)
			if err != nil {
				return nil, fmt.Errorf("check name uniqueness: %w", err)
			}
			if dup != nil {
				return nil, &models.APIError{
					Code:    "conflict",
					Message: fmt.Sprintf("command with name %q already exists", *input.Name),
				}
			}
		}
		existing.Name = *input.Name
	}
	if input.Description != nil {
		existing.Description = *input.Description
	}
	if input.Category != nil {
		existing.Category = *input.Category
	}
	if input.DockerImage != nil {
		existing.DockerImage = *input.DockerImage
	}
	if input.CommandString != nil {
		existing.CommandString = *input.CommandString
	}
	if input.ParameterSchema != nil {
		existing.ParameterSchema = *input.ParameterSchema
	}
	if input.AllowedRoles != nil {
		if len(input.AllowedRoles) == 0 {
			return nil, &models.APIError{
				Code:    "validation_error",
				Message: "at least one allowed role is required",
			}
		}
		existing.AllowedRoles = input.AllowedRoles
	}
	if input.ResourceLimits != nil {
		existing.ResourceLimits = *input.ResourceLimits
	}
	if input.Volumes != nil {
		existing.Volumes = input.Volumes
	}
	if input.TimeoutSeconds != nil {
		existing.TimeoutSeconds = *input.TimeoutSeconds
	}
	if input.AllowConcurrent != nil {
		existing.AllowConcurrent = *input.AllowConcurrent
	}
	if input.ExecutionMode != nil {
		existing.ExecutionMode = *input.ExecutionMode
	}
	if input.TargetContainer != nil {
		existing.TargetContainer = *input.TargetContainer
	}
	if input.Artifacts != nil {
		existing.Artifacts = input.Artifacts
	}
	if input.ArtifactDestination != nil {
		existing.ArtifactDestination = input.ArtifactDestination
	}
	if input.LogOptions != nil {
		existing.LogOptions = input.LogOptions
	}

	execMode := existing.ExecutionMode
	if input.ExecutionMode != nil {
		execMode = *input.ExecutionMode
	}
	if err := validateCommandFields(
		execMode, existing.CommandString, existing.DockerImage,
		existing.TargetContainer, existing.LogOptions, existing.TimeoutSeconds,
	); err != nil {
		return nil, err
	}

	// Increment version and update timestamp
	existing.Version++
	existing.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, existing, existing.AllowedRoles); err != nil {
		return nil, fmt.Errorf("update command: %w", err)
	}

	return existing, nil
}

// DeactivateCommand sets is_active=false for the given command.
func (s *catalogService) DeactivateCommand(ctx context.Context, id string) error {
	if err := s.repo.Deactivate(ctx, id); err != nil {
		return fmt.Errorf("deactivate command: %w", err)
	}
	return nil
}

// GetCommand retrieves a command by ID, returning a not_found error if it doesn't exist.
func (s *catalogService) GetCommand(ctx context.Context, id string) (*models.CommandEntry, error) {
	entry, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get command: %w", err)
	}
	if entry == nil {
		return nil, &models.APIError{
			Code:    "not_found",
			Message: "command not found",
		}
	}
	return entry, nil
}

// ListCommands returns a paginated list of active commands filtered by the user's roles.
func (s *catalogService) ListCommands(ctx context.Context, filter models.CommandFilter) (*models.PaginatedResult, error) {
	// Always filter to active commands only
	active := true
	filter.IsActive = &active

	result, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list commands: %w", err)
	}
	return result, nil
}

// SearchCommands performs a case-insensitive substring search on name or description,
// filtered by user roles and active status.
func (s *catalogService) SearchCommands(ctx context.Context, query string, userRoles []models.Role) ([]models.CommandEntry, error) {
	active := true
	filter := models.CommandFilter{
		IsActive: &active,
		Roles:    userRoles,
		Search:   query,
		Page:     1,
		PageSize: 100,
	}

	result, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("search commands: %w", err)
	}

	entries, ok := result.Items.([]models.CommandEntry)
	if !ok {
		return []models.CommandEntry{}, nil
	}
	return entries, nil
}
