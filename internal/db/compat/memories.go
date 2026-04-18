package compat

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/simplyblock/postbrain/internal/db"
)

// GetMemory retrieves a memory by ID. Returns nil, nil if not found.
func GetMemory(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*db.Memory, error) {
	q := db.New(pool)
	m, err := q.GetMemory(ctx, id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get memory: %w", err)
	}
	return memoryFromGetMemoryRow(m), nil
}

// ListMemoriesByScope returns active memories for a scope.
func ListMemoriesByScope(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, limit, offset int) ([]*db.Memory, error) {
	if offset < 0 && offset > math.MaxInt32 {
		return nil, fmt.Errorf("sharing: invalid offset: %d", offset)
	}
	q := db.New(pool)
	ms, err := q.ListMemoriesByScope(ctx, db.ListMemoriesByScopeParams{
		ScopeID: scopeID,
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("db: list memories by scope: %w", err)
	}
	out := make([]*db.Memory, len(ms))
	for i, r := range ms {
		out[i] = memoryFromListMemoriesByScopeRow(r)
	}
	return out, nil
}

// CreateMemory inserts a new memory record.
func CreateMemory(ctx context.Context, pool *pgxpool.Pool, m *db.Memory) (*db.Memory, error) {
	if m.Meta == nil {
		m.Meta = []byte("{}")
	}
	if m.ContentKind == "" {
		m.ContentKind = "text"
	}
	if m.PromotionStatus == "" {
		m.PromotionStatus = "none"
	}

	var version interface{}
	if m.Version != 0 {
		version = m.Version
	}
	var confidence interface{}
	if m.Confidence != 0 {
		confidence = m.Confidence
	}
	var importance interface{}
	if m.Importance != 0 {
		importance = m.Importance
	}

	q := db.New(pool)
	created, err := q.CreateMemory(ctx, db.CreateMemoryParams{
		MemoryType:           m.MemoryType,
		ScopeID:              m.ScopeID,
		AuthorID:             m.AuthorID,
		Content:              m.Content,
		Summary:              m.Summary,
		Embedding:            m.Embedding,
		EmbeddingModelID:     m.EmbeddingModelID,
		EmbeddingCode:        m.EmbeddingCode,
		EmbeddingCodeModelID: m.EmbeddingCodeModelID,
		ContentKind:          m.ContentKind,
		Meta:                 m.Meta,
		Column12:             version,
		Column13:             confidence,
		Column14:             importance,
		ExpiresAt:            m.ExpiresAt,
		PromotionStatus:      m.PromotionStatus,
		PromotedTo:           m.PromotedTo,
		SourceRef:            m.SourceRef,
		ParentMemoryID:       m.ParentMemoryID,
	})
	if err != nil {
		return nil, fmt.Errorf("db: create memory: %w", err)
	}
	return memoryFromCreateMemoryRow(created), nil
}

// UpdateMemoryContent updates content, summary, metadata, and embeddings.
func UpdateMemoryContent(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, content string, summary *string, embedding, embeddingCode []float32, textModelID, codeModelID *uuid.UUID, contentKind string, meta []byte) (*db.Memory, error) {
	if meta == nil {
		meta = []byte("{}")
	}
	q := db.New(pool)
	m, err := q.UpdateMemoryContent(ctx, db.UpdateMemoryContentParams{
		ID:                   id,
		Content:              content,
		Embedding:            vecPtr(embedding),
		EmbeddingModelID:     textModelID,
		EmbeddingCode:        vecPtr(embeddingCode),
		EmbeddingCodeModelID: codeModelID,
		ContentKind:          contentKind,
		Summary:              summary,
		Meta:                 meta,
	})
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: update memory content: %w", err)
	}
	return memoryFromUpdateMemoryContentRow(m), nil
}

// SoftDeleteMemory marks a memory as inactive.
func SoftDeleteMemory(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	q := db.New(pool)
	return q.SoftDeleteMemory(ctx, id)
}

// HardDeleteMemory permanently deletes a memory.
func HardDeleteMemory(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	q := db.New(pool)
	return q.HardDeleteMemory(ctx, id)
}

// IncrementMemoryAccess increments access_count and sets last_accessed.
func IncrementMemoryAccess(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	q := db.New(pool)
	return q.IncrementMemoryAccess(ctx, id)
}

// FindNearDuplicates finds active memories with cosine distance <= threshold.
func FindNearDuplicates(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, embedding []float32, threshold float64, excludeID *uuid.UUID) ([]*db.Memory, error) {
	var excl uuid.UUID
	if excludeID != nil {
		excl = *excludeID
	}
	q := db.New(pool)
	ms, err := q.FindNearDuplicates(ctx, db.FindNearDuplicatesParams{
		ScopeID:   scopeID,
		Embedding: vecPtr(embedding),
		Column3:   threshold,
		Column4:   excl,
	})
	if err != nil {
		return nil, fmt.Errorf("db: find near duplicates: %w", err)
	}
	out := make([]*db.Memory, len(ms))
	for i, r := range ms {
		out[i] = memoryFromFindNearDuplicatesRow(r)
	}
	return out, nil
}

// RecallMemoriesByVector performs ANN search.
func RecallMemoriesByVector(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, queryVec []float32, limit int, since, until *time.Time) ([]db.MemoryScore, error) {
	q := db.New(pool)
	lower, upper := normalizeRecallWindowBounds(since, until)
	rows, err := q.RecallMemoriesByVector(ctx, db.RecallMemoriesByVectorParams{
		Column1:   scopeIDs,
		Limit:     int32(limit),
		Embedding: vecPtr(queryVec),
		Column4:   lower,
		Column5:   upper,
	})
	if err != nil {
		return nil, fmt.Errorf("db: recall memories by vector: %w", err)
	}
	results := make([]db.MemoryScore, len(rows))
	for i, r := range rows {
		mem := memoryFromRecallByVectorRow(r)
		results[i] = db.MemoryScore{
			Memory:   mem,
			VecScore: float64(r.VecScore),
		}
	}
	return results, nil
}

// RecallMemoriesByCodeVector performs ANN on embedding_code.
func RecallMemoriesByCodeVector(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, queryVec []float32, limit int, since, until *time.Time) ([]db.MemoryScore, error) {
	q := db.New(pool)
	lower, upper := normalizeRecallWindowBounds(since, until)
	rows, err := q.RecallMemoriesByCodeVector(ctx, db.RecallMemoriesByCodeVectorParams{
		Column1:       scopeIDs,
		Limit:         int32(limit),
		EmbeddingCode: vecPtr(queryVec),
		Column4:       lower,
		Column5:       upper,
	})
	if err != nil {
		return nil, fmt.Errorf("db: recall memories by code vector: %w", err)
	}
	results := make([]db.MemoryScore, len(rows))
	for i, r := range rows {
		mem := memoryFromRecallByCodeVectorRow(r)
		results[i] = db.MemoryScore{
			Memory:   mem,
			VecScore: float64(r.VecScore),
		}
	}
	return results, nil
}

// RecallMemoriesByFTS performs BM25 full-text search.
func RecallMemoriesByFTS(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, query string, limit int, since, until *time.Time) ([]db.MemoryScore, error) {
	q := db.New(pool)
	lower, upper := normalizeRecallWindowBounds(since, until)
	rows, err := q.RecallMemoriesByFTS(ctx, db.RecallMemoriesByFTSParams{
		Column1:        scopeIDs,
		Limit:          int32(limit),
		PlaintoTsquery: query,
		Column4:        lower,
		Column5:        upper,
	})
	if err != nil {
		return nil, fmt.Errorf("db: recall memories by fts: %w", err)
	}
	results := make([]db.MemoryScore, len(rows))
	for i, r := range rows {
		mem := memoryFromRecallByFTSRow(r)
		results[i] = db.MemoryScore{
			Memory:    mem,
			BM25Score: float64(r.Bm25Score),
		}
	}
	return results, nil
}

// RecallMemoriesByTrigram performs trigram similarity recall.
func RecallMemoriesByTrigram(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, query string, limit int, since, until *time.Time) ([]db.MemoryScore, error) {
	q := db.New(pool)
	lower, upper := normalizeRecallWindowBounds(since, until)
	rows, err := q.RecallMemoriesByTrigram(ctx, db.RecallMemoriesByTrigramParams{
		Column1:    scopeIDs,
		Limit:      int32(limit),
		Similarity: query,
		Column4:    lower,
		Column5:    upper,
	})
	if err != nil {
		return nil, fmt.Errorf("db: recall memories by trigram: %w", err)
	}
	results := make([]db.MemoryScore, len(rows))
	for i, r := range rows {
		mem := memoryFromRecallByTrigramRow(r)
		results[i] = db.MemoryScore{
			Memory:    mem,
			TrgmScore: float64(r.TrgmScore),
		}
	}
	return results, nil
}

// ListChunkMemories returns chunk memories (children) for a given parent memory.
func ListChunkMemories(ctx context.Context, pool *pgxpool.Pool, parentMemoryID uuid.UUID) ([]*db.Memory, error) {
	q := db.New(pool)
	rows, err := q.ListChunkMemories(ctx, &parentMemoryID)
	if err != nil {
		return nil, fmt.Errorf("db: list chunk memories: %w", err)
	}
	out := make([]*db.Memory, len(rows))
	for i, r := range rows {
		out[i] = memoryFromListChunkMemoriesRow(r)
	}
	return out, nil
}

// ListConsolidationCandidates returns low-importance, low-access memories.
func ListConsolidationCandidates(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID) ([]*db.Memory, error) {
	q := db.New(pool)
	ms, err := q.ListConsolidationCandidates(ctx, scopeID)
	if err != nil {
		return nil, fmt.Errorf("db: list consolidation candidates: %w", err)
	}
	out := make([]*db.Memory, len(ms))
	for i, r := range ms {
		out[i] = memoryFromListConsolidationCandidatesRow(r)
	}
	return out, nil
}

// CreateConsolidation inserts a consolidation record.
func CreateConsolidation(ctx context.Context, pool *pgxpool.Pool, c *db.Consolidation) (*db.Consolidation, error) {
	q := db.New(pool)
	result, err := q.CreateConsolidation(ctx, db.CreateConsolidationParams{
		ScopeID:   c.ScopeID,
		SourceIds: c.SourceIds,
		ResultID:  c.ResultID,
		Strategy:  c.Strategy,
		Reason:    c.Reason,
	})
	if err != nil {
		return nil, fmt.Errorf("db: create consolidation: %w", err)
	}
	return result, nil
}

// ── private row converters ────────────────────────────────────────────────────

func memoryFromGetMemoryRow(r *db.GetMemoryRow) *db.Memory {
	return &db.Memory{
		ID:                   r.ID,
		MemoryType:           r.MemoryType,
		ScopeID:              r.ScopeID,
		AuthorID:             r.AuthorID,
		Content:              r.Content,
		Summary:              r.Summary,
		Embedding:            r.Embedding,
		EmbeddingModelID:     r.EmbeddingModelID,
		EmbeddingCode:        r.EmbeddingCode,
		EmbeddingCodeModelID: r.EmbeddingCodeModelID,
		ContentKind:          r.ContentKind,
		Meta:                 r.Meta,
		Version:              r.Version,
		IsActive:             r.IsActive,
		Confidence:           r.Confidence,
		Importance:           r.Importance,
		AccessCount:          r.AccessCount,
		LastAccessed:         r.LastAccessed,
		ExpiresAt:            r.ExpiresAt,
		PromotionStatus:      r.PromotionStatus,
		PromotedTo:           r.PromotedTo,
		SourceRef:            r.SourceRef,
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

func memoryFromCreateMemoryRow(r *db.CreateMemoryRow) *db.Memory {
	return &db.Memory{
		ID:                   r.ID,
		MemoryType:           r.MemoryType,
		ScopeID:              r.ScopeID,
		AuthorID:             r.AuthorID,
		Content:              r.Content,
		Summary:              r.Summary,
		Embedding:            r.Embedding,
		EmbeddingModelID:     r.EmbeddingModelID,
		EmbeddingCode:        r.EmbeddingCode,
		EmbeddingCodeModelID: r.EmbeddingCodeModelID,
		ContentKind:          r.ContentKind,
		Meta:                 r.Meta,
		Version:              r.Version,
		IsActive:             r.IsActive,
		Confidence:           r.Confidence,
		Importance:           r.Importance,
		AccessCount:          r.AccessCount,
		LastAccessed:         r.LastAccessed,
		ExpiresAt:            r.ExpiresAt,
		PromotionStatus:      r.PromotionStatus,
		PromotedTo:           r.PromotedTo,
		SourceRef:            r.SourceRef,
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

func memoryFromUpdateMemoryContentRow(r *db.UpdateMemoryContentRow) *db.Memory {
	return &db.Memory{
		ID:                   r.ID,
		MemoryType:           r.MemoryType,
		ScopeID:              r.ScopeID,
		AuthorID:             r.AuthorID,
		Content:              r.Content,
		Summary:              r.Summary,
		Embedding:            r.Embedding,
		EmbeddingModelID:     r.EmbeddingModelID,
		EmbeddingCode:        r.EmbeddingCode,
		EmbeddingCodeModelID: r.EmbeddingCodeModelID,
		ContentKind:          r.ContentKind,
		Meta:                 r.Meta,
		Version:              r.Version,
		IsActive:             r.IsActive,
		Confidence:           r.Confidence,
		Importance:           r.Importance,
		AccessCount:          r.AccessCount,
		LastAccessed:         r.LastAccessed,
		ExpiresAt:            r.ExpiresAt,
		PromotionStatus:      r.PromotionStatus,
		PromotedTo:           r.PromotedTo,
		SourceRef:            r.SourceRef,
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

func memoryFromFindNearDuplicatesRow(r *db.FindNearDuplicatesRow) *db.Memory {
	return &db.Memory{
		ID:                   r.ID,
		MemoryType:           r.MemoryType,
		ScopeID:              r.ScopeID,
		AuthorID:             r.AuthorID,
		Content:              r.Content,
		Summary:              r.Summary,
		Embedding:            r.Embedding,
		EmbeddingModelID:     r.EmbeddingModelID,
		EmbeddingCode:        r.EmbeddingCode,
		EmbeddingCodeModelID: r.EmbeddingCodeModelID,
		ContentKind:          r.ContentKind,
		Meta:                 r.Meta,
		Version:              r.Version,
		IsActive:             r.IsActive,
		Confidence:           r.Confidence,
		Importance:           r.Importance,
		AccessCount:          r.AccessCount,
		LastAccessed:         r.LastAccessed,
		ExpiresAt:            r.ExpiresAt,
		PromotionStatus:      r.PromotionStatus,
		PromotedTo:           r.PromotedTo,
		SourceRef:            r.SourceRef,
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

func memoryFromListMemoriesByScopeRow(r *db.ListMemoriesByScopeRow) *db.Memory {
	return &db.Memory{
		ID:                   r.ID,
		MemoryType:           r.MemoryType,
		ScopeID:              r.ScopeID,
		AuthorID:             r.AuthorID,
		Content:              r.Content,
		Summary:              r.Summary,
		Embedding:            r.Embedding,
		EmbeddingModelID:     r.EmbeddingModelID,
		EmbeddingCode:        r.EmbeddingCode,
		EmbeddingCodeModelID: r.EmbeddingCodeModelID,
		ContentKind:          r.ContentKind,
		Meta:                 r.Meta,
		Version:              r.Version,
		IsActive:             r.IsActive,
		Confidence:           r.Confidence,
		Importance:           r.Importance,
		AccessCount:          r.AccessCount,
		LastAccessed:         r.LastAccessed,
		ExpiresAt:            r.ExpiresAt,
		PromotionStatus:      r.PromotionStatus,
		PromotedTo:           r.PromotedTo,
		SourceRef:            r.SourceRef,
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

func memoryFromListConsolidationCandidatesRow(r *db.ListConsolidationCandidatesRow) *db.Memory {
	return &db.Memory{
		ID:                   r.ID,
		MemoryType:           r.MemoryType,
		ScopeID:              r.ScopeID,
		AuthorID:             r.AuthorID,
		Content:              r.Content,
		Summary:              r.Summary,
		Embedding:            r.Embedding,
		EmbeddingModelID:     r.EmbeddingModelID,
		EmbeddingCode:        r.EmbeddingCode,
		EmbeddingCodeModelID: r.EmbeddingCodeModelID,
		ContentKind:          r.ContentKind,
		Meta:                 r.Meta,
		Version:              r.Version,
		IsActive:             r.IsActive,
		Confidence:           r.Confidence,
		Importance:           r.Importance,
		AccessCount:          r.AccessCount,
		LastAccessed:         r.LastAccessed,
		ExpiresAt:            r.ExpiresAt,
		PromotionStatus:      r.PromotionStatus,
		PromotedTo:           r.PromotedTo,
		SourceRef:            r.SourceRef,
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

func memoryFromListMemoriesForEntityRow(r *db.ListMemoriesForEntityRow) *db.Memory {
	return &db.Memory{
		ID:                   r.ID,
		MemoryType:           r.MemoryType,
		ScopeID:              r.ScopeID,
		AuthorID:             r.AuthorID,
		Content:              r.Content,
		Summary:              r.Summary,
		Embedding:            r.Embedding,
		EmbeddingModelID:     r.EmbeddingModelID,
		EmbeddingCode:        r.EmbeddingCode,
		EmbeddingCodeModelID: r.EmbeddingCodeModelID,
		ContentKind:          r.ContentKind,
		Meta:                 r.Meta,
		Version:              r.Version,
		IsActive:             r.IsActive,
		Confidence:           r.Confidence,
		Importance:           r.Importance,
		AccessCount:          r.AccessCount,
		LastAccessed:         r.LastAccessed,
		ExpiresAt:            r.ExpiresAt,
		PromotionStatus:      r.PromotionStatus,
		PromotedTo:           r.PromotedTo,
		SourceRef:            r.SourceRef,
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

func memoryFromListChunkMemoriesRow(r *db.ListChunkMemoriesRow) *db.Memory {
	return &db.Memory{
		ID:                   r.ID,
		MemoryType:           r.MemoryType,
		ScopeID:              r.ScopeID,
		AuthorID:             r.AuthorID,
		Content:              r.Content,
		Summary:              r.Summary,
		Embedding:            r.Embedding,
		EmbeddingModelID:     r.EmbeddingModelID,
		EmbeddingCode:        r.EmbeddingCode,
		EmbeddingCodeModelID: r.EmbeddingCodeModelID,
		ContentKind:          r.ContentKind,
		Meta:                 r.Meta,
		Version:              r.Version,
		IsActive:             r.IsActive,
		Confidence:           r.Confidence,
		Importance:           r.Importance,
		AccessCount:          r.AccessCount,
		LastAccessed:         r.LastAccessed,
		ExpiresAt:            r.ExpiresAt,
		PromotionStatus:      r.PromotionStatus,
		PromotedTo:           r.PromotedTo,
		SourceRef:            r.SourceRef,
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

func memoryFromRecallByVectorRow(r *db.RecallMemoriesByVectorRow) *db.Memory {
	return &db.Memory{
		ID:                   r.ID,
		MemoryType:           r.MemoryType,
		ScopeID:              r.ScopeID,
		AuthorID:             r.AuthorID,
		Content:              r.Content,
		Summary:              r.Summary,
		Embedding:            r.Embedding,
		EmbeddingModelID:     r.EmbeddingModelID,
		EmbeddingCode:        r.EmbeddingCode,
		EmbeddingCodeModelID: r.EmbeddingCodeModelID,
		ContentKind:          r.ContentKind,
		Meta:                 r.Meta,
		Version:              r.Version,
		IsActive:             r.IsActive,
		Confidence:           r.Confidence,
		Importance:           r.Importance,
		AccessCount:          r.AccessCount,
		LastAccessed:         r.LastAccessed,
		ExpiresAt:            r.ExpiresAt,
		PromotionStatus:      r.PromotionStatus,
		PromotedTo:           r.PromotedTo,
		SourceRef:            r.SourceRef,
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

func memoryFromRecallByCodeVectorRow(r *db.RecallMemoriesByCodeVectorRow) *db.Memory {
	return &db.Memory{
		ID:                   r.ID,
		MemoryType:           r.MemoryType,
		ScopeID:              r.ScopeID,
		AuthorID:             r.AuthorID,
		Content:              r.Content,
		Summary:              r.Summary,
		Embedding:            r.Embedding,
		EmbeddingModelID:     r.EmbeddingModelID,
		EmbeddingCode:        r.EmbeddingCode,
		EmbeddingCodeModelID: r.EmbeddingCodeModelID,
		ContentKind:          r.ContentKind,
		Meta:                 r.Meta,
		Version:              r.Version,
		IsActive:             r.IsActive,
		Confidence:           r.Confidence,
		Importance:           r.Importance,
		AccessCount:          r.AccessCount,
		LastAccessed:         r.LastAccessed,
		ExpiresAt:            r.ExpiresAt,
		PromotionStatus:      r.PromotionStatus,
		PromotedTo:           r.PromotedTo,
		SourceRef:            r.SourceRef,
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

func memoryFromRecallByFTSRow(r *db.RecallMemoriesByFTSRow) *db.Memory {
	return &db.Memory{
		ID:                   r.ID,
		MemoryType:           r.MemoryType,
		ScopeID:              r.ScopeID,
		AuthorID:             r.AuthorID,
		Content:              r.Content,
		Summary:              r.Summary,
		Embedding:            r.Embedding,
		EmbeddingModelID:     r.EmbeddingModelID,
		EmbeddingCode:        r.EmbeddingCode,
		EmbeddingCodeModelID: r.EmbeddingCodeModelID,
		ContentKind:          r.ContentKind,
		Meta:                 r.Meta,
		Version:              r.Version,
		IsActive:             r.IsActive,
		Confidence:           r.Confidence,
		Importance:           r.Importance,
		AccessCount:          r.AccessCount,
		LastAccessed:         r.LastAccessed,
		ExpiresAt:            r.ExpiresAt,
		PromotionStatus:      r.PromotionStatus,
		PromotedTo:           r.PromotedTo,
		SourceRef:            r.SourceRef,
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

func memoryFromRecallByTrigramRow(r *db.RecallMemoriesByTrigramRow) *db.Memory {
	return &db.Memory{
		ID:                   r.ID,
		MemoryType:           r.MemoryType,
		ScopeID:              r.ScopeID,
		AuthorID:             r.AuthorID,
		Content:              r.Content,
		Summary:              r.Summary,
		Embedding:            r.Embedding,
		EmbeddingModelID:     r.EmbeddingModelID,
		EmbeddingCode:        r.EmbeddingCode,
		EmbeddingCodeModelID: r.EmbeddingCodeModelID,
		ContentKind:          r.ContentKind,
		Meta:                 r.Meta,
		Version:              r.Version,
		IsActive:             r.IsActive,
		Confidence:           r.Confidence,
		Importance:           r.Importance,
		AccessCount:          r.AccessCount,
		LastAccessed:         r.LastAccessed,
		ExpiresAt:            r.ExpiresAt,
		PromotionStatus:      r.PromotionStatus,
		PromotedTo:           r.PromotedTo,
		SourceRef:            r.SourceRef,
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}
