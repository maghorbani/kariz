package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kariz/kariz/internal/models"
)

// newTestSession creates a Session with sensible defaults for testing.
func newTestSession(userID string) *models.Session {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return &models.Session{
		ID:           uuid.New().String(),
		UserID:       userID,
		SessionToken: uuid.New().String(),
		SessionData:  `{"user_id":"` + userID + `"}`,
		ExpiresAt:    now.Add(24 * time.Hour),
		CreatedAt:    now,
	}
}

// --- Interface compliance ---

func TestSessionRepositoryImplementsInterface(t *testing.T) {
	// Compile-time check that sessionRepository satisfies SessionRepository.
	var _ SessionRepository = (*sessionRepository)(nil)
}

// --- Constructor ---

func TestNewSessionRepository(t *testing.T) {
	repo := NewSessionRepository(nil)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}

func TestNewSessionRepository_ReturnsCorrectType(t *testing.T) {
	repo := NewSessionRepository(nil)
	if _, ok := repo.(*sessionRepository); !ok {
		t.Fatal("expected *sessionRepository type")
	}
}

// --- Model helpers ---

func TestNewTestSession(t *testing.T) {
	userID := uuid.New().String()
	s := newTestSession(userID)
	if s.UserID != userID {
		t.Errorf("expected user_id %s, got %s", userID, s.UserID)
	}
	if s.ID == "" {
		t.Error("expected non-empty ID")
	}
	if s.SessionToken == "" {
		t.Error("expected non-empty SessionToken")
	}
	if s.SessionData == "" {
		t.Error("expected non-empty SessionData")
	}
	if s.ExpiresAt.Before(time.Now()) {
		t.Error("expected ExpiresAt to be in the future")
	}
	if s.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestNewTestSession_UniqueTokens(t *testing.T) {
	userID := uuid.New().String()
	s1 := newTestSession(userID)
	s2 := newTestSession(userID)
	if s1.SessionToken == s2.SessionToken {
		t.Error("expected different session tokens for different sessions")
	}
	if s1.ID == s2.ID {
		t.Error("expected different IDs for different sessions")
	}
}

func TestNewTestSession_ExpiredSession(t *testing.T) {
	userID := uuid.New().String()
	s := newTestSession(userID)
	// Manually set to expired
	s.ExpiresAt = time.Now().UTC().Add(-1 * time.Hour)
	if s.ExpiresAt.After(time.Now()) {
		t.Error("expected ExpiresAt to be in the past for expired session")
	}
}
