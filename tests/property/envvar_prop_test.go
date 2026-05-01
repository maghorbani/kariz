package property_test

// Feature: kariz-command-dashboard, Property 17: environment variable resolution precedence

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/kariz/kariz/internal/envvar"
	"github.com/kariz/kariz/internal/models"
	"pgregory.net/rapid"
)

// --- Mock DockerManager for Property 17 ---

type prop17MockDockerManager struct {
	// containerEnvs maps container name to its environment variables.
	containerEnvs map[string]map[string]string
}

func newProp17MockDockerManager() *prop17MockDockerManager {
	return &prop17MockDockerManager{
		containerEnvs: make(map[string]map[string]string),
	}
}

func (m *prop17MockDockerManager) CreateContainer(_ context.Context, _ models.ContainerConfig) (string, error) {
	return "", nil
}
func (m *prop17MockDockerManager) StartContainer(_ context.Context, _ string) error { return nil }
func (m *prop17MockDockerManager) AttachStream(_ context.Context, _ string) (<-chan models.OutputChunk, error) {
	return nil, nil
}
func (m *prop17MockDockerManager) StopContainer(_ context.Context, _ string, _ int) error { return nil }
func (m *prop17MockDockerManager) RemoveContainer(_ context.Context, _ string) error      { return nil }
func (m *prop17MockDockerManager) IsAvailable(_ context.Context) error                    { return nil }
func (m *prop17MockDockerManager) ExecInContainer(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}
func (m *prop17MockDockerManager) AttachExecStream(_ context.Context, _ string) (<-chan models.OutputChunk, error) {
	return nil, nil
}
func (m *prop17MockDockerManager) InspectExec(_ context.Context, _ string) (*models.ExecInspectResult, error) {
	return nil, nil
}
func (m *prop17MockDockerManager) CopyFromContainer(_ context.Context, _ string, _ string) (io.ReadCloser, error) {
	return nil, nil
}

func (m *prop17MockDockerManager) InspectContainerEnv(_ context.Context, containerNameOrID string) (map[string]string, error) {
	envs, ok := m.containerEnvs[containerNameOrID]
	if !ok {
		return nil, fmt.Errorf("container %q not found or not running", containerNameOrID)
	}
	return envs, nil
}

// --- Generators ---

// genP17ParamName generates a valid parameter name.
func genP17ParamName(t *rapid.T, label string) string {
	return rapid.StringMatching(`^[a-z][a-z0-9_]{2,15}`).Draw(t, label)
}

// genP17ContainerName generates a valid container name.
func genP17ContainerName(t *rapid.T, label string) string {
	return rapid.StringMatching(`^[a-z][a-z0-9-]{2,15}`).Draw(t, label)
}

// genP17EnvVarName generates a valid environment variable name.
func genP17EnvVarName(t *rapid.T, label string) string {
	return rapid.StringMatching(`^[A-Z][A-Z0-9_]{2,15}`).Draw(t, label)
}

// genP17EnvVarValue generates a non-empty string value for an env var.
func genP17EnvVarValue(t *rapid.T, label string) string {
	return rapid.StringMatching(`^[a-zA-Z0-9._/-]{1,30}`).Draw(t, label)
}

// --- Property 17 Tests ---
// **Validates: Requirements 11.2, 11.5**

// TestProperty17_UserProvidedValueTakesPrecedence tests that for any parameter with
// an EnvVarMapping where the user also provides a value, ResolveParams returns the
// user-provided value.
func TestProperty17_UserProvidedValueTakesPrecedence(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random parameter, container, and env var names.
		paramName := genP17ParamName(t, "paramName")
		containerName := genP17ContainerName(t, "containerName")
		envVarName := genP17EnvVarName(t, "envVarName")
		containerEnvValue := genP17EnvVarValue(t, "containerEnvValue")
		userValue := genP17EnvVarValue(t, "userValue")

		// Set up mock with the container env var.
		mock := newProp17MockDockerManager()
		mock.containerEnvs[containerName] = map[string]string{
			envVarName: containerEnvValue,
		}

		resolver := envvar.NewEnvVarResolver(mock)

		schema := models.ParameterSchema{
			Parameters: []models.ParameterDefinition{
				{
					Name: paramName,
					Type: models.ParamString,
					EnvVarMapping: &models.EnvVarMapping{
						SourceContainer: containerName,
						EnvVarName:      envVarName,
					},
				},
			},
		}

		// User provides an explicit value.
		userParams := map[string]interface{}{
			paramName: userValue,
		}

		result, err := resolver.ResolveParams(context.Background(), schema, userParams)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// The user-provided value must take precedence.
		got, ok := result[paramName]
		if !ok {
			t.Fatalf("expected parameter %q in result", paramName)
		}
		if got != userValue {
			t.Fatalf("expected user-provided value %q, got %v", userValue, got)
		}
	})
}

// TestProperty17_EnvVarMappingResolvesWhenNoUserValue tests that for any parameter
// with an EnvVarMapping where the user does NOT provide a value, ResolveParams
// returns the value from the container's environment variable.
func TestProperty17_EnvVarMappingResolvesWhenNoUserValue(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		paramName := genP17ParamName(t, "paramName")
		containerName := genP17ContainerName(t, "containerName")
		envVarName := genP17EnvVarName(t, "envVarName")
		containerEnvValue := genP17EnvVarValue(t, "containerEnvValue")

		mock := newProp17MockDockerManager()
		mock.containerEnvs[containerName] = map[string]string{
			envVarName: containerEnvValue,
		}

		resolver := envvar.NewEnvVarResolver(mock)

		schema := models.ParameterSchema{
			Parameters: []models.ParameterDefinition{
				{
					Name: paramName,
					Type: models.ParamString,
					EnvVarMapping: &models.EnvVarMapping{
						SourceContainer: containerName,
						EnvVarName:      envVarName,
					},
				},
			},
		}

		// User does NOT provide a value.
		userParams := map[string]interface{}{}

		result, err := resolver.ResolveParams(context.Background(), schema, userParams)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// The resolved value must come from the container env var.
		got, ok := result[paramName]
		if !ok {
			t.Fatalf("expected parameter %q in result", paramName)
		}
		if got != containerEnvValue {
			t.Fatalf("expected container env value %q, got %v", containerEnvValue, got)
		}
	})
}

// TestProperty17_MixedPrecedence tests that in a schema with multiple parameters,
// some with user values and some without, the precedence rule holds for each.
func TestProperty17_MixedPrecedence(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a number of mapped parameters (1-5).
		numParams := rapid.IntRange(1, 5).Draw(t, "numParams")

		mock := newProp17MockDockerManager()
		var params []models.ParameterDefinition
		userParams := make(map[string]interface{})
		expectedValues := make(map[string]string)

		for i := 0; i < numParams; i++ {
			paramName := fmt.Sprintf("param_%d", i)
			containerName := fmt.Sprintf("container_%d", i)
			envVarName := fmt.Sprintf("ENV_%d", i)
			containerEnvValue := genP17EnvVarValue(t, fmt.Sprintf("containerVal_%d", i))

			mock.containerEnvs[containerName] = map[string]string{
				envVarName: containerEnvValue,
			}

			params = append(params, models.ParameterDefinition{
				Name: paramName,
				Type: models.ParamString,
				EnvVarMapping: &models.EnvVarMapping{
					SourceContainer: containerName,
					EnvVarName:      envVarName,
				},
			})

			// Randomly decide if user provides a value.
			userProvides := rapid.Bool().Draw(t, fmt.Sprintf("userProvides_%d", i))
			if userProvides {
				userVal := genP17EnvVarValue(t, fmt.Sprintf("userVal_%d", i))
				userParams[paramName] = userVal
				expectedValues[paramName] = userVal
			} else {
				expectedValues[paramName] = containerEnvValue
			}
		}

		resolver := envvar.NewEnvVarResolver(mock)
		schema := models.ParameterSchema{Parameters: params}

		result, err := resolver.ResolveParams(context.Background(), schema, userParams)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify each parameter has the expected value.
		for paramName, expected := range expectedValues {
			got, ok := result[paramName]
			if !ok {
				t.Fatalf("expected parameter %q in result", paramName)
			}
			if got != expected {
				_, isUserProvided := userParams[paramName]
				t.Fatalf("parameter %q: expected %q, got %v (user_provided=%v)", paramName, expected, got, isUserProvided)
			}
		}
	})
}
