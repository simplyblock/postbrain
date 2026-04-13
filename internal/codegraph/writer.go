package codegraph

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
)

// persistFileMemory creates a file-level semantic memory for src.
// Returns nil when opts.AuthorID is zero or the write fails.
func persistFileMemory(ctx context.Context, pool *pgxpool.Pool, opts IndexOptions, fileName string, src []byte) *uuid.UUID {
	if opts.AuthorID == uuid.Nil {
		return nil
	}
	fileSourceRef := "file:" + fileName
	fileMem, err := compat.CreateMemory(ctx, pool, &db.Memory{
		MemoryType:      "semantic",
		ScopeID:         opts.ScopeID,
		AuthorID:        opts.AuthorID,
		Content:         string(src),
		ContentKind:     "code",
		SourceRef:       &fileSourceRef,
		PromotionStatus: "none",
	})
	if err != nil {
		slog.WarnContext(ctx, "codegraph: create file memory", "file", fileName, "err", err)
		return nil
	}
	if fileMem != nil {
		return &fileMem.ID
	}
	return nil
}

// persistSymbolEntities upserts each symbol as a graph entity and links the file
// memory to any KindFile symbols. Returns a name→ID map for relation resolution.
func persistSymbolEntities(ctx context.Context, pool *pgxpool.Pool, opts IndexOptions, syms []Symbol, fileMemoryID *uuid.UUID, res *IndexResult) map[string]uuid.UUID {
	symToID := make(map[string]uuid.UUID, len(syms))
	for _, sym := range syms {
		canonical := sym.Name
		if sym.Package != "" {
			canonical = sym.Package + "." + sym.Name
		}
		ent, uErr := compat.UpsertEntity(ctx, pool, &db.Entity{
			ScopeID:    opts.ScopeID,
			EntityType: string(sym.Kind),
			Name:       sym.Name,
			Canonical:  canonical,
		})
		if uErr != nil {
			slog.WarnContext(ctx, "codegraph: upsert entity", "name", sym.Name, "err", uErr)
			continue
		}
		symToID[sym.Name] = ent.ID
		symToID[canonical] = ent.ID

		if sym.Kind == KindFile && fileMemoryID != nil {
			if lErr := compat.LinkMemoryToEntity(ctx, pool, *fileMemoryID, ent.ID, ""); lErr != nil {
				slog.WarnContext(ctx, "codegraph: link file memory to entity", "err", lErr)
			}
		}

		res.SymbolsUpserted++
	}
	return symToID
}

// persistChunkMemories creates child semantic memories for substantive code symbols.
func persistChunkMemories(ctx context.Context, pool *pgxpool.Pool, opts IndexOptions, fileName string, src []byte, syms []Symbol, fileMemoryID *uuid.UUID, res *IndexResult) {
	if fileMemoryID == nil {
		return
	}
	for _, sym := range syms {
		switch sym.Kind {
		case KindFunction, KindMethod, KindClass, KindStruct, KindInterface:
			if sym.StartByte == sym.EndByte {
				continue
			}
			if int(sym.EndByte) > len(src) || int(sym.StartByte) >= len(src) {
				continue
			}
			chunkContent := string(src[sym.StartByte:sym.EndByte])
			chunkSourceRef := fmt.Sprintf("file:%s:%d", fileName, sym.StartLine+1)
			_, cErr := compat.CreateMemory(ctx, pool, &db.Memory{
				MemoryType:      "semantic",
				ScopeID:         opts.ScopeID,
				AuthorID:        opts.AuthorID,
				Content:         chunkContent,
				ContentKind:     "code",
				SourceRef:       &chunkSourceRef,
				ParentMemoryID:  fileMemoryID,
				PromotionStatus: "none",
			})
			if cErr != nil {
				slog.WarnContext(ctx, "codegraph: create chunk memory", "sym", sym.Name, "err", cErr)
				continue
			}
			res.ChunksCreated++
		}
	}
}

// persistRelations resolves and upserts edges between symbols.
func persistRelations(ctx context.Context, pool *pgxpool.Pool, opts IndexOptions, fileName string, edges []Edge, resolver *Resolver, symToID map[string]uuid.UUID, res *IndexResult) {
	for _, edge := range edges {
		subjectID, ok := resolver.Resolve(ctx, fileName, edge.SubjectName, symToID)
		if !ok {
			continue
		}
		objectID, ok := resolver.Resolve(ctx, fileName, edge.ObjectName, symToID)
		if !ok {
			continue
		}

		_, rErr := compat.UpsertRelation(ctx, pool, &db.Relation{
			ScopeID:    opts.ScopeID,
			SubjectID:  subjectID,
			Predicate:  edge.Predicate,
			ObjectID:   objectID,
			Confidence: 1.0,
			SourceFile: &fileName,
		})
		if rErr != nil {
			slog.WarnContext(ctx, "codegraph: upsert relation",
				"predicate", edge.Predicate, "err", rErr)
			continue
		}
		res.RelationsUpserted++
	}
}
