package codegraph

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/simplyblock/postbrain/internal/codegraph/lsp"
)

// lspClientAdapter bridges lsp.Client to the LSPResolver interface used by the
// codegraph indexer.  It resolves a symbol name to a canonical identifier by
// scanning the on-disk file for the symbol's position and delegating to
// lsp.Client.CanonicalName.
//
// LSP resolution therefore requires the source tree to be present on disk
// (i.e. GoLSPRootDir must point at an actual checkout).
type lspClientAdapter struct {
	client  lsp.Client
	rootDir string
}

func (a *lspClientAdapter) Language() string { return a.client.Language() }
func (a *lspClientAdapter) Close() error     { return a.client.Close() }

// Resolve implements LSPResolver.  It locates symbol in file on disk,
// converts the byte offset to an LSP position, and returns the fully-qualified
// canonical name reported by gopls.
func (a *lspClientAdapter) Resolve(ctx context.Context, file, symbol string) (string, error) {
	absPath := file
	if !filepath.IsAbs(file) {
		absPath = filepath.Join(a.rootDir, filepath.FromSlash(file))
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("lsp: read %q: %w", absPath, err)
	}
	pos, ok := findSymbolPos(data, symbol)
	if !ok {
		return "", nil
	}
	return a.client.CanonicalName(ctx, absPath, pos)
}

// findSymbolPos scans src for the first occurrence of symbol and returns its
// LSP Position.  It prefers "symbol(" (call-site) over bare "symbol" so that
// definition jumps land on the declaration rather than a comment.
func findSymbolPos(src []byte, symbol string) (lsp.Position, bool) {
	off := bytes.Index(src, []byte(symbol+"("))
	if off < 0 {
		off = bytes.Index(src, []byte(symbol))
	}
	if off < 0 {
		return lsp.Position{}, false
	}
	var line, char uint32
	for i := 0; i < off; i++ {
		if src[i] == '\n' {
			line++
			char = 0
		} else {
			char++
		}
	}
	return lsp.Position{Line: line, Character: char}, true
}
