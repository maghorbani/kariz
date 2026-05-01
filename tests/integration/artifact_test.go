package integration

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kariz/kariz/internal/artifact"
	"github.com/kariz/kariz/internal/models"
)

// --- In-memory ExecutionArtifactRepository ---

type inMemoryArtifactRepo struct {
	mu        sync.Mutex
	artifacts map[string]*models.ExecutionArtifact
}

func newInMemoryArtifactRepo() *inMemoryArtifactRepo {
	return &inMemoryArtifactRepo{
		artifacts: make(map[string]*models.ExecutionArtifact),
	}
}

func (r *inMemoryArtifactRepo) Create(ctx context.Context, a *models.ExecutionArtifact) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.artifacts[a.ID] = a
	return nil
}

func (r *inMemoryArtifactRepo) GetByExecutionID(ctx context.Context, executionID string) ([]models.ExecutionArtifact, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []models.ExecutionArtifact
	for _, a := range r.artifacts {
		if a.ExecutionID == executionID {
			result = append(result, *a)
		}
	}
	return result, nil
}

func (r *inMemoryArtifactRepo) GetByID(ctx context.Context, id string) (*models.ExecutionArtifact, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.artifacts[id]
	if !ok {
		return nil, nil
	}
	return a, nil
}

func (r *inMemoryArtifactRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.artifacts, id)
	return nil
}

func (r *inMemoryArtifactRepo) getAll() []*models.ExecutionArtifact {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []*models.ExecutionArtifact
	for _, a := range r.artifacts {
		result = append(result, a)
	}
	return result
}

// createTarArchive creates a tar archive containing a single file with the given name and content.
func createTarArchive(t *testing.T, fileName string, content []byte) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	hdr := &tar.Header{
		Name: fileName,
		Mode: 0644,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("failed to write tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("failed to write tar content: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	return &buf
}

// TestArtifactStoreAndDownload tests the full artifact lifecycle:
// Store an artifact → verify it's on disk → download (read) it → verify content → delete.
func TestArtifactStoreAndDownload(t *testing.T) {
	// Create a temp directory for artifact storage.
	tmpDir, err := os.MkdirTemp("", "kariz-artifact-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store := artifact.NewLocalArtifactStore(tmpDir)
	ctx := context.Background()

	// 1. Store an artifact.
	content := []byte("Hello, this is test artifact content.\nLine 2.\n")
	reader := bytes.NewReader(content)

	stored, err := store.Store(ctx, "exec-001", "Test Report", "report.txt", reader)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if stored.FileSizeBytes != int64(len(content)) {
		t.Errorf("file size = %d, want %d", stored.FileSizeBytes, len(content))
	}
	if stored.StorageType != "local" {
		t.Errorf("storage type = %q, want local", stored.StorageType)
	}

	// 2. Verify the file exists on disk.
	expectedPath := filepath.Join(tmpDir, "exec-001", "report.txt")
	if stored.StoragePath != expectedPath {
		t.Errorf("storage path = %q, want %q", stored.StoragePath, expectedPath)
	}

	info, err := os.Stat(stored.StoragePath)
	if err != nil {
		t.Fatalf("artifact file not found on disk: %v", err)
	}
	if info.Size() != int64(len(content)) {
		t.Errorf("file size on disk = %d, want %d", info.Size(), len(content))
	}

	// 3. Get download URL (for local, it's the file path).
	downloadPath, err := store.GetDownloadURL(ctx, stored.StoragePath, models.DestLocal)
	if err != nil {
		t.Fatalf("GetDownloadURL failed: %v", err)
	}
	if downloadPath != stored.StoragePath {
		t.Errorf("download path = %q, want %q", downloadPath, stored.StoragePath)
	}

	// 4. Read the file and verify content.
	readContent, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatalf("failed to read artifact file: %v", err)
	}
	if !bytes.Equal(readContent, content) {
		t.Errorf("artifact content mismatch: got %q, want %q", readContent, content)
	}

	// 5. Delete the artifact.
	if err := store.Delete(ctx, stored.StoragePath, models.DestLocal); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify file is gone.
	if _, err := os.Stat(stored.StoragePath); !os.IsNotExist(err) {
		t.Error("expected artifact file to be deleted")
	}
}

// TestArtifactCopier_WithMockDocker tests the ArtifactCopier using a mock Docker
// that returns a tar stream containing a file. Also tests partial failure handling.
func TestArtifactCopier_WithMockDocker(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "kariz-copier-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store := artifact.NewLocalArtifactStore(tmpDir)
	repo := newInMemoryArtifactRepo()

	// Create a mock Docker that returns a tar stream for one file and fails for another.
	fileContent := []byte("artifact file content here")
	dockerMgr := &mockDockerManager{
		copyFromContainerFn: func(ctx context.Context, containerID string, srcPath string) (io.ReadCloser, error) {
			if srcPath == "/output/report.csv" {
				tarBuf := createTarArchive(t, "report.csv", fileContent)
				return io.NopCloser(tarBuf), nil
			}
			return nil, fmt.Errorf("file not found in container: %s", srcPath)
		},
	}

	copier := artifact.NewArtifactCopier(dockerMgr, store, repo)

	// Declare artifacts: one that exists and one that doesn't.
	artifacts := []models.ArtifactDeclare{
		{ContainerPath: "/output/report.csv", Label: "CSV Report"},
		{ContainerPath: "/output/missing.log", Label: "Missing Log"},
	}

	// Copy artifacts.
	copier.CopyArtifacts(context.Background(), "exec-copier-1", "container-001", artifacts, nil)

	// Wait a moment for processing.
	time.Sleep(500 * time.Millisecond)

	// Verify results.
	allArtifacts := repo.getAll()
	if len(allArtifacts) != 2 {
		t.Fatalf("expected 2 artifact records, got %d", len(allArtifacts))
	}

	var storedCount, failedCount int
	for _, a := range allArtifacts {
		switch a.Status {
		case "stored":
			storedCount++
			if a.Label != "CSV Report" {
				t.Errorf("stored artifact label = %q, want CSV Report", a.Label)
			}
			if a.FileName != "report.csv" {
				t.Errorf("stored artifact filename = %q, want report.csv", a.FileName)
			}
			if a.FileSizeBytes != int64(len(fileContent)) {
				t.Errorf("stored artifact size = %d, want %d", a.FileSizeBytes, len(fileContent))
			}
			// Verify the file content on disk.
			diskContent, err := os.ReadFile(a.StoragePath)
			if err != nil {
				t.Errorf("failed to read stored artifact from disk: %v", err)
			} else if !bytes.Equal(diskContent, fileContent) {
				t.Errorf("artifact content mismatch on disk")
			}
		case "failed":
			failedCount++
			if a.Label != "Missing Log" {
				t.Errorf("failed artifact label = %q, want Missing Log", a.Label)
			}
			if a.ErrorMessage == "" {
				t.Error("expected error message for failed artifact")
			}
		}
	}

	if storedCount != 1 {
		t.Errorf("expected 1 stored artifact, got %d", storedCount)
	}
	if failedCount != 1 {
		t.Errorf("expected 1 failed artifact, got %d", failedCount)
	}
}
