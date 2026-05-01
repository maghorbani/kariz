package property_test

// Feature: kariz-command-dashboard, Property 12: role-based execution denial

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kariz/kariz/internal/auth"
	"github.com/kariz/kariz/internal/models"
	"pgregory.net/rapid"
)

// allRoles is the complete set of valid roles in the system.
var allRoles = []models.Role{
	models.RoleAdmin,
	models.RoleQA,
	models.RoleData,
	models.RoleOperations,
}

// --- Generators ---

// genDisjointRoleSets generates two non-empty role subsets with empty intersection.
func genDisjointRoleSets(t *rapid.T) (userRoles []models.Role, commandRoles []models.Role) {
	// Pick a partition point: at least 1 role on each side
	// We'll split allRoles into two disjoint groups
	n := len(allRoles)
	// Generate a bitmask for user roles (at least 1 bit set, not all bits set)
	userMask := rapid.IntRange(1, (1<<n)-2).Draw(t, "userMask")
	// Command roles get the complement
	commandMask := ((1 << n) - 1) ^ userMask

	for i, r := range allRoles {
		if userMask&(1<<i) != 0 {
			userRoles = append(userRoles, r)
		}
		if commandMask&(1<<i) != 0 {
			commandRoles = append(commandRoles, r)
		}
	}
	return userRoles, commandRoles
}

// --- Property 12 Tests ---
// **Validates: Requirements 5.3**

// TestProperty12_DisjointRolesRejected tests that for any user role set and any
// CommandEntry allowed role set where the intersection is empty, attempting to
// access a route protected by RequireRole with the command's roles should be
// rejected with a 403 Forbidden status.
func TestProperty12_DisjointRolesRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rapid.Check(t, func(t *rapid.T) {
		userRoles, commandRoles := genDisjointRoleSets(t)

		// Create a test Gin engine with auth middleware and RequireRole
		router := gin.New()

		// Create a mock session store that returns a session with the user's roles
		sessionData := &models.SessionData{
			UserID:    "user-prop12",
			Username:  "testuser",
			Roles:     userRoles,
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}

		// Set up a route that requires the command's roles
		router.GET("/test", func(c *gin.Context) {
			// Simulate AuthMiddleware by setting session in context
			c.Set("session", sessionData)
			c.Next()
		}, auth.RequireRole(commandRoles...), func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "allowed"})
		})

		// Make a request
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		router.ServeHTTP(w, req)

		// Must be rejected with 403
		if w.Code != http.StatusForbidden {
			t.Fatalf("userRoles=%v, commandRoles=%v: expected 403, got %d",
				userRoles, commandRoles, w.Code)
		}
	})
}

// TestProperty12_OverlappingRolesAllowed tests that for any user role set and any
// CommandEntry allowed role set where the intersection is non-empty, the request
// should be allowed (200 OK).
func TestProperty12_OverlappingRolesAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rapid.Check(t, func(t *rapid.T) {
		// Generate user roles and command roles that share at least one role
		sharedRole := allRoles[rapid.IntRange(0, len(allRoles)-1).Draw(t, "sharedRoleIdx")]

		// User roles: shared role + random additional roles
		userRoles := []models.Role{sharedRole}
		for _, r := range allRoles {
			if r != sharedRole && rapid.Bool().Draw(t, "addUserRole_"+string(r)) {
				userRoles = append(userRoles, r)
			}
		}

		// Command roles: shared role + random additional roles
		commandRoles := []models.Role{sharedRole}
		for _, r := range allRoles {
			if r != sharedRole && rapid.Bool().Draw(t, "addCmdRole_"+string(r)) {
				commandRoles = append(commandRoles, r)
			}
		}

		// Create a test Gin engine
		router := gin.New()

		sessionData := &models.SessionData{
			UserID:    "user-prop12",
			Username:  "testuser",
			Roles:     userRoles,
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}

		router.GET("/test", func(c *gin.Context) {
			c.Set("session", sessionData)
			c.Next()
		}, auth.RequireRole(commandRoles...), func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "allowed"})
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		router.ServeHTTP(w, req)

		// Must be allowed with 200
		if w.Code != http.StatusOK {
			t.Fatalf("userRoles=%v, commandRoles=%v (shared=%s): expected 200, got %d",
				userRoles, commandRoles, sharedRole, w.Code)
		}
	})
}
