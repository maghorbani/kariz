package auth

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kariz/kariz/internal/models"
)

// cleanupMockSessionStore tracks calls to CleanExpired.
type cleanupMockSessionStore struct {
	cleanCount atomic.Int32
	cleanErr   error
}

func (m *cleanupMockSessionStore) Create(_ context.Context, _ models.SessionData) (string, error) {
	return "", nil
}

func (m *cleanupMockSessionStore) Get(_ context.Context, _ string) (*models.SessionData, error) {
	return nil, nil
}

func (m *cleanupMockSessionStore) Delete(_ context.Context, _ string) error {
	return nil
}

func (m *cleanupMockSessionStore) DeleteByUserID(_ context.Context, _ string) error {
	return nil
}

func (m *cleanupMockSessionStore) CleanExpired(_ context.Context) error {
	m.cleanCount.Add(1)
	return m.cleanErr
}

func TestStartSessionCleanup_RunsPeriodicCleanup(t *testing.T) {
	store := &cleanupMockSessionStore{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a very short interval for testing
	StartSessionCleanup(ctx, store, 50*time.Millisecond)

	// Wait enough time for at least 2 ticks
	time.Sleep(180 * time.Millisecond)
	cancel()

	count := store.cleanCount.Load()
	if count < 2 {
		t.Errorf("expected at least 2 cleanup calls, got %d", count)
	}
}

func TestStartSessionCleanup_StopsOnContextCancel(t *testing.T) {
	store := &cleanupMockSessionStore{}
	ctx, cancel := context.WithCancel(context.Background())

	StartSessionCleanup(ctx, store, 50*time.Millisecond)

	// Let it run a bit
	time.Sleep(120 * time.Millisecond)
	cancel()

	// Record count after cancel
	time.Sleep(100 * time.Millisecond)
	countAfterCancel := store.cleanCount.Load()

	// Wait more and verify no additional calls
	time.Sleep(150 * time.Millisecond)
	countLater := store.cleanCount.Load()

	if countLater != countAfterCancel {
		t.Errorf("expected no more cleanup calls after cancel, got %d after vs %d later", countAfterCancel, countLater)
	}
}

func TestStartSessionCleanup_HandlesErrors(t *testing.T) {
	store := &cleanupMockSessionStore{
		cleanErr: context.DeadlineExceeded,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Should not panic even when CleanExpired returns errors
	StartSessionCleanup(ctx, store, 50*time.Millisecond)

	time.Sleep(120 * time.Millisecond)

	count := store.cleanCount.Load()
	if count < 1 {
		t.Errorf("expected at least 1 cleanup call even with errors, got %d", count)
	}
}
