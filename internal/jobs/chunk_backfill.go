package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"

	"github.com/simplyblock/postbrain/internal/chunking"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
)

// textEmbedder is the subset of embedding.EmbeddingService used by ChunkBackfillJob.
type textEmbedder interface {
	EmbedText(ctx context.Context, text string) ([]float32, error)
}

// chunkBackfillRow holds the minimal fields needed to create chunk memories.
type chunkBackfillRow struct {
	ID       uuid.UUID
	ScopeID  uuid.UUID
	AuthorID uuid.UUID
	Content  string
}

// chunkBackfillStore abstracts DB access for ChunkBackfillJob.
type chunkBackfillStore interface {
	fetchMemoriesWithoutChunks(ctx context.Context, batchSize, offset int) ([]chunkBackfillRow, error)
	fetchArtifactsWithoutChunks(ctx context.Context, batchSize, offset int) ([]chunkBackfillRow, error)
	createMemory(ctx context.Context, m *db.Memory) (*db.Memory, error)
}

// poolChunkBackfillStore implements chunkBackfillStore against a real pgxpool.
type poolChunkBackfillStore struct {
	pool *pgxpool.Pool
}

func (p *poolChunkBackfillStore) fetchMemoriesWithoutChunks(ctx context.Context, batchSize, offset int) ([]chunkBackfillRow, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, scope_id, author_id, content FROM memories
		 WHERE char_length(content) > $1
		   AND parent_memory_id IS NULL
		   AND NOT EXISTS (
		       SELECT 1 FROM memories c WHERE c.parent_memory_id = memories.id
		   )
		 ORDER BY created_at
		 LIMIT $2 OFFSET $3`,
		chunking.MinContentRunes, batchSize, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChunkBackfillRows(rows)
}

func (p *poolChunkBackfillStore) fetchArtifactsWithoutChunks(ctx context.Context, batchSize, offset int) ([]chunkBackfillRow, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT a.id, a.owner_scope_id, a.author_id, a.content FROM knowledge_artifacts a
		 WHERE char_length(a.content) > $1
		   AND NOT EXISTS (
		       SELECT 1 FROM memories m
		       WHERE m.source_ref LIKE 'artifact:' || a.id::text || ':chunk:%'
		   )
		 ORDER BY a.created_at
		 LIMIT $2 OFFSET $3`,
		chunking.MinContentRunes, batchSize, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChunkBackfillRows(rows)
}

func scanChunkBackfillRows(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]chunkBackfillRow, error) {
	var batch []chunkBackfillRow
	for rows.Next() {
		var r chunkBackfillRow
		if err := rows.Scan(&r.ID, &r.ScopeID, &r.AuthorID, &r.Content); err != nil {
			return nil, err
		}
		batch = append(batch, r)
	}
	return batch, rows.Err()
}

func (p *poolChunkBackfillStore) createMemory(ctx context.Context, m *db.Memory) (*db.Memory, error) {
	return db.CreateMemory(ctx, p.pool, m)
}

const defaultChunkBackfillBatchSize = 20

// ChunkBackfillJob scans large memories and knowledge artifacts that have no
// chunk children and creates overlapping chunk memories for each so that the
// recall path can surface specific passages via their own embeddings.
type ChunkBackfillJob struct {
	store     chunkBackfillStore
	embedder  textEmbedder
	batchSize int
}

// NewChunkBackfillJob creates a ChunkBackfillJob backed by pool.
// svc must be non-nil for embedding; if it is nil the job is a no-op.
// batchSize 0 defaults to 20 (smaller than summaries: each row spawns many embeds).
func NewChunkBackfillJob(pool *pgxpool.Pool, svc *embedding.EmbeddingService, batchSize int) *ChunkBackfillJob {
	if batchSize <= 0 {
		batchSize = defaultChunkBackfillBatchSize
	}
	j := &ChunkBackfillJob{
		store:     &poolChunkBackfillStore{pool: pool},
		batchSize: batchSize,
	}
	if svc != nil {
		j.embedder = svc
	}
	return j
}

// RunMemories backfills chunk children for large memories that have none yet.
func (j *ChunkBackfillJob) RunMemories(ctx context.Context) error {
	return j.runBatch(ctx, "memories", j.store.fetchMemoriesWithoutChunks, j.chunkMemory)
}

// RunArtifacts backfills chunk memories for large knowledge artifacts.
func (j *ChunkBackfillJob) RunArtifacts(ctx context.Context) error {
	return j.runBatch(ctx, "artifacts", j.store.fetchArtifactsWithoutChunks, j.chunkArtifact)
}

func (j *ChunkBackfillJob) runBatch(
	ctx context.Context,
	kind string,
	fetch func(context.Context, int, int) ([]chunkBackfillRow, error),
	process func(context.Context, chunkBackfillRow) int,
) error {
	offset := 0
	total := 0
	for {
		batch, err := fetch(ctx, j.batchSize, offset)
		if err != nil {
			return fmt.Errorf("chunk backfill %s: fetch at offset %d: %w", kind, offset, err)
		}
		if len(batch) == 0 {
			break
		}
		for _, r := range batch {
			total += process(ctx, r)
		}
		slog.Info("chunk backfill: batch processed",
			"kind", kind, "offset", offset, "count", len(batch), "chunks_created", total)
		if len(batch) < j.batchSize {
			break
		}
		offset += j.batchSize
	}
	slog.Info("chunk backfill: complete", "kind", kind, "total_chunks_created", total)
	return nil
}

// chunkMemory creates chunk children for a single large memory.
// Returns the number of chunks created.
func (j *ChunkBackfillJob) chunkMemory(ctx context.Context, r chunkBackfillRow) int {
	if j.embedder == nil {
		return 0
	}
	chunks := chunking.Chunk(r.Content, chunking.DefaultChunkRunes, chunking.DefaultOverlap)
	if len(chunks) <= 1 {
		return 0
	}
	created := 0
	parentID := r.ID
	for i, chunk := range chunks {
		if utf8.RuneCountInString(chunk) == 0 {
			continue
		}
		vec, err := j.embedder.EmbedText(ctx, chunk)
		if err != nil {
			slog.WarnContext(ctx, "chunk backfill: memory embed failed",
				"memory_id", r.ID, "chunk", i, "err", err)
			continue
		}
		v := pgvector.NewVector(vec)
		ref := fmt.Sprintf("chunk:%d", i)
		m := &db.Memory{
			MemoryType:      "semantic",
			ScopeID:         r.ScopeID,
			AuthorID:        r.AuthorID,
			Content:         chunk,
			ContentKind:     "text",
			Embedding:       &v,
			ParentMemoryID:  &parentID,
			SourceRef:       &ref,
			PromotionStatus: "none",
		}
		if _, err := j.store.createMemory(ctx, m); err != nil {
			slog.WarnContext(ctx, "chunk backfill: memory store failed",
				"memory_id", r.ID, "chunk", i, "err", err)
			continue
		}
		created++
	}
	return created
}

// chunkArtifact creates chunk memories for a single large knowledge artifact.
// Returns the number of chunks created.
func (j *ChunkBackfillJob) chunkArtifact(ctx context.Context, r chunkBackfillRow) int {
	if j.embedder == nil {
		return 0
	}
	chunks := chunking.Chunk(r.Content, chunking.DefaultChunkRunes, chunking.DefaultOverlap)
	if len(chunks) <= 1 {
		return 0
	}
	created := 0
	for i, chunk := range chunks {
		if utf8.RuneCountInString(chunk) == 0 {
			continue
		}
		vec, err := j.embedder.EmbedText(ctx, chunk)
		if err != nil {
			slog.WarnContext(ctx, "chunk backfill: artifact embed failed",
				"artifact_id", r.ID, "chunk", i, "err", err)
			continue
		}
		v := pgvector.NewVector(vec)
		ref := fmt.Sprintf("artifact:%s:chunk:%d", r.ID, i)
		m := &db.Memory{
			MemoryType:      "semantic",
			ScopeID:         r.ScopeID,
			AuthorID:        r.AuthorID,
			Content:         chunk,
			ContentKind:     "text",
			Embedding:       &v,
			SourceRef:       &ref,
			PromotionStatus: "none",
		}
		if _, err := j.store.createMemory(ctx, m); err != nil {
			slog.WarnContext(ctx, "chunk backfill: artifact chunk store failed",
				"artifact_id", r.ID, "chunk", i, "err", err)
			continue
		}
		created++
	}
	return created
}
