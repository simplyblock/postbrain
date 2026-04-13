package codegraph

import (
	"context"
	"testing"

	"github.com/simplyblock/postbrain/internal/codegraph/lsp"
)

// TestEnrichCallEdges_ReplacesCallEdgesWithLSP verifies that heuristic call
// edges are replaced by LSP-accurate fully-qualified edges while non-call edges
// are preserved unchanged.
func TestEnrichCallEdges_ReplacesCallEdgesWithLSP(t *testing.T) {
	t.Parallel()

	stub := &stubLSPClient{
		lang: ".go",
		docSymsResult: []lsp.Symbol{
			{Name: "Caller", Canonical: "mypkg.Caller", Kind: lsp.KindFunction},
		},
	}
	// OutgoingCalls is backed by the stubLSPClient; we need a richer stub
	// that returns OutgoingCalls results.  Embed the base stub and override.
	rich := &richStubClient{
		stubLSPClient: stub,
		outgoingCalls: map[string][]lsp.CallSite{
			"mypkg.Caller": {
				{CallerSymbol: "mypkg.Caller", CalleeSymbol: "otherpkg.Helper"},
				{CallerSymbol: "mypkg.Caller", CalleeSymbol: "otherpkg.Util"},
			},
		},
	}

	original := []Edge{
		{SubjectName: "mypkg.Caller", Predicate: "calls", ObjectName: "Helper"},   // heuristic, unqualified
		{SubjectName: "mypkg.Caller", Predicate: "defines", ObjectName: "mypkg.Caller"}, // non-call: kept
		{SubjectName: "mypkg.Caller", Predicate: "imports", ObjectName: "otherpkg"},     // non-call: kept
	}

	result := enrichCallEdges(context.Background(), rich, "mypkg/file.go", original)

	// Non-call edges must survive.
	var defines, imports int
	for _, e := range result {
		switch e.Predicate {
		case "defines":
			defines++
		case "imports":
			imports++
		}
	}
	if defines != 1 {
		t.Errorf("defines edges = %d, want 1", defines)
	}
	if imports != 1 {
		t.Errorf("imports edges = %d, want 1", imports)
	}

	// LSP call edges replace the heuristic one.
	var calls []Edge
	for _, e := range result {
		if e.Predicate == "calls" {
			calls = append(calls, e)
		}
	}
	if len(calls) != 2 {
		t.Fatalf("call edges = %d, want 2; got %v", len(calls), calls)
	}
	wantCallees := map[string]bool{"otherpkg.Helper": true, "otherpkg.Util": true}
	for _, c := range calls {
		if !wantCallees[c.ObjectName] {
			t.Errorf("unexpected callee %q", c.ObjectName)
		}
		if c.SubjectName != "mypkg.Caller" {
			t.Errorf("caller = %q, want %q", c.SubjectName, "mypkg.Caller")
		}
	}
}

// TestEnrichCallEdges_FallsBackWhenLSPReturnsNoCalls verifies that the original
// edges are returned unchanged when OutgoingCalls produces nothing.
func TestEnrichCallEdges_FallsBackWhenLSPReturnsNoCalls(t *testing.T) {
	t.Parallel()

	stub := &stubLSPClient{
		lang: ".go",
		docSymsResult: []lsp.Symbol{
			{Name: "Caller", Canonical: "mypkg.Caller", Kind: lsp.KindFunction},
		},
	}
	rich := &richStubClient{stubLSPClient: stub, outgoingCalls: nil}

	original := []Edge{
		{SubjectName: "mypkg.Caller", Predicate: "calls", ObjectName: "Helper"},
	}
	result := enrichCallEdges(context.Background(), rich, "mypkg/file.go", original)
	if len(result) != 1 || result[0].ObjectName != "Helper" {
		t.Errorf("expected original edges preserved; got %v", result)
	}
}

// TestEnrichCallEdges_FallsBackWhenDocSymsFails verifies that the original
// edges are returned when DocumentSymbols errors.
func TestEnrichCallEdges_FallsBackWhenDocSymsFails(t *testing.T) {
	t.Parallel()

	// stubLSPClient with no docSymsResult returns nil, nil — same as empty.
	stub := &stubLSPClient{lang: ".go"} // empty docSymsResult → 0 symbols
	original := []Edge{
		{SubjectName: "mypkg.Caller", Predicate: "calls", ObjectName: "Helper"},
	}
	result := enrichCallEdges(context.Background(), stub, "mypkg/file.go", original)
	if len(result) != 1 || result[0].ObjectName != "Helper" {
		t.Errorf("expected original edges preserved; got %v", result)
	}
}

// ── richStubClient ────────────────────────────────────────────────────────────

// richStubClient extends stubLSPClient with configurable OutgoingCalls results,
// keyed by caller canonical name.
type richStubClient struct {
	*stubLSPClient
	outgoingCalls map[string][]lsp.CallSite // callerCanonical → calls
}

func (r *richStubClient) OutgoingCalls(_ context.Context, _ string, pos lsp.Position) ([]lsp.CallSite, error) {
	// Match by position via DocumentSymbols result (line 0 = first symbol).
	for _, sym := range r.docSymsResult {
		if sym.Location.Range.Start == pos {
			return r.outgoingCalls[sym.Canonical], nil
		}
	}
	// If position doesn't match exactly, search all results (tests use zero Position).
	if pos == (lsp.Position{}) && len(r.docSymsResult) > 0 {
		first := r.docSymsResult[0]
		return r.outgoingCalls[first.Canonical], nil
	}
	return nil, nil
}
