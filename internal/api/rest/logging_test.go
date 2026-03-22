package rest

import (
	"context"
	"log/slog"
	"testing"
)

// TestLogFromContext_ReturnsDefault verifies that LogFromContext returns
// slog.Default() when no logger has been stored in the context.
func TestLogFromContext_ReturnsDefault(t *testing.T) {
	ctx := context.Background()
	logger := LogFromContext(ctx)
	if logger != slog.Default() {
		t.Errorf("expected slog.Default(), got a different logger: %v", logger)
	}
}

// TestLogFromContext_ReturnsStoredLogger verifies that LogFromContext returns
// the logger injected by requestLoggerMiddleware.
func TestLogFromContext_ReturnsStoredLogger(t *testing.T) {
	ctx := context.Background()
	custom := slog.New(slog.Default().Handler())
	ctx = context.WithValue(ctx, loggerKey{}, custom)
	logger := LogFromContext(ctx)
	if logger != custom {
		t.Errorf("expected custom logger, got %v", logger)
	}
}
