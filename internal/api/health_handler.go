package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kariz/kariz/internal/docker"
)

// HealthHandler handles the health check endpoint.
type HealthHandler struct {
	DB            *sqlx.DB
	DockerManager docker.DockerManager
}

// NewHealthHandler creates a new HealthHandler with the given dependencies.
func NewHealthHandler(db *sqlx.DB, dockerMgr docker.DockerManager) *HealthHandler {
	return &HealthHandler{
		DB:            db,
		DockerManager: dockerMgr,
	}
}

// subsystemStatus represents the health status of a single subsystem.
type subsystemStatus struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// healthResponse is the JSON response for the health check endpoint.
type healthResponse struct {
	Status   string          `json:"status"`
	Database subsystemStatus `json:"database"`
	Docker   subsystemStatus `json:"docker"`
}

// RegisterRoutes registers the health check route on the given router group.
// The health endpoint is public (no authentication required).
func (h *HealthHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", h.HealthCheck)
}

// HealthCheck handles GET /api/health.
// It checks database connectivity and Docker daemon availability,
// returning JSON with the status of each subsystem and an overall status.
func (h *HealthHandler) HealthCheck(c *gin.Context) {
	resp := healthResponse{
		Status:   "healthy",
		Database: h.checkDatabase(c),
		Docker:   h.checkDocker(c),
	}

	// If any subsystem is unhealthy, the overall status is unhealthy.
	if resp.Database.Status != "healthy" || resp.Docker.Status != "healthy" {
		resp.Status = "unhealthy"
		c.JSON(http.StatusServiceUnavailable, resp)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// checkDatabase pings the database and returns its health status.
func (h *HealthHandler) checkDatabase(c *gin.Context) subsystemStatus {
	if h.DB == nil {
		return subsystemStatus{
			Status: "unhealthy",
			Error:  "database connection not configured",
		}
	}

	if err := h.DB.PingContext(c.Request.Context()); err != nil {
		return subsystemStatus{
			Status: "unhealthy",
			Error:  "database ping failed",
		}
	}

	return subsystemStatus{Status: "healthy"}
}

// checkDocker checks Docker daemon availability and returns its health status.
func (h *HealthHandler) checkDocker(c *gin.Context) subsystemStatus {
	if h.DockerManager == nil {
		return subsystemStatus{
			Status: "unhealthy",
			Error:  "Docker manager not configured",
		}
	}

	if err := h.DockerManager.IsAvailable(c.Request.Context()); err != nil {
		return subsystemStatus{
			Status: "unhealthy",
			Error:  "Docker daemon unreachable",
		}
	}

	return subsystemStatus{Status: "healthy"}
}
