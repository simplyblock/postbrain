//go:build integration

package codegraph_test

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/codegraph"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

const (
	lspPkgTargetCanonical = "longpkg.Target"
	lspBogusCanonical     = "x.Target"
	lspCallerName         = "longpkg.Caller"
)

func TestIndexRepo_LSPEnabled_ResolvesCallsDifferentlyThanFallback(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found in PATH")
	}

	ctx := context.Background()
	pool := testhelper.NewTestPool(t)

	repoDir, _, branch := initRepo(t, map[string]string{
		"go.mod": "module example.com/lspfixture\n\ngo 1.25.0\n",
		"a_target.go": `package longpkg

func Target() {}
`,
		"z_caller.go": `package longpkg

func Caller() { Target() }
`,
	})

	// Baseline scope (LSP disabled).
	basePrincipal := testhelper.CreateTestPrincipal(t, pool, "user", "lsp-off-"+uuid.New().String())
	baseScope := testhelper.CreateTestScope(t, pool, "team", "lsp-off-"+uuid.New().String(), nil, basePrincipal.ID)
	seedCompetingSuffixEntities(t, ctx, pool, baseScope.ID)

	if _, err := codegraph.IndexRepo(ctx, pool, codegraph.IndexOptions{
		ScopeID:       baseScope.ID,
		RepoURL:       repoDir,
		DefaultBranch: branch,
	}); err != nil {
		t.Fatalf("IndexRepo (lsp disabled): %v", err)
	}

	baseTarget := callTargetForCaller(t, ctx, pool, baseScope.ID)
	if baseTarget != lspBogusCanonical {
		t.Fatalf("lsp-disabled target = %q, want %q", baseTarget, lspBogusCanonical)
	}

	// LSP-enabled scope.
	lspPrincipal := testhelper.CreateTestPrincipal(t, pool, "user", "lsp-on-"+uuid.New().String())
	lspScope := testhelper.CreateTestScope(t, pool, "team", "lsp-on-"+uuid.New().String(), nil, lspPrincipal.ID)
	seedCompetingSuffixEntities(t, ctx, pool, lspScope.ID)

	if _, err := codegraph.IndexRepo(ctx, pool, codegraph.IndexOptions{
		ScopeID:       lspScope.ID,
		RepoURL:       repoDir,
		DefaultBranch: branch,
		GoLSPRootDir:  repoDir,
		GoLSPTimeout:  5 * time.Second,
	}); err != nil {
		t.Fatalf("IndexRepo (lsp enabled): %v", err)
	}

	lspTarget := callTargetForCaller(t, ctx, pool, lspScope.ID)
	if lspTarget != lspPkgTargetCanonical {
		t.Fatalf("lsp-enabled target = %q, want %q", lspTarget, lspPkgTargetCanonical)
	}
}

func seedCompetingSuffixEntities(t *testing.T, ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID) {
	t.Helper()
	for _, canonical := range []string{lspBogusCanonical, lspPkgTargetCanonical} {
		if _, err := compat.UpsertEntity(ctx, pool, &db.Entity{
			ScopeID:    scopeID,
			EntityType: "function",
			Name:       canonical,
			Canonical:  canonical,
			Meta:       []byte("{}"),
		}); err != nil {
			t.Fatalf("upsert seeded entity %q: %v", canonical, err)
		}
	}
}

func callTargetForCaller(t *testing.T, ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID) string {
	t.Helper()
	var canonical string
	err := pool.QueryRow(ctx, `
SELECT obj.canonical
FROM relations r
JOIN entities subj ON subj.id = r.subject_id
JOIN entities obj  ON obj.id = r.object_id
WHERE r.scope_id = $1
  AND r.predicate = 'calls'
  AND subj.name = $2
ORDER BY obj.canonical
LIMIT 1
`, scopeID, lspCallerName).Scan(&canonical)
	if err != nil {
		t.Fatalf("query call target canonical: %v", err)
	}
	return canonical
}
