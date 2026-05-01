package stream

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"github.com/kariz/kariz/internal/models"
)

// StreamManager manages SSE connections for real-time output delivery.
type StreamManager interface {
	// Subscribe returns a channel of SSE events for an execution.
	// If lastEventID is provided, replays events from that point.
	Subscribe(executionID string, lastEventID string) (<-chan models.SSEEvent, func())

	// Publish sends an output chunk to all subscribers of an execution.
	Publish(executionID string, chunk models.OutputChunk)

	// Complete signals execution completion to all subscribers.
	Complete(executionID string, finalStatus models.ExecutionStatus)

	// Cleanup removes all state for an execution.
	Cleanup(executionID string)
}

// subscriber represents a single SSE subscriber channel.
type subscriber struct {
	ch chan models.SSEEvent
}

// executionStream holds per-execution state: event buffer, subscribers, and metadata.
type executionStream struct {
	mu          sync.RWMutex
	events      []models.SSEEvent
	subscribers map[*subscriber]struct{}
	nextID      int
	done        bool
}

// inMemoryStreamManager is the in-memory implementation of StreamManager.
type inMemoryStreamManager struct {
	mu      sync.RWMutex
	streams map[string]*executionStream
}

// NewStreamManager creates a new in-memory StreamManager.
func NewStreamManager() StreamManager {
	return &inMemoryStreamManager{
		streams: make(map[string]*executionStream),
	}
}

// getOrCreateStream returns the executionStream for the given ID, creating one if needed.
func (m *inMemoryStreamManager) getOrCreateStream(executionID string) *executionStream {
	m.mu.Lock()
	defer m.mu.Unlock()

	es, ok := m.streams[executionID]
	if !ok {
		es = &executionStream{
			events:      make([]models.SSEEvent, 0),
			subscribers: make(map[*subscriber]struct{}),
			nextID:      1,
		}
		m.streams[executionID] = es
	}
	return es
}

// Subscribe returns a channel of SSEEvent and an unsubscribe function.
// If lastEventID is provided, replays events from that point (ID+1).
// If no lastEventID, replays all buffered events.
// Then streams new events as they arrive.
func (m *inMemoryStreamManager) Subscribe(executionID string, lastEventID string) (<-chan models.SSEEvent, func()) {
	es := m.getOrCreateStream(executionID)

	sub := &subscriber{
		ch: make(chan models.SSEEvent, 256),
	}

	es.mu.Lock()

	// Determine replay start index.
	replayFrom := 0
	if lastEventID != "" {
		if id, err := strconv.Atoi(lastEventID); err == nil {
			// Replay events after the given ID.
			for i, evt := range es.events {
				evtID, _ := strconv.Atoi(evt.ID)
				if evtID > id {
					replayFrom = i
					break
				}
				// If we reach the end without finding a greater ID, no replay needed.
				replayFrom = len(es.events)
			}
		}
	}

	// Replay buffered events.
	for i := replayFrom; i < len(es.events); i++ {
		sub.ch <- es.events[i]
	}

	// If the execution is already done, close the channel immediately.
	if es.done {
		close(sub.ch)
		es.mu.Unlock()
		return sub.ch, func() {}
	}

	// Register subscriber for future events.
	es.subscribers[sub] = struct{}{}
	es.mu.Unlock()

	// Unsubscribe function removes the subscriber and closes its channel.
	unsubscribe := func() {
		es.mu.Lock()
		defer es.mu.Unlock()
		if _, ok := es.subscribers[sub]; ok {
			delete(es.subscribers, sub)
			close(sub.ch)
		}
	}

	return sub.ch, unsubscribe
}

// Publish assigns a sequential event ID, buffers the event, and fans out to all subscribers.
func (m *inMemoryStreamManager) Publish(executionID string, chunk models.OutputChunk) {
	es := m.getOrCreateStream(executionID)

	data, _ := json.Marshal(chunk)

	es.mu.Lock()
	defer es.mu.Unlock()

	if es.done {
		return
	}

	event := models.SSEEvent{
		ID:    fmt.Sprintf("%d", es.nextID),
		Event: "output",
		Data:  string(data),
	}
	es.nextID++
	es.events = append(es.events, event)

	// Fan out to all active subscribers.
	for sub := range es.subscribers {
		select {
		case sub.ch <- event:
		default:
			// Subscriber channel full — skip to avoid blocking.
		}
	}
}

// Complete sends a completion SSEEvent and closes all subscriber channels.
func (m *inMemoryStreamManager) Complete(executionID string, finalStatus models.ExecutionStatus) {
	es := m.getOrCreateStream(executionID)

	data, _ := json.Marshal(map[string]string{
		"status": string(finalStatus),
	})

	es.mu.Lock()
	defer es.mu.Unlock()

	if es.done {
		return
	}

	event := models.SSEEvent{
		ID:    fmt.Sprintf("%d", es.nextID),
		Event: "complete",
		Data:  string(data),
	}
	es.nextID++
	es.events = append(es.events, event)
	es.done = true

	// Send completion event and close all subscriber channels.
	for sub := range es.subscribers {
		select {
		case sub.ch <- event:
		default:
		}
		close(sub.ch)
	}
	es.subscribers = make(map[*subscriber]struct{})
}

// Cleanup removes the event buffer and subscriber list for an execution.
func (m *inMemoryStreamManager) Cleanup(executionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if es, ok := m.streams[executionID]; ok {
		es.mu.Lock()
		// Close any remaining subscriber channels.
		for sub := range es.subscribers {
			close(sub.ch)
		}
		es.subscribers = nil
		es.events = nil
		es.mu.Unlock()
	}

	delete(m.streams, executionID)
}
