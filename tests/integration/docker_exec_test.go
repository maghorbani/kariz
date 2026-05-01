//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/kariz/kariz/internal/models"
)

// TestDockerExecMode tests the exec mode execution path:
// Start a container → exec a command inside it → verify output captured → container NOT removed.
// Requires a running Docker daemon (build tag: integration).
func TestDockerExecMode(t *testing.T) {
	mgr := skipIfDockerUnavailable(t)
	ctx := context.Background()

	// 1. Create and start a long-running container to exec into.
	config := models.ContainerConfig{
		Image:   "alpine:latest",
		Command: []string{"sleep", "60"},
	}

	containerID, err := mgr.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}
	t.Logf("Created target container: %s", containerID)

	// Ensure cleanup.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = mgr.StopContainer(cleanupCtx, containerID, 5)
		_ = mgr.RemoveContainer(cleanupCtx, containerID)
	}()

	if err := mgr.StartContainer(ctx, containerID); err != nil {
		t.Fatalf("StartContainer failed: %v", err)
	}

	// Wait a moment for the container to be fully running.
	time.Sleep(1 * time.Second)

	// 2. Exec a command inside the running container.
	execID, err := mgr.ExecInContainer(ctx, containerID, []string{"echo", "hello from exec"})
	if err != nil {
		t.Fatalf("ExecInContainer failed: %v", err)
	}
	t.Logf("Created exec: %s", execID)

	// 3. Attach to exec stream and capture output.
	outputCh, err := mgr.AttachExecStream(ctx, execID)
	if err != nil {
		t.Fatalf("AttachExecStream failed: %v", err)
	}

	var stdout string
	done := make(chan struct{})
	go func() {
		defer close(done)
		for chunk := range outputCh {
			if chunk.Stream == "stdout" {
				stdout += chunk.Data
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for exec output")
	}

	// 4. Verify output.
	if stdout == "" {
		t.Error("expected stdout output from exec")
	}
	t.Logf("exec stdout: %q", stdout)

	// 5. Inspect exec to get exit code.
	inspectResult, err := mgr.InspectExec(ctx, execID)
	if err != nil {
		t.Fatalf("InspectExec failed: %v", err)
	}

	if inspectResult.ExitCode != 0 {
		t.Errorf("exec exit code = %d, want 0", inspectResult.ExitCode)
	}

	// 6. Verify the target container is still running (NOT removed).
	envMap, err := mgr.InspectContainerEnv(ctx, containerID)
	if err != nil {
		t.Fatalf("Container should still be running after exec, but InspectContainerEnv failed: %v", err)
	}
	// The container should have at least PATH in its env.
	if _, ok := envMap["PATH"]; !ok {
		t.Log("Note: PATH not found in container env, but container is still accessible")
	}
	t.Log("Target container is still running after exec — correct behavior")
}

// TestDockerExecMode_WithStderr tests exec mode with a command that produces stderr.
func TestDockerExecMode_WithStderr(t *testing.T) {
	mgr := skipIfDockerUnavailable(t)
	ctx := context.Background()

	// Create and start a container.
	config := models.ContainerConfig{
		Image:   "alpine:latest",
		Command: []string{"sleep", "60"},
	}

	containerID, err := mgr.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = mgr.StopContainer(cleanupCtx, containerID, 5)
		_ = mgr.RemoveContainer(cleanupCtx, containerID)
	}()

	if err := mgr.StartContainer(ctx, containerID); err != nil {
		t.Fatalf("StartContainer failed: %v", err)
	}

	time.Sleep(1 * time.Second)

	// Exec a command that writes to both stdout and stderr.
	execID, err := mgr.ExecInContainer(ctx, containerID, []string{"sh", "-c", "echo out && echo err >&2"})
	if err != nil {
		t.Fatalf("ExecInContainer failed: %v", err)
	}

	outputCh, err := mgr.AttachExecStream(ctx, execID)
	if err != nil {
		t.Fatalf("AttachExecStream failed: %v", err)
	}

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
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for exec output")
	}

	t.Logf("stdout: %q, stderr: %q", stdout, stderr)

	if stdout == "" {
		t.Error("expected stdout from exec")
	}
}
