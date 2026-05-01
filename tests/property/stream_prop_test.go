package property_test

// Feature: kariz-command-dashboard, Property 15: SSE stream replay and resume

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/kariz/kariz/internal/models"
	"github.com/kariz/kariz/internal/stream"
	"pgregory.net/rapid"
)

// collectSSEEvents drains a channel into a slice, with a timeout to prevent hanging.
func collectSSEEvents(ch <-chan models.SSEEvent, timeout time.Duration) []models.SSEEvent {
	var events []models.SSEEvent
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, evt)
		case <-timer.C:
			return events
		}
	}
}

// publishNEvents publishes N output events to the stream manager for the given execution ID.
func publishNEvents(mgr stream.StreamManager, executionID string, n int) {
	for i := 0; i < n; i++ {
		mgr.Publish(executionID, models.OutputChunk{
			Stream:    "stdout",
			Data:      fmt.Sprintf("output-line-%d", i+1),
			Timestamp: time.Now(),
		})
	}
}

// --- Property 15 Tests ---
// **Validates: Requirements 9.3**

// TestProperty15_ReplayFromLastEventID tests that for any execution that has produced
// N output events, subscribing with a lastEventID of K (where 0 ≤ K ≤ N) delivers
// events starting from K+1 through N, followed by the completion event.
// Event IDs must be sequential.
func TestProperty15_ReplayFromLastEventID(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 50).Draw(t, "N")
		k := rapid.IntRange(0, n).Draw(t, "K")

		mgr := stream.NewStreamManager()
		execID := fmt.Sprintf("exec-prop15-replay-%d-%d", n, k)

		// Publish N events before subscribing.
		publishNEvents(mgr, execID, n)

		// Subscribe with lastEventID = K.
		lastEventID := fmt.Sprintf("%d", k)
		ch, unsub := mgr.Subscribe(execID, lastEventID)
		defer unsub()

		// Complete the execution so the channel closes.
		mgr.Complete(execID, models.StatusCompleted)

		events := collectSSEEvents(ch, 2*time.Second)

		// Expected: events from K+1 through N (output events) + 1 completion event.
		expectedOutputCount := n - k
		expectedTotal := expectedOutputCount + 1 // +1 for completion event

		if len(events) != expectedTotal {
			t.Fatalf("N=%d, K=%d: expected %d events (%d output + 1 complete), got %d",
				n, k, expectedTotal, expectedOutputCount, len(events))
		}

		// Verify output events start from ID K+1 and are sequential.
		for i := 0; i < expectedOutputCount; i++ {
			evt := events[i]
			expectedID := k + 1 + i

			if evt.ID != fmt.Sprintf("%d", expectedID) {
				t.Fatalf("N=%d, K=%d: event[%d] expected ID '%d', got %q",
					n, k, i, expectedID, evt.ID)
			}
			if evt.Event != "output" {
				t.Fatalf("N=%d, K=%d: event[%d] expected type 'output', got %q",
					n, k, i, evt.Event)
			}
		}

		// Verify the last event is the completion event.
		completeEvt := events[len(events)-1]
		if completeEvt.Event != "complete" {
			t.Fatalf("N=%d, K=%d: last event expected type 'complete', got %q",
				n, k, completeEvt.Event)
		}

		// Completion event ID should be N+1 (sequential after all output events).
		expectedCompleteID := fmt.Sprintf("%d", n+1)
		if completeEvt.ID != expectedCompleteID {
			t.Fatalf("N=%d, K=%d: completion event expected ID '%s', got %q",
				n, k, expectedCompleteID, completeEvt.ID)
		}
	})
}

// TestProperty15_ReplayAllWithEmptyLastEventID tests that subscribing with an empty
// lastEventID replays all N events from the beginning, followed by the completion event.
func TestProperty15_ReplayAllWithEmptyLastEventID(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 50).Draw(t, "N")

		mgr := stream.NewStreamManager()
		execID := fmt.Sprintf("exec-prop15-all-%d", n)

		// Publish N events before subscribing.
		publishNEvents(mgr, execID, n)

		// Subscribe with empty lastEventID — should replay all.
		ch, unsub := mgr.Subscribe(execID, "")
		defer unsub()

		// Complete the execution so the channel closes.
		mgr.Complete(execID, models.StatusCompleted)

		events := collectSSEEvents(ch, 2*time.Second)

		// Expected: N output events + 1 completion event.
		expectedTotal := n + 1

		if len(events) != expectedTotal {
			t.Fatalf("N=%d: expected %d events (%d output + 1 complete), got %d",
				n, expectedTotal, n, len(events))
		}

		// Verify all output events are present with sequential IDs starting from 1.
		for i := 0; i < n; i++ {
			evt := events[i]
			expectedID := i + 1

			if evt.ID != fmt.Sprintf("%d", expectedID) {
				t.Fatalf("N=%d: event[%d] expected ID '%d', got %q",
					n, i, expectedID, evt.ID)
			}
			if evt.Event != "output" {
				t.Fatalf("N=%d: event[%d] expected type 'output', got %q",
					n, i, evt.Event)
			}
		}

		// Verify the completion event.
		completeEvt := events[len(events)-1]
		if completeEvt.Event != "complete" {
			t.Fatalf("N=%d: last event expected type 'complete', got %q",
				n, completeEvt.Event)
		}

		expectedCompleteID := fmt.Sprintf("%d", n+1)
		if completeEvt.ID != expectedCompleteID {
			t.Fatalf("N=%d: completion event expected ID '%s', got %q",
				n, expectedCompleteID, completeEvt.ID)
		}
	})
}

// TestProperty15_AllEventIDsSequential tests that for any N events published and
// then completed, all event IDs (output + completion) form a strictly sequential
// sequence from 1 to N+1.
func TestProperty15_AllEventIDsSequential(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 50).Draw(t, "N")

		mgr := stream.NewStreamManager()
		execID := fmt.Sprintf("exec-prop15-seq-%d", n)

		// Publish N events.
		publishNEvents(mgr, execID, n)

		// Subscribe with empty lastEventID to get all events.
		ch, unsub := mgr.Subscribe(execID, "")
		defer unsub()

		// Complete the execution.
		mgr.Complete(execID, models.StatusCompleted)

		events := collectSSEEvents(ch, 2*time.Second)

		// Verify sequential IDs from 1 to N+1.
		for i, evt := range events {
			expectedID := i + 1
			actualID, err := strconv.Atoi(evt.ID)
			if err != nil {
				t.Fatalf("N=%d: event[%d] has non-numeric ID %q", n, i, evt.ID)
			}
			if actualID != expectedID {
				t.Fatalf("N=%d: event[%d] expected ID %d, got %d", n, i, expectedID, actualID)
			}
		}
	})
}

// TestProperty15_ReplaySubsetContainsAllExpectedEvents tests that for any K in [0, N],
// the set of event IDs received when subscribing with lastEventID=K is exactly
// {K+1, K+2, ..., N, N+1} where N+1 is the completion event.
func TestProperty15_ReplaySubsetContainsAllExpectedEvents(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 50).Draw(t, "N")
		k := rapid.IntRange(0, n).Draw(t, "K")

		mgr := stream.NewStreamManager()
		execID := fmt.Sprintf("exec-prop15-subset-%d-%d", n, k)

		// Publish N events.
		publishNEvents(mgr, execID, n)

		// Subscribe with lastEventID = K.
		lastEventID := fmt.Sprintf("%d", k)
		ch, unsub := mgr.Subscribe(execID, lastEventID)
		defer unsub()

		// Complete the execution.
		mgr.Complete(execID, models.StatusCompleted)

		events := collectSSEEvents(ch, 2*time.Second)

		// Build the set of received event IDs.
		receivedIDs := make(map[int]bool, len(events))
		for _, evt := range events {
			id, err := strconv.Atoi(evt.ID)
			if err != nil {
				t.Fatalf("N=%d, K=%d: non-numeric event ID %q", n, k, evt.ID)
			}
			receivedIDs[id] = true
		}

		// Verify all expected IDs are present: K+1 through N (output) + N+1 (complete).
		for expectedID := k + 1; expectedID <= n+1; expectedID++ {
			if !receivedIDs[expectedID] {
				t.Fatalf("N=%d, K=%d: missing expected event ID %d. Received IDs: %v",
					n, k, expectedID, receivedIDs)
			}
		}

		// Verify no unexpected IDs are present (nothing <= K).
		for id := range receivedIDs {
			if id <= k {
				t.Fatalf("N=%d, K=%d: received unexpected event ID %d (should be > %d)",
					n, k, id, k)
			}
		}
	})
}
