package envvar

import (
	"context"
	"fmt"

	"github.com/kariz/kariz/internal/docker"
	"github.com/kariz/kariz/internal/models"
)

// EnvVarResolver resolves parameter values from environment variables of running
// sibling containers at execution time.
type EnvVarResolver interface {
	// ResolveParams takes a parameter schema and user-provided params, resolves any
	// EnvVarMappings by reading env vars from running containers, and returns the
	// fully resolved parameter map. User-provided values take precedence over mappings.
	ResolveParams(ctx context.Context, schema models.ParameterSchema, userParams map[string]interface{}) (map[string]interface{}, error)
}

// envVarResolver is the concrete implementation of EnvVarResolver.
type envVarResolver struct {
	dockerMgr docker.DockerManager
}

// NewEnvVarResolver creates a new EnvVarResolver with the given DockerManager dependency.
func NewEnvVarResolver(dockerMgr docker.DockerManager) EnvVarResolver {
	return &envVarResolver{
		dockerMgr: dockerMgr,
	}
}

// ResolveParams iterates over the parameter schema. For each parameter with an
// EnvVarMapping and no user-provided value, it calls DockerManager.InspectContainerEnv
// to read the env var from the source container. User-provided values always take
// precedence over mapped values.
func (r *envVarResolver) ResolveParams(ctx context.Context, schema models.ParameterSchema, userParams map[string]interface{}) (map[string]interface{}, error) {
	resolved := make(map[string]interface{})

	// Copy all user-provided values first.
	for k, v := range userParams {
		resolved[k] = v
	}

	for _, param := range schema.Parameters {
		// Skip parameters without an env var mapping.
		if param.EnvVarMapping == nil {
			continue
		}

		// If the user already provided a value for this parameter, use it (precedence rule).
		if _, hasUserValue := userParams[param.Name]; hasUserValue {
			continue
		}

		// Resolve the env var from the source container.
		envMap, err := r.dockerMgr.InspectContainerEnv(ctx, param.EnvVarMapping.SourceContainer)
		if err != nil {
			return nil, &models.EnvVarResolutionError{
				ParameterName:   param.Name,
				SourceContainer: param.EnvVarMapping.SourceContainer,
				EnvVarName:      param.EnvVarMapping.EnvVarName,
				Reason:          fmt.Sprintf("failed to inspect container: %v", err),
			}
		}

		value, found := envMap[param.EnvVarMapping.EnvVarName]
		if !found {
			return nil, &models.EnvVarResolutionError{
				ParameterName:   param.Name,
				SourceContainer: param.EnvVarMapping.SourceContainer,
				EnvVarName:      param.EnvVarMapping.EnvVarName,
				Reason:          "env var not found on container",
			}
		}

		resolved[param.Name] = value
	}

	return resolved, nil
}
