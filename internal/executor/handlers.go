package executor

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kariz/kariz/internal/auth"
	"github.com/kariz/kariz/internal/catalog"
	"github.com/kariz/kariz/internal/models"
)

// ExecutorHandler handles HTTP endpoints for command execution.
type ExecutorHandler struct {
	executorSvc ExecutorService
	catalogSvc  catalog.CatalogService
	repo        ExecutionRepository
}

// NewExecutorHandler creates a new ExecutorHandler with the given dependencies.
func NewExecutorHandler(executorSvc ExecutorService, catalogSvc catalog.CatalogService, repo ExecutionRepository) *ExecutorHandler {
	return &ExecutorHandler{
		executorSvc: executorSvc,
		catalogSvc:  catalogSvc,
		repo:        repo,
	}
}

// RegisterRoutes registers execution routes on the given router group.
// All routes require authentication.
func (h *ExecutorHandler) RegisterRoutes(rg *gin.RouterGroup, authMiddleware gin.HandlerFunc) {
	// Execute command — role-based access checked in handler.
	commands := rg.Group("/commands")
	commands.Use(authMiddleware)
	commands.POST("/:id/execute", h.ExecuteCommand)

	// Execution management.
	executions := rg.Group("/executions")
	executions.Use(authMiddleware)
	executions.GET("", h.ListExecutions)
	executions.GET("/:id", h.GetExecution)
	executions.POST("/:id/cancel", h.CancelExecution)
}

// executeRequest is the JSON body for POST /api/commands/:id/execute.
type executeRequest struct {
	Parameters map[string]interface{} `json:"parameters"`
}

// ExecuteCommand handles POST /api/commands/:id/execute.
// It retrieves the command from the catalog, checks role access, and executes.
func (h *ExecutorHandler) ExecuteCommand(c *gin.Context) {
	session := auth.GetSessionFromContext(c)
	if session == nil {
		c.JSON(http.StatusUnauthorized, models.APIError{
			Code:    "unauthorized",
			Message: "authentication required",
		})
		return
	}

	commandID := c.Param("id")

	// Get command from catalog.
	cmd, err := h.catalogSvc.GetCommand(c.Request.Context(), commandID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	// Check role access.
	if !hasRoleIntersection(session.Roles, cmd.AllowedRoles) {
		c.JSON(http.StatusForbidden, models.APIError{
			Code:    "forbidden",
			Message: "access denied",
		})
		return
	}

	// Parse request body.
	var req executeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body — treat as no parameters.
		req.Parameters = make(map[string]interface{})
	}
	if req.Parameters == nil {
		req.Parameters = make(map[string]interface{})
	}

	// Execute command.
	record, err := h.executorSvc.ExecuteCommand(c.Request.Context(), *cmd, req.Parameters, session.UserID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusAccepted, record)
}

// ListExecutions handles GET /api/executions.
// Returns a paginated list of execution records with optional filters.
func (h *ExecutorHandler) ListExecutions(c *gin.Context) {
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
	commandName := c.Query("command_name")
	userID := c.Query("user_id")
	status := c.Query("status")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	filter := models.ExecutionFilter{
		CommandName: commandName,
		UserID:      userID,
		Status:      models.ExecutionStatus(status),
		Page:        page,
		PageSize:    pageSize,
	}

	result, err := h.repo.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to list executions",
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetExecution handles GET /api/executions/:id.
// Returns the execution details for the given ID.
func (h *ExecutorHandler) GetExecution(c *gin.Context) {
	session := auth.GetSessionFromContext(c)
	if session == nil {
		c.JSON(http.StatusUnauthorized, models.APIError{
			Code:    "unauthorized",
			Message: "authentication required",
		})
		return
	}

	executionID := c.Param("id")

	record, err := h.repo.GetByID(c.Request.Context(), executionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to get execution",
		})
		return
	}
	if record == nil {
		c.JSON(http.StatusNotFound, models.APIError{
			Code:    "not_found",
			Message: "execution not found",
		})
		return
	}

	c.JSON(http.StatusOK, record)
}

// CancelExecution handles POST /api/executions/:id/cancel.
// Cancels a running execution.
func (h *ExecutorHandler) CancelExecution(c *gin.Context) {
	session := auth.GetSessionFromContext(c)
	if session == nil {
		c.JSON(http.StatusUnauthorized, models.APIError{
			Code:    "unauthorized",
			Message: "authentication required",
		})
		return
	}

	executionID := c.Param("id")

	if err := h.executorSvc.CancelExecution(c.Request.Context(), executionID); err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "execution cancelled"})
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
