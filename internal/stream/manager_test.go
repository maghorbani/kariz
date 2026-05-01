package stream

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kariz/kariz/internal/models"
)

// collectEvents drains a channel into a slice, with a timeout to prevent hanging.
func collectEvents(ch <-chan models.SSEEvent, timeout time.Duration) []models.SSEEvent {
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

func TestPublishAndSubscribeReceivesEvents(t *testing.T) {
	mgr := NewStreamManager()
	execID := "exec-1"

	ch, unsub := mgr.Subscribe(execID, "")
	defer unsub()

	chunk := models.OutputChunk{
		Stream:    "stdout",
		Data:      "hello world",
		Timestamp: time.Now(),
	}
	mgr.Publish(execID, chunk)

	select {
	case evt := <-ch:
		if evt.ID != "1" {
			t.Errorf("expected event ID '1', got %q", evt.ID)
		}
		if evt.Event != "output" {
			t.Errorf("expected event type 'output', got %q", evt.Event)
		}
		var received models.OutputChunk
		if err := json.Unmarshal([]byte(evt.Data), &received); err != nil {
			t.Fatalf("failed to unmarshal event data: %v", err)
		}
		if received.Data != "hello world" {
			t.Errorf("expected data 'hello world', got %q", received.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSubscribeWithLastEventIDReplaysFromThatPoint(t *testing.T) {
	mgr := NewStreamManager()
	execID := "exec-2"

	// Publish 3 events before subscribing.
	for i := 0; i < 3; i++ {
		mgr.Publish(execID, models.OutputChunk{
			Stream:    "stdout",
			Data:      "line " + string(rune('A'+i)),
			Timestamp: time.Now(),
		})
	}

	// Subscribe with lastEventID "1" — should replay events 2 and 3.
	ch, unsub := mgr.Subscribe(execID, "1")
	defer unsub()

	// Complete so the channel closes and we can collect.
	mgr.Complete(execID, models.StatusCompleted)

	events := collectEvents(ch, time.Second)

	// Should have event 2, event 3, and the completion event.
	if len(events) != 3 {
		t.Fatalf("expected 3 events (2 replayed + 1 complete), got %d", len(events))
	}
	if events[0].ID != "2" {
		t.Errorf("expected first replayed event ID '2', got %q", events[0].ID)
	}
	if events[1].ID != "3" {
		t.Errorf("expected second replayed event ID '3', got %q", events[1].ID)
	}
	if events[2].Event != "complete" {
		t.Errorf("expected completion event, got %q", events[2].Event)
	}
}

func TestSubscribeWithNoLastEventIDReplaysAll(t *testing.T) {
	mgr := NewStreamManager()
	execID := "exec-3"

	// Publish 3 events before subscribing.
	for i := 0; i < 3; i++ {
		mgr.Publish(execID, models.OutputChunk{
			Stream:    "stdout",
			Data:      "line",
			Timestamp: time.Now(),
		})
	}

	// Subscribe with no lastEventID — should replay all 3 events.
	ch, unsub := mgr.Subscribe(execID, "")
	defer unsub()

	// Complete so the channel closes.
	mgr.Complete(execID, models.StatusCompleted)

	events := collectEvents(ch, time.Second)

	// Should have 3 replayed events + 1 completion event.
	if len(events) != 4 {
		t.Fatalf("expected 4 events (3 replayed + 1 complete), got %d", len(events))
	}
	if events[0].ID != "1" {
		t.Errorf("expected first event ID '1', got %q", events[0].ID)
	}
	if events[1].ID != "2" {
		t.Errorf("expected second event ID '2', got %q", events[1].ID)
	}
	if events[2].ID != "3" {
		t.Errorf("expected third event ID '3', got %q", events[2].ID)
	}
}

func TestMultipleSubscribersReceiveSameEvents(t *testing.T) {
	mgr := NewStreamManager()
	execID := "exec-4"

	ch1, unsub1 := mgr.Subscribe(execID, "")
	defer unsub1()

	ch2, unsub2 := mgr.Subscribe(execID, "")
	defer unsub2()

	chunk := models.OutputChunk{
		Stream:    "stdout",
		Data:      "shared data",
		Timestamp: time.Now(),
	}
	mgr.Publish(execID, chunk)

	// Both subscribers should receive the event.
	for i, ch := range []<-chan models.SSEEvent{ch1, ch2} {
		select {
		case evt := <-ch:
			if evt.ID != "1" {
				t.Errorf("subscriber %d: expected event ID '1', got %q", i, evt.ID)
			}
			if evt.Event != "output" {
				t.Errorf("subscriber %d: expected event type 'output', got %q", i, evt.Event)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out waiting for event", i)
		}
	}
}

func TestCompleteClosesSubscriberChannels(t *testing.T) {
	mgr := NewStreamManager()
	execID := "exec-5"

	ch, unsub := mgr.Subscribe(execID, "")
	defer unsub()

	mgr.Complete(execID, models.StatusCompleted)

	events := collectEvents(ch, time.Second)

	// Should receive the completion event and then the channel should close.
	if len(events) != 1 {
		t.Fatalf("expected 1 event (complete), got %d", len(events))
	}
	if events[0].Event != "complete" {
		t.Errorf("expected event type 'complete', got %q", events[0].Event)
	}

	// Verify the status in the completion data.
	var data map[string]string
	if err := json.Unmarshal([]byte(events[0].Data), &data); err != nil {
		t.Fatalf("failed to unmarshal completion data: %v", err)
	}
	if data["status"] != "completed" {
		t.Errorf("expected status 'completed', got %q", data["status"])
	}
}

func TestCleanupRemovesExecutionState(t *testing.T) {
	mgr := NewStreamManager()
	execID := "exec-6"

	// Publish some events.
	mgr.Publish(execID, models.OutputChunk{
		Stream:    "stdout",
		Data:      "data",
		Timestamp: time.Now(),
	})

	// Cleanup.
	mgr.Cleanup(execID)

	// Subscribe after cleanup — should get no replayed events.
	ch, unsub := mgr.Subscribe(execID, "")
	defer unsub()

	// Publish a new event — should get ID 1 since state was reset.
	mgr.Publish(execID, models.OutputChunk{
		Stream:    "stdout",
		Data:      "new data",
		Timestamp: time.Now(),
	})

	select {
	case evt := <-ch:
		if evt.ID != "1" {
			t.Errorf("expected event ID '1' after cleanup, got %q", evt.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event after cleanup")
	}
}

func TestCompleteIsIdempotent(t *testing.T) {
	mgr := NewStreamManager()
	execID := "exec-7"

	ch, unsub := mgr.Subscribe(execID, "")
	defer unsub()

	mgr.Complete(execID, models.StatusCompleted)
	// Second complete should be a no-op.
	mgr.Complete(execID, models.StatusFailed)

	events := collectEvents(ch, time.Second)

	if len(events) != 1 {
		t.Fatalf("expected 1 completion event, got %d", len(events))
	}
}

func TestPublishAfterCompleteIsIgnored(t *testing.T) {
	mgr := NewStreamManager()
	execID := "exec-8"

	ch, unsub := mgr.Subscribe(execID, "")
	defer unsub()

	mgr.Complete(execID, models.StatusCompleted)

	// Publish after complete should be ignored.
	mgr.Publish(execID, models.OutputChunk{
		Stream:    "stdout",
		Data:      "should not appear",
		Timestamp: time.Now(),
	})

	events := collectEvents(ch, 200*time.Millisecond)

	// Only the completion event should be present.
	if len(events) != 1 {
		t.Fatalf("expected 1 event (complete only), got %d", len(events))
	}
}

func TestConcurrentPublishAndSubscribe(t *testing.T) {
	mgr := NewStreamManager()
	execID := "exec-9"

	ch, unsub := mgr.Subscribe(execID, "")
	defer unsub()

	const numEvents = 100
	var wg sync.WaitGroup
	wg.Add(numEvents)

	for i := 0; i < numEvents; i++ {
		go func() {
			defer wg.Done()
			mgr.Publish(execID, models.OutputChunk{
				Stream:    "stdout",
				Data:      "concurrent",
				Timestamp: time.Now(),
			})
		}()
	}

	wg.Wait()
	mgr.Complete(execID, models.StatusCompleted)

	events := collectEvents(ch, 2*time.Second)

	// Should receive all published events plus the completion event.
	if len(events) != numEvents+1 {
		t.Errorf("expected %d events, got %d", numEvents+1, len(events))
	}
}

func TestSubscribeAfterCompleteReplaysAllAndCloses(t *testing.T) {
	mgr := NewStreamManager()
	execID := "exec-10"

	// Publish and complete before subscribing.
	mgr.Publish(execID, models.OutputChunk{
		Stream:    "stdout",
		Data:      "before complete",
		Timestamp: time.Now(),
	})
	mgr.Complete(execID, models.StatusFailed)

	// Subscribe after completion — should replay all events and close immediately.
	ch, unsub := mgr.Subscribe(execID, "")
	defer unsub()

	events := collectEvents(ch, time.Second)

	// Should have the output event and the completion event.
	if len(events) != 2 {
		t.Fatalf("expected 2 events (1 output + 1 complete), got %d", len(events))
	}
	if events[0].Event != "output" {
		t.Errorf("expected first event type 'output', got %q", events[0].Event)
	}
	if events[1].Event != "complete" {
		t.Errorf("expected second event type 'complete', got %q", events[1].Event)
	}
}

func TestSequentialEventIDs(t *testing.T) {
	mgr := NewStreamManager()
	execID := "exec-11"

	for i := 0; i < 5; i++ {
		mgr.Publish(execID, models.OutputChunk{
			Stream:    "stdout",
			Data:      "data",
			Timestamp: time.Now(),
		})
	}

	ch, unsub := mgr.Subscribe(execID, "")
	defer unsub()

	mgr.Complete(execID, models.StatusCompleted)

	events := collectEvents(ch, time.Second)

	// 5 output events + 1 complete = 6 total, IDs 1-6.
	if len(events) != 6 {
		t.Fatalf("expected 6 events, got %d", len(events))
	}
	for i, evt := range events {
		expectedID := i + 1
		if evt.ID != fmt.Sprintf("%d", expectedID) {
			t.Errorf("event %d: expected ID '%d', got %q", i, expectedID, evt.ID)
		}
	}
}
