package artifact

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kariz/kariz/internal/models"
)

func TestLocalArtifactStore_ImplementsInterface(t *testing.T) {
	var _ ArtifactStore = (*LocalArtifactStore)(nil)
}

func TestNewLocalArtifactStore(t *testing.T) {
	store := NewLocalArtifactStore("/tmp/artifacts")
	if store == nil {
		t.Fatal("expected non-nil store")
	}
	if store.basePath != "/tmp/artifacts" {
		t.Errorf("expected basePath /tmp/artifacts, got %s", store.basePath)
	}
}

func TestLocalArtifactStore_Store(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewLocalArtifactStore(tmpDir)
	ctx := context.Background()

	content := []byte("SELECT * FROM users;")
	reader := bytes.NewReader(content)

	result, err := store.Store(ctx, "exec-123", "DB Dump", "dump.sql", reader)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if result.FileSizeBytes != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), result.FileSizeBytes)
	}
	if result.StorageType != "local" {
		t.Errorf("expected storage type 'local', got %s", result.StorageType)
	}
	expectedPath := filepath.Join(tmpDir, "exec-123", "dump.sql")
	if result.StoragePath != expectedPath {
		t.Errorf("expected path %s, got %s", expectedPath, result.StoragePath)
	}

	// Verify file was actually written.
	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed to read stored file: %v", err)
	}
	if !bytes.Equal(data, content) {
		t.Errorf("stored content mismatch: got %q, want %q", data, content)
	}
}

func TestLocalArtifactStore_Store_CreatesDirectoryStructure(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewLocalArtifactStore(tmpDir)
	ctx := context.Background()

	_, err := store.Store(ctx, "exec-456", "Report", "report.csv", strings.NewReader("a,b,c"))
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	dirPath := filepath.Join(tmpDir, "exec-456")
	info, err := os.Stat(dirPath)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory, got file")
	}
}

func TestLocalArtifactStore_Store_ContentType(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewLocalArtifactStore(tmpDir)
	ctx := context.Background()

	tests := []struct {
		fileName    string
		wantContain string
	}{
		{"dump.sql", ""},          // .sql may not have a registered MIME type
		{"report.csv", "text/csv"},
		{"data.json", "application/json"},
		{"archive.tar.gz", ""},
		{"noext", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.fileName, func(t *testing.T) {
			result, err := store.Store(ctx, "exec-ct", "test", tt.fileName, strings.NewReader("data"))
			if err != nil {
				t.Fatalf("Store failed: %v", err)
			}
			if result.ContentType == "" {
				t.Error("expected non-empty content type")
			}
			if tt.wantContain != "" && !strings.Contains(result.ContentType, tt.wantContain) {
				t.Errorf("expected content type containing %q, got %q", tt.wantContain, result.ContentType)
			}
		})
	}
}

func TestLocalArtifactStore_Store_EmptyContent(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewLocalArtifactStore(tmpDir)
	ctx := context.Background()

	result, err := store.Store(ctx, "exec-empty", "Empty", "empty.txt", strings.NewReader(""))
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	if result.FileSizeBytes != 0 {
		t.Errorf("expected size 0, got %d", result.FileSizeBytes)
	}
}

func TestLocalArtifactStore_GetDownloadURL(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewLocalArtifactStore(tmpDir)
	ctx := context.Background()

	// Store a file first.
	result, err := store.Store(ctx, "exec-dl", "Test", "test.txt", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	url, err := store.GetDownloadURL(ctx, result.StoragePath, models.DestLocal)
	if err != nil {
		t.Fatalf("GetDownloadURL failed: %v", err)
	}
	if url != result.StoragePath {
		t.Errorf("expected URL %s, got %s", result.StoragePath, url)
	}
}

func TestLocalArtifactStore_GetDownloadURL_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewLocalArtifactStore(tmpDir)
	ctx := context.Background()

	_, err := store.GetDownloadURL(ctx, filepath.Join(tmpDir, "nonexistent", "file.txt"), models.DestLocal)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLocalArtifactStore_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewLocalArtifactStore(tmpDir)
	ctx := context.Background()

	// Store a file first.
	result, err := store.Store(ctx, "exec-del", "Test", "test.txt", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Verify file exists.
	if _, err := os.Stat(result.StoragePath); err != nil {
		t.Fatalf("file should exist before delete: %v", err)
	}

	// Delete.
	if err := store.Delete(ctx, result.StoragePath, models.DestLocal); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify file is gone.
	if _, err := os.Stat(result.StoragePath); !os.IsNotExist(err) {
		t.Error("file should not exist after delete")
	}
}

func TestLocalArtifactStore_Delete_NonexistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewLocalArtifactStore(tmpDir)
	ctx := context.Background()

	// Deleting a nonexistent file should not return an error.
	err := store.Delete(ctx, filepath.Join(tmpDir, "nonexistent.txt"), models.DestLocal)
	if err != nil {
		t.Fatalf("Delete of nonexistent file should not error, got: %v", err)
	}
}

func TestLocalArtifactStore_Store_MultipleArtifactsSameExecution(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewLocalArtifactStore(tmpDir)
	ctx := context.Background()

	r1, err := store.Store(ctx, "exec-multi", "Dump", "dump.sql", strings.NewReader("sql data"))
	if err != nil {
		t.Fatalf("Store 1 failed: %v", err)
	}

	r2, err := store.Store(ctx, "exec-multi", "Report", "report.csv", strings.NewReader("csv data"))
	if err != nil {
		t.Fatalf("Store 2 failed: %v", err)
	}

	if r1.StoragePath == r2.StoragePath {
		t.Error("expected different storage paths for different files")
	}

	// Both files should exist.
	for _, path := range []string{r1.StoragePath, r2.StoragePath} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("file should exist: %s", path)
		}
	}
}

func TestDetectContentType(t *testing.T) {
	tests := []struct {
		fileName string
		want     string
	}{
		{"file.json", "application/json"},
		{"file.csv", "text/csv"},
		{"file.txt", "text/plain"},
		{"file", "application/octet-stream"},
		{"", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.fileName, func(t *testing.T) {
			got := detectContentType(tt.fileName)
			if !strings.Contains(got, tt.want) {
				t.Errorf("detectContentType(%q) = %q, want containing %q", tt.fileName, got, tt.want)
			}
		})
	}
}
