package lsp

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"strings"
	"time"
)

// GoplsClient implements Client for Go source files using a gopls subprocess.
type GoplsClient struct {
	*stdioClient
}

// NewGoplsClient starts gopls in stdio mode rooted at rootDir.
func NewGoplsClient(rootDir string, timeout time.Duration) (*GoplsClient, error) {
	c, err := newStdioClient("gopls", []string{"-mode=stdio"}, map[string]int{
		".go": 100,
	}, "go", rootDir, timeout)
	if err != nil {
		return nil, fmt.Errorf("gopls: %w", err)
	}
	return &GoplsClient{stdioClient: c}, nil
}

// Imports implements Client using go/parser for accurate import extraction.
func (c *GoplsClient) Imports(_ context.Context, file string) ([]Import, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
	if err != nil {
		return nil, fmt.Errorf("gopls: parse imports %q: %w", file, err)
	}
	out := make([]Import, 0, len(f.Imports))
	for _, spec := range f.Imports {
		path := strings.Trim(spec.Path.Value, `"`)
		alias := ""
		if spec.Name != nil && spec.Name.Name != "_" && spec.Name.Name != "." {
			alias = spec.Name.Name
		}
		out = append(out, Import{Path: path, Alias: alias, IsStdlib: isGoStdlib(path)})
	}
	return out, nil
}

func isGoStdlib(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}
