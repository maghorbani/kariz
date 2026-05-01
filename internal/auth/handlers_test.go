package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kariz/kariz/internal/models"
)

// --- Mock AuthProvider ---

type mockAuthProvider struct {
	authenticateResult *models.AuthResult
	authenticateErr    error
	roles              []models.Role
	rolesErr           error
}

func (m *mockAuthProvider) Authenticate(_ context.Context, _ models.LoginCredentials) (*models.AuthResult, error) {
	return m.authenticateResult, m.authenticateErr
}

func (m *mockAuthProvider) GetUserRoles(_ context.Context, _ string) ([]models.Role, error) {
	return m.roles, m.rolesErr
}

// --- Mock SessionStore for handlers ---

type handlerMockSessionStore struct {
	sessions   map[string]*models.SessionData
	createErr  error
	getErr     error
	deleteErr  error
	lastToken  string
	lastData   models.SessionData
	deletedTok string
}

func newHandlerMockSessionStore() *handlerMockSessionStore {
	return &handlerMockSessionStore{
		sessions: make(map[string]*models.SessionData),
	}
}

func (m *handlerMockSessionStore) Create(_ context.Context, data models.SessionData) (string, error) {
	if m.createErr != nil {
		return "", m.createErr
	}
	m.lastData = data
	m.lastToken = "test-session-token"
	m.sessions[m.lastToken] = &data
	return m.lastToken, nil
}

func (m *handlerMockSessionStore) Get(_ context.Context, token string) (*models.SessionData, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	data, ok := m.sessions[token]
	if !ok {
		return nil, nil
	}
	return data, nil
}

func (m *handlerMockSessionStore) Delete(_ context.Context, token string) error {
	m.deletedTok = token
	delete(m.sessions, token)
	return m.deleteErr
}

func (m *handlerMockSessionStore) DeleteByUserID(_ context.Context, _ string) error {
	return nil
}

func (m *handlerMockSessionStore) CleanExpired(_ context.Context) error {
	return nil
}

// --- Helper ---

func setupRouter(handler *AuthHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	handler.RegisterRoutes(api)
	return r
}

func jsonBody(v interface{}) *bytes.Buffer {
	b, _ := json.Marshal(v)
	return bytes.NewBuffer(b)
}

// --- Login Tests ---

func TestLogin_Success(t *testing.T) {
	store := newHandlerMockSessionStore()
	provider := &mockAuthProvider{
		authenticateResult: &models.AuthResult{
			Success: true,
			User: &models.UserProfile{
				ID:       "user-1",
				Username: "alice",
				Email:    "alice@example.com",
				Roles:    []models.Role{models.RoleAdmin},
				IsActive: true,
			},
		},
	}

	handler := NewAuthHandler(provider, store, 24*time.Hour)
	r := setupRouter(handler)

	body := jsonBody(models.LoginCredentials{Username: "alice", Password: "secret"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var profile models.UserProfile
	if err := json.Unmarshal(w.Body.Bytes(), &profile); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if profile.ID != "user-1" {
		t.Errorf("expected user ID 'user-1', got %q", profile.ID)
	}
	if profile.Username != "alice" {
		t.Errorf("expected username 'alice', got %q", profile.Username)
	}

	// Check cookie was set
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == sessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session cookie to be set")
	}
	if !sessionCookie.HttpOnly {
		t.Error("expected cookie to be httpOnly")
	}
	if sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("expected SameSite=Strict, got %v", sessionCookie.SameSite)
	}
	if sessionCookie.Path != "/" {
		t.Errorf("expected cookie path '/', got %q", sessionCookie.Path)
	}
	if sessionCookie.MaxAge != 86400 {
		t.Errorf("expected MaxAge 86400, got %d", sessionCookie.MaxAge)
	}

	// Verify session data was stored
	if store.lastData.UserID != "user-1" {
		t.Errorf("expected session UserID 'user-1', got %q", store.lastData.UserID)
	}
	if store.lastData.Username != "alice" {
		t.Errorf("expected session Username 'alice', got %q", store.lastData.Username)
	}
}

func TestLogin_InvalidCredentials_Returns401(t *testing.T) {
	store := newHandlerMockSessionStore()
	provider := &mockAuthProvider{
		authenticateResult: &models.AuthResult{
			Success: false,
			Error:   "invalid credentials",
		},
	}

	handler := NewAuthHandler(provider, store, 24*time.Hour)
	r := setupRouter(handler)

	body := jsonBody(models.LoginCredentials{Username: "alice", Password: "wrong"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}

	var apiErr models.APIError
	if err := json.Unmarshal(w.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if apiErr.Code != "unauthorized" {
		t.Errorf("expected code 'unauthorized', got %q", apiErr.Code)
	}
	if apiErr.Message != "invalid credentials" {
		t.Errorf("expected message 'invalid credentials', got %q", apiErr.Message)
	}
}

func TestLogin_InvalidBody_Returns400(t *testing.T) {
	store := newHandlerMockSessionStore()
	provider := &mockAuthProvider{}

	handler := NewAuthHandler(provider, store, 24*time.Hour)
	r := setupRouter(handler)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLogin_ProviderError_Returns500(t *testing.T) {
	store := newHandlerMockSessionStore()
	provider := &mockAuthProvider{
		authenticateErr: errors.New("db down"),
	}

	handler := NewAuthHandler(provider, store, 24*time.Hour)
	r := setupRouter(handler)

	body := jsonBody(models.LoginCredentials{Username: "alice", Password: "secret"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLogin_SessionCreateError_Returns500(t *testing.T) {
	store := newHandlerMockSessionStore()
	store.createErr = errors.New("session store down")
	provider := &mockAuthProvider{
		authenticateResult: &models.AuthResult{
			Success: true,
			User: &models.UserProfile{
				ID:       "user-1",
				Username: "alice",
				Roles:    []models.Role{models.RoleAdmin},
				IsActive: true,
			},
		},
	}

	handler := NewAuthHandler(provider, store, 24*time.Hour)
	r := setupRouter(handler)

	body := jsonBody(models.LoginCredentials{Username: "alice", Password: "secret"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLogin_MissingUsername_Returns400(t *testing.T) {
	store := newHandlerMockSessionStore()
	provider := &mockAuthProvider{}

	handler := NewAuthHandler(provider, store, 24*time.Hour)
	r := setupRouter(handler)

	body := jsonBody(map[string]string{"password": "secret"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Logout Tests ---

func TestLogout_Success(t *testing.T) {
	store := newHandlerMockSessionStore()
	store.sessions["test-session-token"] = &models.SessionData{
		UserID:   "user-1",
		Username: "alice",
		Roles:    []models.Role{models.RoleAdmin},
	}

	provider := &mockAuthProvider{}
	handler := NewAuthHandler(provider, store, 24*time.Hour)
	r := setupRouter(handler)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "test-session-token"})
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp["message"] != "logged out" {
		t.Errorf("expected message 'logged out', got %q", resp["message"])
	}

	// Verify session was deleted
	if store.deletedTok != "test-session-token" {
		t.Errorf("expected session token to be deleted, got %q", store.deletedTok)
	}

	// Verify cookie was cleared
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == sessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session cookie to be set (cleared)")
	}
	if sessionCookie.MaxAge != -1 {
		t.Errorf("expected MaxAge -1 (clear cookie), got %d", sessionCookie.MaxAge)
	}
}

func TestLogout_NoSession_Returns401(t *testing.T) {
	store := newHandlerMockSessionStore()
	provider := &mockAuthProvider{}
	handler := NewAuthHandler(provider, store, 24*time.Hour)
	r := setupRouter(handler)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Me Tests ---

func TestMe_Success(t *testing.T) {
	store := newHandlerMockSessionStore()
	store.sessions["test-session-token"] = &models.SessionData{
		UserID:   "user-1",
		Username: "alice",
		Roles:    []models.Role{models.RoleAdmin, models.RoleQA},
	}

	provider := &mockAuthProvider{}
	handler := NewAuthHandler(provider, store, 24*time.Hour)
	r := setupRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "test-session-token"})
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var profile models.UserProfile
	if err := json.Unmarshal(w.Body.Bytes(), &profile); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if profile.ID != "user-1" {
		t.Errorf("expected user ID 'user-1', got %q", profile.ID)
	}
	if profile.Username != "alice" {
		t.Errorf("expected username 'alice', got %q", profile.Username)
	}
	if len(profile.Roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(profile.Roles))
	}
}

func TestMe_NoSession_Returns401(t *testing.T) {
	store := newHandlerMockSessionStore()
	provider := &mockAuthProvider{}
	handler := NewAuthHandler(provider, store, 24*time.Hour)
	r := setupRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMe_InvalidSession_Returns401(t *testing.T) {
	store := newHandlerMockSessionStore()
	provider := &mockAuthProvider{}
	handler := NewAuthHandler(provider, store, 24*time.Hour)
	r := setupRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "nonexistent-token"})
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}
