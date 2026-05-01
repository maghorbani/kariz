package envvar

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/kariz/kariz/internal/models"
)

// --- Mock DockerManager for envvar tests ---

type mockDockerManager struct {
	// containerEnvs maps container name/ID to its environment variables.
	containerEnvs map[string]map[string]string
	// inspectErr simulates an error when inspecting a container.
	inspectErr error
}

func newMockDockerManager() *mockDockerManager {
	return &mockDockerManager{
		containerEnvs: make(map[string]map[string]string),
	}
}

func (m *mockDockerManager) CreateContainer(_ context.Context, _ models.ContainerConfig) (string, error) {
	return "", nil
}
func (m *mockDockerManager) StartContainer(_ context.Context, _ string) error { return nil }
func (m *mockDockerManager) AttachStream(_ context.Context, _ string) (<-chan models.OutputChunk, error) {
	return nil, nil
}
func (m *mockDockerManager) StopContainer(_ context.Context, _ string, _ int) error { return nil }
func (m *mockDockerManager) RemoveContainer(_ context.Context, _ string) error      { return nil }
func (m *mockDockerManager) IsAvailable(_ context.Context) error                    { return nil }
func (m *mockDockerManager) ExecInContainer(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}
func (m *mockDockerManager) AttachExecStream(_ context.Context, _ string) (<-chan models.OutputChunk, error) {
	return nil, nil
}
func (m *mockDockerManager) InspectExec(_ context.Context, _ string) (*models.ExecInspectResult, error) {
	return nil, nil
}
func (m *mockDockerManager) CopyFromContainer(_ context.Context, _ string, _ string) (io.ReadCloser, error) {
	return nil, nil
}

func (m *mockDockerManager) InspectContainerEnv(_ context.Context, containerNameOrID string) (map[string]string, error) {
	if m.inspectErr != nil {
		return nil, m.inspectErr
	}
	envs, ok := m.containerEnvs[containerNameOrID]
	if !ok {
		return nil, fmt.Errorf("container %q not found", containerNameOrID)
	}
	return envs, nil
}

// --- Unit Tests ---

func TestResolveParams_UserProvidedValueTakesPrecedence(t *testing.T) {
	mock := newMockDockerManager()
	mock.containerEnvs["my-db"] = map[string]string{
		"DB_HOST": "container-host",
	}

	resolver := NewEnvVarResolver(mock)

	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{
				Name: "db_host",
				Type: models.ParamString,
				EnvVarMapping: &models.EnvVarMapping{
					SourceContainer: "my-db",
					EnvVarName:      "DB_HOST",
				},
			},
		},
	}

	userParams := map[string]interface{}{
		"db_host": "user-provided-host",
	}

	result, err := resolver.ResolveParams(context.Background(), schema, userParams)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["db_host"] != "user-provided-host" {
		t.Fatalf("expected user-provided value 'user-provided-host', got %v", result["db_host"])
	}
}

func TestResolveParams_EnvVarMappingResolvesWhenNoUserValue(t *testing.T) {
	mock := newMockDockerManager()
	mock.containerEnvs["my-db"] = map[string]string{
		"DB_HOST": "container-host",
	}

	resolver := NewEnvVarResolver(mock)

	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{
				Name: "db_host",
				Type: models.ParamString,
				EnvVarMapping: &models.EnvVarMapping{
					SourceContainer: "my-db",
					EnvVarName:      "DB_HOST",
				},
			},
		},
	}

	userParams := map[string]interface{}{}

	result, err := resolver.ResolveParams(context.Background(), schema, userParams)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["db_host"] != "container-host" {
		t.Fatalf("expected resolved value 'container-host', got %v", result["db_host"])
	}
}

func TestResolveParams_ErrorWhenContainerNotFound(t *testing.T) {
	mock := newMockDockerManager()
	// No containers registered — inspecting will fail.

	resolver := NewEnvVarResolver(mock)

	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{
				Name: "db_host",
				Type: models.ParamString,
				EnvVarMapping: &models.EnvVarMapping{
					SourceContainer: "nonexistent-container",
					EnvVarName:      "DB_HOST",
				},
			},
		},
	}

	userParams := map[string]interface{}{}

	_, err := resolver.ResolveParams(context.Background(), schema, userParams)
	if err == nil {
		t.Fatal("expected error when container not found, got nil")
	}

	resErr, ok := err.(*models.EnvVarResolutionError)
	if !ok {
		t.Fatalf("expected EnvVarResolutionError, got %T: %v", err, err)
	}

	if resErr.ParameterName != "db_host" {
		t.Fatalf("expected parameter name 'db_host', got %q", resErr.ParameterName)
	}
	if resErr.SourceContainer != "nonexistent-container" {
		t.Fatalf("expected source container 'nonexistent-container', got %q", resErr.SourceContainer)
	}
}

func TestResolveParams_ErrorWhenEnvVarNotFound(t *testing.T) {
	mock := newMockDockerManager()
	mock.containerEnvs["my-db"] = map[string]string{
		"OTHER_VAR": "some-value",
	}

	resolver := NewEnvVarResolver(mock)

	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{
				Name: "db_host",
				Type: models.ParamString,
				EnvVarMapping: &models.EnvVarMapping{
					SourceContainer: "my-db",
					EnvVarName:      "DB_HOST",
				},
			},
		},
	}

	userParams := map[string]interface{}{}

	_, err := resolver.ResolveParams(context.Background(), schema, userParams)
	if err == nil {
		t.Fatal("expected error when env var not found, got nil")
	}

	resErr, ok := err.(*models.EnvVarResolutionError)
	if !ok {
		t.Fatalf("expected EnvVarResolutionError, got %T: %v", err, err)
	}

	if resErr.EnvVarName != "DB_HOST" {
		t.Fatalf("expected env var name 'DB_HOST', got %q", resErr.EnvVarName)
	}
	if resErr.Reason != "env var not found on container" {
		t.Fatalf("expected reason 'env var not found on container', got %q", resErr.Reason)
	}
}

func TestResolveParams_MixedParameters(t *testing.T) {
	mock := newMockDockerManager()
	mock.containerEnvs["my-db"] = map[string]string{
		"DB_HOST": "container-host",
		"DB_PORT": "5432",
	}

	resolver := NewEnvVarResolver(mock)

	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{
				Name: "db_host",
				Type: models.ParamString,
				EnvVarMapping: &models.EnvVarMapping{
					SourceContainer: "my-db",
					EnvVarName:      "DB_HOST",
				},
			},
			{
				Name: "db_port",
				Type: models.ParamString,
				EnvVarMapping: &models.EnvVarMapping{
					SourceContainer: "my-db",
					EnvVarName:      "DB_PORT",
				},
			},
			{
				// Parameter without env var mapping — should be passed through from user params.
				Name: "query",
				Type: models.ParamString,
			},
			{
				// Parameter without env var mapping and no user value — should not appear.
				Name: "optional_flag",
				Type: models.ParamBoolean,
			},
		},
	}

	userParams := map[string]interface{}{
		"db_host": "user-host", // User overrides the mapped value.
		"query":   "SELECT 1",  // No mapping, user-provided.
	}

	result, err := resolver.ResolveParams(context.Background(), schema, userParams)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// db_host: user-provided value takes precedence.
	if result["db_host"] != "user-host" {
		t.Fatalf("expected 'user-host' for db_host, got %v", result["db_host"])
	}

	// db_port: resolved from container env var.
	if result["db_port"] != "5432" {
		t.Fatalf("expected '5432' for db_port, got %v", result["db_port"])
	}

	// query: passed through from user params.
	if result["query"] != "SELECT 1" {
		t.Fatalf("expected 'SELECT 1' for query, got %v", result["query"])
	}

	// optional_flag: no mapping, no user value — should not be in result.
	if _, exists := result["optional_flag"]; exists {
		t.Fatal("expected optional_flag to not be in result")
	}
}
