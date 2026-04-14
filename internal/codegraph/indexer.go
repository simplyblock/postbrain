package codegraph

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IndexOptions controls how a repository is indexed.
type IndexOptions struct {
	// ScopeID is the project scope to associate symbols and relations with.
	ScopeID uuid.UUID
	// RepoURL is the git clone URL.
	RepoURL string
	// DefaultBranch is the branch to check out (defaults to "main").
	DefaultBranch string
	// CodeMemory controls whether to create code memory entities.
	CodeMemory *bool
	// AuthorID is the principal UUID recorded as the author of created memories.
	// If zero, memories are not created (code graph entities/relations are still indexed).
	AuthorID uuid.UUID
	// AuthToken is an optional bearer token injected as HTTP basic auth password.
	AuthToken string
	// SSHKey is an optional PEM-encoded private key used for SSH clone URLs
	// (e.g. git@github.com:user/repo.git or ssh://git@...).
	// SSHKeyPassphrase is the passphrase for the key, if encrypted.
	SSHKey           string
	SSHKeyPassphrase string
	// PrevCommit is the last indexed commit SHA; if non-empty, performs an
	// incremental diff rather than a full re-index.
	PrevCommit string
	// MaxBytesPerFile caps the maximum file size that will be parsed.
	// Files larger than this are silently skipped. 0 → 512 KiB.
	MaxBytesPerFile int64
	// Depth controls the git clone depth. 0 defaults to 1 (shallow, production default).
	// Set higher in tests to make previous commits reachable for incremental diffs.
	Depth int
	// GoLSPRootDir enables optional Go LSP resolution via a local gopls process.
	// It must be the absolute path to the checked-out source tree that gopls
	// should index.  Empty disables LSP for this run.
	GoLSPRootDir string
	// GoLSPTimeout controls per-request timeouts for the gopls subprocess.
	GoLSPTimeout time.Duration
	// TypeScriptLSPRootDir enables optional TypeScript/JavaScript LSP
	// resolution via a local stdio language server process.
	// It must be the absolute path to the checked-out source tree.
	// Empty disables TypeScript LSP for this run.
	TypeScriptLSPRootDir string
	// TypeScriptLSPTimeout controls per-request timeouts for the TypeScript
	// language server subprocess.
	TypeScriptLSPTimeout time.Duration
	// TypeScriptLSPUseTSGo selects tsgo (`tsgo --lsp`) instead of
	// typescript-language-server for TypeScript/JavaScript extensions.
	TypeScriptLSPUseTSGo bool
	// ClangdLSPRootDir enables optional C/C++ LSP resolution via clangd.
	// It must be the absolute path to the checked-out source tree.
	// Empty disables clangd for this run.
	ClangdLSPRootDir string
	// ClangdLSPTimeout controls per-request timeouts for the clangd subprocess.
	ClangdLSPTimeout time.Duration
	// PythonLSPRootDir enables optional Python LSP resolution via pyright.
	// It must be the absolute path to the checked-out source tree.
	// Empty disables Python LSP for this run.
	PythonLSPRootDir string
	// PythonLSPTimeout controls per-request timeouts for the pyright subprocess.
	PythonLSPTimeout time.Duration
	// MarkdownLSPRootDir enables optional Markdown LSP resolution via marksman.
	// It must be the absolute path to the checked-out source tree.
	// Empty disables marksman for this run.
	MarkdownLSPRootDir string
	// MarkdownLSPTimeout controls per-request timeouts for the marksman subprocess.
	MarkdownLSPTimeout time.Duration
}

const defaultMaxBytes int64 = 512 * 1024

// IndexResult summarises what was written during an index run.
type IndexResult struct {
	CommitSHA         string
	FilesIndexed      int
	FilesSkipped      int
	SymbolsUpserted   int
	RelationsUpserted int
	ChunksCreated     int
}

// IndexRepo clones (or diffs) the repository in-memory and upserts all extracted
// symbols and relations into the database. Returns the HEAD commit SHA on success.
func IndexRepo(ctx context.Context, pool *pgxpool.Pool, opts IndexOptions) (*IndexResult, error) {
	if opts.MaxBytesPerFile <= 0 {
		opts.MaxBytesPerFile = defaultMaxBytes
	}
	if opts.DefaultBranch == "" {
		opts.DefaultBranch = "main"
	}

	lspSelections := newLSPRegistry(opts)
	if len(lspSelections) > 0 {
		defer closeLSPSelections(ctx, lspSelections)
	}

	cloneOpts, err := newInMemoryCloneOptions(opts)
	if err != nil {
		return nil, fmt.Errorf("codegraph: auth: %w", err)
	}

	slog.InfoContext(ctx, "codegraph: cloning repository",
		"url", sanitizeURL(opts.RepoURL),
		"branch", opts.DefaultBranch,
		"scope_id", opts.ScopeID,
	)

	repo, err := gogit.CloneContext(ctx, memory.NewStorage(), memfs.New(), cloneOpts)
	if err != nil {
		return nil, fmt.Errorf("codegraph: clone: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("codegraph: head: %w", err)
	}
	headSHA := head.Hash().String()

	headCommit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("codegraph: head commit: %w", err)
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("codegraph: head tree: %w", err)
	}

	res := &IndexResult{CommitSHA: headSHA}

	if opts.PrevCommit != "" && opts.PrevCommit != headSHA {
		prevHash := plumbing.NewHash(opts.PrevCommit)
		prevCommit, fetchErr := repo.CommitObject(prevHash)
		if fetchErr != nil {
			// prev commit unreachable in shallow clone — full re-index
			slog.WarnContext(ctx, "codegraph: prev commit not reachable, falling back to full index",
				"prev_sha", opts.PrevCommit)
			if err2 := indexFullTree(ctx, pool, opts, headTree, res, lspSelections); err2 != nil {
				return nil, err2
			}
		} else {
			prevTree, treeErr := prevCommit.Tree()
			if treeErr != nil {
				return nil, fmt.Errorf("codegraph: prev tree: %w", treeErr)
			}
			if err2 := indexDiff(ctx, pool, opts, prevTree, headTree, res, lspSelections); err2 != nil {
				return nil, err2
			}
		}
	} else {
		if err2 := indexFullTree(ctx, pool, opts, headTree, res, lspSelections); err2 != nil {
			return nil, err2
		}
	}

	return res, nil
}
