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
// stream by tokenising the entire stream and tracking string operands that
// precede the text-showing operators Tj, TJ, ' and ".
//
// Two common failure modes of a line-based approach are avoided:
//   - strings and their operator on different lines  →  handled by full-stream scan
//   - TJ arrays interleaving strings with kerning numbers  →  numbers are skipped
//     without discarding already-collected string operands
func parsePDFContentStream(stream []byte) string {
	var out strings.Builder
	var pending []string // string operands accumulated before the next operator

	data := strings.TrimSpace(string(stream))
	for data != "" {
		c := data[0]

		// Literal string ( … )
		if c == '(' {
			s, tail, ok := nextPDFString(data)
			if ok {
				pending = append(pending, s)
				data = strings.TrimSpace(tail)
			} else {
				data = data[1:]
			}
			continue
		}

		// Hex string < … >  (but not dictionary << … >>)
		if c == '<' && (len(data) < 2 || data[1] != '<') {
			s, tail, ok := nextPDFString(data)
			if ok {
				pending = append(pending, s)
				data = strings.TrimSpace(tail)
			} else {
				data = data[1:]
			}
			continue
		}

		// Dictionary << … >> — skip to matching >>
		if c == '<' && len(data) >= 2 && data[1] == '<' {
			end := strings.Index(data[2:], ">>")
			if end >= 0 {
				data = strings.TrimSpace(data[end+4:])
			} else {
				data = ""
			}
			continue
		}

		// Array brackets — just consume; inner strings are picked up naturally
		if c == '[' || c == ']' {
			data = data[1:]
			continue
		}

		// Comment — skip to end of line
		if c == '%' {
			end := strings.IndexAny(data, "\r\n")
			if end < 0 {
				data = ""
			} else {
				data = data[end+1:]
			}
			continue
		}

		// Whitespace
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			data = strings.TrimLeft(data, " \t\r\n")
			continue
		}

		// Read a bare token (operator, number, name starting with /)
		i := strings.IndexAny(data, " \t\r\n([<]>%")
		var token string
		if i < 0 {
			token, data = data, ""
		} else {
			token, data = data[:i], strings.TrimSpace(data[i:])
		}
		if token == "" {
			continue
		}

		switch token {
		case "Tj", "'", "\"":
			if len(pending) > 0 {
				if out.Len() > 0 {
					out.WriteByte(' ')
				}
				out.WriteString(pending[len(pending)-1])
				pending = pending[:0]
			}
		case "TJ":
			for _, s := range pending {
				if s != "" {
					if out.Len() > 0 {
						out.WriteByte(' ')
					}
					out.WriteString(s)
				}
			}
			pending = pending[:0]
		default:
			// Numbers and PDF names (/Foo) sit between strings in TJ arrays
			// or as operator arguments — leave pending intact.
			// Any other alphabetic operator marks the end of a text operand group.
			if !isPDFNumberOrName(token) {
				pending = pending[:0]
			}
		}
	}
	return strings.TrimSpace(out.String())
}

// isPDFNumberOrName returns true for tokens that can appear between string
// operands without ending the operand group: numeric values and PDF names (/Foo).
func isPDFNumberOrName(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == '/' {
		return true
	}
	for i, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c == '.' || c == '-' || c == '+':
			if (c == '-' || c == '+') && i != 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
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
