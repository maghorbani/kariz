package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kariz/kariz/internal/auth"
	"github.com/kariz/kariz/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type adminTestUserRepo struct {
	users map[string]*models.User
	roles map[string][]models.Role
}

func newAdminTestUserRepo() *adminTestUserRepo {
	return &adminTestUserRepo{
		users: make(map[string]*models.User),
		roles: make(map[string][]models.Role),
	}
}

func (r *adminTestUserRepo) Create(_ context.Context, user *models.User, roles []models.Role) error {
	r.users[user.ID] = user
	r.roles[user.ID] = roles
	return nil
}

func (r *adminTestUserRepo) GetByID(_ context.Context, id string) (*models.User, error) {
	u, ok := r.users[id]
	if !ok {
		return nil, nil
	}
	cp := *u
	return &cp, nil
}

func (r *adminTestUserRepo) GetByUsername(_ context.Context, username string) (*models.User, error) {
	for _, u := range r.users {
		if u.Username == username {
			cp := *u
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *adminTestUserRepo) GetUserRoles(_ context.Context, userID string) ([]models.Role, error) {
	return r.roles[userID], nil
}

func (r *adminTestUserRepo) UpdateRoles(_ context.Context, userID string, roles []models.Role) error {
	r.roles[userID] = roles
	return nil
}

func (r *adminTestUserRepo) Update(_ context.Context, user *models.User) error {
	r.users[user.ID] = user
	return nil
}

func (r *adminTestUserRepo) UpdatePassword(_ context.Context, userID, hash string) error {
	if u, ok := r.users[userID]; ok {
		u.PasswordHash = hash
	}
	return nil
}

func (r *adminTestUserRepo) CountActiveAdmins(_ context.Context) (int, error) {
	return r.countActiveAdmins(""), nil
}

func (r *adminTestUserRepo) CountActiveAdminsWithRole(_ context.Context, excludeUserID string) (int, error) {
	return r.countActiveAdmins(excludeUserID), nil
}

func (r *adminTestUserRepo) countActiveAdmins(exclude string) int {
	n := 0
	for id, u := range r.users {
		if id == exclude || !u.IsActive {
			continue
		}
		for _, role := range r.roles[id] {
			if role == models.RoleAdmin {
				n++
				break
			}
		}
	}
	return n
}

func (r *adminTestUserRepo) List(_ context.Context) ([]models.UserProfile, error) {
	out := make([]models.UserProfile, 0, len(r.users))
	for id, u := range r.users {
		out = append(out, models.UserProfile{
			ID:       u.ID,
			Username: u.Username,
			Email:    u.Email,
			Roles:    r.roles[id],
			IsActive: u.IsActive,
		})
	}
	return out, nil
}

type adminTestSessionRepo struct {
	deleted []string
}

func (r *adminTestSessionRepo) Create(_ context.Context, _ *models.Session) error {
	return nil
}
func (r *adminTestSessionRepo) GetByToken(_ context.Context, _ string) (*models.Session, error) {
	return nil, nil
}
func (r *adminTestSessionRepo) Delete(_ context.Context, _ string) error { return nil }
func (r *adminTestSessionRepo) DeleteByUserID(_ context.Context, userID string) error {
	r.deleted = append(r.deleted, userID)
	return nil
}
func (r *adminTestSessionRepo) CleanExpired(_ context.Context) (int64, error) { return 0, nil }

func setupAdminRouter(repo *adminTestUserRepo, sessions *adminTestSessionRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewAdminHandler(repo, sessions)
	api := r.Group("/api")
	api.Use(func(c *gin.Context) {
		c.Set("session", &models.SessionData{
			UserID:   "admin-1",
			Username: "admin",
			Roles:    []models.Role{models.RoleAdmin},
		})
		c.Next()
	})
	h.RegisterRoutes(api, func(c *gin.Context) { c.Next() })
	return r
}

func TestCreateUser(t *testing.T) {
	repo := newAdminTestUserRepo()
	sessions := &adminTestSessionRepo{}
	router := setupAdminRouter(repo, sessions)

	body := `{"username":"newuser","password":"secret123","email":"u@test.com","roles":["qa"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var profile models.UserProfile
	if err := json.Unmarshal(w.Body.Bytes(), &profile); err != nil {
		t.Fatal(err)
	}
	if profile.Username != "newuser" {
		t.Errorf("username = %q, want newuser", profile.Username)
	}
}

func TestCreateUser_InvalidRole(t *testing.T) {
	repo := newAdminTestUserRepo()
	router := setupAdminRouter(repo, &adminTestSessionRepo{})

	body := `{"username":"x","password":"p","roles":["superuser"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestDeactivateUser_LastAdminBlocked(t *testing.T) {
	repo := newAdminTestUserRepo()
	now := time.Now().UTC()
	hash, _ := bcrypt.GenerateFromPassword([]byte("pass"), bcrypt.DefaultCost)
	repo.users["admin-1"] = &models.User{
		ID: "admin-1", Username: "admin", PasswordHash: string(hash),
		Email: "a@test.com", IsActive: true, CreatedAt: now, UpdatedAt: now,
	}
	repo.roles["admin-1"] = []models.Role{models.RoleAdmin}

	router := setupAdminRouter(repo, &adminTestSessionRepo{})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/admin-1/deactivate", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", w.Code, w.Body.String())
	}
}

func TestUpdateUserRoles_InvalidatesSessions(t *testing.T) {
	repo := newAdminTestUserRepo()
	sessions := &adminTestSessionRepo{}
	now := time.Now().UTC()
	hash, _ := bcrypt.GenerateFromPassword([]byte("pass"), bcrypt.DefaultCost)
	repo.users["u1"] = &models.User{
		ID: "u1", Username: "user1", PasswordHash: string(hash),
		Email: "u@test.com", IsActive: true, CreatedAt: now, UpdatedAt: now,
	}
	repo.roles["u1"] = []models.Role{models.RoleQA}
	repo.users["admin-1"] = &models.User{
		ID: "admin-1", Username: "admin", PasswordHash: string(hash),
		Email: "a@test.com", IsActive: true, CreatedAt: now, UpdatedAt: now,
	}
	repo.roles["admin-1"] = []models.Role{models.RoleAdmin}

	router := setupAdminRouter(repo, sessions)
	body := `{"roles":["data","operations"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/users/u1/roles", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if len(sessions.deleted) != 1 || sessions.deleted[0] != "u1" {
		t.Errorf("sessions deleted = %v, want [u1]", sessions.deleted)
	}
}

// Ensure auth package import is used for middleware key compatibility in real app.
var _ = auth.GetSessionFromContext
