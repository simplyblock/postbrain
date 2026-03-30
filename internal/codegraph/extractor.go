// Package codegraph extracts structural code symbols and edges from source files
// using tree-sitter, storing them as entities and relations in the graph.
package codegraph

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// SymbolKind describes the kind of a code symbol.
type SymbolKind string

const (
	KindFile      SymbolKind = "file"
	KindPackage   SymbolKind = "package"
	KindFunction  SymbolKind = "function"
	KindMethod    SymbolKind = "method"
	KindType      SymbolKind = "type"
	KindInterface SymbolKind = "interface"
	KindStruct    SymbolKind = "struct"
	KindVariable  SymbolKind = "variable"
	KindClass     SymbolKind = "class"
	KindModule    SymbolKind = "module"
)

// Symbol is a named code entity extracted from a file.
type Symbol struct {
	// Name is the canonical name as extracted (qualified where possible).
	Name string
	// Kind is the symbol kind.
	Kind SymbolKind
	// Package is the package/module name declared in the file (language-dependent).
	Package string
	// File is the source file path as provided to Extract.
	File string
}

// Edge is a directional structural relationship between two symbols (by name).
// Targets may be unresolved — they hold the raw name as it appears in the source.
type Edge struct {
	// SubjectName is the canonical name of the source symbol.
	SubjectName string
	// Predicate is the relationship kind (defines, imports, calls, uses, …).
	Predicate string
	// ObjectName is the raw target name (may be unresolved).
	ObjectName string
}

// ErrUnsupportedLanguage is returned by Extract when the file extension is not
// handled by any registered extractor.
type ErrUnsupportedLanguage struct {
	Ext string
}

func (e ErrUnsupportedLanguage) Error() string {
	return fmt.Sprintf("codegraph: unsupported language for extension %q", e.Ext)
}

// Extract dispatches to the appropriate language extractor based on the file
// extension of filename.  src must be the complete source text.
//
// Supported extensions:
//
//	.go                            → Go
//	.py                            → Python
//	.ts .tsx                       → TypeScript
//	.js .jsx .mjs .cjs             → JavaScript
//	.rs                            → Rust
//	.sh .bash .zsh                 → Bash
//	.c .h                          → C
//	.cpp .cc .cxx .hpp .hxx        → C++
//	.cs                            → C#
//	.java                          → Java
//	.kt .kts                       → Kotlin
//	.lua                           → Lua
//	.php                           → PHP
//	.rb                            → Ruby
//	.css .scss .sass .less         → CSS
//	.html .htm                     → HTML
//	Dockerfile (basename match)    → Dockerfile
//	.tf .hcl                       → HCL
//	.proto                         → Protobuf
//	.sql                           → SQL
//	.toml                          → TOML
//	.yaml .yml                     → YAML
func Extract(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	base := strings.ToLower(filepath.Base(filename))

	switch ext {
	case ".go":
		return ExtractGo(ctx, src, filename)
	case ".py":
		return ExtractPython(ctx, src, filename)
	case ".ts", ".tsx":
		return ExtractTypeScript(ctx, src, filename)
	case ".js", ".jsx", ".mjs", ".cjs":
		return ExtractJavaScript(ctx, src, filename)
	case ".rs":
		return ExtractRust(ctx, src, filename)
	case ".sh", ".bash", ".zsh":
		return ExtractBash(ctx, src, filename)
	case ".c", ".h":
		return ExtractC(ctx, src, filename)
	case ".cpp", ".cc", ".cxx", ".hpp", ".hxx":
		return ExtractCPP(ctx, src, filename)
	case ".cs":
		return ExtractCSharp(ctx, src, filename)
	case ".java":
		return ExtractJava(ctx, src, filename)
	case ".kt", ".kts":
		return ExtractKotlin(ctx, src, filename)
	case ".lua":
		return ExtractLua(ctx, src, filename)
	case ".php":
		return ExtractPHP(ctx, src, filename)
	case ".rb":
		return ExtractRuby(ctx, src, filename)
	case ".css", ".scss", ".sass", ".less":
		return ExtractCSS(ctx, src, filename)
	case ".html", ".htm":
		return ExtractHTML(ctx, src, filename)
	case ".tf", ".hcl":
		return ExtractHCL(ctx, src, filename)
	case ".proto":
		return ExtractProtobuf(ctx, src, filename)
	case ".sql":
		return ExtractSQL(ctx, src, filename)
	case ".toml":
		return ExtractTOML(ctx, src, filename)
	case ".yaml", ".yml":
		return ExtractYAML(ctx, src, filename)
	}

	// Basename matches for files without extensions.
	if base == "dockerfile" || strings.HasPrefix(base, "dockerfile.") {
		return ExtractDockerfile(ctx, src, filename)
	}

	return nil, nil, ErrUnsupportedLanguage{Ext: ext}
}

// SupportedExtensions returns all file extensions that Extract can handle.
func SupportedExtensions() []string {
	return []string{
		".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".rs",
		".sh", ".bash", ".zsh",
		".c", ".h", ".cpp", ".cc", ".cxx", ".hpp", ".hxx",
		".cs", ".java", ".kt", ".kts",
		".lua", ".php", ".rb",
		".css", ".scss", ".sass", ".less",
		".html", ".htm",
		".tf", ".hcl", ".proto", ".sql", ".toml", ".yaml", ".yml",
	}
}