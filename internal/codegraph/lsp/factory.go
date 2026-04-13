package lsp

import (
	"strings"
	"time"
)

// NewClientForExt creates a Client for the given file extension (e.g. ".go").
// Returns (nil, nil) when no implementation is registered for the extension so
// callers can skip LSP gracefully for unsupported languages.
func NewClientForExt(ext, rootDir string, timeout time.Duration) (Client, error) {
	switch strings.ToLower(ext) {
	case ".go":
		return NewGoplsClient(rootDir, timeout)
	case ".py":
		return NewPyrightClient(rootDir, timeout)
	}
	return nil, nil
}
