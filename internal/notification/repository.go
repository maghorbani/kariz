package notification

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kariz/kariz/internal/models"
)

// NotificationRepository defines the data access interface for notifications.
type NotificationRepository interface {
	Create(ctx context.Context, notification *models.Notification) error
	GetByUserID(ctx context.Context, userID string, unreadOnly bool) ([]models.Notification, error)
	MarkAsRead(ctx context.Context, id string) error
}

// notificationRepository implements NotificationRepository using sqlx.
type notificationRepository struct {
	db *sqlx.DB
}

// NewNotificationRepository creates a new NotificationRepository backed by the given database.
func NewNotificationRepository(db *sqlx.DB) NotificationRepository {
	return &notificationRepository{db: db}
}

// Create inserts a new notification into the notifications table.
func (r *notificationRepository) Create(ctx context.Context, notification *models.Notification) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO notifications (id, user_id, type, title, message, execution_id, is_read, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		notification.ID, notification.UserID, notification.Type,
		notification.Title, notification.Message, notification.ExecutionID,
		notification.IsRead, notification.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert notification: %w", err)
	}
	return nil
}

// GetByUserID retrieves notifications for a given user, ordered by created_at descending.
// If unreadOnly is true, only unread notifications are returned.
func (r *notificationRepository) GetByUserID(ctx context.Context, userID string, unreadOnly bool) ([]models.Notification, error) {
	var notifications []models.Notification
	var err error

	if unreadOnly {
		err = r.db.SelectContext(ctx, &notifications, `
			SELECT id, user_id, type, title, message, execution_id, is_read, created_at
			FROM notifications
			WHERE user_id = $1 AND is_read = false
			ORDER BY created_at DESC`, userID)
	} else {
		err = r.db.SelectContext(ctx, &notifications, `
			SELECT id, user_id, type, title, message, execution_id, is_read, created_at
			FROM notifications
			WHERE user_id = $1
			ORDER BY created_at DESC`, userID)
	}

	if err != nil {
		return nil, fmt.Errorf("get notifications by user id: %w", err)
	}

	if notifications == nil {
		notifications = []models.Notification{}
	}

	return notifications, nil
}

// MarkAsRead sets is_read to true for the notification with the given ID.
// Returns an error wrapping sql.ErrNoRows if the notification is not found.
func (r *notificationRepository) MarkAsRead(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE notifications SET is_read = true WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("mark notification as read: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("notification not found: %w", sql.ErrNoRows)
	}

	return nil
}
