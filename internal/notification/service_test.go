package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kariz/kariz/internal/config"
	"github.com/kariz/kariz/internal/models"
)

// --- Mock repository ---

type mockNotificationRepo struct {
	notifications []*models.Notification
	createErr     error
	getErr        error
	markReadErr   error
	markReadID    string
}

func (m *mockNotificationRepo) Create(_ context.Context, n *models.Notification) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.notifications = append(m.notifications, n)
	return nil
}

func (m *mockNotificationRepo) GetByUserID(_ context.Context, userID string, unreadOnly bool) ([]models.Notification, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	var result []models.Notification
	for _, n := range m.notifications {
		if n.UserID != userID {
			continue
		}
		if unreadOnly && n.IsRead {
			continue
		}
		result = append(result, *n)
	}
	if result == nil {
		result = []models.Notification{}
	}
	return result, nil
}

func (m *mockNotificationRepo) MarkAsRead(_ context.Context, id string) error {
	m.markReadID = id
	if m.markReadErr != nil {
		return m.markReadErr
	}
	for _, n := range m.notifications {
		if n.ID == id {
			n.IsRead = true
			return nil
		}
	}
	return fmt.Errorf("notification not found")
}

// --- Helpers ---

func newTestExecution(status models.ExecutionStatus) models.ExecutionRecord {
	params, _ := json.Marshal(map[string]interface{}{"env": "staging"})
	now := time.Now().UTC()
	return models.ExecutionRecord{
		ID:          uuid.New().String(),
		CommandID:   uuid.New().String(),
		CommandName: "deploy-service",
		UserID:      uuid.New().String(),
		Parameters:  params,
		Status:      status,
		CreatedAt:   now,
		StartedAt:   &now,
		CompletedAt: &now,
	}
}

func newTestConfig() *config.Config {
	return &config.Config{
		AppURL: "http://localhost:8080",
	}
}

func newTestConfigWithSMTP() *config.Config {
	return &config.Config{
		AppURL:   "https://kariz.example.com",
		SMTPHost: "smtp.example.com",
		SMTPPort: "587",
		SMTPFrom: "kariz@example.com",
	}
}

// --- Interface compliance ---

func TestNotificationServiceImplementsInterface(t *testing.T) {
	var _ NotificationService = (*notificationService)(nil)
}

// --- Constructor ---

func TestNewNotificationService(t *testing.T) {
	repo := &mockNotificationRepo{}
	cfg := newTestConfig()
	svc := NewNotificationService(repo, cfg)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

// --- NotifyExecutionComplete ---

func TestNotifyExecutionComplete_Completed(t *testing.T) {
	repo := &mockNotificationRepo{}
	svc := NewNotificationService(repo, newTestConfig())

	exec := newTestExecution(models.StatusCompleted)
	err := svc.NotifyExecutionComplete(context.Background(), exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repo.notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(repo.notifications))
	}

	n := repo.notifications[0]
	if n.Type != models.NotifyComplete {
		t.Errorf("expected type %s, got %s", models.NotifyComplete, n.Type)
	}
	if n.UserID != exec.UserID {
		t.Errorf("expected user_id %s, got %s", exec.UserID, n.UserID)
	}
	if n.ExecutionID != exec.ID {
		t.Errorf("expected execution_id %s, got %s", exec.ID, n.ExecutionID)
	}
	if n.IsRead {
		t.Error("expected IsRead to be false")
	}
	if n.ID == "" {
		t.Error("expected non-empty notification ID")
	}
	if n.Title == "" {
		t.Error("expected non-empty title")
	}
	if n.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestNotifyExecutionComplete_Failed(t *testing.T) {
	repo := &mockNotificationRepo{}
	svc := NewNotificationService(repo, newTestConfig())

	exec := newTestExecution(models.StatusFailed)
	err := svc.NotifyExecutionComplete(context.Background(), exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repo.notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(repo.notifications))
	}
	if repo.notifications[0].Type != models.NotifyFailed {
		t.Errorf("expected type %s, got %s", models.NotifyFailed, repo.notifications[0].Type)
	}
}

func TestNotifyExecutionComplete_TimedOut(t *testing.T) {
	repo := &mockNotificationRepo{}
	svc := NewNotificationService(repo, newTestConfig())

	exec := newTestExecution(models.StatusTimedOut)
	err := svc.NotifyExecutionComplete(context.Background(), exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repo.notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(repo.notifications))
	}
	if repo.notifications[0].Type != models.NotifyTimedOut {
		t.Errorf("expected type %s, got %s", models.NotifyTimedOut, repo.notifications[0].Type)
	}
}

func TestNotifyExecutionComplete_NonTerminalStatus_Skipped(t *testing.T) {
	nonTerminal := []models.ExecutionStatus{
		models.StatusQueued,
		models.StatusRunning,
		models.StatusCancelled,
	}

	for _, status := range nonTerminal {
		t.Run(string(status), func(t *testing.T) {
			repo := &mockNotificationRepo{}
			svc := NewNotificationService(repo, newTestConfig())

			exec := newTestExecution(status)
			err := svc.NotifyExecutionComplete(context.Background(), exec)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(repo.notifications) != 0 {
				t.Errorf("expected 0 notifications for status %s, got %d", status, len(repo.notifications))
			}
		})
	}
}

func TestNotifyExecutionComplete_RepoError(t *testing.T) {
	repo := &mockNotificationRepo{createErr: fmt.Errorf("db connection lost")}
	svc := NewNotificationService(repo, newTestConfig())

	exec := newTestExecution(models.StatusCompleted)
	err := svc.NotifyExecutionComplete(context.Background(), exec)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestNotifyExecutionComplete_WithSMTP(t *testing.T) {
	// Verify that SMTP-configured service doesn't error (placeholder logs only).
	repo := &mockNotificationRepo{}
	svc := NewNotificationService(repo, newTestConfigWithSMTP())

	exec := newTestExecution(models.StatusCompleted)
	err := svc.NotifyExecutionComplete(context.Background(), exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(repo.notifications))
	}
}

func TestNotifyExecutionComplete_NilConfig(t *testing.T) {
	repo := &mockNotificationRepo{}
	svc := NewNotificationService(repo, nil)

	exec := newTestExecution(models.StatusCompleted)
	err := svc.NotifyExecutionComplete(context.Background(), exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(repo.notifications))
	}
}

// --- GetNotifications ---

func TestGetNotifications_All(t *testing.T) {
	repo := &mockNotificationRepo{}
	svc := NewNotificationService(repo, newTestConfig())

	userID := uuid.New().String()

	// Create some notifications via the service.
	for _, status := range []models.ExecutionStatus{models.StatusCompleted, models.StatusFailed} {
		exec := newTestExecution(status)
		exec.UserID = userID
		if err := svc.NotifyExecutionComplete(context.Background(), exec); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	notifications, err := svc.GetNotifications(context.Background(), userID, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifications) != 2 {
		t.Errorf("expected 2 notifications, got %d", len(notifications))
	}
}

func TestGetNotifications_UnreadOnly(t *testing.T) {
	repo := &mockNotificationRepo{}
	svc := NewNotificationService(repo, newTestConfig())

	userID := uuid.New().String()

	// Create two notifications.
	exec1 := newTestExecution(models.StatusCompleted)
	exec1.UserID = userID
	if err := svc.NotifyExecutionComplete(context.Background(), exec1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	exec2 := newTestExecution(models.StatusFailed)
	exec2.UserID = userID
	if err := svc.NotifyExecutionComplete(context.Background(), exec2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Mark the first one as read.
	if err := svc.MarkAsRead(context.Background(), repo.notifications[0].ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	notifications, err := svc.GetNotifications(context.Background(), userID, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifications) != 1 {
		t.Errorf("expected 1 unread notification, got %d", len(notifications))
	}
}

func TestGetNotifications_EmptyResult(t *testing.T) {
	repo := &mockNotificationRepo{}
	svc := NewNotificationService(repo, newTestConfig())

	notifications, err := svc.GetNotifications(context.Background(), "nonexistent-user", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifications) != 0 {
		t.Errorf("expected 0 notifications, got %d", len(notifications))
	}
}

func TestGetNotifications_RepoError(t *testing.T) {
	repo := &mockNotificationRepo{getErr: fmt.Errorf("db error")}
	svc := NewNotificationService(repo, newTestConfig())

	_, err := svc.GetNotifications(context.Background(), "user1", false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- MarkAsRead ---

func TestMarkAsRead_Success(t *testing.T) {
	repo := &mockNotificationRepo{}
	svc := NewNotificationService(repo, newTestConfig())

	exec := newTestExecution(models.StatusCompleted)
	if err := svc.NotifyExecutionComplete(context.Background(), exec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	notifID := repo.notifications[0].ID
	err := svc.MarkAsRead(context.Background(), notifID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !repo.notifications[0].IsRead {
		t.Error("expected notification to be marked as read")
	}
}

func TestMarkAsRead_RepoError(t *testing.T) {
	repo := &mockNotificationRepo{markReadErr: fmt.Errorf("db error")}
	svc := NewNotificationService(repo, newTestConfig())

	err := svc.MarkAsRead(context.Background(), "some-id")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- Status mapping ---

func TestMapStatusToNotificationType(t *testing.T) {
	tests := []struct {
		status   models.ExecutionStatus
		expected models.NotificationType
	}{
		{models.StatusCompleted, models.NotifyComplete},
		{models.StatusFailed, models.NotifyFailed},
		{models.StatusTimedOut, models.NotifyTimedOut},
		{models.StatusQueued, ""},
		{models.StatusRunning, ""},
		{models.StatusCancelled, ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := mapStatusToNotificationType(tt.status)
			if got != tt.expected {
				t.Errorf("mapStatusToNotificationType(%s) = %s, want %s", tt.status, got, tt.expected)
			}
		})
	}
}

func TestStatusLabel(t *testing.T) {
	tests := []struct {
		status   models.ExecutionStatus
		expected string
	}{
		{models.StatusCompleted, "completed"},
		{models.StatusFailed, "failed"},
		{models.StatusTimedOut, "timed out"},
		{models.StatusRunning, "running"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := statusLabel(tt.status)
			if got != tt.expected {
				t.Errorf("statusLabel(%s) = %s, want %s", tt.status, got, tt.expected)
			}
		})
	}
}
