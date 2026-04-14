// Package lsp defines the common language-server interface used by codegraph.
//
// The interface is intentionally scoped to the subset of LSP capabilities that
// are available and meaningful across all target languages:
//
//   - Go      (gopls)
//   - TypeScript / JavaScript  (typescript-language-server)
//   - Java    (eclipse.jdt.ls)
//   - C / C++ (clangd)
//   - Python  (pylsp / pyright)
//
// Each language server exposes these capabilities slightly differently, but the
// semantic contract — file-position in, structured results out — is uniform
// enough to hide behind a single interface.  Language-specific quirks live
// inside the concrete adapter, not here.
package lsp

import "context"

// ─── Core position / location types ─────────────────────────────────────────

// Position is a zero-based line/character offset inside a source file.
// Both fields follow the LSP convention: line 0, character 0 is the very first
// byte of the file.
type Position struct {
	Line      uint32
	Character uint32
}

// Range is a half-open [Start, End) span inside a single file.
type Range struct {
	Start Position
	End   Position
}

// Location is a file URI plus a range within that file.
// URIs are always "file://" URIs so consumers can convert them to OS paths
// with a simple url.Parse call.
type Location struct {
	// URI is the absolute file:// URI of the source file.
	URI string
	// Range is the span that covers the symbol declaration or reference.
	Range Range
}

// ─── Symbol description ──────────────────────────────────────────────────────

// SymbolKind mirrors the LSP SymbolKind enum for the kinds that are
// universally supported.  Language-specific kinds (e.g. Go's "package",
// C++'s "namespace") are mapped to the closest match.
type SymbolKind uint8

const (
	KindUnknown     SymbolKind = 0
	KindFile        SymbolKind = 1
	KindModule      SymbolKind = 2 // package / namespace / module
	KindClass       SymbolKind = 3 // class, struct (OO languages)
	KindMethod      SymbolKind = 4 // method bound to a type
	KindFunction    SymbolKind = 5 // free / static function
	KindVariable    SymbolKind = 6 // field, global, constant
	KindInterface   SymbolKind = 7 // interface, protocol, abstract class
	KindType        SymbolKind = 8 // type alias, typedef, enum
	KindConstructor SymbolKind = 9
)

// Symbol is a named declaration returned by workspace or document symbol queries.
type Symbol struct {
	// Name is the simple (unqualified) name of the symbol.
	Name string
	// Canonical is the fully-qualified name as the language server reports it
	// (e.g. "com.example.Foo.bar" for Java, "pkg.Type.Method" for Go).
	// Empty if the server does not provide container context.
	Canonical string
	// Kind classifies the symbol.
	Kind SymbolKind
	// Location is where the symbol is declared.
	Location Location
}

// ─── Import / dependency description ────────────────────────────────────────

// Import describes a single import/include/require statement in a source file.
// The meaning of Path depends on the language:
//
//   - Go:         module path, e.g. "github.com/foo/bar"
//   - TypeScript: module specifier, e.g. "./util" or "react"
//   - Java:       fully-qualified type/package, e.g. "java.util.List"
//   - C/C++:      header path as written, e.g. "<vector>" or "mylib.h"
//   - Python:     module name, e.g. "os.path" or "numpy"
type Import struct {
	// Path is the raw import path as it appears in the source.
	Path string
	// Alias is the local alias introduced by the import statement, if any.
	// Empty when the language or statement does not provide one.
	Alias string
	// IsStdlib is true when the import refers to a standard-library module.
	// False when unknown.
	IsStdlib bool
}

// ─── Call-site description ───────────────────────────────────────────────────

// CallSite describes a single call from one symbol to another.
type CallSite struct {
	// CallerSymbol is the canonical name of the calling symbol.
	CallerSymbol string
	// CalleeSymbol is the canonical name of the called symbol.
	CalleeSymbol string
	// Location is the source position of the call expression.
	Location Location
}

// ─── Diagnostic description ─────────────────────────────────────────────────

// DiagnosticSeverity mirrors LSP DiagnosticSeverity.
type DiagnosticSeverity uint8

const (
	SeverityError       DiagnosticSeverity = 1
	SeverityWarning     DiagnosticSeverity = 2
	SeverityInformation DiagnosticSeverity = 3
	SeverityHint        DiagnosticSeverity = 4
)

// Diagnostic is a compiler/linter message attached to a source range.
type Diagnostic struct {
	Range    Range
	Severity DiagnosticSeverity
	// Code is the language-server-specific error code or rule name, e.g. "E501".
	// May be empty.
	Code    string
	Message string
	// Source identifies which tool produced this diagnostic, e.g. "pylsp" or "clangd".
	Source string
}

// ─── The common LSP interface ────────────────────────────────────────────────

// Client is the uniform interface that every language-server adapter must
// implement.  All methods are context-aware so callers can enforce timeouts.
//
// Implementations are expected to manage the connection lifecycle internally;
// Close must be called when the client is no longer needed.
//
// Method contracts:
//
//   - Methods return (nil, nil) / (empty slice, nil) when the server provides
//     no result, rather than an error.
//   - Errors are returned only for transport failures or malformed responses.
//   - File arguments are always absolute OS paths; adapters convert to/from
//     file:// URIs internally.
type Client interface {
	// ── Identity ─────────────────────────────────────────────────────────────

	// SupportedLanguages returns the file extensions this client supports and
	// their relative priority, where higher numbers win (1-100).
	//
	// Example:
	//   {".ts": 100, ".tsx": 95, ".js": 90, ".jsx": 85}
	//
	// Callers should treat extension keys case-insensitively.
	SupportedLanguages() map[string]int

	// ── Definition / declaration ──────────────────────────────────────────────

	// Definition returns the declaration location(s) for the symbol at the
	// given position.  Most servers return exactly one location; some (e.g.
	// TypeScript for overloads) may return several.
	//
	// LSP method: textDocument/definition
	Definition(ctx context.Context, file string, pos Position) ([]Location, error)

	// ── References / callers ─────────────────────────────────────────────────

	// References returns all locations in the workspace that reference the
	// symbol at pos.  includeDeclaration controls whether the declaration
	// site itself is included.
	//
	// LSP method: textDocument/references
	References(ctx context.Context, file string, pos Position, includeDeclaration bool) ([]Location, error)

	// ── Call hierarchy ────────────────────────────────────────────────────────

	// IncomingCalls returns the set of call-sites that call the function at
	// pos (i.e. callers).  Returns nil when the server does not support the
	// call-hierarchy extension.
	//
	// LSP method: callHierarchy/incomingCalls (two-step: prepareCallHierarchy
	// then callHierarchy/incomingCalls)
	IncomingCalls(ctx context.Context, file string, pos Position) ([]CallSite, error)

	// OutgoingCalls returns the set of functions called from the function at
	// pos (i.e. callees).  Returns nil when the server does not support the
	// call-hierarchy extension.
	//
	// LSP method: callHierarchy/outgoingCalls
	OutgoingCalls(ctx context.Context, file string, pos Position) ([]CallSite, error)

	// ── Symbols ───────────────────────────────────────────────────────────────

	// DocumentSymbols returns all symbols declared in the given file.  The
	// result is a flat list; hierarchy (e.g. methods inside a class) is
	// represented by the Canonical field using dot-separated names.
	//
	// LSP method: textDocument/documentSymbol
	DocumentSymbols(ctx context.Context, file string) ([]Symbol, error)

	// WorkspaceSymbols searches the entire workspace for symbols whose name
	// matches the given query string.  An empty query returns all known symbols
	// (subject to server-imposed limits).
	//
	// LSP method: workspace/symbol
	WorkspaceSymbols(ctx context.Context, query string) ([]Symbol, error)

	// ── Imports ───────────────────────────────────────────────────────────────

	// Imports returns the import/include/require statements declared in the
	// given file.  The result is derived from the document symbols or a
	// dedicated request depending on the server; adapters may parse the source
	// directly when the server does not expose imports as symbols.
	//
	// There is no single LSP method for this; adapters use whichever approach
	// is most reliable for their language.
	Imports(ctx context.Context, file string) ([]Import, error)

	// ── Hover / documentation ─────────────────────────────────────────────────

	// Hover returns the hover documentation string for the symbol at pos.
	// Returns ("", nil) when the server provides no hover information.
	//
	// LSP method: textDocument/hover
	Hover(ctx context.Context, file string, pos Position) (string, error)

	// ── Rename / canonical name ───────────────────────────────────────────────

	// CanonicalName resolves the fully-qualified canonical name for a symbol
	// at the given position.  The format is language-specific:
	//
	//   Go:         "pkg.Type.Method"
	//   TypeScript: "Module.ClassName.methodName"
	//   Java:       "com.example.ClassName.methodName"
	//   C/C++:      "Namespace::ClassName::methodName"
	//   Python:     "module.ClassName.method_name"
	//
	// Returns ("", nil) when the server cannot resolve the name.
	// Adapters typically combine Definition + DocumentSymbols to derive this.
	CanonicalName(ctx context.Context, file string, pos Position) (string, error)

	// ── Diagnostics ───────────────────────────────────────────────────────────

	// Diagnostics returns the current diagnostics (errors, warnings) for the
	// given file.  Many servers push diagnostics asynchronously; adapters that
	// cannot pull diagnostics on demand should return (nil, nil).
	//
	// LSP method: textDocument/publishDiagnostics (server-push, polled here)
	Diagnostics(ctx context.Context, file string) ([]Diagnostic, error)

	// ── Lifecycle ─────────────────────────────────────────────────────────────

	// Close shuts down the language server connection and releases all
	// resources.  Callers must not use the client after Close returns.
	//
	// LSP methods: shutdown + exit notification.
	Close() error
}
