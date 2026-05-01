package auth

import (
	"context"
	"log/slog"
	"time"
)

// StartSessionCleanup starts a background goroutine that periodically removes expired sessions.
// It runs on the given interval and stops when the context is cancelled.
// This function is intended to be called once during application startup.
func StartSessionCleanup(ctx context.Context, store SessionStore, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		slog.Info("session cleanup started", "interval", interval.String())

		for {
			select {
			case <-ctx.Done():
				slog.Info("session cleanup stopped")
				return
			case <-ticker.C:
				if err := store.CleanExpired(ctx); err != nil {
					slog.Error("session cleanup failed", "error", err)
				} else {
					slog.Debug("session cleanup completed")
				}
			}
		}
	}()
}
