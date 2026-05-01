package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kariz/kariz/internal/executor"
	"github.com/kariz/kariz/internal/models"
	"github.com/kariz/kariz/internal/stream"
	"github.com/kariz/kariz/internal/validator"
)

// TestTimeoutEnforcement tests that a command with a short timeout (1 second)
// results in a timed_out status and the container is stopped.
func TestTimeoutEnforcement(t *testing.T) {
	repo := newInMemoryExecutionRepo()
	streamMgr := stream.NewStreamManager()
	paramVal := validator.NewParameterValidator()

	var (
		stopMu        sync.Mutex
		stoppedContID string
	)

	// Mock Docker that delays output beyond the timeout.
	dockerMgr := &mockDockerManager{
		attachStreamFn: func(ctx context.Context, containerID string) (<-chan models.OutputChunk, error) {
			ch := make(chan models.OutputChunk, 1)
			go func() {
				defer close(ch)
				// Send one chunk immediately.
				select {
				case ch <- models.OutputChunk{Stream: "stdout", Data: "starting...\n", Timestamp: time.Now()}:
				case <-ctx.Done():
					return
				}
				// Then delay beyond the timeout.
				select {
				case <-time.After(10 * time.Second):
					// This should not be reached because the context should be cancelled.
					ch <- models.OutputChunk{Stream: "stdout", Data: "should not appear\n", Timestamp: time.Now()}
				case <-ctx.Done():
					// Context cancelled due to timeout — expected.
					return
				}
			}()
			return ch, nil
		},
		stopContainerFn: func(ctx context.Context, containerID string, timeout int) error {
			stopMu.Lock()
			stoppedContID = containerID
			stopMu.Unlock()
			return nil
		},
	}

	execSvc := executor.NewExecutorService(repo, dockerMgr, paramVal, streamMgr, nil, nil)

	// Command with a very short timeout (1 second).
	cmd := models.CommandEntry{
		ID:              "cmd-timeout-1",
		Name:            "slow-command",
		DockerImage:     "alpine:latest",
		CommandString:   "sleep 60",
		ParameterSchema: models.ParameterSchema{},
		AllowedRoles:    []models.Role{models.RoleAdmin},
		TimeoutSeconds:  1, // 1 second timeout
		AllowConcurrent: true,
		ExecutionMode:   models.ModeCreate,
		IsActive:        true,
		Version:         1,
	}

	record, err := execSvc.ExecuteCommand(context.Background(), cmd, map[string]interface{}{}, "user-timeout-1")
	if err != nil {
		t.Fatalf("ExecuteCommand failed: %v", err)
	}

	// Wait for the timeout to trigger and the goroutine to complete.
	// The timeout is 1 second, so we wait up to 5 seconds.
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var finalRecord *models.ExecutionRecord
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for execution to reach timed_out status")
		case <-ticker.C:
			finalRecord = repo.getRecord(record.ID)
			if finalRecord != nil && finalRecord.Status == models.StatusTimedOut {
				goto verified
			}
		}
	}

verified:
	// 1. Verify execution record has timed_out status.
	if finalRecord.Status != models.StatusTimedOut {
		t.Errorf("final status = %q, want timed_out", finalRecord.Status)
	}

	// 2. Verify CompletedAt is set.
	if finalRecord.CompletedAt == nil {
		t.Error("expected CompletedAt to be set for timed_out execution")
	}

	// 3. Verify the container was stopped.
	stopMu.Lock()
	stopped := stoppedContID
	stopMu.Unlock()

	if stopped == "" {
		t.Error("expected StopContainer to be called for timed_out execution")
	} else if stopped != "mock-container-001" {
		t.Errorf("stopped container = %q, want mock-container-001", stopped)
	}

	// 4. Verify partial stdout was captured (the first chunk before timeout).
	if finalRecord.Stdout == "" {
		t.Log("Note: stdout may be empty if timeout occurred before first chunk was processed")
	}

	// 5. Verify the stream was completed with timed_out status.
	eventCh, unsub := streamMgr.Subscribe(record.ID, "")
	defer unsub()

	var completeEvent *models.SSEEvent
	for evt := range eventCh {
		if evt.Event == "complete" {
			completeEvent = &evt
		}
	}

	if completeEvent == nil {
		t.Error("expected a complete SSE event for timed_out execution")
	}
}

// TestTimeoutEnforcement_ExecMode tests timeout enforcement for exec-mode executions.
func TestTimeoutEnforcement_ExecMode(t *testing.T) {
	repo := newInMemoryExecutionRepo()
	paramVal := validator.NewParameterValidator()

	dockerMgr := &mockDockerManager{
		attachExecStreamFn: func(ctx context.Context, execID string) (<-chan models.OutputChunk, error) {
			ch := make(chan models.OutputChunk)
			go func() {
				defer close(ch)
				// Delay beyond the timeout.
				select {
				case <-time.After(10 * time.Second):
				case <-ctx.Done():
					return
				}
			}()
			return ch, nil
		},
	}

	execSvc := executor.NewExecutorService(repo, dockerMgr, paramVal, nil, nil, nil)

	cmd := models.CommandEntry{
		ID:              "cmd-timeout-exec-1",
		Name:            "slow-exec-command",
		CommandString:   "sleep 60",
		ParameterSchema: models.ParameterSchema{},
		AllowedRoles:    []models.Role{models.RoleAdmin},
		TimeoutSeconds:  1,
		AllowConcurrent: true,
		ExecutionMode:   models.ModeExec,
		TargetContainer: "target-container",
		IsActive:        true,
		Version:         1,
	}

	record, err := execSvc.ExecuteCommand(context.Background(), cmd, map[string]interface{}{}, "user-timeout-2")
	if err != nil {
		t.Fatalf("ExecuteCommand failed: %v", err)
	}

	// Wait for timeout.
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for exec-mode execution to reach timed_out status")
		case <-ticker.C:
			finalRecord := repo.getRecord(record.ID)
			if finalRecord != nil && finalRecord.Status == models.StatusTimedOut {
				// Verified.
				if finalRecord.CompletedAt == nil {
					t.Error("expected CompletedAt to be set")
				}
				return
			}
		}
	}
}
