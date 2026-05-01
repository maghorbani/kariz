package scheduler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kariz/kariz/internal/auth"
	"github.com/kariz/kariz/internal/models"
)

// SchedulerHandler handles HTTP endpoints for schedule management.
type SchedulerHandler struct {
	service    SchedulerService
	catalogSvc CatalogService
}

// NewSchedulerHandler creates a new SchedulerHandler with the given dependencies.
func NewSchedulerHandler(service SchedulerService, catalogSvc CatalogService) *SchedulerHandler {
	return &SchedulerHandler{
		service:    service,
		catalogSvc: catalogSvc,
	}
}

// RegisterRoutes registers schedule routes on the given router group.
// All routes require authentication.
func (h *SchedulerHandler) RegisterRoutes(rg *gin.RouterGroup, authMiddleware gin.HandlerFunc) {
	schedules := rg.Group("/schedules")
	schedules.Use(authMiddleware)

	schedules.GET("", h.ListSchedules)
	schedules.GET("/:id", h.GetSchedule)
	schedules.POST("", h.CreateSchedule)
	schedules.PUT("/:id", h.UpdateSchedule)
	schedules.DELETE("/:id", h.DeleteSchedule)
	schedules.POST("/:id/enable", h.EnableSchedule)
	schedules.POST("/:id/disable", h.DisableSchedule)
}

// CreateSchedule handles POST /api/schedules.
func (h *SchedulerHandler) CreateSchedule(c *gin.Context) {
	session := auth.GetSessionFromContext(c)
	if session == nil {
		c.JSON(http.StatusUnauthorized, models.APIError{
			Code:    "unauthorized",
			Message: "authentication required",
		})
		return
	}

	var input models.CreateScheduleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{
			Code:    "bad_request",
			Message: "invalid request body",
		})
		return
	}

	// Validate command exists and user has access
	cmd, err := h.catalogSvc.GetCommand(c.Request.Context(), input.CommandID)
	if err != nil {
		handleScheduleServiceError(c, err)
		return
	}

	// Check role access
	if !hasRoleIntersection(session.Roles, cmd.AllowedRoles) {
		c.JSON(http.StatusForbidden, models.APIError{
			Code:    "forbidden",
			Message: "access denied",
		})
		return
	}

	schedule, err := h.service.CreateSchedule(c.Request.Context(), input, session.UserID, cmd.Name)
	if err != nil {
		handleScheduleServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, schedule)
}

// UpdateSchedule handles PUT /api/schedules/:id.
func (h *SchedulerHandler) UpdateSchedule(c *gin.Context) {
	session := auth.GetSessionFromContext(c)
	if session == nil {
		c.JSON(http.StatusUnauthorized, models.APIError{
			Code:    "unauthorized",
			Message: "authentication required",
		})
		return
	}

	id := c.Param("id")

	var input models.UpdateScheduleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{
			Code:    "bad_request",
			Message: "invalid request body",
		})
		return
	}

	schedule, err := h.service.UpdateSchedule(c.Request.Context(), id, input)
	if err != nil {
		handleScheduleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, schedule)
}

// DeleteSchedule handles DELETE /api/schedules/:id.
func (h *SchedulerHandler) DeleteSchedule(c *gin.Context) {
	session := auth.GetSessionFromContext(c)
	if session == nil {
		c.JSON(http.StatusUnauthorized, models.APIError{
			Code:    "unauthorized",
			Message: "authentication required",
		})
		return
	}

	id := c.Param("id")

	if err := h.service.DeleteSchedule(c.Request.Context(), id); err != nil {
		handleScheduleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "schedule deleted"})
}

// EnableSchedule handles POST /api/schedules/:id/enable.
func (h *SchedulerHandler) EnableSchedule(c *gin.Context) {
	session := auth.GetSessionFromContext(c)
	if session == nil {
		c.JSON(http.StatusUnauthorized, models.APIError{
			Code:    "unauthorized",
			Message: "authentication required",
		})
		return
	}

	id := c.Param("id")

	if err := h.service.EnableSchedule(c.Request.Context(), id); err != nil {
		handleScheduleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "schedule enabled"})
}

// DisableSchedule handles POST /api/schedules/:id/disable.
func (h *SchedulerHandler) DisableSchedule(c *gin.Context) {
	session := auth.GetSessionFromContext(c)
	if session == nil {
		c.JSON(http.StatusUnauthorized, models.APIError{
			Code:    "unauthorized",
			Message: "authentication required",
		})
		return
	}

	id := c.Param("id")

	if err := h.service.DisableSchedule(c.Request.Context(), id); err != nil {
		handleScheduleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "schedule disabled"})
}

// ListSchedules handles GET /api/schedules.
func (h *SchedulerHandler) ListSchedules(c *gin.Context) {
	session := auth.GetSessionFromContext(c)
	if session == nil {
		c.JSON(http.StatusUnauthorized, models.APIError{
			Code:    "unauthorized",
			Message: "authentication required",
		})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	commandID := c.Query("command_id")
	enabledStr := c.Query("is_enabled")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	filter := models.ScheduleFilter{
		CommandID: commandID,
		Roles:     session.Roles,
		Page:      page,
		PageSize:  pageSize,
	}

	if enabledStr != "" {
		enabled := enabledStr == "true"
		filter.IsEnabled = &enabled
	}

	schedules, err := h.service.ListSchedules(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to list schedules",
		})
		return
	}

	c.JSON(http.StatusOK, schedules)
}

// GetSchedule handles GET /api/schedules/:id.
func (h *SchedulerHandler) GetSchedule(c *gin.Context) {
	session := auth.GetSessionFromContext(c)
	if session == nil {
		c.JSON(http.StatusUnauthorized, models.APIError{
			Code:    "unauthorized",
			Message: "authentication required",
		})
		return
	}

	id := c.Param("id")

	schedule, err := h.service.GetSchedule(c.Request.Context(), id)
	if err != nil {
		handleScheduleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, schedule)
}

// handleScheduleServiceError maps service-layer errors to appropriate HTTP responses.
func handleScheduleServiceError(c *gin.Context, err error) {
	if apiErr, ok := err.(*models.APIError); ok {
		switch apiErr.Code {
		case "not_found":
			c.JSON(http.StatusNotFound, apiErr)
		case "conflict":
			c.JSON(http.StatusConflict, apiErr)
		case "validation_error":
			c.JSON(http.StatusBadRequest, apiErr)
		case "forbidden":
			c.JSON(http.StatusForbidden, apiErr)
		default:
			c.JSON(http.StatusInternalServerError, apiErr)
		}
		return
	}

	c.JSON(http.StatusInternalServerError, models.APIError{
		Code:    "internal_error",
		Message: "an unexpected error occurred",
	})
}

// hasRoleIntersection returns true if any role in userRoles matches any role in allowedRoles.
func hasRoleIntersection(userRoles []models.Role, allowedRoles []models.Role) bool {
	for _, ur := range userRoles {
		for _, ar := range allowedRoles {
			if ur == ar {
				return true
			}
		}
	}
	return false
}
