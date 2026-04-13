package codegraph

import (
	"context"
	"io"

	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/closeutil"
)

// indexFile extracts symbols and relations from a single git blob and upserts them.
func indexFile(ctx context.Context, pool *pgxpool.Pool, opts IndexOptions, f *object.File, res *IndexResult, lspResolver LSPResolver) error {
	if f.Size > opts.MaxBytesPerFile {
		res.FilesSkipped++
		return nil
	}

	rc, err := f.Reader()
	if err != nil {
		res.FilesSkipped++
		return nil
	}
	defer closeutil.Log(rc, "git blob reader")

	src, err := io.ReadAll(rc)
	if err != nil {
		res.FilesSkipped++
		return nil
	}

	syms, edges, err := Extract(ctx, src, f.Name)
	if err != nil {
		if isUnsupported(err) {
			res.FilesSkipped++
			return nil
		}
		return err
	}

	res.FilesIndexed++

	fileMemoryID := persistFileMemory(ctx, pool, opts, f.Name, src)
	symToID := persistSymbolEntities(ctx, pool, opts, syms, fileMemoryID, res)
	persistChunkMemories(ctx, pool, opts, f.Name, src, syms, fileMemoryID, res)

	resolver := NewResolver(pool, opts.ScopeID, lspResolver)
	persistRelations(ctx, pool, opts, f.Name, edges, resolver, symToID, res)

	return nil
}

func isUnsupported(err error) bool {
	_, ok := err.(ErrUnsupportedLanguage)
	return ok
}
