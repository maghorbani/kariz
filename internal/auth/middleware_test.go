package auth

import (
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

// mockSessionStore is a minimal SessionStore for testing middleware.
type mockSessionStore struct {
	sessions map[string]*models.SessionData
	getErr   error
}

func newMockSessionStore() *mockSessionStore {
	return &mockSessionStore{
		sessions: make(map[string]*models.SessionData),
	}
}

func (m *mockSessionStore) Create(_ context.Context, _ models.SessionData) (string, error) {
	return "", nil
}

func (m *mockSessionStore) Get(_ context.Context, token string) (*models.SessionData, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	data, ok := m.sessions[token]
	if !ok {
		return nil, nil
	}
	return data, nil
}

func (m *mockSessionStore) Delete(_ context.Context, _ string) error {
	return nil
}

func (m *mockSessionStore) DeleteByUserID(_ context.Context, _ string) error {
	return nil
}

func (m *mockSessionStore) CleanExpired(_ context.Context) error {
	return nil
}

func init() {
	gin.SetMode(gin.TestMode)
}

// --- AuthMiddleware Tests ---

func TestAuthMiddleware_NoCookie_Returns401(t *testing.T) {
	store := newMockSessionStore()
	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(AuthMiddleware(store))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, c.Request)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	var apiErr models.APIError
	if err := json.Unmarshal(w.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if apiErr.Code != "unauthorized" {
		t.Errorf("expected code 'unauthorized', got %q", apiErr.Code)
	}
	if apiErr.Message != "authentication required" {
		t.Errorf("expected message 'authentication required', got %q", apiErr.Message)
	}
}

func TestAuthMiddleware_InvalidToken_Returns401(t *testing.T) {
	store := newMockSessionStore()
	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(AuthMiddleware(store))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
	c.Request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "bad-token"})
	r.ServeHTTP(w, c.Request)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	var apiErr models.APIError
	if err := json.Unmarshal(w.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if apiErr.Code != "unauthorized" {
		t.Errorf("expected code 'unauthorized', got %q", apiErr.Code)
	}
	if apiErr.Message != "session expired or invalid" {
		t.Errorf("expected message 'session expired or invalid', got %q", apiErr.Message)
	}
}

func TestAuthMiddleware_StoreError_Returns401(t *testing.T) {
	store := newMockSessionStore()
	store.getErr = errors.New("db connection failed")
	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(AuthMiddleware(store))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
	c.Request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "some-token"})
	r.ServeHTTP(w, c.Request)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_ValidSession_SetsContextAndCallsNext(t *testing.T) {
	store := newMockSessionStore()
	store.sessions["valid-token"] = &models.SessionData{
		UserID:    "u1",
		Username:  "alice",
		Roles:     []models.Role{models.RoleAdmin},
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	var capturedSession *models.SessionData
	r.Use(AuthMiddleware(store))
	r.GET("/test", func(c *gin.Context) {
		capturedSession = GetSessionFromContext(c)
		c.Status(http.StatusOK)
	})

	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
	c.Request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "valid-token"})
	r.ServeHTTP(w, c.Request)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if capturedSession == nil {
		t.Fatal("expected session data to be set in context")
	}
	if capturedSession.UserID != "u1" {
		t.Errorf("expected user_id 'u1', got %q", capturedSession.UserID)
	}
	if capturedSession.Username != "alice" {
		t.Errorf("expected username 'alice', got %q", capturedSession.Username)
	}
	if len(capturedSession.Roles) != 1 || capturedSession.Roles[0] != models.RoleAdmin {
		t.Errorf("expected [admin] roles, got %v", capturedSession.Roles)
	}
}

func TestAuthMiddleware_EmptyCookieValue_Returns401(t *testing.T) {
	store := newMockSessionStore()
	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(AuthMiddleware(store))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
	c.Request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: ""})
	r.ServeHTTP(w, c.Request)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// --- RequireRole Tests ---

func TestRequireRole_MatchingRole_CallsNext(t *testing.T) {
	store := newMockSessionStore()
	store.sessions["valid-token"] = &models.SessionData{
		UserID:   "u1",
		Username: "alice",
		Roles:    []models.Role{models.RoleAdmin, models.RoleQA},
	}

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(AuthMiddleware(store))
	r.Use(RequireRole(models.RoleQA))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
	c.Request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "valid-token"})
	r.ServeHTTP(w, c.Request)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireRole_NoMatchingRole_Returns403(t *testing.T) {
	store := newMockSessionStore()
	store.sessions["valid-token"] = &models.SessionData{
		UserID:   "u1",
		Username: "alice",
		Roles:    []models.Role{models.RoleQA},
	}

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(AuthMiddleware(store))
	r.Use(RequireRole(models.RoleAdmin))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
	c.Request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "valid-token"})
	r.ServeHTTP(w, c.Request)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}

	var apiErr models.APIError
	if err := json.Unmarshal(w.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if apiErr.Code != "forbidden" {
		t.Errorf("expected code 'forbidden', got %q", apiErr.Code)
	}
	if apiErr.Message != "access denied" {
		t.Errorf("expected message 'access denied', got %q", apiErr.Message)
	}
}

func TestRequireRole_MultipleRequiredRoles_MatchesAny(t *testing.T) {
	store := newMockSessionStore()
	store.sessions["valid-token"] = &models.SessionData{
		UserID:   "u1",
		Username: "alice",
		Roles:    []models.Role{models.RoleData},
	}

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(AuthMiddleware(store))
	r.Use(RequireRole(models.RoleAdmin, models.RoleData))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
	c.Request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "valid-token"})
	r.ServeHTTP(w, c.Request)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireRole_NoSessionInContext_Returns401(t *testing.T) {
	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)

	// Use RequireRole without AuthMiddleware to simulate missing session
	r.Use(RequireRole(models.RoleAdmin))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequireRole_EmptyUserRoles_Returns403(t *testing.T) {
	store := newMockSessionStore()
	store.sessions["valid-token"] = &models.SessionData{
		UserID:   "u1",
		Username: "alice",
		Roles:    []models.Role{},
	}

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(AuthMiddleware(store))
	r.Use(RequireRole(models.RoleAdmin))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
	c.Request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "valid-token"})
	r.ServeHTTP(w, c.Request)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// --- GetSessionFromContext Tests ---

func TestGetSessionFromContext_WithSession(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	expected := &models.SessionData{
		UserID:   "u1",
		Username: "alice",
		Roles:    []models.Role{models.RoleAdmin},
	}
	c.Set(sessionContextKey, expected)

	result := GetSessionFromContext(c)
	if result == nil {
		t.Fatal("expected non-nil session data")
	}
	if result.UserID != "u1" {
		t.Errorf("expected user_id 'u1', got %q", result.UserID)
	}
}

func TestGetSessionFromContext_NoSession(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	result := GetSessionFromContext(c)
	if result != nil {
		t.Error("expected nil when no session in context")
	}
}

func TestGetSessionFromContext_WrongType(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Set(sessionContextKey, "not-a-session-data")

	result := GetSessionFromContext(c)
	if result != nil {
		t.Error("expected nil when context value has wrong type")
	}
}
