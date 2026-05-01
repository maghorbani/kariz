package auth

import (
	"context"
	"fmt"

	"github.com/kariz/kariz/internal/models"
	"golang.org/x/crypto/bcrypt"
)

// AuthProvider allows swapping auth backends (local DB, OAuth2, LDAP).
type AuthProvider interface {
	Authenticate(ctx context.Context, creds models.LoginCredentials) (*models.AuthResult, error)
	GetUserRoles(ctx context.Context, userID string) ([]models.Role, error)
}

// LocalAuthProvider authenticates users against the local users table with bcrypt password hashing.
type LocalAuthProvider struct {
	userRepo UserRepository
}

// NewLocalAuthProvider creates a new LocalAuthProvider backed by the given UserRepository.
func NewLocalAuthProvider(userRepo UserRepository) AuthProvider {
	return &LocalAuthProvider{userRepo: userRepo}
}

// Authenticate verifies the provided credentials against the users table.
// It returns an AuthResult indicating success or failure with an appropriate error message.
func (p *LocalAuthProvider) Authenticate(ctx context.Context, creds models.LoginCredentials) (*models.AuthResult, error) {
	user, err := p.userRepo.GetByUsername(ctx, creds.Username)
	if err != nil {
		return nil, fmt.Errorf("lookup user: %w", err)
	}
	if user == nil {
		return &models.AuthResult{Success: false, Error: "invalid credentials"}, nil
	}

	if !user.IsActive {
		return &models.AuthResult{Success: false, Error: "account disabled"}, nil
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(creds.Password)); err != nil {
		return &models.AuthResult{Success: false, Error: "invalid credentials"}, nil
	}

	roles, err := p.userRepo.GetUserRoles(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("get user roles: %w", err)
	}

	profile := models.UserProfile{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Roles:    roles,
		IsActive: user.IsActive,
	}

	return &models.AuthResult{
		Success: true,
		User:    &profile,
	}, nil
}

// GetUserRoles delegates to the UserRepository to fetch roles for the given user.
func (p *LocalAuthProvider) GetUserRoles(ctx context.Context, userID string) ([]models.Role, error) {
	return p.userRepo.GetUserRoles(ctx, userID)
}

// HashPassword hashes a plaintext password using bcrypt with the default cost.
// This is used when creating or updating user passwords.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}
