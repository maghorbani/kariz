package property_test

// Feature: kariz-command-dashboard, Property 16: notification creation on terminal status

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kariz/kariz/internal/models"
	"github.com/kariz/kariz/internal/notification"
	"pgregory.net/rapid"
)

// --- Mock repository for Property 16 ---

type prop16MockRepo struct {
	notifications []*models.Notification
}

func newProp16MockRepo() *prop16MockRepo {
	return &prop16MockRepo{
		notifications: make([]*models.Notification, 0),
	}
}

func (m *prop16MockRepo) Create(_ context.Context, n *models.Notification) error {
	m.notifications = append(m.notifications, n)
	return nil
}

func (m *prop16MockRepo) GetByUserID(_ context.Context, userID string, unreadOnly bool) ([]models.Notification, error) {
	var result []models.Notification
	for _, n := range m.notifications {
		if n.UserID == userID {
			if unreadOnly && n.IsRead {
				continue
			}
			result = append(result, *n)
		}
	}
	if result == nil {
		result = []models.Notification{}
	}
	return result, nil
}

func (m *prop16MockRepo) MarkAsRead(_ context.Context, id string) error {
	for _, n := range m.notifications {
		if n.ID == id {
			n.IsRead = true
			return nil
		}
	}
	return fmt.Errorf("notification not found")
}

// --- Generators ---

// genTerminalStatus generates a random terminal execution status.
func genTerminalStatus(t *rapid.T) models.ExecutionStatus {
	statuses := []models.ExecutionStatus{
		models.StatusCompleted,
		models.StatusFailed,
		models.StatusTimedOut,
	}
	idx := rapid.IntRange(0, len(statuses)-1).Draw(t, "terminalStatusIdx")
	return statuses[idx]
}

// genNonTerminalStatus generates a random non-terminal execution status.
func genNonTerminalStatus(t *rapid.T) models.ExecutionStatus {
	statuses := []models.ExecutionStatus{
		models.StatusQueued,
		models.StatusRunning,
		models.StatusCancelled,
	}
	idx := rapid.IntRange(0, len(statuses)-1).Draw(t, "nonTerminalStatusIdx")
	return statuses[idx]
}

// genExecutionRecord generates a random ExecutionRecord with the given status.
func genExecutionRecord(t *rapid.T, status models.ExecutionStatus) models.ExecutionRecord {
	params, _ := json.Marshal(map[string]interface{}{
		"key": rapid.StringMatching(`^[a-z]{3,10}`).Draw(t, "paramKey"),
	})
	now := time.Now().UTC()
	cmdName := rapid.StringMatching(`^[a-z][a-z0-9-]{2,15}`).Draw(t, "commandName")

	return models.ExecutionRecord{
		ID:          uuid.New().String(),
		CommandID:   uuid.New().String(),
		CommandName: cmdName,
		UserID:      uuid.New().String(),
		Parameters:  params,
		Status:      status,
		CreatedAt:   now,
		StartedAt:   &now,
		CompletedAt: &now,
	}
}

// expectedNotificationType returns the expected notification type for a terminal status.
func expectedNotificationType(status models.ExecutionStatus) models.NotificationType {
	switch status {
	case models.StatusCompleted:
		return models.NotifyComplete
	case models.StatusFailed:
		return models.NotifyFailed
	case models.StatusTimedOut:
		return models.NotifyTimedOut
	default:
		return ""
	}
}

// --- Property 16 Tests ---
// **Validates: Requirements 10.1**

// TestProperty16_TerminalStatusCreatesNotification tests that for any execution
// that reaches a terminal status (completed, failed, or timed_out), a notification
// is created for the user who initiated the execution, with a type matching the
// terminal status and referencing the correct execution ID.
func TestProperty16_TerminalStatusCreatesNotification(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		status := genTerminalStatus(t)
		exec := genExecutionRecord(t, status)

		repo := newProp16MockRepo()
		svc := notification.NewNotificationService(repo, nil)

		err := svc.NotifyExecutionComplete(context.Background(), exec)
		if err != nil {
			t.Fatalf("unexpected error for terminal status %s: %v", status, err)
		}

		// Verify a notification was created
		if len(repo.notifications) != 1 {
			t.Fatalf("expected 1 notification for status %s, got %d", status, len(repo.notifications))
		}

		n := repo.notifications[0]

		// Verify notification type matches the status mapping
		expectedType := expectedNotificationType(status)
		if n.Type != expectedType {
			t.Fatalf("status %s: expected notification type %s, got %s", status, expectedType, n.Type)
		}

		// Verify notification references the correct execution ID
		if n.ExecutionID != exec.ID {
			t.Fatalf("expected execution_id %s, got %s", exec.ID, n.ExecutionID)
		}

		// Verify notification is for the correct user ID
		if n.UserID != exec.UserID {
			t.Fatalf("expected user_id %s, got %s", exec.UserID, n.UserID)
		}

		// Verify notification has a non-empty ID, title, and message
		if n.ID == "" {
			t.Fatal("expected non-empty notification ID")
		}
		if n.Title == "" {
			t.Fatal("expected non-empty notification title")
		}
		if n.Message == "" {
			t.Fatal("expected non-empty notification message")
		}

		// Verify notification starts as unread
		if n.IsRead {
			t.Fatal("expected notification to be unread (IsRead=false)")
		}
	})
}

// TestProperty16_NonTerminalStatusDoesNotCreateNotification tests that non-terminal
// statuses (queued, running, cancelled) do NOT create notifications.
func TestProperty16_NonTerminalStatusDoesNotCreateNotification(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		status := genNonTerminalStatus(t)
		exec := genExecutionRecord(t, status)

		repo := newProp16MockRepo()
		svc := notification.NewNotificationService(repo, nil)

		err := svc.NotifyExecutionComplete(context.Background(), exec)
		if err != nil {
			t.Fatalf("unexpected error for non-terminal status %s: %v", status, err)
		}

		// Verify NO notification was created
		if len(repo.notifications) != 0 {
			t.Fatalf("expected 0 notifications for non-terminal status %s, got %d", status, len(repo.notifications))
		}
	})
}
