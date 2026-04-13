package lsp

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// PyrightClient implements Client for Python source files using a
// pyright-langserver subprocess.
type PyrightClient struct {
	*stdioClient
}

// NewPyrightClient starts pyright-langserver in stdio mode rooted at rootDir.
func NewPyrightClient(rootDir string, timeout time.Duration) (*PyrightClient, error) {
	c, err := newStdioClient("pyright-langserver", []string{"--stdio"}, ".py", "python", rootDir, timeout)
	if err != nil {
		return nil, fmt.Errorf("pyright: %w", err)
	}
	return &PyrightClient{stdioClient: c}, nil
}

var (
	reImport     = regexp.MustCompile(`^import\s+(.+)$`)
	reFromImport = regexp.MustCompile(`^from\s+(\S+)\s+import\s+`)
)

// Imports implements Client by parsing Python import statements from the
// source file.
func (c *PyrightClient) Imports(_ context.Context, file string) ([]Import, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("pyright: open %q: %w", file, err)
	}
	defer f.Close()

	var out []Import
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if m := reFromImport.FindStringSubmatch(line); m != nil {
			out = append(out, Import{Path: m[1]})
			continue
		}

		if m := reImport.FindStringSubmatch(line); m != nil {
			for part := range strings.SplitSeq(m[1], ",") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				var imp Import
				if before, after, ok := strings.Cut(part, " as "); ok {
					imp.Path = strings.TrimSpace(before)
					imp.Alias = strings.TrimSpace(after)
				} else {
					imp.Path = part
				}
				out = append(out, imp)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("pyright: scan %q: %w", file, err)
	}
	return out, nil
}
