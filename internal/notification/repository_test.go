package notification

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kariz/kariz/internal/models"
)

// newTestNotification creates a Notification with sensible defaults for testing.
func newTestNotification(userID string) *models.Notification {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return &models.Notification{
		ID:          uuid.New().String(),
		UserID:      userID,
		Type:        models.NotifyComplete,
		Title:       "Execution completed",
		Message:     "Command 'deploy' finished successfully.",
		ExecutionID: uuid.New().String(),
		IsRead:      false,
		CreatedAt:   now,
	}
}

// --- Interface compliance ---

func TestNotificationRepositoryImplementsInterface(t *testing.T) {
	// Compile-time check that notificationRepository satisfies NotificationRepository.
	var _ NotificationRepository = (*notificationRepository)(nil)
}

// --- Constructor ---

func TestNewNotificationRepository(t *testing.T) {
	repo := NewNotificationRepository(nil)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}

func TestNewNotificationRepository_ReturnsCorrectType(t *testing.T) {
	repo := NewNotificationRepository(nil)
	if _, ok := repo.(*notificationRepository); !ok {
		t.Fatal("expected *notificationRepository type")
	}
}

// --- Model helpers ---

func TestNewTestNotification(t *testing.T) {
	userID := uuid.New().String()
	n := newTestNotification(userID)
	if n.UserID != userID {
		t.Errorf("expected user_id %s, got %s", userID, n.UserID)
	}
	if n.ID == "" {
		t.Error("expected non-empty ID")
	}
	if n.Type != models.NotifyComplete {
		t.Errorf("expected type %s, got %s", models.NotifyComplete, n.Type)
	}
	if n.Title == "" {
		t.Error("expected non-empty Title")
	}
	if n.Message == "" {
		t.Error("expected non-empty Message")
	}
	if n.ExecutionID == "" {
		t.Error("expected non-empty ExecutionID")
	}
	if n.IsRead {
		t.Error("expected IsRead to be false")
	}
	if n.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestNewTestNotification_UniqueIDs(t *testing.T) {
	userID := uuid.New().String()
	n1 := newTestNotification(userID)
	n2 := newTestNotification(userID)
	if n1.ID == n2.ID {
		t.Error("expected different IDs for different notifications")
	}
	if n1.ExecutionID == n2.ExecutionID {
		t.Error("expected different ExecutionIDs for different notifications")
	}
}

func TestNewTestNotification_NotificationTypes(t *testing.T) {
	tests := []struct {
		name     string
		notifType models.NotificationType
	}{
		{"complete", models.NotifyComplete},
		{"failed", models.NotifyFailed},
		{"timed_out", models.NotifyTimedOut},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := newTestNotification(uuid.New().String())
			n.Type = tt.notifType
			if n.Type != tt.notifType {
				t.Errorf("expected type %s, got %s", tt.notifType, n.Type)
			}
		})
	}
}
