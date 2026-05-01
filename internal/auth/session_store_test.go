package auth

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kariz/kariz/internal/models"
)

// mockSessionRepo is a minimal in-memory SessionRepository for unit testing the session store.
type mockSessionRepo struct {
	sessions map[string]*models.Session // keyed by session token

	createErr       error
	getByTokenErr   error
	deleteErr       error
	deleteByUserErr error
	cleanExpiredErr error
	cleanedCount    int64

	lastCreated *models.Session
}

func newMockSessionRepo() *mockSessionRepo {
	return &mockSessionRepo{
		sessions: make(map[string]*models.Session),
	}
}

func (m *mockSessionRepo) Create(_ context.Context, session *models.Session) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.sessions[session.SessionToken] = session
	m.lastCreated = session
	return nil
}

func (m *mockSessionRepo) GetByToken(_ context.Context, token string) (*models.Session, error) {
	if m.getByTokenErr != nil {
		return nil, m.getByTokenErr
	}
	s, ok := m.sessions[token]
	if !ok {
		return nil, nil
	}
	// Simulate expired session filtering (like the real repo does with SQL)
	if s.ExpiresAt.Before(time.Now()) {
		return nil, nil
	}
	return s, nil
}

func (m *mockSessionRepo) Delete(_ context.Context, token string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.sessions, token)
	return nil
}

func (m *mockSessionRepo) DeleteByUserID(_ context.Context, userID string) error {
	if m.deleteByUserErr != nil {
		return m.deleteByUserErr
	}
	for token, s := range m.sessions {
		if s.UserID == userID {
			delete(m.sessions, token)
		}
	}
	return nil
}

func (m *mockSessionRepo) CleanExpired(_ context.Context) (int64, error) {
	if m.cleanExpiredErr != nil {
		return 0, m.cleanExpiredErr
	}
	var count int64
	for token, s := range m.sessions {
		if s.ExpiresAt.Before(time.Now()) {
			delete(m.sessions, token)
			count++
		}
	}
	m.cleanedCount = count
	return count, nil
}

// --- Interface compliance ---

func TestSessionStoreImplementsInterface(t *testing.T) {
	var _ SessionStore = (*sessionStore)(nil)
}

// --- Constructor ---

func TestNewSessionStore(t *testing.T) {
	store := NewSessionStore(newMockSessionRepo(), time.Hour)
	if store == nil {
		t.Fatal("expected non-nil session store")
	}
}

func TestNewSessionStore_ReturnsCorrectType(t *testing.T) {
	store := NewSessionStore(newMockSessionRepo(), time.Hour)
	if _, ok := store.(*sessionStore); !ok {
		t.Fatal("expected *sessionStore type")
	}
}

// --- Create ---

func TestCreate_ReturnsToken(t *testing.T) {
	repo := newMockSessionRepo()
	store := NewSessionStore(repo, time.Hour)

	token, err := store.Create(context.Background(), models.SessionData{
		UserID:   "u1",
		Username: "alice",
		Roles:    []models.Role{models.RoleAdmin},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
	// Token should be 64 hex characters (32 bytes)
	if len(token) != 64 {
		t.Errorf("expected 64-char hex token, got %d chars", len(token))
	}
}

func TestCreate_TokenIsHexEncoded(t *testing.T) {
	repo := newMockSessionRepo()
	store := NewSessionStore(repo, time.Hour)

	token, err := store.Create(context.Background(), models.SessionData{
		UserID:   "u1",
		Username: "alice",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify all characters are valid hex
	for _, c := range token {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("token contains non-hex character: %c", c)
		}
	}
}

func TestCreate_UniqueTokens(t *testing.T) {
	repo := newMockSessionRepo()
	store := NewSessionStore(repo, time.Hour)

	token1, err := store.Create(context.Background(), models.SessionData{
		UserID:   "u1",
		Username: "alice",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	token2, err := store.Create(context.Background(), models.SessionData{
		UserID:   "u1",
		Username: "alice",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if token1 == token2 {
		t.Error("expected unique tokens for different sessions")
	}
}

func TestCreate_StoresSessionInRepo(t *testing.T) {
	repo := newMockSessionRepo()
	store := NewSessionStore(repo, time.Hour)

	token, err := store.Create(context.Background(), models.SessionData{
		UserID:   "u1",
		Username: "alice",
		Roles:    []models.Role{models.RoleAdmin},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	session, ok := repo.sessions[token]
	if !ok {
		t.Fatal("expected session to be stored in repo")
	}
	if session.UserID != "u1" {
		t.Errorf("expected user_id u1, got %s", session.UserID)
	}
	if session.SessionToken != token {
		t.Errorf("expected session token to match returned token")
	}
	if session.ID == "" {
		t.Error("expected non-empty session ID")
	}
}

func TestCreate_SetsExpiresAt(t *testing.T) {
	repo := newMockSessionRepo()
	ttl := 2 * time.Hour
	store := NewSessionStore(repo, ttl)

	before := time.Now().UTC()
	_, err := store.Create(context.Background(), models.SessionData{
		UserID:   "u1",
		Username: "alice",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after := time.Now().UTC()

	session := repo.lastCreated
	expectedMin := before.Add(ttl)
	expectedMax := after.Add(ttl)

	if session.ExpiresAt.Before(expectedMin) || session.ExpiresAt.After(expectedMax) {
		t.Errorf("expires_at %v not in expected range [%v, %v]", session.ExpiresAt, expectedMin, expectedMax)
	}
}

func TestCreate_MarshalSessionData(t *testing.T) {
	repo := newMockSessionRepo()
	store := NewSessionStore(repo, time.Hour)

	_, err := store.Create(context.Background(), models.SessionData{
		UserID:   "u1",
		Username: "alice",
		Roles:    []models.Role{models.RoleAdmin, models.RoleQA},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	session := repo.lastCreated
	var data models.SessionData
	if err := json.Unmarshal([]byte(session.SessionData), &data); err != nil {
		t.Fatalf("failed to unmarshal session data: %v", err)
	}
	if data.UserID != "u1" {
		t.Errorf("expected user_id u1, got %s", data.UserID)
	}
	if data.Username != "alice" {
		t.Errorf("expected username alice, got %s", data.Username)
	}
	if len(data.Roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(data.Roles))
	}
}

func TestCreate_RepoError(t *testing.T) {
	repo := newMockSessionRepo()
	repo.createErr = errors.New("db write failed")
	store := NewSessionStore(repo, time.Hour)

	_, err := store.Create(context.Background(), models.SessionData{
		UserID:   "u1",
		Username: "alice",
	})
	if err == nil {
		t.Fatal("expected error from repo failure")
	}
}

// --- Get ---

func TestGet_ReturnsSessionData(t *testing.T) {
	repo := newMockSessionRepo()
	store := NewSessionStore(repo, time.Hour)

	token, err := store.Create(context.Background(), models.SessionData{
		UserID:   "u1",
		Username: "alice",
		Roles:    []models.Role{models.RoleAdmin},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := store.Get(context.Background(), token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil session data")
	}
	if data.UserID != "u1" {
		t.Errorf("expected user_id u1, got %s", data.UserID)
	}
	if data.Username != "alice" {
		t.Errorf("expected username alice, got %s", data.Username)
	}
	if len(data.Roles) != 1 || data.Roles[0] != models.RoleAdmin {
		t.Errorf("expected [admin] roles, got %v", data.Roles)
	}
}

func TestGet_NotFound(t *testing.T) {
	repo := newMockSessionRepo()
	store := NewSessionStore(repo, time.Hour)

	data, err := store.Get(context.Background(), "nonexistent-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data != nil {
		t.Fatal("expected nil for nonexistent token")
	}
}

func TestGet_ExpiredSession(t *testing.T) {
	repo := newMockSessionRepo()
	// Use a very short TTL so the session expires immediately
	store := NewSessionStore(repo, time.Nanosecond)

	token, err := store.Create(context.Background(), models.SessionData{
		UserID:   "u1",
		Username: "alice",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait a moment to ensure expiration
	time.Sleep(time.Millisecond)

	data, err := store.Get(context.Background(), token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data != nil {
		t.Fatal("expected nil for expired session")
	}
}

func TestGet_RepoError(t *testing.T) {
	repo := newMockSessionRepo()
	repo.getByTokenErr = errors.New("db read failed")
	store := NewSessionStore(repo, time.Hour)

	_, err := store.Get(context.Background(), "some-token")
	if err == nil {
		t.Fatal("expected error from repo failure")
	}
}

// --- Delete ---

func TestDelete_RemovesSession(t *testing.T) {
	repo := newMockSessionRepo()
	store := NewSessionStore(repo, time.Hour)

	token, err := store.Create(context.Background(), models.SessionData{
		UserID:   "u1",
		Username: "alice",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := store.Delete(context.Background(), token); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := store.Get(context.Background(), token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data != nil {
		t.Fatal("expected nil after deletion")
	}
}

func TestDelete_RepoError(t *testing.T) {
	repo := newMockSessionRepo()
	repo.deleteErr = errors.New("db delete failed")
	store := NewSessionStore(repo, time.Hour)

	err := store.Delete(context.Background(), "some-token")
	if err == nil {
		t.Fatal("expected error from repo failure")
	}
}

// --- DeleteByUserID ---

func TestDeleteByUserID_RemovesAllUserSessions(t *testing.T) {
	repo := newMockSessionRepo()
	store := NewSessionStore(repo, time.Hour)

	// Create two sessions for the same user
	token1, err := store.Create(context.Background(), models.SessionData{
		UserID:   "u1",
		Username: "alice",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	token2, err := store.Create(context.Background(), models.SessionData{
		UserID:   "u1",
		Username: "alice",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Create a session for a different user
	token3, err := store.Create(context.Background(), models.SessionData{
		UserID:   "u2",
		Username: "bob",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := store.DeleteByUserID(context.Background(), "u1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// u1's sessions should be gone
	data1, _ := store.Get(context.Background(), token1)
	data2, _ := store.Get(context.Background(), token2)
	if data1 != nil || data2 != nil {
		t.Error("expected u1's sessions to be deleted")
	}

	// u2's session should remain
	data3, _ := store.Get(context.Background(), token3)
	if data3 == nil {
		t.Error("expected u2's session to remain")
	}
}

func TestDeleteByUserID_RepoError(t *testing.T) {
	repo := newMockSessionRepo()
	repo.deleteByUserErr = errors.New("db delete failed")
	store := NewSessionStore(repo, time.Hour)

	err := store.DeleteByUserID(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected error from repo failure")
	}
}

// --- CleanExpired ---

func TestCleanExpired_DelegatesToRepo(t *testing.T) {
	repo := newMockSessionRepo()
	store := NewSessionStore(repo, time.Hour)

	err := store.CleanExpired(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCleanExpired_RepoError(t *testing.T) {
	repo := newMockSessionRepo()
	repo.cleanExpiredErr = errors.New("db clean failed")
	store := NewSessionStore(repo, time.Hour)

	err := store.CleanExpired(context.Background())
	if err == nil {
		t.Fatal("expected error from repo failure")
	}
}
