package property_test

// Feature: kariz-command-dashboard, Property 1: command storage round-trip
// Feature: kariz-command-dashboard, Property 2: duplicate name rejection
// Feature: kariz-command-dashboard, Property 3: version increment on update
// Feature: kariz-command-dashboard, Property 4: deactivation hides from non-admins
// Feature: kariz-command-dashboard, Property 5: invalid command definition rejection
// Feature: kariz-command-dashboard, Property 6: role-based command filtering
// Feature: kariz-command-dashboard, Property 7: search filter correctness

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/kariz/kariz/internal/catalog"
	"github.com/kariz/kariz/internal/models"
	"pgregory.net/rapid"
)

// --- Mock CommandRepository for catalog property tests ---

// catalogMockRepo is an in-memory CommandRepository for property testing.
type catalogMockRepo struct {
	mu      sync.Mutex
	entries map[string]*models.CommandEntry // id -> entry
	byName  map[string]*models.CommandEntry // name -> entry
	roles   map[string][]models.Role        // id -> roles
}

func newCatalogMockRepo() *catalogMockRepo {
	return &catalogMockRepo{
		entries: make(map[string]*models.CommandEntry),
		byName:  make(map[string]*models.CommandEntry),
		roles:   make(map[string][]models.Role),
	}
}

func (m *catalogMockRepo) Create(_ context.Context, entry *models.CommandEntry, roles []models.Role) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *entry
	m.entries[entry.ID] = &cp
	m.byName[entry.Name] = &cp
	m.roles[entry.ID] = append([]models.Role{}, roles...)
	return nil
}

func (m *catalogMockRepo) GetByID(_ context.Context, id string) (*models.CommandEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[id]
	if !ok {
		return nil, nil
	}
	cp := *e
	cp.AllowedRoles = append([]models.Role{}, m.roles[id]...)
	return &cp, nil
}

func (m *catalogMockRepo) GetByName(_ context.Context, name string) (*models.CommandEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.byName[name]
	if !ok {
		return nil, nil
	}
	cp := *e
	cp.AllowedRoles = append([]models.Role{}, m.roles[e.ID]...)
	return &cp, nil
}

func (m *catalogMockRepo) Update(_ context.Context, entry *models.CommandEntry, roles []models.Role) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Remove old name mapping if name changed
	if old, ok := m.entries[entry.ID]; ok {
		if old.Name != entry.Name {
			delete(m.byName, old.Name)
		}
	}
	cp := *entry
	m.entries[entry.ID] = &cp
	m.byName[entry.Name] = &cp
	m.roles[entry.ID] = append([]models.Role{}, roles...)
	return nil
}

func (m *catalogMockRepo) Deactivate(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[id]
	if !ok {
		return fmt.Errorf("command entry not found: %s", id)
	}
	e.IsActive = false
	return nil
}

func (m *catalogMockRepo) List(_ context.Context, filter models.CommandFilter) (*models.PaginatedResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var matched []models.CommandEntry
	for id, e := range m.entries {
		// Active filter
		if filter.IsActive != nil && e.IsActive != *filter.IsActive {
			continue
		}
		// Category filter
		if filter.Category != "" && e.Category != filter.Category {
			continue
		}
		// Role filter
		if len(filter.Roles) > 0 {
			entryRoles := m.roles[id]
			if !catalogHasRoleOverlap(entryRoles, filter.Roles) {
				continue
			}
		}
		// Search filter
		if filter.Search != "" {
			lowerSearch := strings.ToLower(filter.Search)
			if !strings.Contains(strings.ToLower(e.Name), lowerSearch) &&
				!strings.Contains(strings.ToLower(e.Description), lowerSearch) {
				continue
			}
		}
		cp := *e
		cp.AllowedRoles = append([]models.Role{}, m.roles[id]...)
		matched = append(matched, cp)
	}

	// Pagination
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	total := len(matched)
	offset := (page - 1) * pageSize
	if offset > total {
		offset = total
	}
	end := offset + pageSize
	if end > total {
		end = total
	}

	if matched == nil {
		matched = []models.CommandEntry{}
	}

	return &models.PaginatedResult{
		Items:    matched[offset:end],
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func catalogHasRoleOverlap(a, b []models.Role) bool {
	set := make(map[models.Role]bool, len(a))
	for _, r := range a {
		set[r] = true
	}
	for _, r := range b {
		if set[r] {
			return true
		}
	}
	return false
}

// --- Generators ---

// genCatalogRoles generates a non-empty subset of roles using a bitmask approach.
func genCatalogRoles(t *rapid.T, label string) []models.Role {
	mask := rapid.IntRange(1, (1<<len(allRoles))-1).Draw(t, label+"_mask")
	var roles []models.Role
	for i, r := range allRoles {
		if mask&(1<<i) != 0 {
			roles = append(roles, r)
		}
	}
	return roles
}

// genCatalogNonAdminRoles generates a non-empty subset of non-admin roles.
func genCatalogNonAdminRoles(t *rapid.T, label string) []models.Role {
	nonAdmin := []models.Role{models.RoleQA, models.RoleData, models.RoleOperations}
	mask := rapid.IntRange(1, (1<<len(nonAdmin))-1).Draw(t, label+"_mask")
	var roles []models.Role
	for i, r := range nonAdmin {
		if mask&(1<<i) != 0 {
			roles = append(roles, r)
		}
	}
	return roles
}

// genValidCreateInput generates a valid CreateCommandInput with unique name.
func genValidCreateInput(t *rapid.T, label string) models.CreateCommandInput {
	return models.CreateCommandInput{
		Name:            rapid.StringMatching(`^[a-z][a-z0-9-]{3,20}`).Draw(t, label+"_name"),
		Description:     rapid.StringMatching(`^[A-Za-z ]{0,50}`).Draw(t, label+"_desc"),
		Category:        rapid.SampledFrom([]string{"ops", "data", "qa", "infra"}).Draw(t, label+"_cat"),
		DockerImage:     rapid.StringMatching(`^[a-z]+:[a-z0-9.]+`).Draw(t, label+"_image"),
		CommandString:   rapid.StringMatching(`^[a-z]+ [a-z]+`).Draw(t, label+"_cmd"),
		AllowedRoles:    genCatalogRoles(t, label+"_roles"),
		TimeoutSeconds:  rapid.IntRange(10, 3600).Draw(t, label+"_timeout"),
		AllowConcurrent: rapid.Bool().Draw(t, label+"_concurrent"),
	}
}

// --- Property 1 Tests ---
// **Validates: Requirements 1.1, 8.1**

// TestProperty1_CommandStorageRoundTrip tests that for any valid CreateCommandInput,
// creating and then retrieving by ID returns identical values.
func TestProperty1_CommandStorageRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		repo := newCatalogMockRepo()
		svc := catalog.NewCatalogService(repo)

		input := genValidCreateInput(t, "input")

		created, err := svc.CreateCommand(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error creating command: %v", err)
		}

		retrieved, err := svc.GetCommand(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("unexpected error retrieving command: %v", err)
		}

		// Verify all fields match
		if retrieved.ID != created.ID {
			t.Fatalf("ID mismatch: created=%q, retrieved=%q", created.ID, retrieved.ID)
		}
		if retrieved.Name != input.Name {
			t.Fatalf("Name mismatch: input=%q, retrieved=%q", input.Name, retrieved.Name)
		}
		if retrieved.Description != input.Description {
			t.Fatalf("Description mismatch: input=%q, retrieved=%q", input.Description, retrieved.Description)
		}
		if retrieved.Category != input.Category {
			t.Fatalf("Category mismatch: input=%q, retrieved=%q", input.Category, retrieved.Category)
		}
		if retrieved.DockerImage != input.DockerImage {
			t.Fatalf("DockerImage mismatch: input=%q, retrieved=%q", input.DockerImage, retrieved.DockerImage)
		}
		if retrieved.CommandString != input.CommandString {
			t.Fatalf("CommandString mismatch: input=%q, retrieved=%q", input.CommandString, retrieved.CommandString)
		}
		if retrieved.TimeoutSeconds != input.TimeoutSeconds {
			t.Fatalf("TimeoutSeconds mismatch: input=%d, retrieved=%d", input.TimeoutSeconds, retrieved.TimeoutSeconds)
		}
		if retrieved.AllowConcurrent != input.AllowConcurrent {
			t.Fatalf("AllowConcurrent mismatch: input=%v, retrieved=%v", input.AllowConcurrent, retrieved.AllowConcurrent)
		}
		if len(retrieved.AllowedRoles) != len(input.AllowedRoles) {
			t.Fatalf("AllowedRoles length mismatch: input=%d, retrieved=%d", len(input.AllowedRoles), len(retrieved.AllowedRoles))
		}
		inputRoleSet := make(map[models.Role]bool)
		for _, r := range input.AllowedRoles {
			inputRoleSet[r] = true
		}
		for _, r := range retrieved.AllowedRoles {
			if !inputRoleSet[r] {
				t.Fatalf("retrieved role %q not in input roles", r)
			}
		}
		if retrieved.Version != 1 {
			t.Fatalf("expected version 1, got %d", retrieved.Version)
		}
		if !retrieved.IsActive {
			t.Fatal("expected is_active=true")
		}
	})
}

// --- Property 2 Tests ---
// **Validates: Requirements 1.2**

// TestProperty2_DuplicateNameRejection tests that creating a command with an
// already-existing name is rejected and the catalog remains unchanged.
func TestProperty2_DuplicateNameRejection(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		repo := newCatalogMockRepo()
		svc := catalog.NewCatalogService(repo)

		// Create the first command
		input1 := genValidCreateInput(t, "first")
		original, err := svc.CreateCommand(context.Background(), input1)
		if err != nil {
			t.Fatalf("unexpected error creating first command: %v", err)
		}

		// Attempt to create a second command with the same name
		input2 := genValidCreateInput(t, "second")
		input2.Name = input1.Name // force duplicate name

		dup, err := svc.CreateCommand(context.Background(), input2)
		if dup != nil {
			t.Fatal("expected nil result for duplicate name")
		}
		if err == nil {
			t.Fatal("expected error for duplicate name, got nil")
		}

		apiErr, ok := err.(*models.APIError)
		if !ok {
			t.Fatalf("expected *models.APIError, got %T: %v", err, err)
		}
		if apiErr.Code != "conflict" {
			t.Fatalf("expected error code 'conflict', got %q", apiErr.Code)
		}

		// Verify original command is unchanged
		retrieved, err := svc.GetCommand(context.Background(), original.ID)
		if err != nil {
			t.Fatalf("unexpected error retrieving original: %v", err)
		}
		if retrieved.Name != original.Name {
			t.Fatalf("original name changed: expected %q, got %q", original.Name, retrieved.Name)
		}
		if retrieved.CommandString != original.CommandString {
			t.Fatalf("original command_string changed")
		}
	})
}

// --- Property 3 Tests ---
// **Validates: Requirements 1.3**

// TestProperty3_VersionIncrementOnUpdate tests that for any existing command at
// version V, applying a valid update results in version V+1. For N updates,
// the final version equals initial version + N.
func TestProperty3_VersionIncrementOnUpdate(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		repo := newCatalogMockRepo()
		svc := catalog.NewCatalogService(repo)

		input := genValidCreateInput(t, "input")
		created, err := svc.CreateCommand(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error creating command: %v", err)
		}

		initialVersion := created.Version
		if initialVersion != 1 {
			t.Fatalf("expected initial version 1, got %d", initialVersion)
		}

		// Apply N updates (1 to 5)
		numUpdates := rapid.IntRange(1, 5).Draw(t, "numUpdates")
		for i := 0; i < numUpdates; i++ {
			newDesc := fmt.Sprintf("updated-%d-%s", i, rapid.StringMatching(`^[a-z]{3,8}`).Draw(t, fmt.Sprintf("desc_%d", i)))
			updateInput := models.UpdateCommandInput{
				Description: &newDesc,
			}
			updated, err := svc.UpdateCommand(context.Background(), created.ID, updateInput)
			if err != nil {
				t.Fatalf("unexpected error on update %d: %v", i+1, err)
			}

			expectedVersion := initialVersion + i + 1
			if updated.Version != expectedVersion {
				t.Fatalf("after update %d: expected version %d, got %d", i+1, expectedVersion, updated.Version)
			}
		}

		// Verify final version
		final, err := svc.GetCommand(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("unexpected error retrieving final: %v", err)
		}
		expectedFinal := initialVersion + numUpdates
		if final.Version != expectedFinal {
			t.Fatalf("final version mismatch: expected %d, got %d", expectedFinal, final.Version)
		}
	})
}

// --- Property 4 Tests ---
// **Validates: Requirements 1.4**

// TestProperty4_DeactivationHidesFromNonAdmins tests that after deactivation,
// listing for non-admin roles excludes the command.
func TestProperty4_DeactivationHidesFromNonAdmins(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		repo := newCatalogMockRepo()
		svc := catalog.NewCatalogService(repo)

		// Create a command
		input := genValidCreateInput(t, "input")
		created, err := svc.CreateCommand(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error creating command: %v", err)
		}

		// Deactivate the command
		err = svc.DeactivateCommand(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("unexpected error deactivating command: %v", err)
		}

		// Verify the command is now inactive
		retrieved, err := svc.GetCommand(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("unexpected error retrieving deactivated command: %v", err)
		}
		if retrieved.IsActive {
			t.Fatal("expected is_active=false after deactivation")
		}

		// ListCommands for non-admin roles should NOT include the deactivated command
		// (ListCommands always filters to active=true)
		nonAdminRoles := genCatalogNonAdminRoles(t, "nonAdminRoles")
		filter := models.CommandFilter{
			Roles:    nonAdminRoles,
			Page:     1,
			PageSize: 100,
		}
		result, err := svc.ListCommands(context.Background(), filter)
		if err != nil {
			t.Fatalf("unexpected error listing commands: %v", err)
		}

		entries, ok := result.Items.([]models.CommandEntry)
		if !ok {
			t.Fatal("expected []models.CommandEntry from ListCommands")
		}
		for _, e := range entries {
			if e.ID == created.ID {
				t.Fatalf("deactivated command %q should not appear in non-admin listing", created.ID)
			}
		}
	})
}

// --- Property 5 Tests ---
// **Validates: Requirements 1.5**

// TestProperty5_InvalidCommandRejection tests that commands with empty name,
// empty command string, or zero roles are rejected.
func TestProperty5_InvalidCommandRejection(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		repo := newCatalogMockRepo()
		svc := catalog.NewCatalogService(repo)

		// Choose which validation to violate
		violation := rapid.IntRange(0, 2).Draw(t, "violation")

		var input models.CreateCommandInput
		switch violation {
		case 0:
			// Empty name
			input = genValidCreateInput(t, "input")
			input.Name = ""
		case 1:
			// Empty command string
			input = genValidCreateInput(t, "input")
			input.CommandString = ""
		case 2:
			// Zero roles
			input = genValidCreateInput(t, "input")
			input.AllowedRoles = []models.Role{}
		}

		result, err := svc.CreateCommand(context.Background(), input)
		if result != nil {
			t.Fatal("expected nil result for invalid input")
		}
		if err == nil {
			t.Fatalf("expected validation error for violation type %d, got nil", violation)
		}

		apiErr, ok := err.(*models.APIError)
		if !ok {
			t.Fatalf("expected *models.APIError, got %T: %v", err, err)
		}
		if apiErr.Code != "validation_error" {
			t.Fatalf("expected error code 'validation_error', got %q", apiErr.Code)
		}

		// Verify no entry was created in the repo
		repo.mu.Lock()
		entryCount := len(repo.entries)
		repo.mu.Unlock()
		if entryCount != 0 {
			t.Fatalf("expected 0 entries in repo after rejection, got %d", entryCount)
		}
	})
}

// --- Property 6 Tests ---
// **Validates: Requirements 2.1**

// TestProperty6_RoleBasedFiltering tests that every command returned by ListCommands
// has is_active=true and at least one role in common with the user's roles.
func TestProperty6_RoleBasedFiltering(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		repo := newCatalogMockRepo()
		svc := catalog.NewCatalogService(repo)

		// Create several commands with various roles
		numCommands := rapid.IntRange(2, 8).Draw(t, "numCommands")
		var createdIDs []string
		for i := 0; i < numCommands; i++ {
			input := genValidCreateInput(t, fmt.Sprintf("cmd%d", i))
			// Ensure unique names by appending index
			input.Name = fmt.Sprintf("%s-%d", input.Name, i)
			cmd, err := svc.CreateCommand(context.Background(), input)
			if err != nil {
				t.Fatalf("unexpected error creating command %d: %v", i, err)
			}
			createdIDs = append(createdIDs, cmd.ID)
		}

		// Optionally deactivate some commands
		deactivateCount := rapid.IntRange(0, len(createdIDs)/2).Draw(t, "deactivateCount")
		for i := 0; i < deactivateCount; i++ {
			idx := rapid.IntRange(0, len(createdIDs)-1).Draw(t, fmt.Sprintf("deactIdx%d", i))
			_ = svc.DeactivateCommand(context.Background(), createdIDs[idx])
		}

		// Generate user roles
		userRoles := genCatalogRoles(t, "userRoles")

		// List commands for these roles
		filter := models.CommandFilter{
			Roles:    userRoles,
			Page:     1,
			PageSize: 100,
		}
		result, err := svc.ListCommands(context.Background(), filter)
		if err != nil {
			t.Fatalf("unexpected error listing commands: %v", err)
		}

		entries, ok := result.Items.([]models.CommandEntry)
		if !ok {
			t.Fatal("expected []models.CommandEntry from ListCommands")
		}

		userRoleSet := make(map[models.Role]bool)
		for _, r := range userRoles {
			userRoleSet[r] = true
		}

		for _, e := range entries {
			// Property: every returned command must be active
			if !e.IsActive {
				t.Fatalf("returned command %q has is_active=false", e.Name)
			}

			// Property: every returned command must have at least one role in common
			hasOverlap := false
			for _, r := range e.AllowedRoles {
				if userRoleSet[r] {
					hasOverlap = true
					break
				}
			}
			if !hasOverlap {
				t.Fatalf("returned command %q has no role overlap with user roles %v (command roles: %v)",
					e.Name, userRoles, e.AllowedRoles)
			}
		}
	})
}

// --- Property 7 Tests ---
// **Validates: Requirements 2.3**

// TestProperty7_SearchCorrectness tests that every command returned by SearchCommands
// contains the search query as a case-insensitive substring of name or description.
func TestProperty7_SearchCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		repo := newCatalogMockRepo()
		svc := catalog.NewCatalogService(repo)

		// Create several commands with varied names and descriptions
		numCommands := rapid.IntRange(3, 10).Draw(t, "numCommands")
		for i := 0; i < numCommands; i++ {
			input := genValidCreateInput(t, fmt.Sprintf("cmd%d", i))
			input.Name = fmt.Sprintf("%s-%d", input.Name, i)
			_, err := svc.CreateCommand(context.Background(), input)
			if err != nil {
				t.Fatalf("unexpected error creating command %d: %v", i, err)
			}
		}

		// Generate a search query (short substring)
		searchQuery := rapid.StringMatching(`^[a-z]{1,4}`).Draw(t, "searchQuery")
		userRoles := genCatalogRoles(t, "userRoles")

		results, err := svc.SearchCommands(context.Background(), searchQuery, userRoles)
		if err != nil {
			t.Fatalf("unexpected error searching commands: %v", err)
		}

		lowerQuery := strings.ToLower(searchQuery)
		for _, e := range results {
			nameMatch := strings.Contains(strings.ToLower(e.Name), lowerQuery)
			descMatch := strings.Contains(strings.ToLower(e.Description), lowerQuery)
			if !nameMatch && !descMatch {
				t.Fatalf("returned command %q (desc=%q) does not contain search query %q in name or description",
					e.Name, e.Description, searchQuery)
			}
		}
	})
}
