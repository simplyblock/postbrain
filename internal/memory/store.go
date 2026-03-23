package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
)

// embeddingService is the subset of embedding.EmbeddingService used by this package.
type embeddingService interface {
	EmbedText(ctx context.Context, text string) ([]float32, error)
	EmbedCode(ctx context.Context, text string) ([]float32, error)
	TextEmbedder() embeddingIface
	CodeEmbedder() embeddingIface // may return nil
}

// embeddingIface is the subset of embedding.Embedder needed here.
type embeddingIface interface {
	ModelSlug() string
	Dimensions() int
}

// embeddingServiceAdapter adapts *embedding.EmbeddingService to embeddingService.
type embeddingServiceAdapter struct {
	svc *embedding.EmbeddingService
}

func (a *embeddingServiceAdapter) EmbedText(ctx context.Context, text string) ([]float32, error) {
	return a.svc.EmbedText(ctx, text)
}

func (a *embeddingServiceAdapter) EmbedCode(ctx context.Context, text string) ([]float32, error) {
	return a.svc.EmbedCode(ctx, text)
}

func (a *embeddingServiceAdapter) TextEmbedder() embeddingIface {
	return a.svc.TextEmbedder()
}

func (a *embeddingServiceAdapter) CodeEmbedder() embeddingIface {
	return a.svc.CodeEmbedder()
}

// memoryDB abstracts the database operations used by Store so tests can inject
// a mock without requiring a real PostgreSQL connection.
type memoryDB interface {
	CreateMemory(ctx context.Context, m *db.Memory) (*db.Memory, error)
	FindNearDuplicates(ctx context.Context, scopeID uuid.UUID, embedding []float32, threshold float64, excludeID *uuid.UUID) ([]*db.Memory, error)
	UpdateMemoryContent(ctx context.Context, id uuid.UUID, content string, embedding, embeddingCode []float32, textModelID, codeModelID *uuid.UUID, contentKind string) (*db.Memory, error)
	SoftDeleteMemory(ctx context.Context, id uuid.UUID) error
	UpsertEntity(ctx context.Context, e *db.Entity) (*db.Entity, error)
	LinkMemoryToEntity(ctx context.Context, memoryID, entityID uuid.UUID, role string) error
}

// poolMemoryDB wraps a real pgxpool.Pool to implement memoryDB.
type poolMemoryDB struct {
	pool *pgxpool.Pool
}

func (p *poolMemoryDB) CreateMemory(ctx context.Context, m *db.Memory) (*db.Memory, error) {
	return db.CreateMemory(ctx, p.pool, m)
}

func (p *poolMemoryDB) FindNearDuplicates(ctx context.Context, scopeID uuid.UUID, embedding []float32, threshold float64, excludeID *uuid.UUID) ([]*db.Memory, error) {
	return db.FindNearDuplicates(ctx, p.pool, scopeID, embedding, threshold, excludeID)
}

func (p *poolMemoryDB) UpdateMemoryContent(ctx context.Context, id uuid.UUID, content string, embedding, embeddingCode []float32, textModelID, codeModelID *uuid.UUID, contentKind string) (*db.Memory, error) {
	return db.UpdateMemoryContent(ctx, p.pool, id, content, embedding, embeddingCode, textModelID, codeModelID, contentKind)
}

func (p *poolMemoryDB) SoftDeleteMemory(ctx context.Context, id uuid.UUID) error {
	return db.SoftDeleteMemory(ctx, p.pool, id)
}

func (p *poolMemoryDB) UpsertEntity(ctx context.Context, e *db.Entity) (*db.Entity, error) {
	return db.UpsertEntity(ctx, p.pool, e)
}

func (p *poolMemoryDB) LinkMemoryToEntity(ctx context.Context, memoryID, entityID uuid.UUID, role string) error {
	return db.LinkMemoryToEntity(ctx, p.pool, memoryID, entityID, role)
}

// Store provides memory CRUD and embedding operations.
type Store struct {
	pool     *pgxpool.Pool
	svc      embeddingService
	creator  memoryDB
	recallDB recallDB   // overridable for tests
	fanOut   fanOutFunc // overridable for tests
}

// NewStore creates a new Store backed by the given pool and embedding service.
func NewStore(pool *pgxpool.Pool, svc *embedding.EmbeddingService) *Store {
	return &Store{
		pool:    pool,
		svc:     &embeddingServiceAdapter{svc: svc},
		creator: &poolMemoryDB{pool: pool},
	}
}

// CreateInput holds the fields required to create a new memory.
type CreateInput struct {
	Content    string
	MemoryType string // semantic|episodic|procedural|working
	ScopeID    uuid.UUID
	AuthorID   uuid.UUID
	Importance float64 // 0–1, default 0.5
	SourceRef  *string
	Entities   []string // entity canonical names to link
	ExpiresIn  *int     // seconds; meaningful only for working memory
	Meta       []byte
}

// CreateResult is returned by Create.
type CreateResult struct {
	MemoryID uuid.UUID
	Action   string // "created" | "updated"
}

// Create embeds, deduplicates, and persists a memory.
func (s *Store) Create(ctx context.Context, input CreateInput) (*CreateResult, error) {
	if input.Importance == 0 {
		input.Importance = 0.5
	}

	// 1. Classify content kind.
	contentKind := embedding.ClassifyContent(input.Content, safeDeref(input.SourceRef))

	// 2. Embed text.
	textVec, err := s.svc.EmbedText(ctx, input.Content)
	if err != nil {
		return nil, fmt.Errorf("memory: embed text: %w", err)
	}
	if len(textVec) == 0 {
		return nil, fmt.Errorf("memory: embed text: embedding service returned empty vector (is the model available?)")
	}

	// 3. Embed code if content_kind == "code" and a code embedder is available.
	var codeVec []float32
	if contentKind == "code" && s.svc.CodeEmbedder() != nil {
		codeVec, err = s.svc.EmbedCode(ctx, input.Content)
		if err != nil {
			return nil, fmt.Errorf("memory: embed code: %w", err)
		}
	}

	// 4. TTL logic.
	var expiresAt *time.Time
	if input.MemoryType == "working" {
		ttl := 3600
		if input.ExpiresIn != nil {
			ttl = *input.ExpiresIn
		}
		t := time.Now().Add(time.Duration(ttl) * time.Second)
		expiresAt = &t
	}

	// 5. Near-duplicate check.
	dupes, err := s.creator.FindNearDuplicates(ctx, input.ScopeID, textVec, 0.05, nil)
	if err != nil {
		return nil, fmt.Errorf("memory: find near duplicates: %w", err)
	}
	if len(dupes) > 0 {
		existing := dupes[0]
		updated, err := s.creator.UpdateMemoryContent(ctx, existing.ID, input.Content, textVec, codeVec, nil, nil, contentKind)
		if err != nil {
			return nil, fmt.Errorf("memory: update duplicate: %w", err)
		}
		return &CreateResult{MemoryID: updated.ID, Action: "updated"}, nil
	}

	// 6. Insert.
	textVecVal := pgvector.NewVector(textVec)
	codeVecVal := pgvector.NewVector(codeVec)
	m := &db.Memory{
		MemoryType:    input.MemoryType,
		ScopeID:       input.ScopeID,
		AuthorID:      input.AuthorID,
		Content:       input.Content,
		Embedding:     &textVecVal,
		EmbeddingCode: &codeVecVal,
		ContentKind:   contentKind,
		Meta:          input.Meta,
		Importance:    input.Importance,
		ExpiresAt:     expiresAt,
		SourceRef:     input.SourceRef,
	}
	created, err := s.creator.CreateMemory(ctx, m)
	if err != nil {
		return nil, fmt.Errorf("memory: create: %w", err)
	}

	// 7. Link entities.
	for _, canonical := range input.Entities {
		entity := &db.Entity{
			ScopeID:    input.ScopeID,
			EntityType: "concept",
			Name:       canonical,
			Canonical:  canonical,
		}
		ent, err := s.creator.UpsertEntity(ctx, entity)
		if err != nil {
			return nil, fmt.Errorf("memory: upsert entity %q: %w", canonical, err)
		}
		if err := s.creator.LinkMemoryToEntity(ctx, created.ID, ent.ID, "related"); err != nil {
			return nil, fmt.Errorf("memory: link entity %q: %w", canonical, err)
		}
	}

	return &CreateResult{MemoryID: created.ID, Action: "created"}, nil
}

// Update re-embeds and persists updated content for a memory.
func (s *Store) Update(ctx context.Context, id uuid.UUID, content string, importance float64) (*db.Memory, error) {
	contentKind := embedding.ClassifyContent(content, "")

	textVec, err := s.svc.EmbedText(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("memory: update embed text: %w", err)
	}
	if len(textVec) == 0 {
		return nil, fmt.Errorf("memory: update embed text: embedding service returned empty vector (is the model available?)")
	}

	var codeVec []float32
	if contentKind == "code" && s.svc.CodeEmbedder() != nil {
		codeVec, err = s.svc.EmbedCode(ctx, content)
		if err != nil {
			return nil, fmt.Errorf("memory: update embed code: %w", err)
		}
	}

	return s.creator.UpdateMemoryContent(ctx, id, content, textVec, codeVec, nil, nil, contentKind)
}

// SoftDelete marks a memory as inactive.
func (s *Store) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return s.creator.SoftDeleteMemory(ctx, id)
}

// HardDelete permanently removes a memory.
func (s *Store) HardDelete(ctx context.Context, id uuid.UUID) error {
	if s.pool == nil {
		return fmt.Errorf("memory: hard delete: pool is nil")
	}
	return db.HardDeleteMemory(ctx, s.pool, id)
}

// safeDeref returns "" if s is nil, else *s.
func safeDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
