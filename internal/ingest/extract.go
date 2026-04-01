// Package ingest provides text extraction from uploaded documents.
package ingest

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrUnsupportedFormat is returned when the file extension is not supported.
var ErrUnsupportedFormat = errors.New("ingest: unsupported file format")

// markitdownExts lists file extensions delegated to the markitdown CLI.
var markitdownExts = map[string]bool{
	".pptx": true,
	".docx": true,
	".xlsx": true,
	".xls":  true,
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".bmp":  true,
	".webp": true,
	".pdf":  true,
}

// Extract extracts plain text from file data based on the filename extension.
// Supported natively: .txt, .md, .pdf, .docx
// Delegated to markitdown CLI: .pptx, .xlsx, .xls, .png, .jpg, .jpeg, .gif, .bmp, .webp
func Extract(filename string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".txt", ".md":
		return string(data), nil
	default:
		if markitdownExts[ext] {
			return extractViaMarkitdown(data, ext)
		}
		return "", fmt.Errorf("%w: %s", ErrUnsupportedFormat, ext)
	}
}

// extractViaMarkitdown writes data to a temp file and invokes the markitdown
// CLI, returning its stdout as markdown text.
func extractViaMarkitdown(data []byte, ext string) (string, error) {
	tmp, err := os.CreateTemp("", "postbrain-*"+ext)
	if err != nil {
		return "", fmt.Errorf("ingest: markitdown temp file: %w", err)
	}
	defer func() {
		_ = os.Remove(tmp.Name())
	}()

	if _, err := tmp.Write(data); err != nil {
		if closeErr := tmp.Close(); closeErr != nil {
			return "", fmt.Errorf("ingest: markitdown close temp after write failure: %w", closeErr)
		}
		return "", fmt.Errorf("ingest: markitdown write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("ingest: markitdown close temp: %w", err)
	}

	out, err := exec.Command("markitdown", tmp.Name()).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("ingest: markitdown: %w: %s", err, exitErr.Stderr)
		}
		if errors.Is(err, exec.ErrNotFound) {
			return "", fmt.Errorf("ingest: markitdown not found in PATH; install with: pip install markitdown")
		}
		return "", fmt.Errorf("ingest: markitdown: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}
