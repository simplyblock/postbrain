// Package ingest provides text extraction from uploaded documents.
package ingest

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	dslipakpdf "github.com/dslipak/pdf"
)

// ErrUnsupportedFormat is returned when the file extension is not supported.
var ErrUnsupportedFormat = errors.New("ingest: unsupported file format")

// Extract extracts plain text from file data based on the filename extension.
// Supported extensions: .txt, .md, .pdf, .docx
func Extract(filename string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".txt", ".md":
		return string(data), nil
	case ".pdf":
		return extractPDF(data)
	case ".docx":
		return extractDOCX(data)
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedFormat, ext)
	}
}

func extractPDF(data []byte) (string, error) {
	r, err := dslipakpdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("ingest: open pdf: %w", err)
	}
	var buf strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		text, err := r.Page(i).GetPlainText(nil)
		if err != nil || text == "" {
			continue
		}
		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(text)
	}
	return strings.TrimSpace(buf.String()), nil
}

func extractDOCX(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("ingest: open docx: %w", err)
	}
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("ingest: open document.xml: %w", err)
		}
		defer rc.Close()
		content, err := io.ReadAll(rc)
		if err != nil {
			return "", fmt.Errorf("ingest: read document.xml: %w", err)
		}
		return extractDocxText(content), nil
	}
	return "", fmt.Errorf("ingest: word/document.xml not found in docx archive")
}

// extractDocxText walks the XML token stream and collects text from <w:t> elements,
// inserting newlines between <w:p> paragraphs.
func extractDocxText(xmlData []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(xmlData))
	var buf strings.Builder
	inT := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "p" && buf.Len() > 0 {
				buf.WriteByte('\n')
			}
			inT = t.Name.Local == "t"
		case xml.EndElement:
			if t.Name.Local == "t" {
				inT = false
			}
		case xml.CharData:
			if inT {
				buf.Write(t)
			}
		}
	}
	return strings.TrimSpace(buf.String())
}
