package closeutil

import (
	"io"
	"log/slog"
)

// Log closes c and emits a warning when Close returns an error.
func Log(c io.Closer, resource string) {
	if c == nil {
		return
	}
	if err := c.Close(); err != nil {
		slog.Warn("failed to close resource", "resource", resource, "err", err)
	}
}
