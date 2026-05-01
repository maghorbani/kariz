package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kariz/kariz/internal/models"
)

const (
	// sessionCookieName is the name of the httpOnly cookie that holds the session token.
	sessionCookieName = "kariz_session"

	// sessionContextKey is the Gin context key used to store the authenticated SessionData.
	sessionContextKey = "session"
)

// AuthMiddleware returns a Gin middleware that extracts the session token from an
// httpOnly cookie, validates it against the SessionStore, and attaches the SessionData
// to the Gin context. Returns 401 if the cookie is missing or the session is invalid/expired.
func AuthMiddleware(sessionStore SessionStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(sessionCookieName)
		if err != nil || token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, models.APIError{
				Code:    "unauthorized",
				Message: "authentication required",
			})
			return
		}

		sessionData, err := sessionStore.Get(c.Request.Context(), token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, models.APIError{
				Code:    "unauthorized",
				Message: "session expired or invalid",
			})
			return
		}
		if sessionData == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, models.APIError{
				Code:    "unauthorized",
				Message: "session expired or invalid",
			})
			return
		}

		c.Set(sessionContextKey, sessionData)
		c.Next()
	}
}

// RequireRole returns a Gin middleware that checks whether the authenticated user
// has at least one of the required roles. Returns 403 if no intersection exists.
// This middleware must run after AuthMiddleware.
func RequireRole(roles ...models.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionData := GetSessionFromContext(c)
		if sessionData == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, models.APIError{
				Code:    "unauthorized",
				Message: "authentication required",
			})
			return
		}

		if !hasRoleIntersection(sessionData.Roles, roles) {
			c.AbortWithStatusJSON(http.StatusForbidden, models.APIError{
				Code:    "forbidden",
				Message: "access denied",
			})
			return
		}

		c.Next()
	}
}

// GetSessionFromContext extracts the SessionData from the Gin context.
// Returns nil if the session data is not present or has an unexpected type.
func GetSessionFromContext(c *gin.Context) *models.SessionData {
	val, exists := c.Get(sessionContextKey)
	if !exists {
		return nil
	}
	sessionData, ok := val.(*models.SessionData)
	if !ok {
		return nil
	}
	return sessionData
}

// hasRoleIntersection returns true if any role in userRoles matches any role in requiredRoles.
func hasRoleIntersection(userRoles []models.Role, requiredRoles []models.Role) bool {
	for _, ur := range userRoles {
		for _, rr := range requiredRoles {
			if ur == rr {
				return true
			}
		}
	}
	return false
}
