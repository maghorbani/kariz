package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kariz/kariz/internal/auth"
	"github.com/kariz/kariz/internal/models"
)

// AdminHandler handles admin-only HTTP endpoints for user management.
type AdminHandler struct {
	UserRepository    auth.UserRepository
	SessionRepository auth.SessionRepository
}

// NewAdminHandler creates a new AdminHandler with the given dependencies.
func NewAdminHandler(userRepo auth.UserRepository, sessionRepo auth.SessionRepository) *AdminHandler {
	return &AdminHandler{
		UserRepository:    userRepo,
		SessionRepository: sessionRepo,
	}
}

// RegisterRoutes registers admin routes on the given router group.
// All routes require authentication and admin role.
func (h *AdminHandler) RegisterRoutes(rg *gin.RouterGroup, authMiddleware gin.HandlerFunc) {
	admin := rg.Group("/admin")
	admin.Use(authMiddleware)
	admin.Use(auth.RequireRole(models.RoleAdmin))
	admin.GET("/users", h.ListUsers)
	admin.PUT("/users/:id/roles", h.UpdateUserRoles)
}

// ListUsers handles GET /api/admin/users.
// Returns a list of all users with their roles (admin only).
func (h *AdminHandler) ListUsers(c *gin.Context) {
	users, err := h.UserRepository.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to list users",
		})
		return
	}

	c.JSON(http.StatusOK, users)
}

// updateRolesRequest is the JSON body for the PUT /api/admin/users/:id/roles endpoint.
type updateRolesRequest struct {
	Roles []string `json:"roles" binding:"required"`
}

// UpdateUserRoles handles PUT /api/admin/users/:id/roles.
// Assigns or removes roles for a user, invalidates their sessions, and returns the updated profile.
func (h *AdminHandler) UpdateUserRoles(c *gin.Context) {
	userID := c.Param("id")

	var req updateRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{
			Code:    "bad_request",
			Message: "invalid request body: roles field is required",
		})
		return
	}

	// Verify the user exists
	user, err := h.UserRepository.GetByID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to look up user",
		})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, models.APIError{
			Code:    "not_found",
			Message: "user not found",
		})
		return
	}

	// Convert string roles to models.Role
	roles := make([]models.Role, len(req.Roles))
	for i, r := range req.Roles {
		roles[i] = models.Role(r)
	}

	// Update roles in the database
	if err := h.UserRepository.UpdateRoles(c.Request.Context(), userID, roles); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to update user roles",
		})
		return
	}

	// Invalidate existing sessions so new permissions take effect on next request (Req 5.4)
	if err := h.SessionRepository.DeleteByUserID(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to invalidate user sessions",
		})
		return
	}

	// Return updated user profile
	profile := models.UserProfile{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Roles:    roles,
		IsActive: user.IsActive,
	}

	c.JSON(http.StatusOK, profile)
}
