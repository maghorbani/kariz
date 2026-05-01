package scheduler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kariz/kariz/internal/models"
)

// newTestSchedule creates a Schedule with sensible defaults for testing.
func newTestSchedule(commandID, userID string) *models.Schedule {
	now := time.Now().UTC().Truncate(time.Microsecond)
	nextRun := now.Add(1 * time.Hour)
	interval := 3600
	return &models.Schedule{
		ID:             uuid.New().String(),
		CommandID:      commandID,
		CreatedByUser:  userID,
		CommandName:    "test-command",
		CronExpression: "0 * * * *",
		IntervalSec:    &interval,
		Parameters:     json.RawMessage(`{"key":"value"}`),
		IsEnabled:      true,
		NextRunAt:      &nextRun,
		LastRunAt:      nil,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// --- Interface compliance ---

func TestScheduleRepositoryImplementsInterface(t *testing.T) {
	// Compile-time check that scheduleRepository satisfies ScheduleRepository.
	var _ ScheduleRepository = (*scheduleRepository)(nil)
}

// --- Constructor ---

func TestNewScheduleRepository(t *testing.T) {
	repo := NewScheduleRepository(nil)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}

func TestNewScheduleRepository_ReturnsCorrectType(t *testing.T) {
	repo := NewScheduleRepository(nil)
	if _, ok := repo.(*scheduleRepository); !ok {
		t.Fatal("expected *scheduleRepository type")
	}
}

// --- Model helpers ---

func TestNewTestSchedule(t *testing.T) {
	commandID := uuid.New().String()
	userID := uuid.New().String()
	s := newTestSchedule(commandID, userID)

	if s.CommandID != commandID {
		t.Errorf("expected command_id %s, got %s", commandID, s.CommandID)
	}
	if s.CreatedByUser != userID {
		t.Errorf("expected created_by_user %s, got %s", userID, s.CreatedByUser)
	}
	if s.ID == "" {
		t.Error("expected non-empty ID")
	}
	if s.CommandName != "test-command" {
		t.Errorf("expected command_name 'test-command', got %s", s.CommandName)
	}
	if s.CronExpression != "0 * * * *" {
		t.Errorf("expected cron_expression '0 * * * *', got %s", s.CronExpression)
	}
	if s.IntervalSec == nil || *s.IntervalSec != 3600 {
		t.Errorf("expected interval_seconds 3600, got %v", s.IntervalSec)
	}
	if s.Parameters == nil {
		t.Error("expected non-nil Parameters")
	}
	if !s.IsEnabled {
		t.Error("expected IsEnabled to be true")
	}
	if s.NextRunAt == nil {
		t.Error("expected non-nil NextRunAt")
	}
	if s.LastRunAt != nil {
		t.Error("expected nil LastRunAt for new schedule")
	}
	if s.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
	if s.UpdatedAt.IsZero() {
		t.Error("expected non-zero UpdatedAt")
	}
}

func TestNewTestSchedule_UniqueIDs(t *testing.T) {
	commandID := uuid.New().String()
	userID := uuid.New().String()
	s1 := newTestSchedule(commandID, userID)
	s2 := newTestSchedule(commandID, userID)
	if s1.ID == s2.ID {
		t.Error("expected different IDs for different schedules")
	}
}

func TestNewTestSchedule_NilParameters(t *testing.T) {
	s := newTestSchedule(uuid.New().String(), uuid.New().String())
	s.Parameters = nil
	if s.Parameters != nil {
		t.Error("expected nil Parameters after explicit set")
	}
}

func TestNewTestSchedule_DisabledSchedule(t *testing.T) {
	s := newTestSchedule(uuid.New().String(), uuid.New().String())
	s.IsEnabled = false
	if s.IsEnabled {
		t.Error("expected IsEnabled to be false")
	}
}

func TestNewTestSchedule_WithLastRunAt(t *testing.T) {
	s := newTestSchedule(uuid.New().String(), uuid.New().String())
	lastRun := time.Now().UTC().Truncate(time.Microsecond)
	s.LastRunAt = &lastRun
	if s.LastRunAt == nil {
		t.Error("expected non-nil LastRunAt")
	}
	if !s.LastRunAt.Equal(lastRun) {
		t.Errorf("expected LastRunAt %v, got %v", lastRun, *s.LastRunAt)
	}
}

func TestScheduleFilter_Defaults(t *testing.T) {
	filter := models.ScheduleFilter{}
	if filter.CommandID != "" {
		t.Error("expected empty CommandID by default")
	}
	if filter.IsEnabled != nil {
		t.Error("expected nil IsEnabled by default")
	}
	if filter.Roles != nil {
		t.Error("expected nil Roles by default")
	}
	if filter.Page != 0 {
		t.Errorf("expected Page 0 by default, got %d", filter.Page)
	}
	if filter.PageSize != 0 {
		t.Errorf("expected PageSize 0 by default, got %d", filter.PageSize)
	}
}

func TestScheduleFilter_WithRoles(t *testing.T) {
	filter := models.ScheduleFilter{
		Roles:    []models.Role{models.RoleAdmin, models.RoleQA},
		Page:     1,
		PageSize: 10,
	}
	if len(filter.Roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(filter.Roles))
	}
	if filter.Roles[0] != models.RoleAdmin {
		t.Errorf("expected first role admin, got %s", filter.Roles[0])
	}
	if filter.Roles[1] != models.RoleQA {
		t.Errorf("expected second role qa, got %s", filter.Roles[1])
	}
}
