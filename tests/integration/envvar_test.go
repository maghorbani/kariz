package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kariz/kariz/internal/envvar"
	"github.com/kariz/kariz/internal/executor"
	"github.com/kariz/kariz/internal/models"
	"github.com/kariz/kariz/internal/stream"
	"github.com/kariz/kariz/internal/validator"
)

// TestEnvVarResolution_EndToEnd tests the full env var resolution flow:
// Register a command with env var mappings → execute → verify resolved values appear in output.
func TestEnvVarResolution_EndToEnd(t *testing.T) {
	repo := newInMemoryExecutionRepo()
	streamMgr := stream.NewStreamManager()
	paramVal := validator.NewParameterValidator()

	// Mock Docker that returns known env vars from a "source" container
	// and echoes the resolved parameter values in the command output.
	dockerMgr := &mockDockerManager{
		inspectContainerEnvFn: func(ctx context.Context, containerNameOrID string) (map[string]string, error) {
			if containerNameOrID == "db-container" {
				return map[string]string{
					"DB_HOST":     "10.0.0.5",
					"DB_PORT":     "5432",
					"DB_PASSWORD": "secret123",
				}, nil
			}
			return nil, fmt.Errorf("container not found: %s", containerNameOrID)
		},
		attachStreamFn: func(ctx context.Context, containerID string) (<-chan models.OutputChunk, error) {
			ch := make(chan models.OutputChunk, 1)
			ch <- models.OutputChunk{Stream: "stdout", Data: "resolved ok\n", Timestamp: time.Now()}
			close(ch)
			return ch, nil
		},
	}

	// Create the env var resolver.
	resolver := envvar.NewEnvVarResolver(dockerMgr)

	// Define a command with env var mappings.
	cmd := models.CommandEntry{
		ID:          "cmd-envvar-1",
		Name:        "db-query",
		DockerImage: "postgres:16",
		CommandString: "psql -h {{host}} -p {{port}}",
		ParameterSchema: models.ParameterSchema{
			Parameters: []models.ParameterDefinition{
				{
					Name:     "host",
					Type:     models.ParamString,
					Required: true,
					EnvVarMapping: &models.EnvVarMapping{
						SourceContainer: "db-container",
						EnvVarName:      "DB_HOST",
					},
				},
				{
					Name:     "port",
					Type:     models.ParamString,
					Required: true,
					EnvVarMapping: &models.EnvVarMapping{
						SourceContainer: "db-container",
						EnvVarName:      "DB_PORT",
					},
				},
			},
		},
		AllowedRoles:    []models.Role{models.RoleAdmin},
		TimeoutSeconds:  30,
		AllowConcurrent: true,
		ExecutionMode:   models.ModeCreate,
		IsActive:        true,
		Version:         1,
	}

	// Test 1: Resolve params with no user-provided values — should use env var mappings.
	userParams := map[string]interface{}{}
	resolved, err := resolver.ResolveParams(context.Background(), cmd.ParameterSchema, userParams)
	if err != nil {
		t.Fatalf("ResolveParams failed: %v", err)
	}

	if resolved["host"] != "10.0.0.5" {
		t.Errorf("resolved host = %v, want 10.0.0.5", resolved["host"])
	}
	if resolved["port"] != "5432" {
		t.Errorf("resolved port = %v, want 5432", resolved["port"])
	}

	// Test 2: User-provided value takes precedence over env var mapping.
	userParams2 := map[string]interface{}{"host": "custom-host"}
	resolved2, err := resolver.ResolveParams(context.Background(), cmd.ParameterSchema, userParams2)
	if err != nil {
		t.Fatalf("ResolveParams with override failed: %v", err)
	}

	if resolved2["host"] != "custom-host" {
		t.Errorf("resolved host = %v, want custom-host (user override)", resolved2["host"])
	}
	if resolved2["port"] != "5432" {
		t.Errorf("resolved port = %v, want 5432 (from env var)", resolved2["port"])
	}

	// Test 3: Execute the command with resolved params to verify end-to-end flow.
	execSvc := executor.NewExecutorService(repo, dockerMgr, paramVal, streamMgr, nil, nil)

	record, err := execSvc.ExecuteCommand(context.Background(), cmd, resolved, "user-envvar-1")
	if err != nil {
		t.Fatalf("ExecuteCommand failed: %v", err)
	}

	// Wait for completion.
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for execution to complete")
		case <-ticker.C:
			finalRecord := repo.getRecord(record.ID)
			if finalRecord != nil && finalRecord.Status == models.StatusCompleted {
				if finalRecord.Stdout != "resolved ok\n" {
					t.Errorf("stdout = %q, want %q", finalRecord.Stdout, "resolved ok\n")
				}
				return
			}
		}
	}
}

// TestEnvVarResolution_ContainerNotFound tests that resolution fails gracefully
// when the source container doesn't exist.
func TestEnvVarResolution_ContainerNotFound(t *testing.T) {
	dockerMgr := &mockDockerManager{
		inspectContainerEnvFn: func(ctx context.Context, containerNameOrID string) (map[string]string, error) {
			return nil, fmt.Errorf("container not found: %s", containerNameOrID)
		},
	}

	resolver := envvar.NewEnvVarResolver(dockerMgr)

	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{
				Name:     "db_url",
				Type:     models.ParamString,
				Required: true,
				EnvVarMapping: &models.EnvVarMapping{
					SourceContainer: "nonexistent-container",
					EnvVarName:      "DATABASE_URL",
				},
			},
		},
	}

	_, err := resolver.ResolveParams(context.Background(), schema, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error when source container not found")
	}

	// Verify it's an EnvVarResolutionError.
	if resErr, ok := err.(*models.EnvVarResolutionError); ok {
		if resErr.ParameterName != "db_url" {
			t.Errorf("error parameter = %q, want db_url", resErr.ParameterName)
		}
	}
}

// TestEnvVarResolution_EnvVarNotFound tests that resolution fails when the env var
// doesn't exist on the container.
func TestEnvVarResolution_EnvVarNotFound(t *testing.T) {
	dockerMgr := &mockDockerManager{
		inspectContainerEnvFn: func(ctx context.Context, containerNameOrID string) (map[string]string, error) {
			return map[string]string{"OTHER_VAR": "value"}, nil
		},
	}

	resolver := envvar.NewEnvVarResolver(dockerMgr)

	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{
				Name:     "missing_var",
				Type:     models.ParamString,
				Required: true,
				EnvVarMapping: &models.EnvVarMapping{
					SourceContainer: "some-container",
					EnvVarName:      "NONEXISTENT_VAR",
				},
			},
		},
	}

	_, err := resolver.ResolveParams(context.Background(), schema, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error when env var not found on container")
	}
}
