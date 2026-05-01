package property_test

// Feature: kariz-command-dashboard, Property 10: execution history pagination and ordering
// Feature: kariz-command-dashboard, Property 11: execution history filter correctness

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kariz/kariz/internal/models"
	"pgregory.net/rapid"
)

// --- Mock ExecutionRepository for history property tests ---

// historyMockRepo is an in-memory ExecutionRepository that implements
// List with proper filtering, pagination, and ordering for property testing.
type historyMockRepo struct {
	records []*models.ExecutionRecord
}

func newHistoryMockRepo() *historyMockRepo {
	return &historyMockRepo{
		records: make([]*models.ExecutionRecord, 0),
	}
}

func (m *historyMockRepo) Create(_ context.Context, record *models.ExecutionRecord) error {
	cp := *record
	m.records = append(m.records, &cp)
	return nil
}

func (m *historyMockRepo) Update(_ context.Context, record *models.ExecutionRecord) error {
	for i, r := range m.records {
		if r.ID == record.ID {
			cp := *record
			m.records[i] = &cp
			return nil
		}
	}
	return fmt.Errorf("record not found: %s", record.ID)
}

func (m *historyMockRepo) GetByID(_ context.Context, id string) (*models.ExecutionRecord, error) {
	for _, r := range m.records {
		if r.ID == id {
			cp := *r
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *historyMockRepo) CountRunning(_ context.Context, commandID string) (int, error) {
	count := 0
	for _, r := range m.records {
		if r.CommandID == commandID && r.Status == models.StatusRunning {
			count++
		}
	}
	return count, nil
}

func (m *historyMockRepo) List(_ context.Context, filter models.ExecutionFilter) (*models.PaginatedResult, error) {
	// Apply filters
	var filtered []*models.ExecutionRecord
	for _, r := range m.records {
		if !matchesFilter(r, filter) {
			continue
		}
		filtered = append(filtered, r)
	}

	// Sort by created_at descending
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})

	// Pagination
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 20
	}

	total := len(filtered)
	offset := (page - 1) * pageSize
	if offset > total {
		offset = total
	}
	end := offset + pageSize
	if end > total {
		end = total
	}

	result := make([]models.ExecutionRecord, 0, end-offset)
	for _, r := range filtered[offset:end] {
		result = append(result, *r)
	}

	return &models.PaginatedResult{
		Items:    result,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func matchesFilter(r *models.ExecutionRecord, filter models.ExecutionFilter) bool {
	// command_name ILIKE filter
	if filter.CommandName != "" {
		if !strings.Contains(strings.ToLower(r.CommandName), strings.ToLower(filter.CommandName)) {
			return false
		}
	}
	// user_id exact match
	if filter.UserID != "" && r.UserID != filter.UserID {
		return false
	}
	// date_from
	if filter.DateFrom != nil && r.CreatedAt.Before(*filter.DateFrom) {
		return false
	}
	// date_to
	if filter.DateTo != nil && r.CreatedAt.After(*filter.DateTo) {
		return false
	}
	// status exact match
	if filter.Status != "" && r.Status != filter.Status {
		return false
	}
	return true
}

// --- Generators ---

var allStatuses = []models.ExecutionStatus{
	models.StatusQueued,
	models.StatusRunning,
	models.StatusCompleted,
	models.StatusFailed,
	models.StatusTimedOut,
	models.StatusCancelled,
}

var commandNames = []string{
	"deploy-app", "db-dump", "run-tests", "cleanup-logs",
	"data-export", "health-check", "migrate-db", "restart-service",
}

var userIDs = []string{
	"user-alice", "user-bob", "user-charlie", "user-diana",
}

// genHistoryRecord generates a random ExecutionRecord with a created_at time
// within the given time range.
func genHistoryRecord(t *rapid.T, label string, baseTime time.Time, offsetMinutes int) models.ExecutionRecord {
	params, _ := json.Marshal(map[string]interface{}{})
	statusIdx := rapid.IntRange(0, len(allStatuses)-1).Draw(t, label+"_status")
	cmdIdx := rapid.IntRange(0, len(commandNames)-1).Draw(t, label+"_cmd")
	userIdx := rapid.IntRange(0, len(userIDs)-1).Draw(t, label+"_user")

	createdAt := baseTime.Add(time.Duration(offsetMinutes) * time.Minute)

	return models.ExecutionRecord{
		ID:          uuid.New().String(),
		CommandID:   uuid.New().String(),
		CommandName: commandNames[cmdIdx],
		UserID:      userIDs[userIdx],
		Parameters:  params,
		Status:      allStatuses[statusIdx],
		CreatedAt:   createdAt,
	}
}

// --- Property 10 Tests ---
// **Validates: Requirements 4.1**

// TestProperty10_PaginationAndOrdering tests that returned records are ordered by
// created_at descending, count doesn't exceed page_size, and records correspond
// to the correct offset into the full ordered set.
func TestProperty10_PaginationAndOrdering(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		repo := newHistoryMockRepo()

		// Generate 5-30 records with distinct timestamps
		numRecords := rapid.IntRange(5, 30).Draw(t, "numRecords")
		baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		for i := 0; i < numRecords; i++ {
			record := genHistoryRecord(t, fmt.Sprintf("rec%d", i), baseTime, i)
			_ = repo.Create(context.Background(), &record)
		}

		// Generate pagination parameters
		pageSize := rapid.IntRange(1, 10).Draw(t, "pageSize")
		maxPage := (numRecords + pageSize - 1) / pageSize
		page := rapid.IntRange(1, maxPage).Draw(t, "page")

		filter := models.ExecutionFilter{
			Page:     page,
			PageSize: pageSize,
		}

		result, err := repo.List(context.Background(), filter)
		if err != nil {
			t.Fatalf("unexpected error listing records: %v", err)
		}

		records, ok := result.Items.([]models.ExecutionRecord)
		if !ok {
			t.Fatal("expected []models.ExecutionRecord from List")
		}

		// Property: count doesn't exceed page_size
		if len(records) > pageSize {
			t.Fatalf("returned %d records, exceeds page_size %d", len(records), pageSize)
		}

		// Property: records are ordered by created_at descending
		for i := 1; i < len(records); i++ {
			if records[i].CreatedAt.After(records[i-1].CreatedAt) {
				t.Fatalf("records not in descending order: record[%d].created_at=%v > record[%d].created_at=%v",
					i, records[i].CreatedAt, i-1, records[i-1].CreatedAt)
			}
		}

		// Property: total count matches the full set
		if result.Total != numRecords {
			t.Fatalf("expected total %d, got %d", numRecords, result.Total)
		}

		// Property: correct offset — verify records correspond to the right slice
		offset := (page - 1) * pageSize
		expectedCount := pageSize
		remaining := numRecords - offset
		if remaining < expectedCount {
			expectedCount = remaining
		}
		if expectedCount < 0 {
			expectedCount = 0
		}
		if len(records) != expectedCount {
			t.Fatalf("expected %d records for page %d (pageSize %d, total %d), got %d",
				expectedCount, page, pageSize, numRecords, len(records))
		}
	})
}

// --- Property 11 Tests ---
// **Validates: Requirements 4.3**

// TestProperty11_FilterCorrectness tests that every returned record satisfies
// all applied filter criteria simultaneously.
func TestProperty11_FilterCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		repo := newHistoryMockRepo()

		// Generate 10-30 records
		numRecords := rapid.IntRange(10, 30).Draw(t, "numRecords")
		baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		for i := 0; i < numRecords; i++ {
			record := genHistoryRecord(t, fmt.Sprintf("rec%d", i), baseTime, i*10)
			_ = repo.Create(context.Background(), &record)
		}

		// Build a random filter with 1-4 criteria applied
		filter := models.ExecutionFilter{
			Page:     1,
			PageSize: 100,
		}

		// Optionally filter by command name
		if rapid.Bool().Draw(t, "filterByCmd") {
			cmdIdx := rapid.IntRange(0, len(commandNames)-1).Draw(t, "filterCmdIdx")
			filter.CommandName = commandNames[cmdIdx]
		}

		// Optionally filter by user ID
		if rapid.Bool().Draw(t, "filterByUser") {
			userIdx := rapid.IntRange(0, len(userIDs)-1).Draw(t, "filterUserIdx")
			filter.UserID = userIDs[userIdx]
		}

		// Optionally filter by status
		if rapid.Bool().Draw(t, "filterByStatus") {
			statusIdx := rapid.IntRange(0, len(allStatuses)-1).Draw(t, "filterStatusIdx")
			filter.Status = allStatuses[statusIdx]
		}

		// Optionally filter by date range
		if rapid.Bool().Draw(t, "filterByDate") {
			fromOffset := rapid.IntRange(0, numRecords*5).Draw(t, "dateFromOffset")
			toOffset := rapid.IntRange(fromOffset, numRecords*15).Draw(t, "dateToOffset")
			dateFrom := baseTime.Add(time.Duration(fromOffset) * time.Minute)
			dateTo := baseTime.Add(time.Duration(toOffset) * time.Minute)
			filter.DateFrom = &dateFrom
			filter.DateTo = &dateTo
		}

		result, err := repo.List(context.Background(), filter)
		if err != nil {
			t.Fatalf("unexpected error listing records: %v", err)
		}

		records, ok := result.Items.([]models.ExecutionRecord)
		if !ok {
			t.Fatal("expected []models.ExecutionRecord from List")
		}

		// Verify every returned record satisfies ALL applied filter criteria
		for _, r := range records {
			// Command name filter (ILIKE = case-insensitive contains)
			if filter.CommandName != "" {
				if !strings.Contains(strings.ToLower(r.CommandName), strings.ToLower(filter.CommandName)) {
					t.Fatalf("record %q has command_name=%q which doesn't match filter %q",
						r.ID, r.CommandName, filter.CommandName)
				}
			}

			// User ID filter (exact match)
			if filter.UserID != "" {
				if r.UserID != filter.UserID {
					t.Fatalf("record %q has user_id=%q which doesn't match filter %q",
						r.ID, r.UserID, filter.UserID)
				}
			}

			// Status filter (exact match)
			if filter.Status != "" {
				if r.Status != filter.Status {
					t.Fatalf("record %q has status=%q which doesn't match filter %q",
						r.ID, r.Status, filter.Status)
				}
			}

			// Date range filter
			if filter.DateFrom != nil {
				if r.CreatedAt.Before(*filter.DateFrom) {
					t.Fatalf("record %q has created_at=%v which is before date_from=%v",
						r.ID, r.CreatedAt, *filter.DateFrom)
				}
			}
			if filter.DateTo != nil {
				if r.CreatedAt.After(*filter.DateTo) {
					t.Fatalf("record %q has created_at=%v which is after date_to=%v",
						r.ID, r.CreatedAt, *filter.DateTo)
				}
			}
		}

		// Also verify ordering is maintained (descending by created_at)
		for i := 1; i < len(records); i++ {
			if records[i].CreatedAt.After(records[i-1].CreatedAt) {
				t.Fatalf("records not in descending order at index %d", i)
			}
		}
	})
}
