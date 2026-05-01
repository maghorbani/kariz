package docker

import (
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/kariz/kariz/internal/models"
)

// ContainerCreateConfig holds the translated Docker API configuration
// produced from a models.ContainerConfig. This struct is used for testing
// the config translation logic without calling the Docker daemon.
type ContainerCreateConfig struct {
	Binds     []string
	Resources container.Resources
	Image     string
	Cmd       []string
	Env       []string
}

// BuildContainerCreateConfig translates a models.ContainerConfig into the
// Docker API structures used by ContainerCreate. It filters out Docker socket
// mounts and maps resource limits to Docker API fields.
func BuildContainerCreateConfig(config models.ContainerConfig) ContainerCreateConfig {
	// Build environment variables slice
	var envVars []string
	for k, v := range config.Environment {
		envVars = append(envVars, k+"="+v)
	}

	// Build bind mounts, filtering out Docker socket
	var binds []string
	for _, vol := range config.Volumes {
		if strings.Contains(vol.HostPath, dockerSocketPath) || strings.Contains(vol.ContainerPath, dockerSocketPath) {
			continue // Never mount Docker socket into sibling containers
		}
		bind := vol.HostPath + ":" + vol.ContainerPath
		if vol.ReadOnly {
			bind += ":ro"
		}
		binds = append(binds, bind)
	}

	// Build resource limits
	resources := container.Resources{}
	if config.ResourceLimits.CPUShares != nil {
		resources.CPUShares = *config.ResourceLimits.CPUShares
	}
	if config.ResourceLimits.MemoryMB != nil {
		resources.Memory = *config.ResourceLimits.MemoryMB * 1024 * 1024 // Convert MB to bytes
	}
	if config.ResourceLimits.CPUCount != nil {
		resources.NanoCPUs = *config.ResourceLimits.CPUCount * 1e9 // Convert CPU count to NanoCPUs
	}

	return ContainerCreateConfig{
		Binds:     binds,
		Resources: resources,
		Image:     config.Image,
		Cmd:       config.Command,
		Env:       envVars,
	}
}
