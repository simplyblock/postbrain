//go:build integration

package knowledge_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/chunking"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func largeChunkableContent() string {
	sentence := strings.Repeat("word ", 200) + ". "
	return strings.Repeat(sentence, 4)
}

func TestCreate_ChunkGraphEntitiesAndRelations(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "chunk-graph-author-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "chunk-graph-scope-"+uuid.New().String(), nil, author.ID)
	store := knowledge.NewStore(pool, svc)

	content := largeChunkableContent()
	chunks := chunking.Chunk(content, chunking.DefaultChunkRunes, chunking.DefaultOverlap)
	if len(chunks) < 2 {
		t.Fatalf("test content yielded %d chunks; want >= 2", len(chunks))
	}

	artifact, err := store.Create(ctx, knowledge.CreateInput{
		KnowledgeType: "semantic",
		OwnerScopeID:  scope.ID,
		AuthorID:      author.ID,
		Visibility:    "project",
		Title:         "chunk graph test",
		Content:       content,
		AutoPublish:   true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	artifactCanonical := fmt.Sprintf("artifact:%s", artifact.ID)
	artifactEntity, err := db.GetEntityByCanonical(ctx, pool, scope.ID, "artifact", artifactCanonical)
	if err != nil {
		t.Fatalf("GetEntityByCanonical(artifact): %v", err)
	}
	if artifactEntity == nil {
		t.Fatalf("artifact entity not found for canonical %q", artifactCanonical)
	}

	chunkEntities := make([]*db.Entity, len(chunks))
	for i := range chunks {
		canonical := fmt.Sprintf("artifact:%s:chunk:%d", artifact.ID, i)
		e, err := db.GetEntityByCanonical(ctx, pool, scope.ID, "artifact_chunk", canonical)
		if err != nil {
			t.Fatalf("GetEntityByCanonical(chunk %d): %v", i, err)
		}
		if e == nil {
			t.Fatalf("chunk entity not found for canonical %q", canonical)
		}
		chunkEntities[i] = e

		rels, err := db.ListOutgoingRelations(ctx, pool, scope.ID, e.ID, "chunk_of")
		if err != nil {
			t.Fatalf("ListOutgoingRelations(chunk_of, chunk %d): %v", i, err)
		}
		if len(rels) != 1 {
			t.Fatalf("chunk %d has %d chunk_of relations, want 1", i, len(rels))
		}
		if rels[0].ObjectID != artifactEntity.ID {
			t.Fatalf("chunk %d chunk_of object = %v, want %v", i, rels[0].ObjectID, artifactEntity.ID)
		}
	}

	relsByScope, err := db.ListRelationsByScope(ctx, pool, scope.ID)
	if err != nil {
		t.Fatalf("ListRelationsByScope: %v", err)
	}

	var nextCount, siblingCount int
	for _, r := range relsByScope {
		switch r.Predicate {
		case "next_chunk":
			nextCount++
		case "chunk_sibling":
			siblingCount++
		}
	}

	wantNext := len(chunks) - 1
	if nextCount != wantNext {
		t.Fatalf("next_chunk relation count = %d, want %d", nextCount, wantNext)
	}
	if siblingCount != 0 {
		t.Fatalf("chunk_sibling relation count = %d, want 0", siblingCount)
	}

	for i := 0; i < len(chunkEntities)-1; i++ {
		rels, err := db.ListOutgoingRelations(ctx, pool, scope.ID, chunkEntities[i].ID, "next_chunk")
		if err != nil {
			t.Fatalf("ListOutgoingRelations(next_chunk, chunk %d): %v", i, err)
		}
		if len(rels) != 1 {
			t.Fatalf("chunk %d has %d next_chunk relations, want 1", i, len(rels))
		}
		if rels[0].ObjectID != chunkEntities[i+1].ID {
			t.Fatalf("chunk %d next_chunk object = %v, want %v", i, rels[0].ObjectID, chunkEntities[i+1].ID)
		}
	}

	lastIdx := len(chunkEntities) - 1
	lastNext, err := db.ListOutgoingRelations(ctx, pool, scope.ID, chunkEntities[lastIdx].ID, "next_chunk")
	if err != nil {
		t.Fatalf("ListOutgoingRelations(next_chunk, last chunk): %v", err)
	}
	if len(lastNext) != 0 {
		t.Fatalf("last chunk has %d next_chunk relations, want 0", len(lastNext))
	}
}
