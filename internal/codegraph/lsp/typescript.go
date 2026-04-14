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

// TypeScriptLanguageServerClient implements Client for TypeScript/JavaScript
// source files via typescript-language-server in stdio mode.
type TypeScriptLanguageServerClient struct {
	*stdioClient
}

// NewTypeScriptLanguageServerClient starts typescript-language-server in stdio
// mode rooted at rootDir.
func NewTypeScriptLanguageServerClient(rootDir string, timeout time.Duration) (*TypeScriptLanguageServerClient, error) {
	c, err := newStdioClient("typescript-language-server", []string{"--stdio"}, map[string]int{
		".ts":  100,
		".tsx": 95,
		".js":  90,
		".jsx": 85,
	}, "typescript", rootDir, timeout)
	if err != nil {
		return nil, fmt.Errorf("typescript-language-server: %w", err)
	}
	return &TypeScriptLanguageServerClient{stdioClient: c}, nil
}

var (
	reJSImportFrom  = regexp.MustCompile(`^\s*import(?:\s+type)?\s+.+\s+from\s+["']([^"']+)["']`)
	reJSImportSide  = regexp.MustCompile(`^\s*import\s+["']([^"']+)["']`)
	reJSExportFrom  = regexp.MustCompile(`^\s*export\s+.+\s+from\s+["']([^"']+)["']`)
	reJSRequireCall = regexp.MustCompile(`require\(\s*["']([^"']+)["']\s*\)`)
)

// Imports implements Client by parsing TypeScript/JavaScript import syntax.
func (c *TypeScriptLanguageServerClient) Imports(_ context.Context, file string) ([]Import, error) {
	return parseTypeScriptImports(file)
}

func parseTypeScriptImports(file string) ([]Import, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("typescript import parse: open %q: %w", file, err)
	}
	defer func() {
		_ = f.Close()
	}()

	var out []Import
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		switch {
		case reJSImportFrom.MatchString(line):
			m := reJSImportFrom.FindStringSubmatch(line)
			out = append(out, Import{Path: m[1]})
		case reJSImportSide.MatchString(line):
			m := reJSImportSide.FindStringSubmatch(line)
			out = append(out, Import{Path: m[1]})
		case reJSExportFrom.MatchString(line):
			m := reJSExportFrom.FindStringSubmatch(line)
			out = append(out, Import{Path: m[1]})
		default:
			for _, m := range reJSRequireCall.FindAllStringSubmatch(line, -1) {
				if len(m) > 1 {
					out = append(out, Import{Path: m[1]})
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("typescript import parse: scan %q: %w", file, err)
	}
	return out, nil
}
