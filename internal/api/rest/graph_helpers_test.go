package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/graph"
)

// ── traversalResult ───────────────────────────────────────────────────────────

func TestTraversalResult_NoNeighbours(t *testing.T) {
	t.Parallel()
	entityID := uuid.New()
	res := &graph.TraversalResult{
		Entity: &db.Entity{
			ID:         entityID,
			Name:       "MyFunc",
			Canonical:  "pkg.MyFunc",
			EntityType: "function",
		},
		Neighbours: nil,
	}

	out := traversalResult(res)

	if out.ID != entityID {
		t.Errorf("ID = %v, want %v", out.ID, entityID)
	}
	if out.Name != "MyFunc" {
		t.Errorf("Name = %q, want %q", out.Name, "MyFunc")
	}
	if out.Canonical != "pkg.MyFunc" {
		t.Errorf("Canonical = %q, want %q", out.Canonical, "pkg.MyFunc")
	}
	if out.Type != "function" {
		t.Errorf("Type = %q, want %q", out.Type, "function")
	}
	if len(out.Neighbours) != 0 {
		t.Errorf("Neighbours len = %d, want 0", len(out.Neighbours))
	}
}

func TestTraversalResult_BothDirections(t *testing.T) {
	t.Parallel()
	entityID := uuid.New()
	callerID := uuid.New()
	calleeID := uuid.New()
	sf := "internal/pkg/service.go"

	res := &graph.TraversalResult{
		Entity: &db.Entity{
			ID:         entityID,
			Name:       "Handle",
			Canonical:  "server.Handle",
			EntityType: "method",
		},
		Neighbours: []graph.Neighbour{
			{
				Entity: &db.Entity{
					ID:         callerID,
					Name:       "Caller",
					Canonical:  "server.Caller",
					EntityType: "function",
				},
				Predicate:  "calls",
				Direction:  "incoming",
				Confidence: 0.9,
				SourceFile: &sf,
			},
			{
				Entity: &db.Entity{
					ID:         calleeID,
					Name:       "Callee",
					Canonical:  "server.Callee",
					EntityType: "function",
				},
				Predicate:  "calls",
				Direction:  "outgoing",
				Confidence: 0.75,
				SourceFile: nil,
			},
		},
	}

	out := traversalResult(res)

	if out.ID != entityID {
		t.Errorf("ID = %v, want %v", out.ID, entityID)
	}
	if len(out.Neighbours) != 2 {
		t.Fatalf("Neighbours len = %d, want 2", len(out.Neighbours))
	}

	incoming := out.Neighbours[0]
	if incoming.ID != callerID {
		t.Errorf("incoming ID = %v, want %v", incoming.ID, callerID)
	}
	if incoming.Direction != "incoming" {
		t.Errorf("incoming Direction = %q, want %q", incoming.Direction, "incoming")
	}
	if incoming.Confidence != 0.9 {
		t.Errorf("incoming Confidence = %v, want 0.9", incoming.Confidence)
	}
	if incoming.SourceFile == nil || *incoming.SourceFile != sf {
		t.Errorf("incoming SourceFile = %v, want %q", incoming.SourceFile, sf)
	}

	outgoing := out.Neighbours[1]
	if outgoing.ID != calleeID {
		t.Errorf("outgoing ID = %v, want %v", outgoing.ID, calleeID)
	}
	if outgoing.Direction != "outgoing" {
		t.Errorf("outgoing Direction = %q, want %q", outgoing.Direction, "outgoing")
	}
	if outgoing.SourceFile != nil {
		t.Errorf("outgoing SourceFile = %v, want nil", outgoing.SourceFile)
	}
}

// ── scopeAndSymbol ────────────────────────────────────────────────────────────

func TestScopeAndSymbol(t *testing.T) {
	t.Parallel()
	validID := uuid.New()

	tests := []struct {
		name       string
		query      string
		wantOK     bool
		wantID     uuid.UUID
		wantSymbol string
	}{
		{
			name:       "valid scope_id and symbol",
			query:      "scope_id=" + validID.String() + "&symbol=MyFunc",
			wantOK:     true,
			wantID:     validID,
			wantSymbol: "MyFunc",
		},
		{
			name:   "missing scope_id",
			query:  "symbol=MyFunc",
			wantOK: false,
		},
		{
			name:   "missing symbol",
			query:  "scope_id=" + validID.String(),
			wantOK: false,
		},
		{
			name:   "invalid scope_id UUID",
			query:  "scope_id=not-a-uuid&symbol=MyFunc",
			wantOK: false,
		},
		{
			name:   "both missing",
			query:  "",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/?"+tt.query, nil)
			id, symbol, ok := scopeAndSymbol(req)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if id != tt.wantID {
				t.Errorf("id = %v, want %v", id, tt.wantID)
			}
			if symbol != tt.wantSymbol {
				t.Errorf("symbol = %q, want %q", symbol, tt.wantSymbol)
			}
		})
	}
}
