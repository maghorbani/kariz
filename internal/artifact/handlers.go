package artifact

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kariz/kariz/internal/auth"
	"github.com/kariz/kariz/internal/models"
)

// ArtifactHandler handles HTTP endpoints for artifact listing and download.
type ArtifactHandler struct {
	repo  ExecutionArtifactRepository
	store ArtifactStore
}

// NewArtifactHandler creates a new ArtifactHandler with the given dependencies.
func NewArtifactHandler(repo ExecutionArtifactRepository, store ArtifactStore) *ArtifactHandler {
	return &ArtifactHandler{
		repo:  repo,
		store: store,
	}
}

// RegisterRoutes registers artifact routes on the given router group.
// All routes require authentication via the provided middleware.
func (h *ArtifactHandler) RegisterRoutes(rg *gin.RouterGroup, authMiddleware gin.HandlerFunc) {
	executions := rg.Group("/executions")
	executions.Use(authMiddleware)
	executions.GET("/:id/artifacts", h.ListArtifacts)
	executions.GET("/:id/artifacts/:artifactId/download", h.DownloadArtifact)
}

// ListArtifacts handles GET /api/executions/:id/artifacts.
// Returns all artifacts for the given execution ID.
func (h *ArtifactHandler) ListArtifacts(c *gin.Context) {
	session := auth.GetSessionFromContext(c)
	if session == nil {
		c.JSON(http.StatusUnauthorized, models.APIError{
			Code:    "unauthorized",
			Message: "authentication required",
		})
		return
	}

	executionID := c.Param("id")

	artifacts, err := h.repo.GetByExecutionID(c.Request.Context(), executionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to list artifacts",
		})
		return
	}

	c.JSON(http.StatusOK, artifacts)
}

// DownloadArtifact handles GET /api/executions/:id/artifacts/:artifactId/download.
// For local artifacts, it serves the file directly. For S3/MinIO artifacts, it
// redirects to a presigned URL.
func (h *ArtifactHandler) DownloadArtifact(c *gin.Context) {
	session := auth.GetSessionFromContext(c)
	if session == nil {
		c.JSON(http.StatusUnauthorized, models.APIError{
			Code:    "unauthorized",
			Message: "authentication required",
		})
		return
	}

	artifactID := c.Param("artifactId")

	artifact, err := h.repo.GetByID(c.Request.Context(), artifactID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{
			Code:    "internal_error",
			Message: "failed to get artifact",
		})
		return
	}
	if artifact == nil {
		c.JSON(http.StatusNotFound, models.APIError{
			Code:    "not_found",
			Message: "artifact not found",
		})
		return
	}

	if artifact.Status != "stored" {
		c.JSON(http.StatusUnprocessableEntity, models.APIError{
			Code:    "artifact_failed",
			Message: "artifact was not stored successfully: " + artifact.ErrorMessage,
		})
		return
	}

	storageType := models.ArtifactDestType(artifact.StorageType)

	// For local storage, serve the file directly.
	if storageType == models.DestLocal {
		downloadURL, err := h.store.GetDownloadURL(c.Request.Context(), artifact.StoragePath, storageType)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.APIError{
				Code:    "internal_error",
				Message: "artifact file not found on disk",
			})
			return
		}
		c.Header("Content-Disposition", "attachment; filename=\""+artifact.FileName+"\"")
		c.Header("Content-Type", artifact.ContentType)
		c.File(downloadURL)
		return
	}

	// For S3/MinIO, redirect to a presigned URL.
	if storageType == models.DestS3 || storageType == models.DestMinIO {
		downloadURL, err := h.store.GetDownloadURL(c.Request.Context(), artifact.StoragePath, storageType)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.APIError{
				Code:    "internal_error",
				Message: "failed to generate download URL",
			})
			return
		}
		// Presigned URLs start with http
		if strings.HasPrefix(downloadURL, "http") {
			c.Redirect(http.StatusTemporaryRedirect, downloadURL)
			return
		}
		// Fallback: serve as file
		c.File(downloadURL)
		return
	}

	c.JSON(http.StatusInternalServerError, models.APIError{
		Code:    "internal_error",
		Message: "unsupported storage type: " + artifact.StorageType,
	})
}
