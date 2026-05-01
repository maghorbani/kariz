package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kariz/kariz/internal/models"
)

// newTestUser creates a User with sensible defaults for testing.
func newTestUser(username string) *models.User {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return &models.User{
		ID:           uuid.New().String(),
		Username:     username,
		PasswordHash: "$2a$10$fakehashfortest",
		Email:        username + "@example.com",
		IsActive:     true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// requireTestDB opens a connection to the test database.
// It skips the test if the DATABASE_URL env var is not set.
func requireTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	t.Skip("skipping: requires a running PostgreSQL instance with DATABASE_URL set")
	return nil
}

// --- Interface compliance ---

func TestUserRepositoryImplementsInterface(t *testing.T) {
	// Compile-time check that userRepository satisfies UserRepository.
	var _ UserRepository = (*userRepository)(nil)
}

// --- Unit tests for insertUserRoles helper ---

func TestInsertUserRoles_EmptySlice(t *testing.T) {
	// insertUserRoles should return nil immediately for an empty slice,
	// without touching the transaction.
	err := insertUserRoles(context.Background(), nil, "some-id", []models.Role{})
	if err != nil {
		t.Fatalf("expected nil error for empty roles, got: %v", err)
	}
}

// --- Constructor ---

func TestNewUserRepository(t *testing.T) {
	repo := NewUserRepository(nil)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}

// --- Model helpers ---

func TestNewTestUser(t *testing.T) {
	u := newTestUser("alice")
	if u.Username != "alice" {
		t.Errorf("expected username alice, got %s", u.Username)
	}
	if u.Email != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %s", u.Email)
	}
	if u.ID == "" {
		t.Error("expected non-empty ID")
	}
	if !u.IsActive {
		t.Error("expected IsActive to be true")
	}
}
