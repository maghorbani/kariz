package artifact

import (
	"context"
	"io"

	"github.com/kariz/kariz/internal/models"
)

// StoredArtifact holds metadata about a successfully stored artifact.
type StoredArtifact struct {
	StoragePath   string `json:"storage_path"`
	StorageType   string `json:"storage_type"`
	FileSizeBytes int64  `json:"file_size_bytes"`
	ContentType   string `json:"content_type"`
}

// ArtifactStore abstracts local filesystem and S3-compatible object storage
// behind a unified interface for storing and retrieving execution artifacts.
type ArtifactStore interface {
	// Store saves an artifact from a reader to the configured storage backend.
	// Returns the stored artifact metadata including storage path and file size.
	Store(ctx context.Context, executionID string, label string, fileName string, content io.Reader) (*StoredArtifact, error)

	// GetDownloadURL returns a URL or file path for downloading an artifact.
	// For S3/MinIO, returns a presigned URL. For local, returns a file-serve path.
	GetDownloadURL(ctx context.Context, storagePath string, storageType models.ArtifactDestType) (string, error)

	// Delete removes an artifact from storage.
	Delete(ctx context.Context, storagePath string, storageType models.ArtifactDestType) error
}
