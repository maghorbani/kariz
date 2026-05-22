package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
	admin.POST("/users", h.CreateUser)
	admin.PUT("/users/:id", h.UpdateUser)
	admin.POST("/users/:id/deactivate", h.DeactivateUser)
	admin.PUT("/users/:id/roles", h.UpdateUserRoles)
}

// ListUsers handles GET /api/admin/users.
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

type createUserRequest struct {
	Username string   `json:"username" binding:"required"`
	Password string   `json:"password" binding:"required"`
	Email    string   `json:"email"`
	Roles    []string `json:"roles" binding:"required"`
}

// CreateUser handles POST /api/admin/users.
func (h *AdminHandler) CreateUser(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{
			Code:    "bad_request",
			Message: "invalid request body: username, password, and roles are required",
		})
		return
	}

	roles, err := parseAndValidateRoles(req.Roles)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{
			Code:    "bad_request",
			Message: err.Error(),
		})
		return
	}

	existing, err := h.UserRepository.GetByUsername(c.Request.Context(), req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to look up user",
		})
		return
	}
	if existing != nil {
		c.JSON(http.StatusConflict, models.APIError{
			Code:    "conflict",
			Message: "username already exists",
		})
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to hash password",
		})
		return
	}

	now := time.Now().UTC()
	user := &models.User{
		ID:           uuid.New().String(),
		Username:     req.Username,
		PasswordHash: hash,
		Email:        req.Email,
		IsActive:     true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := h.UserRepository.Create(c.Request.Context(), user, roles); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to create user",
		})
		return
	}

	c.JSON(http.StatusCreated, models.UserProfile{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Roles:    roles,
		IsActive: user.IsActive,
	})
}

type updateUserRequest struct {
	Email    *string `json:"email"`
	IsActive *bool   `json:"is_active"`
	Password *string `json:"password"`
}

// UpdateUser handles PUT /api/admin/users/:id.
func (h *AdminHandler) UpdateUser(c *gin.Context) {
	userID := c.Param("id")

	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{
			Code:    "bad_request",
			Message: "invalid request body",
		})
		return
	}

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

	session := auth.GetSessionFromContext(c)
	invalidateSessions := false

	if req.Email != nil {
		user.Email = *req.Email
	}

	if req.IsActive != nil {
		if !*req.IsActive {
			if session != nil && session.UserID == userID {
				c.JSON(http.StatusBadRequest, models.APIError{
					Code:    "bad_request",
					Message: "cannot deactivate your own account",
				})
				return
			}
			if err := h.ensureNotLastAdmin(c, userID, user); err != nil {
				return
			}
		}
		user.IsActive = *req.IsActive
		invalidateSessions = true
	}

	if req.Password != nil && *req.Password != "" {
		hash, hashErr := auth.HashPassword(*req.Password)
		if hashErr != nil {
			c.JSON(http.StatusInternalServerError, models.APIError{
				Code:    "internal_error",
				Message: "failed to hash password",
			})
			return
		}
		if err := h.UserRepository.UpdatePassword(c.Request.Context(), userID, hash); err != nil {
			c.JSON(http.StatusInternalServerError, models.APIError{
				Code:    "internal_error",
				Message: "failed to update password",
			})
			return
		}
		invalidateSessions = true
	}

	user.UpdatedAt = time.Now().UTC()
	if err := h.UserRepository.Update(c.Request.Context(), user); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to update user",
		})
		return
	}

	if invalidateSessions {
		if err := h.SessionRepository.DeleteByUserID(c.Request.Context(), userID); err != nil {
			c.JSON(http.StatusInternalServerError, models.APIError{
				Code:    "internal_error",
				Message: "failed to invalidate user sessions",
			})
			return
		}
	}

	roles, err := h.UserRepository.GetUserRoles(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to load user roles",
		})
		return
	}

	c.JSON(http.StatusOK, models.UserProfile{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Roles:    roles,
		IsActive: user.IsActive,
	})
}

// DeactivateUser handles POST /api/admin/users/:id/deactivate.
func (h *AdminHandler) DeactivateUser(c *gin.Context) {
	userID := c.Param("id")

	session := auth.GetSessionFromContext(c)
	if session != nil && session.UserID == userID {
		c.JSON(http.StatusBadRequest, models.APIError{
			Code:    "bad_request",
			Message: "cannot deactivate your own account",
		})
		return
	}

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

	if err := h.ensureNotLastAdmin(c, userID, user); err != nil {
		return
	}

	user.IsActive = false
	user.UpdatedAt = time.Now().UTC()
	if err := h.UserRepository.Update(c.Request.Context(), user); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to deactivate user",
		})
		return
	}

	if err := h.SessionRepository.DeleteByUserID(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to invalidate user sessions",
		})
		return
	}

	roles, _ := h.UserRepository.GetUserRoles(c.Request.Context(), userID)
	c.JSON(http.StatusOK, models.UserProfile{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Roles:    roles,
		IsActive: user.IsActive,
	})
}

type updateRolesRequest struct {
	Roles []string `json:"roles" binding:"required"`
}

// UpdateUserRoles handles PUT /api/admin/users/:id/roles.
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

	roles, err := parseAndValidateRoles(req.Roles)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{
			Code:    "bad_request",
			Message: err.Error(),
		})
		return
	}

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

	// Prevent removing admin from the last active admin.
	hasAdmin := false
	for _, r := range roles {
		if r == models.RoleAdmin {
			hasAdmin = true
			break
		}
	}
	if !hasAdmin && user.IsActive {
		if err := h.ensureNotLastAdmin(c, userID, user); err != nil {
			return
		}
	}

	if err := h.UserRepository.UpdateRoles(c.Request.Context(), userID, roles); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to update user roles",
		})
		return
	}

	if err := h.SessionRepository.DeleteByUserID(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to invalidate user sessions",
		})
		return
	}

	c.JSON(http.StatusOK, models.UserProfile{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Roles:    roles,
		IsActive: user.IsActive,
	})
}

func parseAndValidateRoles(roleStrings []string) ([]models.Role, error) {
	valid := map[string]models.Role{
		string(models.RoleAdmin):      models.RoleAdmin,
		string(models.RoleQA):         models.RoleQA,
		string(models.RoleData):       models.RoleData,
		string(models.RoleOperations): models.RoleOperations,
	}
	roles := make([]models.Role, 0, len(roleStrings))
	seen := make(map[models.Role]struct{})
	for _, r := range roleStrings {
		role, ok := valid[r]
		if !ok {
			return nil, errInvalidRole(r)
		}
		if _, dup := seen[role]; dup {
			continue
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	return roles, nil
}

type invalidRoleError string

func (e invalidRoleError) Error() string {
	return "invalid role: " + string(e)
}

func errInvalidRole(r string) error {
	return invalidRoleError(r)
}

func (h *AdminHandler) ensureNotLastAdmin(c *gin.Context, userID string, user *models.User) error {
	currentRoles, err := h.UserRepository.GetUserRoles(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to load user roles",
		})
		return err
	}

	isAdmin := false
	for _, r := range currentRoles {
		if r == models.RoleAdmin {
			isAdmin = true
			break
		}
	}
	if !isAdmin || !user.IsActive {
		return nil
	}

	count, err := h.UserRepository.CountActiveAdminsWithRole(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to count admins",
		})
		return err
	}
	if count == 0 {
		c.JSON(http.StatusBadRequest, models.APIError{
			Code:    "bad_request",
			Message: "cannot remove or deactivate the last active admin",
		})
		return errLastAdmin
	}
	return nil
}

var errLastAdmin = errors.New("last admin")
