//go:build integration

package graph_test

import (
	"context"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/graph"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

// ── fixture setup ─────────────────────────────────────────────────────────────

type fixtures struct {
	scopeID  uuid.UUID
	hub      *db.Entity // 2 callers, 3 callees, 1 dependent (fileA via imports)
	callerA  *db.Entity
	callerB  *db.Entity
	calleeX  *db.Entity
	calleeY  *db.Entity
	calleeZ  *db.Entity
	fileA    *db.Entity // imports hub
	isolated *db.Entity // no edges
}

func setupFixtures(t *testing.T, pool *pgxpool.Pool) fixtures {
	t.Helper()
	ctx := context.Background()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "travtest-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "team", "travtest-"+uuid.New().String(), nil, principal.ID)

	ins := func(name, etype, canonical string) *db.Entity {
		e, err := db.UpsertEntity(ctx, pool, &db.Entity{
			ScopeID:    scope.ID,
			EntityType: etype,
			Name:       name,
			Canonical:  canonical,
		})
		if err != nil {
			t.Fatalf("insert entity %q: %v", canonical, err)
		}
		return e
	}

	f := fixtures{
		scopeID:  scope.ID,
		hub:      ins("Hub", "function", "pkg.Hub"),
		callerA:  ins("CallerA", "function", "pkg.CallerA"),
		callerB:  ins("CallerB", "function", "pkg.CallerB"),
		calleeX:  ins("CalleeX", "function", "pkg.CalleeX"),
		calleeY:  ins("CalleeY", "function", "pkg.CalleeY"),
		calleeZ:  ins("CalleeZ", "function", "pkg.CalleeZ"),
		fileA:    ins("a.go", "file", "src/a.go"),
		isolated: ins("Isolated", "function", "pkg.Isolated"),
	}

	rel := func(subj *db.Entity, pred string, obj *db.Entity) {
		_, err := db.UpsertRelation(ctx, pool, &db.Relation{
			ScopeID:    scope.ID,
			SubjectID:  subj.ID,
			Predicate:  pred,
			ObjectID:   obj.ID,
			Confidence: 1.0,
		})
		if err != nil {
			t.Fatalf("upsert relation %s %s %s: %v", subj.Canonical, pred, obj.Canonical, err)
		}
	}

	rel(f.callerA, "calls",   f.hub)      // hub has 2 callers
	rel(f.callerB, "calls",   f.hub)
	rel(f.hub,     "calls",   f.calleeX)  // hub has 3 callees
	rel(f.hub,     "calls",   f.calleeY)
	rel(f.hub,     "calls",   f.calleeZ)
	rel(f.fileA,   "imports", f.hub)      // fileA depends on hub; hub has 1 dependent

	return f
}

// entityIDs extracts IDs from a neighbour slice for easy set comparison.
func neighbourIDs(ns []graph.Neighbour) map[uuid.UUID]struct{} {
	m := make(map[uuid.UUID]struct{}, len(ns))
	for _, n := range ns {
		m[n.Entity.ID] = struct{}{}
	}
	return m
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestTraversal(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	f := setupFixtures(t, pool)
	ctx := context.Background()

	// ── ResolveSymbol ──────────────────────────────────────────────────────────

	t.Run("ResolveSymbol_ExactCanonicalMatch", func(t *testing.T) {
		e, err := graph.ResolveSymbol(ctx, pool, f.scopeID, "pkg.Hub")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if e == nil {
			t.Fatal("expected entity, got nil")
		}
		if e.ID != f.hub.ID {
			t.Errorf("ID = %v, want %v", e.ID, f.hub.ID)
		}
	})

	t.Run("ResolveSymbol_SuffixFallback", func(t *testing.T) {
		// "Hub" matches canonical "pkg.Hub" via the "ends with .Hub" heuristic.
		e, err := graph.ResolveSymbol(ctx, pool, f.scopeID, "Hub")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if e == nil {
			t.Fatal("expected entity via suffix fallback, got nil")
		}
		if e.ID != f.hub.ID {
			t.Errorf("ID = %v, want %v", e.ID, f.hub.ID)
		}
	})

	t.Run("ResolveSymbol_NotFound", func(t *testing.T) {
		e, err := graph.ResolveSymbol(ctx, pool, f.scopeID, "pkg.DoesNotExist")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if e != nil {
			t.Errorf("expected nil, got entity %v", e.Canonical)
		}
	})

	// ── Callers ────────────────────────────────────────────────────────────────

	t.Run("Callers_2Callers", func(t *testing.T) {
		res, err := graph.Callers(ctx, pool, f.scopeID, "pkg.Hub")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res == nil {
			t.Fatal("expected result, got nil")
		}
		if len(res.Neighbours) != 2 {
			t.Fatalf("Neighbours len = %d, want 2", len(res.Neighbours))
		}
		ids := neighbourIDs(res.Neighbours)
		for _, wantID := range []uuid.UUID{f.callerA.ID, f.callerB.ID} {
			if _, ok := ids[wantID]; !ok {
				t.Errorf("expected caller %v not found in neighbours", wantID)
			}
		}
		for _, n := range res.Neighbours {
			if n.Direction != "incoming" {
				t.Errorf("Direction = %q, want %q", n.Direction, "incoming")
			}
		}
	})

	t.Run("Callers_NoneForIsolated", func(t *testing.T) {
		res, err := graph.Callers(ctx, pool, f.scopeID, "pkg.Isolated")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res == nil {
			t.Fatal("expected result, got nil")
		}
		if len(res.Neighbours) != 0 {
			t.Errorf("Neighbours len = %d, want 0", len(res.Neighbours))
		}
	})

	t.Run("Callers_UnknownSymbol", func(t *testing.T) {
		res, err := graph.Callers(ctx, pool, f.scopeID, "pkg.Unknown")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != nil {
			t.Errorf("expected nil result for unknown symbol, got %v", res.Entity.Canonical)
		}
	})

	// ── Callees ────────────────────────────────────────────────────────────────

	t.Run("Callees_3Callees", func(t *testing.T) {
		res, err := graph.Callees(ctx, pool, f.scopeID, "pkg.Hub")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res == nil {
			t.Fatal("expected result, got nil")
		}
		if len(res.Neighbours) != 3 {
			t.Fatalf("Neighbours len = %d, want 3", len(res.Neighbours))
		}
		ids := neighbourIDs(res.Neighbours)
		for _, wantID := range []uuid.UUID{f.calleeX.ID, f.calleeY.ID, f.calleeZ.ID} {
			if _, ok := ids[wantID]; !ok {
				t.Errorf("expected callee %v not found", wantID)
			}
		}
		for _, n := range res.Neighbours {
			if n.Direction != "outgoing" {
				t.Errorf("Direction = %q, want %q", n.Direction, "outgoing")
			}
		}
	})

	t.Run("Callees_NoneForIsolated", func(t *testing.T) {
		res, err := graph.Callees(ctx, pool, f.scopeID, "pkg.Isolated")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res == nil {
			t.Fatal("expected result, got nil")
		}
		if len(res.Neighbours) != 0 {
			t.Errorf("Neighbours len = %d, want 0", len(res.Neighbours))
		}
	})

	// ── Dependencies ──────────────────────────────────────────────────────────

	t.Run("Dependencies_FileWithImports", func(t *testing.T) {
		res, err := graph.Dependencies(ctx, pool, f.scopeID, "src/a.go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res == nil {
			t.Fatal("expected result, got nil")
		}
		if len(res.Neighbours) != 1 {
			t.Fatalf("Neighbours len = %d, want 1", len(res.Neighbours))
		}
		if res.Neighbours[0].Entity.ID != f.hub.ID {
			t.Errorf("dependency = %v, want %v", res.Neighbours[0].Entity.Canonical, f.hub.Canonical)
		}
		if res.Neighbours[0].Predicate != "imports" {
			t.Errorf("Predicate = %q, want %q", res.Neighbours[0].Predicate, "imports")
		}
	})

	t.Run("Dependencies_NoImports", func(t *testing.T) {
		res, err := graph.Dependencies(ctx, pool, f.scopeID, "pkg.Isolated")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res == nil {
			t.Fatal("expected result, got nil")
		}
		if len(res.Neighbours) != 0 {
			t.Errorf("Neighbours len = %d, want 0", len(res.Neighbours))
		}
	})

	// ── Dependents ────────────────────────────────────────────────────────────

	t.Run("Dependents_WithDependents", func(t *testing.T) {
		res, err := graph.Dependents(ctx, pool, f.scopeID, "pkg.Hub")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res == nil {
			t.Fatal("expected result, got nil")
		}
		// Dependents uses ListIncomingRelations with predicate="" → ALL incoming edges:
		// callerA→calls→hub, callerB→calls→hub, fileA→imports→hub = 3
		if len(res.Neighbours) != 3 {
			t.Fatalf("Neighbours len = %d, want 3", len(res.Neighbours))
		}
		ids := neighbourIDs(res.Neighbours)
		for _, wantID := range []uuid.UUID{f.callerA.ID, f.callerB.ID, f.fileA.ID} {
			if _, ok := ids[wantID]; !ok {
				t.Errorf("expected dependent %v not found", wantID)
			}
		}
	})

	t.Run("Dependents_None", func(t *testing.T) {
		res, err := graph.Dependents(ctx, pool, f.scopeID, "pkg.Isolated")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res == nil {
			t.Fatal("expected result, got nil")
		}
		if len(res.Neighbours) != 0 {
			t.Errorf("Neighbours len = %d, want 0", len(res.Neighbours))
		}
	})

	// ── NeighboursForEntity ───────────────────────────────────────────────────

	t.Run("NeighboursForEntity_MixedEdges", func(t *testing.T) {
		ns, err := graph.NeighboursForEntity(ctx, pool, f.scopeID, f.hub.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// hub: callerA (incoming calls), callerB (incoming calls),
		//      calleeX/Y/Z (outgoing calls), fileA (incoming imports) = 6 total
		if len(ns) != 6 {
			names := make([]string, len(ns))
			for i, n := range ns {
				names[i] = n.Entity.Canonical + "(" + n.Direction + ")"
			}
			sort.Strings(names)
			t.Fatalf("Neighbours len = %d, want 6: %v", len(ns), names)
		}
		incoming, outgoing := 0, 0
		for _, n := range ns {
			switch n.Direction {
			case "incoming":
				incoming++
			case "outgoing":
				outgoing++
			default:
				t.Errorf("unexpected Direction %q", n.Direction)
			}
		}
		if incoming != 3 { // callerA, callerB, fileA
			t.Errorf("incoming = %d, want 3", incoming)
		}
		if outgoing != 3 { // calleeX, calleeY, calleeZ
			t.Errorf("outgoing = %d, want 3", outgoing)
		}
	})

	t.Run("NeighboursForEntity_NoEdges", func(t *testing.T) {
		ns, err := graph.NeighboursForEntity(ctx, pool, f.scopeID, f.isolated.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ns) != 0 {
			t.Errorf("Neighbours len = %d, want 0", len(ns))
		}
	})
}
