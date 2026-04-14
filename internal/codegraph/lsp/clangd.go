package lsp

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"time"
)

// ClangdClient implements Client for C/C++ source files using a clangd
// subprocess in stdio mode.
type ClangdClient struct {
	*stdioClient
}

// NewClangdClient starts clangd rooted at rootDir.
func NewClangdClient(rootDir string, timeout time.Duration) (*ClangdClient, error) {
	c, err := newStdioClient("clangd", nil, map[string]int{
		".c":   100,
		".h":   95,
		".hpp": 95,
		".hh":  95,
		".cpp": 90,
		".cc":  90,
		".cxx": 90,
	}, "cpp", rootDir, timeout)
	if err != nil {
		return nil, fmt.Errorf("clangd: %w", err)
	}
	return &ClangdClient{stdioClient: c}, nil
}

var reInclude = regexp.MustCompile(`^\s*#\s*include\s*([<"])\s*([^>"]+)\s*[>"]`)

// Imports implements Client by parsing C/C++ #include directives from source.
func (c *ClangdClient) Imports(_ context.Context, file string) ([]Import, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("clangd: open %q: %w", file, err)
	}
	defer func() {
		_ = f.Close()
	}()

	var out []Import
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		m := reInclude.FindStringSubmatch(scanner.Text())
		if len(m) != 3 {
			continue
		}
		out = append(out, Import{
			Path:     m[2],
			IsStdlib: m[1] == "<",
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("clangd: scan %q: %w", file, err)
	}
	return out, nil
}
