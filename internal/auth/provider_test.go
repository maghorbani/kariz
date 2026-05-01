package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/kariz/kariz/internal/models"
	"golang.org/x/crypto/bcrypt"
)

// mockUserRepo is a minimal in-memory UserRepository for unit testing the auth provider.
type mockUserRepo struct {
	users map[string]*models.User // keyed by username
	roles map[string][]models.Role // keyed by user ID

	getByUsernameErr error
	getUserRolesErr  error
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{
		users: make(map[string]*models.User),
		roles: make(map[string][]models.Role),
	}
}

func (m *mockUserRepo) Create(_ context.Context, _ *models.User, _ []models.Role) error {
	return nil
}

func (m *mockUserRepo) GetByID(_ context.Context, id string) (*models.User, error) {
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, nil
}

func (m *mockUserRepo) GetByUsername(_ context.Context, username string) (*models.User, error) {
	if m.getByUsernameErr != nil {
		return nil, m.getByUsernameErr
	}
	u, ok := m.users[username]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *mockUserRepo) GetUserRoles(_ context.Context, userID string) ([]models.Role, error) {
	if m.getUserRolesErr != nil {
		return nil, m.getUserRolesErr
	}
	return m.roles[userID], nil
}

func (m *mockUserRepo) UpdateRoles(_ context.Context, _ string, _ []models.Role) error {
	return nil
}

func (m *mockUserRepo) List(_ context.Context) ([]models.UserProfile, error) {
	return nil, nil
}

// addUser is a helper to insert a user with a bcrypt-hashed password.
func (m *mockUserRepo) addUser(id, username, password, email string, active bool, roles []models.Role) {
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	m.users[username] = &models.User{
		ID:           id,
		Username:     username,
		PasswordHash: string(hash),
		Email:        email,
		IsActive:     active,
	}
	m.roles[id] = roles
}

// --- Tests ---

func TestAuthenticate_Success(t *testing.T) {
	repo := newMockUserRepo()
	repo.addUser("u1", "alice", "secret123", "alice@example.com", true, []models.Role{models.RoleAdmin})

	provider := NewLocalAuthProvider(repo)
	result, err := provider.Authenticate(context.Background(), models.LoginCredentials{
		Username: "alice",
		Password: "secret123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if result.User == nil {
		t.Fatal("expected user profile in result")
	}
	if result.User.ID != "u1" {
		t.Errorf("expected user ID u1, got %s", result.User.ID)
	}
	if result.User.Username != "alice" {
		t.Errorf("expected username alice, got %s", result.User.Username)
	}
	if result.User.Email != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %s", result.User.Email)
	}
	if len(result.User.Roles) != 1 || result.User.Roles[0] != models.RoleAdmin {
		t.Errorf("expected [admin] roles, got %v", result.User.Roles)
	}
	if !result.User.IsActive {
		t.Error("expected IsActive true")
	}
}

func TestAuthenticate_UserNotFound(t *testing.T) {
	repo := newMockUserRepo()
	provider := NewLocalAuthProvider(repo)

	result, err := provider.Authenticate(context.Background(), models.LoginCredentials{
		Username: "nobody",
		Password: "pass",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for unknown user")
	}
	if result.Error != "invalid credentials" {
		t.Errorf("expected 'invalid credentials', got %q", result.Error)
	}
}

func TestAuthenticate_AccountDisabled(t *testing.T) {
	repo := newMockUserRepo()
	repo.addUser("u2", "bob", "pass", "bob@example.com", false, nil)

	provider := NewLocalAuthProvider(repo)
	result, err := provider.Authenticate(context.Background(), models.LoginCredentials{
		Username: "bob",
		Password: "pass",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for disabled account")
	}
	if result.Error != "account disabled" {
		t.Errorf("expected 'account disabled', got %q", result.Error)
	}
}

func TestAuthenticate_WrongPassword(t *testing.T) {
	repo := newMockUserRepo()
	repo.addUser("u3", "carol", "correct", "carol@example.com", true, nil)

	provider := NewLocalAuthProvider(repo)
	result, err := provider.Authenticate(context.Background(), models.LoginCredentials{
		Username: "carol",
		Password: "wrong",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for wrong password")
	}
	if result.Error != "invalid credentials" {
		t.Errorf("expected 'invalid credentials', got %q", result.Error)
	}
}

func TestAuthenticate_RepoLookupError(t *testing.T) {
	repo := newMockUserRepo()
	repo.getByUsernameErr = errors.New("db connection lost")

	provider := NewLocalAuthProvider(repo)
	_, err := provider.Authenticate(context.Background(), models.LoginCredentials{
		Username: "alice",
		Password: "pass",
	})
	if err == nil {
		t.Fatal("expected error from repo failure")
	}
}

func TestAuthenticate_RolesLookupError(t *testing.T) {
	repo := newMockUserRepo()
	repo.addUser("u4", "dave", "pass", "dave@example.com", true, nil)
	repo.getUserRolesErr = errors.New("roles table missing")

	provider := NewLocalAuthProvider(repo)
	_, err := provider.Authenticate(context.Background(), models.LoginCredentials{
		Username: "dave",
		Password: "pass",
	})
	if err == nil {
		t.Fatal("expected error from roles lookup failure")
	}
}

func TestGetUserRoles_Delegates(t *testing.T) {
	repo := newMockUserRepo()
	repo.roles["u5"] = []models.Role{models.RoleQA, models.RoleData}

	provider := NewLocalAuthProvider(repo)
	roles, err := provider.GetUserRoles(context.Background(), "u5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(roles))
	}
	if roles[0] != models.RoleQA || roles[1] != models.RoleData {
		t.Errorf("unexpected roles: %v", roles)
	}
}

func TestHashPassword_RoundTrip(t *testing.T) {
	password := "my-secure-password"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if hash == password {
		t.Fatal("hash should not equal plaintext password")
	}

	// Verify the hash matches the original password.
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		t.Errorf("hash does not match original password: %v", err)
	}

	// Verify a wrong password does not match.
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("wrong")); err == nil {
		t.Error("expected mismatch for wrong password")
	}
}

func TestNewLocalAuthProvider_ReturnsNonNil(t *testing.T) {
	provider := NewLocalAuthProvider(newMockUserRepo())
	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
}
