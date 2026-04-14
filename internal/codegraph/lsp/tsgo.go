package lsp

import (
	"context"
	"fmt"
	"time"
)

// TSGoClient implements Client for TypeScript/JavaScript source files via
// tsgo LSP in stdio mode.
type TSGoClient struct {
	*stdioClient
}

// NewTSGoClient starts tsgo in LSP stdio mode rooted at rootDir.
func NewTSGoClient(rootDir string, timeout time.Duration) (*TSGoClient, error) {
	c, err := newStdioClient("tsgo", []string{"--lsp"}, map[string]int{
		".ts":  100,
		".tsx": 95,
		".js":  90,
		".jsx": 85,
	}, "typescript", rootDir, timeout)
	if err != nil {
		return nil, fmt.Errorf("tsgo: %w", err)
	}
	return &TSGoClient{stdioClient: c}, nil
}

// Imports implements Client by parsing TypeScript/JavaScript import syntax.
func (c *TSGoClient) Imports(_ context.Context, file string) ([]Import, error) {
	return parseTypeScriptImports(file)
}
