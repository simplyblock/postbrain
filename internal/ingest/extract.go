// Package ingest provides text extraction from uploaded documents.
package ingest

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
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
// Supported natively: .txt, .md, .docx
// Delegated to markitdown CLI: .pptx, .xlsx, .xls, .png, .jpg, .jpeg, .gif, .bmp, .webp, .pdf
func Extract(filename string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".txt", ".md":
		return string(data), nil
	case ".docx":
		return extractDocx(data)
	default:
		if markitdownExts[ext] {
			return extractViaMarkitdown(data, ext)
		}
		return "", fmt.Errorf("%w: %s", ErrUnsupportedFormat, ext)
	}
}

// extractDocx extracts plain text from a DOCX file (Office Open XML zip format).
func extractDocx(data []byte) (string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("ingest: docx: open zip: %w", err)
	}

	for _, f := range r.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("ingest: docx: open document.xml: %w", err)
		}
		defer func() {
			_ = rc.Close()
		}()
		return extractDocxText(rc)
	}
	return "", fmt.Errorf("ingest: docx: word/document.xml not found")
}

// extractDocxText reads plain text from an Open XML document stream by
// collecting all w:t element contents.
func extractDocxText(r io.Reader) (string, error) {
	dec := xml.NewDecoder(r)
	var sb strings.Builder
	inText := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("ingest: docx: parse xml: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "t" {
				inText = true
			}
		case xml.EndElement:
			if t.Name.Local == "t" {
				inText = false
			}
		case xml.CharData:
			if inText {
				sb.Write(t)
			}
		}
	}
	return sb.String(), nil
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
