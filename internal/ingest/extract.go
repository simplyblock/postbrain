// Package ingest provides text extraction from uploaded documents.
package ingest

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
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
	rs := bytes.NewReader(data)
	ctx, err := pdfapi.ReadAndValidate(rs, nil)
	if err != nil {
		return "", fmt.Errorf("ingest: open pdf: %w", err)
	}

	var buf strings.Builder
	for i := 1; i <= ctx.PageCount; i++ {
		r, err := pdfcpu.ExtractPageContent(ctx, i)
		if err != nil || r == nil {
			continue
		}
		raw, err := io.ReadAll(r)
		if err != nil {
			continue
		}
		text := parsePDFContentStream(raw)
		if buf.Len() > 0 && text != "" {
			buf.WriteByte('\n')
		}
		buf.WriteString(text)
	}
	return strings.TrimSpace(buf.String()), nil
}

// parsePDFContentStream extracts human-readable text from a raw PDF content
// stream by scanning for the text-showing operators Tj, TJ, ' and ".
// It handles literal strings ( ) and hex strings < >.
func parsePDFContentStream(stream []byte) string {
	var out strings.Builder
	sc := bufio.NewScanner(bytes.NewReader(stream))
	sc.Buffer(make([]byte, 1<<20), 1<<20)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		// Collect all string operands on this line, then check the operator.
		var strs []string
		rest := line
		for {
			s, tail, ok := nextPDFString(rest)
			if !ok {
				break
			}
			strs = append(strs, s)
			rest = strings.TrimSpace(tail)
		}

		op := strings.TrimSpace(rest)
		switch op {
		case "Tj", "'", "\"":
			if len(strs) > 0 {
				if out.Len() > 0 {
					out.WriteByte(' ')
				}
				out.WriteString(strs[len(strs)-1])
			}
		case "TJ":
			// strs already contains the array elements in order.
			for _, s := range strs {
				if s != "" {
					if out.Len() > 0 {
						out.WriteByte(' ')
					}
					out.WriteString(s)
				}
			}
		}
	}
	return strings.TrimSpace(out.String())
}

// nextPDFString parses the first PDF string literal from s.
// Returns the decoded string, the remainder of s after the string, and true on success.
// Handles ( literal ) and < hex > forms. Bracket arrays [ ] are unwrapped transparently.
func nextPDFString(s string) (string, string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", s, false
	}
	switch s[0] {
	case '[':
		// Array: recurse to consume all strings inside, ignore numbers/offsets.
		return nextPDFString(s[1:])
	case ']':
		return "", s[1:], false
	case '(':
		// Literal string: scan for matching closing paren, honouring backslash escapes.
		depth := 0
		for i := 0; i < len(s); i++ {
			switch s[i] {
			case '\\':
				i++ // skip escaped byte
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					decoded := decodePDFLiteral(s[1:i])
					return decoded, s[i+1:], true
				}
			}
		}
		return "", "", false
	case '<':
		// Hex string.
		end := strings.Index(s, ">")
		if end < 0 {
			return "", "", false
		}
		hex := s[1:end]
		decoded := decodeHexString(hex)
		return decoded, s[end+1:], true
	default:
		// Not a string token (number, name, operator) — skip one token.
		i := strings.IndexAny(s, " \t\r\n([<")
		if i < 0 {
			return "", "", false
		}
		return "", s[i:], false
	}
}

// decodePDFLiteral interprets backslash escape sequences in a PDF literal string.
func decodePDFLiteral(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '(', ')', '\\':
			b.WriteByte(s[i])
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// decodeHexString converts a PDF hex string (pairs of hex digits) to its string value.
func decodeHexString(hex string) string {
	hex = strings.ReplaceAll(hex, " ", "")
	hex = strings.ReplaceAll(hex, "\n", "")
	if len(hex)%2 != 0 {
		hex += "0"
	}
	var b strings.Builder
	for i := 0; i+1 < len(hex); i += 2 {
		hi := hexNibble(hex[i])
		lo := hexNibble(hex[i+1])
		if hi < 0 || lo < 0 {
			continue
		}
		b.WriteByte(byte(hi<<4 | lo))
	}
	return b.String()
}

func hexNibble(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
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
