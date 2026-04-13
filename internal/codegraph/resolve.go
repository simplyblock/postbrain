package codegraph

import (
	"context"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/codegraph/lsp"
	"github.com/simplyblock/postbrain/internal/db/compat"
)

// Resolver holds the resolution context for a single file extraction pass.
type Resolver struct {
	pool    *pgxpool.Pool
	scopeID uuid.UUID
	// lsp is an optional LSP client for the language being indexed.
	// nil means LSP resolution is disabled for this pass.
	lsp     lsp.Client
	lspRoot string // absolute path of the LSP workspace root
}

// NewResolver creates a Resolver for the given scope.
// lspClient may be nil to disable LSP-assisted resolution.
// lspRoot is the absolute path that was passed to the LSP client as rootDir;
// it is used to convert repo-relative file paths to absolute paths.
func NewResolver(pool *pgxpool.Pool, scopeID uuid.UUID, lspClient lsp.Client, lspRoot string) *Resolver {
	return &Resolver{pool: pool, scopeID: scopeID, lsp: lspClient, lspRoot: lspRoot}
}

// Resolve attempts to map targetName to an entity ID using a four-stage pipeline:
//
//  1. Local symbol table (exact match, no I/O).
//  2. Import-aware DB lookup (walk stored import edges, try pkg.symbol).
//  3. LSP-assisted resolution (DocumentSymbols + Imports + WorkspaceSymbols).
//  4. Suffix fallback (DB LIKE query).
func (r *Resolver) Resolve(ctx context.Context, filePath, targetName string, localSymTable map[string]uuid.UUID) (uuid.UUID, bool) {
	// Stage 1: local symbol table.
	if id, ok := localSymTable[targetName]; ok {
		return id, true
	}

	// Stage 2: import-aware DB lookup.
	if id, ok := r.resolveViaImports(ctx, filePath, targetName); ok {
		return id, true
	}

	// Stage 3: LSP-assisted resolution.
	if id, ok := r.resolveViaLSP(ctx, filePath, targetName, localSymTable); ok {
		return id, true
	}

	// Stage 4: suffix fallback.
	if r.pool != nil {
		candidates, err := compat.FindEntitiesBySuffix(ctx, r.pool, r.scopeID, targetName)
		if err == nil && len(candidates) > 0 {
			return candidates[0].ID, true
		}
	}

	return uuid.UUID{}, false
}

// resolveViaImports looks up the file entity, walks its outgoing "imports"
// edges, and tries canonical lookups of the form <pkg>.<targetName>.
func (r *Resolver) resolveViaImports(ctx context.Context, filePath, targetName string) (uuid.UUID, bool) {
	if r.pool == nil {
		return uuid.UUID{}, false
	}

	fileCanon := strings.ToLower(filepath.ToSlash(filePath))
	fileEntity, err := compat.GetEntityByCanonical(ctx, r.pool, r.scopeID, "file", fileCanon)
	if err != nil || fileEntity == nil {
		return uuid.UUID{}, false
	}

	relations, err := compat.ListOutgoingRelations(ctx, r.pool, r.scopeID, fileEntity.ID, "imports")
	if err != nil || len(relations) == 0 {
		return uuid.UUID{}, false
	}

	codeTypes := []string{"function", "method", "type", "struct", "interface", "class", "variable"}
	for _, rel := range relations {
		pkgEntity, err := compat.GetEntityByID(ctx, r.pool, rel.ObjectID)
		if err != nil || pkgEntity == nil {
			continue
		}
		for _, cand := range []string{
			pkgEntity.Canonical + "." + targetName,
			pkgEntity.Name + "." + targetName,
		} {
			for _, codeType := range codeTypes {
				ent, err := compat.GetEntityByCanonical(ctx, r.pool, r.scopeID, codeType, cand)
				if err == nil && ent != nil {
					return ent.ID, true
				}
			}
		}
	}
	return uuid.UUID{}, false
}

// resolveViaLSP uses the language server to resolve targetName to a canonical
// entity name and then looks it up in the database.  Two strategies are tried:
//
// Strategy A — document-local declaration:
//
//	DocumentSymbols finds targetName declared in filePath; CanonicalName at
//	that position yields the fully-qualified identifier.
//
// Strategy B — cross-package reference:
//
//	Imports lists the packages in scope; WorkspaceSymbols finds all workspace
//	declarations named targetName; the result whose container matches an
//	imported package is preferred.
func (r *Resolver) resolveViaLSP(ctx context.Context, filePath, targetName string, localSymTable map[string]uuid.UUID) (uuid.UUID, bool) {
	if r.lsp == nil || !strings.EqualFold(filepath.Ext(filePath), r.lsp.Language()) {
		return uuid.UUID{}, false
	}
	absFile := r.absPath(filePath)

	// Strategy A: declaration lives in the same file.
	if docSyms, err := r.lsp.DocumentSymbols(ctx, absFile); err == nil {
		for _, s := range docSyms {
			if !strings.EqualFold(s.Name, targetName) {
				continue
			}
			if canonical, err := r.lsp.CanonicalName(ctx, absFile, s.Location.Range.Start); err == nil && canonical != "" {
				if id, ok := r.lookupCanonical(ctx, canonical, localSymTable); ok {
					return id, true
				}
			}
		}
	}

	// Strategy B: declaration is in another package imported by this file.
	importedPkgs := r.importedPackageNames(ctx, absFile)
	wsSyms, err := r.lsp.WorkspaceSymbols(ctx, targetName)
	if err != nil {
		return uuid.UUID{}, false
	}
	for _, s := range wsSyms {
		if !strings.EqualFold(s.Name, targetName) {
			continue
		}
		// When we know the imported packages, only accept symbols whose
		// container matches one of them.  When Imports() is unavailable we
		// fall back to trusting the first workspace hit.
		if len(importedPkgs) > 0 {
			container := containerFromCanonical(s.Canonical)
			if _, ok := importedPkgs[container]; !ok {
				continue
			}
		}
		if id, ok := r.lookupCanonical(ctx, s.Canonical, localSymTable); ok {
			return id, true
		}
	}

	return uuid.UUID{}, false
}

// importedPackageNames returns the set of package names (alias or base of path)
// that filePath imports, using the LSP client.  Returns nil on error.
func (r *Resolver) importedPackageNames(ctx context.Context, absFile string) map[string]struct{} {
	if r.lsp == nil {
		return nil
	}
	imports, err := r.lsp.Imports(ctx, absFile)
	if err != nil || len(imports) == 0 {
		return nil
	}
	pkgs := make(map[string]struct{}, len(imports))
	for _, imp := range imports {
		name := imp.Alias
		if name == "" {
			name = path.Base(imp.Path) // "github.com/foo/bar" → "bar"
		}
		pkgs[name] = struct{}{}
	}
	return pkgs
}

// lookupCanonical finds a canonical entity name in the local symbol table and
// then, if that fails, in the database across all code entity types.
func (r *Resolver) lookupCanonical(ctx context.Context, canonical string, localSymTable map[string]uuid.UUID) (uuid.UUID, bool) {
	if id, ok := localSymTable[canonical]; ok {
		return id, true
	}
	if r.pool == nil {
		return uuid.UUID{}, false
	}
	for _, codeType := range []string{"function", "method", "type", "struct", "interface", "class", "variable"} {
		ent, err := compat.GetEntityByCanonical(ctx, r.pool, r.scopeID, codeType, canonical)
		if err == nil && ent != nil {
			return ent.ID, true
		}
	}
	return uuid.UUID{}, false
}

// absPath converts a (possibly repo-relative) file path to absolute using the
// LSP workspace root.
func (r *Resolver) absPath(filePath string) string {
	if filepath.IsAbs(filePath) || r.lspRoot == "" {
		return filePath
	}
	return filepath.Join(r.lspRoot, filepath.FromSlash(filePath))
}

// containerFromCanonical returns everything before the final dot in a
// canonical name, e.g. "pkg.Sub" → "pkg", "pkg" → "pkg".
func containerFromCanonical(canonical string) string {
	if i := strings.LastIndex(canonical, "."); i >= 0 {
		return canonical[:i]
	}
	return canonical
}
