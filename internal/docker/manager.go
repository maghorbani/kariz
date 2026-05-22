package docker

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/kariz/kariz/internal/models"
)

// DockerManager abstracts Docker daemon communication for container and exec lifecycle.
type DockerManager interface {
	CreateContainer(ctx context.Context, config models.ContainerConfig) (containerID string, err error)
	StartContainer(ctx context.Context, containerID string) error
	AttachStream(ctx context.Context, containerID string) (<-chan models.OutputChunk, error)
	StopContainer(ctx context.Context, containerID string, timeout int) error
	RemoveContainer(ctx context.Context, containerID string) error
	IsAvailable(ctx context.Context) error
	ExecInContainer(ctx context.Context, containerID string, command []string) (execID string, err error)
	AttachExecStream(ctx context.Context, execID string) (<-chan models.OutputChunk, error)
	InspectExec(ctx context.Context, execID string) (*models.ExecInspectResult, error)
	InspectContainerEnv(ctx context.Context, containerNameOrID string) (map[string]string, error)
	CopyFromContainer(ctx context.Context, containerID string, srcPath string) (io.ReadCloser, error)
	ContainerLogs(ctx context.Context, containerID string, opts models.LogOptions) (<-chan models.OutputChunk, error)
}

// dockerManager is the concrete implementation of DockerManager using the Docker SDK.
type dockerManager struct {
	cli *client.Client
}

// NewDockerManager creates a new DockerManager connected to the Docker daemon via the given socket path.
func NewDockerManager(socketPath string) (DockerManager, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost("unix://"+socketPath),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
	return &dockerManager{cli: cli}, nil
}

// dockerSocketPath is the path that must never be mounted into sibling containers.
const dockerSocketPath = "/var/run/docker.sock"

// CreateContainer translates a ContainerConfig into a Docker API create request.
// It maps volume mounts to bind mounts (rejecting any Docker socket mount),
// applies resource limits, and returns the created container ID.
func (dm *dockerManager) CreateContainer(ctx context.Context, config models.ContainerConfig) (string, error) {
	built := BuildContainerCreateConfig(config)

	containerConfig := &container.Config{
		Image: built.Image,
		Cmd:   built.Cmd,
		Env:   built.Env,
	}

	hostConfig := &container.HostConfig{
		Binds:     built.Binds,
		Resources: built.Resources,
	}

	resp, err := dm.cli.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	return resp.ID, nil
}

// StartContainer starts a previously created container.
func (dm *dockerManager) StartContainer(ctx context.Context, containerID string) error {
	if err := dm.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container %s: %w", containerID, err)
	}
	return nil
}

// AttachStream attaches to a running container's stdout and stderr and returns a channel
// of OutputChunk. The Docker multiplexed stream protocol is used: each frame has an 8-byte
// header where byte 0 is the stream type (1=stdout, 2=stderr) and bytes 4-7 are the
// payload size in big-endian.
func (dm *dockerManager) AttachStream(ctx context.Context, containerID string) (<-chan models.OutputChunk, error) {
	resp, err := dm.cli.ContainerAttach(ctx, containerID, container.AttachOptions{
		Stream: true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to attach to container %s: %w", containerID, err)
	}

	ch := make(chan models.OutputChunk, 64)
	go demuxStream(ctx, resp.Reader, ch)
	return ch, nil
}

// StopContainer gracefully stops a container with the given timeout in seconds.
func (dm *dockerManager) StopContainer(ctx context.Context, containerID string, timeout int) error {
	stopOptions := container.StopOptions{}
	if timeout > 0 {
		stopOptions.Timeout = &timeout
	}
	if err := dm.cli.ContainerStop(ctx, containerID, stopOptions); err != nil {
		return fmt.Errorf("failed to stop container %s: %w", containerID, err)
	}
	return nil
}

// RemoveContainer force-removes a container.
func (dm *dockerManager) RemoveContainer(ctx context.Context, containerID string) error {
	if err := dm.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("failed to remove container %s: %w", containerID, err)
	}
	return nil
}

// IsAvailable pings the Docker daemon and returns a descriptive error if unreachable.
func (dm *dockerManager) IsAvailable(ctx context.Context) error {
	_, err := dm.cli.Ping(ctx)
	if err != nil {
		return fmt.Errorf("docker daemon is unreachable: %w", err)
	}
	return nil
}

// ExecInContainer creates an exec instance in a running container and returns the exec ID.
func (dm *dockerManager) ExecInContainer(ctx context.Context, containerID string, command []string) (string, error) {
	execConfig := container.ExecOptions{
		Cmd:          command,
		AttachStdout: true,
		AttachStderr: true,
	}

	resp, err := dm.cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create exec in container %s: %w", containerID, err)
	}

	return resp.ID, nil
}

// AttachExecStream attaches to an exec instance's stdout/stderr and returns a channel
// of OutputChunk. It starts the exec instance and demuxes the multiplexed stream.
func (dm *dockerManager) AttachExecStream(ctx context.Context, execID string) (<-chan models.OutputChunk, error) {
	resp, err := dm.cli.ContainerExecAttach(ctx, execID, container.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to attach to exec %s: %w", execID, err)
	}

	ch := make(chan models.OutputChunk, 64)
	go demuxStream(ctx, resp.Reader, ch)
	return ch, nil
}

// InspectExec inspects an exec instance to get its exit code and running status.
func (dm *dockerManager) InspectExec(ctx context.Context, execID string) (*models.ExecInspectResult, error) {
	resp, err := dm.cli.ContainerExecInspect(ctx, execID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect exec %s: %w", execID, err)
	}

	return &models.ExecInspectResult{
		ExitCode: resp.ExitCode,
		Running:  resp.Running,
	}, nil
}

// InspectContainerEnv inspects a container and parses its environment variables
// into a map of key-value pairs.
func (dm *dockerManager) InspectContainerEnv(ctx context.Context, containerNameOrID string) (map[string]string, error) {
	info, err := dm.cli.ContainerInspect(ctx, containerNameOrID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container %s: %w", containerNameOrID, err)
	}

	envMap := make(map[string]string)
	for _, envEntry := range info.Config.Env {
		parts := strings.SplitN(envEntry, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	return envMap, nil
}

// ContainerLogs streams stdout/stderr from a container using the Docker logs API.
func (dm *dockerManager) ContainerLogs(ctx context.Context, containerID string, opts models.LogOptions) (<-chan models.OutputChunk, error) {
	tail := "all"
	if opts.TailLines > 0 {
		tail = strconv.Itoa(opts.TailLines)
	}

	reader, err := dm.cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     opts.Follow,
		Tail:       tail,
		Timestamps: opts.Timestamps,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get logs for container %s: %w", containerID, err)
	}

	ch := make(chan models.OutputChunk, 64)
	go func() {
		defer reader.Close()
		demuxStream(ctx, reader, ch)
	}()
	return ch, nil
}

// CopyFromContainer copies a file or directory from a container and returns an
// io.ReadCloser of the tar archive content.
func (dm *dockerManager) CopyFromContainer(ctx context.Context, containerID string, srcPath string) (io.ReadCloser, error) {
	reader, _, err := dm.cli.CopyFromContainer(ctx, containerID, srcPath)
	if err != nil {
		return nil, fmt.Errorf("failed to copy from container %s path %s: %w", containerID, srcPath, err)
	}
	return reader, nil
}

// demuxStream reads the Docker multiplexed stream protocol and sends OutputChunks
// to the provided channel. The 8-byte header format is:
//   - byte 0: stream type (1 = stdout, 2 = stderr)
//   - bytes 1-3: reserved (padding)
//   - bytes 4-7: payload size (big-endian uint32)
//
// The channel is closed when the stream ends or the context is cancelled.
func demuxStream(ctx context.Context, reader io.Reader, ch chan<- models.OutputChunk) {
	defer close(ch)

	header := make([]byte, 8)
	for {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Read the 8-byte header
		_, err := io.ReadFull(reader, header)
		if err != nil {
			return // EOF or error — stream ended
		}

		// Parse stream type from byte 0
		streamType := header[0]
		var streamName string
		switch streamType {
		case 1:
			streamName = "stdout"
		case 2:
			streamName = "stderr"
		default:
			streamName = "stdout" // Default to stdout for unknown stream types
		}

		// Parse payload size from bytes 4-7 (big-endian)
		payloadSize := binary.BigEndian.Uint32(header[4:8])
		if payloadSize == 0 {
			continue
		}

		// Read the payload
		payload := make([]byte, payloadSize)
		_, err = io.ReadFull(reader, payload)
		if err != nil {
			return // Incomplete read — stream ended
		}

		chunk := models.OutputChunk{
			Stream:    streamName,
			Data:      string(payload),
			Timestamp: time.Now(),
		}

		select {
		case ch <- chunk:
		case <-ctx.Done():
			return
		}
	}
}
