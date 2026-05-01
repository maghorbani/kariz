package auth

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kariz/kariz/internal/models"
)

// AuthHandler handles authentication-related HTTP endpoints.
type AuthHandler struct {
	provider AuthProvider
	store    SessionStore
	ttl      time.Duration
}

// NewAuthHandler creates a new AuthHandler with the given auth provider, session store, and session TTL.
func NewAuthHandler(provider AuthProvider, store SessionStore, ttl time.Duration) *AuthHandler {
	return &AuthHandler{
		provider: provider,
		store:    store,
		ttl:      ttl,
	}
}

// RegisterRoutes registers auth routes on the given router group.
// Login is public; logout and me require authentication via AuthMiddleware.
func (h *AuthHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/auth/login", h.Login)

	authenticated := rg.Group("")
	authenticated.Use(AuthMiddleware(h.store))
	authenticated.POST("/auth/logout", h.Logout)
	authenticated.GET("/auth/me", h.Me)
}

// Login handles POST /api/auth/login.
// It authenticates the user, creates a session, sets an httpOnly cookie, and returns the user profile.
func (h *AuthHandler) Login(c *gin.Context) {
	var creds models.LoginCredentials
	if err := c.ShouldBindJSON(&creds); err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{
			Code:    "bad_request",
			Message: "invalid request body",
		})
		return
	}

	result, err := h.provider.Authenticate(c.Request.Context(), creds)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "authentication failed",
		})
		return
	}

	if !result.Success {
		c.JSON(http.StatusUnauthorized, models.APIError{
			Code:    "unauthorized",
			Message: result.Error,
		})
		return
	}

	sessionData := models.SessionData{
		UserID:   result.User.ID,
		Username: result.User.Username,
		Roles:    result.User.Roles,
	}

	token, err := h.store.Create(c.Request.Context(), sessionData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to create session",
		})
		return
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(h.ttl.Seconds()),
	})

	c.JSON(http.StatusOK, result.User)
}

// Logout handles POST /api/auth/logout.
// It deletes the session and clears the cookie.
func (h *AuthHandler) Logout(c *gin.Context) {
	token, err := c.Cookie(sessionCookieName)
	if err == nil && token != "" {
		_ = h.store.Delete(c.Request.Context(), token)
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})

	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

// Me handles GET /api/auth/me.
// It returns the current user's profile from the session.
func (h *AuthHandler) Me(c *gin.Context) {
	session := GetSessionFromContext(c)
	if session == nil {
		c.JSON(http.StatusUnauthorized, models.APIError{
			Code:    "unauthorized",
			Message: "authentication required",
		})
		return
	}

	profile := models.UserProfile{
		ID:       session.UserID,
		Username: session.Username,
		Roles:    session.Roles,
	}

	c.JSON(http.StatusOK, profile)
}
