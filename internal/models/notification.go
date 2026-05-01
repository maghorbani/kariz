package models

import "time"

// NotificationType categorizes the reason for a notification.
type NotificationType string

const (
	NotifyComplete NotificationType = "execution_complete"
	NotifyFailed   NotificationType = "execution_failed"
	NotifyTimedOut NotificationType = "execution_timed_out"
)

// Notification represents an in-app notification for a user.
type Notification struct {
	ID          string           `json:"id" db:"id"`
	UserID      string           `json:"user_id" db:"user_id"`
	Type        NotificationType `json:"type" db:"type"`
	Title       string           `json:"title" db:"title"`
	Message     string           `json:"message" db:"message"`
	ExecutionID string           `json:"execution_id" db:"execution_id"`
	IsRead      bool             `json:"is_read" db:"is_read"`
	CreatedAt   time.Time        `json:"created_at" db:"created_at"`
}
