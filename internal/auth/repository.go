package auth

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kariz/kariz/internal/models"
)

// UserRepository defines the data access interface for users and their roles.
type UserRepository interface {
	Create(ctx context.Context, user *models.User, roles []models.Role) error
	GetByID(ctx context.Context, id string) (*models.User, error)
	GetByUsername(ctx context.Context, username string) (*models.User, error)
	GetUserRoles(ctx context.Context, userID string) ([]models.Role, error)
	UpdateRoles(ctx context.Context, userID string, roles []models.Role) error
	List(ctx context.Context) ([]models.UserProfile, error)
}

// userRepository implements UserRepository using sqlx.
type userRepository struct {
	db *sqlx.DB
}

// NewUserRepository creates a new UserRepository backed by the given database.
func NewUserRepository(db *sqlx.DB) UserRepository {
	return &userRepository{db: db}
}

// Create inserts a new user and their associated roles within a transaction.
func (r *userRepository) Create(ctx context.Context, user *models.User, roles []models.Role) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, email, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		user.ID, user.Username, user.PasswordHash, user.Email,
		user.IsActive, user.CreatedAt, user.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}

	if err := insertUserRoles(ctx, tx, user.ID, roles); err != nil {
		return err
	}

	return tx.Commit()
}

// GetByID retrieves a user by their ID.
// Returns nil, nil if the user is not found.
func (r *userRepository) GetByID(ctx context.Context, id string) (*models.User, error) {
	var user models.User
	err := r.db.GetContext(ctx, &user, `
		SELECT id, username, password_hash, email, is_active, created_at, updated_at
		FROM users WHERE id = $1`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}

	return &user, nil
}

// GetByUsername retrieves a user by their username.
// Returns nil, nil if the user is not found.
func (r *userRepository) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	var user models.User
	err := r.db.GetContext(ctx, &user, `
		SELECT id, username, password_hash, email, is_active, created_at, updated_at
		FROM users WHERE username = $1`, username)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get user by username: %w", err)
	}

	return &user, nil
}

// GetUserRoles fetches the roles assigned to a user.
func (r *userRepository) GetUserRoles(ctx context.Context, userID string) ([]models.Role, error) {
	var roleStrings []string
	err := r.db.SelectContext(ctx, &roleStrings,
		`SELECT role FROM user_roles WHERE user_id = $1 ORDER BY role`, userID)
	if err != nil {
		return nil, fmt.Errorf("get user roles: %w", err)
	}

	roles := make([]models.Role, len(roleStrings))
	for i, s := range roleStrings {
		roles[i] = models.Role(s)
	}
	return roles, nil
}

// UpdateRoles replaces all roles for a user within a transaction.
func (r *userRepository) UpdateRoles(ctx context.Context, userID string, roles []models.Role) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete existing user roles: %w", err)
	}

	if err := insertUserRoles(ctx, tx, userID, roles); err != nil {
		return err
	}

	return tx.Commit()
}

// List returns all users with their roles as UserProfile structs (no password hash).
func (r *userRepository) List(ctx context.Context) ([]models.UserProfile, error) {
	var users []models.User
	err := r.db.SelectContext(ctx, &users, `
		SELECT id, username, password_hash, email, is_active, created_at, updated_at
		FROM users ORDER BY username ASC`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	profiles := make([]models.UserProfile, 0, len(users))
	for _, u := range users {
		roles, err := r.GetUserRoles(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, models.UserProfile{
			ID:       u.ID,
			Username: u.Username,
			Email:    u.Email,
			Roles:    roles,
			IsActive: u.IsActive,
		})
	}

	return profiles, nil
}

// insertUserRoles inserts role records for a user within the given transaction.
func insertUserRoles(ctx context.Context, tx *sqlx.Tx, userID string, roles []models.Role) error {
	if len(roles) == 0 {
		return nil
	}

	for _, role := range roles {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO user_roles (user_id, role) VALUES ($1, $2)`,
			userID, string(role))
		if err != nil {
			return fmt.Errorf("insert user role %s: %w", role, err)
		}
	}
	return nil
}
