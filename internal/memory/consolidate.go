package memory

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
)

// consolidatorDB abstracts the DB operations needed by Consolidator.
type consolidatorDB interface {
	ListConsolidationCandidates(ctx context.Context, scopeID uuid.UUID) ([]*db.Memory, error)
	CreateMemory(ctx context.Context, m *db.Memory) (*db.Memory, error)
	SoftDeleteMemory(ctx context.Context, id uuid.UUID) error
	CreateConsolidation(ctx context.Context, c *db.Consolidation) (*db.Consolidation, error)
}

// poolConsolidatorDB wraps *pgxpool.Pool to implement consolidatorDB.
type poolConsolidatorDB struct {
	pool *pgxpool.Pool
}

func (p *poolConsolidatorDB) ListConsolidationCandidates(ctx context.Context, scopeID uuid.UUID) ([]*db.Memory, error) {
	return db.ListConsolidationCandidates(ctx, p.pool, scopeID)
}

func (p *poolConsolidatorDB) CreateMemory(ctx context.Context, m *db.Memory) (*db.Memory, error) {
	return db.CreateMemory(ctx, p.pool, m)
}

func (p *poolConsolidatorDB) SoftDeleteMemory(ctx context.Context, id uuid.UUID) error {
	return db.SoftDeleteMemory(ctx, p.pool, id)
}

func (p *poolConsolidatorDB) CreateConsolidation(ctx context.Context, c *db.Consolidation) (*db.Consolidation, error) {
	return db.CreateConsolidation(ctx, p.pool, c)
}

// Consolidator merges near-duplicate memories within a scope.
type Consolidator struct {
	pool *pgxpool.Pool
	svc  embeddingService
	cdb  consolidatorDB // overridable for tests
}

// NewConsolidator creates a new Consolidator backed by the given pool and embedding service.
func NewConsolidator(pool *pgxpool.Pool, svc *embedding.EmbeddingService) *Consolidator {
	return &Consolidator{
		pool: pool,
		svc:  &embeddingServiceAdapter{svc: svc},
		cdb:  &poolConsolidatorDB{pool: pool},
	}
}

// FindClusters finds groups of near-duplicate memories in a scope.
// Two memories are in the same cluster if their cosine distance is <= 0.05.
// Returns clusters with >= 2 members.
func (c *Consolidator) FindClusters(ctx context.Context, scopeID uuid.UUID) ([][]*db.Memory, error) {
	cdb := c.cdb
	if cdb == nil {
		cdb = &poolConsolidatorDB{pool: c.pool}
	}

	candidates, err := cdb.ListConsolidationCandidates(ctx, scopeID)
	if err != nil {
		return nil, fmt.Errorf("consolidate: list candidates: %w", err)
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	// Build connected components using union-find.
	n := len(candidates)
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}

	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(x, y int) {
		px, py := find(x), find(y)
		if px != py {
			parent[px] = py
		}
	}

	const threshold = 0.05
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if cosineDist(candidates[i].Embedding.Slice(), candidates[j].Embedding.Slice()) <= threshold {
				union(i, j)
			}
		}
	}

	// Group by component root.
	groups := make(map[int][]*db.Memory)
	for i, m := range candidates {
		root := find(i)
		groups[root] = append(groups[root], m)
	}

	var clusters [][]*db.Memory
	for _, g := range groups {
		if len(g) >= 2 {
			clusters = append(clusters, g)
		}
	}
	return clusters, nil
}

// MergeCluster merges a cluster of memories into a single semantic memory.
// summarizer is an injected function that produces a summary from multiple content strings.
func (c *Consolidator) MergeCluster(ctx context.Context, cluster []*db.Memory, summarizer func(ctx context.Context, contents []string) (string, error)) (*db.Memory, error) {
	cdb := c.cdb
	if cdb == nil {
		cdb = &poolConsolidatorDB{pool: c.pool}
	}

	// 1. Collect contents and find max importance.
	contents := make([]string, len(cluster))
	maxImportance := 0.0
	sourceIDs := make([]uuid.UUID, len(cluster))
	for i, m := range cluster {
		contents[i] = m.Content
		if m.Importance > maxImportance {
			maxImportance = m.Importance
		}
		sourceIDs[i] = m.ID
	}

	// 2. Call summarizer.
	summary, err := summarizer(ctx, contents)
	if err != nil {
		return nil, fmt.Errorf("consolidate: summarize: %w", err)
	}

	// 3. Re-embed the summary.
	vec, err := c.svc.EmbedText(ctx, summary)
	if err != nil {
		return nil, fmt.Errorf("consolidate: embed summary: %w", err)
	}

	// 4. Create new semantic memory.
	scopeID := cluster[0].ScopeID
	authorID := cluster[0].AuthorID
	newMem := &db.Memory{
		MemoryType: "semantic",
		ScopeID:    scopeID,
		AuthorID:   authorID,
		Content:    summary,
		Embedding:  pgvector.NewVector(vec),
		Importance: maxImportance,
	}
	created, err := cdb.CreateMemory(ctx, newMem)
	if err != nil {
		return nil, fmt.Errorf("consolidate: create merged memory: %w", err)
	}

	// 5. Soft-delete source memories.
	for _, m := range cluster {
		if err := cdb.SoftDeleteMemory(ctx, m.ID); err != nil {
			return nil, fmt.Errorf("consolidate: soft delete source %v: %w", m.ID, err)
		}
	}

	// 6. Create consolidation record.
	resultID := created.ID
	_, err = cdb.CreateConsolidation(ctx, &db.Consolidation{
		ScopeID:   scopeID,
		SourceIds: sourceIDs,
		ResultID:  resultID,
		Strategy:  "merge",
	})
	if err != nil {
		return nil, fmt.Errorf("consolidate: create consolidation record: %w", err)
	}

	return created, nil
}

// cosineDist computes the cosine distance between two vectors.
// Returns 2.0 if either vector is empty/nil.
func cosineDist(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 2.0
	}
	var dot, normA, normB float64
	for i := range a {
		if i >= len(b) {
			break
		}
		fa, fb := float64(a[i]), float64(b[i])
		dot += fa * fb
		normA += fa * fa
		normB += fb * fb
	}
	if normA == 0 || normB == 0 {
		return 2.0
	}
	return 1.0 - dot/(math.Sqrt(normA)*math.Sqrt(normB))
}
