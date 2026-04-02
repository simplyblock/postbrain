//go:build integration

package codegraph_test

import (
	"context"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/codegraph"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

const (
	lspPkgTargetCanonical = "longpkg.Target"
	lspBogusCanonical     = "x.Target"
	lspCallerName         = "longpkg.Caller"
)

func TestIndexRepo_LSPEnabled_ResolvesCallsDifferentlyThanFallback(t *testing.T) {
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
	goplsPath := mustFindGoplsBinary(t)
	lspPrincipal := testhelper.CreateTestPrincipal(t, pool, "user", "lsp-on-"+uuid.New().String())
	lspScope := testhelper.CreateTestScope(t, pool, "team", "lsp-on-"+uuid.New().String(), nil, lspPrincipal.ID)
	seedCompetingSuffixEntities(t, ctx, pool, lspScope.ID)

	addr, stop := startGoplsServe(t, goplsPath)
	defer stop()

	rootURI := (&url.URL{Scheme: "file", Path: filepath.ToSlash(repoDir)}).String()
	if _, err := codegraph.IndexRepo(ctx, pool, codegraph.IndexOptions{
		ScopeID:       lspScope.ID,
		RepoURL:       repoDir,
		DefaultBranch: branch,
		GoLSPAddr:     addr,
		GoLSPRootURI:  rootURI,
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
		if _, err := db.UpsertEntity(ctx, pool, &db.Entity{
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

func mustFindGoplsBinary(t *testing.T) string {
	t.Helper()
	if p, err := exec.LookPath("gopls"); err == nil {
		return p
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("unable to resolve test file path for local gopls lookup")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	local := filepath.Join(repoRoot, "bin", "gopls")
	if st, err := os.Stat(local); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
		return local
	}
	t.Skip("gopls not found in PATH or ./bin/gopls")
	return ""
}

func startGoplsServe(t *testing.T, goplsPath string) (string, func()) {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve tcp port: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, goplsPath, "serve", "-listen", addr)
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start gopls serve: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 150*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			cancel()
			_ = cmd.Process.Kill()
			t.Fatalf("gopls did not become ready on %s: %v", addr, err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	stop := func() {
		cancel()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	return addr, stop
}
