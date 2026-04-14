package codegraph

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/codegraph/lsp"
)

// ── stubLSPClient ─────────────────────────────────────────────────────────────

// stubLSPClient is a configurable lsp.Client stub for unit tests.
// Each method records whether it was called and returns the pre-configured result.
type stubLSPClient struct {
	languages map[string]int

	docSymsResult []lsp.Symbol
	docSymsCalled bool

	wsSymsResult map[string][]lsp.Symbol // query → results
	wsSymsCalled bool

	importsResult map[string][]lsp.Import // absFile → imports
	importsCalled bool

	canonicalResult map[string]string // absFile → canonical
	canonicalCalled bool
}

func (s *stubLSPClient) SupportedLanguages() map[string]int {
	out := make(map[string]int, len(s.languages))
	for ext, prio := range s.languages {
		out[ext] = prio
	}
	return out
}

func (s *stubLSPClient) DocumentSymbols(_ context.Context, _ string) ([]lsp.Symbol, error) {
	s.docSymsCalled = true
	return s.docSymsResult, nil
}

func (s *stubLSPClient) WorkspaceSymbols(_ context.Context, query string) ([]lsp.Symbol, error) {
	s.wsSymsCalled = true
	return s.wsSymsResult[query], nil
}

func (s *stubLSPClient) Imports(_ context.Context, absFile string) ([]lsp.Import, error) {
	s.importsCalled = true
	return s.importsResult[absFile], nil
}

func (s *stubLSPClient) CanonicalName(_ context.Context, absFile string, _ lsp.Position) (string, error) {
	s.canonicalCalled = true
	return s.canonicalResult[absFile], nil
}

func (s *stubLSPClient) Definition(context.Context, string, lsp.Position) ([]lsp.Location, error) {
	return nil, nil
}
func (s *stubLSPClient) References(context.Context, string, lsp.Position, bool) ([]lsp.Location, error) {
	return nil, nil
}
func (s *stubLSPClient) IncomingCalls(context.Context, string, lsp.Position) ([]lsp.CallSite, error) {
	return nil, nil
}
func (s *stubLSPClient) OutgoingCalls(context.Context, string, lsp.Position) ([]lsp.CallSite, error) {
	return nil, nil
}
func (s *stubLSPClient) Hover(context.Context, string, lsp.Position) (string, error) { return "", nil }
func (s *stubLSPClient) Diagnostics(context.Context, string) ([]lsp.Diagnostic, error) {
	return nil, nil
}
func (s *stubLSPClient) Close() error { return nil }

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestResolver_Resolve_LSP_SkipsDifferentLanguage verifies that resolveViaLSP
// is a no-op when the file extension does not match the client language.
func TestResolver_Resolve_LSP_SkipsDifferentLanguage(t *testing.T) {
	t.Parallel()

	stub := &stubLSPClient{languages: map[string]int{".go": 100}}
	r := NewResolver(nil, uuid.New(), stub, "")

	_, ok := r.Resolve(context.Background(), "some/file.py", "Target", nil)
	if ok {
		t.Fatal("expected unresolved result for wrong language")
	}
	if stub.docSymsCalled || stub.wsSymsCalled || stub.importsCalled {
		t.Fatal("expected no LSP calls for a non-.go file")
	}
}

// TestResolver_Resolve_LSP_LocalHitSkipsLSP verifies that a local symbol table
// hit short-circuits before any LSP calls are made.
func TestResolver_Resolve_LSP_LocalHitSkipsLSP(t *testing.T) {
	t.Parallel()

	stub := &stubLSPClient{languages: map[string]int{".go": 100}}
	r := NewResolver(nil, uuid.New(), stub, "")

	want := uuid.New()
	_, ok := r.Resolve(context.Background(), "some/file.go", "Target", map[string]uuid.UUID{"Target": want})
	if !ok {
		t.Fatal("expected local hit")
	}
	if stub.docSymsCalled || stub.wsSymsCalled {
		t.Fatal("expected no LSP calls when local symbol table hits")
	}
}

// TestResolver_Resolve_LSP_UsesWorkspaceSymbols verifies that resolveViaLSP
// calls WorkspaceSymbols and uses the returned canonical name.  Without a DB
// the canonical lookup fails, so Resolve returns ok=false; but we can verify
// the right methods were invoked.
func TestResolver_Resolve_LSP_UsesWorkspaceSymbols(t *testing.T) {
	t.Parallel()

	stub := &stubLSPClient{
		languages: map[string]int{".go": 100},
		wsSymsResult: map[string][]lsp.Symbol{
			"Target": {{Name: "Target", Canonical: "pkg.Target", Kind: lsp.KindFunction}},
		},
		// Imports returns "pkg" so the workspace symbol is accepted.
		importsResult: map[string][]lsp.Import{
			"some/file.go": {{Path: "pkg"}},
		},
	}
	r := NewResolver(nil, uuid.New(), stub, "")

	_, ok := r.Resolve(context.Background(), "some/file.go", "Target", nil)
	// ok=false is expected because there is no DB to complete the canonical lookup.
	if ok {
		t.Fatal("expected unresolved result without DB-backed canonical lookup")
	}
	if !stub.wsSymsCalled {
		t.Fatal("expected WorkspaceSymbols to be called")
	}
	if !stub.importsCalled {
		t.Fatal("expected Imports to be called for cross-package filtering")
	}
}

// TestResolver_Resolve_LSP_UsesDocumentSymbols verifies that resolveViaLSP
// tries DocumentSymbols first when the target symbol is declared in the file.
func TestResolver_Resolve_LSP_UsesDocumentSymbols(t *testing.T) {
	t.Parallel()

	stub := &stubLSPClient{
		languages: map[string]int{".go": 100},
		docSymsResult: []lsp.Symbol{
			{Name: "Target", Canonical: "mypkg.Target", Kind: lsp.KindFunction,
				Location: lsp.Location{URI: "file:///some/file.go"}},
		},
		canonicalResult: map[string]string{
			"some/file.go": "mypkg.Target",
		},
	}
	r := NewResolver(nil, uuid.New(), stub, "")

	_, ok := r.Resolve(context.Background(), "some/file.go", "Target", nil)
	if ok {
		t.Fatal("expected unresolved without DB")
	}
	if !stub.docSymsCalled {
		t.Fatal("expected DocumentSymbols to be called")
	}
	if !stub.canonicalCalled {
		t.Fatal("expected CanonicalName to be called for in-file declaration")
	}
}

// TestResolver_Resolve_LSP_PrefersCurrentPackageCanonical verifies that when
// the target is unqualified and no workspace symbol match is available, the
// resolver derives the current package from document symbols and tries
// "<pkg>.<target>" before falling back to suffix matching.
func TestResolver_Resolve_LSP_PrefersCurrentPackageCanonical(t *testing.T) {
	t.Parallel()

	stub := &stubLSPClient{
		languages: map[string]int{".go": 100},
		docSymsResult: []lsp.Symbol{
			{Name: "Caller", Canonical: "longpkg.Caller", Kind: lsp.KindFunction},
		},
		wsSymsResult: map[string][]lsp.Symbol{
			"Target": nil,
		},
	}
	r := NewResolver(nil, uuid.New(), stub, "")

	want := uuid.New()
	got, ok := r.Resolve(context.Background(), "some/file.go", "Target", map[string]uuid.UUID{
		"longpkg.Target": want,
	})
	if !ok {
		t.Fatal("expected canonical resolution via current package")
	}
	if got != want {
		t.Fatalf("resolved id = %s, want %s", got, want)
	}
}

// TestResolver_Resolve_LSP_SupportsAliasExtension verifies that LSP resolution
// is attempted for alias extensions when the client advertises support.
func TestResolver_Resolve_LSP_SupportsAliasExtension(t *testing.T) {
	t.Parallel()

	stub := &stubLSPClient{
		languages: map[string]int{
			".c":   100,
			".cpp": 90,
		},
		docSymsResult: []lsp.Symbol{
			{Name: "Target", Canonical: "cppkg.Target", Kind: lsp.KindFunction},
		},
		canonicalResult: map[string]string{
			"repo/file.cpp": "cppkg.Target",
		},
	}
	r := NewResolver(nil, uuid.New(), stub, "")

	_, _ = r.Resolve(context.Background(), "repo/file.cpp", "Target", nil)
	if !stub.docSymsCalled {
		t.Fatal("expected DocumentSymbols to be called for .cpp alias extension")
	}
}
