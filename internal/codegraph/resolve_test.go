package codegraph

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// Unit-test boundary for Resolver:
//
// Stage 1 (local symbol table) is safe to test without a DB — the map
// lookup short-circuits before any pool access.
//
// Stages 2 (import-aware) and 3 (suffix fallback) both call into the DB via
// a non-nil DBTX interface wrapping a nil *pgxpool.Pool, which panics inside
// pgxpool on the first QueryRow call.  Full coverage of those stages lives in
// resolve_integration_test.go (//go:build integration).

// ── NewResolver ───────────────────────────────────────────────────────────────

func TestNewResolver_StoresFields(t *testing.T) {
	t.Parallel()
	scopeID := uuid.New()
	r := NewResolver(nil, scopeID, nil, "")
	if r == nil {
		t.Fatal("NewResolver returned nil")
	}
	if r.scopeID != scopeID {
		t.Errorf("scopeID = %v, want %v", r.scopeID, scopeID)
	}
}

// ── Resolve — Stage 1: local symbol table ────────────────────────────────────

func TestResolver_Resolve_LocalHit(t *testing.T) {
	t.Parallel()
	r := NewResolver(nil, uuid.New(), nil, "")

	want := uuid.New()
	local := map[string]uuid.UUID{"MyFunc": want}

	got, ok := r.Resolve(context.Background(), "file.go", "MyFunc", local)
	if !ok {
		t.Fatal("expected ok=true for local hit")
	}
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestResolver_Resolve_LocalHit_CaseSensitive(t *testing.T) {
	t.Parallel()
	// The local table is a plain Go map: "MyFunc" and "myfunc" are distinct keys.
	r := NewResolver(nil, uuid.New(), nil, "")

	upper := uuid.New()
	lower := uuid.New()
	local := map[string]uuid.UUID{
		"MyFunc": upper,
		"myfunc": lower,
	}

	got, ok := r.Resolve(context.Background(), "file.go", "MyFunc", local)
	if !ok || got != upper {
		t.Errorf("Resolve(MyFunc): got %v ok=%v, want %v ok=true", got, ok, upper)
	}

	got, ok = r.Resolve(context.Background(), "file.go", "myfunc", local)
	if !ok || got != lower {
		t.Errorf("Resolve(myfunc): got %v ok=%v, want %v ok=true", got, ok, lower)
	}
}

func TestResolver_Resolve_LocalHit_ReturnsCorrectAmongMany(t *testing.T) {
	t.Parallel()
	r := NewResolver(nil, uuid.New(), nil, "")

	local := map[string]uuid.UUID{
		"Alpha": uuid.New(),
		"Beta":  uuid.New(),
		"Gamma": uuid.New(),
	}

	for name, want := range local {
		got, ok := r.Resolve(context.Background(), "pkg/file.go", name, local)
		if !ok {
			t.Errorf("Resolve(%q): expected ok=true", name)
		}
		if got != want {
			t.Errorf("Resolve(%q): got %v, want %v", name, got, want)
		}
	}
}

func TestResolver_Resolve_LocalHit_DifferentFilePaths(t *testing.T) {
	t.Parallel()
	// The filePath parameter is only used in stages 2/3; stage 1 ignores it.
	r := NewResolver(nil, uuid.New(), nil, "")

	want := uuid.New()
	local := map[string]uuid.UUID{"Foo": want}

	for _, path := range []string{"", "main.go", "internal/pkg/service.go", "C:\\Windows\\path.go"} {
		got, ok := r.Resolve(context.Background(), path, "Foo", local)
		if !ok || got != want {
			t.Errorf("filePath=%q: got %v ok=%v, want %v ok=true", path, got, ok, want)
		}
	}
}
