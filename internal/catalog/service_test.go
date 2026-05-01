package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/kariz/kariz/internal/models"
)

// --- Mock CommandRepository ---

type mockCommandRepository struct {
	createErr    error
	updateErr    error
	deactivateErr error

	getByIDResult  *models.CommandEntry
	getByIDErr     error

	getByNameResult *models.CommandEntry
	getByNameErr    error

	listResult *models.PaginatedResult
	listErr    error

	// Capture calls for assertions
	createdEntry *models.CommandEntry
	createdRoles []models.Role
	updatedEntry *models.CommandEntry
	updatedRoles []models.Role
	deactivatedID string
	listFilter    *models.CommandFilter
}

func (m *mockCommandRepository) Create(_ context.Context, entry *models.CommandEntry, roles []models.Role) error {
	m.createdEntry = entry
	m.createdRoles = roles
	return m.createErr
}

func (m *mockCommandRepository) GetByID(_ context.Context, id string) (*models.CommandEntry, error) {
	return m.getByIDResult, m.getByIDErr
}

func (m *mockCommandRepository) GetByName(_ context.Context, name string) (*models.CommandEntry, error) {
	return m.getByNameResult, m.getByNameErr
}

func (m *mockCommandRepository) Update(_ context.Context, entry *models.CommandEntry, roles []models.Role) error {
	m.updatedEntry = entry
	m.updatedRoles = roles
	return m.updateErr
}

func (m *mockCommandRepository) Deactivate(_ context.Context, id string) error {
	m.deactivatedID = id
	return m.deactivateErr
}

func (m *mockCommandRepository) List(_ context.Context, filter models.CommandFilter) (*models.PaginatedResult, error) {
	m.listFilter = &filter
	return m.listResult, m.listErr
}

// --- CreateCommand Tests ---

func TestCreateCommand_ValidInput_Succeeds(t *testing.T) {
	repo := &mockCommandRepository{}
	svc := NewCatalogService(repo)

	input := models.CreateCommandInput{
		Name:          "test-command",
		CommandString: "echo hello",
		AllowedRoles:  []models.Role{models.RoleAdmin},
		DockerImage:   "alpine:latest",
	}

	result, err := svc.CreateCommand(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.Name != "test-command" {
		t.Errorf("expected name 'test-command', got %q", result.Name)
	}
	if result.CommandString != "echo hello" {
		t.Errorf("expected command_string 'echo hello', got %q", result.CommandString)
	}
	if result.ID == "" {
		t.Error("expected non-empty ID")
	}
	if result.Version != 1 {
		t.Errorf("expected version 1, got %d", result.Version)
	}
	if !result.IsActive {
		t.Error("expected is_active to be true")
	}
	if result.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
	if result.UpdatedAt.IsZero() {
		t.Error("expected non-zero updated_at")
	}
	if result.ExecutionMode != models.ModeCreate {
		t.Errorf("expected execution_mode 'create', got %q", result.ExecutionMode)
	}

	// Verify repo was called
	if repo.createdEntry == nil {
		t.Fatal("expected repo.Create to be called")
	}
	if len(repo.createdRoles) != 1 || repo.createdRoles[0] != models.RoleAdmin {
		t.Errorf("expected roles [admin], got %v", repo.createdRoles)
	}
}

func TestCreateCommand_EmptyName_ReturnsValidationError(t *testing.T) {
	repo := &mockCommandRepository{}
	svc := NewCatalogService(repo)

	input := models.CreateCommandInput{
		Name:          "",
		CommandString: "echo hello",
		AllowedRoles:  []models.Role{models.RoleAdmin},
	}

	result, err := svc.CreateCommand(context.Background(), input)
	if result != nil {
		t.Error("expected nil result")
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr, ok := err.(*models.APIError)
	if !ok {
		t.Fatalf("expected *models.APIError, got %T", err)
	}
	if apiErr.Code != "validation_error" {
		t.Errorf("expected code 'validation_error', got %q", apiErr.Code)
	}
	if apiErr.Message != "name is required" {
		t.Errorf("expected message 'name is required', got %q", apiErr.Message)
	}
}

func TestCreateCommand_WhitespaceOnlyName_ReturnsValidationError(t *testing.T) {
	repo := &mockCommandRepository{}
	svc := NewCatalogService(repo)

	input := models.CreateCommandInput{
		Name:          "   ",
		CommandString: "echo hello",
		AllowedRoles:  []models.Role{models.RoleAdmin},
	}

	_, err := svc.CreateCommand(context.Background(), input)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr, ok := err.(*models.APIError)
	if !ok {
		t.Fatalf("expected *models.APIError, got %T", err)
	}
	if apiErr.Code != "validation_error" {
		t.Errorf("expected code 'validation_error', got %q", apiErr.Code)
	}
}

func TestCreateCommand_EmptyCommandString_ReturnsValidationError(t *testing.T) {
	repo := &mockCommandRepository{}
	svc := NewCatalogService(repo)

	input := models.CreateCommandInput{
		Name:          "test-command",
		CommandString: "",
		AllowedRoles:  []models.Role{models.RoleAdmin},
	}

	_, err := svc.CreateCommand(context.Background(), input)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr, ok := err.(*models.APIError)
	if !ok {
		t.Fatalf("expected *models.APIError, got %T", err)
	}
	if apiErr.Code != "validation_error" {
		t.Errorf("expected code 'validation_error', got %q", apiErr.Code)
	}
	if apiErr.Message != "command_string is required" {
		t.Errorf("expected message 'command_string is required', got %q", apiErr.Message)
	}
}

func TestCreateCommand_NoRoles_ReturnsValidationError(t *testing.T) {
	repo := &mockCommandRepository{}
	svc := NewCatalogService(repo)

	input := models.CreateCommandInput{
		Name:          "test-command",
		CommandString: "echo hello",
		AllowedRoles:  []models.Role{},
	}

	_, err := svc.CreateCommand(context.Background(), input)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr, ok := err.(*models.APIError)
	if !ok {
		t.Fatalf("expected *models.APIError, got %T", err)
	}
	if apiErr.Code != "validation_error" {
		t.Errorf("expected code 'validation_error', got %q", apiErr.Code)
	}
	if apiErr.Message != "at least one allowed role is required" {
		t.Errorf("expected message about roles, got %q", apiErr.Message)
	}
}

func TestCreateCommand_DuplicateName_ReturnsConflictError(t *testing.T) {
	repo := &mockCommandRepository{
		getByNameResult: &models.CommandEntry{
			ID:   "existing-id",
			Name: "test-command",
		},
	}
	svc := NewCatalogService(repo)

	input := models.CreateCommandInput{
		Name:          "test-command",
		CommandString: "echo hello",
		AllowedRoles:  []models.Role{models.RoleAdmin},
	}

	result, err := svc.CreateCommand(context.Background(), input)
	if result != nil {
		t.Error("expected nil result")
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr, ok := err.(*models.APIError)
	if !ok {
		t.Fatalf("expected *models.APIError, got %T", err)
	}
	if apiErr.Code != "conflict" {
		t.Errorf("expected code 'conflict', got %q", apiErr.Code)
	}
}

func TestCreateCommand_RepoCreateError_ReturnsError(t *testing.T) {
	repo := &mockCommandRepository{
		createErr: errors.New("db connection failed"),
	}
	svc := NewCatalogService(repo)

	input := models.CreateCommandInput{
		Name:          "test-command",
		CommandString: "echo hello",
		AllowedRoles:  []models.Role{models.RoleAdmin},
	}

	_, err := svc.CreateCommand(context.Background(), input)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- UpdateCommand Tests ---

func TestUpdateCommand_IncrementsVersion(t *testing.T) {
	existing := &models.CommandEntry{
		ID:            "cmd-1",
		Name:          "original-name",
		CommandString: "echo original",
		AllowedRoles:  []models.Role{models.RoleAdmin},
		Version:       3,
		IsActive:      true,
	}
	repo := &mockCommandRepository{
		getByIDResult: existing,
	}
	svc := NewCatalogService(repo)

	newName := "updated-name"
	input := models.UpdateCommandInput{
		Name: &newName,
	}

	result, err := svc.UpdateCommand(context.Background(), "cmd-1", input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Version != 4 {
		t.Errorf("expected version 4, got %d", result.Version)
	}
	if result.Name != "updated-name" {
		t.Errorf("expected name 'updated-name', got %q", result.Name)
	}

	// Verify repo.Update was called
	if repo.updatedEntry == nil {
		t.Fatal("expected repo.Update to be called")
	}
	if repo.updatedEntry.Version != 4 {
		t.Errorf("expected updated version 4, got %d", repo.updatedEntry.Version)
	}
}

func TestUpdateCommand_NotFound_ReturnsError(t *testing.T) {
	repo := &mockCommandRepository{
		getByIDResult: nil,
	}
	svc := NewCatalogService(repo)

	newName := "updated-name"
	input := models.UpdateCommandInput{
		Name: &newName,
	}

	_, err := svc.UpdateCommand(context.Background(), "nonexistent", input)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr, ok := err.(*models.APIError)
	if !ok {
		t.Fatalf("expected *models.APIError, got %T", err)
	}
	if apiErr.Code != "not_found" {
		t.Errorf("expected code 'not_found', got %q", apiErr.Code)
	}
}

func TestUpdateCommand_EmptyName_ReturnsValidationError(t *testing.T) {
	existing := &models.CommandEntry{
		ID:            "cmd-1",
		Name:          "original-name",
		CommandString: "echo original",
		AllowedRoles:  []models.Role{models.RoleAdmin},
		Version:       1,
	}
	repo := &mockCommandRepository{
		getByIDResult: existing,
	}
	svc := NewCatalogService(repo)

	emptyName := "  "
	input := models.UpdateCommandInput{
		Name: &emptyName,
	}

	_, err := svc.UpdateCommand(context.Background(), "cmd-1", input)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr, ok := err.(*models.APIError)
	if !ok {
		t.Fatalf("expected *models.APIError, got %T", err)
	}
	if apiErr.Code != "validation_error" {
		t.Errorf("expected code 'validation_error', got %q", apiErr.Code)
	}
}

func TestUpdateCommand_DuplicateName_ReturnsConflictError(t *testing.T) {
	existing := &models.CommandEntry{
		ID:            "cmd-1",
		Name:          "original-name",
		CommandString: "echo original",
		AllowedRoles:  []models.Role{models.RoleAdmin},
		Version:       1,
	}
	repo := &mockCommandRepository{
		getByIDResult:   existing,
		getByNameResult: &models.CommandEntry{ID: "cmd-2", Name: "taken-name"},
	}
	svc := NewCatalogService(repo)

	takenName := "taken-name"
	input := models.UpdateCommandInput{
		Name: &takenName,
	}

	_, err := svc.UpdateCommand(context.Background(), "cmd-1", input)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr, ok := err.(*models.APIError)
	if !ok {
		t.Fatalf("expected *models.APIError, got %T", err)
	}
	if apiErr.Code != "conflict" {
		t.Errorf("expected code 'conflict', got %q", apiErr.Code)
	}
}

func TestUpdateCommand_PartialUpdate_PreservesUnchangedFields(t *testing.T) {
	existing := &models.CommandEntry{
		ID:            "cmd-1",
		Name:          "original-name",
		Description:   "original description",
		Category:      "ops",
		CommandString: "echo original",
		DockerImage:   "alpine:latest",
		AllowedRoles:  []models.Role{models.RoleAdmin},
		Version:       1,
		IsActive:      true,
	}
	repo := &mockCommandRepository{
		getByIDResult: existing,
	}
	svc := NewCatalogService(repo)

	newDesc := "updated description"
	input := models.UpdateCommandInput{
		Description: &newDesc,
	}

	result, err := svc.UpdateCommand(context.Background(), "cmd-1", input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Changed field
	if result.Description != "updated description" {
		t.Errorf("expected description 'updated description', got %q", result.Description)
	}
	// Unchanged fields
	if result.Name != "original-name" {
		t.Errorf("expected name 'original-name', got %q", result.Name)
	}
	if result.Category != "ops" {
		t.Errorf("expected category 'ops', got %q", result.Category)
	}
	if result.CommandString != "echo original" {
		t.Errorf("expected command_string 'echo original', got %q", result.CommandString)
	}
	if result.Version != 2 {
		t.Errorf("expected version 2, got %d", result.Version)
	}
}

// --- DeactivateCommand Tests ---

func TestDeactivateCommand_CallsRepo(t *testing.T) {
	repo := &mockCommandRepository{}
	svc := NewCatalogService(repo)

	err := svc.DeactivateCommand(context.Background(), "cmd-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if repo.deactivatedID != "cmd-1" {
		t.Errorf("expected deactivated ID 'cmd-1', got %q", repo.deactivatedID)
	}
}

func TestDeactivateCommand_RepoError_ReturnsError(t *testing.T) {
	repo := &mockCommandRepository{
		deactivateErr: errors.New("not found"),
	}
	svc := NewCatalogService(repo)

	err := svc.DeactivateCommand(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- GetCommand Tests ---

func TestGetCommand_Found_ReturnsEntry(t *testing.T) {
	expected := &models.CommandEntry{
		ID:   "cmd-1",
		Name: "test-command",
	}
	repo := &mockCommandRepository{
		getByIDResult: expected,
	}
	svc := NewCatalogService(repo)

	result, err := svc.GetCommand(context.Background(), "cmd-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.ID != "cmd-1" {
		t.Errorf("expected ID 'cmd-1', got %q", result.ID)
	}
}

func TestGetCommand_NotFound_ReturnsError(t *testing.T) {
	repo := &mockCommandRepository{
		getByIDResult: nil,
	}
	svc := NewCatalogService(repo)

	_, err := svc.GetCommand(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr, ok := err.(*models.APIError)
	if !ok {
		t.Fatalf("expected *models.APIError, got %T", err)
	}
	if apiErr.Code != "not_found" {
		t.Errorf("expected code 'not_found', got %q", apiErr.Code)
	}
}

// --- ListCommands Tests ---

func TestListCommands_SetsActiveFilter(t *testing.T) {
	repo := &mockCommandRepository{
		listResult: &models.PaginatedResult{
			Items:    []models.CommandEntry{},
			Total:    0,
			Page:     1,
			PageSize: 20,
		},
	}
	svc := NewCatalogService(repo)

	filter := models.CommandFilter{
		Roles:    []models.Role{models.RoleQA},
		Page:     1,
		PageSize: 20,
	}

	_, err := svc.ListCommands(context.Background(), filter)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if repo.listFilter == nil {
		t.Fatal("expected repo.List to be called")
	}
	if repo.listFilter.IsActive == nil {
		t.Fatal("expected IsActive filter to be set")
	}
	if !*repo.listFilter.IsActive {
		t.Error("expected IsActive to be true")
	}
}

func TestListCommands_PassesRolesFilter(t *testing.T) {
	repo := &mockCommandRepository{
		listResult: &models.PaginatedResult{
			Items:    []models.CommandEntry{},
			Total:    0,
			Page:     1,
			PageSize: 20,
		},
	}
	svc := NewCatalogService(repo)

	filter := models.CommandFilter{
		Roles:    []models.Role{models.RoleQA, models.RoleData},
		Page:     1,
		PageSize: 10,
	}

	_, err := svc.ListCommands(context.Background(), filter)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if repo.listFilter == nil {
		t.Fatal("expected repo.List to be called")
	}
	if len(repo.listFilter.Roles) != 2 {
		t.Errorf("expected 2 roles in filter, got %d", len(repo.listFilter.Roles))
	}
}

func TestListCommands_RepoError_ReturnsError(t *testing.T) {
	repo := &mockCommandRepository{
		listErr: errors.New("db error"),
	}
	svc := NewCatalogService(repo)

	filter := models.CommandFilter{
		Roles:    []models.Role{models.RoleAdmin},
		Page:     1,
		PageSize: 20,
	}

	_, err := svc.ListCommands(context.Background(), filter)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- SearchCommands Tests ---

func TestSearchCommands_ReturnsMatchingResults(t *testing.T) {
	entries := []models.CommandEntry{
		{ID: "cmd-1", Name: "deploy-app", Description: "Deploy the application"},
		{ID: "cmd-2", Name: "deploy-db", Description: "Deploy database migrations"},
	}
	repo := &mockCommandRepository{
		listResult: &models.PaginatedResult{
			Items:    entries,
			Total:    2,
			Page:     1,
			PageSize: 100,
		},
	}
	svc := NewCatalogService(repo)

	results, err := svc.SearchCommands(context.Background(), "deploy", []models.Role{models.RoleAdmin})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// Verify the filter was set correctly
	if repo.listFilter == nil {
		t.Fatal("expected repo.List to be called")
	}
	if repo.listFilter.Search != "deploy" {
		t.Errorf("expected search 'deploy', got %q", repo.listFilter.Search)
	}
	if repo.listFilter.IsActive == nil || !*repo.listFilter.IsActive {
		t.Error("expected IsActive filter to be true")
	}
	if len(repo.listFilter.Roles) != 1 || repo.listFilter.Roles[0] != models.RoleAdmin {
		t.Errorf("expected roles [admin], got %v", repo.listFilter.Roles)
	}
}

func TestSearchCommands_EmptyResults(t *testing.T) {
	repo := &mockCommandRepository{
		listResult: &models.PaginatedResult{
			Items:    []models.CommandEntry{},
			Total:    0,
			Page:     1,
			PageSize: 100,
		},
	}
	svc := NewCatalogService(repo)

	results, err := svc.SearchCommands(context.Background(), "nonexistent", []models.Role{models.RoleQA})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearchCommands_RepoError_ReturnsError(t *testing.T) {
	repo := &mockCommandRepository{
		listErr: errors.New("db error"),
	}
	svc := NewCatalogService(repo)

	_, err := svc.SearchCommands(context.Background(), "test", []models.Role{models.RoleAdmin})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
