//go:build integration

package codegraph_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/codegraph"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

// ── local git repo helpers ────────────────────────────────────────────────────

// initRepo creates a fresh git repo in a temp dir, writes the given files,
// and makes an initial commit. Returns the repo dir and the HEAD commit SHA.
func initRepo(t *testing.T, files map[string]string) (dir, sha, branch string) {
	t.Helper()
	dir = t.TempDir()

	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("git init: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}

	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if _, err := wt.Add(name); err != nil {
			t.Fatalf("git add %s: %v", name, err)
		}
	}

	h, err := wt.Commit("initial commit", &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("git commit: %v", err)
	}

	// Determine the actual branch name (varies by git config: "main" or "master").
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}

	return dir, h.String(), head.Name().Short()
}

// addCommit adds more files to an existing repo and makes a new commit.
// Returns the new HEAD SHA.
func addCommit(t *testing.T, dir string, files map[string]string) string {
	t.Helper()

	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}

	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if _, err := wt.Add(name); err != nil {
			t.Fatalf("git add %s: %v", name, err)
		}
	}

	h, err := wt.Commit("second commit", &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("git commit: %v", err)
	}
	return h.String()
}

// ── tests ─────────────────────────────────────────────────────────────────────

const helloGo = `package testpkg

func Hello() string { return "hello" }
`

const worldGo = `package testpkg

func World() string { return Hello() }
`

func TestIndexRepo_FullIndex(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	principal := testhelper.CreateTestPrincipal(t, pool, "user", "idx-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "team", "idx-"+uuid.New().String(), nil, principal.ID)

	repoDir, _, branch := initRepo(t, map[string]string{
		"hello.go": helloGo,
	})

	res, err := codegraph.IndexRepo(context.Background(), pool, codegraph.IndexOptions{
		ScopeID:       scope.ID,
		RepoURL:       repoDir,
		DefaultBranch: branch,
	})
	if err != nil {
		t.Fatalf("IndexRepo: %v", err)
	}

	if res.FilesIndexed != 1 {
		t.Errorf("FilesIndexed = %d, want 1", res.FilesIndexed)
	}
	if res.SymbolsUpserted == 0 {
		t.Error("SymbolsUpserted = 0, want > 0")
	}

	// Verify entities were written to DB.
	var n int
	err = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM entities WHERE scope_id = $1`, scope.ID,
	).Scan(&n)
	if err != nil {
		t.Fatalf("count entities: %v", err)
	}
	if n == 0 {
		t.Error("no entities in DB after indexing, want > 0")
	}
}

func TestIndexRepo_IncrementalDiff(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	principal := testhelper.CreateTestPrincipal(t, pool, "user", "diff-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "team", "diff-"+uuid.New().String(), nil, principal.ID)

	repoDir, firstSHA, branch := initRepo(t, map[string]string{
		"hello.go": helloGo,
	})

	// First full index.
	res1, err := codegraph.IndexRepo(context.Background(), pool, codegraph.IndexOptions{
		ScopeID:       scope.ID,
		RepoURL:       repoDir,
		DefaultBranch: branch,
		Depth:         2, // include enough history for diff
	})
	if err != nil {
		t.Fatalf("first IndexRepo: %v", err)
	}
	if res1.FilesIndexed != 1 {
		t.Errorf("first run FilesIndexed = %d, want 1", res1.FilesIndexed)
	}

	// Add a second file and commit.
	_ = addCommit(t, repoDir, map[string]string{
		"world.go": worldGo,
	})

	// Incremental re-index from firstSHA.
	res2, err := codegraph.IndexRepo(context.Background(), pool, codegraph.IndexOptions{
		ScopeID:       scope.ID,
		RepoURL:       repoDir,
		DefaultBranch: branch,
		PrevCommit:    firstSHA,
		Depth:         2,
	})
	if err != nil {
		t.Fatalf("incremental IndexRepo: %v", err)
	}

	// Only world.go is new — diff should index exactly 1 file.
	if res2.FilesIndexed != 1 {
		t.Errorf("incremental FilesIndexed = %d, want 1 (only world.go is new)", res2.FilesIndexed)
	}

	// Symbols from world.go must be in DB.
	var worldCount int
	err = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM entities WHERE scope_id = $1 AND name LIKE '%World%'`, scope.ID,
	).Scan(&worldCount)
	if err != nil {
		t.Fatalf("count world entities: %v", err)
	}
	if worldCount == 0 {
		t.Error("World symbol not found in DB after incremental index")
	}
}

func TestIndexRepo_MaxBytesPerFile(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	principal := testhelper.CreateTestPrincipal(t, pool, "user", "skip-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "team", "skip-"+uuid.New().String(), nil, principal.ID)

	// A file whose content exceeds the tiny cap.
	bigContent := "package bigpkg\n\n" + string(make([]byte, 200))
	repoDir, _, branch := initRepo(t, map[string]string{
		"big.go": bigContent,
	})

	res, err := codegraph.IndexRepo(context.Background(), pool, codegraph.IndexOptions{
		ScopeID:         scope.ID,
		RepoURL:         repoDir,
		DefaultBranch:   branch,
		MaxBytesPerFile: 50, // smaller than big.go
	})
	if err != nil {
		t.Fatalf("IndexRepo: %v", err)
	}

	if res.FilesSkipped != 1 {
		t.Errorf("FilesSkipped = %d, want 1", res.FilesSkipped)
	}
	if res.FilesIndexed != 0 {
		t.Errorf("FilesIndexed = %d, want 0", res.FilesIndexed)
	}
}
