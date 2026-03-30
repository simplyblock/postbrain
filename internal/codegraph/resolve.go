package codegraph

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
)

// LSPResolver is the interface for optional language-server-based resolution.
// Implementations are registered per file extension.
type LSPResolver interface {
	// Language returns the file extension this resolver handles (e.g. ".go").
	Language() string
	// Resolve maps an unresolved symbol in a given file to a canonical entity name.
	// Returns "" if unresolvable.
	Resolve(ctx context.Context, file, symbol string) (string, error)
	// Close releases any resources (LSP process, temp dir).
	Close() error
}

// Resolver holds the resolution context for a single file extraction pass.
type Resolver struct {
	pool    *pgxpool.Pool
	scopeID uuid.UUID
	// lsp is an optional language-specific resolver; nil = not configured.
	lsp LSPResolver
}

// NewResolver creates a Resolver for the given scope.
// lsp may be nil for heuristic-only resolution.
func NewResolver(pool *pgxpool.Pool, scopeID uuid.UUID, lsp LSPResolver) *Resolver {
	return &Resolver{pool: pool, scopeID: scopeID, lsp: lsp}
}

// Resolve attempts to resolve a target symbol name to an entity ID using a
// three-stage pipeline: local → import-aware → suffix fallback.
func (r *Resolver) Resolve(ctx context.Context, filePath, targetName string, localSymTable map[string]uuid.UUID) (uuid.UUID, bool) {
	// Stage 1: local symbol table.
	if id, ok := localSymTable[targetName]; ok {
		return id, true
	}

	// Stage 2: import-aware lookup.
	if id, ok := r.resolveViaImports(ctx, filePath, targetName); ok {
		return id, true
	}

	// Stage 3: suffix fallback.
	candidates, err := db.FindEntitiesBySuffix(ctx, r.pool, r.scopeID, targetName)
	if err == nil && len(candidates) > 0 {
		return candidates[0].ID, true
	}

	return uuid.UUID{}, false
}

// resolveViaImports looks up the file entity, walks its outgoing "imports"
// edges, and tries canonical lookups of the form <pkg>.<targetName>.
func (r *Resolver) resolveViaImports(ctx context.Context, filePath, targetName string) (uuid.UUID, bool) {
	// File canonicals are lowercased (see extract_go.go etc).
	fileCanon := strings.ToLower(filepath.ToSlash(filePath))

	// Try to find the file entity in the database.
	fileEntity, err := db.GetEntityByCanonical(ctx, r.pool, r.scopeID, "file", fileCanon)
	if err != nil || fileEntity == nil {
		return uuid.UUID{}, false
	}

	// Get outgoing imports edges for this file entity.
	relations, err := db.ListOutgoingRelations(ctx, r.pool, r.scopeID, fileEntity.ID, "imports")
	if err != nil || len(relations) == 0 {
		return uuid.UUID{}, false
	}

	// Code entity types to try.
	codeTypes := []string{"function", "method", "type", "struct", "interface", "class", "variable"}

	for _, rel := range relations {
		// Get the imported package entity.
		pkgEntity, err := db.GetEntityByID(ctx, r.pool, rel.ObjectID)
		if err != nil || pkgEntity == nil {
			continue
		}

		// Try <pkgCanonical>.<targetName> and <pkgName>.<targetName>.
		candidates := []string{
			pkgEntity.Canonical + "." + targetName,
			pkgEntity.Name + "." + targetName,
		}

		for _, cand := range candidates {
			for _, codeType := range codeTypes {
				ent, err := db.GetEntityByCanonical(ctx, r.pool, r.scopeID, codeType, cand)
				if err == nil && ent != nil {
					return ent.ID, true
				}
			}
		}
	}

	return uuid.UUID{}, false
}
