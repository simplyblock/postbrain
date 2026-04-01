package closeutil

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

type testCloser struct {
	err error
}

func (c *testCloser) Close() error {
	return c.err
}

func TestLog_LogsCloseError(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	slog.SetDefault(logger)
	defer slog.SetDefault(old)

	Log(&testCloser{err: errors.New("boom")}, "test-resource")

	out := buf.String()
	if !strings.Contains(out, "failed to close resource") {
		t.Fatalf("expected close failure log, got: %q", out)
	}
	if !strings.Contains(out, "test-resource") {
		t.Fatalf("expected resource in log, got: %q", out)
	}
	if !strings.Contains(out, "boom") {
		t.Fatalf("expected error in log, got: %q", out)
	}
}

func TestLog_NoErrorNoLog(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	slog.SetDefault(logger)
	defer slog.SetDefault(old)

	Log(&testCloser{}, "test-resource")

	if got := buf.String(); got != "" {
		t.Fatalf("expected no log output, got: %q", got)
	}
}
