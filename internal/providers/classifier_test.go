package providers

import "testing"

func TestClassifyContent(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		sourceRef string
		want      string
	}{
		{
			name:      "file source_ref with Go extension",
			content:   "package main",
			sourceRef: "file:src/auth/jwt.go:42",
			want:      "code",
		},
		{
			name:      "file source_ref with Python extension",
			content:   "import os",
			sourceRef: "file:main.py:1",
			want:      "code",
		},
		{
			name:      "file source_ref with Markdown extension",
			content:   "# Hello world",
			sourceRef: "file:README.md:1",
			want:      "text",
		},
		{
			name:      "conversation source_ref falls through to content heuristic (prose)",
			content:   "This is a long prose sentence about nothing in particular. It has many words and no code patterns.",
			sourceRef: "conversation:abc",
			want:      "text",
		},
		{
			name: "content that is mostly prose",
			content: `The quick brown fox jumps over the lazy dog.
This is a normal sentence.
Another sentence here.
And yet another one.
No code at all.`,
			sourceRef: "",
			want:      "text",
		},
		{
			name: "content that is mostly Go code",
			content: `package main

import "fmt"

func main() {
	var x int
	const y = 42
	return
}`,
			sourceRef: "",
			want:      "code",
		},
		{
			name:      "empty source_ref falls through to content heuristic (prose)",
			content:   "Just some plain text with no code patterns at all.",
			sourceRef: "",
			want:      "text",
		},
		{
			name:      "empty source_ref falls through to content heuristic (code)",
			content:   "func foo() {\n\tvar x = 1\n\treturn x\n}\nfunc bar() {\n\tconst z = 2\n}",
			sourceRef: "",
			want:      "code",
		},
		{
			name:      "file source_ref with TypeScript extension",
			content:   "const x = 1;",
			sourceRef: "file:app/index.ts:10",
			want:      "code",
		},
		{
			name:      "file source_ref with unknown extension falls back to content heuristic (prose)",
			content:   "Hello this is plain text without code.",
			sourceRef: "file:notes.txt:1",
			want:      "text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyContent(tt.content, tt.sourceRef)
			if got != tt.want {
				t.Errorf("ClassifyContent(%q, %q) = %q; want %q", tt.content, tt.sourceRef, got, tt.want)
			}
		})
	}
}
