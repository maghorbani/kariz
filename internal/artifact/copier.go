package artifact

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/kariz/kariz/internal/docker"
	"github.com/kariz/kariz/internal/models"
)

// ArtifactCopier copies declared artifacts from a Docker container into an
// ArtifactStore and records each result in the ExecutionArtifactRepository.
type ArtifactCopier struct {
	dockerMgr docker.DockerManager
	store     ArtifactStore
	repo      ExecutionArtifactRepository
}

// NewArtifactCopier creates a new ArtifactCopier with the given dependencies.
func NewArtifactCopier(dockerMgr docker.DockerManager, store ArtifactStore, repo ExecutionArtifactRepository) *ArtifactCopier {
	return &ArtifactCopier{
		dockerMgr: dockerMgr,
		store:     store,
		repo:      repo,
	}
}

// CopyArtifacts iterates over declared artifacts, copies each from the container,
// stores it via the ArtifactStore, and creates an ExecutionArtifact record.
//
// Partial failure handling (Task 22.5): if CopyFromContainer fails for a file,
// an ExecutionArtifact record with status "failed" and the error message is created,
// and processing continues with the remaining artifacts. The method does not fail
// the entire execution.
func (c *ArtifactCopier) CopyArtifacts(
	ctx context.Context,
	executionID string,
	containerID string,
	artifacts []models.ArtifactDeclare,
	destConfig *models.ArtifactDestConfig,
) {
	for _, decl := range artifacts {
		c.copyOne(ctx, executionID, containerID, decl, destConfig)
	}
}

// copyOne handles a single artifact: copy from container → extract from tar → store → record.
func (c *ArtifactCopier) copyOne(
	ctx context.Context,
	executionID string,
	containerID string,
	decl models.ArtifactDeclare,
	destConfig *models.ArtifactDestConfig,
) {
	fileName := filepath.Base(decl.ContainerPath)
	now := time.Now().UTC()

	// Step 1: Copy from container (returns a tar stream).
	tarReader, err := c.dockerMgr.CopyFromContainer(ctx, containerID, decl.ContainerPath)
	if err != nil {
		slog.Warn("failed to copy artifact from container",
			"execution_id", executionID,
			"container_path", decl.ContainerPath,
			"error", err,
		)
		c.recordFailed(ctx, executionID, decl.Label, fileName, now, fmt.Sprintf("copy from container failed: %v", err))
		return
	}
	defer func() { _ = tarReader.Close() }()

	// Step 2: Extract the file from the tar stream.
	fileReader, err := extractFileFromTar(tarReader)
	if err != nil {
		slog.Warn("failed to extract artifact from tar stream",
			"execution_id", executionID,
			"container_path", decl.ContainerPath,
			"error", err,
		)
		c.recordFailed(ctx, executionID, decl.Label, fileName, now, fmt.Sprintf("extract from tar failed: %v", err))
		return
	}

	// Step 3: Store the artifact.
	stored, err := c.store.Store(ctx, executionID, decl.Label, fileName, fileReader)
	if err != nil {
		slog.Warn("failed to store artifact",
			"execution_id", executionID,
			"file_name", fileName,
			"error", err,
		)
		c.recordFailed(ctx, executionID, decl.Label, fileName, now, fmt.Sprintf("store artifact failed: %v", err))
		return
	}

	// Step 4: Create a successful artifact record.
	artifact := &models.ExecutionArtifact{
		ID:            uuid.New().String(),
		ExecutionID:   executionID,
		Label:         decl.Label,
		FileName:      fileName,
		FileSizeBytes: stored.FileSizeBytes,
		ContentType:   stored.ContentType,
		StoragePath:   stored.StoragePath,
		StorageType:   stored.StorageType,
		Status:        "stored",
		CreatedAt:     now,
	}

	if err := c.repo.Create(ctx, artifact); err != nil {
		slog.Error("failed to record stored artifact",
			"execution_id", executionID,
			"artifact_id", artifact.ID,
			"error", err,
		)
	}
}

// recordFailed creates an ExecutionArtifact record with status "failed" and the given error message.
func (c *ArtifactCopier) recordFailed(ctx context.Context, executionID, label, fileName string, createdAt time.Time, errMsg string) {
	artifact := &models.ExecutionArtifact{
		ID:           uuid.New().String(),
		ExecutionID:  executionID,
		Label:        label,
		FileName:     fileName,
		StorageType:  "local",
		Status:       "failed",
		ErrorMessage: errMsg,
		CreatedAt:    createdAt,
	}

	if err := c.repo.Create(ctx, artifact); err != nil {
		slog.Error("failed to record failed artifact",
			"execution_id", executionID,
			"label", label,
			"error", err,
		)
	}
}

// extractFileFromTar reads the first file entry from a tar archive stream.
// Docker's CopyFromContainer returns a tar archive containing the requested file.
func extractFileFromTar(r io.Reader) (io.Reader, error) {
	tr := tar.NewReader(r)

	header, err := tr.Next()
	if err != nil {
		return nil, fmt.Errorf("read tar header: %w", err)
	}

	if header.Typeflag == tar.TypeDir {
		// Skip directory entries and try the next entry.
		header, err = tr.Next()
		if err != nil {
			return nil, fmt.Errorf("read tar file entry after directory: %w", err)
		}
	}

	if header.Typeflag != tar.TypeReg {
		return nil, fmt.Errorf("unexpected tar entry type: %d", header.Typeflag)
	}

	return tr, nil
}
