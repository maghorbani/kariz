package models

import "time"

// Role represents a named permission group.
type Role string

const (
	RoleAdmin      Role = "admin"
	RoleQA         Role = "qa"
	RoleData       Role = "data"
	RoleOperations Role = "operations"
)

// User represents an authenticated user stored in the database.
type User struct {
	ID           string    `json:"id" db:"id"`
	Username     string    `json:"username" db:"username"`
	PasswordHash string    `json:"-" db:"password_hash"`
	Email        string    `json:"email" db:"email"`
	IsActive     bool      `json:"is_active" db:"is_active"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

// UserProfile is the public-facing representation of a user (no password hash).
type UserProfile struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Roles    []Role `json:"roles"`
	IsActive bool   `json:"is_active"`
}

// Session represents a server-side session stored in PostgreSQL.
type Session struct {
	ID           string    `json:"id" db:"id"`
	UserID       string    `json:"user_id" db:"user_id"`
	SessionToken string    `json:"session_token" db:"session_token"`
	SessionData  string    `json:"session_data" db:"session_data"`
	ExpiresAt    time.Time `json:"expires_at" db:"expires_at"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// SessionData holds the decoded session payload.
type SessionData struct {
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Roles     []Role    `json:"roles"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// LoginCredentials holds the username and password for authentication.
type LoginCredentials struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// AuthResult holds the outcome of an authentication attempt.
type AuthResult struct {
	Success   bool         `json:"success"`
	User      *UserProfile `json:"user,omitempty"`
	SessionID string       `json:"session_id,omitempty"`
	Error     string       `json:"error,omitempty"`
}
