package artifact

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kariz/kariz/internal/models"
)

// newTestArtifact creates an ExecutionArtifact with sensible defaults for testing.
func newTestArtifact(executionID string) *models.ExecutionArtifact {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return &models.ExecutionArtifact{
		ID:            uuid.New().String(),
		ExecutionID:   executionID,
		Label:         "DB Dump",
		FileName:      "dump.sql",
		FileSizeBytes: 1024,
		ContentType:   "application/sql",
		StoragePath:   "/artifacts/" + uuid.New().String() + "/dump.sql",
		StorageType:   "local",
		Status:        "stored",
		ErrorMessage:  "",
		CreatedAt:     now,
	}
}

// --- Interface compliance ---

func TestExecutionArtifactRepositoryImplementsInterface(t *testing.T) {
	// Compile-time check that executionArtifactRepository satisfies ExecutionArtifactRepository.
	var _ ExecutionArtifactRepository = (*executionArtifactRepository)(nil)
}

// --- Constructor ---

func TestNewExecutionArtifactRepository(t *testing.T) {
	repo := NewExecutionArtifactRepository(nil)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}

func TestNewExecutionArtifactRepository_ReturnsCorrectType(t *testing.T) {
	repo := NewExecutionArtifactRepository(nil)
	if _, ok := repo.(*executionArtifactRepository); !ok {
		t.Fatal("expected *executionArtifactRepository type")
	}
}

// --- Model helpers ---

func TestNewTestArtifact(t *testing.T) {
	execID := uuid.New().String()
	a := newTestArtifact(execID)
	if a.ExecutionID != execID {
		t.Errorf("expected execution_id %s, got %s", execID, a.ExecutionID)
	}
	if a.ID == "" {
		t.Error("expected non-empty ID")
	}
	if a.Label != "DB Dump" {
		t.Errorf("expected label 'DB Dump', got %s", a.Label)
	}
	if a.FileName != "dump.sql" {
		t.Errorf("expected file_name 'dump.sql', got %s", a.FileName)
	}
	if a.FileSizeBytes != 1024 {
		t.Errorf("expected file_size_bytes 1024, got %d", a.FileSizeBytes)
	}
	if a.ContentType != "application/sql" {
		t.Errorf("expected content_type 'application/sql', got %s", a.ContentType)
	}
	if a.StoragePath == "" {
		t.Error("expected non-empty StoragePath")
	}
	if a.StorageType != "local" {
		t.Errorf("expected storage_type 'local', got %s", a.StorageType)
	}
	if a.Status != "stored" {
		t.Errorf("expected status 'stored', got %s", a.Status)
	}
	if a.ErrorMessage != "" {
		t.Errorf("expected empty error_message, got %s", a.ErrorMessage)
	}
	if a.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestNewTestArtifact_UniqueIDs(t *testing.T) {
	execID := uuid.New().String()
	a1 := newTestArtifact(execID)
	a2 := newTestArtifact(execID)
	if a1.ID == a2.ID {
		t.Error("expected different IDs for different artifacts")
	}
	if a1.StoragePath == a2.StoragePath {
		t.Error("expected different StoragePaths for different artifacts")
	}
}

func TestNewTestArtifact_FailedStatus(t *testing.T) {
	a := newTestArtifact(uuid.New().String())
	a.Status = "failed"
	a.ErrorMessage = "file not found in container"
	if a.Status != "failed" {
		t.Errorf("expected status 'failed', got %s", a.Status)
	}
	if a.ErrorMessage == "" {
		t.Error("expected non-empty error_message for failed artifact")
	}
}

func TestNewTestArtifact_StorageTypes(t *testing.T) {
	tests := []struct {
		name        string
		storageType string
	}{
		{"local", "local"},
		{"minio", "minio"},
		{"s3", "s3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestArtifact(uuid.New().String())
			a.StorageType = tt.storageType
			if a.StorageType != tt.storageType {
				t.Errorf("expected storage_type %s, got %s", tt.storageType, a.StorageType)
			}
		})
	}
}
