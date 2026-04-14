package lsp

import (
	"encoding/json"
	"testing"
)

func TestDecodeSymbolInformation_DocumentSymbolUsesSelectionRange(t *testing.T) {
	raw := []byte(`[
		{
			"name":"Caller",
			"kind":12,
			"range":{"start":{"line":2,"character":0},"end":{"line":2,"character":24}},
			"selectionRange":{"start":{"line":2,"character":5},"end":{"line":2,"character":11}}
		}
	]`)

	syms, err := decodeSymbolInformation(json.RawMessage(raw), "file:///tmp/z_caller.go")
	if err != nil {
		t.Fatalf("decode symbols: %v", err)
	}
	if len(syms) != 1 {
		t.Fatalf("len(symbols) = %d, want 1", len(syms))
	}
	if syms[0].Location.Range.Start.Line != 2 || syms[0].Location.Range.Start.Character != 5 {
		t.Fatalf("start pos = (%d,%d), want (2,5)",
			syms[0].Location.Range.Start.Line,
			syms[0].Location.Range.Start.Character,
		)
	}
}

func TestRangeContains_EndBoundaryIsExclusive(t *testing.T) {
	r := Range{
		Start: Position{Line: 10, Character: 3},
		End:   Position{Line: 10, Character: 7},
	}
	if !rangeContains(r, Position{Line: 10, Character: 6}) {
		t.Fatal("expected position before end to be contained")
	}
	if rangeContains(r, Position{Line: 10, Character: 7}) {
		t.Fatal("expected end boundary to be excluded")
	}
}
