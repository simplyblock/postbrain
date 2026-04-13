package ingest_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"github.com/simplyblock/postbrain/internal/ingest"
)

func TestExtractTxt(t *testing.T) {
	text, err := ingest.Extract("hello.txt", []byte("hello world"))
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello world" {
		t.Errorf("got %q, want %q", text, "hello world")
	}
}

func TestExtractMarkdown(t *testing.T) {
	md := "# Title\n\nSome content."
	text, err := ingest.Extract("README.md", []byte(md))
	if err != nil {
		t.Fatal(err)
	}
	if text != md {
		t.Errorf("got %q", text)
	}
}

func TestExtractDocx(t *testing.T) {
	data := makeDocx("Hello from DOCX")
	text, err := ingest.Extract("doc.docx", data)
	if err != nil {
		t.Fatal(err)
	}
	if text != "Hello from DOCX" {
		t.Errorf("got %q", text)
	}
}

func TestExtractMarkitdownNotInstalled(t *testing.T) {
	if _, err := exec.LookPath("markitdown"); err == nil {
		t.Skip("markitdown is installed; skipping not-installed test")
	}
	_, err := ingest.Extract("file.pptx", []byte("data"))
	if err == nil {
		t.Fatal("expected error when markitdown not in PATH")
	}
}

func TestExtractUnsupported(t *testing.T) {
	_, err := ingest.Extract("file.xyz", []byte("data"))
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
	if !errors.Is(err, ingest.ErrUnsupportedFormat) {
		t.Errorf("expected ErrUnsupportedFormat, got %v", err)
	}
}

// makeDocx creates a minimal valid DOCX (zip with word/document.xml).
func makeDocx(text string) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("word/document.xml")
	if err != nil {
		panic(err)
	}
	if _, err := fmt.Fprintf(f, `<?xml version="1.0"?>`+
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`+
		`<w:body><w:p><w:r><w:t>%s</w:t></w:r></w:p></w:body></w:document>`, text); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
