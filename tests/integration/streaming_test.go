package integration

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kariz/kariz/internal/models"
	"github.com/kariz/kariz/internal/stream"
	streamPkg "github.com/kariz/kariz/internal/stream"
	"github.com/kariz/kariz/internal/validator"
	"github.com/kariz/kariz/internal/executor"
)

// noopAuthMiddleware is a pass-through middleware for testing (no auth required).
func noopAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

// TestSSEStreamingEndToEnd tests the full SSE streaming flow:
// Execute command → Create SSE handler with real StreamManager → Make HTTP request
// to stream endpoint → Verify output events received → Verify completion event.
func TestSSEStreamingEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 1. Set up real StreamManager and SSE handler.
	streamMgr := stream.NewStreamManager()
	sseHandler := streamPkg.NewSSEHandler(streamMgr)

	// 2. Set up Gin router with SSE endpoint.
	router := gin.New()
	api := router.Group("/api")
	sseHandler.RegisterRoutes(api, noopAuthMiddleware())

	// 3. Set up executor with mock Docker that produces output.
	repo := newInMemoryExecutionRepo()
	paramVal := validator.NewParameterValidator()

	outputChunks := []models.OutputChunk{
		{Stream: "stdout", Data: "line 1\n", Timestamp: time.Now()},
		{Stream: "stdout", Data: "line 2\n", Timestamp: time.Now()},
		{Stream: "stderr", Data: "warning\n", Timestamp: time.Now()},
	}

	dockerMgr := &mockDockerManager{
		attachStreamFn: func(ctx context.Context, containerID string) (<-chan models.OutputChunk, error) {
			ch := make(chan models.OutputChunk, len(outputChunks))
			for _, chunk := range outputChunks {
				ch <- chunk
			}
			close(ch)
			return ch, nil
		},
	}

	execSvc := executor.NewExecutorService(repo, dockerMgr, paramVal, streamMgr, nil, nil)

	// 4. Create and execute a command.
	cmd := models.CommandEntry{
		ID:              "cmd-sse-1",
		Name:            "sse-test-command",
		DockerImage:     "alpine:latest",
		CommandString:   "echo test",
		ParameterSchema: models.ParameterSchema{},
		AllowedRoles:    []models.Role{models.RoleAdmin},
		TimeoutSeconds:  30,
		AllowConcurrent: true,
		ExecutionMode:   models.ModeCreate,
		IsActive:        true,
		Version:         1,
	}

	record, err := execSvc.ExecuteCommand(context.Background(), cmd, map[string]interface{}{}, "user-sse-1")
	if err != nil {
		t.Fatalf("ExecuteCommand failed: %v", err)
	}

	// 5. Start httptest server.
	server := httptest.NewServer(router)
	defer server.Close()

	// 6. Wait briefly for execution goroutine to start publishing.
	time.Sleep(500 * time.Millisecond)

	// 7. Make SSE request to the stream endpoint.
	sseURL := server.URL + "/api/executions/" + record.ID + "/stream"
	req, err := http.NewRequest("GET", sseURL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", contentType)
	}

	// 8. Parse SSE events from the response body.
	scanner := bufio.NewScanner(resp.Body)
	var events []sseEvent
	var currentEvent sseEvent

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			// Empty line marks end of an event.
			if currentEvent.eventType != "" || currentEvent.data != "" {
				events = append(events, currentEvent)
				currentEvent = sseEvent{}
			}
			continue
		}

		if strings.HasPrefix(line, "id: ") {
			currentEvent.id = strings.TrimPrefix(line, "id: ")
		} else if strings.HasPrefix(line, "event: ") {
			currentEvent.eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			currentEvent.data = strings.TrimPrefix(line, "data: ")
		}
	}

	// 9. Verify we received events.
	if len(events) == 0 {
		t.Fatal("expected at least one SSE event, got none")
	}

	// Count output and complete events.
	outputCount := 0
	completeCount := 0
	for _, evt := range events {
		switch evt.eventType {
		case "output":
			outputCount++
		case "complete":
			completeCount++
		}
	}

	if outputCount < 1 {
		t.Errorf("expected at least 1 output event, got %d", outputCount)
	}

	if completeCount < 1 {
		t.Errorf("expected at least 1 complete event, got %d", completeCount)
	}

	// Verify the last event is a completion event.
	lastEvt := events[len(events)-1]
	if lastEvt.eventType != "complete" {
		t.Errorf("last event type = %q, want complete", lastEvt.eventType)
	}

	t.Logf("Received %d SSE events (%d output, %d complete)", len(events), outputCount, completeCount)
}

// sseEvent represents a parsed SSE event from the HTTP response.
type sseEvent struct {
	id        string
	eventType string
	data      string
}

// TestSSEStreamingReplay tests that SSE replay works correctly when connecting
// with a Last-Event-ID header.
func TestSSEStreamingReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)

	streamMgr := stream.NewStreamManager()

	executionID := "exec-replay-1"

	// Publish some events before subscribing.
	streamMgr.Publish(executionID, models.OutputChunk{Stream: "stdout", Data: "line 1\n", Timestamp: time.Now()})
	streamMgr.Publish(executionID, models.OutputChunk{Stream: "stdout", Data: "line 2\n", Timestamp: time.Now()})
	streamMgr.Publish(executionID, models.OutputChunk{Stream: "stdout", Data: "line 3\n", Timestamp: time.Now()})

	// Subscribe with Last-Event-ID = "1" to skip the first event.
	eventCh, unsub := streamMgr.Subscribe(executionID, "1")

	// Collect replayed events.
	var replayed []models.SSEEvent
	go func() {
		for evt := range eventCh {
			replayed = append(replayed, evt)
		}
	}()

	// Complete the execution to close the channel.
	streamMgr.Complete(executionID, models.StatusCompleted)

	// Wait briefly for events to be collected.
	time.Sleep(200 * time.Millisecond)
	unsub()

	// Should have received events 2, 3, and the complete event (skipping event 1).
	if len(replayed) < 2 {
		t.Errorf("expected at least 2 replayed events (after ID 1), got %d", len(replayed))
	}

	// First replayed event should have ID "2".
	if len(replayed) > 0 && replayed[0].ID != "2" {
		t.Errorf("first replayed event ID = %q, want 2", replayed[0].ID)
	}
}
