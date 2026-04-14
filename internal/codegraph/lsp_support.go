package codegraph

import (
	"path/filepath"
	"strings"

	"github.com/simplyblock/postbrain/internal/codegraph/lsp"
)

func lspSupportsFile(client lsp.Client, filePath string) bool {
	if client == nil {
		return false
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == "" {
		return false
	}
	return lspSupportsExt(client, ext)
}

func lspSupportsExt(client lsp.Client, ext string) bool {
	if client == nil {
		return false
	}
	ext = strings.ToLower(strings.TrimSpace(ext))
	if ext == "" {
		return false
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	_, ok := client.SupportedLanguages()[ext]
	return ok
}
