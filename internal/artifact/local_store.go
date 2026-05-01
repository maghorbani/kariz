package artifact

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"

	"github.com/kariz/kariz/internal/models"
)

// LocalArtifactStore stores artifacts on the local filesystem under a configurable
// base directory, organized by execution ID.
type LocalArtifactStore struct {
	basePath string
}

// NewLocalArtifactStore creates a new LocalArtifactStore that writes files under basePath.
func NewLocalArtifactStore(basePath string) *LocalArtifactStore {
	return &LocalArtifactStore{basePath: basePath}
}

// Store saves an artifact to the local filesystem at {basePath}/{executionID}/{fileName}.
// It creates the directory structure if it doesn't exist, writes the file content,
// and returns metadata about the stored artifact.
func (s *LocalArtifactStore) Store(ctx context.Context, executionID string, label string, fileName string, content io.Reader) (*StoredArtifact, error) {
	dir := filepath.Join(s.basePath, executionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create artifact directory: %w", err)
	}

	filePath := filepath.Join(dir, fileName)

	f, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("create artifact file: %w", err)
	}
	defer f.Close()

	written, err := io.Copy(f, content)
	if err != nil {
		return nil, fmt.Errorf("write artifact content: %w", err)
	}

	contentType := detectContentType(fileName)

	return &StoredArtifact{
		StoragePath:   filePath,
		StorageType:   string(models.DestLocal),
		FileSizeBytes: written,
		ContentType:   contentType,
	}, nil
}

// GetDownloadURL returns the local file path for a locally stored artifact.
// For local storage, the path itself is the download reference.
func (s *LocalArtifactStore) GetDownloadURL(ctx context.Context, storagePath string, storageType models.ArtifactDestType) (string, error) {
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		return "", fmt.Errorf("artifact file not found: %s", storagePath)
	}
	return storagePath, nil
}

// Delete removes a locally stored artifact file.
func (s *LocalArtifactStore) Delete(ctx context.Context, storagePath string, storageType models.ArtifactDestType) error {
	if err := os.Remove(storagePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete artifact file: %w", err)
	}
	return nil
}

// detectContentType guesses the MIME type from the file extension.
// Falls back to "application/octet-stream" if unknown.
func detectContentType(fileName string) string {
	ext := filepath.Ext(fileName)
	if ext == "" {
		return "application/octet-stream"
	}
	ct := mime.TypeByExtension(ext)
	if ct == "" {
		return "application/octet-stream"
	}
	return ct
}
