package property_test

// Feature: kariz-command-dashboard, Property 18: artifact storage round-trip
// Feature: kariz-command-dashboard, Property 21: partial artifact failure

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kariz/kariz/internal/artifact"
	"github.com/kariz/kariz/internal/models"
	"pgregory.net/rapid"
)

// --- Generators ---

// genArtifactLabel generates a random human-readable label for an artifact.
func genArtifactLabel(t *rapid.T, label string) string {
	return rapid.StringMatching(`^[a-z][a-z0-9_]{2,12}`).Draw(t, label)
}

// genArtifactFileName generates a random file name with extension.
func genArtifactFileName(t *rapid.T, label string) string {
	name := rapid.StringMatching(`^[a-z][a-z0-9]{2,8}`).Draw(t, label+"_name")
	ext := rapid.SampledFrom([]string{".txt", ".csv", ".json", ".log", ".xml"}).Draw(t, label+"_ext")
	return name + ext
}

// genArtifactContent generates random file content (1-500 bytes).
func genArtifactContent(t *rapid.T, label string) []byte {
	size := rapid.IntRange(1, 500).Draw(t, label+"_size")
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(rapid.IntRange(32, 126).Draw(t, fmt.Sprintf("%s_byte%d", label, i)))
	}
	return data
}

// --- Property 18 Tests ---
// **Validates: Requirements 12.2, 12.3**

// TestProperty18_ArtifactStorageRoundTrip tests that for any N artifacts that all exist,
// after execution completes, there should be N ExecutionArtifact entries with status "stored".
// Uses LocalArtifactStore with a temp directory.
func TestProperty18_ArtifactStorageRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate 1-5 artifacts
		n := rapid.IntRange(1, 5).Draw(t, "artifactCount")

		// Create temp directory for local store
		tmpDir, err := os.MkdirTemp("", "artifact-prop18-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		store := artifact.NewLocalArtifactStore(tmpDir)
		ctx := context.Background()
		executionID := uuid.New().String()

		type artifactData struct {
			label    string
			fileName string
			content  []byte
		}

		// Generate and store N artifacts
		artifacts := make([]artifactData, n)
		storedResults := make([]*artifact.StoredArtifact, n)

		for i := 0; i < n; i++ {
			ad := artifactData{
				label:    genArtifactLabel(t, fmt.Sprintf("label%d", i)),
				fileName: genArtifactFileName(t, fmt.Sprintf("file%d", i)),
				content:  genArtifactContent(t, fmt.Sprintf("content%d", i)),
			}
			artifacts[i] = ad

			stored, err := store.Store(ctx, executionID, ad.label, ad.fileName, bytes.NewReader(ad.content))
			if err != nil {
				t.Fatalf("failed to store artifact %d: %v", i, err)
			}
			storedResults[i] = stored
		}

		// Verify: exactly N stored artifacts
		if len(storedResults) != n {
			t.Fatalf("expected %d stored artifacts, got %d", n, len(storedResults))
		}

		// Verify each stored artifact
		for i, stored := range storedResults {
			// Status should be "stored" (StorageType is "local")
			if stored.StorageType != string(models.DestLocal) {
				t.Fatalf("artifact %d: expected storage type %q, got %q", i, models.DestLocal, stored.StorageType)
			}

			// File size should match content length
			if stored.FileSizeBytes != int64(len(artifacts[i].content)) {
				t.Fatalf("artifact %d: expected size %d, got %d", i, len(artifacts[i].content), stored.FileSizeBytes)
			}

			// Verify round-trip: read back the file and compare content
			downloadPath, err := store.GetDownloadURL(ctx, stored.StoragePath, models.DestLocal)
			if err != nil {
				t.Fatalf("artifact %d: failed to get download URL: %v", i, err)
			}

			readBack, err := os.ReadFile(downloadPath)
			if err != nil {
				t.Fatalf("artifact %d: failed to read stored file: %v", i, err)
			}

			if !bytes.Equal(readBack, artifacts[i].content) {
				t.Fatalf("artifact %d: content mismatch after round-trip. Expected %d bytes, got %d bytes",
					i, len(artifacts[i].content), len(readBack))
			}

			// Verify storage path is under the expected directory
			if !strings.HasPrefix(stored.StoragePath, filepath.Join(tmpDir, executionID)) {
				t.Fatalf("artifact %d: storage path %q not under expected dir %q",
					i, stored.StoragePath, filepath.Join(tmpDir, executionID))
			}
		}
	})
}

// --- Property 21 Tests ---
// **Validates: Requirements 12.5**

// TestProperty21_PartialArtifactFailure tests that for any N artifacts where K exist
// and (N-K) are missing, the execution should still complete. There should be K "stored"
// and (N-K) "failed" artifacts.
func TestProperty21_PartialArtifactFailure(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate total artifact count (2-6) and how many exist (0 to N-1)
		n := rapid.IntRange(2, 6).Draw(t, "totalArtifacts")
		k := rapid.IntRange(0, n-1).Draw(t, "existingArtifacts")

		// Create temp directory for local store
		tmpDir, err := os.MkdirTemp("", "artifact-prop21-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		store := artifact.NewLocalArtifactStore(tmpDir)
		repo := newInMemoryArtifactRepo()
		dockerMgr := newProp21MockDockerManager()
		copier := artifact.NewArtifactCopier(dockerMgr, store, repo)

		ctx := context.Background()
		executionID := uuid.New().String()
		containerID := "container-prop21"

		// Build artifact declarations
		declarations := make([]models.ArtifactDeclare, n)
		for i := 0; i < n; i++ {
			fileName := genArtifactFileName(t, fmt.Sprintf("decl%d", i))
			declarations[i] = models.ArtifactDeclare{
				ContainerPath: fmt.Sprintf("/output/%s", fileName),
				Label:         genArtifactLabel(t, fmt.Sprintf("declLabel%d", i)),
			}
		}

		// Configure mock: first K artifacts exist, rest are missing
		for i := 0; i < k; i++ {
			content := genArtifactContent(t, fmt.Sprintf("existContent%d", i))
			dockerMgr.addFile(declarations[i].ContainerPath, content)
		}
		// Remaining (N-K) artifacts are NOT added to the mock, so CopyFromContainer will fail

		// Execute artifact copy
		destConfig := &models.ArtifactDestConfig{Type: models.DestLocal}
		copier.CopyArtifacts(ctx, executionID, containerID, declarations, destConfig)

		// Verify results
		allArtifacts := repo.getByExecutionID(executionID)

		if len(allArtifacts) != n {
			t.Fatalf("expected %d artifact records, got %d", n, len(allArtifacts))
		}

		storedCount := 0
		failedCount := 0
		for _, a := range allArtifacts {
			switch a.Status {
			case "stored":
				storedCount++
			case "failed":
				failedCount++
				if a.ErrorMessage == "" {
					t.Fatalf("failed artifact %q should have a non-empty error message", a.Label)
				}
			default:
				t.Fatalf("unexpected artifact status %q for %q", a.Status, a.Label)
			}
		}

		if storedCount != k {
			t.Fatalf("expected %d stored artifacts, got %d", k, storedCount)
		}
		if failedCount != n-k {
			t.Fatalf("expected %d failed artifacts, got %d", n-k, failedCount)
		}
	})
}

// --- Mock implementations for Property 21 ---

// prop21MockDockerManager is a mock DockerManager that serves files from an in-memory map.
type prop21MockDockerManager struct {
	files map[string][]byte // containerPath -> file content
}

func newProp21MockDockerManager() *prop21MockDockerManager {
	return &prop21MockDockerManager{
		files: make(map[string][]byte),
	}
}

func (m *prop21MockDockerManager) addFile(containerPath string, content []byte) {
	m.files[containerPath] = content
}

// CopyFromContainer returns a tar stream containing the file if it exists in the mock.
func (m *prop21MockDockerManager) CopyFromContainer(ctx context.Context, containerID string, srcPath string) (io.ReadCloser, error) {
	content, ok := m.files[srcPath]
	if !ok {
		return nil, fmt.Errorf("file not found in container: %s", srcPath)
	}

	// Create a tar archive containing the file
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	fileName := filepath.Base(srcPath)
	hdr := &tar.Header{
		Name: fileName,
		Mode: 0644,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, fmt.Errorf("write tar header: %w", err)
	}
	if _, err := tw.Write(content); err != nil {
		return nil, fmt.Errorf("write tar content: %w", err)
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("close tar writer: %w", err)
	}

	return io.NopCloser(bytes.NewReader(buf.Bytes())), nil
}

// Unused DockerManager methods — satisfy the interface.
func (m *prop21MockDockerManager) CreateContainer(ctx context.Context, config models.ContainerConfig) (string, error) {
	return "", fmt.Errorf("not implemented")
}
func (m *prop21MockDockerManager) StartContainer(ctx context.Context, containerID string) error {
	return fmt.Errorf("not implemented")
}
func (m *prop21MockDockerManager) AttachStream(ctx context.Context, containerID string) (<-chan models.OutputChunk, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *prop21MockDockerManager) StopContainer(ctx context.Context, containerID string, timeout int) error {
	return fmt.Errorf("not implemented")
}
func (m *prop21MockDockerManager) RemoveContainer(ctx context.Context, containerID string) error {
	return fmt.Errorf("not implemented")
}
func (m *prop21MockDockerManager) IsAvailable(ctx context.Context) error {
	return nil
}
func (m *prop21MockDockerManager) ExecInContainer(ctx context.Context, containerID string, command []string) (string, error) {
	return "", fmt.Errorf("not implemented")
}
func (m *prop21MockDockerManager) AttachExecStream(ctx context.Context, execID string) (<-chan models.OutputChunk, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *prop21MockDockerManager) InspectExec(ctx context.Context, execID string) (*models.ExecInspectResult, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *prop21MockDockerManager) InspectContainerEnv(ctx context.Context, containerNameOrID string) (map[string]string, error) {
	return nil, fmt.Errorf("not implemented")
}

// --- In-memory artifact repository for Property 21 ---

type inMemoryArtifactRepo struct {
	mu        sync.Mutex
	artifacts map[string][]*models.ExecutionArtifact // executionID -> artifacts
}

func newInMemoryArtifactRepo() *inMemoryArtifactRepo {
	return &inMemoryArtifactRepo{
		artifacts: make(map[string][]*models.ExecutionArtifact),
	}
}

func (r *inMemoryArtifactRepo) Create(ctx context.Context, a *models.ExecutionArtifact) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.artifacts[a.ExecutionID] = append(r.artifacts[a.ExecutionID], a)
	return nil
}

func (r *inMemoryArtifactRepo) GetByExecutionID(ctx context.Context, executionID string) ([]models.ExecutionArtifact, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ptrs := r.artifacts[executionID]
	result := make([]models.ExecutionArtifact, len(ptrs))
	for i, p := range ptrs {
		result[i] = *p
	}
	return result, nil
}

func (r *inMemoryArtifactRepo) GetByID(ctx context.Context, id string) (*models.ExecutionArtifact, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, list := range r.artifacts {
		for _, a := range list {
			if a.ID == id {
				return a, nil
			}
		}
	}
	return nil, nil
}

func (r *inMemoryArtifactRepo) Delete(ctx context.Context, id string) error {
	return nil
}

func (r *inMemoryArtifactRepo) getByExecutionID(executionID string) []*models.ExecutionArtifact {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.artifacts[executionID]
}
