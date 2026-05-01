//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kariz/kariz/internal/docker"
	"github.com/kariz/kariz/internal/models"
)

// getDockerSocketPath returns the Docker socket path from the environment
// or the default /var/run/docker.sock.
func getDockerSocketPath() string {
	if p := os.Getenv("DOCKER_SOCKET_PATH"); p != "" {
		return p
	}
	return "/var/run/docker.sock"
}

// skipIfDockerUnavailable skips the test if the Docker daemon is not reachable.
func skipIfDockerUnavailable(t *testing.T) docker.DockerManager {
	t.Helper()
	socketPath := getDockerSocketPath()

	mgr, err := docker.NewDockerManager(socketPath)
	if err != nil {
		t.Skipf("Docker not available (cannot create client): %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mgr.IsAvailable(ctx); err != nil {
		t.Skipf("Docker daemon not reachable: %v", err)
	}

	return mgr
}

// TestDockerContainerLifecycle tests the full container lifecycle:
// CreateContainer → StartContainer → AttachStream → read output → StopContainer → RemoveContainer.
// Requires a running Docker daemon (build tag: integration).
func TestDockerContainerLifecycle(t *testing.T) {
	mgr := skipIfDockerUnavailable(t)
	ctx := context.Background()

	// 1. Create a container that echoes output.
	config := models.ContainerConfig{
		Image:   "alpine:latest",
		Command: []string{"sh", "-c", "echo 'hello from alpine' && echo 'error line' >&2"},
	}

	containerID, err := mgr.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}
	t.Logf("Created container: %s", containerID)

	// Ensure cleanup.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = mgr.RemoveContainer(cleanupCtx, containerID)
	}()

	// 2. Attach stream before starting (to capture all output).
	outputCh, err := mgr.AttachStream(ctx, containerID)
	if err != nil {
		t.Fatalf("AttachStream failed: %v", err)
	}

	// 3. Start the container.
	if err := mgr.StartContainer(ctx, containerID); err != nil {
		t.Fatalf("StartContainer failed: %v", err)
	}

	// 4. Read output chunks.
	var stdout, stderr string
	done := make(chan struct{})
	go func() {
		defer close(done)
		for chunk := range outputCh {
			switch chunk.Stream {
			case "stdout":
				stdout += chunk.Data
			case "stderr":
				stderr += chunk.Data
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for container output")
	}

	// 5. Verify output.
	if stdout == "" {
		t.Error("expected stdout output from container")
	}
	t.Logf("stdout: %q", stdout)
	t.Logf("stderr: %q", stderr)

	// 6. Stop the container (it may have already exited).
	stopCtx, stopCancel := context.WithTimeout(ctx, 10*time.Second)
	defer stopCancel()
	// Ignore error since container may have already stopped.
	_ = mgr.StopContainer(stopCtx, containerID, 5)

	// 7. Remove the container.
	if err := mgr.RemoveContainer(ctx, containerID); err != nil {
		t.Fatalf("RemoveContainer failed: %v", err)
	}
	t.Log("Container removed successfully")
}

// TestDockerContainerLifecycle_WithResourceLimits tests container creation with
// resource limits applied.
func TestDockerContainerLifecycle_WithResourceLimits(t *testing.T) {
	mgr := skipIfDockerUnavailable(t)
	ctx := context.Background()

	memMB := int64(64)
	cpuShares := int64(512)

	config := models.ContainerConfig{
		Image:   "alpine:latest",
		Command: []string{"echo", "resource-limited"},
		ResourceLimits: models.ResourceLimits{
			MemoryMB:  &memMB,
			CPUShares: &cpuShares,
		},
	}

	containerID, err := mgr.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("CreateContainer with resource limits failed: %v", err)
	}

	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = mgr.RemoveContainer(cleanupCtx, containerID)
	}()

	if err := mgr.StartContainer(ctx, containerID); err != nil {
		t.Fatalf("StartContainer failed: %v", err)
	}

	// Wait for container to finish.
	time.Sleep(2 * time.Second)

	// Cleanup.
	if err := mgr.RemoveContainer(ctx, containerID); err != nil {
		t.Fatalf("RemoveContainer failed: %v", err)
	}
}
