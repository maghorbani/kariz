package notification

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kariz/kariz/internal/auth"
	"github.com/kariz/kariz/internal/models"
)

// NotificationHandler handles HTTP endpoints for notifications.
type NotificationHandler struct {
	service NotificationService
}

// NewNotificationHandler creates a new NotificationHandler with the given service.
func NewNotificationHandler(service NotificationService) *NotificationHandler {
	return &NotificationHandler{service: service}
}

// RegisterRoutes registers notification routes on the given router group.
// All routes require authentication.
func (h *NotificationHandler) RegisterRoutes(rg *gin.RouterGroup, authMiddleware gin.HandlerFunc) {
	notifications := rg.Group("/notifications")
	notifications.Use(authMiddleware)
	notifications.GET("", h.GetNotifications)
	notifications.PUT("/:id/read", h.MarkAsRead)
}

// GetNotifications handles GET /api/notifications.
// Returns notifications for the authenticated user.
// Query parameter: unread_only=true/false (default false).
func (h *NotificationHandler) GetNotifications(c *gin.Context) {
	session := auth.GetSessionFromContext(c)
	if session == nil {
		c.JSON(http.StatusUnauthorized, models.APIError{
			Code:    "unauthorized",
			Message: "authentication required",
		})
		return
	}

	unreadOnly := strings.EqualFold(c.DefaultQuery("unread_only", "false"), "true")

	notifications, err := h.service.GetNotifications(c.Request.Context(), session.UserID, unreadOnly)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to get notifications",
		})
		return
	}

	c.JSON(http.StatusOK, notifications)
}

// MarkAsRead handles PUT /api/notifications/:id/read.
// Marks a notification as read.
func (h *NotificationHandler) MarkAsRead(c *gin.Context) {
	session := auth.GetSessionFromContext(c)
	if session == nil {
		c.JSON(http.StatusUnauthorized, models.APIError{
			Code:    "unauthorized",
			Message: "authentication required",
		})
		return
	}

	notificationID := c.Param("id")

	if err := h.service.MarkAsRead(c.Request.Context(), notificationID); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to mark notification as read",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "notification marked as read"})
}
