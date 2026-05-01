package catalog

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kariz/kariz/internal/auth"
	"github.com/kariz/kariz/internal/models"
)

// CatalogHandler handles HTTP endpoints for the command catalog.
type CatalogHandler struct {
	service CatalogService
}

// NewCatalogHandler creates a new CatalogHandler with the given service.
func NewCatalogHandler(service CatalogService) *CatalogHandler {
	return &CatalogHandler{service: service}
}

// RegisterRoutes registers catalog routes on the given router group.
// All routes require authentication. POST, PUT, DELETE require admin role.
func (h *CatalogHandler) RegisterRoutes(rg *gin.RouterGroup, authMiddleware gin.HandlerFunc) {
	commands := rg.Group("/commands")
	commands.Use(authMiddleware)

	// Authenticated routes (any role)
	commands.GET("", h.ListCommands)
	commands.GET("/:id", h.GetCommand)

	// Admin-only routes
	admin := commands.Group("")
	admin.Use(auth.RequireRole(models.RoleAdmin))
	admin.POST("", h.CreateCommand)
	admin.PUT("/:id", h.UpdateCommand)
	admin.DELETE("/:id", h.DeactivateCommand)
}

// ListCommands handles GET /api/commands.
// Returns a paginated list of commands filtered by the authenticated user's roles.
// Supports optional query parameters: page, page_size, category, search.
func (h *CatalogHandler) ListCommands(c *gin.Context) {
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
	category := c.Query("category")
	search := c.Query("search")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	// If search is provided, use SearchCommands
	if search != "" {
		entries, err := h.service.SearchCommands(c.Request.Context(), search, session.Roles)
		if err != nil {
			handleServiceError(c, err)
			return
		}
		c.JSON(http.StatusOK, models.PaginatedResult{
			Items:    entries,
			Total:    len(entries),
			Page:     1,
			PageSize: len(entries),
		})
		return
	}

	filter := models.CommandFilter{
		Roles:    session.Roles,
		Page:     page,
		PageSize: pageSize,
		Category: category,
	}

	result, err := h.service.ListCommands(c.Request.Context(), filter)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetCommand handles GET /api/commands/:id.
// Returns the command details for the given ID.
func (h *CatalogHandler) GetCommand(c *gin.Context) {
	id := c.Param("id")

	entry, err := h.service.GetCommand(c.Request.Context(), id)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, entry)
}

// CreateCommand handles POST /api/commands.
// Creates a new command entry (admin only).
func (h *CatalogHandler) CreateCommand(c *gin.Context) {
	var input models.CreateCommandInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{
			Code:    "bad_request",
			Message: "invalid request body",
		})
		return
	}

	entry, err := h.service.CreateCommand(c.Request.Context(), input)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, entry)
}

// UpdateCommand handles PUT /api/commands/:id.
// Updates an existing command entry (admin only).
func (h *CatalogHandler) UpdateCommand(c *gin.Context) {
	id := c.Param("id")

	var input models.UpdateCommandInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{
			Code:    "bad_request",
			Message: "invalid request body",
		})
		return
	}

	entry, err := h.service.UpdateCommand(c.Request.Context(), id, input)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, entry)
}

// DeactivateCommand handles DELETE /api/commands/:id.
// Deactivates a command entry (admin only).
func (h *CatalogHandler) DeactivateCommand(c *gin.Context) {
	id := c.Param("id")

	if err := h.service.DeactivateCommand(c.Request.Context(), id); err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "command deactivated"})
}

// handleServiceError maps service-layer errors to appropriate HTTP responses.
func handleServiceError(c *gin.Context, err error) {
	if apiErr, ok := err.(*models.APIError); ok {
		switch apiErr.Code {
		case "not_found":
			c.JSON(http.StatusNotFound, apiErr)
		case "conflict":
			c.JSON(http.StatusConflict, apiErr)
		case "validation_error":
			c.JSON(http.StatusBadRequest, apiErr)
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
