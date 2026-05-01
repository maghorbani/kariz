package notification

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/kariz/kariz/internal/config"
	"github.com/kariz/kariz/internal/models"
)

// NotificationService manages in-app and optional email notifications.
type NotificationService interface {
	NotifyExecutionComplete(ctx context.Context, execution models.ExecutionRecord) error
	GetNotifications(ctx context.Context, userID string, unreadOnly bool) ([]models.Notification, error)
	MarkAsRead(ctx context.Context, notificationID string) error
}

// notificationService is the concrete implementation of NotificationService.
type notificationService struct {
	repo NotificationRepository
	cfg  *config.Config
}

// NewNotificationService creates a new NotificationService with the given dependencies.
func NewNotificationService(repo NotificationRepository, cfg *config.Config) NotificationService {
	return &notificationService{
		repo: repo,
		cfg:  cfg,
	}
}

// NotifyExecutionComplete creates an in-app notification for the user who triggered
// the execution. The notification type is mapped from the execution's terminal status:
//   - completed → NotifyComplete
//   - failed    → NotifyFailed
//   - timed_out → NotifyTimedOut
//
// If SMTP is configured, it also attempts to send an email notification.
func (s *notificationService) NotifyExecutionComplete(ctx context.Context, execution models.ExecutionRecord) error {
	notifType := mapStatusToNotificationType(execution.Status)
	if notifType == "" {
		// Not a terminal status we notify on — skip silently.
		return nil
	}

	title := fmt.Sprintf("Command '%s' %s", execution.CommandName, statusLabel(execution.Status))
	message := fmt.Sprintf(
		"Command '%s' finished with status: %s. Execution ID: %s",
		execution.CommandName,
		string(execution.Status),
		execution.ID,
	)

	notification := &models.Notification{
		ID:          uuid.New().String(),
		UserID:      execution.UserID,
		Type:        notifType,
		Title:       title,
		Message:     message,
		ExecutionID: execution.ID,
		IsRead:      false,
		CreatedAt:   time.Now().UTC(),
	}

	if err := s.repo.Create(ctx, notification); err != nil {
		return fmt.Errorf("create notification: %w", err)
	}

	// Attempt email notification if SMTP is configured.
	if s.cfg != nil && s.cfg.SMTPConfigured() {
		s.sendEmailNotification(execution)
	}

	return nil
}

// GetNotifications returns notifications for the given user.
// If unreadOnly is true, only unread notifications are returned.
func (s *notificationService) GetNotifications(ctx context.Context, userID string, unreadOnly bool) ([]models.Notification, error) {
	return s.repo.GetByUserID(ctx, userID, unreadOnly)
}

// MarkAsRead marks a notification as read.
func (s *notificationService) MarkAsRead(ctx context.Context, notificationID string) error {
	return s.repo.MarkAsRead(ctx, notificationID)
}

// mapStatusToNotificationType maps an execution status to the corresponding notification type.
// Returns an empty string for statuses that don't trigger notifications.
func mapStatusToNotificationType(status models.ExecutionStatus) models.NotificationType {
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

// statusLabel returns a human-readable label for the execution status.
func statusLabel(status models.ExecutionStatus) string {
	switch status {
	case models.StatusCompleted:
		return "completed"
	case models.StatusFailed:
		return "failed"
	case models.StatusTimedOut:
		return "timed out"
	default:
		return string(status)
	}
}

// sendEmailNotification is a placeholder for email notification delivery.
// When full SMTP support is implemented, this will use net/smtp to send an email.
func (s *notificationService) sendEmailNotification(execution models.ExecutionRecord) {
	subject := fmt.Sprintf("KARIZ: Command '%s' %s", execution.CommandName, statusLabel(execution.Status))
	appURL := "http://localhost:8080"
	if s.cfg != nil && s.cfg.AppURL != "" {
		appURL = s.cfg.AppURL
	}
	link := fmt.Sprintf("%s/executions/%s", appURL, execution.ID)

	slog.Info("email notification would be sent",
		"to_user", execution.UserID,
		"subject", subject,
		"command", execution.CommandName,
		"status", string(execution.Status),
		"execution_link", link,
	)
}
