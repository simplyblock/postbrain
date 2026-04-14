package codegraph

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"

	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/closeutil"
	"github.com/simplyblock/postbrain/internal/codegraph/lsp"
)

// indexFile extracts symbols and relations from a single git blob and upserts them.
func indexFile(ctx context.Context, pool *pgxpool.Pool, opts IndexOptions, f *object.File, res *IndexResult, lspSelections []lspSelection) error {
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

	// When an LSP client is available for this file's language, replace the
	// heuristic call edges produced by Extract with LSP-accurate ones.
	// OutgoingCalls gives fully-qualified caller/callee names, so the
	// resulting edges resolve directly without the multi-stage fallback chain.
	lspClient, lspRootDir := lspClientForFile(ctx, f.Name, lspSelections)
	if lspClient != nil && lspRootDir != "" {
		absFile := filepath.Join(lspRootDir, filepath.FromSlash(f.Name))
		edges = enrichCallEdges(ctx, lspClient, absFile, edges)
	}

	res.FilesIndexed++

	fileMemoryID := persistFileMemory(ctx, pool, opts, f.Name, src)
	symToID := persistSymbolEntities(ctx, pool, opts, syms, fileMemoryID, res)
	persistChunkMemories(ctx, pool, opts, f.Name, src, syms, fileMemoryID, res)

	resolver := NewResolver(pool, opts.ScopeID, lspClient, lspRootDir)
	persistRelations(ctx, pool, opts, f.Name, edges, resolver, symToID, res)

	return nil
}

// enrichCallEdges replaces call edges from Extract with LSP-sourced ones.
// It queries DocumentSymbols for all function/method declarations in the file
// and calls OutgoingCalls for each to obtain fully-qualified callee names.
// Non-call edges (imports, defines, uses, …) are preserved unchanged.
// If LSP queries fail the original edges are returned unmodified.
func enrichCallEdges(ctx context.Context, client lsp.Client, absFile string, edges []Edge) []Edge {
	syms, err := client.DocumentSymbols(ctx, absFile)
	if err != nil || len(syms) == 0 {
		return edges
	}

	// Collect outgoing call edges from every function/method declared in the file.
	var lspCalls []Edge
	for _, sym := range syms {
		if sym.Kind != lsp.KindFunction && sym.Kind != lsp.KindMethod {
			continue
		}
		calls, err := client.OutgoingCalls(ctx, absFile, sym.Location.Range.Start)
		if err != nil {
			slog.WarnContext(ctx, "codegraph: lsp outgoing calls",
				"file", absFile, "symbol", sym.Canonical, "err", err)
			continue
		}
		for _, call := range calls {
			if call.CalleeSymbol == "" {
				continue
			}
			lspCalls = append(lspCalls, Edge{
				SubjectName: sym.Canonical,
				Predicate:   "calls",
				ObjectName:  call.CalleeSymbol,
			})
		}
	}

	// If LSP produced no calls at all (e.g. the file has no outgoing calls or
	// gopls does not support call hierarchy for this module), keep the heuristic
	// edges so we don't silently drop data.
	if len(lspCalls) == 0 {
		return edges
	}

	// Drop heuristic call edges and replace with LSP-accurate ones.
	result := make([]Edge, 0, len(edges)+len(lspCalls))
	for _, e := range edges {
		if e.Predicate != "calls" {
			result = append(result, e)
		}
	}
	return append(result, lspCalls...)
}

func isUnsupported(err error) bool {
	_, ok := err.(ErrUnsupportedLanguage)
	return ok
}
