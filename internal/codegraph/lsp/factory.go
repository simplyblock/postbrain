package lsp

import (
	"strings"
	"time"
)

// ClientOptions controls optional backend selection per language.
type ClientOptions struct {
	// UseTSGo selects the tsgo language server backend for TypeScript
	// extensions instead of typescript-language-server.
	UseTSGo bool
}

var (
	newGoplsClient = func(rootDir string, timeout time.Duration) (Client, error) {
		return NewGoplsClient(rootDir, timeout)
	}
	newPyrightClient = func(rootDir string, timeout time.Duration) (Client, error) {
		return NewPyrightClient(rootDir, timeout)
	}
	newClangdClient = func(rootDir string, timeout time.Duration) (Client, error) {
		return NewClangdClient(rootDir, timeout)
	}
	newMarksmanClient = func(rootDir string, timeout time.Duration) (Client, error) {
		return NewMarksmanClient(rootDir, timeout)
	}
	newTypeScriptLanguageServerClient = func(rootDir string, timeout time.Duration) (Client, error) {
		return NewTypeScriptLanguageServerClient(rootDir, timeout)
	}
	newTSGoClient = func(rootDir string, timeout time.Duration) (Client, error) {
		return NewTSGoClient(rootDir, timeout)
	}
)

// NewClientForExt creates a Client for the given file extension (e.g. ".go").
// Returns (nil, nil) when no implementation is registered for the extension so
// callers can skip LSP gracefully for unsupported languages.
func NewClientForExt(ext, rootDir string, timeout time.Duration, opts ClientOptions) (Client, error) {
	switch strings.ToLower(ext) {
	case ".go":
		return newGoplsClient(rootDir, timeout)
	case ".py":
		return newPyrightClient(rootDir, timeout)
	case ".c", ".h", ".hpp", ".hh", ".cpp", ".cc", ".cxx":
		return newClangdClient(rootDir, timeout)
	case ".md", ".markdown":
		return newMarksmanClient(rootDir, timeout)
	case ".ts", ".tsx", ".js", ".jsx":
		if opts.UseTSGo {
			return newTSGoClient(rootDir, timeout)
		}
		return newTypeScriptLanguageServerClient(rootDir, timeout)
	}
	return nil, nil
}
