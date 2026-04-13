package codegraph

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/go-git/go-git/v5/plumbing/object"
	merkletrie "github.com/go-git/go-git/v5/utils/merkletrie"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/codegraph/lsp"
	"github.com/simplyblock/postbrain/internal/db/compat"
)

// indexFullTree walks every file in the tree and upserts symbols/relations.
func indexFullTree(ctx context.Context, pool *pgxpool.Pool, opts IndexOptions, tree *object.Tree, res *IndexResult, lspClient lsp.Client) error {
	return tree.Files().ForEach(func(f *object.File) error {
		if err := indexFile(ctx, pool, opts, f, res, lspClient); err != nil {
			slog.WarnContext(ctx, "codegraph: index file error", "file", f.Name, "err", err)
		}
		return nil
	})
}

// indexDiff re-extracts only added/modified files and deletes relations for removed files.
func indexDiff(ctx context.Context, pool *pgxpool.Pool, opts IndexOptions, prevTree, currTree *object.Tree, res *IndexResult, lspClient lsp.Client) error {
	changes, err := prevTree.Diff(currTree)
	if err != nil {
		return fmt.Errorf("codegraph: tree diff: %w", err)
	}

	for _, change := range changes {
		action, err := change.Action()
		if err != nil {
			continue
		}
		switch action {
		case merkletrie.Delete:
			_ = compat.DeleteRelationsBySourceFile(ctx, pool, opts.ScopeID, change.From.Name)

		case merkletrie.Insert:
			f, err := currTree.File(change.To.Name)
			if err != nil {
				continue
			}
			if err := indexFile(ctx, pool, opts, f, res, lspClient); err != nil {
				slog.WarnContext(ctx, "codegraph: index file error (insert)", "file", f.Name, "err", err)
			}

		case merkletrie.Modify:
			_ = compat.DeleteRelationsBySourceFile(ctx, pool, opts.ScopeID, change.To.Name)
			f, err := currTree.File(change.To.Name)
			if err != nil {
				continue
			}
			if err := indexFile(ctx, pool, opts, f, res, lspClient); err != nil {
				slog.WarnContext(ctx, "codegraph: index file error (modify)", "file", f.Name, "err", err)
			}
		}
	}
	return nil
}
