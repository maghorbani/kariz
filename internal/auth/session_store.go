package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kariz/kariz/internal/models"
)

// SessionStore manages server-side sessions in PostgreSQL.
type SessionStore interface {
	Create(ctx context.Context, data models.SessionData) (sessionToken string, err error)
	Get(ctx context.Context, sessionToken string) (*models.SessionData, error)
	Delete(ctx context.Context, sessionToken string) error
	DeleteByUserID(ctx context.Context, userID string) error
	CleanExpired(ctx context.Context) error
}

// sessionStore implements SessionStore using a SessionRepository.
type sessionStore struct {
	repo SessionRepository
	ttl  time.Duration
}

// NewSessionStore creates a new SessionStore backed by the given repository and session TTL.
func NewSessionStore(repo SessionRepository, ttl time.Duration) SessionStore {
	return &sessionStore{
		repo: repo,
		ttl:  ttl,
	}
}

// Create generates a secure random session token, marshals the session data to JSON,
// and persists the session via the repository. Returns the hex-encoded token.
func (s *sessionStore) Create(ctx context.Context, data models.SessionData) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	sessionID := uuid.New().String()

	now := time.Now().UTC()
	expiresAt := now.Add(s.ttl)

	data.CreatedAt = now
	data.ExpiresAt = expiresAt

	sessionDataJSON, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal session data: %w", err)
	}

	session := &models.Session{
		ID:           sessionID,
		UserID:       data.UserID,
		SessionToken: token,
		SessionData:  string(sessionDataJSON),
		ExpiresAt:    expiresAt,
		CreatedAt:    now,
	}

	if err := s.repo.Create(ctx, session); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	return token, nil
}

// Get retrieves a session by token. Returns nil, nil if the session is not found or expired.
func (s *sessionStore) Get(ctx context.Context, sessionToken string) (*models.SessionData, error) {
	session, err := s.repo.GetByToken(ctx, sessionToken)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if session == nil {
		return nil, nil
	}

	var data models.SessionData
	if err := json.Unmarshal([]byte(session.SessionData), &data); err != nil {
		return nil, fmt.Errorf("unmarshal session data: %w", err)
	}

	return &data, nil
}

// Delete removes a session by its token.
func (s *sessionStore) Delete(ctx context.Context, sessionToken string) error {
	return s.repo.Delete(ctx, sessionToken)
}

// DeleteByUserID removes all sessions for a given user.
func (s *sessionStore) DeleteByUserID(ctx context.Context, userID string) error {
	return s.repo.DeleteByUserID(ctx, userID)
}

// CleanExpired removes all expired sessions.
func (s *sessionStore) CleanExpired(ctx context.Context) error {
	_, err := s.repo.CleanExpired(ctx)
	return err
}
