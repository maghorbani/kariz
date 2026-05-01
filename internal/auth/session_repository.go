package auth

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kariz/kariz/internal/models"
)

// SessionRepository defines the data access interface for sessions.
type SessionRepository interface {
	Create(ctx context.Context, session *models.Session) error
	GetByToken(ctx context.Context, token string) (*models.Session, error)
	Delete(ctx context.Context, token string) error
	DeleteByUserID(ctx context.Context, userID string) error
	CleanExpired(ctx context.Context) (int64, error)
}

// sessionRepository implements SessionRepository using sqlx.
type sessionRepository struct {
	db *sqlx.DB
}

// NewSessionRepository creates a new SessionRepository backed by the given database.
func NewSessionRepository(db *sqlx.DB) SessionRepository {
	return &sessionRepository{db: db}
}

// Create inserts a new session into the sessions table.
func (r *sessionRepository) Create(ctx context.Context, session *models.Session) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, session_token, session_data, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		session.ID, session.UserID, session.SessionToken,
		session.SessionData, session.ExpiresAt, session.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// GetByToken retrieves a session by its token, only if it has not expired.
// Returns nil, nil if the session is not found or has expired.
func (r *sessionRepository) GetByToken(ctx context.Context, token string) (*models.Session, error) {
	var session models.Session
	err := r.db.GetContext(ctx, &session, `
		SELECT id, user_id, session_token, session_data, expires_at, created_at
		FROM sessions
		WHERE session_token = $1 AND expires_at > NOW()`, token)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get session by token: %w", err)
	}
	return &session, nil
}

// Delete removes a session by its token.
func (r *sessionRepository) Delete(ctx context.Context, token string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM sessions WHERE session_token = $1`, token)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteByUserID removes all sessions for a given user.
// This is used when roles change to force re-authentication (Req 5.4).
func (r *sessionRepository) DeleteByUserID(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM sessions WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete sessions by user id: %w", err)
	}
	return nil
}

// CleanExpired removes all expired sessions and returns the number of rows deleted.
func (r *sessionRepository) CleanExpired(ctx context.Context) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM sessions WHERE expires_at <= NOW()`)
	if err != nil {
		return 0, fmt.Errorf("clean expired sessions: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("get rows affected: %w", err)
	}
	return count, nil
}
