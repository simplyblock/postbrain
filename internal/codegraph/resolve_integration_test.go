//go:build integration

package codegraph_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/codegraph"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

// TestResolverStage2_ImportAware verifies that Resolver.Resolve finds a symbol
// via the import-aware lookup (stage 2): file → imports → pkg, pkg.Symbol exists.
func TestResolverStage2_ImportAware(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	principal := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-stage2-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "resolver-stage2-"+uuid.New().String(), nil, principal.ID)

	mustUpsert := func(entityType, name, canonical string) *db.Entity {
		t.Helper()
		e, err := compat.UpsertEntity(ctx, pool, &db.Entity{
			ScopeID:    scope.ID,
			EntityType: entityType,
			Name:       name,
			Canonical:  canonical,
			Meta:       []byte("{}"),
		})
		if err != nil {
			t.Fatalf("upsert entity %s %s: %v", entityType, canonical, err)
		}
		return e
	}

	// Create: file entity, package entity (target of the import), and
	// the function entity whose canonical is pkg.Name.
	fileEnt := mustUpsert("file", "auth/token.go", "auth/token.go")
	pkgEnt := mustUpsert("module", "fmt", "fmt")
	fnEnt := mustUpsert("function", "Println", "fmt.Println")

	// Wire file → imports → package.
	if _, err := compat.UpsertRelation(ctx, pool, &db.Relation{
		ScopeID:    scope.ID,
		SubjectID:  fileEnt.ID,
		Predicate:  "imports",
		ObjectID:   pkgEnt.ID,
		Confidence: 1.0,
	}); err != nil {
		t.Fatalf("upsert relation: %v", err)
	}

	r := codegraph.NewResolver(pool, scope.ID, nil)

	// "Println" is absent from the local table → stage 1 misses → stage 2 finds it.
	id, ok := r.Resolve(ctx, "auth/token.go", "Println", nil)
	if !ok {
		t.Fatal("expected stage-2 import-aware resolution to succeed")
	}
	if id != fnEnt.ID {
		t.Errorf("resolved to %v, want %v", id, fnEnt.ID)
	}
}

// TestResolverStage3_SuffixFallback verifies that Resolve finds a symbol via
// the suffix fallback (stage 3) when no file/import path exists.
func TestResolverStage3_SuffixFallback(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	principal := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-stage3-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "resolver-stage3-"+uuid.New().String(), nil, principal.ID)

	// A dotted canonical: FindEntitiesBySuffix matches "VerifyToken" via
	// "canonical LIKE ('%.' || $2)".
	fn, err := compat.UpsertEntity(ctx, pool, &db.Entity{
		ScopeID:    scope.ID,
		EntityType: "function",
		Name:       "VerifyToken",
		Canonical:  "auth.VerifyToken",
		Meta:       []byte("{}"),
	})
	if err != nil {
		t.Fatalf("upsert entity: %v", err)
	}

	r := codegraph.NewResolver(pool, scope.ID, nil)

	// No file entity in DB → stage 2 is skipped. Stage 3 matches by suffix.
	id, ok := r.Resolve(ctx, "some/other/file.go", "VerifyToken", nil)
	if !ok {
		t.Fatal("expected stage-3 suffix fallback to succeed")
	}
	if id != fn.ID {
		t.Errorf("resolved to %v, want %v", id, fn.ID)
	}
}

// TestResolverStage3_NotFound verifies that Resolve returns (uuid.UUID{}, false)
// when no entity matches in any stage.
func TestResolverStage3_NotFound(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	principal := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-notfound-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "resolver-notfound-"+uuid.New().String(), nil, principal.ID)

	r := codegraph.NewResolver(pool, scope.ID, nil)

	id, ok := r.Resolve(ctx, "file.go", "NonExistentSymbolXYZ", nil)
	if ok {
		t.Errorf("expected ok=false for unknown symbol, got id=%v", id)
	}
	if id != (uuid.UUID{}) {
		t.Errorf("expected zero UUID, got %v", id)
	}
}
